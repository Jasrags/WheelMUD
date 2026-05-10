package cmd

// Phase E #25 slice 2 — `quaff <potion>` consumable producer.
// Phase E #25 slice 3 — multi-charge consumables.
//
// Resolves a potion (or other consumable) from the caller's inventory,
// looks up its effect via the effects catalog (ConsumableStats.EffectID
// is chargen.HashID(string id)), applies the effect through
// affects.Apply, and either deletes or decrements the item.
//
// Charge accounting (slice 3 — see ConsumableStats.Charges):
//   - Charges == 0 → unlimited. Item stays in inventory after every
//     quaff. Mirrors ToolStats.Charges == 0 convention.
//   - Charges == 1 → final dose. Item is deleted; inventory_json
//     pointer cleaned up.
//   - Charges  > 1 → multi-dose. Charges decremented via
//     ItemRepo.UpdateStats; item remains in inventory.

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
			stats, ok := consumableStatsOf(it.Stats)
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

			// Charge accounting. Failures here are logged but never
			// surfaced to the player — the affect already applied.
			switch {
			case stats.Charges == 0:
				// Unlimited; nothing to do.
			case stats.Charges > 1:
				newStats := &repo.ConsumableStats{
					Charges:  stats.Charges - 1,
					EffectID: stats.EffectID,
				}
				if err := items.UpdateStats(c.Ctx, it.ID, newStats); err != nil {
					slog.Warn("quaff: decrement charges failed",
						"item", it.ID, "char", s.CharacterID, "error", err)
				}
			default: // 1 (or any negative — defensive)
				if err := items.Delete(c.Ctx, it.ID); err != nil {
					slog.Warn("quaff: delete item failed",
						"item", it.ID, "char", s.CharacterID, "error", err)
				}
				cleanInventoryRef(c, characters, s, it.ID)
			}

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

// consumableStatsOf extracts ConsumableStats from an item's Stats
// regardless of pointer-vs-value packaging. The world loader builds
// items with `*ConsumableStats` (StatsForType returns pointers); a
// few in-process call sites (admin spawn, tests) build them as
// values. Both must work.
func consumableStatsOf(s repo.ItemStats) (repo.ConsumableStats, bool) {
	switch v := s.(type) {
	case *repo.ConsumableStats:
		if v == nil {
			return repo.ConsumableStats{}, false
		}
		return *v, true
	case repo.ConsumableStats:
		return v, true
	default:
		return repo.ConsumableStats{}, false
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
