package mode

import (
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

func TestGameCompleteVerb(t *testing.T) {
	r := telnet.NewRegistry()
	mustReg(t, r, &telnet.Command{Name: "look", Help: "look around", Run: noopRun})
	mustReg(t, r, &telnet.Command{Name: "list", Help: "list things", Run: noopRun})
	mustReg(t, r, &telnet.Command{Name: "quit", Help: "leave", Run: noopRun})

	g := NewGame(r)
	s := &telnet.Session{AuthLevel: telnet.AuthGuest}

	cands := g.Complete(s, "l")
	names := candidateNames(cands)
	if !contains(names, "look") || !contains(names, "list") {
		t.Fatalf("expected look + list, got %v", names)
	}
	if contains(names, "quit") {
		t.Fatalf("unexpected quit in %v", names)
	}
}

func TestGameCompleteVerbAuthFiltered(t *testing.T) {
	r := telnet.NewRegistry()
	mustReg(t, r, &telnet.Command{Name: "look", Help: "look", Run: noopRun})
	mustReg(t, r, &telnet.Command{Name: "loot", Help: "loot", Auth: telnet.AuthAdmin, Run: noopRun})

	g := NewGame(r)
	guest := &telnet.Session{AuthLevel: telnet.AuthGuest}

	names := candidateNames(g.Complete(guest, "lo"))
	if !contains(names, "look") || contains(names, "loot") {
		t.Fatalf("guest should see look but not loot: %v", names)
	}

	admin := &telnet.Session{AuthLevel: telnet.AuthAdmin}
	names = candidateNames(g.Complete(admin, "lo"))
	if !contains(names, "loot") {
		t.Fatalf("admin should see loot: %v", names)
	}
}

func TestGameCompleteArgsDelegates(t *testing.T) {
	r := telnet.NewRegistry()
	mustReg(t, r, &telnet.Command{
		Name: "say",
		Run:  noopRun,
		Completer: func(_ *telnet.Session, args string) []telnet.Candidate {
			partial, _ := telnet.CompletionPartial(args)
			return []telnet.Candidate{{Text: "echo:" + partial}}
		},
	})

	g := NewGame(r)
	s := &telnet.Session{AuthLevel: telnet.AuthGuest}

	cands := g.Complete(s, "say hel")
	if len(cands) != 1 || cands[0].Text != "echo:hel" {
		t.Fatalf("expected echo:hel, got %v", cands)
	}
}

func TestGameCompleteArgsNoCompleter(t *testing.T) {
	r := telnet.NewRegistry()
	mustReg(t, r, &telnet.Command{Name: "quit", Run: noopRun})
	g := NewGame(r)
	s := &telnet.Session{AuthLevel: telnet.AuthGuest}

	if cands := g.Complete(s, "quit "); cands != nil {
		t.Fatalf("expected nil for command without completer, got %v", cands)
	}
}

func TestGameCompleteArgsAuthGated(t *testing.T) {
	r := telnet.NewRegistry()
	mustReg(t, r, &telnet.Command{
		Name: "secret",
		Auth: telnet.AuthAdmin,
		Run:  noopRun,
		Completer: func(_ *telnet.Session, _ string) []telnet.Candidate {
			return []telnet.Candidate{{Text: "leaked"}}
		},
	})

	g := NewGame(r)
	guest := &telnet.Session{AuthLevel: telnet.AuthGuest}

	if cands := g.Complete(guest, "secret f"); cands != nil {
		t.Fatalf("guest should not get arg completions for privileged command: %v", cands)
	}
}

func mustReg(t *testing.T, r *telnet.Registry, c *telnet.Command) {
	t.Helper()
	if err := r.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func noopRun(*telnet.Context) error { return nil }

func candidateNames(cs []telnet.Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Text
	}
	return out
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
