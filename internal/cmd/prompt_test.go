package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

const testServerDefault = "<%h/%H hp> "

func seedPromptCharacter(t *testing.T) (repo.CharacterRepo, repo.Character) {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	c, err := chars.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Egwene",
	})
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	return chars, c
}

func runPromptCmd(t *testing.T, cmdDef *telnet.Command, s *telnet.Session, raw string) {
	t.Helper()
	args, err := telnet.Tokenize(raw)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	ctx := &telnet.Context{
		Ctx:     context.Background(),
		Session: s,
		Name:    cmdDef.Name,
		Args:    args,
		Raw:     strings.TrimSpace(raw),
	}
	if err := cmdDef.Run(ctx); err != nil {
		t.Fatalf("%s.Run: %v", cmdDef.Name, err)
	}
}

func TestPrompt_ShowDefault(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	s, out := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	runPromptCmd(t, NewPrompt(chars, testServerDefault), s, "")

	if !strings.Contains(out.String(), "(server default)") {
		t.Fatalf("expected server-default marker; got %q", out.String())
	}
	if !strings.Contains(out.String(), testServerDefault) {
		t.Fatalf("expected default template echoed; got %q", out.String())
	}
}

func TestPrompt_SetPersists(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	s, out := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	cmdDef := NewPrompt(chars, testServerDefault)
	runPromptCmd(t, cmdDef, s, "set [%h/%H] %r$ ")

	if !strings.Contains(out.String(), "Prompt updated") {
		t.Fatalf("expected confirmation; got %q", out.String())
	}
	got, _ := chars.FindByName(context.Background(), "Egwene")
	if got.PromptTemplate != "[%h/%H] %r$" {
		t.Fatalf("persisted PromptTemplate = %q, want %q", got.PromptTemplate, "[%h/%H] %r$")
	}
}

func TestPrompt_Clear(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	_ = chars.RecordPromptTemplate(context.Background(), c.ID, "[%h]")

	s, out := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	runPromptCmd(t, NewPrompt(chars, testServerDefault), s, "clear")

	if !strings.Contains(out.String(), "Reverted to server default") {
		t.Fatalf("expected revert message; got %q", out.String())
	}
	got, _ := chars.FindByName(context.Background(), "Egwene")
	if got.PromptTemplate != "" {
		t.Fatalf("PromptTemplate after clear = %q, want empty", got.PromptTemplate)
	}
}

func TestPrompt_RejectEmptySet(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	s, out := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	cmdDef := NewPrompt(chars, testServerDefault)
	ctx := &telnet.Context{
		Ctx:     context.Background(),
		Session: s,
		Name:    cmdDef.Name,
		Args:    []string{"set"},
		Raw:     "set ",
	}
	if err := cmdDef.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "Invalid template") {
		t.Fatalf("expected invalid-template rejection on empty body; got %q", out.String())
	}
}

func TestPrompt_StripsControlBytes(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	s, _ := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	cmdDef := NewPrompt(chars, testServerDefault)
	// Inject a control byte directly via Raw; tokenizer would also have
	// produced it as part of the second arg, but we want to assert the
	// sanitize pass strips it without rejecting the surrounding template.
	ctx := &telnet.Context{
		Ctx:     context.Background(),
		Session: s,
		Name:    cmdDef.Name,
		Args:    []string{"set", "\x01foo"},
		Raw:     "set \x01foo",
	}
	if err := cmdDef.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := chars.FindByName(context.Background(), "Egwene")
	if got.PromptTemplate != "foo" {
		t.Fatalf("PromptTemplate = %q, want %q (control byte stripped)", got.PromptTemplate, "foo")
	}
}

func TestPrompt_Help(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	s, out := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	runPromptCmd(t, NewPrompt(chars, testServerDefault), s, "help")

	body := out.String()
	for _, want := range []string{"Placeholders", "%h", "%r", "%g", "{{", "Example"} {
		if !strings.Contains(body, want) {
			t.Fatalf("help output missing %q; got %q", want, body)
		}
	}
}

func TestPrompt_ResetAliasOfClear(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	_ = chars.RecordPromptTemplate(context.Background(), c.ID, "[%h]")

	s, out := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	runPromptCmd(t, NewPrompt(chars, testServerDefault), s, "reset")

	if !strings.Contains(out.String(), "Reverted to server default") {
		t.Fatalf("expected revert confirmation; got %q", out.String())
	}
	got, _ := chars.FindByName(context.Background(), "Egwene")
	if got.PromptTemplate != "" {
		t.Fatalf("PromptTemplate after reset = %q, want empty", got.PromptTemplate)
	}
}

func TestPrompt_ShowAfterOverride(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	if err := chars.RecordPromptTemplate(context.Background(), c.ID, "[%h]"); err != nil {
		t.Fatalf("seed override: %v", err)
	}
	s, out := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	runPromptCmd(t, NewPrompt(chars, testServerDefault), s, "")

	if strings.Contains(out.String(), "(server default)") {
		t.Fatalf("show after override should not say server-default; got %q", out.String())
	}
	if !strings.Contains(out.String(), "[%h]") {
		t.Fatalf("show should echo current override; got %q", out.String())
	}
}

func TestPrompt_RejectOverlong(t *testing.T) {
	chars, c := seedPromptCharacter(t)
	s, out := bufSession(t)
	s.CharacterID = c.ID
	s.CharacterName = c.Name

	long := strings.Repeat("x", promptMaxLen+1)
	runPromptCmd(t, NewPrompt(chars, testServerDefault), s, "set "+long)

	if !strings.Contains(out.String(), "Invalid template") {
		t.Fatalf("expected rejection of overlong template; got %q", out.String())
	}
	got, _ := chars.FindByName(context.Background(), "Egwene")
	if got.PromptTemplate != "" {
		t.Fatalf("overlong template stored: %q", got.PromptTemplate)
	}
}

func TestPrompt_NoCharacterUnavailable(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	s, out := bufSession(t)
	// CharacterID == 0

	runPromptCmd(t, NewPrompt(chars, testServerDefault), s, "")

	if !strings.Contains(out.String(), "unavailable") {
		t.Fatalf("expected unavailable message; got %q", out.String())
	}
}
