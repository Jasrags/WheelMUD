package cmd

import (
	"strings"

	"github.com/Jasrags/WheelMUD/telnet"
)

// NewHelp builds a help command bound to the given registry. The command
// supports `help` (list) and `help <verb-or-prefix>` (long-form for one,
// or list of matches for an ambiguous prefix).
func NewHelp(r *telnet.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "help",
		Aliases: []string{"?"},
		Help:    "Show command list or detail for one command",
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				return writeList(c.Session, r.All(), "Commands:")
			}
			query := strings.ToLower(c.Args[0])
			cmd, err := r.Lookup(query)
			if err == nil {
				return writeDetail(c.Session, cmd)
			}
			matches := r.Prefix(query)
			if len(matches) == 0 {
				return c.Session.WriteRaw([]byte("No such command: " + query + "\r\n"))
			}
			return writeList(c.Session, matches, "Commands matching "+query+":")
		},
		// Tab in `help <prefix>` lists every verb the session can see —
		// we filter by AuthLevel so a guest can't enumerate privileged
		// commands through completion.
		Completer: func(s *telnet.Session, args string) []telnet.Candidate {
			partial, _ := telnet.CompletionPartial(args)
			matches := r.Prefix(strings.ToLower(partial))
			out := make([]telnet.Candidate, 0, len(matches))
			for _, c := range matches {
				if s.AuthLevel < c.Auth {
					continue
				}
				out = append(out, telnet.Candidate{Text: c.Name, Help: c.Help})
			}
			return out
		},
	}
}

func writeList(s *telnet.Session, cmds []*telnet.Command, title string) error {
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\r\n")
	for _, c := range cmds {
		b.WriteString("  ")
		b.WriteString(c.Name)
		if c.Help != "" {
			b.WriteString("  -  ")
			b.WriteString(c.Help)
		}
		b.WriteString("\r\n")
	}
	return s.WriteRaw([]byte(b.String()))
}

func writeDetail(s *telnet.Session, c *telnet.Command) error {
	var b strings.Builder
	b.WriteString(c.Name)
	if len(c.Aliases) > 0 {
		b.WriteString(" (aliases: ")
		b.WriteString(strings.Join(c.Aliases, ", "))
		b.WriteString(")")
	}
	b.WriteString("\r\n")
	if c.Help != "" {
		b.WriteString(c.Help)
		b.WriteString("\r\n")
	}
	if c.Long != "" {
		b.WriteString("\r\n")
		b.WriteString(c.Long)
		b.WriteString("\r\n")
	}
	return s.WriteRaw([]byte(b.String()))
}
