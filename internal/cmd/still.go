package cmd

// still / unstill are the AuthAdmin verbs that flip
// Channeling.Stilled on an online target. A stilled channeler is
// permanently severed from the Source — slot refresh skips them and
// their slots are zeroed at flip time so the gate is observable
// immediately. unstill clears the flag but does NOT auto-refill —
// the next 8h refresh pulse from internal/channeling restores
// slots on its own cadence.
//
// Online-only this slice — offline targets refuse rather than
// touching the DB blind. Both verbs audit on success; refusals do
// not.

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewStill builds the `still` admin verb.
func NewStill(characters repo.CharacterRepo, sessions *session.Registry, audits repo.AdminAuditRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "still",
		Help:    "still <player> — sever a channeler from the Source",
		Long:    "Usage: still <player>\n\n<player> matches an online character name.",
		Auth:    telnet.AuthAdmin,
		MinArgs: 1,
		Run: func(c *telnet.Context) error {
			return runStillToggle(c, characters, sessions, audits, true, "still")
		},
	}
}

// NewUnstill builds the `unstill` admin verb.
func NewUnstill(characters repo.CharacterRepo, sessions *session.Registry, audits repo.AdminAuditRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "unstill",
		Help:    "unstill <player> — restore a stilled channeler",
		Long:    "Usage: unstill <player>\n\n<player> matches an online character name.\nSlots refill on the next 8h refresh pulse, not immediately.",
		Auth:    telnet.AuthAdmin,
		MinArgs: 1,
		Run: func(c *telnet.Context) error {
			return runStillToggle(c, characters, sessions, audits, false, "unstill")
		},
	}
}

func runStillToggle(c *telnet.Context, characters repo.CharacterRepo, sessions *session.Registry, audits repo.AdminAuditRepo, stilled bool, verb string) error {
	s := c.Session
	name := c.Args[0]
	peer := lookupByCharacter(sessions, name)
	if peer == nil {
		return s.WriteString("{{No such player online: " + sanitizeArg(name) + "}}::red\r\n")
	}
	char, err := characters.FindByName(c.Ctx, peer.CharacterName)
	if errors.Is(err, repo.ErrCharacterNotFound) {
		return s.WriteString("{{No such player: " + sanitizeArg(peer.CharacterName) + "}}::red\r\n")
	}
	if err != nil {
		slog.Error(verb+": char lookup", "char", peer.CharacterID, "error", err)
		return s.WriteString("{{Could not " + verb + " " + sanitizeArg(peer.CharacterName) + ".}}::red\r\n")
	}
	if char.Channeling == nil {
		return s.WriteString("{{" + sanitizeArg(peer.CharacterName) + " cannot channel.}}::yellow\r\n")
	}
	if char.Channeling.Stilled == stilled {
		if stilled {
			return s.WriteString("{{" + sanitizeArg(peer.CharacterName) + " is already stilled.}}::yellow\r\n")
		}
		return s.WriteString("{{" + sanitizeArg(peer.CharacterName) + " is not stilled.}}::yellow\r\n")
	}
	char.Channeling.Stilled = stilled
	if stilled {
		for i := range char.Channeling.Slots {
			char.Channeling.Slots[i].Cur = 0
		}
	}
	if err := characters.RecordChanneling(c.Ctx, char.ID, char.Channeling); err != nil {
		slog.Error(verb+": persist", "char", char.ID, "error", err)
		return s.WriteString("{{Could not " + verb + " " + sanitizeArg(peer.CharacterName) + ".}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, s, verb, peer.CharacterName, strings.Join(c.Args, " "))
	if stilled {
		_ = peer.WriteAsync("{{You feel the True Source torn from your reach.}}::red\r\n")
		return s.WriteString("{{You sever " + sanitizeArg(peer.CharacterName) + " from the Source.}}::cyan\r\n")
	}
	_ = peer.WriteAsync("{{You feel the True Source within reach once more.}}::cyan\r\n")
	return s.WriteString("{{You restore " + sanitizeArg(peer.CharacterName) + " to the Source.}}::cyan\r\n")
}
