package combat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// TestApplySpeedFactor pins the multiplier table and the safety
// fallback when the factor is zero / negative.
func TestApplySpeedFactor(t *testing.T) {
	base := 3 * time.Second
	cases := []struct {
		name   string
		factor float32
		want   time.Duration
	}{
		{"identity", 1.0, base},
		{"aiel", 0.7, 2100 * time.Millisecond},
		{"ogier", 1.2, 3600 * time.Millisecond},
		{"zero falls back", 0.0, base},
		{"negative falls back", -0.5, base},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplySpeedFactor(base, tc.factor)
			diff := got - tc.want
			if diff < 0 {
				diff = -diff
			}
			// 1ms tolerance: float32 multiplication on a ns duration
			// loses several digits but the visible cadence error is
			// well under a tick.
			if diff > time.Millisecond {
				t.Errorf("ApplySpeedFactor(%v, %.2f) = %v, want %v (Δ %v)",
					base, tc.factor, got, tc.want, diff)
			}
		})
	}
}

// TestEffectiveStaminaRegen pins the heavy-armor halving rule and
// the floor that prevents a base of 1 collapsing to 0.
func TestEffectiveStaminaRegen(t *testing.T) {
	cases := []struct {
		name  string
		base  int32
		armor string
		want  int32
	}{
		{"none", 2, "", 2},
		{"light unaffected", 2, "light", 2},
		{"medium unaffected", 2, "medium", 2},
		{"heavy halves", 4, "heavy", 2},
		{"heavy floor", 1, "heavy", 1},
		{"zero stays zero", 0, "heavy", 0},
		{"negative stays negative", -1, "heavy", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectiveStaminaRegen(tc.base, tc.armor); got != tc.want {
				t.Errorf("EffectiveStaminaRegen(%d, %q) = %d, want %d",
					tc.base, tc.armor, got, tc.want)
			}
		})
	}
}

// TestDefaultActionStamina pins the per-(kind,variant) cost table so
// drift in the cadence vs. cost values gets caught at unit time.
func TestDefaultActionStamina(t *testing.T) {
	cases := []struct {
		kind    ActionKind
		variant AttackVariant
		want    int32
	}{
		{ActionAttack, VariantNormal, 5},
		{ActionAttack, VariantPower, 12},
		{ActionAttack, VariantQuick, 3},
		{ActionParry, VariantNormal, 4},
		{ActionFlee, VariantNormal, 8},
		{ActionDodge, VariantNormal, 3},
		{ActionThrow, VariantNormal, 6},
		{ActionSidestep, VariantNormal, 2},
		{ActionNone, VariantNormal, 0},
	}
	for _, tc := range cases {
		got := DefaultActionStamina(tc.kind, tc.variant)
		if got != tc.want {
			t.Errorf("DefaultActionStamina(%v, %v) = %d, want %d",
				tc.kind, tc.variant, got, tc.want)
		}
	}
}

// TestEnqueueAction_RefusesOnLowStamina verifies the gate refuses an
// attack from a character whose StaminaCurrent is below the action's
// cost. StaminaMax must be > 0 to engage the gate (0 means
// "unconfigured pool" — see the action.go staminaGate godoc).
func TestEnqueueAction_RefusesOnLowStamina(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Winded", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 10, HPMax: 10,
			StaminaCurrent: 2,   // below Normal=5
			StaminaMax:     100, // pool configured → gate engaged
		},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	mobs := repo.NewMemoryMobInstanceRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{ExternalID: "dummy"})
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core:       creature.Core{Name: "dummy", HPCurrent: 5, HPMax: 5, CurrentRoomID: 1},
	})

	bus := eventbus.New()
	mgr := New(bus, chars, mobs, templates, repo.NewMemoryItemRepo())
	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: ch.ID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, 1, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	err = mgr.EnqueueAction(1, parts[0], Action{Kind: ActionAttack, Target: parts[1]})
	if !errors.Is(err, ErrInsufficientStamina) {
		t.Fatalf("EnqueueAction err = %v, want ErrInsufficientStamina", err)
	}
}

// TestEnqueueAction_UnconfiguredPoolUngated proves that StaminaMax==0
// (pre-0049 characters / test fixtures) keeps the pre-slice-63
// behavior — the gate is a no-op so the action queues normally.
func TestEnqueueAction_UnconfiguredPoolUngated(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Legacy", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 10, HPMax: 10,
			// StaminaCurrent == StaminaMax == 0 ⇒ unconfigured.
		},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	mobs := repo.NewMemoryMobInstanceRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{ExternalID: "dummy"})
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core:       creature.Core{Name: "dummy", HPCurrent: 5, HPMax: 5, CurrentRoomID: 1},
	})
	bus := eventbus.New()
	mgr := New(bus, chars, mobs, templates, repo.NewMemoryItemRepo())
	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: ch.ID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, 1, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mgr.EnqueueAction(1, parts[0], Action{Kind: ActionAttack, Target: parts[1]}); err != nil {
		t.Fatalf("EnqueueAction: %v, want nil (unconfigured pool)", err)
	}
}

// TestStaminaTicker_RefillsTowardMax exercises the regen path end to
// end: a partly-drained pool ticks up by StaminaRegen and is clamped
// at StaminaMax on the second pulse.
func TestStaminaTicker_RefillsTowardMax(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "u", PasswordHash: "h"})
	ch, _ := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Regen", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 10, HPMax: 10,
			StaminaCurrent: 96, StaminaMax: 100, StaminaRegen: 3,
		},
	})

	candidates := func() []StaminaCandidate {
		return []StaminaCandidate{{CharacterID: ch.ID, RoomID: 1}}
	}
	tk := NewStaminaTicker(candidates, chars, nil, nil)
	tk.Tick(ctx)

	got, _ := chars.GetByID(ctx, ch.ID)
	if got.Core.StaminaCurrent != 99 {
		t.Fatalf("after tick 1: stamina = %d, want 99", got.Core.StaminaCurrent)
	}

	tk.Tick(ctx)
	got, _ = chars.GetByID(ctx, ch.ID)
	if got.Core.StaminaCurrent != 100 {
		t.Fatalf("after tick 2: stamina = %d, want 100 (clamped)", got.Core.StaminaCurrent)
	}

	// Already-full character: no further write.
	tk.Tick(ctx)
	got, _ = chars.GetByID(ctx, ch.ID)
	if got.Core.StaminaCurrent != 100 {
		t.Fatalf("after tick 3: stamina = %d, want 100 (no overflow)", got.Core.StaminaCurrent)
	}
}

// TestStaminaTicker_HaltsAtZeroHP mirrors the affects ticker rule —
// stamina recovery does not run while the character is dead /
// unconscious. The pool stays put across the pulse.
func TestStaminaTicker_HaltsAtZeroHP(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "u", PasswordHash: "h"})
	ch, _ := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Down", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 0, HPMax: 10,
			StaminaCurrent: 50, StaminaMax: 100, StaminaRegen: 5,
		},
	})
	candidates := func() []StaminaCandidate {
		return []StaminaCandidate{{CharacterID: ch.ID, RoomID: 1}}
	}
	tk := NewStaminaTicker(candidates, chars, nil, nil)
	tk.Tick(ctx)

	got, _ := chars.GetByID(ctx, ch.ID)
	if got.Core.StaminaCurrent != 50 {
		t.Fatalf("stamina at 0 HP = %d, want 50 (no regen)", got.Core.StaminaCurrent)
	}
}

// TestStaminaTicker_HeavyArmorHalvesRegen exercises the items lookup
// path: an actor in heavy plate tops up at half their base regen
// (rounded down, floored at 1).
func TestStaminaTicker_HeavyArmorHalvesRegen(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	items := repo.NewMemoryItemRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "u", PasswordHash: "h"})
	plate, _ := items.Create(ctx, repo.Item{
		ExternalID: "test.plate", Name: "plate",
		Type: repo.ItemTypeArmor, Weight: 50,
		Stats: &repo.ArmorStats{WeightClass: "heavy"},
	})
	eq := creature.Equipment{}
	eq.Set(creature.SlotArmor, plate.ID)
	ch, _ := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Tank", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 10, HPMax: 10,
			StaminaCurrent: 50, StaminaMax: 100, StaminaRegen: 4,
		},
		Equipment: eq,
	})

	candidates := func() []StaminaCandidate {
		return []StaminaCandidate{{CharacterID: ch.ID, RoomID: 1}}
	}
	tk := NewStaminaTicker(candidates, chars, items, nil)
	tk.Tick(ctx)

	got, _ := chars.GetByID(ctx, ch.ID)
	if got.Core.StaminaCurrent != 52 {
		t.Fatalf("plate-wearer regen: stamina = %d, want 52 (4/2)",
			got.Core.StaminaCurrent)
	}
}
