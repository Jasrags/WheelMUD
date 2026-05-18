package cmd

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

func TestGenerateCommandTopics_NilRegistry(t *testing.T) {
	if got := GenerateCommandTopics(nil); got != nil {
		t.Errorf("nil registry should return nil, got %v", got)
	}
}

func TestGenerateCommandTopics_SkipsEmptyHelp(t *testing.T) {
	r := telnet.NewRegistry()
	if err := r.Register(&telnet.Command{
		Name: "ghost",
		Run:  func(*telnet.Context) error { return nil },
		// Help and Long both empty — should be skipped.
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := GenerateCommandTopics(r); len(got) != 0 {
		t.Errorf("expected 0 topics for empty-help command, got %d", len(got))
	}
}

func TestGenerateCommandTopics_BodyPreference(t *testing.T) {
	r := telnet.NewRegistry()
	if err := r.Register(
		&telnet.Command{
			Name: "summary-only",
			Help: "one-line summary",
			Run:  func(*telnet.Context) error { return nil },
		},
		&telnet.Command{
			Name: "long-only",
			Long: "multi-line\nbody",
			Run:  func(*telnet.Context) error { return nil },
		},
		&telnet.Command{
			Name: "both",
			Help: "summary",
			Long: "long body wins",
			Run:  func(*telnet.Context) error { return nil },
		},
	); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := GenerateCommandTopics(r)
	if len(got) != 3 {
		t.Fatalf("expected 3 topics, got %d", len(got))
	}
	byID := map[string]string{}
	for _, tp := range got {
		byID[tp.ID] = tp.Body
	}
	if byID["summary-only"] != "one-line summary" {
		t.Errorf("summary-only.Body = %q, want fallback to Help", byID["summary-only"])
	}
	if byID["long-only"] != "multi-line\nbody" {
		t.Errorf("long-only.Body = %q, want Long", byID["long-only"])
	}
	if byID["both"] != "long body wins" {
		t.Errorf("both.Body = %q, want Long over Help", byID["both"])
	}
}

func TestGenerateCommandTopics_AliasesBecomeKeywords(t *testing.T) {
	r := telnet.NewRegistry()
	if err := r.Register(&telnet.Command{
		Name:    "shout",
		Aliases: []string{"yell", "holler"},
		Help:    "shout at the zone",
		Run:     func(*telnet.Context) error { return nil },
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got := GenerateCommandTopics(r)
	if len(got) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(got))
	}
	if strings.Join(got[0].Keywords, ",") != "yell,holler" {
		t.Errorf("Keywords = %v, want [yell,holler]", got[0].Keywords)
	}
	// Title includes the summary.
	if !strings.Contains(got[0].Title, "shout at the zone") {
		t.Errorf("Title = %q, want to contain summary", got[0].Title)
	}
}

func TestGenerateCommandTopics_AliasDedupedWithName(t *testing.T) {
	// Defensive: a Command whose Aliases include its own name shouldn't
	// surface the name as a keyword (would collide with the topic ID
	// during MergeGenerated).
	r := telnet.NewRegistry()
	if err := r.Register(&telnet.Command{
		Name:    "look",
		Aliases: []string{"look", "examine"},
		Help:    "examine surroundings",
		Run:     func(*telnet.Context) error { return nil },
	}); err != nil {
		// Registry may reject duplicate name-as-alias; if so the test
		// still proves the safety net is upstream.
		t.Skipf("Registry rejects name-in-aliases: %v", err)
	}
	got := GenerateCommandTopics(r)
	if len(got) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(got))
	}
	for _, kw := range got[0].Keywords {
		if kw == "look" {
			t.Errorf("Keywords should not include the topic's own ID, got %v", got[0].Keywords)
		}
	}
}
