package lua

import (
	"context"
	"errors"
	"strings"
	"testing"

	gluua "github.com/yuin/gopher-lua"
)

// V3 surface (Phase F #32 slice 3): apply_affect, give_item,
// target.hp, target.level. Each binding's nil-hook stub raises a
// classified Lua error so the trigger fault path can log which API
// was missing.

func TestAPIv3_ApplyAffect_HappyPath(t *testing.T) {
	cat := loadScript(t, "aa", `apply_affect(42, "bull_strength")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	type call struct {
		Target int64
		Effect string
	}
	var seen call
	bindings := APIBindings{
		ApplyAffect: func(target int64, effect string, _ int32) error {
			seen = call{Target: target, Effect: effect}
			return nil
		},
	}
	if err := r.Run(context.Background(), "aa", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen.Target != 42 || seen.Effect != "bull_strength" {
		t.Fatalf("hook args: %+v", seen)
	}
}

func TestAPIv3_ApplyAffect_HookErrorSurfaces(t *testing.T) {
	cat := loadScript(t, "aa_err", `apply_affect(7, "x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	hookErr := errors.New("unknown effect")
	bindings := APIBindings{
		ApplyAffect: func(int64, string, int32) error { return hookErr },
	}
	err := r.Run(context.Background(), "aa_err", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
	if !strings.Contains(err.Error(), "unknown effect") {
		t.Fatalf("err = %v; expected hook message in chain", err)
	}
}

func TestAPIv3_ApplyAffect_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "aa_nil", `apply_affect(1, "x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "aa_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
	if !strings.Contains(err.Error(), "apply_affect not bound") {
		t.Fatalf("err = %v; want 'apply_affect not bound'", err)
	}
}

func TestAPIv3_GiveItem_HappyPath(t *testing.T) {
	cat := loadScript(t, "gi", `give_item(99, "potion_healing")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	type call struct {
		Target   int64
		External string
	}
	var seen call
	bindings := APIBindings{
		GiveItem: func(target int64, ext string) error {
			seen = call{Target: target, External: ext}
			return nil
		},
	}
	if err := r.Run(context.Background(), "gi", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen.Target != 99 || seen.External != "potion_healing" {
		t.Fatalf("hook args: %+v", seen)
	}
}

func TestAPIv3_GiveItem_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "gi_nil", `give_item(1, "x")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "gi_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "give_item not bound") {
		t.Fatalf("err = %v; want 'give_item not bound'", err)
	}
}

func TestAPIv3_TargetHP_ReturnsBothValues(t *testing.T) {
	cat := loadScript(t, "th", `
local cur, max = target.hp(7)
if cur ~= 12 then error("cur=" .. tostring(cur)) end
if max ~= 30 then error("max=" .. tostring(max)) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		TargetHP: func(id int64) (int32, int32, error) {
			if id != 7 {
				t.Errorf("hook id = %d, want 7", id)
			}
			return 12, 30, nil
		},
	}
	if err := r.Run(context.Background(), "th", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv3_TargetHP_HookErrorSurfaces(t *testing.T) {
	cat := loadScript(t, "th_err", `target.hp(99)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		TargetHP: func(int64) (int32, int32, error) {
			return 0, 0, errors.New("character not found")
		},
	}
	err := r.Run(context.Background(), "th_err", func(l *gluua.LState) { bindings.Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "character not found") {
		t.Fatalf("err = %v; want hook message in chain", err)
	}
}

func TestAPIv3_TargetHP_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "th_nil", `target.hp(1)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "th_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "target.hp not bound") {
		t.Fatalf("err = %v; want 'target.hp not bound'", err)
	}
}

func TestAPIv3_TargetLevel_HappyPath(t *testing.T) {
	cat := loadScript(t, "tl", `
local lvl = target.level(7)
if lvl ~= 5 then error("lvl=" .. tostring(lvl)) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		TargetLevel: func(id int64) (int, error) { return 5, nil },
	}
	if err := r.Run(context.Background(), "tl", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv3_TargetLevel_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "tl_nil", `target.level(1)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "tl_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "target.level not bound") {
		t.Fatalf("err = %v; want 'target.level not bound'", err)
	}
}

// Successive calls on the same Runner must observe wiped V3
// globals. Mirrors TestAPIv2_ReleaseClearsV2Globals — bundle two
// scripts in one catalog, run the bound one >= poolSize times,
// then run the leak probe >= poolSize times to cycle every pooled
// LState through the bound→released transition.
func TestAPIv3_ReleaseClearsV3Globals(t *testing.T) {
	body := func(name, src string) (string, []byte) { return name + ".lua", []byte(src) }
	n1, b1 := body("with_v3", `apply_affect(1, "x"); give_item(1, "y"); local _,_ = target.hp(1); local _ = target.level(1)`)
	n2, b2 := body("leakprobe_v3", `
if type(apply_affect) ~= "nil" then error("apply_affect should be nil, got " .. type(apply_affect)) end
if type(give_item) ~= "nil" then error("give_item should be nil, got " .. type(give_item)) end
if type(target) ~= "nil" then error("target should be nil, got " .. type(target)) end
`)
	parser := gluua.NewState()
	defer parser.Close()
	cat := loadCatalogMulti(t, parser, map[string][]byte{n1: b1, n2: b2})

	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		ApplyAffect: func(int64, string, int32) error { return nil },
		GiveItem:    func(int64, string) error { return nil },
		TargetHP:    func(int64) (int32, int32, error) { return 1, 2, nil },
		TargetLevel: func(int64) (int, error) { return 1, nil },
	}
	bind := func(l *gluua.LState) { bindings.Bind(l) }
	for i := 0; i < poolSize; i++ {
		if err := r.Run(context.Background(), "with_v3", bind); err != nil {
			t.Fatalf("with_v3 Run #%d: %v", i, err)
		}
	}
	for i := 0; i < poolSize; i++ {
		if err := r.Run(context.Background(), "leakprobe_v3", nil); err != nil {
			t.Fatalf("leakprobe_v3 Run #%d (release should have wiped): %v", i, err)
		}
	}
}
