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

// NewTrain builds the `train` verb. Slice 2: read-only diagnostic
// only. The verb resolves the trainer in the current room, looks up
// the trainer's class against the chargen catalog, computes pending
// level-ups (LevelForXP - characterLevel), and reports the next step
// to the player. No state mutation.
func NewTrain(characters repo.CharacterRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	trainers repo.TrainerRepo, cat *chargen.Catalog,
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

			var className string
			if cat != nil {
				if cl, ok := cat.Class(res.trainer.ClassID); ok {
					className = cl.Name
				}
			}
			if className == "" {
				// Catalog miss is not a player-facing error in V1 —
				// the slice 3 commit path will refuse politely. Log
				// for ops so a builder typo surfaces.
				slog.Warn("train: trainer class not in catalog",
					"class_id", res.trainer.ClassID,
					"mob_template_id", res.trainer.MobTemplateID)
				className = res.trainer.ClassID
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

			// Slice 2 stops here — no mutation, no audit.
			return s.WriteString(fmt.Sprintf(
				"{{%s eyes you up and down. \"You could train as a %s — but that path's still being mapped.\"}}::cyan\r\n",
				res.keeperName(), className))
		},
	}
}
