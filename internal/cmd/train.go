package cmd

// train is the trainer-NPC verb that commits one pending level-up
// into the player's ClassLevels (Phase E #23). Slice 2 ships the
// resolver + read-only diagnostics: it reports whether a trainer is
// in the room, what class that trainer teaches, and how many
// level-ups (if any) the player has banked. Slice 3 turns the same
// resolver into the level-commit path (HP / BAB / saves recompute,
// audit, ClassLevels write).
//
// Today the verb's only mutation is the audit row when a future
// commit lands; slice 2 deliberately writes nothing.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/progression"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// errNoTrainerHere is the sentinel for "no trainer in the current
// room". Callers translate it into a player-facing line.
var errNoTrainerHere = errors.New("cmd: no trainer here")

// resolvedTrainer pairs a Trainer config with the mob in the room
// that backs it. Mirrors resolvedShop / resolvedBanker.
type resolvedTrainer struct {
	trainer   repo.Trainer
	keeper    creature.MobInstance
	keeperTpl creature.MobTemplate
}

func (r resolvedTrainer) keeperName() string {
	if r.keeperTpl.Core.Name != "" {
		return r.keeperTpl.Core.Name
	}
	return "the trainer"
}

// findTrainer walks the mobs in roomID and returns the first one
// whose template has a matching trainers row. Returns
// errNoTrainerHere if the room has no trainer-capable mob.
func findTrainer(ctx context.Context, roomID int64,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo, trainers repo.TrainerRepo,
) (resolvedTrainer, error) {
	if roomID == 0 {
		return resolvedTrainer{}, errNoTrainerHere
	}
	occupants, err := mobs.ListInRoom(ctx, roomID)
	if err != nil {
		return resolvedTrainer{}, fmt.Errorf("list room mobs: %w", err)
	}
	for _, m := range occupants {
		tr, err := trainers.GetByMobTemplateID(ctx, m.TemplateID)
		if errors.Is(err, repo.ErrTrainerNotFound) {
			continue
		}
		if err != nil {
			return resolvedTrainer{}, fmt.Errorf("get trainer: %w", err)
		}
		tpl, err := templates.GetByID(ctx, m.TemplateID)
		if err != nil {
			return resolvedTrainer{}, fmt.Errorf("get template: %w", err)
		}
		return resolvedTrainer{trainer: tr, keeper: m, keeperTpl: tpl}, nil
	}
	return resolvedTrainer{}, errNoTrainerHere
}

// NewTrain builds the `train` verb. Slice 3: commits one class level
// when the player has banked XP and is at a trainer for that class.
// Multiclass is at-will — visiting a different-class trainer opens
// the new class at level 1.
//
// Refusal paths (no trainer, not ready, catalog miss, repo error)
// do NOT audit. Successful commits write one admin_audit row via
// audit.Record(verb="train", target=classID, args="L<n>").
func NewTrain(characters repo.CharacterRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	trainers repo.TrainerRepo, cat *chargen.Catalog,
	audits repo.AdminAuditRepo,
) *telnet.Command {
	return &telnet.Command{
		Name: "train",
		Help: "Train — advance one class level with a trainer NPC",
		Long: "Usage: train\n\nMust be at a trainer for the class you want to advance.",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			res, err := findTrainer(c.Ctx, s.CurrentRoomID, mobs, templates, trainers)
			if errors.Is(err, errNoTrainerHere) {
				return s.WriteString("{{There's no trainer here.}}::yellow\r\n")
			}
			if err != nil {
				slog.Error("train: trainer lookup", "error", err)
				return s.WriteString("{{The trainer can't help you right now.}}::red\r\n")
			}

			// Hard refuse on catalog miss — a typoed YAML must not
			// dump the player into a broken progression state.
			cl, ok := lookupClass(cat, res.trainer.ClassID)
			if !ok {
				slog.Warn("train: trainer class not in catalog",
					"class_id", res.trainer.ClassID,
					"mob_template_id", res.trainer.MobTemplateID)
				return s.WriteString(fmt.Sprintf(
					"{{%s shakes their head. \"I can't help you advance — that path's broken.\"}}::red\r\n",
					res.keeperName()))
			}

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("train: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}

			level := progression.LevelForXP(char.XP)
			classTotal := characterLevel(char)
			pending := level - classTotal
			if pending <= 0 {
				return s.WriteString(fmt.Sprintf(
					"{{%s nods. \"You're not ready to advance yet.\"}}::yellow\r\n",
					res.keeperName()))
			}

			gains, err := progression.ComputeLevelUp(char, cat, cl.Enum)
			if err != nil {
				// Should be unreachable given lookupClass succeeded,
				// but guard against future catalog drift.
				slog.Error("train: compute level-up",
					"class_id", res.trainer.ClassID, "error", err)
				return s.WriteString("{{Something blocks the lesson. Try again later.}}::red\r\n")
			}

			if err := characters.RecordLevelUp(c.Ctx, char.ID, repo.LevelUpFields{
				ClassLevels:              gains.ClassLevels,
				HPCurrent:                gains.NewHPCurrent,
				HPMax:                    gains.NewHPMax,
				BAB:                      gains.NewBAB,
				Saves:                    gains.NewSaves,
				PendingFeatsDelta:        gains.FeatDelta,
				PendingSkillPointsDelta:  gains.SkillDelta,
				PendingAbilityBumpsDelta: gains.AbilityDelta,
				PendingWeavesDelta:       gains.WeaveDelta,
				PracticePointsDelta:      gains.PracticeDelta,
			}); err != nil {
				slog.Error("train: record level-up",
					"char", char.ID, "class_id", res.trainer.ClassID, "error", err)
				return s.WriteString("{{The lesson slips away as you reach for it.}}::red\r\n")
			}

			audit.Record(c.Ctx, audits, s, "train", res.trainer.ClassID,
				fmt.Sprintf("L%d", gains.NewLevel))

			if err := s.WriteString(fmt.Sprintf(
				"{{%s teaches you the next step. (%s — L%d, +%d HP)}}::green|bold\r\n",
				res.keeperName(), cl.Name, gains.NewLevel, gains.HPDelta)); err != nil {
				return err
			}
			if line := pendingGainsLine(gains); line != "" {
				return s.WriteString(line)
			}
			return nil
		},
	}
}

// pendingGainsLine renders the pending-pool deltas from a level-up
// as a single comma-joined line for the player. Returns "" when no
// pool changed (suppress the line entirely so the success message
// stays one line for boring levels).
func pendingGainsLine(g progression.LevelGains) string {
	parts := make([]string, 0, 4)
	if g.FeatDelta > 0 {
		parts = append(parts, pluralize(g.FeatDelta, "feat pick", "feat picks"))
	}
	if g.SkillDelta > 0 {
		parts = append(parts, pluralize(g.SkillDelta, "skill point", "skill points"))
	}
	if g.AbilityDelta > 0 {
		parts = append(parts, pluralize(g.AbilityDelta, "ability bump", "ability bumps"))
	}
	if g.WeaveDelta > 0 {
		parts = append(parts, pluralize(g.WeaveDelta, "weave slot", "weave slots"))
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += ", " + parts[i]
	}
	return fmt.Sprintf("{{You gained %s.}}::cyan\r\n", out)
}

func pluralize(n int32, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// lookupClass resolves a trainer's class id against the chargen
// catalog. Returns ok=false on nil catalog or unknown id; the cmd
// translates this into a player-facing refusal.
func lookupClass(cat *chargen.Catalog, id string) (*chargen.Class, bool) {
	if cat == nil {
		return nil, false
	}
	cl, ok := cat.Class(id)
	if !ok {
		return nil, false
	}
	return cl, true
}

