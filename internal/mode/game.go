package mode

import (
	"context"
	"strings"

	"github.com/Jasrags/WheelMUD/telnet"
)

// Game is the in-world command-dispatch mode. It wraps a Registry and
// delegates Handle to Registry.Dispatch.
type Game struct {
	Registry *telnet.Registry
}

func NewGame(r *telnet.Registry) *Game { return &Game{Registry: r} }

func (g *Game) Handle(ctx context.Context, s *telnet.Session, line string) error {
	return g.Registry.Dispatch(ctx, s, line)
}

func (g *Game) Prompt(_ *telnet.Session) string { return "> " }

func (g *Game) OnEnter(_ *telnet.Session) error { return nil }
func (g *Game) OnExit(_ *telnet.Session) error  { return nil }

// Complete satisfies telnet.Completer. With no whitespace in the buffer
// it does verb completion via Registry.Prefix; once whitespace appears
// it splits off the verb, looks up the command, and delegates to the
// command's Completer (if any). Privilege-gated commands fall through
// to nil so a tab can't be used to enumerate them — the same anti-
// enumeration policy Registry.Dispatch enforces.
func (g *Game) Complete(s *telnet.Session, buffer string) []telnet.Candidate {
	if !strings.ContainsAny(buffer, " \t") {
		cmds := g.Registry.Prefix(buffer)
		if len(cmds) == 0 {
			return nil
		}
		out := make([]telnet.Candidate, 0, len(cmds))
		for _, c := range cmds {
			if s.AuthLevel < c.Auth {
				continue
			}
			out = append(out, telnet.Candidate{Text: c.Name, Help: c.Help})
		}
		return out
	}
	verb, rest := telnet.SplitVerb(buffer)
	cmd, err := g.Registry.Lookup(verb)
	if err != nil || cmd.Completer == nil || s.AuthLevel < cmd.Auth {
		return nil
	}
	return cmd.Completer(s, rest)
}
