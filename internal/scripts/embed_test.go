package scripts

import (
	"strings"
	"testing"
	"testing/fstest"

	lua "github.com/yuin/gopher-lua"
)

// newL returns a transient LState used only for parse during Load
// tests. Callers must Close it.
func newL(t *testing.T) *lua.LState {
	t.Helper()
	l := lua.NewState()
	t.Cleanup(l.Close)
	return l
}

func TestLoad_Embedded(t *testing.T) {
	// The embedded default ships zero scripts today (just a README).
	// Load must accept that without erroring; builders add scripts in
	// internal/scripts/default/<name>.lua or via SCRIPT_DIR.
	fsys, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS: %v", err)
	}
	cat, err := Load(fsys, newL(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cat == nil {
		t.Fatal("nil catalog")
	}
}

func TestLoad_AllowsEmpty(t *testing.T) {
	cat, err := Load(fstest.MapFS{}, newL(t))
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if len(cat.ByName) != 0 {
		t.Fatalf("expected empty catalog, got %d", len(cat.ByName))
	}
}

func TestLoad_HappyPath(t *testing.T) {
	src := `say("hello")` + "\n"
	fsys := fstest.MapFS{
		"warden.lua": &fstest.MapFile{Data: []byte(src)},
	}
	cat, err := Load(fsys, newL(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := cat.Get("warden")
	if !ok {
		t.Fatal("warden not in catalog")
	}
	if got.Source != src {
		t.Fatalf("source mismatch: got %q", got.Source)
	}
	if got.Compiled == nil {
		t.Fatal("Compiled prototype is nil")
	}
}

func TestLoad_RejectsSyntaxError(t *testing.T) {
	// Unbalanced parens — gopher-lua's parser surfaces a clear error.
	fsys := fstest.MapFS{
		"broken.lua": &fstest.MapFile{Data: []byte(`say("hi"`)},
	}
	_, err := Load(fsys, newL(t))
	if err == nil {
		t.Fatal("expected syntax error")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Fatalf("error should name the file: %v", err)
	}
}

func TestLoad_RejectsDuplicateName(t *testing.T) {
	body := []byte(`say("hi")`)
	fsys := fstest.MapFS{
		"a/dup.lua": &fstest.MapFile{Data: body},
		"b/dup.lua": &fstest.MapFile{Data: body},
	}
	_, err := Load(fsys, newL(t))
	if err == nil {
		t.Fatal("expected dup error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %q", err)
	}
}

func TestLoad_SkipsNonLua(t *testing.T) {
	fsys := fstest.MapFS{
		"warden.lua": &fstest.MapFile{Data: []byte(`say("hi")`)},
		"README":     &fstest.MapFile{Data: []byte("# notes")},
		"notes.txt":  &fstest.MapFile{Data: []byte("ignore me")},
	}
	cat, err := Load(fsys, newL(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cat.Get("warden"); !ok {
		t.Fatal("expected warden")
	}
	if _, ok := cat.Get("README"); ok {
		t.Fatal("README should be skipped")
	}
}

func TestCatalog_GetNil(t *testing.T) {
	var c *Catalog
	if _, ok := c.Get("anything"); ok {
		t.Fatal("nil catalog should miss")
	}
}
