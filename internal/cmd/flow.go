package cmd

import (
	"strings"

	"github.com/Jasrags/WheelMUD/telnet"
)

// NewFlowVerb builds the §O.1 admin launcher. With no args, it
// prints the catalog of loaded flow ids; with a single arg it pushes
// the matching flow's mode onto the session.
//
// `list` is typically `(*flow.Catalog).IDs` — kept as a closure so
// this verb has no `internal/flow` import and the boot wiring stays
// flexible (a future `flow show <id>` polish slice could swap the
// closure for a richer accessor).
func NewFlowVerb(push PushFlowFn, list func() []string) *telnet.Command {
	return &telnet.Command{
		Name:    "flow",
		Help:    "flow <id> — launch a Flow definition (admin)",
		Long:    "flow             — list available flows\nflow <id>        — launch a flow",
		Auth:    telnet.AuthAdmin,
		MinArgs: 0,
		Completer: func(_ *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			if slot != 0 {
				return nil
			}
			out := []telnet.Candidate{}
			lower := strings.ToLower(partial)
			for _, id := range list() {
				if strings.HasPrefix(id, lower) {
					out = append(out, telnet.Candidate{Text: id, Help: "flow id"})
				}
			}
			return out
		},
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				ids := list()
				if len(ids) == 0 {
					return c.Session.WriteString("{{No flows loaded.}}::yellow\r\n")
				}
				return c.Session.WriteString(
					"{{Flows: }}::cyan{{" + strings.Join(ids, ", ") + "}}::magenta\r\n")
			}
			return push(c.Session, c.Args[0])
		},
	}
}
