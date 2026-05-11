package cmd

// bump is the spend verb for pending_ability_bumps (Phase E #25).
// Players type `bump <abil>` to spend one pending bump and raise the
// chosen ability's Current score by 1. The pool was deposited by the
// level-up commit (Phase E #23 slice 4) at L4/8/12/16/20.
//
// V1 scope:
//   - Hard cap of 20 per ability score; refuse on overflow.
//   - 1 pending bump per +1.
//   - Anywhere, anytime.
//
// Refusals (empty pool, unknown ability, cap) do NOT mutate or audit.
// Successful spends write one admin_audit row.

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// AbilityBumpCap is the hard ceiling per ability score for the `bump`
// verb. d20 baseline.
const AbilityBumpCap int8 = 20

// NewBump builds the `bump` verb.
func NewBump(characters repo.CharacterRepo,
	audits repo.AdminAuditRepo,
) *telnet.Command {
	return &telnet.Command{
		Name: "bump",
		Help: "Bump — spend a pending ability bump on str/dex/con/int/wis/cha",
		Long: "Usage: bump                show pending bumps and current scores\n" +
			"       bump <ability>      spend one bump (str/dex/con/int/wis/cha)",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("bump: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}
			if len(c.Args) == 0 {
				return writeBumpMenu(s, char)
			}
			key, ok := parseAbilityKey(c.Args[0])
			if !ok {
				return s.WriteString(fmt.Sprintf(
					"{{Unknown ability \"%s\". Try str/dex/con/int/wis/cha.}}::yellow\r\n",
					display.Defang(c.Args[0], ""),
				))
			}
			return commitBump(c, characters, audits, char, key)
		},
	}
}

// writeBumpMenu renders the current score table and pending count.
func writeBumpMenu(s *telnet.Session, char repo.Character) error {
	if err := display.SectionHeader(s, "Ability Bumps"); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Bumps available",
		fmt.Sprintf("%d", char.PendingAbilityBumps), 18); err != nil {
		return err
	}
	if err := display.Subsection(s, "Current scores"); err != nil {
		return err
	}
	rows := []struct {
		key   string
		score int8
	}{
		{"Strength", char.Core.Abilities.Str.Current},
		{"Dexterity", char.Core.Abilities.Dex.Current},
		{"Constitution", char.Core.Abilities.Con.Current},
		{"Intelligence", char.Core.Abilities.Int.Current},
		{"Wisdom", char.Core.Abilities.Wis.Current},
		{"Charisma", char.Core.Abilities.Cha.Current},
	}
	for _, r := range rows {
		atCap := ""
		if r.score >= AbilityBumpCap {
			atCap = "  {{(at cap)}}::gray"
		}
		if err := s.WriteString(fmt.Sprintf(
			"  {{%-12s}}::yellow|bold  {{%2d}}::green|bold/%d%s\r\n",
			r.key, r.score, AbilityBumpCap, atCap,
		)); err != nil {
			return err
		}
	}
	return s.WriteString(
		"\r\n  Usage: {{bump <ability>}}::yellow|bold  " +
			"(str/dex/con/int/wis/cha)\r\n",
	)
}

// commitBump enforces empty-pool + cap then calls RecordAbilityBump.
func commitBump(c *telnet.Context, characters repo.CharacterRepo,
	audits repo.AdminAuditRepo, char repo.Character, key repo.AbilityKey,
) error {
	s := c.Session
	if char.PendingAbilityBumps <= 0 {
		return s.WriteString("{{No ability bumps available.}}::yellow\r\n")
	}
	cur := abilityScore(char, key)
	if cur >= AbilityBumpCap {
		return s.WriteString(fmt.Sprintf(
			"{{That would push %s past %d.}}::yellow\r\n",
			abilityFullName(key.String()), AbilityBumpCap,
		))
	}
	newScore := cur + 1
	newPending := char.PendingAbilityBumps - 1
	if err := characters.RecordAbilityBump(c.Ctx, char.ID, key, newScore, newPending); err != nil {
		slog.Error("bump: record ability bump",
			"char", char.ID, "ability", key.String(), "error", err)
		return s.WriteString("{{Your training falters.}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, s, "bump", key.String(),
		fmt.Sprintf("score=%d", newScore))

	bumps := "bumps remain"
	if newPending == 1 {
		bumps = "bump remains"
	}
	return s.WriteString(fmt.Sprintf(
		"{{Your %s rises to %d.}}::green|bold  (%d → %d)  %d %s.\r\n",
		abilityFullName(key.String()), newScore, cur, newScore,
		newPending, bumps,
	))
}

// parseAbilityKey maps a player-typed token (case-insensitive, 3-letter
// abbreviation or full name) to the AbilityKey enum.
func parseAbilityKey(token string) (repo.AbilityKey, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "str", "strength":
		return repo.AbilityStr, true
	case "dex", "dexterity":
		return repo.AbilityDex, true
	case "con", "constitution":
		return repo.AbilityCon, true
	case "int", "intelligence":
		return repo.AbilityInt, true
	case "wis", "wisdom":
		return repo.AbilityWis, true
	case "cha", "charisma":
		return repo.AbilityCha, true
	}
	return 0, false
}

// abilityScore reads a Character's current ability score by enum.
func abilityScore(char repo.Character, key repo.AbilityKey) int8 {
	a := char.Core.Abilities
	switch key {
	case repo.AbilityStr:
		return a.Str.Current
	case repo.AbilityDex:
		return a.Dex.Current
	case repo.AbilityCon:
		return a.Con.Current
	case repo.AbilityInt:
		return a.Int.Current
	case repo.AbilityWis:
		return a.Wis.Current
	case repo.AbilityCha:
		return a.Cha.Current
	}
	return 0
}
