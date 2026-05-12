package combat

import (
	"context"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// Phase F #32 slice 5a — direct tests on ApplyDamageExternal /
// ApplyHealing. These exercise the entry points used by Lua's
// deal_damage / heal bindings; the full combat pipeline is not
// involved (no roll, no parry, no threat).

func newExternalDamageFixture(t *testing.T) (
	*Manager,
	*repo.MemoryCharacterRepo,
	repo.MobInstanceRepo,
	repo.MobTemplateRepo,
	int64, // charID
	int64, // mobID
	int64, // roomID
	*eventbus.Bus,
) {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "u", PasswordHash: "h"})
	const roomID int64 = 100
	const boundRoomID int64 = 200
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice",
		CurrentRoomID: roomID, BoundRoomID: boundRoomID,
		Core: creature.Core{HPCurrent: 30, HPMax: 50},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "test.dummy", ChallengeCode: 'A',
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "training dummy", HPCurrent: 20, HPMax: 40,
			CurrentRoomID: roomID,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, roomID); err != nil {
		t.Fatalf("place mob: %v", err)
	}

	items := repo.NewMemoryItemRepo()
	mgr := New(bus, chars, mobs, templates, items)
	return mgr, chars, mobs, templates, ch.ID, mob.ID, roomID, bus
}

func TestApplyDamageExternal_Character_NonLethal_PersistsHP(t *testing.T) {
	ctx := context.Background()
	mgr, chars, _, _, charID, _, roomID, bus := newExternalDamageFixture(t)

	var mu sync.Mutex
	var hits []ScriptDamageDealt
	eventbus.Subscribe(bus, func(_ context.Context, ev ScriptDamageDealt) {
		mu.Lock()
		defer mu.Unlock()
		hits = append(hits, ev)
	})

	if err := mgr.ApplyDamageExternal(ctx, ActorRef{}, ActorRef{Kind: ActorKindCharacter, ID: charID}, 7, "fire_trap"); err != nil {
		t.Fatalf("ApplyDamageExternal: %v", err)
	}
	ch, _ := chars.GetByID(ctx, charID)
	if ch.Core.HPCurrent != 23 {
		t.Errorf("hp = %d, want 23", ch.Core.HPCurrent)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hits) != 1 {
		t.Fatalf("ScriptDamageDealt events = %d, want 1", len(hits))
	}
	if hits[0].Amount != 7 || hits[0].Source != "fire_trap" ||
		hits[0].RoomID != roomID || hits[0].Lethal {
		t.Errorf("event = %+v", hits[0])
	}
}

func TestApplyDamageExternal_Character_Lethal_RoutesCharacterDeath(t *testing.T) {
	ctx := context.Background()
	mgr, _, _, _, charID, _, _, bus := newExternalDamageFixture(t)

	var mu sync.Mutex
	var hits []ScriptDamageDealt
	var died []CharacterDied
	var respawned []CharacterRespawned
	eventbus.Subscribe(bus, func(_ context.Context, ev ScriptDamageDealt) {
		mu.Lock()
		hits = append(hits, ev)
		mu.Unlock()
	})
	eventbus.Subscribe(bus, func(_ context.Context, ev CharacterDied) {
		mu.Lock()
		died = append(died, ev)
		mu.Unlock()
	})
	eventbus.Subscribe(bus, func(_ context.Context, ev CharacterRespawned) {
		mu.Lock()
		respawned = append(respawned, ev)
		mu.Unlock()
	})

	// 30 HP target; 100 damage easily lethal.
	if err := mgr.ApplyDamageExternal(ctx, ActorRef{}, ActorRef{Kind: ActorKindCharacter, ID: charID}, 100, "void"); err != nil {
		t.Fatalf("ApplyDamageExternal: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hits) != 1 || !hits[0].Lethal {
		t.Fatalf("ScriptDamageDealt lethal expected, got %+v", hits)
	}
	if len(died) != 1 {
		t.Fatalf("CharacterDied events = %d, want 1", len(died))
	}
	if len(respawned) != 1 {
		t.Fatalf("CharacterRespawned events = %d, want 1", len(respawned))
	}
}

func TestApplyDamageExternal_Character_AlreadyDead_NoOp(t *testing.T) {
	ctx := context.Background()
	mgr, chars, _, _, charID, _, _, bus := newExternalDamageFixture(t)

	// Force HP to zero via the repo so the next call observes a
	// dead target. RecordCore writes the four mutable fields atomically.
	if err := chars.RecordCore(ctx, charID, 0, 0, 0, 0); err != nil {
		t.Fatalf("seed dead: %v", err)
	}

	var hits int
	eventbus.Subscribe(bus, func(_ context.Context, _ ScriptDamageDealt) { hits++ })

	if err := mgr.ApplyDamageExternal(ctx, ActorRef{}, ActorRef{Kind: ActorKindCharacter, ID: charID}, 50, "void"); err != nil {
		t.Fatalf("ApplyDamageExternal: %v", err)
	}
	if hits != 0 {
		t.Errorf("hits = %d, want 0 (already dead is a silent no-op)", hits)
	}
}

func TestApplyDamageExternal_Mob_Lethal_RoutesMobDeath(t *testing.T) {
	ctx := context.Background()
	mgr, _, _, _, _, mobID, _, bus := newExternalDamageFixture(t)

	var mu sync.Mutex
	var deaths []CombatDeath
	eventbus.Subscribe(bus, func(_ context.Context, ev CombatDeath) {
		mu.Lock()
		deaths = append(deaths, ev)
		mu.Unlock()
	})

	if err := mgr.ApplyDamageExternal(ctx, ActorRef{}, ActorRef{Kind: ActorKindMob, ID: mobID}, 999, "lightning"); err != nil {
		t.Fatalf("ApplyDamageExternal: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deaths) != 1 {
		t.Fatalf("CombatDeath events = %d, want 1", len(deaths))
	}
	if deaths[0].Victim.Kind != ActorKindMob || deaths[0].Victim.ID != mobID {
		t.Errorf("victim = %+v", deaths[0].Victim)
	}
}

func TestApplyDamageExternal_NoThreatTableMutation(t *testing.T) {
	ctx := context.Background()
	mgr, _, _, _, charID, _, roomID, _ := newExternalDamageFixture(t)

	// Open a fight so the threat-table machinery exists; we want to
	// verify the external-damage path does NOT add a threat row.
	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: charID},
	}
	if _, err := mgr.Start(ctx, roomID, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := mgr.ApplyDamageExternal(ctx, ActorRef{}, ActorRef{Kind: ActorKindCharacter, ID: charID}, 5, "trap"); err != nil {
		t.Fatalf("ApplyDamageExternal: %v", err)
	}

	mgr.mu.RLock()
	f := mgr.fights[roomID]
	threatRows := 0
	for _, row := range f.Threat {
		threatRows += len(row)
	}
	mgr.mu.RUnlock()
	if threatRows != 0 {
		t.Errorf("threat rows after external damage = %d, want 0", threatRows)
	}
}

func TestApplyDamageExternal_ArgValidation(t *testing.T) {
	ctx := context.Background()
	mgr, _, _, _, charID, _, _, _ := newExternalDamageFixture(t)

	cases := []struct {
		name   string
		target ActorRef
		amount int32
	}{
		{"zero amount", ActorRef{Kind: ActorKindCharacter, ID: charID}, 0},
		{"negative amount", ActorRef{Kind: ActorKindCharacter, ID: charID}, -3},
		{"unknown kind", ActorRef{Kind: ActorKindUnknown, ID: charID}, 5},
		{"zero id", ActorRef{Kind: ActorKindCharacter, ID: 0}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := mgr.ApplyDamageExternal(ctx, ActorRef{}, tc.target, tc.amount, "x"); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestApplyHealing_Character_ClampsAtHPMax(t *testing.T) {
	ctx := context.Background()
	mgr, chars, _, _, charID, _, roomID, bus := newExternalDamageFixture(t)

	var mu sync.Mutex
	var heals []ScriptHealingApplied
	eventbus.Subscribe(bus, func(_ context.Context, ev ScriptHealingApplied) {
		mu.Lock()
		heals = append(heals, ev)
		mu.Unlock()
	})

	// 30/50 HP — heal 999 should clamp to delta 20.
	if err := mgr.ApplyHealing(ctx, ActorRef{Kind: ActorKindCharacter, ID: charID}, 999); err != nil {
		t.Fatalf("ApplyHealing: %v", err)
	}
	ch, _ := chars.GetByID(ctx, charID)
	if ch.Core.HPCurrent != 50 {
		t.Errorf("hp = %d, want 50", ch.Core.HPCurrent)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(heals) != 1 || heals[0].Amount != 20 || heals[0].RoomID != roomID {
		t.Fatalf("event = %+v", heals)
	}
}

func TestApplyHealing_Character_AlreadyAtFull_AmountZero(t *testing.T) {
	ctx := context.Background()
	mgr, chars, _, _, charID, _, _, bus := newExternalDamageFixture(t)

	// Top off to max.
	if err := chars.RecordCore(ctx, charID, 50, 0, 0, 0); err != nil {
		t.Fatalf("seed full hp: %v", err)
	}

	var heals []ScriptHealingApplied
	eventbus.Subscribe(bus, func(_ context.Context, ev ScriptHealingApplied) {
		heals = append(heals, ev)
	})
	if err := mgr.ApplyHealing(ctx, ActorRef{Kind: ActorKindCharacter, ID: charID}, 5); err != nil {
		t.Fatalf("ApplyHealing: %v", err)
	}
	if len(heals) != 1 || heals[0].Amount != 0 {
		t.Errorf("event = %+v; want one ScriptHealingApplied with Amount=0", heals)
	}
}

func TestApplyHealing_DeadTarget_NoOp(t *testing.T) {
	ctx := context.Background()
	mgr, chars, _, _, charID, _, _, bus := newExternalDamageFixture(t)

	if err := chars.RecordCore(ctx, charID, 0, 0, 0, 0); err != nil {
		t.Fatalf("seed dead: %v", err)
	}
	var heals int
	eventbus.Subscribe(bus, func(_ context.Context, _ ScriptHealingApplied) { heals++ })
	if err := mgr.ApplyHealing(ctx, ActorRef{Kind: ActorKindCharacter, ID: charID}, 50); err != nil {
		t.Fatalf("ApplyHealing: %v", err)
	}
	if heals != 0 {
		t.Errorf("heal events = %d, want 0 (dead target is silent no-op)", heals)
	}
	ch, _ := chars.GetByID(ctx, charID)
	if ch.Core.HPCurrent != 0 {
		t.Errorf("hp = %d, want 0 (heal does not raise dead)", ch.Core.HPCurrent)
	}
}

func TestApplyHealing_Mob_ClampsAtHPMax(t *testing.T) {
	ctx := context.Background()
	mgr, _, mobs, _, _, mobID, _, bus := newExternalDamageFixture(t)

	var heals []ScriptHealingApplied
	eventbus.Subscribe(bus, func(_ context.Context, ev ScriptHealingApplied) {
		heals = append(heals, ev)
	})
	// 20/40 HP — heal 30 should clamp to delta 20.
	if err := mgr.ApplyHealing(ctx, ActorRef{Kind: ActorKindMob, ID: mobID}, 30); err != nil {
		t.Fatalf("ApplyHealing: %v", err)
	}
	mob, _ := mobs.GetByID(ctx, mobID)
	if mob.Core.HPCurrent != 40 {
		t.Errorf("hp = %d, want 40", mob.Core.HPCurrent)
	}
	if len(heals) != 1 || heals[0].Amount != 20 {
		t.Errorf("event = %+v", heals)
	}
}
