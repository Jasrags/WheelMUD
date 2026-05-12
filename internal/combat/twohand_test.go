package combat

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// twoHandFixture seeds a character + mob in roomID. The character is
// equipped with a primary weapon and optionally an off-hand weapon.
// bab controls the iterative chain length. Returns the manager, the
// captured Hit/Miss events under a shared mutex, and the seeded ids
// so callers can assert on schedule / drop / cadence.
type twoHandRig struct {
	mgr           *Manager
	chars         *repo.MemoryCharacterRepo
	items         *repo.MemoryItemRepo
	charID        int64
	primaryID     int64
	offHandID     int64
	mobID         int64
	roomID        int64
	mu            *sync.Mutex
	hits          *[]CombatHit
	misses        *[]CombatMiss
	parts         []ActorRef
}

func newTwoHandRig(t *testing.T, bab int16, equipOffHand bool) *twoHandRig {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	roomID := int64(7)

	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice",
		CurrentRoomID: roomID,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, Defense: 0, BAB: bab,
			Abilities: creature.Abilities{Str: creature.AbilityScore{Current: 10}},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}

	items := repo.NewMemoryItemRepo()
	primary, err := items.Create(ctx, repo.Item{
		ExternalID: "test.twf.primary", Name: "a longsword",
		OwnerCharacterID: ch.ID, Type: repo.ItemTypeWeapon, Weight: 4.0,
		Stats: &repo.WeaponStats{Damage: "1d8", DamageType: []string{"S"}, Range: "melee"},
	})
	if err != nil {
		t.Fatalf("seed primary: %v", err)
	}
	eq := ch.Equipment
	eq.Set(creature.SlotPrimaryWield, primary.ID)
	var offHandID int64
	if equipOffHand {
		off, err := items.Create(ctx, repo.Item{
			ExternalID: "test.twf.offhand", Name: "a dagger",
			OwnerCharacterID: ch.ID, Type: repo.ItemTypeWeapon, Weight: 1.0,
			Stats: &repo.WeaponStats{Damage: "1d4", DamageType: []string{"P"}, Range: "melee"},
		})
		if err != nil {
			t.Fatalf("seed offhand: %v", err)
		}
		offHandID = off.ID
		eq.Set(creature.SlotOffHand, offHandID)
	}
	if err := chars.RecordEquipment(ctx, ch.ID, eq); err != nil {
		t.Fatalf("seed equip: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "twf.dummy", ChallengeCode: 'A',
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "dummy", HPCurrent: 1000, HPMax: 1000,
			Defense: 0, BAB: 0, CurrentRoomID: roomID,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, roomID); err != nil {
		t.Fatalf("place mob: %v", err)
	}

	mgr := New(bus, chars, mobs, templates, items)
	var mu sync.Mutex
	var hits []CombatHit
	var misses []CombatMiss
	eventbus.Subscribe[CombatHit](bus, func(_ context.Context, ev CombatHit) {
		mu.Lock()
		hits = append(hits, ev)
		mu.Unlock()
	})
	eventbus.Subscribe[CombatMiss](bus, func(_ context.Context, ev CombatMiss) {
		mu.Lock()
		misses = append(misses, ev)
		mu.Unlock()
	})

	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: ch.ID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, roomID, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}

	return &twoHandRig{
		mgr: mgr, chars: chars, items: items,
		charID: ch.ID, primaryID: primary.ID, offHandID: offHandID, mobID: mob.ID,
		roomID: roomID, mu: &mu, hits: &hits, misses: &misses, parts: parts,
	}
}

// TestSchedule_OffHandAppendsToChain verifies rollInitiative appends
// one off-hand SwingPlan per iterative bonus when SlotOffHand is
// wielded. Primary-only schedules are unchanged.
func TestSchedule_OffHandAppendsToChain(t *testing.T) {
	cases := []struct {
		name        string
		bab         int16
		offHand     bool
		wantSwings  []SwingPlan
	}{
		{
			name: "bab0_no_offhand",
			bab:  0, offHand: false,
			wantSwings: []SwingPlan{
				{Slot: creature.SlotPrimaryWield, Bonus: 0},
			},
		},
		{
			name: "bab0_with_offhand",
			bab:  0, offHand: true,
			wantSwings: []SwingPlan{
				{Slot: creature.SlotPrimaryWield, Bonus: 0},
				{Slot: creature.SlotOffHand, Bonus: -4},
			},
		},
		{
			name: "bab6_no_offhand",
			bab:  6, offHand: false,
			wantSwings: []SwingPlan{
				{Slot: creature.SlotPrimaryWield, Bonus: 0},
				{Slot: creature.SlotPrimaryWield, Bonus: -5},
			},
		},
		{
			name: "bab11_with_offhand",
			bab:  11, offHand: true,
			wantSwings: []SwingPlan{
				{Slot: creature.SlotPrimaryWield, Bonus: 0},
				{Slot: creature.SlotPrimaryWield, Bonus: -5},
				{Slot: creature.SlotPrimaryWield, Bonus: -10},
				{Slot: creature.SlotOffHand, Bonus: -4},
				{Slot: creature.SlotOffHand, Bonus: -9},
				{Slot: creature.SlotOffHand, Bonus: -14},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newTwoHandRig(t, tc.bab, tc.offHand)
			rig.mgr.mu.RLock()
			defer rig.mgr.mu.RUnlock()
			f := rig.mgr.fights[rig.roomID]
			var got ActorEntry
			for _, e := range f.Order {
				if e.Ref.Kind == ActorKindCharacter {
					got = e
					break
				}
			}
			if len(got.Swings) != len(tc.wantSwings) {
				t.Fatalf("Swings = %+v, want %+v", got.Swings, tc.wantSwings)
			}
			for i, sp := range got.Swings {
				if sp != tc.wantSwings[i] {
					t.Errorf("Swings[%d] = %+v, want %+v", i, sp, tc.wantSwings[i])
				}
			}
			if got.PendingSwings != len(tc.wantSwings) {
				t.Errorf("PendingSwings = %d, want %d", got.PendingSwings, len(tc.wantSwings))
			}
		})
	}
}

// TestTick_OffHandFiresAlongsidePrimary: with a dual-wielder queued
// against a 1000-HP target, one Tick must publish exactly one primary
// hit AND one off-hand hit, both keyed to the same attacker.
func TestTick_OffHandFiresAlongsidePrimary(t *testing.T) {
	rig := newTwoHandRig(t, 0, true)
	// Pin RNG after Start so the initiative roll is decoupled. d20=20
	// auto-hits even with the -4 off-hand penalty.
	rig.mgr.SetRNG(rand.New(rand.NewSource(stubSeedForRoll(20))))

	if err := rig.mgr.EnqueueAction(rig.roomID, rig.parts[0], Action{
		Kind: ActionAttack, Target: rig.parts[1], WeaponID: rig.primaryID,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	rig.mgr.Tick(context.Background())

	rig.mu.Lock()
	defer rig.mu.Unlock()
	if len(*rig.hits) != 2 {
		t.Fatalf("CombatHit = %d, want 2: %+v", len(*rig.hits), *rig.hits)
	}
	if (*rig.hits)[0].OffHand {
		t.Errorf("first hit OffHand = true, want false (primary fires first)")
	}
	if !(*rig.hits)[1].OffHand {
		t.Errorf("second hit OffHand = false, want true")
	}
	if (*rig.hits)[0].Weapon != rig.primaryID {
		t.Errorf("primary hit Weapon = %d, want %d", (*rig.hits)[0].Weapon, rig.primaryID)
	}
	if (*rig.hits)[1].Weapon != rig.offHandID {
		t.Errorf("off-hand hit Weapon = %d, want %d", (*rig.hits)[1].Weapon, rig.offHandID)
	}
}

// TestTick_OffHandUnwieldedMidFight: schedule built with off-hand;
// the player un-wields between Start and Tick. The off-hand swing
// quietly no-ops (no event); primary swing still publishes.
func TestTick_OffHandUnwieldedMidFight(t *testing.T) {
	rig := newTwoHandRig(t, 0, true)
	rig.mgr.SetRNG(rand.New(rand.NewSource(stubSeedForRoll(20))))

	// Un-wield off-hand between Start and Tick.
	ctx := context.Background()
	ch, err := rig.chars.GetByID(ctx, rig.charID)
	if err != nil {
		t.Fatalf("reload char: %v", err)
	}
	eq := ch.Equipment
	eq.Set(creature.SlotOffHand, 0)
	if err := rig.chars.RecordEquipment(ctx, rig.charID, eq); err != nil {
		t.Fatalf("unwield: %v", err)
	}

	if err := rig.mgr.EnqueueAction(rig.roomID, rig.parts[0], Action{
		Kind: ActionAttack, Target: rig.parts[1], WeaponID: rig.primaryID,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	rig.mgr.Tick(ctx)

	rig.mu.Lock()
	defer rig.mu.Unlock()
	if len(*rig.hits) != 1 {
		t.Fatalf("CombatHit = %d, want 1 (primary only): %+v", len(*rig.hits), *rig.hits)
	}
	if (*rig.hits)[0].OffHand {
		t.Errorf("the single hit should be primary, got OffHand=true")
	}
}

// TestFumble_OffHand_DropsOffHandWeapon: nat-1 on an off-hand swing
// drops the off-hand weapon, not primary. Primary slot stays wielded.
// Primary swing is forced to miss (low d20 vs high defense) so no
// damage roll consumes RNG between the two d20s — keeps the
// two-roll seed pinning deterministic.
func TestFumble_OffHand_DropsOffHandWeapon(t *testing.T) {
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	roomID := int64(7)

	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice",
		CurrentRoomID: roomID,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, Defense: 0, BAB: 0,
			Abilities: creature.Abilities{Str: creature.AbilityScore{Current: 10}},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}
	items := repo.NewMemoryItemRepo()
	primary, _ := items.Create(ctx, repo.Item{
		ExternalID: "tw.fumble.primary", Name: "primary",
		OwnerCharacterID: ch.ID, Type: repo.ItemTypeWeapon, Weight: 4.0,
		Stats: &repo.WeaponStats{Damage: "1d8", DamageType: []string{"S"}, Range: "melee"},
	})
	off, _ := items.Create(ctx, repo.Item{
		ExternalID: "tw.fumble.offhand", Name: "offhand",
		OwnerCharacterID: ch.ID, Type: repo.ItemTypeWeapon, Weight: 1.0,
		Stats: &repo.WeaponStats{Damage: "1d4", DamageType: []string{"P"}, Range: "melee"},
	})
	eq := ch.Equipment
	eq.Set(creature.SlotPrimaryWield, primary.ID)
	eq.Set(creature.SlotOffHand, off.ID)
	if err := chars.RecordEquipment(ctx, ch.ID, eq); err != nil {
		t.Fatalf("equip: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "tw.tank", ChallengeCode: 'A',
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "tank", HPCurrent: 1000, HPMax: 1000,
			Defense: 99, BAB: 0, CurrentRoomID: roomID,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, roomID); err != nil {
		t.Fatalf("place mob: %v", err)
	}

	mgr := New(bus, chars, mobs, templates, items)
	var mu sync.Mutex
	var hits []CombatHit
	var misses []CombatMiss
	eventbus.Subscribe[CombatHit](bus, func(_ context.Context, ev CombatHit) {
		mu.Lock()
		hits = append(hits, ev)
		mu.Unlock()
	})
	eventbus.Subscribe[CombatMiss](bus, func(_ context.Context, ev CombatMiss) {
		mu.Lock()
		misses = append(misses, ev)
		mu.Unlock()
	})

	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: ch.ID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, roomID, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Pin RNG AFTER Start: primary d20=2 (miss, no fumble), off-hand
	// d20=1 (fumble). Two d20s consumed total — primary misses with
	// no damage roll, so the RNG sequence stays clean.
	mgr.SetRNG(rand.New(rand.NewSource(stubSeedFor2Rolls(t, 2, 1))))
	if err := mgr.EnqueueAction(roomID, parts[0], Action{
		Kind: ActionAttack, Target: parts[1], WeaponID: primary.ID,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	mgr.Tick(ctx)

	// Switch to local names for the assertions below.
	rig := &twoHandRig{
		chars: chars, items: items,
		charID: ch.ID, primaryID: primary.ID, offHandID: off.ID,
		roomID: roomID, mu: &mu, hits: &hits, misses: &misses,
	}

	rig.mu.Lock()
	defer rig.mu.Unlock()
	// Both swings missed; one primary miss (no fumble) + one
	// off-hand miss (with fumble).
	if len(*rig.hits) != 0 {
		t.Fatalf("hits = %+v, want 0", *rig.hits)
	}
	if len(*rig.misses) != 2 {
		t.Fatalf("misses = %+v, want 2", *rig.misses)
	}
	primaryMiss := (*rig.misses)[0]
	offHandMiss := (*rig.misses)[1]
	if primaryMiss.OffHand || primaryMiss.Fumble {
		t.Errorf("primary miss: OffHand=%v Fumble=%v, want false/false",
			primaryMiss.OffHand, primaryMiss.Fumble)
	}
	if !offHandMiss.OffHand || !offHandMiss.Fumble {
		t.Errorf("off-hand miss: OffHand=%v Fumble=%v, want true/true",
			offHandMiss.OffHand, offHandMiss.Fumble)
	}
	if offHandMiss.WeaponDroppedID != rig.offHandID {
		t.Errorf("WeaponDroppedID = %d, want %d (off-hand)",
			offHandMiss.WeaponDroppedID, rig.offHandID)
	}

	// Verify the equipment + item rows post-swing.
	ch, err = rig.chars.GetByID(context.Background(), rig.charID)
	if err != nil {
		t.Fatalf("reload char: %v", err)
	}
	if ch.Equipment.Get(creature.SlotOffHand) != 0 {
		t.Errorf("SlotOffHand still wielded, want 0")
	}
	if ch.Equipment.Get(creature.SlotPrimaryWield) != rig.primaryID {
		t.Errorf("SlotPrimaryWield = %d, want %d (untouched)",
			ch.Equipment.Get(creature.SlotPrimaryWield), rig.primaryID)
	}
	it, err := rig.items.GetByID(context.Background(), rig.offHandID)
	if err != nil {
		t.Fatalf("reload off-hand: %v", err)
	}
	if it.RoomID != rig.roomID || it.OwnerCharacterID != 0 {
		t.Errorf("off-hand drop bad: RoomID=%d OwnerCharacterID=%d, want room=%d owner=0",
			it.RoomID, it.OwnerCharacterID, rig.roomID)
	}
}

// TestActorActionCost_OffHandReadsOffHandWeight: actorActionCost(slot)
// returns different cadence for primary (heavy) vs off-hand (light)
// on the same character.
func TestActorActionCost_OffHandReadsOffHandWeight(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	items := repo.NewMemoryItemRepo()
	templates := repo.NewMemoryMobTemplateRepo()

	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	gs, err := items.Create(ctx, repo.Item{
		ExternalID: "tw.gs", Name: "greatsword",
		Weight: 16.0, Type: repo.ItemTypeWeapon,
		Stats: &repo.WeaponStats{Damage: "1d12"},
	})
	if err != nil {
		t.Fatalf("seed gs: %v", err)
	}
	dagger, err := items.Create(ctx, repo.Item{
		ExternalID: "tw.dagger", Name: "dagger",
		Weight: 1.0, Type: repo.ItemTypeWeapon,
		Stats: &repo.WeaponStats{Damage: "1d4"},
	})
	if err != nil {
		t.Fatalf("seed dagger: %v", err)
	}
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Dual",
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50,
			Abilities: creature.Abilities{Str: creature.AbilityScore{Current: 10}},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}
	eq := ch.Equipment
	eq.Set(creature.SlotPrimaryWield, gs.ID)
	eq.Set(creature.SlotOffHand, dagger.ID)
	if err := chars.RecordEquipment(ctx, ch.ID, eq); err != nil {
		t.Fatalf("equip: %v", err)
	}

	bus := eventbus.New()
	mgr := New(bus, chars, mobs, templates, items)

	primaryCost := mgr.actorActionCost(ctx,
		ActorRef{Kind: ActorKindCharacter, ID: ch.ID},
		Action{Kind: ActionAttack, Variant: VariantNormal},
		creature.SlotPrimaryWield)
	offHandCost := mgr.actorActionCost(ctx,
		ActorRef{Kind: ActorKindCharacter, ID: ch.ID},
		Action{Kind: ActionAttack, Variant: VariantNormal},
		creature.SlotOffHand)
	if primaryCost <= offHandCost {
		t.Errorf("primary (%v, greatsword) should cost more than off-hand (%v, dagger)",
			primaryCost, offHandCost)
	}
}

// TestActorActionCost_TwoWeaponGraceHalvesOffHand: feat_two_weapon_grace
// is wired through ResolveFeatModifiers and applied only to off-hand
// swings. Primary cadence is unchanged.
func TestActorActionCost_TwoWeaponGraceHalvesOffHand(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	items := repo.NewMemoryItemRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})

	// Same matched weapon in both slots so weight factor is identical
	// — only the OffHandCostMul should differ between calls.
	sword, err := items.Create(ctx, repo.Item{
		ExternalID: "twg.sword", Name: "shortsword",
		Weight: 3.0, Type: repo.ItemTypeWeapon,
		Stats: &repo.WeaponStats{Damage: "1d6"},
	})
	if err != nil {
		t.Fatalf("seed sword: %v", err)
	}

	graceHash := chargen.HashID("feat_two_weapon_grace")
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Graced",
		Feats:     []int32{graceHash},
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50,
			Abilities: creature.Abilities{Str: creature.AbilityScore{Current: 10}},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}
	eq := ch.Equipment
	eq.Set(creature.SlotPrimaryWield, sword.ID)
	eq.Set(creature.SlotOffHand, sword.ID)
	if err := chars.RecordEquipment(ctx, ch.ID, eq); err != nil {
		t.Fatalf("equip: %v", err)
	}

	// Catalog wired so ResolveFeatModifiers sees feat_two_weapon_grace.
	src, err := chargen.SourceFS()
	if err != nil {
		t.Fatalf("chargen.SourceFS: %v", err)
	}
	cat, err := chargen.Load(src)
	if err != nil {
		t.Fatalf("chargen.Load: %v", err)
	}

	bus := eventbus.New()
	mgr := New(bus, chars, mobs, templates, items)
	mgr.SetCatalog(cat)

	primaryCost := mgr.actorActionCost(ctx,
		ActorRef{Kind: ActorKindCharacter, ID: ch.ID},
		Action{Kind: ActionAttack, Variant: VariantNormal},
		creature.SlotPrimaryWield)
	offHandCost := mgr.actorActionCost(ctx,
		ActorRef{Kind: ActorKindCharacter, ID: ch.ID},
		Action{Kind: ActionAttack, Variant: VariantNormal},
		creature.SlotOffHand)
	// Off-hand should be exactly half the primary cost (same weapon
	// weight; only the 0.5 OffHandCostMul differs).
	if want := primaryCost / 2; absDur(offHandCost-want) > time.Millisecond {
		t.Errorf("off-hand cost = %v, want %v (half of primary %v)", offHandCost, want, primaryCost)
	}
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
