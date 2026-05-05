package cmd

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/help"
	"github.com/Jasrags/WheelMUD/telnet"
)

// helpFixture builds a registry with a few well-known commands and
// loads the real embedded help catalog. The seeded commands intersect
// the topic ID space (look/loot vs combat/channeling/currency, plus a
// "channels" cmd that prefix-collides with the "channeling" topic) so
// every resolution path can be exercised.
func helpFixture(t *testing.T) (*telnet.Registry, *help.Catalog) {
	t.Helper()
	r := telnet.NewRegistry()
	noop := func(c *telnet.Context) error { return nil }
	must := func(err error) {
		if err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	must(r.Register(&telnet.Command{Name: "look", Help: "Look around", Run: noop}))
	must(r.Register(&telnet.Command{Name: "loot", Help: "Loot a corpse", Run: noop}))
	must(r.Register(&telnet.Command{Name: "channels", Help: "List chat channels", Run: noop}))
	hc, err := help.Load()
	if err != nil {
		t.Fatalf("help.Load: %v", err)
	}
	must(r.Register(NewHelp(r, hc)))
	return r, hc
}

func helpRun(t *testing.T, r *telnet.Registry, raw string) string {
	t.Helper()
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	hc, err := r.LookupExact("help")
	if err != nil {
		t.Fatalf("lookup help: %v", err)
	}
	runCmd(t, hc, s, raw)
	return conn.String()
}

func TestHelp_NoArgsListsCommandsAndTopics(t *testing.T) {
	r, _ := helpFixture(t)
	out := helpRun(t, r, "")
	if !strings.Contains(out, "Commands:") {
		t.Errorf("missing Commands header: %q", out)
	}
	if !strings.Contains(out, "Topics:") {
		t.Errorf("missing Topics header: %q", out)
	}
	if !strings.Contains(out, "look") || !strings.Contains(out, "combat") {
		t.Errorf("missing entries: %q", out)
	}
}

func TestHelp_ExactCommandWins(t *testing.T) {
	r, _ := helpFixture(t)
	out := helpRun(t, r, "channels")
	// "channels" is a command and prefix-matches the "channeling"
	// topic. Exact-cmd must win over prefix.
	if !strings.Contains(out, "List chat channels") {
		t.Errorf("expected command detail; got: %q", out)
	}
	if strings.Contains(out, "Channeling the One Power") {
		t.Errorf("topic should not be rendered when cmd exact matches: %q", out)
	}
}

func TestHelp_ExactTopicResolves(t *testing.T) {
	r, _ := helpFixture(t)
	out := helpRun(t, r, "combat")
	if !strings.Contains(out, "Combat") {
		t.Errorf("expected topic title: %q", out)
	}
	if !strings.Contains(out, "round-based") {
		t.Errorf("expected topic body: %q", out)
	}
}

func TestHelp_KeywordResolvesToTopic(t *testing.T) {
	r, _ := helpFixture(t)
	out := helpRun(t, r, "fight")
	if !strings.Contains(out, "Combat") {
		t.Errorf("keyword fight should resolve to Combat: %q", out)
	}
}

func TestHelp_UniquePrefixAcrossSpaces(t *testing.T) {
	r, _ := helpFixture(t)
	// "comb" matches only the "combat" topic; no command starts with
	// it. Should resolve to the topic.
	out := helpRun(t, r, "comb")
	if !strings.Contains(out, "Combat") {
		t.Errorf("expected Combat topic via unique prefix: %q", out)
	}
}

func TestHelp_AmbiguousPrefixGrouped(t *testing.T) {
	r, _ := helpFixture(t)
	// "ch" prefix-matches the "channels" command and the
	// "channeling" topic — must list both, grouped.
	out := helpRun(t, r, "ch")
	if !strings.Contains(out, "Commands matching ch:") {
		t.Errorf("missing commands group: %q", out)
	}
	if !strings.Contains(out, "Topics matching ch:") {
		t.Errorf("missing topics group: %q", out)
	}
	if !strings.Contains(out, "channels") || !strings.Contains(out, "channeling") {
		t.Errorf("missing entries: %q", out)
	}
}

func TestHelp_UnknownReportsBoth(t *testing.T) {
	r, _ := helpFixture(t)
	out := helpRun(t, r, "nosuchquery")
	if !strings.Contains(out, "No such command or topic: nosuchquery") {
		t.Errorf("unexpected miss output: %q", out)
	}
}

func TestHelp_CompleterIncludesCommandsAndTopics(t *testing.T) {
	r, _ := helpFixture(t)
	hc, err := r.LookupExact("help")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	cands := hc.Completer(s, "c")
	gotIDs := map[string]bool{}
	for _, c := range cands {
		gotIDs[c.Text] = true
	}
	for _, want := range []string{"channels", "channeling", "combat", "currency"} {
		if !gotIDs[want] {
			t.Errorf("completer missing %q; got %v", want, gotIDs)
		}
	}
}

func TestHelp_GuestCannotEnumeratePrivilegedCommands(t *testing.T) {
	// Mirrors the dispatcher's "Unknown command" enumeration
	// guard: `help <admin-verb>` and `help <admin-prefix>` from a
	// guest must not surface the privileged command's name or body.
	r := telnet.NewRegistry()
	noop := func(c *telnet.Context) error { return nil }
	if err := r.Register(
		&telnet.Command{Name: "look", Help: "Look around", Run: noop},
		&telnet.Command{Name: "shutdown", Help: "Stop the server", Long: "ADMIN ONLY", Auth: telnet.AuthAdmin, Run: noop},
	); err != nil {
		t.Fatalf("register: %v", err)
	}
	hcCat, err := help.Load()
	if err != nil {
		t.Fatalf("help.Load: %v", err)
	}
	if err := r.Register(NewHelp(r, hcCat)); err != nil {
		t.Fatalf("register help: %v", err)
	}
	hcmd, err := r.LookupExact("help")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	// Exact-name path: guest must not see the admin command.
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthGuest
	runCmd(t, hcmd, s, "shutdown")
	out := conn.String()
	if strings.Contains(out, "ADMIN ONLY") || strings.Contains(out, "Stop the server") {
		t.Errorf("guest saw privileged help body: %q", out)
	}
	if !strings.Contains(out, "No such command or topic") {
		t.Errorf("guest should get not-found, got: %q", out)
	}

	// Prefix path: guest must not see "shutdown" listed in
	// ambiguity output, and a unique-prefix that resolves only to
	// a privileged command must miss instead.
	conn.Reset()
	runCmd(t, hcmd, s, "shu")
	out = conn.String()
	if strings.Contains(out, "shutdown") {
		t.Errorf("guest saw privileged cmd via prefix: %q", out)
	}
}

func TestHelp_NilCatalogCollapsesToCommandsOnly(t *testing.T) {
	r := telnet.NewRegistry()
	noop := func(c *telnet.Context) error { return nil }
	if err := r.Register(&telnet.Command{Name: "look", Help: "Look", Run: noop}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(NewHelp(r, nil)); err != nil {
		t.Fatalf("register help: %v", err)
	}
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	hc, _ := r.LookupExact("help")
	runCmd(t, hc, s, "")
	out := conn.String()
	if !strings.Contains(out, "Commands:") {
		t.Errorf("missing Commands header: %q", out)
	}
	if strings.Contains(out, "Topics:") {
		t.Errorf("nil catalog should not emit Topics header: %q", out)
	}
}
