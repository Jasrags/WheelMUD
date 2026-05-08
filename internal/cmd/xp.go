package cmd

// xp surfaces the player's current XP total, level, and progress to
// the next threshold. Read-only — it does NOT advance the character;
// level commits land at a trainer NPC (#23 slice 3).
//
// The "level-up available" line shows whenever LevelForXP(XP) exceeds
// the sum of ClassLevels — XP has crossed a threshold but no `train`
// has cashed it in yet. At MaxLevel the next-threshold fields render
// as "—" and the level-up line is suppressed.

import (
	"fmt"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/progression"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

const xpLabelGutter = 14

// NewXP builds the xp command.
func NewXP(characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "xp",
		Aliases: []string{"experience"},
		Help:    "Show your XP and progress to the next level",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			ch, err := characters.FindByName(c.Ctx, c.Session.CharacterName)
			if err != nil {
				return c.Session.WriteString(
					"{{Could not load your character.}}::red\r\n")
			}
			return renderXP(c.Session, ch)
		},
	}
}

// renderXP writes the XP summary block to s. Broken out so tests can
// drive it without the registry / context machinery.
func renderXP(s *telnet.Session, ch repo.Character) error {
	if err := display.SectionHeader(s, "Experience — "+ch.Name); err != nil {
		return err
	}

	level, toNext := progression.XPToNext(ch.XP)
	classTotal := characterLevel(ch)

	if err := display.FieldRow(s, "Level", fmt.Sprintf("%d", level), xpLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Class total", fmt.Sprintf("%d", classTotal), xpLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "XP", fmt.Sprintf("%d", ch.XP), xpLabelGutter); err != nil {
		return err
	}

	if level >= progression.MaxLevel {
		if err := display.FieldRow(s, "Next at", "—", xpLabelGutter); err != nil {
			return err
		}
		if err := display.FieldRow(s, "To next", "—", xpLabelGutter); err != nil {
			return err
		}
	} else {
		nextAt := progression.XPForLevel(level + 1)
		if err := display.FieldRow(s, "Next at", fmt.Sprintf("%d", nextAt), xpLabelGutter); err != nil {
			return err
		}
		if err := display.FieldRow(s, "To next", fmt.Sprintf("%d", toNext), xpLabelGutter); err != nil {
			return err
		}
	}

	if ch.XPDebt > 0 {
		if err := display.FieldRow(s, "XP debt",
			fmt.Sprintf("%d", ch.XPDebt), xpLabelGutter); err != nil {
			return err
		}
		if err := s.WriteString(
			"  {{(drained off your next XP awards before they credit)}}::gray\r\n",
		); err != nil {
			return err
		}
	}

	if pending := level - classTotal; pending > 0 {
		word := "level-up"
		if pending != 1 {
			word = "level-ups"
		}
		if err := s.WriteString(fmt.Sprintf(
			"\r\n{{*** %d %s available — find a trainer. ***}}::green|bold\r\n",
			pending, word)); err != nil {
			return err
		}
	}

	return display.Rule(s)
}
