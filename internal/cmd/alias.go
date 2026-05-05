package cmd

import (
	"strings"

	"github.com/Jasrags/WheelMUD/telnet"
)

// NewAlias returns the `alias` command:
//
//	alias                       — list all aliases for this session
//	alias <name>                — show a single alias's expansion
//	alias <name> <expansion...> — define / replace an alias
//	alias <name>=<expansion>    — same, equals form
//
// Aliases are session-scoped only — they do NOT persist across
// reconnects. Persisting on the character is the next step on the
// roadmap; until then a new login starts with an empty table.
func NewAlias() *telnet.Command {
	return &telnet.Command{
		Name: "alias",
		Help: "Define a per-session command alias",
		Long: "Usage: alias                       - list aliases\n" +
			"       alias <name>                - show one alias\n" +
			"       alias <name> <expansion>    - define alias\n" +
			"       alias <name>=<expansion>    - same, equals form\n\n" +
			"Aliases are resolved once before lookup, so chained\n" +
			"aliases (alias-of-alias) do not recurse.",
		Auth: telnet.AuthGuest,
		Run: func(c *telnet.Context) error {
			tbl := c.Session.Aliases
			if tbl == nil {
				return c.Session.WriteRaw([]byte("Aliases unavailable on this session.\r\n"))
			}
			if len(c.Args) == 0 {
				return listAliases(c.Session, tbl)
			}
			name, expansion := parseAliasArgs(c.Args, c.Raw)
			if expansion == "" {
				if v, ok := tbl.Lookup(name); ok {
					return c.Session.WriteRaw([]byte(name + " = " + v + "\r\n"))
				}
				return c.Session.WriteRaw([]byte("No such alias: " + name + "\r\n"))
			}
			if !tbl.Set(name, expansion) {
				return c.Session.WriteRaw([]byte("Invalid alias name or expansion.\r\n"))
			}
			return c.Session.WriteRaw([]byte("alias " + name + " = " + expansion + "\r\n"))
		},
	}
}

// NewUnalias returns the `unalias` command. Removes one or more
// aliases by name. Reports per-name success/failure inline.
func NewUnalias() *telnet.Command {
	return &telnet.Command{
		Name:    "unalias",
		Help:    "unalias <name> [name...] — remove one or more session aliases",
		Auth:    telnet.AuthGuest,
		MinArgs: 1,
		Run: func(c *telnet.Context) error {
			tbl := c.Session.Aliases
			if tbl == nil {
				return c.Session.WriteRaw([]byte("Aliases unavailable on this session.\r\n"))
			}
			var b strings.Builder
			for _, name := range c.Args {
				if tbl.Delete(name) {
					b.WriteString("Removed alias " + strings.ToLower(name) + "\r\n")
				} else {
					b.WriteString("No such alias: " + name + "\r\n")
				}
			}
			return c.Session.WriteRaw([]byte(b.String()))
		},
	}
}

// parseAliasArgs splits an alias declaration into (name, expansion).
// Two surface forms:
//   - `alias name=value rest of line`  → name="name", exp="value rest of line"
//   - `alias name rest of line`        → name="name", exp="rest of line"
//
// raw is the full args line as `Context.Raw` delivers it: the post-verb
// remainder with leading/trailing whitespace stripped, so it always
// begins with the first argument token. We rely on that invariant to
// skip the name by length rather than by substring scan, which would
// be ambiguous when the name appears again later in the expansion.
func parseAliasArgs(args []string, raw string) (name, expansion string) {
	first := args[0]
	if eq := strings.IndexByte(first, '='); eq > 0 {
		// Equals form: `alias ll=look north` → name "ll", expansion
		// is everything after the first '=' in raw. '=' cannot
		// appear in a valid verb so the first '=' is unambiguous.
		name = first[:eq]
		if idx := strings.IndexByte(raw, '='); idx >= 0 {
			expansion = strings.TrimSpace(raw[idx+1:])
		}
		return name, expansion
	}
	name = first
	if len(args) == 1 {
		return name, ""
	}
	// raw starts with `first`; advance by its byte length and trim the
	// separator(s).
	if len(raw) < len(first) {
		return name, ""
	}
	expansion = strings.TrimSpace(raw[len(first):])
	return name, expansion
}

func listAliases(s *telnet.Session, tbl *telnet.AliasTable) error {
	names, exps := tbl.All()
	if len(names) == 0 {
		return s.WriteRaw([]byte("No aliases defined.\r\n"))
	}
	var b strings.Builder
	b.WriteString("Aliases:\r\n")
	for i, n := range names {
		b.WriteString("  ")
		b.WriteString(n)
		b.WriteString(" = ")
		b.WriteString(exps[i])
		b.WriteString("\r\n")
	}
	return s.WriteRaw([]byte(b.String()))
}
