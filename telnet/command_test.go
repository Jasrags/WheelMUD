package telnet

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func newPipeSession(t *testing.T) (*Session, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	return NewSession(server), client
}

func cmd(name string, run func(*Context) error, opts ...func(*Command)) *Command {
	c := &Command{Name: name, Run: run}
	for _, o := range opts {
		o(c)
	}
	return c
}

func withAliases(a ...string) func(*Command) { return func(c *Command) { c.Aliases = a } }
func withMinArgs(n int) func(*Command)       { return func(c *Command) { c.MinArgs = n } }
func withHelp(h string) func(*Command)       { return func(c *Command) { c.Help = h } }

func noopRun(_ *Context) error { return nil }

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name    string
		cmds    []*Command
		wantErr error
	}{
		{
			name: "ok",
			cmds: []*Command{cmd("look", noopRun), cmd("quit", noopRun)},
		},
		{
			name:    "duplicate name",
			cmds:    []*Command{cmd("look", noopRun), cmd("look", noopRun)},
			wantErr: ErrDuplicateCommand,
		},
		{
			name:    "duplicate alias",
			cmds:    []*Command{cmd("look", noopRun, withAliases("l")), cmd("listen", noopRun, withAliases("l"))},
			wantErr: ErrDuplicateCommand,
		},
		{
			name:    "alias collides with name",
			cmds:    []*Command{cmd("look", noopRun), cmd("listen", noopRun, withAliases("look"))},
			wantErr: ErrDuplicateCommand,
		},
		{
			name:    "uppercase name rejected",
			cmds:    []*Command{cmd("Look", noopRun)},
			wantErr: ErrInvalidCommand,
		},
		{
			name:    "name with space rejected",
			cmds:    []*Command{cmd("look here", noopRun)},
			wantErr: ErrInvalidCommand,
		},
		{
			name:    "empty name rejected",
			cmds:    []*Command{cmd("", noopRun)},
			wantErr: ErrInvalidCommand,
		},
		{
			name:    "nil run rejected",
			cmds:    []*Command{{Name: "look"}},
			wantErr: ErrInvalidCommand,
		},
		{
			name:    "non-ASCII name rejected",
			cmds:    []*Command{cmd("lookè", noopRun)},
			wantErr: ErrInvalidCommand,
		},
		{
			name:    "control byte name rejected",
			cmds:    []*Command{cmd("look\x01", noopRun)},
			wantErr: ErrInvalidCommand,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			err := r.Register(tc.cmds...)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("got error %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestRegistry_Lookup(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(
		cmd("look", noopRun, withAliases("l")),
		cmd("loot", noopRun),
		cmd("quit", noopRun, withAliases("q", "exit")),
		cmd("help", noopRun),
	); err != nil {
		t.Fatalf("register: %v", err)
	}

	tests := []struct {
		verb    string
		want    string // canonical name; empty if error expected
		wantErr error
	}{
		{verb: "look", want: "look"},
		{verb: "LOOK", want: "look"}, // case-insensitive
		{verb: "l", want: "look"},    // alias wins over prefix
		{verb: "q", want: "quit"},
		{verb: "exit", want: "quit"},
		{verb: "he", want: "help"}, // unique prefix
		{verb: "lo", wantErr: ErrAmbiguousPrefix},
		{verb: "zzz", wantErr: ErrUnknownCommand},
		{verb: "", wantErr: ErrUnknownCommand},
	}
	for _, tc := range tests {
		t.Run(tc.verb, func(t *testing.T) {
			got, err := r.Lookup(tc.verb)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got err %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Name != tc.want {
				t.Fatalf("got %q, want %q", got.Name, tc.want)
			}
		})
	}
}

func TestRegistry_Prefix(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(
		cmd("look", noopRun),
		cmd("loot", noopRun),
		cmd("listen", noopRun),
		cmd("quit", noopRun),
	)

	got := r.Prefix("lo")
	if len(got) != 2 || got[0].Name != "look" || got[1].Name != "loot" {
		t.Fatalf("prefix lo: got %v", names(got))
	}
	if all := r.Prefix(""); len(all) != 4 {
		t.Fatalf("prefix empty: want 4, got %d", len(all))
	}
	if zzz := r.Prefix("zzz"); len(zzz) != 0 {
		t.Fatalf("prefix zzz: want 0, got %d", len(zzz))
	}
}

func TestRegistry_Dispatch(t *testing.T) {
	var called *Context
	r := NewRegistry()
	_ = r.Register(
		cmd("say", func(c *Context) error { called = c; return nil },
			withMinArgs(1), withHelp("say <message>")),
		cmd("quit", func(c *Context) error { called = c; return nil }),
	)

	tests := []struct {
		name     string
		line     string
		wantOut  string // substring expected on the wire
		wantCmd  string // canonical command that should have run, "" if none
		wantArgs []string
		wantRaw  string
	}{
		{name: "exact no args", line: "quit", wantCmd: "quit"},
		{name: "tokenize args", line: "say hello world", wantCmd: "say", wantArgs: []string{"hello", "world"}, wantRaw: "hello world"},
		{name: "min args fail", line: "say", wantOut: "Usage: say <message>"},
		{name: "unknown", line: "fnord", wantOut: "Unknown command"},
		{name: "blank line", line: "   "},
		{name: "trim leading", line: "  quit  ", wantCmd: "quit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = nil
			s, peer := newPipeSession(t)

			outCh := make(chan string, 1)
			go func() {
				buf := make([]byte, 256)
				_ = peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				n, _ := peer.Read(buf)
				outCh <- string(buf[:n])
			}()

			done := make(chan error, 1)
			go func() { done <- r.Dispatch(s, tc.line) }()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("dispatch: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("dispatch hung")
			}

			var out string
			select {
			case out = <-outCh:
			case <-time.After(300 * time.Millisecond):
			}

			if tc.wantOut != "" && !strings.Contains(out, tc.wantOut) {
				t.Fatalf("output %q does not contain %q", out, tc.wantOut)
			}
			if tc.wantCmd == "" {
				if called != nil {
					t.Fatalf("command %q ran unexpectedly", called.Name)
				}
				return
			}
			if called == nil {
				t.Fatalf("expected %q to run, none did", tc.wantCmd)
			}
			if called.Name != tc.wantCmd {
				t.Fatalf("ran %q, want %q", called.Name, tc.wantCmd)
			}
			if tc.wantArgs != nil && !equalStrings(called.Args, tc.wantArgs) {
				t.Fatalf("args %v, want %v", called.Args, tc.wantArgs)
			}
			if tc.wantRaw != "" && called.Raw != tc.wantRaw {
				t.Fatalf("raw %q, want %q", called.Raw, tc.wantRaw)
			}
		})
	}
}

func TestRegistry_Dispatch_Ambiguous(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(cmd("look", noopRun), cmd("loot", noopRun))
	s, peer := newPipeSession(t)

	outCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		_ = peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, _ := peer.Read(buf)
		outCh <- string(buf[:n])
	}()
	if err := r.Dispatch(s, "lo"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	out := <-outCh
	if !strings.Contains(out, "ambiguous") && !strings.Contains(out, "Ambiguous") {
		t.Fatalf("expected ambiguous message, got %q", out)
	}
	if !strings.Contains(out, "look") || !strings.Contains(out, "loot") {
		t.Fatalf("expected candidate names in output, got %q", out)
	}
}

func names(cmds []*Command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Name
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
