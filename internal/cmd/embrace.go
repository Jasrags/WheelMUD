package cmd

// embrace / release toggle Channeling.Embraced for the calling
// player. While embraced, male channelers (Saidin) accrue Madness on
// each Regen pulse via internal/channeling.AccrueMadness — see Phase
// E #27. Stilled channelers cannot embrace.
//
// V1 ships only the toggle and the timestamp stamp. The full set of
// embrace effects (rest/heal blockers, same-gender perception,
// saidar aura, gender detection within 15 ft) is deferred — see
// docs/PLAN.md and docs/reference/the-one-power.md.

import (
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewEmbrace builds the `embrace` verb (Auth=Player).
func NewEmbrace(characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name: "embrace",
		Help: "embrace — open yourself to the True Source",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			return runEmbraceToggle(c, characters, true)
		},
	}
}

// NewRelease builds the `release` verb (Auth=Player).
func NewRelease(characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name: "release",
		Help: "release — let go of the True Source",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			return runEmbraceToggle(c, characters, false)
		},
	}
}

func runEmbraceToggle(c *telnet.Context, characters repo.CharacterRepo, embrace bool) error {
	s := c.Session
	char, err := characters.FindByName(c.Ctx, s.CharacterName)
	if err != nil {
		slog.Error("embrace: char lookup", "char", s.CharacterID, "error", err)
		return s.WriteString("{{You can't find your records.}}::red\r\n")
	}
	if char.Channeling == nil {
		return s.WriteString("{{You cannot channel.}}::yellow\r\n")
	}
	if char.Channeling.Stilled {
		return s.WriteString("{{The Source is beyond your reach. You have been stilled.}}::red\r\n")
	}
	if char.Channeling.Embraced == embrace {
		if embrace {
			return s.WriteString("{{You are already holding the Source.}}::yellow\r\n")
		}
		return s.WriteString("{{You are not holding the Source.}}::yellow\r\n")
	}
	char.Channeling.Embraced = embrace
	if embrace {
		char.Channeling.EmbracedSince = time.Now()
	} else {
		char.Channeling.EmbracedSince = time.Time{}
	}
	if err := characters.RecordChanneling(c.Ctx, char.ID, char.Channeling); err != nil {
		slog.Error("embrace: persist", "char", char.ID, "error", err)
		return s.WriteString("{{Could not change your grip on the Source.}}::red\r\n")
	}
	if embrace {
		return s.WriteString("{{You open yourself to the True Source. Power floods through you.}}::cyan\r\n")
	}
	return s.WriteString("{{You release the True Source. The Power fades.}}::cyan\r\n")
}
