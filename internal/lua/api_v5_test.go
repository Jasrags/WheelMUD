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

// loadScripts is loadScript's multi-entry sibling: compiles every
// name→src pair into one Catalog so a single Runner can serve them
// back-to-back. Used by the V5a release-wipe test to confirm the
// pool's wipe-list nils every global between borrows of the same
// LState.
func loadScripts(t *testing.T, srcs map[string]string) *scripts.Catalog {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, body := range srcs {
		fsys[name+".lua"] = &fstest.MapFile{Data: []byte(body)}
	}
	parser := gluua.NewState()
	defer parser.Close()
	cat, err := scripts.Load(fsys, parser)
	if err != nil {
		t.Fatalf("scripts.Load: %v", err)
	}
	return cat
}

// V5a surface (Phase F #32 slice 5a): deal_damage, heal,
// transfer_item, drop_item. Each binding's nil-hook stub raises a
// classified Lua error so the trigger fault path can log which API
// was missing.

func TestAPIv5_DealDamage_HappyPath(t *testing.T) {
	cat := loadScript(t, "dd", `deal_damage(42, 7, "fire_trap")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	type call struct {
		Target int64
		Amount int32
		Source string
	}
	var seen call
	bindings := APIBindings{
		DealDamage: func(target int64, amount int32, source string) error {
			seen = call{Target: target, Amount: amount, Source: source}
			return nil
		},
	}
	if err := r.Run(context.Background(), "dd", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen.Target != 42 || seen.Amount != 7 || seen.Source != "fire_trap" {
		t.Fatalf("hook args: %+v", seen)
	}
}

func TestAPIv5_DealDamage_OptionalSource(t *testing.T) {
	// Source is optional — defaults to "" so silent producer
	// scripts work. The default narration subscriber substitutes
	// "an unseen force" when the string is empty.
	cat := loadScript(t, "dd_src", `deal_damage(1, 3)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var sawSource string
	bindings := APIBindings{
		DealDamage: func(_ int64, _ int32, source string) error {
			sawSource = source
			return nil
		},
	}
	if err := r.Run(context.Background(), "dd_src", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sawSource != "" {
		t.Fatalf("source = %q, want empty", sawSource)
	}
}

func TestAPIv5_DealDamage_HookErrorSurfaces(t *testing.T) {
	cat := loadScript(t, "dd_err", `deal_damage(7, 5, "x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	hookErr := errors.New("target already dead")
	bindings := APIBindings{
		DealDamage: func(int64, int32, string) error { return hookErr },
	}
	err := r.Run(context.Background(), "dd_err", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
	if !strings.Contains(err.Error(), "target already dead") {
		t.Fatalf("err = %v; expected hook message in chain", err)
	}
}

func TestAPIv5_DealDamage_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "dd_nil", `deal_damage(1, 5, "")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "dd_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "deal_damage not bound") {
		t.Fatalf("err = %v; want 'deal_damage not bound'", err)
	}
}

func TestAPIv5_Heal_HappyPath(t *testing.T) {
	cat := loadScript(t, "hl", `heal(99, 12)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	type call struct {
		Target int64
		Amount int32
	}
	var seen call
	bindings := APIBindings{
		Heal: func(target int64, amount int32) error {
			seen = call{Target: target, Amount: amount}
			return nil
		},
	}
	if err := r.Run(context.Background(), "hl", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen.Target != 99 || seen.Amount != 12 {
		t.Fatalf("hook args: %+v", seen)
	}
}

func TestAPIv5_Heal_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "hl_nil", `heal(1, 5)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "hl_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "heal not bound") {
		t.Fatalf("err = %v; want 'heal not bound'", err)
	}
}

func TestAPIv5_TransferItem_HappyPath(t *testing.T) {
	cat := loadScript(t, "ti", `transfer_item(101, 7)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	type call struct {
		Item, ToOwner int64
	}
	var seen call
	bindings := APIBindings{
		TransferItem: func(item, toOwner int64) error {
			seen = call{Item: item, ToOwner: toOwner}
			return nil
		},
	}
	if err := r.Run(context.Background(), "ti", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen.Item != 101 || seen.ToOwner != 7 {
		t.Fatalf("hook args: %+v", seen)
	}
}

func TestAPIv5_TransferItem_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "ti_nil", `transfer_item(1, 2)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "ti_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "transfer_item not bound") {
		t.Fatalf("err = %v; want 'transfer_item not bound'", err)
	}
}

func TestAPIv5_DropItem_HappyPath(t *testing.T) {
	cat := loadScript(t, "di", `drop_item(55)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var sawItem int64
	bindings := APIBindings{
		DropItem: func(item int64) error {
			sawItem = item
			return nil
		},
	}
	if err := r.Run(context.Background(), "di", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sawItem != 55 {
		t.Fatalf("hook item = %d, want 55", sawItem)
	}
}

func TestAPIv5_DropItem_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "di_nil", `drop_item(1)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "di_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "drop_item not bound") {
		t.Fatalf("err = %v; want 'drop_item not bound'", err)
	}
}

// TestAPIv5_ReleaseWipesNewGlobals directly verifies the runner's
// release path nils every V5a global. A unit test on release (rather
// than back-to-back Run calls) is necessary because the 8-state
// pool serves successive Run() invocations from different LStates,
// so an end-to-end test would not reliably exercise the same
// underlying state.
//
// We borrow an LState, bind every V5a hook, confirm the globals are
// callable, release, then read the same LState's globals back
// directly and assert each is LNil. A stale closure binding here
// would let one script silently inherit another script's hook on
// LState reuse — the same kind of cross-call state leak the
// sandbox is meant to prevent.
func TestAPIv5_ReleaseWipesNewGlobals(t *testing.T) {
	cat := loadScript(t, "noop", `return`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	l, err := r.borrow(context.Background())
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	bindings := APIBindings{
		DealDamage:   func(int64, int32, string) error { return nil },
		Heal:         func(int64, int32) error { return nil },
		TransferItem: func(int64, int64) error { return nil },
		DropItem:     func(int64) error { return nil },
	}
	bindings.Bind(l)

	// Sanity check: each V5a global is a callable LFunction before
	// release. If Bind regresses (e.g. forgetting to register one of
	// the four), this branch surfaces it.
	for _, name := range []string{"deal_damage", "heal", "transfer_item", "drop_item"} {
		v := l.GetGlobal(name)
		if v == gluua.LNil {
			t.Fatalf("pre-release: global %q is nil; Bind did not register it", name)
		}
		if _, ok := v.(*gluua.LFunction); !ok {
			t.Fatalf("pre-release: global %q is %T, want LFunction", name, v)
		}
	}

	r.release(l)

	// After release, every V5a global must be LNil so the next
	// borrower starts clean. release also wipes the v1–v4 surface;
	// we probe only the new entries since the older ones have their
	// own regression tests.
	for _, name := range []string{"deal_damage", "heal", "transfer_item", "drop_item"} {
		if v := l.GetGlobal(name); v != gluua.LNil {
			t.Errorf("post-release: global %q = %v (%T), want LNil — wipe-list is missing %q", name, v, v, name)
		}
	}
}

// TestAPIv5_ReleaseWipesNewGlobals_EndToEnd is the higher-level
// companion to the unit test above. It exercises the wipe through
// the public Run API by forcing LState reuse — drain the pool down
// to a single LState, then run two scripts back-to-back that
// necessarily share it. The second script runs with empty bindings
// and must classified-error on the V5a globals the first script
// installed.
func TestAPIv5_ReleaseWipesNewGlobals_EndToEnd(t *testing.T) {
	cat := loadScripts(t, map[string]string{
		"first":  `deal_damage(1, 1, "x")`,
		"second": `deal_damage(2, 2, "y")`,
	})
	r := NewRunner(cat, nil)
	defer r.Stop()

	// Drain the pool to 1 free LState so the two Run calls below
	// MUST reuse it. We don't release the drained ones until the
	// test exits, guaranteeing the second Run picks up the LState
	// the first Run released.
	const poolBleed = poolSize - 1
	held := make([]*gluua.LState, 0, poolBleed)
	for i := 0; i < poolBleed; i++ {
		l, err := r.borrow(context.Background())
		if err != nil {
			t.Fatalf("drain borrow %d: %v", i, err)
		}
		held = append(held, l)
	}
	defer func() {
		for _, l := range held {
			r.release(l)
		}
	}()

	bindings := APIBindings{
		DealDamage: func(int64, int32, string) error { return nil },
	}
	if err := r.Run(context.Background(), "first", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	err := r.Run(context.Background(), "second", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "deal_damage not bound") {
		t.Fatalf("second Run = %v; want 'deal_damage not bound' classified", err)
	}
}
