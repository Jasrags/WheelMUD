package lua

import (
	"context"
	"errors"
	"strings"
	"testing"

	gluua "github.com/yuin/gopher-lua"
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

// Successive calls reuse the pool — make sure release wipes the
// quest / push_mode globals so a script can't observe leaked
// closures from the previous borrow.
func TestAPIv2_ReleaseClearsV2Globals(t *testing.T) {
	cat := loadScript(t, "leakprobe", `
if type(quest) ~= "nil" then error("expected quest=nil, got " .. type(quest)) end
if type(push_mode) ~= "nil" then error("expected push_mode=nil, got " .. type(push_mode)) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	// First call: install bindings.
	first := loadScript(t, "first", `quest.accept("x") push_mode("y")`)
	r2 := NewRunner(first, nil)
	defer r2.Stop()
	bindings := APIBindings{
		QuestAccept: func(string) error { return nil },
		PushMode:    func(string) error { return nil },
	}
	if err := r2.Run(context.Background(), "first", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	// Second call on same runner with no bindings — globals must
	// observe as nil because release wiped them.
	if err := r.Run(context.Background(), "leakprobe", nil); err != nil {
		t.Fatalf("leakprobe Run: %v", err)
	}
}
