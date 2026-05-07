package combat

import (
	"context"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// TestHighestThreat_PicksLargestContributor verifies the row-max
// lookup with a deterministic tie break (ascending ActorRef.ID).
func TestHighestThreat_PicksLargestContributor(t *testing.T) {
	defender := ActorRef{Kind: ActorKindMob, ID: 100}
	atkA := ActorRef{Kind: ActorKindCharacter, ID: 1}
	atkB := ActorRef{Kind: ActorKindCharacter, ID: 2}

	cases := []struct {
		name   string
		row    map[ActorRef]int32
		expect ActorRef
	}{
		{
			name:   "single attacker",
			row:    map[ActorRef]int32{atkA: 5},
			expect: atkA,
		},
		{
			name:   "larger contributor wins",
			row:    map[ActorRef]int32{atkA: 5, atkB: 10},
			expect: atkB,
		},
		{
			name:   "tie breaks by ascending ID",
			row:    map[ActorRef]int32{atkA: 7, atkB: 7},
			expect: atkA,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &Fight{Threat: map[ActorRef]map[ActorRef]int32{defender: tc.row}}
			if got := f.HighestThreat(defender); got != tc.expect {
				t.Fatalf("HighestThreat = %+v, want %+v", got, tc.expect)
			}
		})
	}
}

// TestHighestThreat_UnknownDefenderReturnsZero asserts the empty-map
// and missing-defender paths both return the zero ActorRef rather
// than panicking on a nil row.
func TestHighestThreat_UnknownDefenderReturnsZero(t *testing.T) {
	defender := ActorRef{Kind: ActorKindMob, ID: 100}
	other := ActorRef{Kind: ActorKindMob, ID: 200}

	t.Run("nil map", func(t *testing.T) {
		f := &Fight{}
		if got := f.HighestThreat(defender); got != (ActorRef{}) {
			t.Fatalf("HighestThreat on nil map = %+v, want zero", got)
		}
	})
	t.Run("missing defender row", func(t *testing.T) {
		f := &Fight{Threat: map[ActorRef]map[ActorRef]int32{
			other: {{Kind: ActorKindCharacter, ID: 1}: 5},
		}}
		if got := f.HighestThreat(defender); got != (ActorRef{}) {
			t.Fatalf("HighestThreat on missing row = %+v, want zero", got)
		}
	})
	t.Run("empty defender row", func(t *testing.T) {
		f := &Fight{Threat: map[ActorRef]map[ActorRef]int32{defender: {}}}
		if got := f.HighestThreat(defender); got != (ActorRef{}) {
			t.Fatalf("HighestThreat on empty row = %+v, want zero", got)
		}
	})
}

// TestPruneDead_ClearsThreatBothDirections is the regression guard
// for the bidirectional prune: a removed actor must vanish from
// every defender row (column) AND its own defender row (row).
func TestPruneDead_ClearsThreatBothDirections(t *testing.T) {
	a := ActorRef{Kind: ActorKindCharacter, ID: 1}
	b := ActorRef{Kind: ActorKindCharacter, ID: 2}
	c := ActorRef{Kind: ActorKindMob, ID: 3}

	f := &Fight{
		Order: []ActorEntry{{Ref: a}, {Ref: b}, {Ref: c}},
		Threat: map[ActorRef]map[ActorRef]int32{
			// a is a defender being attacked by b and c
			a: {b: 7, c: 3},
			// c is a defender being attacked by a and b
			c: {a: 5, b: 4},
		},
		Dead: map[ActorRef]struct{}{a: {}},
	}

	if !f.pruneDead() {
		t.Fatal("pruneDead returned false; expected true with one Dead actor")
	}

	if _, ok := f.Threat[a]; ok {
		t.Fatal("defender row for dead actor a still present")
	}
	if _, ok := f.Threat[c][a]; ok {
		t.Fatal("attacker column for dead actor a still present in c's row")
	}
	if got := f.Threat[c][b]; got != 4 {
		t.Fatalf("non-dead column wrongly affected: c[b] = %d, want 4", got)
	}
}

// TestThreat_DamageBumpsRow drives a real attack through Tick and
// asserts the per-defender threat row mirrors DamageTally for the
// attacker. Closes the integration gap left by the unit tests above.
func TestThreat_DamageBumpsRow(t *testing.T) {
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, BAB: 50, Defense: 10,
			Abilities: creature.Abilities{
				Str: creature.AbilityScore{Current: 18},
			},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{ExternalID: "dummy", ChallengeCode: 'A'})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "training dummy", HPCurrent: 999, HPMax: 999,
			Defense: 0, CurrentRoomID: 1,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, 1); err != nil {
		t.Fatalf("place mob: %v", err)
	}

	items := repo.NewMemoryItemRepo()
	mgr := New(bus, chars, mobs, templates, items)

	atkRef := ActorRef{Kind: ActorKindCharacter, ID: ch.ID}
	defRef := ActorRef{Kind: ActorKindMob, ID: mob.ID}
	if _, err := mgr.Start(ctx, 1, []ActorRef{atkRef, defRef}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Tick until Alice has acted at least once. Mob has no queued
	// action so its turn is a no-op; the dummy has 999 HP so won't
	// die. Bail once we observe a non-zero DamageTally entry for her.
	for i := 0; i < 6; i++ {
		_ = mgr.EnqueueAction(1, atkRef, Action{Kind: ActionAttack, Target: defRef})
		mgr.Tick(ctx)
		f, _ := mgr.Get(1)
		if f != nil && f.DamageTally[atkRef] > 0 {
			break
		}
	}

	f, ok := mgr.Get(1)
	if !ok {
		t.Fatal("fight ended unexpectedly")
	}
	dealt := f.DamageTally[atkRef]
	if dealt <= 0 {
		t.Fatalf("attacker dealt no damage after 6 ticks; tally=%v", f.DamageTally)
	}
	if got := f.Threat[defRef][atkRef]; got != dealt {
		t.Fatalf("Threat[def][atk] = %d, want %d (== DamageTally)", got, dealt)
	}
	if got := f.HighestThreat(defRef); got != atkRef {
		t.Fatalf("HighestThreat(def) = %+v, want %+v", got, atkRef)
	}
}
