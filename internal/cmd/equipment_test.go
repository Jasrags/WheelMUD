package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// equipFixture extends invFixture with a sword + an armor + a shield
// + a piece of clothing pre-placed in Alice's inventory. Items are
// handed out by ExternalID for keyword stability.
type equipFixture struct {
	*invFixture
	sword    repo.Item
	armor    repo.Item
	shield   repo.Item
	clothing repo.Item
	rock     repo.Item
}

func newEquipFixture(t *testing.T) *equipFixture {
	t.Helper()
	f := newInvFixture(t)
	insert := func(it repo.Item) repo.Item {
		stored, err := f.items.Create(context.Background(), it)
		if err != nil {
			t.Fatalf("seed item %q: %v", it.ExternalID, err)
		}
		return stored
	}
	mk := func(extID, name string, typ repo.ItemType, stats repo.ItemStats) repo.Item {
		return insert(repo.Item{
			ExternalID: extID, Name: name,
			OwnerCharacterID: f.alice.CharacterID,
			Type:             typ, Quality: repo.QualityNormal, Weight: 1,
			Stats: stats,
		})
	}
	return &equipFixture{
		invFixture: f,
		sword:      mk("sword", "a sword", repo.ItemTypeWeapon, &repo.WeaponStats{}),
		armor:      mk("hauberk", "a hauberk", repo.ItemTypeArmor, &repo.ArmorStats{}),
		shield:     mk("shield", "a shield", repo.ItemTypeShield, &repo.ShieldStats{}),
		clothing:   mk("cloak", "a cloak", repo.ItemTypeClothing, nil),
		rock:       mk("rock", "a rock", repo.ItemTypeTrash, nil),
	}
}

func TestWear_EquipsArmor(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewWear(f.items, f.characters, f.sessions), f.alice, "hauberk")

	if !strings.Contains(f.aOut.String(), "You put on a hauberk") {
		t.Fatalf("alice missing self echo; got %q", f.aOut.String())
	}
	if !strings.Contains(f.bOut.String(), "Alice puts on a hauberk") {
		t.Fatalf("bob missing room broadcast; got %q", f.bOut.String())
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.Armor != f.armor.ID {
		t.Fatalf("Equipment.Armor = %d, want %d", c.Equipment.Armor, f.armor.ID)
	}
}

func TestWear_RoutesByType(t *testing.T) {
	f := newEquipFixture(t)
	wear := NewWear(f.items, f.characters, f.sessions)

	runCmd(t, wear, f.alice, "shield")
	runCmd(t, wear, f.alice, "cloak")

	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.Shield != f.shield.ID {
		t.Fatalf("shield not equipped: %+v", c.Equipment)
	}
	if c.Equipment.Outfit != f.clothing.ID {
		t.Fatalf("clothing not in outfit slot: %+v", c.Equipment)
	}
}

func TestWear_RefusesWeapon(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewWear(f.items, f.characters, f.sessions), f.alice, "sword")

	if !strings.Contains(f.aOut.String(), "wield") {
		t.Fatalf("expected wield nudge; got %q", f.aOut.String())
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.PrimaryWield != 0 || c.Equipment.Armor != 0 {
		t.Fatalf("nothing should be equipped: %+v", c.Equipment)
	}
}

func TestWear_RefusesUnwearable(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewWear(f.items, f.characters, f.sessions), f.alice, "rock")
	if !strings.Contains(f.aOut.String(), "can't wear that") {
		t.Fatalf("expected refusal; got %q", f.aOut.String())
	}
}

func TestWear_RefusesAlreadyOccupied(t *testing.T) {
	f := newEquipFixture(t)
	// Seed a second armor so a real conflict can happen.
	second, _ := f.items.Create(context.Background(), repo.Item{
		ExternalID: "scale", Name: "a scale",
		OwnerCharacterID: f.alice.CharacterID,
		Type:             repo.ItemTypeArmor, Quality: repo.QualityNormal,
		Stats: &repo.ArmorStats{},
	})
	wear := NewWear(f.items, f.characters, f.sessions)
	runCmd(t, wear, f.alice, "hauberk")
	f.aOut.Reset()
	runCmd(t, wear, f.alice, "scale")

	if !strings.Contains(f.aOut.String(), "remove") {
		t.Fatalf("expected slot-occupied refusal; got %q", f.aOut.String())
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.Armor != f.armor.ID {
		t.Fatalf("first armor should still be equipped; got %d (second=%d)", c.Equipment.Armor, second.ID)
	}
}

func TestWield_PrimaryAndOffHand(t *testing.T) {
	f := newEquipFixture(t)
	// Two weapons so primary + off-hand can each get one.
	dagger, _ := f.items.Create(context.Background(), repo.Item{
		ExternalID: "dagger", Name: "a dagger",
		OwnerCharacterID: f.alice.CharacterID,
		Type:             repo.ItemTypeWeapon, Quality: repo.QualityNormal,
		Stats: &repo.WeaponStats{},
	})

	wield := NewWield(f.items, f.characters, f.sessions)
	runCmd(t, wield, f.alice, "sword")
	runCmd(t, wield, f.alice, "dagger off")

	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.PrimaryWield != f.sword.ID {
		t.Fatalf("PrimaryWield = %d, want %d", c.Equipment.PrimaryWield, f.sword.ID)
	}
	if c.Equipment.OffHand != dagger.ID {
		t.Fatalf("OffHand = %d, want %d", c.Equipment.OffHand, dagger.ID)
	}
}

func TestWield_RefusesNonWeapon(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewWield(f.items, f.characters, f.sessions), f.alice, "hauberk")
	if !strings.Contains(f.aOut.String(), "isn't a weapon") {
		t.Fatalf("expected non-weapon refusal; got %q", f.aOut.String())
	}
}

func TestRemove_UnequipsAndKeepsOwnership(t *testing.T) {
	f := newEquipFixture(t)
	wield := NewWield(f.items, f.characters, f.sessions)
	remove := NewRemove(f.items, f.characters, f.sessions)
	runCmd(t, wield, f.alice, "sword")
	f.aOut.Reset()
	f.bOut.Reset()

	runCmd(t, remove, f.alice, "sword")

	if !strings.Contains(f.aOut.String(), "stop wielding") {
		t.Fatalf("alice missing self echo; got %q", f.aOut.String())
	}
	if !strings.Contains(f.bOut.String(), "Alice stops wielding") {
		t.Fatalf("bob missing broadcast; got %q", f.bOut.String())
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.PrimaryWield != 0 {
		t.Fatalf("PrimaryWield not cleared: %d", c.Equipment.PrimaryWield)
	}
	// Item still owned (equipment is overlay, not relocation).
	held, _ := f.items.ListInInventory(context.Background(), f.alice.CharacterID)
	found := false
	for _, it := range held {
		if it.ID == f.sword.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("sword no longer in inventory after remove")
	}
}

func TestRemove_RefusesUnequipped(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewRemove(f.items, f.characters, f.sessions), f.alice, "sword")
	if !strings.Contains(f.aOut.String(), "aren't wearing or wielding that") {
		t.Fatalf("expected refusal; got %q", f.aOut.String())
	}
}

func TestEquipment_Listing(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewWield(f.items, f.characters, f.sessions), f.alice, "sword")
	runCmd(t, NewWear(f.items, f.characters, f.sessions), f.alice, "hauberk")
	f.aOut.Reset()

	runCmd(t, NewEquipment(f.items, f.characters), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "primary hand") || !strings.Contains(out, "a sword") {
		t.Fatalf("missing primary hand line; got %q", out)
	}
	if !strings.Contains(out, "armor") || !strings.Contains(out, "a hauberk") {
		t.Fatalf("missing armor line; got %q", out)
	}
}

func TestEquipment_EmptyShowsNothingEquipped(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewEquipment(f.items, f.characters), f.alice, "")
	if !strings.Contains(f.aOut.String(), "nothing equipped") {
		t.Fatalf("expected empty marker; got %q", f.aOut.String())
	}
}

func TestDrop_AutoUnequips(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewWield(f.items, f.characters, f.sessions), f.alice, "sword")

	runCmd(t, NewDrop(f.items, f.characters, f.sessions), f.alice, "sword")

	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.PrimaryWield != 0 {
		t.Fatalf("PrimaryWield not auto-cleared on drop: %d", c.Equipment.PrimaryWield)
	}
	floor, _ := f.items.ListInRoom(context.Background(), 1)
	if len(floor) != 1 || floor[0].ID != f.sword.ID {
		t.Fatalf("sword not on floor: %+v", floor)
	}
}

func TestGive_AutoUnequips(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewWield(f.items, f.characters, f.sessions), f.alice, "sword")

	runCmd(t, NewGive(f.items, f.characters, f.sessions), f.alice, "sword Bob")

	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.PrimaryWield != 0 {
		t.Fatalf("PrimaryWield not auto-cleared on give: %d", c.Equipment.PrimaryWield)
	}
	bobInv, _ := f.items.ListInInventory(context.Background(), f.bob.CharacterID)
	if len(bobInv) != 1 || bobInv[0].ID != f.sword.ID {
		t.Fatalf("sword did not arrive in Bob's inventory: %+v", bobInv)
	}
}

func TestPut_AutoUnequips(t *testing.T) {
	f := newEquipFixture(t)
	chest, _ := f.items.Create(context.Background(), repo.Item{
		ExternalID: "chest", Name: "a chest",
		OwnerCharacterID: f.alice.CharacterID,
		Type:             repo.ItemTypeContainer, Quality: repo.QualityNormal,
		Stats: &repo.ContainerStats{CapacityLbs: 50},
	})
	runCmd(t, NewWield(f.items, f.characters, f.sessions), f.alice, "sword")

	runCmd(t, NewPut(f.items, f.characters, f.sessions), f.alice, "sword in chest")

	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if c.Equipment.PrimaryWield != 0 {
		t.Fatalf("PrimaryWield not auto-cleared on put: %d", c.Equipment.PrimaryWield)
	}
	contents, _ := f.items.ListInContainer(context.Background(), chest.ID)
	if len(contents) != 1 || contents[0].ID != f.sword.ID {
		t.Fatalf("sword did not land in chest: %+v", contents)
	}
}

func TestInventory_AnnotatesEquipped(t *testing.T) {
	f := newEquipFixture(t)
	runCmd(t, NewWield(f.items, f.characters, f.sessions), f.alice, "sword")
	runCmd(t, NewWear(f.items, f.characters, f.sessions), f.alice, "hauberk")
	f.aOut.Reset()

	runCmd(t, NewInventory(f.items, f.characters), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "(wielded)") {
		t.Fatalf("missing (wielded) annotation; got %q", out)
	}
	if !strings.Contains(out, "(worn)") {
		t.Fatalf("missing (worn) annotation; got %q", out)
	}
}

func TestSplitWieldArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      []string
		want    string
		offhand bool
	}{
		{[]string{"sword"}, "sword", false},
		{[]string{"long", "sword"}, "long sword", false},
		{[]string{"sword", "off"}, "sword", true},
		{[]string{"long", "sword", "offhand"}, "long sword", true},
		{[]string{"off"}, "", true},
		{[]string{}, "", false},
	}
	for _, tc := range cases {
		got, slot := splitWieldArgs(tc.in)
		if got != tc.want {
			t.Errorf("splitWieldArgs(%v) item = %q, want %q", tc.in, got, tc.want)
		}
		gotOff := slot == creature.SlotOffHand
		if gotOff != tc.offhand {
			t.Errorf("splitWieldArgs(%v) offhand = %v, want %v", tc.in, gotOff, tc.offhand)
		}
	}
}
