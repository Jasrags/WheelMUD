package cmd

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewPvP returns the `pvp` command — show or toggle the character's
// player-vs-player opt-in flag.
//
//	pvp            — show current state
//	pvp on/off     — explicit set
//	pvp 1/0        — same
//	pvp true/false — same
//
// The room-side `nopvp` flag and the newbie level cap are independent
// gates checked by the `attack` verb; this flag is only the per-
// character consent half.
func NewPvP(characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name: "pvp",
		Help: "Show or toggle your PvP opt-in (pvp on|off)",
		Long: "Usage: pvp [on|off]\r\n" +
			"       Bare `pvp` shows your current state.\r\n" +
			"       Both attacker and defender must opt in before\r\n" +
			"       `attack <player>` is allowed; `nopvp` rooms and the\r\n" +
			"       newbie level cap (level " + strconv.Itoa(NewbiePvPLevelCap) +
			" and below) still refuse regardless.\r\n",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CharacterID == 0 {
				return s.WriteString("{{PvP unavailable on this session.}}::yellow\r\n")
			}
			ch, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("pvp: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{Could not load your character.}}::red\r\n")
			}
			if len(c.Args) == 0 {
				return s.WriteString(pvpStateLine(ch.PvP))
			}
			on, ok := parsePvPArg(c.Args[0])
			if !ok {
				return s.WriteString("{{Usage: pvp [on|off]}}::yellow\r\n")
			}
			if on == ch.PvP {
				return s.WriteString(pvpStateLine(ch.PvP))
			}
			if err := characters.RecordPvP(c.Ctx, ch.ID, on); err != nil {
				slog.Warn("pvp: record failed", "char", ch.ID, "error", err)
				return s.WriteString("{{Could not save your PvP setting.}}::red\r\n")
			}
			return s.WriteString(pvpStateLine(on))
		},
	}
}

func pvpStateLine(on bool) string {
	if on {
		return "{{PvP: on}}::red\r\n"
	}
	return "{{PvP: off}}::green\r\n"
}

func parsePvPArg(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "1", "true", "yes", "y":
		return true, true
	case "off", "0", "false", "no", "n":
		return false, true
	}
	return false, false
}

