package help

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestSourceFS_Embedded(t *testing.T) {
	// Unset HELP_DIR so the embedded fallback is exercised.
	t.Setenv(HelpDirEnv, "")

	fsys, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS embedded: %v", err)
	}
	cat, err := LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS embedded: %v", err)
	}
	if len(cat.All()) == 0 {
		t.Fatal("embedded LoadFS returned an empty catalog")
	}
	// The bundled topics include channeling/combat/currency.
	found := map[string]bool{}
	for _, tp := range cat.All() {
		found[tp.ID] = true
	}
	for _, want := range []string{"channeling", "combat", "currency"} {
		if !found[want] {
			t.Errorf("embedded catalog missing topic %q", want)
		}
	}
}

func TestSourceFS_HelpDirOverride(t *testing.T) {
	dir := t.TempDir()
	topicsDir := filepath.Join(dir, "topics")
	if err := os.MkdirAll(topicsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(topicsDir, "alpha.md"), []byte(
		"---\nid: alpha\ntitle: Alpha\nkeywords: a\n---\nbody\n",
	), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv(HelpDirEnv, dir)

	fsys, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS override: %v", err)
	}
	cat, err := LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS override: %v", err)
	}
	tp, ok := cat.LookupExact("alpha")
	if !ok {
		t.Fatalf("override topic %q not found", "alpha")
	}
	if tp.Title != "Alpha" || strings.TrimSpace(tp.Body) != "body" {
		t.Errorf("topic = %+v", tp)
	}
	// Keyword lookup also works through the override.
	if _, ok := cat.LookupKeyword("a"); !ok {
		t.Errorf("keyword %q not indexed", "a")
	}
}

func TestSourceFS_HelpDirMissing(t *testing.T) {
	t.Setenv(HelpDirEnv, filepath.Join(t.TempDir(), "does-not-exist"))
	if _, err := SourceFS(); err == nil {
		t.Fatal("expected error for missing HELP_DIR target")
	}
}

func TestSourceFS_HelpDirNotADirectory(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "notadir-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Setenv(HelpDirEnv, f.Name())
	if _, err := SourceFS(); err == nil {
		t.Fatal("expected error when HELP_DIR points at a file")
	}
}

func TestMergeGenerated_GapFill(t *testing.T) {
	c := mustCatalog(t, []*Topic{
		{ID: "combat", Title: "Combat (authored)", Keywords: []string{"fight"}, Body: "authored"},
	})

	added, skipped := c.MergeGenerated([]*Topic{
		// Distinct id — should land.
		{ID: "look", Title: "look — examine surroundings", Body: "look-body"},
		// Same id as authored — should be skipped.
		{ID: "combat", Title: "combat (generated)", Body: "gen"},
		// Id collides with an existing keyword — should be skipped.
		{ID: "fight", Title: "fight (generated)", Body: "gen"},
		// Nil and empty-id entries — should be ignored without panic.
		nil,
		{ID: ""},
	})

	if added != 1 || skipped != 2 {
		t.Errorf("added=%d skipped=%d, want 1/2", added, skipped)
	}

	// Authored topic preserved.
	authored, ok := c.LookupExact("combat")
	if !ok || !strings.HasPrefix(authored.Title, "Combat (authored)") {
		t.Errorf("authored combat lost or shadowed: %+v", authored)
	}

	// Generated topic resolvable.
	gen, ok := c.LookupExact("look")
	if !ok || gen.Body != "look-body" {
		t.Errorf("generated look missing: %+v", gen)
	}

	// All() includes both, sorted.
	ids := make([]string, 0)
	for _, tp := range c.All() {
		ids = append(ids, tp.ID)
	}
	sort.Strings(ids)
	want := []string{"combat", "look"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestMergeGenerated_KeywordCollisionDropped(t *testing.T) {
	c := mustCatalog(t, []*Topic{
		{ID: "say", Title: "Say", Keywords: []string{"talk"}, Body: "x"},
	})

	added, skipped := c.MergeGenerated([]*Topic{
		// "tell" lands; its "talk" alias collides with say's keyword and
		// gets dropped, but the topic itself still lands.
		{ID: "tell", Title: "tell — send a private message", Keywords: []string{"talk", "msg"}, Body: "tell-body"},
	})
	if added != 1 || skipped != 0 {
		t.Errorf("added=%d skipped=%d, want 1/0", added, skipped)
	}
	tp, ok := c.LookupExact("tell")
	if !ok {
		t.Fatal("tell not added")
	}
	// "talk" should not have been remapped to tell.
	got, _ := c.LookupKeyword("talk")
	if got == nil || got.ID != "say" {
		t.Errorf("keyword talk = %+v, want say", got)
	}
	// "msg" should map to tell (clean keyword).
	got2, _ := c.LookupKeyword("msg")
	if got2 == nil || got2.ID != "tell" {
		t.Errorf("keyword msg = %+v, want tell", got2)
	}
	// Topic.Keywords reflects the cleaning.
	if strings.Join(tp.Keywords, ",") != "msg" {
		t.Errorf("tell.Keywords = %v, want [msg]", tp.Keywords)
	}
}
