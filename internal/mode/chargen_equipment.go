package mode

// Starting-equipment bundle (#15 slice 3). Substep slots in between
// channeling (when present) and review:
//
//	... → channeling → equipment → review
//	... → skills     → equipment → review   (non-channeler classes)
//
// What gets picked here:
//
//   1. Bundle — exactly 1 of bg.EquipmentOptions. Each option is a
//      builder-authored alternative starting kit (e.g. "Tent, cadin'sor,
//      buckler, waterskin, 2 healer's balms").
//
// On finalize the picked bundle is cloned via ItemRepo.Create with
// fresh runtime external_ids ("<id>#cgen-<charID>-<i>") into the
// character's inventory, and the appropriate outfit / armor / shield /
// primary weapon is auto-equipped via Equipment.Set + RecordEquipment.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// initEquipmentStepIfNeeded is a placeholder for symmetry with the
// other init helpers. Equipment has no per-entry default to stamp —
// the menu reads bg.EquipmentOptions each render.
func (m *CharacterCreate) initEquipmentStepIfNeeded() { _ = m }

// equipmentRowComplete reports whether the equipment hub row counts
// as filled. True once the player has picked one of the bundles AND
// the index resolves against the current background's options
// (defensive — flipping background drops a stale pick).
func (m *CharacterCreate) equipmentRowComplete() bool {
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return false
	}
	idx := m.draft.SelectedEquipmentOptionIdx
	return idx >= 1 && idx <= len(bg.EquipmentOptions)
}

func (m *CharacterCreate) writeEquipmentMenu(s *telnet.Session) error {
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return writeError(s, "Pick a background first.")
	}
	if err := writeStepHeader(s, chargenStepEquipment); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("{{Starting equipment — pick one bundle:}}::yellow|bold\r\n")
	for i, opt := range bg.EquipmentOptions {
		mark := "  "
		row := "yellow"
		if i+1 == m.draft.SelectedEquipmentOptionIdx {
			mark = "*"
			row = "yellow|bold"
		}
		fmt.Fprintf(&b,
			"  {{%s}}::green|bold {{%2d)}}::gray {{%s}}::%s\r\n",
			mark, i+1, defangChargenField(opt.Label), row)
	}
	if m.draft.SelectedEquipmentOptionIdx > 0 &&
		m.draft.SelectedEquipmentOptionIdx <= len(bg.EquipmentOptions) {
		opt := bg.EquipmentOptions[m.draft.SelectedEquipmentOptionIdx-1]
		fmt.Fprintf(&b, "  {{Selected:}}::green|bold {{%s}}::green\r\n",
			defangChargenField(opt.Label))
	}
	b.WriteString(
		"\r\n  Pick a number  ·  {{[I]}}::yellow nfo <#>  ·  {{[D]}}::green|bold one  ·  {{[B]}}::yellow ack to hub\r\n")
	return s.WriteString(b.String())
}

// writeEquipmentInfo renders the picked bundle's contents row by row
// — name, type, weight, value — so the player can compare bundles
// before committing.
func (m *CharacterCreate) writeEquipmentInfo(s *telnet.Session, bg *chargen.Background, idx int) error {
	opt := bg.EquipmentOptions[idx]
	if err := s.WriteString(fmt.Sprintf(
		"{{Bundle %d: %s}}::cyan|bold\r\n",
		idx+1, defangChargenField(opt.Label),
	)); err != nil {
		return err
	}
	for _, ref := range opt.Items {
		tpl, ok := m.catalog.Item(ref)
		if !ok {
			if err := writeFieldRow(s, ref, "(missing template)"); err != nil {
				return err
			}
			continue
		}
		val := tpl.ParsedValue()
		valStr := "—"
		if val > 0 {
			valStr = val.Short()
		}
		if err := writeFieldRow(s, tpl.Name,
			fmt.Sprintf("%s · %.1f lb · %s",
				tpl.Type, tpl.Weight, valStr)); err != nil {
			return err
		}
	}
	return writeRule(s)
}

// applyEquipment routes one line of input on the equipment substep.
//
//	<n>            pick bundle #n
//	pick <n>       same
//	i <n> | info <n>   show bundle contents
//	d | done       advance to review
//	b | back       return to hub  (handled in handleMulti)
//	show           re-render the menu
func (m *CharacterCreate) applyEquipment(s *telnet.Session, input string) error {
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return writeError(s, "Internal catalog error; background missing.")
	}
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return m.writeEquipmentMenu(s)
	}

	if rest, ok := stripInfoVerb(input); ok {
		if rest == "" {
			return writeError(s, "Type 'i <#>' for bundle details.")
		}
		idx, err := parsePositiveIndex(rest, len(bg.EquipmentOptions))
		if err != nil {
			return writeError(s, fmt.Sprintf(
				"Pick a row number 1..%d.", len(bg.EquipmentOptions)))
		}
		return m.writeEquipmentInfo(s, bg, idx)
	}

	verb := strings.ToLower(fields[0])
	rest := strings.Join(fields[1:], " ")
	switch verb {
	case "show":
		return m.writeEquipmentMenu(s)
	case "d", "done", "next":
		if !m.equipmentRowComplete() {
			return writeError(s, "Pick a bundle first.")
		}
		m.step = chargenStepHub
		return m.writeHub(s)
	case "pick":
		return m.applyEquipmentPick(s, bg, rest)
	}

	// Bare "<n>" is also accepted as a pick.
	return m.applyEquipmentPick(s, bg, verb)
}

func (m *CharacterCreate) applyEquipmentPick(s *telnet.Session, bg *chargen.Background, tok string) error {
	idx, err := parsePositiveIndex(tok, len(bg.EquipmentOptions))
	if err != nil {
		return writeError(s, fmt.Sprintf(
			"Pick a row number 1..%d.", len(bg.EquipmentOptions)))
	}
	m.draft.SelectedEquipmentOptionIdx = idx + 1
	return m.writeEquipmentMenu(s)
}

// applyStartingEquipment clones each item in the picked bundle into the
// new character's inventory via ItemRepo.Create, then auto-equips the
// first armor / shield / outfit / weapon via Equipment.Set and persists
// the slot map. Best-effort failure semantics — the caller logs but
// does not unwind on error.
//
// Auto-equip rules (V1, intentionally minimal):
//   - First Armor    → SlotArmor
//   - First Shield   → SlotShield
//   - First Clothing → SlotOutfit
//   - First Weapon   → SlotPrimaryWield
//
// Two-handed / off-hand / light / quiver auto-equip and the Cloak /
// Backpack / WornMisc / BeltPouches slot disambiguation stay deferred
// (already tracked in slice-1 / slice-2 follow-ups).
func (m *CharacterCreate) applyStartingEquipment(ctx context.Context, char *repo.Character) error {
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return fmt.Errorf("background missing")
	}
	idx := m.draft.SelectedEquipmentOptionIdx
	if idx < 1 || idx > len(bg.EquipmentOptions) {
		return fmt.Errorf("no equipment option picked")
	}
	opt := bg.EquipmentOptions[idx-1]

	invIDs := make([]int64, 0, len(opt.Items))
	for i, tplID := range opt.Items {
		tpl, ok := m.catalog.Item(tplID)
		if !ok {
			// Catalog validation should have caught this; defensive
			// log + skip rather than aborting the whole bundle.
			slog.Warn("chargen: missing item template", "id", tplID,
				"bg", bg.ID, "char", char.ID)
			continue
		}
		fresh := repo.Item{
			ExternalID:       fmt.Sprintf("%s#cgen-%d-%d", tplID, char.ID, i),
			Name:             tpl.Name,
			NameLower:        strings.ToLower(tpl.Name),
			ShortDesc:        tpl.Short,
			OwnerCharacterID: char.ID,
			Type:             tpl.Type,
			Weight:           tpl.Weight,
			Value:            tpl.ParsedValue(),
			Quality:          tpl.Quality,
			Flags:            tpl.ParsedFlags(),
			Stats:            repo.CloneItemStats(tpl.ParsedStats()),
		}
		created, err := m.items.Create(ctx, fresh)
		if err != nil {
			return fmt.Errorf("create item %q: %w", tplID, err)
		}
		invIDs = append(invIDs, created.ID)
		autoEquipFromType(&char.Equipment, tpl.Type, created.ID)
	}

	if err := m.repo.RecordInventory(ctx, char.ID, invIDs); err != nil {
		return fmt.Errorf("record inventory: %w", err)
	}
	if err := m.repo.RecordEquipment(ctx, char.ID, char.Equipment); err != nil {
		return fmt.Errorf("record equipment: %w", err)
	}
	char.Inventory = invIDs
	return nil
}

// autoEquipFromType moves a freshly-spawned item into the matching
// single-occupancy slot when that slot is empty. First-wins per
// category — second armor / shield / outfit / weapon stays in
// inventory. Two-handed weapons, off-hand auto-equip, light items,
// and the cloak/backpack slot disambiguation are slice-1/2
// follow-ups (see chargen_features_followups.md).
func autoEquipFromType(eq *creature.Equipment, t repo.ItemType, itemID int64) {
	var slot creature.Slot
	switch t {
	case repo.ItemTypeArmor:
		slot = creature.SlotArmor
	case repo.ItemTypeShield:
		slot = creature.SlotShield
	case repo.ItemTypeClothing:
		slot = creature.SlotOutfit
	case repo.ItemTypeWeapon:
		slot = creature.SlotPrimaryWield
	default:
		return
	}
	if eq.Get(slot) == 0 {
		eq.Set(slot, itemID)
	}
}
