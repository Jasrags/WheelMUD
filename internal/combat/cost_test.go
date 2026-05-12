package combat

import (
	"context"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// TestWeaponSpeedFactor pins the four weight bands plus the unarmed
// special-case (weight ≤ 0). Boundary values use the upper edge of
// each band to verify the switch is closed-upper / open-lower as the
// godoc claims.
func TestWeaponSpeedFactor(t *testing.T) {
	cases := []struct {
		weight float64
		want   float64
	}{
		{0, 0.90},    // unarmed
		{-1, 0.90},   // negative ≡ unarmed
		{0.5, 0.80},  // light: dagger
		{2.0, 0.80},  // light upper edge
		{3.0, 1.00},  // medium: shortsword
		{10.0, 1.00}, // medium upper edge
		{12.0, 1.30}, // heavy: battleaxe
		{15.0, 1.30}, // heavy upper edge
		{16.0, 1.50}, // two-handed: greatsword
		{99.0, 1.50}, // two-handed open-upper
	}
	for _, tc := range cases {
		if got := weaponSpeedFactor(tc.weight); got != tc.want {
			t.Errorf("weaponSpeedFactor(%.2f) = %.2f, want %.2f",
				tc.weight, got, tc.want)
		}
	}
}

// TestArmorSpeedFactor pins the four well-formed classes plus the
// "" / "none" / unknown degraded paths. Unknown logs but returns 1.0.
func TestArmorSpeedFactor(t *testing.T) {
	cases := []struct {
		class string
		want  float64
	}{
		{"", 1.00},
		{"none", 1.00},
		{"light", 1.05},
		{"medium", 1.15},
		{"heavy", 1.30},
		{"gilded", 1.00}, // unknown — degrade
	}
	for _, tc := range cases {
		if got := armorSpeedFactor(tc.class); got != tc.want {
			t.Errorf("armorSpeedFactor(%q) = %.2f, want %.2f",
				tc.class, got, tc.want)
		}
	}
}

// TestActionCost_GreatswordPlate is the headline integration: a
// Power swing (4.5s base) wielding a 16-lb greatsword and wearing
// heavy plate is 4.5 × 1.5 × 1.3 = 8.775s.
func TestActionCost_GreatswordPlate(t *testing.T) {
	base := DefaultActionCost(ActionAttack, VariantPower)
	got := ActionCost(base, 16.0, "heavy")
	want := time.Duration(float64(base) * 1.5 * 1.3)
	if got != want {
		t.Errorf("ActionCost(power, greatsword, plate) = %v, want %v", got, want)
	}
}

// TestActionCost_UnarmedNaked verifies the zero-gear baseline: a
// Normal swing (3s base) with no weapon and no armor is 3 × 0.9 ×
// 1.0 = 2.7s. This is the cadence existing seedManager-based tests
// land on now.
func TestActionCost_UnarmedNaked(t *testing.T) {
	base := DefaultActionCost(ActionAttack, VariantNormal)
	got := ActionCost(base, 0, "")
	want := 2700 * time.Millisecond
	if got != want {
		t.Errorf("ActionCost(normal, unarmed, naked) = %v, want %v", got, want)
	}
}

// TestResolveGearFactors_NilRepo proves nil items repo yields the
// unarmed/naked baseline (0.9 × 1.0). Used by tests that don't wire
// an ItemRepo and by score rendering when the cmd-layer hands in nil.
func TestResolveGearFactors_NilRepo(t *testing.T) {
	g := ResolveGearFactors(context.Background(), nil, creature.Equipment{}, creature.SlotPrimaryWield)
	if g.WeaponFactor != 0.90 || g.ArmorFactor != 1.00 {
		t.Errorf("nil repo gear = (%.2f, %.2f), want (0.90, 1.00)",
			g.WeaponFactor, g.ArmorFactor)
	}
}

// TestResolveGearFactors_FullKit walks the full path: build a memory
// items repo, seed a greatsword and a plate, equip them in the right
// slots, and assert the resolved factors match the static tables.
func TestResolveGearFactors_FullKit(t *testing.T) {
	ctx := context.Background()
	items := repo.NewMemoryItemRepo()
	gs, err := items.Create(ctx, repo.Item{
		ExternalID: "test.greatsword",
		Name:       "greatsword",
		Weight:     16.0,
		Type:       repo.ItemTypeWeapon,
		Stats:      &repo.WeaponStats{Damage: "1d12"},
	})
	if err != nil {
		t.Fatalf("seed weapon: %v", err)
	}
	pl, err := items.Create(ctx, repo.Item{
		ExternalID: "test.plate",
		Name:       "plate",
		Weight:     50.0,
		Type:       repo.ItemTypeArmor,
		Stats:      &repo.ArmorStats{WeightClass: "heavy", Bonus: 8},
	})
	if err != nil {
		t.Fatalf("seed armor: %v", err)
	}

	eq := creature.Equipment{}
	eq.Set(creature.SlotPrimaryWield, gs.ID)
	eq.Set(creature.SlotArmor, pl.ID)

	g := ResolveGearFactors(ctx, items, eq, creature.SlotPrimaryWield)
	if g.WeaponFactor != 1.50 {
		t.Errorf("weapon factor = %.2f, want 1.50", g.WeaponFactor)
	}
	if g.ArmorFactor != 1.30 {
		t.Errorf("armor factor = %.2f, want 1.30", g.ArmorFactor)
	}
	if got, want := g.Multiplier(), 1.50*1.30; absDiff(got, want) > 1e-9 {
		t.Errorf("multiplier = %.4f, want %.4f", got, want)
	}
}

func absDiff(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

// TestActorActionCost_BlademasterAttenuates seeds a character with
// the Blademaster feat (weapon_weight_penalty_mul: 0.5), wields a
// 16-lb greatsword, and asserts the cadence drops from the un-feated
// baseline. Pure-fn math: 3.0s × (1 + (1.5-1)*0.5) × 1.0 (no armor)
// = 3.0s × 1.25 = 3.75s, versus 3.0s × 1.5 = 4.5s without the feat.
// Phase L slice 65.
func TestActorActionCost_BlademasterAttenuates(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	items := repo.NewMemoryItemRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	accs := repo.NewMemoryAccountRepo()

	gs, err := items.Create(ctx, repo.Item{
		ExternalID: "test.greatsword", Name: "greatsword",
		Weight: 16.0, Type: repo.ItemTypeWeapon,
		Stats: &repo.WeaponStats{Damage: "1d12"},
	})
	if err != nil {
		t.Fatalf("seed greatsword: %v", err)
	}

	acc, _ := accs.Create(ctx, repo.Account{Username: "u", PasswordHash: "h"})
	eq := creature.Equipment{}
	eq.Set(creature.SlotPrimaryWield, gs.ID)

	bm, _ := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Blademaster", CurrentRoomID: 1,
		Equipment: eq,
		Feats:     []int32{chargen.HashID("blademaster")},
	})
	plain, _ := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Plain", CurrentRoomID: 1,
		Equipment: eq,
	})

	bus := eventbus.New()
	mgr := New(bus, chars, mobs, templates, items)
	mgr.SetCatalog(loadFakeCatalog(t))

	bmCost := mgr.actorActionCost(ctx,
		ActorRef{Kind: ActorKindCharacter, ID: bm.ID},
		Action{Kind: ActionAttack, Variant: VariantNormal},
		creature.SlotPrimaryWield)
	plainCost := mgr.actorActionCost(ctx,
		ActorRef{Kind: ActorKindCharacter, ID: plain.ID},
		Action{Kind: ActionAttack, Variant: VariantNormal},
		creature.SlotPrimaryWield)

	// Plain: 3000ms × 1.5 (greatsword) × 1.0 (no armor) × 1.0 (human race) = 4500ms
	// BM:    3000ms × (1+(1.5-1)*0.5) × 1.0 × 1.0 = 3750ms
	wantBM := 3750 * time.Millisecond
	wantPlain := 4500 * time.Millisecond
	if bmCost != wantBM {
		t.Errorf("blademaster cost = %v, want %v", bmCost, wantBM)
	}
	if plainCost != wantPlain {
		t.Errorf("plain cost = %v, want %v", plainCost, wantPlain)
	}
	if bmCost >= plainCost {
		t.Errorf("blademaster (%v) should be faster than plain (%v)", bmCost, plainCost)
	}
}

// TestActorActionCost_HeavyVsLight asserts a plate-greatsword actor's
// per-swing cadence is more than 1.5× a leather-dagger actor's, via
// the Manager's actorActionCost method. Exercises the full mob-side
// equipment-lookup path against memory repos.
func TestActorActionCost_HeavyVsLight(t *testing.T) {
	ctx := context.Background()
	chars := repo.NewMemoryCharacterRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	items := repo.NewMemoryItemRepo()
	templates := repo.NewMemoryMobTemplateRepo()

	gs, err := items.Create(ctx, repo.Item{
		ExternalID: "test.greatsword", Name: "greatsword",
		Weight: 16.0, Type: repo.ItemTypeWeapon,
		Stats: &repo.WeaponStats{Damage: "1d12"},
	})
	if err != nil {
		t.Fatalf("seed greatsword: %v", err)
	}
	plate, err := items.Create(ctx, repo.Item{
		ExternalID: "test.plate", Name: "plate", Weight: 50.0,
		Type:  repo.ItemTypeArmor,
		Stats: &repo.ArmorStats{WeightClass: "heavy"},
	})
	if err != nil {
		t.Fatalf("seed plate: %v", err)
	}
	dagger, err := items.Create(ctx, repo.Item{
		ExternalID: "test.dagger", Name: "dagger",
		Weight: 1.0, Type: repo.ItemTypeWeapon,
		Stats: &repo.WeaponStats{Damage: "1d4"},
	})
	if err != nil {
		t.Fatalf("seed dagger: %v", err)
	}

	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "test.dummy", ChallengeCode: 'A',
	})
	heavyMob, err := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core:       creature.Core{Name: "heavy"},
		Equipment: creature.Equipment{
			PrimaryWield: gs.ID,
			Armor:        plate.ID,
		},
	})
	if err != nil {
		t.Fatalf("seed heavy mob: %v", err)
	}
	lightMob, err := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core:       creature.Core{Name: "light"},
		Equipment: creature.Equipment{
			PrimaryWield: dagger.ID,
		},
	})
	if err != nil {
		t.Fatalf("seed light mob: %v", err)
	}

	bus := eventbus.New()
	mgr := New(bus, chars, mobs, templates, items)

	heavyCost := mgr.actorActionCost(ctx,
		ActorRef{Kind: ActorKindMob, ID: heavyMob.ID},
		Action{Kind: ActionAttack, Variant: VariantNormal},
		creature.SlotPrimaryWield)
	lightCost := mgr.actorActionCost(ctx,
		ActorRef{Kind: ActorKindMob, ID: lightMob.ID},
		Action{Kind: ActionAttack, Variant: VariantNormal},
		creature.SlotPrimaryWield)

	// Heavy: 3s × 1.5 (two-handed) × 1.3 (heavy armor) = 5.85s
	// Light: 3s × 0.8 (light weapon) × 1.0 (no armor)   = 2.40s
	if heavyCost != 5850*time.Millisecond {
		t.Errorf("heavy cost = %v, want 5.85s", heavyCost)
	}
	if lightCost != 2400*time.Millisecond {
		t.Errorf("light cost = %v, want 2.40s", lightCost)
	}
	if ratio := float64(heavyCost) / float64(lightCost); ratio <= 1.5 {
		t.Errorf("heavy/light ratio = %.2f, want > 1.5", ratio)
	}
}
