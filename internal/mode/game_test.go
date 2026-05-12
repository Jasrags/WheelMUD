package mode

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

func TestGamePromptRendersHP(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	c, err := chars.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Rand",
		Core:      creature.Core{HPCurrent: 7, HPMax: 10},
		Coin:      currency.MustNew(0, 0, 0, 0),
	})
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}

	g := NewGame(telnet.NewRegistry(), chars, nil, "<%h/%H hp> ")
	s := &telnet.Session{CharacterID: c.ID, CharacterName: c.Name}

	if got, want := g.Prompt(context.Background(), s), "<7/10 hp> "; got != want {
		t.Fatalf("Prompt = %q, want %q", got, want)
	}
}

func TestGamePromptFallbacks(t *testing.T) {
	cases := []struct {
		name string
		g    *Game
		s    *telnet.Session
	}{
		{
			name: "empty_template",
			g:    NewGame(telnet.NewRegistry(), nil, nil, ""),
			s:    &telnet.Session{CharacterName: "Rand"},
		},
		{
			name: "no_character_name",
			g:    NewGame(telnet.NewRegistry(), repo.NewMemoryCharacterRepo(), nil, "<%h/%H>"),
			s:    &telnet.Session{},
		},
		{
			name: "lookup_miss",
			g:    NewGame(telnet.NewRegistry(), repo.NewMemoryCharacterRepo(), nil, "<%h>"),
			s:    &telnet.Session{CharacterName: "Ghost"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.g.Prompt(context.Background(), tc.s); got != "> " {
				t.Fatalf("Prompt = %q, want fallback %q", got, "> ")
			}
		})
	}
}

func TestGamePromptRendersGold(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	c, err := chars.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Thom",
		Core:      creature.Core{HPCurrent: 4, HPMax: 4},
		Coin:      currency.MustNew(5, 0, 2, 0),
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := NewGame(telnet.NewRegistry(), chars, nil, "%h/%H %g")
	s := &telnet.Session{CharacterID: c.ID, CharacterName: c.Name}

	got := g.Prompt(context.Background(), s)
	want := c.Coin.Short()
	if !strings.Contains(got, want) {
		t.Fatalf("expected coin %q in prompt; got %q", want, got)
	}
}

func TestGamePromptCfmtColor(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	c, err := chars.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Lan",
		Core:      creature.Core{HPCurrent: 7, HPMax: 10},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := NewGame(telnet.NewRegistry(), chars, nil, "{{%h}}::red/%H ")
	s := &telnet.Session{CharacterID: c.ID, CharacterName: c.Name}

	got := g.Prompt(context.Background(), s)
	// cfmt emits a real ANSI escape for the colorized run.
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected ANSI escape from cfmt; got %q", got)
	}
	// HP value still interpolates inside the colored span.
	if !strings.Contains(got, "7") || !strings.Contains(got, "/10") {
		t.Fatalf("HP missing from rendered prompt: %q", got)
	}
}

func TestGamePromptDefangsRoomName(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	rooms := repo.NewMemoryRoomRepo()
	// Malicious room name: a `}}::red` would close any cfmt span the
	// player opened in their template and recolor the rest. Defang
	// must neutralize it before cfmt rendering.
	rooms.Insert(repo.Room{ID: 9, ExternalID: "trap", Name: "Trap}}::red"})
	c, _ := chars.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Moiraine",
		Core:      creature.Core{HPCurrent: 5, HPMax: 5},
	})

	g := NewGame(telnet.NewRegistry(), chars, rooms, "%r %h/%H")
	s := &telnet.Session{
		CharacterID:   c.ID,
		CharacterName: c.Name,
		CurrentRoomID: 9,
	}

	got := g.Prompt(context.Background(), s)
	// `}}` must have been broken; verify the cfmt close sequence is gone.
	if strings.Contains(got, "}}::red") {
		t.Fatalf("undefanged cfmt syntax leaked from room name: %q", got)
	}
	// HP still renders.
	if !strings.Contains(got, "5/5") {
		t.Fatalf("HP missing: %q", got)
	}
}

func TestGamePromptCharacterTemplateOverride(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	c, _ := chars.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Perrin",
		Core:      creature.Core{HPCurrent: 8, HPMax: 12},
	})
	if err := chars.RecordPromptTemplate(context.Background(), c.ID, "(%h)"); err != nil {
		t.Fatalf("RecordPromptTemplate: %v", err)
	}

	// Server default would render <%h/%H hp> — the override should win.
	g := NewGame(telnet.NewRegistry(), chars, nil, "<%h/%H hp> ")
	s := &telnet.Session{CharacterID: c.ID, CharacterName: c.Name}

	if got, want := g.Prompt(context.Background(), s), "(8)"; got != want {
		t.Fatalf("Prompt = %q, want %q (per-character override)", got, want)
	}
}

func TestGamePromptRoomLookup(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 42, ExternalID: "glade", Name: "Vast Glade"})
	c, err := chars.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Egwene",
		Core:      creature.Core{HPCurrent: 5, HPMax: 5},
	})
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}

	g := NewGame(telnet.NewRegistry(), chars, rooms, "[%r] %h/%H> ")
	s := &telnet.Session{
		CharacterID:   c.ID,
		CharacterName: c.Name,
		CurrentRoomID: 42,
	}
	if got, want := g.Prompt(context.Background(), s), "[Vast Glade] 5/5> "; got != want {
		t.Fatalf("Prompt = %q, want %q", got, want)
	}
}

func TestGameCompleteVerb(t *testing.T) {
	r := telnet.NewRegistry()
	mustReg(t, r, &telnet.Command{Name: "look", Help: "look around", Run: noopRun})
	mustReg(t, r, &telnet.Command{Name: "list", Help: "list things", Run: noopRun})
	mustReg(t, r, &telnet.Command{Name: "quit", Help: "leave", Run: noopRun})

	g := NewGame(r, nil, nil, "")
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

	g := NewGame(r, nil, nil, "")
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

	g := NewGame(r, nil, nil, "")
	s := &telnet.Session{AuthLevel: telnet.AuthGuest}

	cands := g.Complete(s, "say hel")
	if len(cands) != 1 || cands[0].Text != "echo:hel" {
		t.Fatalf("expected echo:hel, got %v", cands)
	}
}

func TestGameCompleteArgsNoCompleter(t *testing.T) {
	r := telnet.NewRegistry()
	mustReg(t, r, &telnet.Command{Name: "quit", Run: noopRun})
	g := NewGame(r, nil, nil, "")
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

	g := NewGame(r, nil, nil, "")
	guest := &telnet.Session{AuthLevel: telnet.AuthGuest}

	if cands := g.Complete(guest, "secret f"); cands != nil {
		t.Fatalf("guest should not get arg completions for privileged command: %v", cands)
	}
}

func TestGameAuditHookFiresOnceAfterDispatch(t *testing.T) {
	r := telnet.NewRegistry()
	mustReg(t, r, &telnet.Command{Name: "look", Run: noopRun})

	g := NewGame(r, nil, nil, "")
	var (
		called   int
		lastLine string
		lastErr  error
	)
	g.SetAudit(func(_ context.Context, _ *telnet.Session, line string, err error) {
		called++
		lastLine = line
		lastErr = err
	})

	s := &telnet.Session{AuthLevel: telnet.AuthGuest, CharacterID: 7, CharacterName: "Moiraine"}
	if err := g.Handle(context.Background(), s, "look around"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if called != 1 {
		t.Fatalf("audit calls = %d, want 1", called)
	}
	if lastLine != "look around" {
		t.Fatalf("audit line = %q, want %q", lastLine, "look around")
	}
	if lastErr != nil {
		t.Fatalf("audit err = %v, want nil", lastErr)
	}
}

func TestGameAuditHookFiresOnUnknownCommand(t *testing.T) {
	// Audit must capture refusals too — the forensic record needs to
	// show attempted commands, not just successful ones. Use a real
	// piped session because the unknown-command path writes a refusal
	// line back to the client.
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := cli.Read(buf); err != nil {
				return
			}
		}
	}()
	s := telnet.NewSession(srv)
	s.AuthLevel = telnet.AuthGuest
	s.CharacterID = 1

	r := telnet.NewRegistry()
	g := NewGame(r, nil, nil, "")
	var called int
	g.SetAudit(func(_ context.Context, _ *telnet.Session, _ string, _ error) {
		called++
	})
	_ = g.Handle(context.Background(), s, "blargle")
	if called != 1 {
		t.Fatalf("audit calls = %d, want 1 (refusals must audit too)", called)
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
