package cmd

// Phase E #26 slice B — per-skill cooldown verbs.
//
// `cooldowns` (Player) lists the caller's active per-skill cooldowns
// sorted alphabetically by skill id. Past-deadline entries are
// omitted; empty maps print "(no active cooldowns)".
// `cooldown` (Admin, audited) stamps or clears a cooldown deadline
// on an online target. Mirrors the affects #25 admin-producer
// pattern — the only V1 writer of skill_cooldowns_json. Real
// player-driven skill checks (track / hide / lockpick) will gain
// stamping when those verbs grow real skill-check gates.

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewCooldowns builds the `cooldowns` player verb.
func NewCooldowns(characters repo.CharacterRepo, cat *chargen.Catalog) *telnet.Command {
	return &telnet.Command{
		Name: "cooldowns",
		Help: "cooldowns — list per-skill cooldowns currently affecting you",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			char, err := characters.GetByID(c.Ctx, s.CharacterID)
			if errors.Is(err, repo.ErrCharacterNotFound) {
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}
			if err != nil {
				slog.Error("cooldowns: load self", "char", s.CharacterID, "error", err)
				return s.WriteString("{{Could not read your cooldowns.}}::red\r\n")
			}
			now := time.Now()
			active := liveSkillCooldowns(char.SkillCooldowns, now)
			if len(active) == 0 {
				return s.WriteString("{{(no active cooldowns)}}::yellow\r\n")
			}
			lines := make([]string, 0, len(active))
			for skillID, deadline := range active {
				name := skillDisplayName(cat, skillID)
				secs := int64(deadline.Sub(now).Round(time.Second) / time.Second)
				if secs < 1 {
					secs = 1
				}
				lines = append(lines, fmt.Sprintf("  %-22s ~%ds", sanitizeArg(name), secs))
			}
			sort.Strings(lines)
			var b strings.Builder
			b.WriteString("{{Active cooldowns:}}::cyan\r\n")
			for _, line := range lines {
				b.WriteString(line)
				b.WriteString("\r\n")
			}
			return s.WriteString(b.String())
		},
	}
}

// NewCooldown builds the `cooldown` admin verb.
func NewCooldown(characters repo.CharacterRepo, sessions *session.Registry, audits repo.AdminAuditRepo, cat *chargen.Catalog) *telnet.Command {
	return &telnet.Command{
		Name: "cooldown",
		Help: "cooldown <player> <skill-id> <seconds> — stamp a per-skill cooldown",
		Long: "Usage: cooldown <player> <skill-id> <seconds>\n\n" +
			"<seconds>=0 clears the entry. Negative seconds are rejected.\n" +
			"<skill-id> resolves case-insensitively against the chargen\n" +
			"skill catalog (id or display name).",
		Auth:    telnet.AuthAdmin,
		MinArgs: 3,
		Run: func(c *telnet.Context) error {
			s := c.Session
			peer := lookupByCharacter(sessions, c.Args[0])
			if peer == nil {
				return s.WriteString("{{No such player online: " + sanitizeArg(c.Args[0]) + "}}::red\r\n")
			}
			skillID, ok := resolveSkillToken(c.Args[1], cat)
			if !ok {
				return s.WriteString("{{Unknown skill: " + sanitizeArg(c.Args[1]) + "}}::red\r\n")
			}
			seconds, err := strconv.Atoi(c.Args[2])
			if err != nil || seconds < 0 {
				return s.WriteString("{{Bad seconds: " + sanitizeArg(c.Args[2]) + " (must be >= 0).}}::red\r\n")
			}
			var deadline time.Time
			if seconds > 0 {
				deadline = time.Now().Add(time.Duration(seconds) * time.Second)
			}
			key := chargen.HashID(skillID)
			if err := characters.RecordSkillCooldown(c.Ctx, peer.CharacterID, key, deadline); err != nil {
				slog.Error("cooldown: persist", "char", peer.CharacterID, "skill", skillID, "error", err)
				return s.WriteString("{{Could not stamp cooldown for " + sanitizeArg(peer.CharacterName) + ".}}::red\r\n")
			}
			audit.Record(c.Ctx, audits, s, "cooldown", peer.CharacterName,
				fmt.Sprintf("skill=%s seconds=%d", skillID, seconds))
			if seconds == 0 {
				_ = peer.WriteAsync("{{Your " + sanitizeArg(skillID) + " cooldown clears.}}::cyan")
				return s.WriteString("{{Cleared " + sanitizeArg(skillID) + " cooldown on " +
					sanitizeArg(peer.CharacterName) + ".}}::green\r\n")
			}
			_ = peer.WriteAsync("{{Your " + sanitizeArg(skillID) + " is on cooldown for " +
				strconv.Itoa(seconds) + "s.}}::cyan")
			return s.WriteString("{{Stamped " + sanitizeArg(skillID) + " cooldown on " +
				sanitizeArg(peer.CharacterName) + " (" + strconv.Itoa(seconds) + "s).}}::green\r\n")
		},
	}
}

// liveSkillCooldowns returns a copy of in with past-now entries
// dropped. Pure helper; the SQL writer also prunes on-write.
func liveSkillCooldowns(in map[int32]time.Time, now time.Time) map[int32]time.Time {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int32]time.Time, len(in))
	for k, v := range in {
		if v.After(now) {
			out[k] = v
		}
	}
	return out
}

// skillDisplayName looks up the catalog id for the given hash key.
// Falls back to "#<hash>" when the catalog can't reverse-resolve
// (e.g. a stale hash from a removed skill).
func skillDisplayName(cat *chargen.Catalog, key int32) string {
	if cat != nil {
		for _, sk := range cat.Skills() {
			if chargen.HashID(sk.ID) == key {
				return sk.ID
			}
		}
	}
	return "#" + strconv.FormatInt(int64(key), 10)
}

// resolveSkillToken matches a user-supplied token against the
// chargen skill catalog. Case-insensitive. Matches both id and
// display Name. Returns the canonical catalog id on hit.
func resolveSkillToken(token string, cat *chargen.Catalog) (string, bool) {
	if cat == nil || token == "" {
		return "", false
	}
	if sk, ok := cat.Skill(strings.ToLower(token)); ok {
		return sk.ID, true
	}
	for _, sk := range cat.Skills() {
		if strings.EqualFold(sk.ID, token) || strings.EqualFold(sk.Name, token) {
			return sk.ID, true
		}
	}
	return "", false
}
