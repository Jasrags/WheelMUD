package cmd

// Phase E #25 slice 1 — affects verbs.
//
// `affects` (Player) lists the calling character's live buffs/debuffs.
// `affect` (Admin) applies a creature.Affect to an online target with
// the supplied StatMod list and duration in ticks.
// `dispel` (Admin) clears one or all affects from an online target.
//
// V1 producers are admin-only; weave/consumable/combat-hit producers
// are deferred. The Source field on admin-applied affects is a
// sentinel (-1) so the inspect verb can render "(admin)" instead of
// trying to resolve a character id.

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// adminAffectSource is the sentinel Source id stamped on affects
// applied by the `affect` admin verb. Distinguishes admin-injected
// affects from caster-id (>0) and unknown (0) Sources in the inspect
// verb's display resolver.
const adminAffectSource int64 = -1

// consumableAffectSource is the sentinel Source stamped on affects
// applied by `quaff` (Phase E #25 slice 2). Sibling to
// adminAffectSource; renders as "potion" in the inspect verb.
const consumableAffectSource int64 = -2

// LuaAffectSource is the sentinel Source stamped on affects applied
// by Lua scripts via the `apply_affect` binding (Phase F #32 slice
// 3). Sibling to adminAffectSource / consumableAffectSource;
// renders as "script" in the inspect verb. Exported so the
// cmd/server/main.go closure that wires the binding can stamp it
// without re-importing this package's internals.
const LuaAffectSource int64 = -3

// NewAffects builds the `affects` player verb. Lists the caller's
// live affects with name, modifiers, source, and remaining duration.
func NewAffects(characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name: "affects",
		Help: "affects — list buffs and debuffs currently affecting you",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			char, err := characters.GetByID(c.Ctx, s.CharacterID)
			if errors.Is(err, repo.ErrCharacterNotFound) {
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}
			if err != nil {
				slog.Error("affects: load self", "char", s.CharacterID, "error", err)
				return s.WriteString("{{Could not read your affects.}}::red\r\n")
			}
			if len(char.Core.Affects) == 0 {
				return s.WriteString("{{(no affects)}}::yellow\r\n")
			}
			var b strings.Builder
			b.WriteString("{{Active affects:}}::cyan\r\n")
			for i, a := range char.Core.Affects {
				fmt.Fprintf(&b, "  %d) %-20s %-32s ~%ds left  [%s]\r\n",
					i+1,
					sanitizeArg(a.Name),
					renderModifiers(a.Modifiers),
					int(a.DurationTicks)*affects.TickSeconds,
					resolveAffectSource(c, characters, a.Source),
				)
			}
			return s.WriteString(b.String())
		},
	}
}

// NewAffect builds the `affect` admin verb.
func NewAffect(characters repo.CharacterRepo, sessions *session.Registry, audits repo.AdminAuditRepo) *telnet.Command {
	return &telnet.Command{
		Name: "affect",
		Help: "affect <player> <name> [<field>=<delta>...] [duration=<ticks>] — apply an affect",
		Long: "Usage: affect <player> <name> [<field>=<delta>...] [duration=<ticks>]\n\n" +
			"Applies a buff/debuff to an online target.\n\n" +
			"<field> is one of:\n" +
			"  Str.Current  Dex.Current  Con.Current  Int.Current  Wis.Current  Cha.Current\n" +
			"  Defense  Saves.Fort  Saves.Ref  Saves.Will  Speed.BaseFt  BAB\n\n" +
			"<delta> is a signed integer (e.g. +2, -1).\n" +
			"duration defaults to 10 ticks if omitted (one tick = " + strconv.Itoa(affects.TickSeconds) + "s).\n\n" +
			"Example: affect Bob blessed Saves.Will=+1 Defense=+1 duration=20",
		Auth:    telnet.AuthAdmin,
		MinArgs: 2,
		Run: func(c *telnet.Context) error {
			s := c.Session
			peer := lookupByCharacter(sessions, c.Args[0])
			if peer == nil {
				return s.WriteString("{{No such player online: " + sanitizeArg(c.Args[0]) + "}}::red\r\n")
			}
			name := strings.TrimSpace(c.Args[1])
			if name == "" {
				return s.WriteString("{{Affect name cannot be empty.}}::red\r\n")
			}
			mods, duration, err := parseAffectKVPairs(c.Args[2:])
			if err != nil {
				return s.WriteString("{{" + sanitizeArg(err.Error()) + "}}::red\r\n" + allowedFieldsHint())
			}
			if duration <= 0 {
				duration = 10
			}
			char, err := characters.GetByID(c.Ctx, peer.CharacterID)
			if errors.Is(err, repo.ErrCharacterNotFound) {
				return s.WriteString("{{No such player: " + sanitizeArg(peer.CharacterName) + "}}::red\r\n")
			}
			if err != nil {
				slog.Error("affect: load target", "char", peer.CharacterID, "error", err)
				return s.WriteString("{{Could not affect " + sanitizeArg(peer.CharacterName) + ".}}::red\r\n")
			}
			next := affects.Apply(char.Core.Affects, creature.Affect{
				Source:        adminAffectSource,
				Name:          name,
				Modifiers:     mods,
				DurationTicks: int32(duration),
			})
			if err := characters.RecordAffects(c.Ctx, char.ID, next); err != nil {
				slog.Error("affect: persist", "char", char.ID, "error", err)
				return s.WriteString("{{Could not affect " + sanitizeArg(peer.CharacterName) + ".}}::red\r\n")
			}
			audit.Record(c.Ctx, audits, s, "affect", peer.CharacterName,
				fmt.Sprintf("name=%s mods=%s duration=%d", name, renderModifiers(mods), duration))
			_ = peer.WriteAsync("{{You feel " + sanitizeArg(name) + " take hold.}}::cyan")
			return s.WriteString("{{Applied " + sanitizeArg(name) + " to " +
				sanitizeArg(peer.CharacterName) + " (" + renderModifiers(mods) +
				", " + strconv.Itoa(duration) + " ticks).}}::green\r\n")
		},
	}
}

// NewDispel builds the `dispel` admin verb.
func NewDispel(characters repo.CharacterRepo, sessions *session.Registry, audits repo.AdminAuditRepo) *telnet.Command {
	return &telnet.Command{
		Name: "dispel",
		Help: "dispel <player> [<name>] — clear one or all affects from a player",
		Long: "Usage: dispel <player>          - clear all affects\n" +
			"       dispel <player> <name>   - clear the named affect only\n\n" +
			"<player> matches an online character name (case-insensitive).\n" +
			"No-op (no audit, no notify) when the target has no matching affect.",
		Auth:    telnet.AuthAdmin,
		MinArgs: 1,
		Run: func(c *telnet.Context) error {
			s := c.Session
			peer := lookupByCharacter(sessions, c.Args[0])
			if peer == nil {
				return s.WriteString("{{No such player online: " + sanitizeArg(c.Args[0]) + "}}::red\r\n")
			}
			char, err := characters.GetByID(c.Ctx, peer.CharacterID)
			if errors.Is(err, repo.ErrCharacterNotFound) {
				return s.WriteString("{{No such player: " + sanitizeArg(peer.CharacterName) + "}}::red\r\n")
			}
			if err != nil {
				slog.Error("dispel: load target", "char", peer.CharacterID, "error", err)
				return s.WriteString("{{Could not dispel " + sanitizeArg(peer.CharacterName) + ".}}::red\r\n")
			}
			var name string
			if len(c.Args) >= 2 {
				name = strings.TrimSpace(c.Args[1])
			}
			next := filterAffects(char.Core.Affects, name)
			if len(next) == len(char.Core.Affects) {
				return s.WriteString("{{" + sanitizeArg(peer.CharacterName) + " has no matching affect.}}::yellow\r\n")
			}
			if err := characters.RecordAffects(c.Ctx, char.ID, next); err != nil {
				slog.Error("dispel: persist", "char", char.ID, "error", err)
				return s.WriteString("{{Could not dispel " + sanitizeArg(peer.CharacterName) + ".}}::red\r\n")
			}
			target := peer.CharacterName
			args := target
			if name != "" {
				args = target + " " + name
			}
			audit.Record(c.Ctx, audits, s, "dispel", target, args)
			if name == "" {
				_ = peer.WriteAsync("{{You feel every lingering weave unravel.}}::cyan")
				return s.WriteString("{{Cleared all affects from " + sanitizeArg(target) + ".}}::green\r\n")
			}
			_ = peer.WriteAsync("{{You feel " + sanitizeArg(name) + " unravel.}}::cyan")
			return s.WriteString("{{Cleared " + sanitizeArg(name) + " from " + sanitizeArg(target) + ".}}::green\r\n")
		},
	}
}

// parseAffectKVPairs walks the trailing `key=value` argument tokens
// and produces a StatMod list + a duration in ticks. Unknown keys
// return an error with the offending token.
func parseAffectKVPairs(args []string) ([]creature.StatMod, int, error) {
	var (
		mods     []creature.StatMod
		duration int
	)
	for _, raw := range args {
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 || eq == len(raw)-1 {
			return nil, 0, fmt.Errorf("expected key=value, got %q", raw)
		}
		key := strings.TrimSpace(raw[:eq])
		val := strings.TrimSpace(raw[eq+1:])
		if strings.EqualFold(key, "duration") {
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				return nil, 0, fmt.Errorf("bad duration: %q", val)
			}
			duration = n
			continue
		}
		canonical, ok := canonicalStatModField(key)
		if !ok {
			return nil, 0, fmt.Errorf("unknown field: %q", key)
		}
		n, err := strconv.Atoi(strings.TrimPrefix(val, "+"))
		if err != nil {
			return nil, 0, fmt.Errorf("bad delta for %s: %q", canonical, val)
		}
		if n < -32768 || n > 32767 {
			return nil, 0, fmt.Errorf("delta out of range for %s: %d", canonical, n)
		}
		mods = append(mods, creature.StatMod{Field: canonical, Delta: int16(n)})
	}
	return mods, duration, nil
}

// canonicalStatModField resolves a user-supplied field name (case-
// insensitive) against the affects allow-list. Returns the exact
// catalogued spelling so applyMod's switch matches.
func canonicalStatModField(in string) (string, bool) {
	for _, f := range affects.AllowedStatModFields {
		if strings.EqualFold(in, f) {
			return f, true
		}
	}
	return "", false
}

// allowedFieldsHint renders the allow-list as a one-line refusal
// suffix so the admin sees the legal field names without consulting
// the help.
func allowedFieldsHint() string {
	return "{{Fields: " + strings.Join(affects.AllowedStatModFields, ", ") +
		". Special key: duration=<ticks>.}}::yellow\r\n"
}

// renderModifiers prints a StatMod list as a human-readable string
// like "Str.Current +2, Saves.Will -1". Empty input renders as
// "(no modifiers)".
func renderModifiers(mods []creature.StatMod) string {
	if len(mods) == 0 {
		return "(no modifiers)"
	}
	parts := make([]string, 0, len(mods))
	for _, m := range mods {
		sign := "+"
		if m.Delta < 0 {
			sign = ""
		}
		parts = append(parts, fmt.Sprintf("%s %s%d", m.Field, sign, m.Delta))
	}
	// Stable order helps tests and forensic logs without forcing
	// callers to pre-sort their input.
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// filterAffects returns a new slice with the named affect removed.
// If name is empty the result is nil (clear-all). Stable order.
func filterAffects(in []creature.Affect, name string) []creature.Affect {
	if name == "" {
		return nil
	}
	out := make([]creature.Affect, 0, len(in))
	for _, a := range in {
		if strings.EqualFold(a.Name, name) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// resolveAffectSource maps an Affect.Source id to a display string for
// the inspect verb. -1 = admin sentinel, 0 = unknown, >0 = look up
// character name (falls back to "#<id>" if the row no longer exists).
func resolveAffectSource(c *telnet.Context, characters repo.CharacterRepo, src int64) string {
	switch {
	case src == adminAffectSource:
		return "admin"
	case src == consumableAffectSource:
		return "potion"
	case src == LuaAffectSource:
		return "script"
	case src == 0:
		return "unknown"
	default:
		caster, err := characters.GetByID(c.Ctx, src)
		if err != nil || caster.Name == "" {
			return "#" + strconv.FormatInt(src, 10)
		}
		return caster.Name
	}
}
