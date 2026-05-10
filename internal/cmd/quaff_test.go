package cmd

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/effects"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

func loadTestEffects(t *testing.T) *effects.Catalog {
	t.Helper()
	body := `effects:
  - id: healing_draught
    name: healing draught
    duration_ticks: 4
    tick_effect: regen
    tick_damage: 2
    message_on_apply: "Warmth fills you."
  - id: bull_strength
    name: bull's strength
    duration_ticks: 50
    modifiers:
      - field: Str.Current
        delta: 2
`
	cat, err := effects.Load(fstest.MapFS{"e.yaml": &fstest.MapFile{Data: []byte(body)}})
	if err != nil {
		t.Fatalf("load test effects: %v", err)
	}
	return cat
}

func TestQuaff_AppliesEffectAndConsumesItem(t *testing.T) {
	f := newInvFixture(t)
	cat := loadTestEffects(t)
	potion := f.items.Insert(repo.Item{
		ExternalID: "potion_healing", Name: "a healing potion",
		Type:       repo.ItemTypeConsumable,
		OwnerCharacterID: f.alice.CharacterID,
		Stats: repo.ConsumableStats{
			Charges:  1,
			EffectID: chargen.HashID("healing_draught"),
		},
	})
	// Mirror the inventory_json ordering for the cleanup path.
	if err := f.characters.RecordInventory(context.Background(), f.alice.CharacterID, []int64{potion.ID}); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	runCmd(t, NewQuaff(f.items, f.characters, cat, f.sessions), f.alice, "potion")

	out := f.aOut.String()
	if !strings.Contains(out, "You quaff a healing potion") {
		t.Fatalf("self echo missing: %q", out)
	}
	if !strings.Contains(out, "Warmth fills you") {
		t.Fatalf("on-apply message missing: %q", out)
	}
	if !strings.Contains(f.bOut.String(), "Alice quaffs a healing potion") {
		t.Fatalf("room broadcast missing: %q", f.bOut.String())
	}

	if _, err := f.items.GetByID(context.Background(), potion.ID); err == nil {
		t.Fatalf("potion should be deleted")
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if len(c.Core.Affects) != 1 || c.Core.Affects[0].Name != "healing draught" {
		t.Fatalf("affect not applied: %+v", c.Core.Affects)
	}
	if c.Core.Affects[0].Source != consumableAffectSource {
		t.Fatalf("Source sentinel: want %d, got %d",
			consumableAffectSource, c.Core.Affects[0].Source)
	}
	if len(c.Inventory) != 0 {
		t.Fatalf("inventory_json not cleaned: %+v", c.Inventory)
	}
}

func TestQuaff_NotInInventory(t *testing.T) {
	f := newInvFixture(t)
	cat := loadTestEffects(t)
	runCmd(t, NewQuaff(f.items, f.characters, cat, f.sessions), f.alice, "potion")
	if !strings.Contains(f.aOut.String(), "You aren't carrying that") {
		t.Fatalf("missing refusal: %q", f.aOut.String())
	}
}

func TestQuaff_NotConsumable(t *testing.T) {
	f := newInvFixture(t)
	cat := loadTestEffects(t)
	rock := f.items.Insert(repo.Item{
		ExternalID:  "rock", Name: "a smooth rock",
		OwnerCharacterID: f.alice.CharacterID,
	})
	_ = rock
	runCmd(t, NewQuaff(f.items, f.characters, cat, f.sessions), f.alice, "rock")
	if !strings.Contains(f.aOut.String(), "isn't something you can drink") {
		t.Fatalf("missing refusal: %q", f.aOut.String())
	}
}

func TestQuaff_UnknownEffectFizzles(t *testing.T) {
	f := newInvFixture(t)
	cat := loadTestEffects(t)
	potion := f.items.Insert(repo.Item{
		ExternalID: "mystery_potion", Name: "a mystery potion",
		Type:       repo.ItemTypeConsumable,
		OwnerCharacterID: f.alice.CharacterID,
		Stats: repo.ConsumableStats{
			Charges:  1,
			EffectID: chargen.HashID("nonexistent_effect"),
		},
	})
	runCmd(t, NewQuaff(f.items, f.characters, cat, f.sessions), f.alice, "potion")
	if !strings.Contains(f.aOut.String(), "fizzles harmlessly") {
		t.Fatalf("missing fizzle: %q", f.aOut.String())
	}
	// Item must still be consumed (fizzle is a side effect, not a refusal).
	if _, err := f.items.GetByID(context.Background(), potion.ID); err == nil {
		t.Fatalf("fizzled potion should be deleted")
	}
}
