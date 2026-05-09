package lua

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	gluua "github.com/yuin/gopher-lua"

	"github.com/Jasrags/WheelMUD/internal/scripts"
)

// V2 surface (Phase F #32 slice 2): quest.accept, quest.advance,
// push_mode globals. Each binding's nil-hook stub raises a
// classified Lua error (not "attempt to call nil") so the trigger
// fault path can log which API was missing.

func TestAPIv2_QuestAccept_HappyPath(t *testing.T) {
	cat := loadScript(t, "qa", `quest.accept("lost_lamb")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var seen string
	bindings := APIBindings{
		QuestAccept: func(id string) error { seen = id; return nil },
	}
	if err := r.Run(context.Background(), "qa", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen != "lost_lamb" {
		t.Fatalf("hook arg = %q, want lost_lamb", seen)
	}
}

func TestAPIv2_QuestAdvance_HookErrorSurfaces(t *testing.T) {
	cat := loadScript(t, "qadv", `quest.advance("x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	hookErr := errors.New("not on quest")
	bindings := APIBindings{
		QuestAdvance: func(string) error { return hookErr },
	}
	err := r.Run(context.Background(), "qadv", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
	if !strings.Contains(err.Error(), "not on quest") {
		t.Fatalf("err = %v; expected hook message in chain", err)
	}
}

func TestAPIv2_QuestAdvance_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "qnil", `quest.advance("x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{}
	err := r.Run(context.Background(), "qnil", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
	if !strings.Contains(err.Error(), "quest.advance not bound") {
		t.Fatalf("err = %v; want classified 'quest.advance not bound'", err)
	}
}

func TestAPIv2_PushMode_HappyPath(t *testing.T) {
	cat := loadScript(t, "pm", `push_mode("shop")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var captured string
	bindings := APIBindings{
		PushMode: func(name string) error { captured = name; return nil },
	}
	if err := r.Run(context.Background(), "pm", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if captured != "shop" {
		t.Fatalf("hook arg = %q, want shop", captured)
	}
}

func TestAPIv2_PushMode_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "pmnil", `push_mode("shop")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{}
	err := r.Run(context.Background(), "pmnil", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
	if !strings.Contains(err.Error(), "push_mode not bound") {
		t.Fatalf("err = %v; want 'push_mode not bound'", err)
	}
}

// Successive calls on the SAME runner must observe wiped V2
// globals — release clears `quest` / `push_mode` so a pooled
// LState never sees leaked closures from the previous borrow.
// Bundle both scripts in one catalog so the second call hits the
// same runner (and, with poolSize=1 in production behavior, very
// likely the same LState — but the test claim is correctness, not
// observed pool-slot reuse).
func TestAPIv2_ReleaseClearsV2Globals(t *testing.T) {
	body := func(name, src string) (string, []byte) { return name + ".lua", []byte(src) }
	n1, b1 := body("with_bindings", `quest.accept("x") push_mode("y")`)
	n2, b2 := body("leakprobe", `
if type(quest) ~= "nil" then error("expected quest=nil, got " .. type(quest)) end
if type(push_mode) ~= "nil" then error("expected push_mode=nil, got " .. type(push_mode)) end
`)
	parser := gluua.NewState()
	defer parser.Close()
	cat := loadCatalogMulti(t, parser, map[string][]byte{n1: b1, n2: b2})

	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		QuestAccept: func(string) error { return nil },
		PushMode:    func(string) error { return nil },
	}
	bind := func(l *gluua.LState) { bindings.Bind(l) }
	// The pool size is poolSize=8; running with_bindings >= poolSize
	// times guarantees every pooled LState has installed (and then
	// released) the V2 globals. A subsequent run of leakprobe
	// >= poolSize times then exercises all of them in the unbound
	// state; a release-bug on any single LState surfaces.
	for i := 0; i < poolSize; i++ {
		if err := r.Run(context.Background(), "with_bindings", bind); err != nil {
			t.Fatalf("with_bindings Run #%d: %v", i, err)
		}
	}
	for i := 0; i < poolSize; i++ {
		if err := r.Run(context.Background(), "leakprobe", nil); err != nil {
			t.Fatalf("leakprobe Run #%d (release should have wiped): %v", i, err)
		}
	}
}

// loadCatalogMulti compiles each (name → body) pair into one
// scripts.Catalog so a single Runner can serve both. Mirrors
// loadScript's shape but accepts a map.
func loadCatalogMulti(t *testing.T, parser *gluua.LState, files map[string][]byte) *scripts.Catalog {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: body}
	}
	cat, err := scripts.Load(fsys, parser)
	if err != nil {
		t.Fatalf("scripts.Load: %v", err)
	}
	return cat
}
