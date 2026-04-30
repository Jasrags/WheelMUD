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

// Complete satisfies telnet.Completer for verb completion. Argument
// completion is deferred — once the buffer contains whitespace we return
// nil, which the Tab handler renders as a bell. Registry.Prefix already
// lowercases its argument, so we pass `buffer` through verbatim.
func (g *Game) Complete(_ *telnet.Session, buffer string) []telnet.Candidate {
	if strings.ContainsAny(buffer, " \t") {
		return nil
	}
	cmds := g.Registry.Prefix(buffer)
	if len(cmds) == 0 {
		return nil
	}
	out := make([]telnet.Candidate, len(cmds))
	for i, c := range cmds {
		out[i] = telnet.Candidate{Text: c.Name, Help: c.Help}
	}
	return out
}
