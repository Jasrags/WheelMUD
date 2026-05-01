package cmd

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

// safeBuf is a mutex-guarded byte sink used by the pipe drainer. The
// underlying strings.Builder is not safe for concurrent Read/Write,
// which trips the race detector under -race.
type safeBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// pipeSession returns a session whose writes can be inspected via the
// returned drain. Mirrors the pattern in telnet/command_test.go.
func pipeSession(t *testing.T) (*telnet.Session, *safeBuf, func()) {
	t.Helper()
	srv, peer := net.Pipe()
	s := telnet.NewSession(srv)
	out := &safeBuf{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 256)
		for {
			n, err := peer.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	cleanup := func() {
		_ = srv.Close()
		_ = peer.Close()
		wg.Wait()
	}
	return s, out, cleanup
}

func TestAliasSetAndList(t *testing.T) {
	s, out, cleanup := pipeSession(t)
	defer cleanup()

	r := telnet.NewRegistry()
	if err := r.Register(NewAlias(), NewUnalias()); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := r.Dispatch(context.Background(), s, "alias ll look"); err != nil {
		t.Fatalf("dispatch set: %v", err)
	}
	if v, ok := s.Aliases.Lookup("ll"); !ok || v != "look" {
		t.Fatalf("alias not stored: (%q,%v)", v, ok)
	}

	if err := r.Dispatch(context.Background(), s, "alias"); err != nil {
		t.Fatalf("dispatch list: %v", err)
	}
	if !strings.Contains(out.String(), "ll = look") {
		t.Fatalf("listing missing entry: %q", out.String())
	}
}

func TestAliasEqualsForm(t *testing.T) {
	s, _, cleanup := pipeSession(t)
	defer cleanup()
	r := telnet.NewRegistry()
	if err := r.Register(NewAlias()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Dispatch(context.Background(), s, "alias gn=move north"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if v, _ := s.Aliases.Lookup("gn"); v != "move north" {
		t.Fatalf("expansion = %q, want 'move north'", v)
	}
}

func TestUnalias(t *testing.T) {
	s, _, cleanup := pipeSession(t)
	defer cleanup()
	r := telnet.NewRegistry()
	if err := r.Register(NewAlias(), NewUnalias()); err != nil {
		t.Fatalf("register: %v", err)
	}
	s.Aliases.Set("ll", "look")
	if err := r.Dispatch(context.Background(), s, "unalias ll"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if _, ok := s.Aliases.Lookup("ll"); ok {
		t.Fatal("alias should be gone")
	}
}

func TestAliasExpandsInDispatch(t *testing.T) {
	s, _, cleanup := pipeSession(t)
	defer cleanup()

	called := false
	r := telnet.NewRegistry()
	if err := r.Register(&telnet.Command{
		Name: "look",
		Run: func(*telnet.Context) error {
			called = true
			return nil
		},
	}); err != nil {
		t.Fatalf("register look: %v", err)
	}
	s.Aliases.Set("ll", "look")
	if err := r.Dispatch(context.Background(), s, "ll"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !called {
		t.Fatal("alias did not resolve to the look command")
	}
}
