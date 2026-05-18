package telnet

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
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
func withLong(l string) func(*Command)       { return func(c *Command) { c.Long = l } }

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

func TestRegistry_LookupExact(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(
		cmd("look", noopRun, withAliases("l")),
		cmd("loot", noopRun),
		cmd("help", noopRun),
	); err != nil {
		t.Fatalf("register: %v", err)
	}

	tests := []struct {
		verb    string
		want    string
		wantErr error
	}{
		{verb: "look", want: "look"},
		{verb: "LOOK", want: "look"},             // case-insensitive
		{verb: "l", want: "look"},                // alias
		{verb: "lo", wantErr: ErrUnknownCommand}, // no prefix fallback
		{verb: "he", wantErr: ErrUnknownCommand}, // even when prefix is unique
		{verb: "zzz", wantErr: ErrUnknownCommand},
		{verb: "", wantErr: ErrUnknownCommand},
	}
	for _, tc := range tests {
		t.Run(tc.verb, func(t *testing.T) {
			got, err := r.LookupExact(tc.verb)
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
			go func() { done <- r.Dispatch(context.Background(), s, tc.line) }()

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

func TestRegistry_Dispatch_MinArgsPrefersLong(t *testing.T) {
	// When a verb supplies a Long body, MinArgs failure prints Long
	// verbatim instead of "Usage: <Help>" — Help is a one-line
	// description and prefixing it with "Usage:" misleads the user.
	r := NewRegistry()
	_ = r.Register(cmd("spawn", func(c *Context) error { return nil },
		withMinArgs(2),
		withHelp("Spawn a mob or item from a template"),
		withLong("Usage: spawn mob <ext> [count]\n       spawn item <ext> [count]")))
	s, peer := newPipeSession(t)
	outCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 512)
		_ = peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, _ := peer.Read(buf)
		outCh <- string(buf[:n])
	}()
	go func() { _ = r.Dispatch(context.Background(), s, "spawn") }()
	select {
	case out := <-outCh:
		if !strings.Contains(out, "spawn mob <ext>") {
			t.Fatalf("expected Long body in output, got %q", out)
		}
		if strings.Contains(out, "Usage: Spawn a mob or item from a template") {
			t.Fatalf("MinArgs fallback used Help when Long was set: %q", out)
		}
	case <-time.After(time.Second):
		t.Fatal("no output")
	}
}

func TestRegistry_Dispatch_AuthEnforcement(t *testing.T) {
	r := NewRegistry()
	called := false
	admin := &Command{
		Name: "shutdown",
		Auth: AuthAdmin,
		Run:  func(_ *Context) error { called = true; return nil },
	}
	if err := r.Register(admin); err != nil {
		t.Fatalf("register: %v", err)
	}

	t.Run("guest sees Unknown command (does not leak existence)", func(t *testing.T) {
		s, peer := newPipeSession(t)
		s.AuthLevel = AuthGuest

		outCh := make(chan string, 1)
		go func() {
			buf := make([]byte, 256)
			_ = peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, _ := peer.Read(buf)
			outCh <- string(buf[:n])
		}()
		if err := r.Dispatch(context.Background(), s, "shutdown"); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		if got := <-outCh; !strings.Contains(got, "Unknown command") {
			t.Fatalf("guest output = %q, want 'Unknown command'", got)
		}
		if called {
			t.Fatal("Run was invoked despite guest auth level")
		}
	})

	t.Run("admin runs the command", func(t *testing.T) {
		called = false
		s, peer := newPipeSession(t)
		s.AuthLevel = AuthAdmin

		// Drain.
		go func() {
			buf := make([]byte, 256)
			_ = peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			_, _ = peer.Read(buf)
		}()
		if err := r.Dispatch(context.Background(), s, "shutdown"); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		if !called {
			t.Fatal("admin Run was not invoked")
		}
	})
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
	if err := r.Dispatch(context.Background(), s, "lo"); err != nil {
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

func TestRegistry_Dispatch_SemicolonChain(t *testing.T) {
	type call struct {
		name string
		raw  string
	}
	var got []call
	r := NewRegistry()
	record := func(c *Context) error {
		got = append(got, call{name: c.Name, raw: c.Raw})
		return nil
	}
	_ = r.Register(
		cmd("look", record),
		cmd("north", record, withAliases("n")),
		cmd("say", record, withMinArgs(1), withHelp("say <msg>")),
	)
	s, peer := newPipeSession(t)

	var wg sync.WaitGroup
	_ = drainPeer(peer, &wg)
	defer wg.Wait()

	if err := r.Dispatch(context.Background(), s, "look ; n ; say hi there"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	_ = peer.Close()

	want := []call{
		{name: "look", raw: ""},
		{name: "north", raw: ""},
		{name: "say", raw: "hi there"},
	}
	if len(got) != len(want) {
		t.Fatalf("calls = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestRegistry_Dispatch_SemicolonInQuotedArg(t *testing.T) {
	var capturedRaw string
	r := NewRegistry()
	_ = r.Register(cmd("say", func(c *Context) error {
		capturedRaw = c.Raw
		return nil
	}, withMinArgs(1), withHelp("say <msg>")))
	s, peer := newPipeSession(t)

	var wg sync.WaitGroup
	_ = drainPeer(peer, &wg)
	defer wg.Wait()

	if err := r.Dispatch(context.Background(), s, `say "hello; world"`); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	_ = peer.Close()

	if capturedRaw != `"hello; world"` {
		t.Fatalf("Raw = %q, want %q", capturedRaw, `"hello; world"`)
	}
}

func TestRegistry_Dispatch_SemicolonContinuesAfterUnknownVerb(t *testing.T) {
	var ranSecond bool
	r := NewRegistry()
	_ = r.Register(cmd("look", func(_ *Context) error { ranSecond = true; return nil }))
	s, peer := newPipeSession(t)

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := r.Dispatch(context.Background(), s, "fnord ; look"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	_ = peer.Close()
	wg.Wait()

	if !strings.Contains(out.String(), "Unknown command") {
		t.Fatalf("expected 'Unknown command' in output, got %q", out.String())
	}
	if !ranSecond {
		t.Fatal("second segment did not run after unknown-verb segment")
	}
}

func TestRegistry_Dispatch_SemicolonRunErrorPropagatedFirst(t *testing.T) {
	wantErr := errors.New("boom")
	var ranSecond bool
	r := NewRegistry()
	_ = r.Register(
		cmd("first", func(_ *Context) error { return wantErr }),
		cmd("second", func(_ *Context) error { ranSecond = true; return nil }),
	)
	s, peer := newPipeSession(t)

	var wg sync.WaitGroup
	_ = drainPeer(peer, &wg)
	defer wg.Wait()

	err := r.Dispatch(context.Background(), s, "first ; second")
	_ = peer.Close()

	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if !ranSecond {
		t.Fatal("second segment must run even after first returned an error")
	}
}

func TestRegistry_Dispatch_SemicolonTruncated(t *testing.T) {
	var count int
	r := NewRegistry()
	_ = r.Register(cmd("ping", func(_ *Context) error { count++; return nil }))
	s, peer := newPipeSession(t)

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	parts := make([]string, 20)
	for i := range parts {
		parts[i] = "ping"
	}
	line := strings.Join(parts, " ; ")
	if err := r.Dispatch(context.Background(), s, line); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	_ = peer.Close()
	wg.Wait()

	if count != maxSegmentsPerLine {
		t.Fatalf("ran %d times, want %d", count, maxSegmentsPerLine)
	}
	if !strings.Contains(out.String(), "too many commands") {
		t.Fatalf("expected truncation notice in %q", out.String())
	}
}

func TestRegistry_Dispatch_AliasExpandsToChain(t *testing.T) {
	type call struct{ name string }
	var got []call
	r := NewRegistry()
	record := func(c *Context) error { got = append(got, call{c.Name}); return nil }
	_ = r.Register(cmd("look", record), cmd("smile", record))

	s, peer := newPipeSession(t)
	s.Aliases = NewAliasTable()
	s.Aliases.Set("m", "look; smile")

	var wg sync.WaitGroup
	_ = drainPeer(peer, &wg)
	defer wg.Wait()

	if err := r.Dispatch(context.Background(), s, "m"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	_ = peer.Close()

	if len(got) != 2 || got[0].name != "look" || got[1].name != "smile" {
		t.Fatalf("calls = %+v, want [look smile]", got)
	}
}

func names(cmds []*Command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Name
	}
	return out
}

func withLag(d time.Duration) func(*Command) { return func(c *Command) { c.Lag = d } }

// TestRegistry_Dispatch_LagStampedOnSuccess: a successful Run on a
// verb with Lag>0 leaves the session lagged on the next dispatch.
func TestRegistry_Dispatch_LagStampedOnSuccess(t *testing.T) {
	var ran int
	r := NewRegistry()
	_ = r.Register(cmd("attack", func(_ *Context) error { ran++; return nil }, withLag(50*time.Millisecond)))
	s, peer := newPipeSession(t)
	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := r.Dispatch(context.Background(), s, "attack"); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if err := r.Dispatch(context.Background(), s, "attack"); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}
	_ = peer.Close()
	wg.Wait()

	if ran != 1 {
		t.Fatalf("Run executions = %d, want 1 (second should be gated)", ran)
	}
	if !strings.Contains(out.String(), "too busy") {
		t.Fatalf("output missing too-busy refusal: %q", out.String())
	}
}

// TestRegistry_Dispatch_LagOnSuccessOnly: a Run that returns an error
// must NOT stamp lag — failure path stays unrestricted.
func TestRegistry_Dispatch_LagOnSuccessOnly(t *testing.T) {
	var ran int
	r := NewRegistry()
	_ = r.Register(cmd("attack", func(_ *Context) error {
		ran++
		return errors.New("bad target")
	}, withLag(50*time.Millisecond)))
	s, peer := newPipeSession(t)
	var wg sync.WaitGroup
	_ = drainPeer(peer, &wg)
	defer wg.Wait()

	_ = r.Dispatch(context.Background(), s, "attack")
	_ = r.Dispatch(context.Background(), s, "attack")
	_ = peer.Close()
	wg.Wait()

	if ran != 2 {
		t.Fatalf("Run executions = %d, want 2 (failed Run must not lag)", ran)
	}
}

// TestRegistry_Dispatch_ZeroLagTransparent: an unstamped session
// runs verbs with Lag==0 back-to-back without any gating.
func TestRegistry_Dispatch_ZeroLagTransparent(t *testing.T) {
	var ran int
	r := NewRegistry()
	_ = r.Register(cmd("look", func(_ *Context) error { ran++; return nil }))
	s, peer := newPipeSession(t)
	var wg sync.WaitGroup
	_ = drainPeer(peer, &wg)
	defer wg.Wait()

	for i := 0; i < 5; i++ {
		_ = r.Dispatch(context.Background(), s, "look")
	}
	_ = peer.Close()
	wg.Wait()

	if ran != 5 {
		t.Fatalf("Run executions = %d, want 5", ran)
	}
}

// TestRegistry_Dispatch_LagPerSegment: chained input runs the
// unlagged head, stamps lag, then refuses the lagged tail without
// aborting the chain.
func TestRegistry_Dispatch_LagPerSegment(t *testing.T) {
	var calls []string
	r := NewRegistry()
	_ = r.Register(
		cmd("look", func(c *Context) error { calls = append(calls, c.Name); return nil }),
		cmd("attack", func(c *Context) error { calls = append(calls, c.Name); return nil }, withLag(50*time.Millisecond)),
	)
	s, peer := newPipeSession(t)
	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := r.Dispatch(context.Background(), s, "look; attack; attack"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	_ = peer.Close()
	wg.Wait()

	want := []string{"look", "attack"}
	if !equalStrings(calls, want) {
		t.Fatalf("calls = %v, want %v (third segment must be gated)", calls, want)
	}
	if !strings.Contains(out.String(), "too busy") {
		t.Fatalf("output missing too-busy refusal: %q", out.String())
	}
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

func TestRegistry_Unregister(t *testing.T) {
	t.Run("happy path drops name and aliases", func(t *testing.T) {
		r := NewRegistry()
		if err := r.Register(cmd("look", noopRun, withAliases("l", "lk"))); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if err := r.Unregister("look"); err != nil {
			t.Fatalf("Unregister: %v", err)
		}
		if _, err := r.Lookup("look"); !errors.Is(err, ErrUnknownCommand) {
			t.Fatalf("Lookup after Unregister: got %v want ErrUnknownCommand", err)
		}
		// Aliases must be wiped too.
		if _, err := r.Lookup("l"); !errors.Is(err, ErrUnknownCommand) {
			t.Fatalf("alias 'l' should be gone, got %v", err)
		}
		if _, err := r.Lookup("lk"); !errors.Is(err, ErrUnknownCommand) {
			t.Fatalf("alias 'lk' should be gone, got %v", err)
		}
	})

	t.Run("unknown name returns ErrUnknownCommand", func(t *testing.T) {
		r := NewRegistry()
		if err := r.Unregister("nope"); !errors.Is(err, ErrUnknownCommand) {
			t.Fatalf("got %v want ErrUnknownCommand", err)
		}
	})

	t.Run("empty name returns ErrUnknownCommand", func(t *testing.T) {
		r := NewRegistry()
		if err := r.Unregister(""); !errors.Is(err, ErrUnknownCommand) {
			t.Fatalf("got %v want ErrUnknownCommand", err)
		}
	})

	t.Run("case-insensitive lookup", func(t *testing.T) {
		r := NewRegistry()
		_ = r.Register(cmd("smile", noopRun))
		if err := r.Unregister("SMILE"); err != nil {
			t.Fatalf("uppercase Unregister: %v", err)
		}
	})

	t.Run("allows re-Register of the same name", func(t *testing.T) {
		r := NewRegistry()
		_ = r.Register(cmd("smile", noopRun, withAliases("sm")))
		if err := r.Unregister("smile"); err != nil {
			t.Fatalf("Unregister: %v", err)
		}
		// Both the name and the freed alias must be available again.
		if err := r.Register(cmd("smile", noopRun, withAliases("sm"))); err != nil {
			t.Fatalf("re-Register: %v", err)
		}
	})

	t.Run("concurrent Unregister vs Dispatch is race-free", func(t *testing.T) {
		// Pin the locking story documented on Unregister: the
		// dispatcher releases mu after lookup and holds the *Command
		// pointer until Run returns. An in-flight dispatch must not
		// race with the concurrent Unregister.
		r := NewRegistry()
		const verb = "spin"
		started := make(chan struct{})
		release := make(chan struct{})
		_ = r.Register(cmd(verb, func(ctx *Context) error {
			close(started)
			<-release
			return nil
		}))
		s, peer := newPipeSession(t)
		s.AuthLevel = AuthGuest
		// Drain the peer pipe so any post-dispatch write doesn't
		// deadlock on net.Pipe back-pressure.
		go func() {
			buf := make([]byte, 256)
			for {
				if _, err := peer.Read(buf); err != nil {
					return
				}
			}
		}()
		dispatched := make(chan error, 1)
		go func() {
			dispatched <- r.Dispatch(context.Background(), s, verb)
		}()
		<-started
		// Unregister while the Run is blocked. Must not deadlock or
		// race; the in-flight Run completes with the old *Command.
		if err := r.Unregister(verb); err != nil {
			t.Fatalf("Unregister mid-dispatch: %v", err)
		}
		close(release)
		if err := <-dispatched; err != nil {
			t.Fatalf("in-flight Dispatch returned: %v", err)
		}
		// Lookup confirms the verb is gone (Dispatch itself swallows
		// ErrUnknownCommand after writing the "Unknown command" line).
		if _, err := r.Lookup(verb); !errors.Is(err, ErrUnknownCommand) {
			t.Fatalf("post-Unregister Lookup: got %v want ErrUnknownCommand", err)
		}
	})
}
