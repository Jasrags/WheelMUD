package cmd

// Phase E #25 slice 2 — `quaff <potion>` consumable producer.
//
// Resolves a potion (or other consumable) from the caller's inventory,
// looks up its effect via the effects catalog (ConsumableStats.EffectID
// is chargen.HashID(string id)), applies the effect through
// affects.Apply, and destroys the item. V1 ignores Charges and always
// deletes the item; multi-charge consumables wait for an
// ItemRepo.UpdateStats method (slice 3+).

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/effects"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewQuaff builds the `quaff <potion>` player verb.
func NewQuaff(items repo.ItemRepo, characters repo.CharacterRepo, eff *effects.Catalog, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "quaff",
		Help:    "quaff <potion> — drink a potion or other consumable from your inventory",
		Long:    "Usage: quaff <potion>\n\nDrinks a single charge from a consumable item in your inventory. The potion is consumed.",
		Auth:    telnet.AuthPlayer,
		MinArgs: 1,
		Run: func(c *telnet.Context) error {
			s := c.Session
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("quaff: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			it, ok := MatchItem(target, held)
			if !ok {
				return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
			}
			if it.Type != repo.ItemTypeConsumable {
				return s.WriteString("{{" + it.Name + " isn't something you can drink.}}::yellow\r\n")
			}
			stats, ok := it.Stats.(repo.ConsumableStats)
			if !ok {
				slog.Warn("quaff: consumable item missing ConsumableStats",
					"item", it.ID, "external", it.ExternalID)
				return s.WriteString("{{The " + it.Name + " fizzles harmlessly.}}::yellow\r\n")
			}
			effectID, ok := eff.IDForHash(stats.EffectID)
			if !ok {
				slog.Warn("quaff: effect hash not in catalog",
					"item", it.ID, "external", it.ExternalID, "hash", stats.EffectID)
				_ = items.Delete(c.Ctx, it.ID)
				cleanInventoryRef(c, characters, s, it.ID)
				return s.WriteString("{{The " + it.Name + " fizzles harmlessly.}}::yellow\r\n")
			}
			e, ok := eff.Get(effectID)
			if !ok {
				slog.Warn("quaff: effect id not in catalog",
					"item", it.ID, "effect", effectID)
				_ = items.Delete(c.Ctx, it.ID)
				cleanInventoryRef(c, characters, s, it.ID)
				return s.WriteString("{{The " + it.Name + " fizzles harmlessly.}}::yellow\r\n")
			}

			char, err := characters.GetByID(c.Ctx, s.CharacterID)
			if errors.Is(err, repo.ErrCharacterNotFound) {
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}
			if err != nil {
				slog.Error("quaff: load self", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't focus on the potion.}}::red\r\n")
			}
			next := affects.Apply(char.Core.Affects, e.ToAffect(consumableAffectSource))
			if err := characters.RecordAffects(c.Ctx, s.CharacterID, next); err != nil {
				slog.Error("quaff: persist affects", "char", s.CharacterID, "error", err)
				return s.WriteString("{{The potion slips through your fingers.}}::red\r\n")
			}

			// Item is consumed regardless of authored Charges (V1 — see
			// header comment). Failure here is a server-side warning, not
			// a player refusal: the affect already applied.
			if err := items.Delete(c.Ctx, it.ID); err != nil {
				slog.Warn("quaff: delete item failed", "item", it.ID, "char", s.CharacterID, "error", err)
			}
			cleanInventoryRef(c, characters, s, it.ID)

			actor := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actor+" quaffs "+it.Name+".}}::cyan\r\n")
			line := "{{You quaff " + it.Name + ".}}::cyan\r\n"
			if e.MessageOnApply != "" {
				line += "{{" + e.MessageOnApply + "}}::cyan\r\n"
			}
			return s.WriteString(line)
		},
	}
}

// cleanInventoryRef removes the item id from the caller's
// inventory_json ordering. Mirrors the post-drop / post-give cleanup
// in inventory.go. Failures are logged but never surfaced to the
// player — the SQL location columns are the source of truth.
func cleanInventoryRef(c *telnet.Context, characters repo.CharacterRepo, s *telnet.Session, itemID int64) {
	char, err := characters.FindByName(c.Ctx, s.CharacterName)
	if err != nil {
		slog.Warn("quaff: char lookup failed", "char", s.CharacterID, "error", err)
		return
	}
	newInv := removeID(char.Inventory, itemID)
	if err := characters.RecordInventory(c.Ctx, s.CharacterID, newInv); err != nil {
		slog.Warn("quaff: record inventory failed", "char", s.CharacterID, "error", err)
	}
}
