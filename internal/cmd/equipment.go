package cmd

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// slotForWear maps an item type to the single-occupancy slot a `wear`
// targets. Weapons are intentionally absent — they go through `wield`
// so the primary/off-hand split is explicit. Returns ok=false for
// types V1 doesn't know how to wear (Cloak / Backpack split needs
// item-flag work; see plan follow-ups).
func slotForWear(t repo.ItemType) (creature.Slot, bool) {
	switch t {
	case repo.ItemTypeArmor:
		return creature.SlotArmor, true
	case repo.ItemTypeShield:
		return creature.SlotShield, true
	case repo.ItemTypeClothing:
		return creature.SlotOutfit, true
	}
	return 0, false
}

// orderedSlots is the display order for the `equipment` listing. The
// underlying enum order is API-stable (zero = Armor) but UX-wise we
// want hands listed first so a player skims wielded gear top-down.
var orderedSlots = []creature.Slot{
	creature.SlotPrimaryWield,
	creature.SlotOffHand,
	creature.SlotShield,
	creature.SlotArmor,
	creature.SlotOutfit,
	creature.SlotCloak,
	creature.SlotBackpack,
	creature.SlotHeldInHand,
	creature.SlotMount,
}

// NewWear builds the `wear <item>` command. Equips armor / shield /
// clothing into the slot derived from ItemType. Weapons get nudged to
// `wield` so the primary/off-hand split stays explicit.
func NewWear(items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:      "wear",
		Help:      "Wear <item> — put on armor, a shield, or clothing",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeInventoryItems(items),
		Run: func(c *telnet.Context) error {
			s := c.Session
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("wear: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			it, ok := MatchItem(target, held)
			if !ok {
				return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
			}
			if it.Type == repo.ItemTypeWeapon {
				return s.WriteString("{{Try {{wield}}::cyan for weapons.}}::yellow\r\n")
			}
			slot, ok := slotForWear(it.Type)
			if !ok {
				return s.WriteString("{{You can't wear that.}}::yellow\r\n")
			}

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("wear: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented.}}::red\r\n")
			}
			if _, equipped := char.Equipment.FindByItem(it.ID); equipped {
				return s.WriteString("{{You are already wearing that.}}::yellow\r\n")
			}
			if char.Equipment.Get(slot) != 0 {
				return s.WriteString(fmt.Sprintf("{{You must remove what you have on your %s first.}}::yellow\r\n", slot.SlotName()))
			}
			char.Equipment.Set(slot, it.ID)
			if err := characters.RecordEquipment(c.Ctx, char.ID, char.Equipment); err != nil {
				slog.Warn("wear: record equipment failed", "char", char.ID, "error", err)
				return s.WriteString("{{The garment slips through your fingers.}}::red\r\n")
			}

			actor := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actor+" puts on "+it.Name+".}}::cyan\r\n")
			return s.WriteString("{{You put on " + it.Name + ".}}::cyan\r\n")
		},
	}
}

// NewWield builds the `wield <item> [off]` command. Equips a weapon
// to PrimaryWield (default) or OffHand when the second arg is "off"
// or "offhand".
func NewWield(items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name: "wield",
		Help: "Wield <weapon> [off] — ready a weapon",
		Long: "Usage: wield <weapon>            - in your primary hand\n" +
			"       wield <weapon> off        - in your off hand\n",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeInventoryItems(items),
		Run: func(c *telnet.Context) error {
			s := c.Session
			itemArg, slot := splitWieldArgs(c.Args)
			target := strings.ToLower(strings.TrimSpace(itemArg))
			if target == "" {
				return s.WriteString("{{Usage: wield <weapon> [off]}}::yellow\r\n")
			}
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("wield: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			it, ok := MatchItem(target, held)
			if !ok {
				return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
			}
			if it.Type != repo.ItemTypeWeapon {
				return s.WriteString("{{That isn't a weapon.}}::yellow\r\n")
			}

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("wield: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented.}}::red\r\n")
			}
			if existing, found := char.Equipment.FindByItem(it.ID); found {
				if existing == slot {
					return s.WriteString("{{You are already wielding that.}}::yellow\r\n")
				}
				return s.WriteString(fmt.Sprintf("{{You are already wielding that in your %s.}}::yellow\r\n", existing.SlotName()))
			}
			if char.Equipment.Get(slot) != 0 {
				return s.WriteString(fmt.Sprintf("{{Your %s is full — remove what's there first.}}::yellow\r\n", slot.SlotName()))
			}
			char.Equipment.Set(slot, it.ID)
			if err := characters.RecordEquipment(c.Ctx, char.ID, char.Equipment); err != nil {
				slog.Warn("wield: record equipment failed", "char", char.ID, "error", err)
				return s.WriteString("{{The weapon slips from your grip.}}::red\r\n")
			}

			actor := safeActor(s)
			handPhrase := "primary hand"
			if slot == creature.SlotOffHand {
				handPhrase = "off hand"
			}
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actor+" wields "+it.Name+" in "+pronounTheir(s)+" "+handPhrase+".}}::cyan\r\n")
			return s.WriteString("{{You wield " + it.Name + " in your " + handPhrase + ".}}::cyan\r\n")
		},
	}
}

// NewRemove builds the `remove <item>` command. Unequips an item back
// to inventory; the item never left owner_character_id, so this is a
// pure equipment_json overlay edit.
func NewRemove(items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:      "remove",
		Help:      "Remove <item> — take off something you are wearing or wielding",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeInventoryItems(items),
		Run: func(c *telnet.Context) error {
			s := c.Session
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("remove: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("remove: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented.}}::red\r\n")
			}

			// Only items currently equipped are valid targets — narrow
			// the candidate set so `remove sword` doesn't misfire on a
			// spare sword in the pack.
			equipped := equippedItems(held, char.Equipment)
			it, ok := MatchItem(target, equipped)
			if !ok {
				return s.WriteString("{{You aren't wearing or wielding that.}}::yellow\r\n")
			}
			slot, ok := char.Equipment.ClearItem(it.ID)
			if !ok {
				// Should be unreachable given equippedItems filter.
				return s.WriteString("{{You aren't wearing or wielding that.}}::yellow\r\n")
			}
			if err := characters.RecordEquipment(c.Ctx, char.ID, char.Equipment); err != nil {
				slog.Warn("remove: record equipment failed", "char", char.ID, "error", err)
				return s.WriteString("{{It clings to you stubbornly.}}::red\r\n")
			}

			actor := safeActor(s)
			verb := "removes"
			selfVerb := "remove"
			if slot == creature.SlotPrimaryWield || slot == creature.SlotOffHand {
				verb = "stops wielding"
				selfVerb = "stop wielding"
			}
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actor+" "+verb+" "+it.Name+".}}::cyan\r\n")
			return s.WriteString("{{You " + selfVerb + " " + it.Name + ".}}::cyan\r\n")
		},
	}
}

// NewEquipment builds the `equipment` command. Lists each occupied
// slot in display order with the equipped item's name.
func NewEquipment(items repo.ItemRepo, characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "equipment",
		Aliases: []string{"eq"},
		Help:    "Show what you have worn and wielded",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("equipment: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented.}}::red\r\n")
			}
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("equipment: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			byID := make(map[int64]repo.Item, len(held))
			for _, it := range held {
				byID[it.ID] = it
			}

			var b strings.Builder
			b.WriteString("{{You are using:}}::green|bold\r\n")
			any := false
			for _, slot := range orderedSlots {
				id := char.Equipment.Get(slot)
				if id == 0 {
					continue
				}
				any = true
				name := "(unknown)"
				if it, ok := byID[id]; ok {
					name = it.Name
				}
				b.WriteString(fmt.Sprintf("  {{<%-13s>}}::yellow {{%s}}::green\r\n", slot.SlotName(), name))
			}
			// Slice slots — currently unused by V1 verbs, but render any
			// admin-stamped entries so `equipment` is faithful to the
			// stored state.
			renderSliceSlot := func(label string, ids []int64) {
				if len(ids) == 0 {
					return
				}
				any = true
				// Stable order regardless of map traversal noise.
				cp := append([]int64(nil), ids...)
				sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
				for _, id := range cp {
					name := "(unknown)"
					if it, ok := byID[id]; ok {
						name = it.Name
					}
					b.WriteString(fmt.Sprintf("  {{<%-13s>}}::yellow {{%s}}::green\r\n", label, name))
				}
			}
			renderSliceSlot("belt pouch", char.Equipment.BeltPouches)
			renderSliceSlot("worn", char.Equipment.WornMisc)

			if !any {
				b.WriteString("  {{(nothing equipped)}}::gray\r\n")
			}
			return s.WriteString(b.String())
		},
	}
}

// autoUnequipIfHeld clears any equipment slot pointing at itemID and
// persists the result. Called from drop / give / put so leaving the
// inventory can never strand a dangling equipment_json reference. The
// owner is read from c.Session.CharacterName — the caller is always
// the actor whose item just moved. Errors are logged but not
// returned: the inventory transfer already committed, so the worst
// case here is a stale slot pointer that `equipment` will render as
// `(unknown)` until the next wear/wield.
func autoUnequipIfHeld(c *telnet.Context, characters repo.CharacterRepo, itemID int64) {
	char, err := characters.FindByName(c.Ctx, c.Session.CharacterName)
	if err != nil {
		slog.Debug("auto-unequip: char lookup failed", "char", c.Session.CharacterID, "error", err)
		return
	}
	if _, cleared := char.Equipment.ClearItem(itemID); !cleared {
		return
	}
	if err := characters.RecordEquipment(c.Ctx, char.ID, char.Equipment); err != nil {
		slog.Warn("auto-unequip: record equipment failed", "char", char.ID, "error", err)
	}
}

// equippedItems filters held to those currently in a single-occupancy
// slot. Used by `remove` to narrow keyword resolution.
func equippedItems(held []repo.Item, eq creature.Equipment) []repo.Item {
	out := held[:0:0]
	for _, it := range held {
		if _, ok := eq.FindByItem(it.ID); ok {
			out = append(out, it)
		}
	}
	return out
}

// splitWieldArgs returns the item-name portion and the destination
// slot. Trailing "off" / "offhand" / "second" picks SlotOffHand;
// anything else (or no qualifier) picks SlotPrimaryWield. Multi-word
// item names work as long as the qualifier is the very last token.
func splitWieldArgs(args []string) (item string, slot creature.Slot) {
	slot = creature.SlotPrimaryWield
	if len(args) == 0 {
		return "", slot
	}
	last := strings.ToLower(args[len(args)-1])
	if last == "off" || last == "offhand" || last == "second" {
		if len(args) == 1 {
			// Only the qualifier was given — leave item empty so caller
			// emits the usage line.
			return "", creature.SlotOffHand
		}
		return strings.Join(args[:len(args)-1], " "), creature.SlotOffHand
	}
	return strings.Join(args, " "), slot
}

// pronounTheir is the third-person possessive used in room
// broadcasts. V1 uses gender-neutral "their" so we don't have to
// thread Core.Gender into every broadcast string; switch later if
// gendered pronouns become a feature.
func pronounTheir(_ *telnet.Session) string {
	return "their"
}
