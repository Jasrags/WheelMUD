package cmd

import (
	"strings"

	"github.com/Jasrags/WheelMUD/internal/help"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewHelp builds a help command bound to the given registry and the
// help-topic catalog. Resolution order for `help <q>` mirrors
// telnet.Registry.Lookup but spans both spaces:
//
//  1. Exact: command name/alias, then topic id.
//  2. Keyword: topic keyword (commands have no keywords).
//  3. Unique prefix across cmd Names + topic IDs combined.
//  4. Otherwise: ambiguity list (grouped by space) or "no such".
//
// Topic catalog may be nil — useful for tests that only exercise the
// command path; in that case behaviour collapses to the
// commands-only case.
func NewHelp(r *telnet.Registry, hc *help.Catalog) *telnet.Command {
	return &telnet.Command{
		Name:    "help",
		Aliases: []string{"?"},
		Help:    "Show command list, or detail for one command or topic",
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				return writeIndex(c.Session, r.All(), topicsAll(hc))
			}
			query := strings.ToLower(c.Args[0])

			// 1. Exact: command/alias first, then topic id. The
			// AuthLevel gate matches the dispatcher's
			// privilege-hiding policy so `help <admin-verb>`
			// does not disclose a privileged command's
			// existence to a guest.
			if cmd, err := r.LookupExact(query); err == nil && c.Session.AuthLevel >= cmd.Auth {
				return writeDetail(c.Session, cmd)
			}
			if t, ok := topicLookupExact(hc, query); ok {
				return writeTopic(c.Session, t)
			}

			// 2. Keyword (topics only).
			if t, ok := topicLookupKeyword(hc, query); ok {
				return writeTopic(c.Session, t)
			}

			// 3. Unique-prefix across both spaces. Filter the cmd
			// side by the session's AuthLevel so a guest cannot
			// enumerate privileged command names through `help
			// <prefix>` — the same enumeration guard the
			// dispatcher applies via "Unknown command" on a
			// privilege-denied lookup.
			cmdMatches := visibleCmds(r.Prefix(query), c.Session.AuthLevel)
			topicMatches := topicPrefix(hc, query)
			total := len(cmdMatches) + len(topicMatches)
			switch total {
			case 0:
				return c.Session.WriteRaw([]byte("No such command or topic: " + query + "\r\n"))
			case 1:
				if len(cmdMatches) == 1 {
					return writeDetail(c.Session, cmdMatches[0])
				}
				return writeTopic(c.Session, topicMatches[0])
			default:
				return writeAmbiguous(c.Session, query, cmdMatches, topicMatches)
			}
		},
		// Tab in `help <prefix>` lists every verb the session can see
		// plus every topic. Commands filter by AuthLevel (so a guest
		// can't enumerate privileged commands); topics have no auth
		// gate.
		Completer: func(s *telnet.Session, args string) []telnet.Candidate {
			partial, _ := telnet.CompletionPartial(args)
			lower := strings.ToLower(partial)
			cmds := r.Prefix(lower)
			out := make([]telnet.Candidate, 0, len(cmds))
			for _, c := range cmds {
				if s.AuthLevel < c.Auth {
					continue
				}
				out = append(out, telnet.Candidate{Text: c.Name, Help: c.Help})
			}
			for _, t := range topicPrefix(hc, lower) {
				out = append(out, telnet.Candidate{Text: t.ID, Help: t.Title})
			}
			return out
		},
	}
}

// visibleCmds drops commands the session lacks privilege to invoke.
// Used to keep the merged help-prefix path from leaking privileged
// command names through ambiguity listings or single-prefix-resolves.
func visibleCmds(cmds []*telnet.Command, auth telnet.AuthLevel) []*telnet.Command {
	out := cmds[:0:0]
	for _, c := range cmds {
		if auth < c.Auth {
			continue
		}
		out = append(out, c)
	}
	return out
}

// topicsAll, topicLookupExact, topicLookupKeyword, topicPrefix wrap
// the catalog so a nil hc collapses to "no topics" without scattering
// nil-checks through Run.

func topicsAll(hc *help.Catalog) []*help.Topic {
	if hc == nil {
		return nil
	}
	return hc.All()
}

func topicLookupExact(hc *help.Catalog, q string) (*help.Topic, bool) {
	if hc == nil {
		return nil, false
	}
	return hc.LookupExact(q)
}

func topicLookupKeyword(hc *help.Catalog, q string) (*help.Topic, bool) {
	if hc == nil {
		return nil, false
	}
	return hc.LookupKeyword(q)
}

func topicPrefix(hc *help.Catalog, p string) []*help.Topic {
	if hc == nil {
		return nil
	}
	return hc.Prefix(p)
}

// writeIndex renders the no-args help: command list, then a topic
// list when the catalog is non-empty.
func writeIndex(s *telnet.Session, cmds []*telnet.Command, topics []*help.Topic) error {
	var b strings.Builder
	b.WriteString("Commands:\r\n")
	for _, c := range cmds {
		b.WriteString("  ")
		b.WriteString(c.Name)
		if c.Help != "" {
			b.WriteString("  -  ")
			b.WriteString(c.Help)
		}
		b.WriteString("\r\n")
	}
	if len(topics) > 0 {
		b.WriteString("\r\nTopics:\r\n")
		for _, t := range topics {
			b.WriteString("  ")
			b.WriteString(t.ID)
			if t.Title != "" {
				b.WriteString("  -  ")
				b.WriteString(t.Title)
			}
			b.WriteString("\r\n")
		}
	}
	return s.WritePaged([]byte(b.String()))
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
		// Long is authored with bare "\n" — telnet needs CR+LF.
		body := strings.ReplaceAll(c.Long, "\r\n", "\n")
		b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
		b.WriteString("\r\n")
	}
	return s.WritePaged([]byte(b.String()))
}

func writeTopic(s *telnet.Session, t *help.Topic) error {
	var b strings.Builder
	b.WriteString(t.Title)
	b.WriteString("\r\n\r\n")
	body := strings.ReplaceAll(t.Body, "\r\n", "\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	if !strings.HasSuffix(b.String(), "\r\n") {
		b.WriteString("\r\n")
	}
	return s.WritePaged([]byte(b.String()))
}

func writeAmbiguous(s *telnet.Session, query string, cmds []*telnet.Command, topics []*help.Topic) error {
	var b strings.Builder
	if len(cmds) > 0 {
		b.WriteString("Commands matching ")
		b.WriteString(query)
		b.WriteString(":\r\n")
		for _, c := range cmds {
			b.WriteString("  ")
			b.WriteString(c.Name)
			if c.Help != "" {
				b.WriteString("  -  ")
				b.WriteString(c.Help)
			}
			b.WriteString("\r\n")
		}
	}
	if len(topics) > 0 {
		if len(cmds) > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString("Topics matching ")
		b.WriteString(query)
		b.WriteString(":\r\n")
		for _, t := range topics {
			b.WriteString("  ")
			b.WriteString(t.ID)
			if t.Title != "" {
				b.WriteString("  -  ")
				b.WriteString(t.Title)
			}
			b.WriteString("\r\n")
		}
	}
	return s.WritePaged([]byte(b.String()))
}
