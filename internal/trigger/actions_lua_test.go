package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	gluua "github.com/yuin/gopher-lua"

	intlua "github.com/Jasrags/WheelMUD/internal/lua"
	"github.com/Jasrags/WheelMUD/internal/scripts"
)

// loadCatalog compiles src as one script named name and wraps it in
// a *scripts.Catalog. Mirrors internal/lua/sandbox_test.go's helper
// without the cross-package dep.
func loadCatalog(t *testing.T, name, src string) *scripts.Catalog {
	t.Helper()
	fsys := fstest.MapFS{name + ".lua": &fstest.MapFile{Data: []byte(src)}}
	parser := gluua.NewState()
	defer parser.Close()
	cat, err := scripts.Load(fsys, parser)
	if err != nil {
		t.Fatalf("scripts.Load: %v", err)
	}
	return cat
}

// TestLuaAction_QuestAccept_DispatchesToHook covers the happy path:
// a `lua` trigger fires a script that calls `quest.accept(...)`. The
// handler resolves the calling character from EventCtx.ActorID and
// invokes the wired hook with the right ids.
func TestLuaAction_QuestAccept_DispatchesToHook(t *testing.T) {
	cat := loadCatalog(t, "qa", `quest.accept("lost_lamb")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var seenChar int64
	var seenQuest string
	hooks := LuaQuestHooks{
		Accept: func(_ context.Context, charID int64, questID string) error {
			seenChar = charID
			seenQuest = questID
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	h := reg.Lookup("lua")
	if h == nil {
		t.Fatal("lua handler not registered")
	}
	payload, _ := json.Marshal(LuaPayload{Script: "qa"})
	err := h(context.Background(), ActionDeps{}, OwnerRef{Kind: OwnerMobTemplate, RoomID: 100},
		EventCtx{Event: EventOnEnter, ActorKind: "character", ActorID: 42}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if seenChar != 42 || seenQuest != "lost_lamb" {
		t.Fatalf("hook captured (%d, %q); want (42, lost_lamb)", seenChar, seenQuest)
	}
}

// A trigger event with no character actor (e.g. on_tick) MUST refuse
// the quest API and surface as a fault so the per-trigger budget
// catches the misuse and disables the row.
func TestLuaAction_QuestAccept_NoCharacterActor_Faults(t *testing.T) {
	cat := loadCatalog(t, "tick_advance", `quest.advance("x")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaQuestHooks{
		Advance: func(_ context.Context, _ int64, _ string) error {
			t.Fatal("hook should not be called for a non-character actor")
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "tick_advance"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{Kind: OwnerMobTemplate, RoomID: 1},
		EventCtx{Event: EventOnTick, BucketName: "phase"}, payload)
	if !errors.Is(err, ErrActionFaulted) {
		t.Fatalf("err = %v, want ErrActionFaulted", err)
	}
	if !strings.Contains(err.Error(), "character actor") {
		t.Fatalf("err = %v; want 'character actor' in chain", err)
	}
}

// nil hooks register Lua-side stubs that raise classified errors so
// authoring misuse trips the fault budget instead of silently no-
// opping. The script that calls a nil-bound API surfaces via
// ErrLuaError → ErrActionFaulted.
func TestLuaAction_QuestAdvance_NilHook_Faults(t *testing.T) {
	cat := loadCatalog(t, "qadv_nil", `quest.advance("x")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	reg := NewActionRegistry()
	// Empty hooks — both Accept/Advance unbound.
	RegisterLuaAction(reg, runner, cat, LuaQuestHooks{})

	payload, _ := json.Marshal(LuaPayload{Script: "qadv_nil"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{ActorKind: "character", ActorID: 7}, payload)
	if !errors.Is(err, ErrActionFaulted) {
		t.Fatalf("err = %v, want ErrActionFaulted", err)
	}
	if !strings.Contains(err.Error(), "quest.advance not bound") {
		t.Fatalf("err = %v; want 'quest.advance not bound' in chain", err)
	}
}

// Hook errors propagate as classified faults. The Lua-side closure
// raises with the error message; runner classifies as ErrLuaError;
// handler wraps as ErrActionFaulted.
func TestLuaAction_QuestAccept_HookError_Faults(t *testing.T) {
	cat := loadCatalog(t, "qa_err", `quest.accept("missing")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hookErr := errors.New("repo down")
	hooks := LuaQuestHooks{
		Accept: func(context.Context, int64, string) error { return hookErr },
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "qa_err"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{ActorKind: "character", ActorID: 5}, payload)
	if !errors.Is(err, ErrActionFaulted) {
		t.Fatalf("err = %v, want ErrActionFaulted", err)
	}
	if !strings.Contains(err.Error(), "repo down") {
		t.Fatalf("err = %v; want hook message in chain", err)
	}
}
