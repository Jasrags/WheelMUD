package lua

import (
	"context"
	"errors"
	"strings"
	"testing"

	gluua "github.com/yuin/gopher-lua"
)

// V5b surface (Phase F #32 slice 5b): wait, inventory. Same shape
// as the slice 5a tests — happy path, nil-bound classified error,
// release-wipe coverage (added to the existing direct unit test).

func TestAPIv5b_Wait_HappyPath(t *testing.T) {
	cat := loadScript(t, "wt", `wait(7, "follow_up")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	type call struct {
		Seconds int32
		Script  string
	}
	var seen call
	bindings := APIBindings{
		Wait: func(seconds int32, scriptName string) error {
			seen = call{Seconds: seconds, Script: scriptName}
			return nil
		},
	}
	if err := r.Run(context.Background(), "wt", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen.Seconds != 7 || seen.Script != "follow_up" {
		t.Fatalf("hook args: %+v", seen)
	}
}

func TestAPIv5b_Wait_HookErrorSurfaces(t *testing.T) {
	cat := loadScript(t, "wt_err", `wait(5, "x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	hookErr := errors.New("script not in catalog")
	bindings := APIBindings{
		Wait: func(int32, string) error { return hookErr },
	}
	err := r.Run(context.Background(), "wt_err", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
	if !strings.Contains(err.Error(), "script not in catalog") {
		t.Fatalf("err = %v; expected hook message in chain", err)
	}
}

func TestAPIv5b_Wait_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "wt_nil", `wait(1, "x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "wt_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "wait not bound") {
		t.Fatalf("err = %v; want 'wait not bound'", err)
	}
}

func TestAPIv5b_Inventory_ReturnsTable(t *testing.T) {
	cat := loadScript(t, "inv", `
local items = inventory(42)
if #items ~= 2 then error("len=" .. #items) end
if items[1].id ~= 101 then error("id1=" .. items[1].id) end
if items[1].name ~= "sword" then error("name1=" .. items[1].name) end
if items[1].external_id ~= "tr.sword" then error("ext1=" .. items[1].external_id) end
if items[2].id ~= 102 then error("id2=" .. items[2].id) end
if items[2].external_id ~= "tr.cloak" then error("ext2=" .. items[2].external_id) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var seenTarget int64
	bindings := APIBindings{
		Inventory: func(targetID int64) ([]InventoryEntry, error) {
			seenTarget = targetID
			return []InventoryEntry{
				{ID: 101, Name: "sword", ExternalID: "tr.sword"},
				{ID: 102, Name: "cloak", ExternalID: "tr.cloak"},
			}, nil
		},
	}
	if err := r.Run(context.Background(), "inv", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seenTarget != 42 {
		t.Fatalf("hook target = %d, want 42", seenTarget)
	}
}

func TestAPIv5b_Inventory_EmptyReturnsEmptyTable(t *testing.T) {
	// Empty inventory must yield an iterable table, not nil — Lua
	// ipairs over nil errors out, so an "empty character" script
	// shouldn't surface as a classified fault.
	cat := loadScript(t, "inv_empty", `
local items = inventory(1)
if items == nil then error("nil table") end
if #items ~= 0 then error("len=" .. #items) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		Inventory: func(int64) ([]InventoryEntry, error) {
			return []InventoryEntry{}, nil
		},
	}
	if err := r.Run(context.Background(), "inv_empty", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv5b_Inventory_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "inv_nil", `inventory(1)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "inv_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "inventory not bound") {
		t.Fatalf("err = %v; want 'inventory not bound'", err)
	}
}

func TestAPIv5b_Inventory_HookErrorSurfaces(t *testing.T) {
	cat := loadScript(t, "inv_err", `inventory(7)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	hookErr := errors.New("repo down")
	bindings := APIBindings{
		Inventory: func(int64) ([]InventoryEntry, error) { return nil, hookErr },
	}
	err := r.Run(context.Background(), "inv_err", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "repo down") {
		t.Fatalf("err = %v; want hook error in chain", err)
	}
}

// V5b release-wipe regression: extend the direct unit test that
// confirms release() nils every per-call global. wait + inventory
// must join the wipe-list or a leaked closure could let one script
// silently inherit another's hook when the pool reuses an LState.
func TestAPIv5b_ReleaseWipesNewGlobals(t *testing.T) {
	cat := loadScript(t, "noop", `return`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	l, err := r.borrow(context.Background())
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	bindings := APIBindings{
		Wait:      func(int32, string) error { return nil },
		Inventory: func(int64) ([]InventoryEntry, error) { return nil, nil },
	}
	bindings.Bind(l)

	for _, name := range []string{"wait", "inventory"} {
		v := l.GetGlobal(name)
		if v == gluua.LNil {
			t.Fatalf("pre-release: global %q is nil; Bind did not register it", name)
		}
	}
	r.release(l)
	for _, name := range []string{"wait", "inventory"} {
		if v := l.GetGlobal(name); v != gluua.LNil {
			t.Errorf("post-release: global %q = %v (%T), want LNil — wipe-list missing %q", name, v, v, name)
		}
	}
}
