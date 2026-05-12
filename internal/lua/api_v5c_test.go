package lua

import (
	"context"
	"errors"
	"strings"
	"testing"

	gluua "github.com/yuin/gopher-lua"
)

// V5c surface (Phase F #32 slice 5c): wait_ms, inventory_all. Same
// shape coverage as the slice 5b tests — happy path, nil-bound
// classified error, hook-error surface, release-wipe regression.

func TestAPIv5c_WaitMs_HappyPath(t *testing.T) {
	cat := loadScript(t, "wms", `wait_ms(250, "fast_followup")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	type call struct {
		Ms     int32
		Script string
	}
	var seen call
	bindings := APIBindings{
		WaitMs: func(ms int32, scriptName string) error {
			seen = call{Ms: ms, Script: scriptName}
			return nil
		},
	}
	if err := r.Run(context.Background(), "wms", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen.Ms != 250 || seen.Script != "fast_followup" {
		t.Fatalf("hook args: %+v", seen)
	}
}

func TestAPIv5c_WaitMs_HookErrorSurfaces(t *testing.T) {
	cat := loadScript(t, "wms_err", `wait_ms(50, "x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	hookErr := errors.New("ms must be >= 100")
	bindings := APIBindings{
		WaitMs: func(int32, string) error { return hookErr },
	}
	err := r.Run(context.Background(), "wms_err", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
	if !strings.Contains(err.Error(), "ms must be >= 100") {
		t.Fatalf("err = %v; expected hook message in chain", err)
	}
}

func TestAPIv5c_WaitMs_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "wms_nil", `wait_ms(200, "x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "wms_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "wait_ms not bound") {
		t.Fatalf("err = %v; want 'wait_ms not bound'", err)
	}
}

func TestAPIv5c_InventoryAll_ReturnsTable(t *testing.T) {
	cat := loadScript(t, "inv_all", `
local items = inventory_all(42)
if #items ~= 3 then error("len=" .. #items) end
if items[1].id ~= 101 then error("id1=" .. items[1].id) end
if items[2].id ~= 102 then error("id2=" .. items[2].id) end
if items[3].id ~= 103 then error("id3=" .. items[3].id) end
if items[3].external_id ~= "tr.coin" then error("ext3=" .. items[3].external_id) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var seenTarget int64
	bindings := APIBindings{
		InventoryAll: func(targetID int64) ([]InventoryEntry, error) {
			seenTarget = targetID
			return []InventoryEntry{
				{ID: 101, Name: "satchel", ExternalID: "tr.satchel"},
				{ID: 102, Name: "vial", ExternalID: "tr.vial"}, // nested in satchel
				{ID: 103, Name: "coin", ExternalID: "tr.coin"}, // nested in vial
			}, nil
		},
	}
	if err := r.Run(context.Background(), "inv_all", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seenTarget != 42 {
		t.Fatalf("hook target = %d, want 42", seenTarget)
	}
}

func TestAPIv5c_InventoryAll_EmptyReturnsEmptyTable(t *testing.T) {
	cat := loadScript(t, "inv_all_empty", `
local items = inventory_all(1)
if items == nil then error("nil table") end
if #items ~= 0 then error("len=" .. #items) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		InventoryAll: func(int64) ([]InventoryEntry, error) {
			return []InventoryEntry{}, nil
		},
	}
	if err := r.Run(context.Background(), "inv_all_empty", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv5c_InventoryAll_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "inv_all_nil", `inventory_all(1)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "inv_all_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "inventory_all not bound") {
		t.Fatalf("err = %v; want 'inventory_all not bound'", err)
	}
}

// V5c release-wipe regression — mirrors the v5b version. wait_ms +
// inventory_all must join the wipe-list or a leaked closure could
// let one script silently inherit another's hook when the pool
// reuses an LState.
func TestAPIv5c_ReleaseWipesNewGlobals(t *testing.T) {
	cat := loadScript(t, "noop_v5c", `return`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	l, err := r.borrow(context.Background())
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	bindings := APIBindings{
		WaitMs:       func(int32, string) error { return nil },
		InventoryAll: func(int64) ([]InventoryEntry, error) { return nil, nil },
	}
	bindings.Bind(l)

	for _, name := range []string{"wait_ms", "inventory_all"} {
		if v := l.GetGlobal(name); v == gluua.LNil {
			t.Fatalf("pre-release: global %q is nil; Bind did not register it", name)
		}
	}
	r.release(l)
	for _, name := range []string{"wait_ms", "inventory_all"} {
		if v := l.GetGlobal(name); v != gluua.LNil {
			t.Errorf("post-release: global %q = %v (%T), want LNil — wipe-list missing %q", name, v, v, name)
		}
	}
}
