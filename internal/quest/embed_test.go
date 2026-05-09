package quest

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoad_Embedded(t *testing.T) {
	// The embedded default ships zero quests today (just a README).
	// Load must accept that without erroring; builders add quests in
	// internal/quest/default/<id>.yaml or via QUEST_DIR.
	fsys, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS: %v", err)
	}
	cat, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cat == nil {
		t.Fatal("Load returned nil catalog")
	}
}

func TestLoad_AllowsEmpty(t *testing.T) {
	fsys := fstest.MapFS{}
	cat, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if len(cat.ByID) != 0 {
		t.Fatalf("expected empty catalog, got %d", len(cat.ByID))
	}
}

func TestLoad_RejectsDuplicate(t *testing.T) {
	body := []byte(`id: dup
name: A
steps:
  - kind: talk_to
    mob: m
    prompt: hi
`)
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: body},
		"b.yaml": &fstest.MapFile{Data: body},
	}
	_, err := Load(fsys)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "duplicate quest id") {
		t.Fatalf("err = %q", err)
	}
}

func TestLoad_RejectsMissingID(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte("name: anonymous\nsteps: []\n")},
	}
	if _, err := Load(fsys); err == nil {
		t.Fatal("expected missing-id error")
	}
}

func TestLoad_SkipsNonYAML(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml":   &fstest.MapFile{Data: []byte("id: ok\nname: A\nsteps:\n  - kind: talk_to\n    mob: m\n    prompt: hi\n")},
		"README":   &fstest.MapFile{Data: []byte("# notes")},
		"junk.txt": &fstest.MapFile{Data: []byte("ignore me")},
	}
	cat, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cat.Get("ok"); !ok {
		t.Fatal("expected ok quest")
	}
}
