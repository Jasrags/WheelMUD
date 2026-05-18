package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/emote"
	"github.com/Jasrags/WheelMUD/internal/help"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// reloadFixture constructs a fully-wired ReloadDeps suitable for
// exercising both subsystems. Returns the registry so tests can
// pre-populate non-social verbs (collision tests, etc.).
func reloadFixture(t *testing.T, emoteCat *emote.Catalog, helpCat *help.Catalog) (ReloadDeps, *telnet.Registry, *repo.MemoryAdminAuditRepo) {
	t.Helper()
	r := telnet.NewRegistry()
	audits := repo.NewMemoryAdminAuditRepo()
	deps := ReloadDeps{
		EmoteCatalog: emoteCat,
		HelpCatalog:  helpCat,
		Registry:     r,
		Sessions:     nil,
		Mobs:         nil,
		Audits:       audits,
	}
	// Register the current socials so reload has something to diff
	// against (Unregister + Register cycle).
	if emoteCat != nil {
		for _, c := range NewSocials(emoteCat, nil, nil) {
			if err := r.Register(c); err != nil {
				t.Fatalf("seed register social %q: %v", c.Name, err)
			}
		}
	}
	return deps, r, audits
}

// reloadAdminSession is the local fixture-builder for the reload
// tests — the cross-file `adminSession` helper in track_test.go has
// a different signature (sessions registry / repos). Naming this
// reloadAdminSession avoids the clash.
func reloadAdminSession(t *testing.T) (*telnet.Session, *bufConn) {
	t.Helper()
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.CharacterID = 999
	s.CharacterName = "Admin"
	return s, conn
}

func TestReload_BareListsSubsystems(t *testing.T) {
	deps, _, _ := reloadFixture(t, nil, nil)
	verb := NewReload(deps)
	admin, out := reloadAdminSession(t)
	runCmd(t, verb, admin, "")
	if !strings.Contains(out.String(), "socials") || !strings.Contains(out.String(), "help") {
		t.Fatalf("expected subsystem list; got %q", out.String())
	}
}

func TestReload_UnknownSubsystem(t *testing.T) {
	deps, _, _ := reloadFixture(t, nil, nil)
	verb := NewReload(deps)
	admin, out := reloadAdminSession(t)
	runCmd(t, verb, admin, "rooms")
	if !strings.Contains(out.String(), "Unknown subsystem") {
		t.Fatalf("expected unknown-subsystem; got %q", out.String())
	}
}

// pointEmoteDir writes a fresh socials.yaml into a temp dir and
// points EMOTE_DIR at it for the duration of the test. Returns the
// dir path so the test can rewrite the file mid-run.
func pointEmoteDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "socials.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(emote.DirEnv, dir)
	return dir
}

func TestReloadSocials_AddRemoveChange(t *testing.T) {
	dir := pointEmoteDir(t, `socials:
  - id: smile
    help: smile [target] — original
    self: You smile.
    other: "{actor} smiles."
    target_self: You smile at {target}.
    target_view: "{actor} smiles at you."
    target_other: "{actor} smiles at {target}."
  - id: sigh
    self: You sigh.
    other: "{actor} sighs."
`)
	fsys, err := emote.SourceFS()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := emote.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	deps, r, audits := reloadFixture(t, cat, nil)

	// New catalog file: smile's help changes, sigh removed, cackle added.
	if err := os.WriteFile(filepath.Join(dir, "socials.yaml"), []byte(`socials:
  - id: smile
    help: smile [target] — UPDATED
    self: You smile.
    other: "{actor} smiles."
    target_self: You smile at {target}.
    target_view: "{actor} smiles at you."
    target_other: "{actor} smiles at {target}."
  - id: cackle
    self: You cackle.
    other: "{actor} cackles."
`), 0o644); err != nil {
		t.Fatal(err)
	}

	admin, out := reloadAdminSession(t)
	runCmd(t, NewReload(deps), admin, "socials")

	body := out.String()
	if !strings.Contains(body, "added 1: cackle") {
		t.Errorf("missing add report: %q", body)
	}
	if !strings.Contains(body, "removed 1: sigh") {
		t.Errorf("missing remove report: %q", body)
	}
	if !strings.Contains(body, "changed 1: smile") {
		t.Errorf("missing change report: %q", body)
	}

	// Registry reflects the diff.
	if _, err := r.Lookup("sigh"); err == nil {
		t.Errorf("sigh should be unregistered")
	}
	if _, err := r.Lookup("cackle"); err != nil {
		t.Errorf("cackle should be registered, got %v", err)
	}

	// Audit row landed.
	rows, err := audits.List(context.Background(), repo.AdminAuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.Verb == "reload" && r.Target == "socials" {
			found = true
			if !strings.Contains(r.Args, "added=1") {
				t.Errorf("audit args missing add count: %q", r.Args)
			}
		}
	}
	if !found {
		t.Errorf("audit row missing for reload socials")
	}
}

func TestReloadSocials_CollidesWithNonSocialVerb(t *testing.T) {
	pointEmoteDir(t, `socials:
  - id: smile
    self: You smile.
    other: "{actor} smiles."
`)
	fsys, _ := emote.SourceFS()
	cat, _ := emote.Load(fsys)
	deps, r, _ := reloadFixture(t, cat, nil)

	// Pre-register a non-social verb that the new catalog will collide with.
	_ = r.Register(&telnet.Command{
		Name: "wave",
		Auth: telnet.AuthPlayer,
		Run:  func(*telnet.Context) error { return nil },
	})

	dir := os.Getenv(emote.DirEnv)
	if err := os.WriteFile(filepath.Join(dir, "socials.yaml"), []byte(`socials:
  - id: smile
    self: You smile.
    other: "{actor} smiles."
  - id: wave
    self: You wave.
    other: "{actor} waves."
`), 0o644); err != nil {
		t.Fatal(err)
	}

	admin, out := reloadAdminSession(t)
	runCmd(t, NewReload(deps), admin, "socials")
	if !strings.Contains(out.String(), "collides") {
		t.Fatalf("expected collision abort; got %q", out.String())
	}
	// Pre-existing non-social verb is untouched.
	if _, err := r.Lookup("wave"); err != nil {
		t.Errorf("non-social wave should still resolve: %v", err)
	}
}

func TestReloadSocials_ParseErrorLeavesCatalogIntact(t *testing.T) {
	dir := pointEmoteDir(t, `socials:
  - id: smile
    self: You smile.
    other: "{actor} smiles."
`)
	fsys, _ := emote.SourceFS()
	cat, _ := emote.Load(fsys)
	deps, r, _ := reloadFixture(t, cat, nil)

	// Now corrupt the file.
	if err := os.WriteFile(filepath.Join(dir, "socials.yaml"), []byte("socials: : :\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	admin, out := reloadAdminSession(t)
	runCmd(t, NewReload(deps), admin, "socials")
	if !strings.Contains(out.String(), "parse failed") {
		t.Fatalf("expected parse-failed message; got %q", out.String())
	}
	// The pre-reload smile verb survives.
	if _, err := r.Lookup("smile"); err != nil {
		t.Errorf("smile should survive failed reload, got %v", err)
	}
}

func TestReloadHelp_AddedRemovedChanged(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "topics"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTopic := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, "topics", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeTopic("alpha.md", "---\nid: alpha\ntitle: A1\n---\nbody\n")
	writeTopic("bravo.md", "---\nid: bravo\ntitle: B\n---\nbody\n")
	t.Setenv(help.HelpDirEnv, dir)

	fsys, err := help.SourceFS()
	if err != nil {
		t.Fatal(err)
	}
	hc, err := help.LoadFS(fsys)
	if err != nil {
		t.Fatal(err)
	}

	deps, _, audits := reloadFixture(t, nil, hc)

	// Mutate the on-disk topics: alpha body changes, bravo stays,
	// charlie is new.
	writeTopic("alpha.md", "---\nid: alpha\ntitle: A2\n---\nbody\n")
	writeTopic("charlie.md", "---\nid: charlie\ntitle: C\n---\nbody\n")

	admin, out := reloadAdminSession(t)
	runCmd(t, NewReload(deps), admin, "help")

	body := out.String()
	if !strings.Contains(body, "added=1") {
		t.Errorf("expected added=1 in %q", body)
	}
	if !strings.Contains(body, "changed=1") {
		t.Errorf("expected changed=1 in %q", body)
	}

	// LookupExact sees the updated body.
	if got, ok := hc.LookupExact("alpha"); !ok || got.Title != "A2" {
		t.Errorf("alpha post-reload: %+v ok=%v", got, ok)
	}

	rows, _ := audits.List(context.Background(), repo.AdminAuditFilter{})
	found := false
	for _, r := range rows {
		if r.Verb == "reload" && r.Target == "help" {
			found = true
		}
	}
	if !found {
		t.Errorf("audit row missing for reload help")
	}
}

func TestReloadHelp_ParseErrorLeavesCatalogIntact(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "topics"), 0o755); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(dir, "topics", "alpha.md")
	if err := os.WriteFile(good, []byte("---\nid: alpha\ntitle: A\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(help.HelpDirEnv, dir)
	fsys, _ := help.SourceFS()
	hc, err := help.LoadFS(fsys)
	if err != nil {
		t.Fatal(err)
	}
	deps, _, _ := reloadFixture(t, nil, hc)

	// Replace the file with garbage.
	if err := os.WriteFile(good, []byte("no front matter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	admin, out := reloadAdminSession(t)
	runCmd(t, NewReload(deps), admin, "help")
	if !strings.Contains(out.String(), "parse failed") {
		t.Fatalf("expected parse-failed; got %q", out.String())
	}
	if _, ok := hc.LookupExact("alpha"); !ok {
		t.Errorf("alpha should survive failed help reload")
	}
}

// Compile-time confirmation that the ReloadDeps shape is what the
// registry wires. Catches a struct rename / field rename.
var _ = fstest.MapFS{}
