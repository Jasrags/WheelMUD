package mode

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// charCreateWithItemsFixture is a charCreateFixture extended with a
// memory ItemRepo wired through SetItems so finalize-time spawn can
// be asserted against the items table.
type charCreateWithItemsFixture struct {
	*charCreateFixture
	items *repo.MemoryItemRepo
}

// pushCharacterCreateMultiWithItems builds a multi-step CharacterCreate
// fixture wired to a memory ItemRepo so the equipment-spawn path runs
// at finalize time. Mirrors pushCharacterCreateMulti.
func pushCharacterCreateMultiWithItems(t *testing.T) *charCreateWithItemsFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	fsys, err := chargen.SourceFS()
	if err != nil {
		t.Fatalf("chargen source: %v", err)
	}
	cat, err := chargen.Load(fsys)
	if err != nil {
		t.Fatalf("chargen load: %v", err)
	}

	cr := repo.NewMemoryCharacterRepo()
	ir := repo.NewMemoryItemRepo()
	game := &stubMode{name: "game"}
	mode := NewCharacterCreate(cr, game)
	mode.SetCatalog(cat)
	mode.SetItems(ir)

	s := telnet.NewSession(server)
	s.AccountID = 1

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}
	return &charCreateWithItemsFixture{
		charCreateFixture: &charCreateFixture{
			t: t, session: s, peer: client, chars: cr, game: game, captured: captured,
		},
		items: ir,
	}
}

// TestEquipmentSubstep_PickAndDone verifies the substep persists the
// 1-based bundle index, refuses `done` before a pick, and lands back
// on the hub on `done`.
func TestEquipmentSubstep_PickAndDone(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("7") // hub → equipment (row 7 for non-channelers)
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepEquipment {
		t.Fatalf("step = %d, want chargenStepEquipment", mc.step)
	}

	// done with no pick is rejected.
	f.captured.Reset()
	f.feed("done")
	if mc.step != chargenStepEquipment {
		t.Fatalf("done without pick advanced: step = %d", mc.step)
	}
	if !strings.Contains(f.captured.String(), "Pick a bundle") {
		t.Fatalf("expected pick-first hint:\n%s", f.captured.String())
	}

	// info shows bundle contents but does not commit.
	f.captured.Reset()
	f.feed("info 2")
	if !strings.Contains(f.captured.String(), "Bundle 2") {
		t.Fatalf("info did not render bundle:\n%s", f.captured.String())
	}
	if mc.draft.SelectedEquipmentOptionIdx != 0 {
		t.Fatalf("info committed selection: idx = %d",
			mc.draft.SelectedEquipmentOptionIdx)
	}

	// pick by bare number.
	f.feed("2")
	if mc.draft.SelectedEquipmentOptionIdx != 2 {
		t.Fatalf("pick failed: idx = %d", mc.draft.SelectedEquipmentOptionIdx)
	}

	f.feed("done")
	if mc.step != chargenStepHub {
		t.Fatalf("step = %d, want chargenStepHub after done", mc.step)
	}
}

// TestEquipmentSubstep_BackgroundChangeClearsPick asserts that
// re-picking a different background drops the prior bundle index —
// equipment_options are bg-specific and a stale index would point
// into the wrong table.
func TestEquipmentSubstep_BackgroundChangeClearsPick(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("7")
	f.feed("1")
	f.feed("done")
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.draft.SelectedEquipmentOptionIdx != 1 {
		t.Fatalf("setup: idx = %d, want 1", mc.draft.SelectedEquipmentOptionIdx)
	}

	// Flip background — equipment pick should clear.
	f.feed("1")
	f.feed("aiel")
	if mc.draft.SelectedEquipmentOptionIdx != 0 {
		t.Fatalf("background flip did not clear pick: idx = %d",
			mc.draft.SelectedEquipmentOptionIdx)
	}
}

// TestStartingEquipment_SpawnsAndAutoEquips drives a midlander/armsman
// through to commit and asserts the picked bundle landed in the
// character's inventory with the right ExternalID pattern, and that
// the leather armor + small steel shield + boar spear ended up in the
// matching Equipment slots.
func TestStartingEquipment_SpawnsAndAutoEquips(t *testing.T) {
	f := pushCharacterCreateMultiWithItems(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("3")
	f.feed("done")
	f.feed("4")
	f.feed("done")
	f.feed("5")
	f.feed("pick 1")
	f.feed("done")
	f.feed("6")
	f.feed("done")
	f.feed("7") // equipment
	// Bundle #2 for midlander: "Boar spear; longsword; leather armor;
	// small steel shield; tent" — exercises every auto-equip rule.
	f.feed("2")
	f.feed("done")
	f.feed("yes") // commit

	got, err := f.chars.FindByName(context.Background(), "Hero")
	if err != nil {
		t.Fatalf("find Hero: %v", err)
	}

	inv, err := f.items.ListInInventory(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("list inv: %v", err)
	}
	if len(inv) == 0 {
		t.Fatalf("no items spawned for character")
	}
	// Every spawned item's external_id must match the cgen pattern and
	// belong to the new character.
	for _, it := range inv {
		if it.OwnerCharacterID != got.ID {
			t.Errorf("item %q owner = %d, want %d", it.Name, it.OwnerCharacterID, got.ID)
		}
		if !strings.Contains(it.ExternalID, "#cgen-") {
			t.Errorf("item %q ext_id %q missing #cgen- marker", it.Name, it.ExternalID)
		}
	}

	// Auto-equip: leather armor → SlotArmor, shield → SlotShield,
	// boar spear → SlotPrimaryWield (first weapon).
	byType := map[repo.ItemType]repo.Item{}
	for _, it := range inv {
		if _, seen := byType[it.Type]; !seen {
			byType[it.Type] = it
		}
	}
	if armor, ok := byType[repo.ItemTypeArmor]; ok {
		if got.Equipment.Get(creature.SlotArmor) != armor.ID {
			t.Errorf("armor slot = %d, want %d", got.Equipment.Get(creature.SlotArmor), armor.ID)
		}
	} else {
		t.Errorf("expected an armor in midlander bundle 2")
	}
	if shield, ok := byType[repo.ItemTypeShield]; ok {
		if got.Equipment.Get(creature.SlotShield) != shield.ID {
			t.Errorf("shield slot = %d, want %d", got.Equipment.Get(creature.SlotShield), shield.ID)
		}
	}
	if weapon, ok := byType[repo.ItemTypeWeapon]; ok {
		if got.Equipment.Get(creature.SlotPrimaryWield) != weapon.ID {
			t.Errorf("primary-wield slot = %d, want %d",
				got.Equipment.Get(creature.SlotPrimaryWield), weapon.ID)
		}
	}
}

// TestStartingEquipment_NilItemsRepoSkipsSilently asserts that the
// finalize path with no item repo wired (legacy fixture) still
// commits the character — equipment spawn is best-effort, not gating.
func TestStartingEquipment_NilItemsRepoSkipsSilently(t *testing.T) {
	f := pushCharacterCreateMulti(t) // no items repo wired
	walkHubToReview(t, f)
	f.feed("yes")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	if _, err := f.chars.FindByName(context.Background(), "Hero"); err != nil {
		t.Fatalf("character should commit even without items repo: %v", err)
	}
}
