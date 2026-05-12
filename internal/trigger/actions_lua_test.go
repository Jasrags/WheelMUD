package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// V3 surface (Phase F #32 slice 3) — apply_affect / give_item /
// target.* dispatched through the trigger handler. Same fault
// budget contract as V2.

func TestLuaAction_ApplyAffect_DispatchesToHook(t *testing.T) {
	cat := loadCatalog(t, "aa", `apply_affect(42, "bull_strength")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var seenTarget int64
	var seenEffect string
	hooks := LuaHooks{
		ApplyAffect: func(_ context.Context, target int64, effect string, _ int32) error {
			seenTarget = target
			seenEffect = effect
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "aa"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{Kind: OwnerMobTemplate, RoomID: 100},
		EventCtx{Event: EventOnEnter, ActorKind: "character", ActorID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if seenTarget != 42 || seenEffect != "bull_strength" {
		t.Fatalf("hook captured (%d, %q); want (42, bull_strength)", seenTarget, seenEffect)
	}
}

func TestLuaAction_ApplyAffect_NoCharacterActor_Faults(t *testing.T) {
	cat := loadCatalog(t, "aa_tick", `apply_affect(1, "x")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaHooks{
		ApplyAffect: func(context.Context, int64, string, int32) error {
			t.Fatal("hook should not fire for non-character actor")
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "aa_tick"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{Event: EventOnTick, BucketName: "phase"}, payload)
	if !errors.Is(err, ErrActionFaulted) {
		t.Fatalf("err = %v, want ErrActionFaulted", err)
	}
	if !strings.Contains(err.Error(), "apply_affect requires a character actor") {
		t.Fatalf("err = %v; want 'character actor' in chain", err)
	}
}

func TestLuaAction_GiveItem_DispatchesToHook(t *testing.T) {
	cat := loadCatalog(t, "gi", `give_item(7, "tr.potion")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var seenTarget int64
	var seenExt string
	hooks := LuaHooks{
		GiveItem: func(_ context.Context, target int64, ext string) error {
			seenTarget = target
			seenExt = ext
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "gi"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{ActorKind: "character", ActorID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if seenTarget != 7 || seenExt != "tr.potion" {
		t.Fatalf("hook captured (%d, %q); want (7, tr.potion)", seenTarget, seenExt)
	}
}

func TestLuaAction_GiveItem_PerInvocationCap(t *testing.T) {
	// Script calls give_item MaxGiveItemsPerInvocation+1 times in a
	// loop. The first MaxGiveItemsPerInvocation succeed; the next
	// one raises a Lua error that classifies as ErrActionFaulted.
	src := `for i=1,` + fmt.Sprintf("%d", MaxGiveItemsPerInvocation+1) + ` do give_item(1, "tr.potion") end`
	cat := loadCatalog(t, "gi_cap", src)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	calls := 0
	hooks := LuaHooks{
		GiveItem: func(context.Context, int64, string) error {
			calls++
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "gi_cap"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{ActorKind: "character", ActorID: 1}, payload)
	if !errors.Is(err, ErrActionFaulted) ||
		!strings.Contains(err.Error(), "exceeded per-invocation cap") {
		t.Fatalf("err = %v; want cap-exceeded fault", err)
	}
	if calls != MaxGiveItemsPerInvocation {
		t.Fatalf("hook should have been invoked exactly %d times before cap; got %d",
			MaxGiveItemsPerInvocation, calls)
	}
}

func TestLuaAction_GiveItem_CapResetsBetweenInvocations(t *testing.T) {
	// Two separate script invocations each get a fresh budget.
	cat := loadCatalog(t, "gi_one", `give_item(1, "tr.potion")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	calls := 0
	hooks := LuaHooks{
		GiveItem: func(context.Context, int64, string) error {
			calls++
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "gi_one"})
	for i := 0; i < MaxGiveItemsPerInvocation+5; i++ {
		err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
			EventCtx{ActorKind: "character", ActorID: 1}, payload)
		if err != nil {
			t.Fatalf("invocation %d failed: %v", i, err)
		}
	}
	if calls != MaxGiveItemsPerInvocation+5 {
		t.Fatalf("each invocation should reset cap; got %d hook calls, want %d",
			calls, MaxGiveItemsPerInvocation+5)
	}
}

func TestLuaAction_GiveItem_NoCharacterActor_Faults(t *testing.T) {
	cat := loadCatalog(t, "gi_tick", `give_item(1, "x")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaHooks{
		GiveItem: func(context.Context, int64, string) error {
			t.Fatal("hook should not fire for non-character actor")
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "gi_tick"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{Event: EventOnTick, BucketName: "phase"}, payload)
	if !errors.Is(err, ErrActionFaulted) ||
		!strings.Contains(err.Error(), "give_item requires a character actor") {
		t.Fatalf("err = %v; want give_item character-actor refusal", err)
	}
}

func TestLuaAction_TargetReads_NoActorGuard(t *testing.T) {
	// Read APIs intentionally have no actor-kind guard — a script
	// firing from on_tick can still query a character's HP/level.
	cat := loadCatalog(t, "th_tick", `local cur, max = target.hp(7); local lvl = target.level(7)`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaHooks{
		TargetHP:    func(context.Context, int64) (int32, int32, error) { return 1, 2, nil },
		TargetLevel: func(context.Context, int64) (int, error) { return 3, nil },
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "th_tick"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{Event: EventOnTick, BucketName: "phase"}, payload)
	if err != nil {
		t.Fatalf("read APIs should not be guarded; got %v", err)
	}
}

// V4 surface (Phase F #32 slice 4) — room.players / room.mobs /
// clock / target.classes / apply_affect duration-override
// dispatched through the trigger handler.

func TestLuaAction_RoomPlayers_ResolvesRoomFromEvent(t *testing.T) {
	cat := loadCatalog(t, "rp", `
local players = room.players()
if #players ~= 1 or players[1] ~= 99 then error("ids") end
`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var seenRoom int64
	hooks := LuaHooks{
		RoomPlayers: func(_ context.Context, roomID int64) ([]int64, error) {
			seenRoom = roomID
			return []int64{99}, nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "rp"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{Event: EventOnEnter, RoomID: 555, ActorKind: "character", ActorID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if seenRoom != 555 {
		t.Fatalf("hook should resolve roomID from EventCtx; got %d, want 555", seenRoom)
	}
}

func TestLuaAction_RoomPlayers_ZeroRoomFaults(t *testing.T) {
	// Regression: mob-template-owned on_tick triggers leave
	// EventCtx.RoomID == 0. Without the guard, room.players()
	// would silently return an empty table; the guard surfaces
	// a classified fault so authors see the misconfiguration.
	cat := loadCatalog(t, "rp_zero", `room.players()`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaHooks{
		RoomPlayers: func(context.Context, int64) ([]int64, error) {
			t.Fatal("hook should not fire when ev.RoomID is 0")
			return nil, nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "rp_zero"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{Event: EventOnTick, BucketName: "phase"}, payload)
	if !errors.Is(err, ErrActionFaulted) ||
		!strings.Contains(err.Error(), "room.players requires a room context") {
		t.Fatalf("err = %v; want room-context refusal", err)
	}
}

func TestLuaAction_RoomMobs_ZeroRoomFaults(t *testing.T) {
	cat := loadCatalog(t, "rm_zero", `room.mobs()`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaHooks{
		RoomMobs: func(context.Context, int64) ([]int64, error) {
			t.Fatal("hook should not fire when ev.RoomID is 0")
			return nil, nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "rm_zero"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{Event: EventOnTick, BucketName: "phase"}, payload)
	if !errors.Is(err, ErrActionFaulted) ||
		!strings.Contains(err.Error(), "room.mobs requires a room context") {
		t.Fatalf("err = %v; want room-context refusal", err)
	}
}

func TestLuaAction_RoomMobs_ResolvesRoomFromEvent(t *testing.T) {
	cat := loadCatalog(t, "rm", `local m = room.mobs(); if #m ~= 1 or m[1] ~= 7 then error("ids") end`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var seenRoom int64
	hooks := LuaHooks{
		RoomMobs: func(_ context.Context, roomID int64) ([]int64, error) {
			seenRoom = roomID
			return []int64{7}, nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "rm"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{RoomID: 200, ActorKind: "character", ActorID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if seenRoom != 200 {
		t.Fatalf("hook should resolve roomID from EventCtx; got %d, want 200", seenRoom)
	}
}

func TestLuaAction_Clock_DispatchesToHooks(t *testing.T) {
	cat := loadCatalog(t, "ck", `
if clock.hour() ~= 17 then error("hour") end
if clock.day() ~= 9 then error("day") end
`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaHooks{
		ClockHour: func() int { return 17 },
		ClockDay:  func() int64 { return 9 },
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "ck"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{ActorKind: "character", ActorID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func TestLuaAction_TargetClasses_NoActorGuard(t *testing.T) {
	// Read APIs unguarded — fires on on_tick (no character actor).
	cat := loadCatalog(t, "tc_tick", `
local m = target.classes(7)
if m.warrior ~= 5 then error("warrior") end
`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaHooks{
		TargetClasses: func(context.Context, int64) (map[string]int, error) {
			return map[string]int{"warrior": 5}, nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "tc_tick"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{Event: EventOnTick, BucketName: "phase"}, payload)
	if err != nil {
		t.Fatalf("read API should not be guarded; got %v", err)
	}
}

func TestLuaAction_ApplyAffect_DurationOverridePropagates(t *testing.T) {
	cat := loadCatalog(t, "aa_dur", `apply_affect(7, "weak_poison", 50)`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var seenDur int32
	hooks := LuaHooks{
		ApplyAffect: func(_ context.Context, _ int64, _ string, dur int32) error {
			seenDur = dur
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)

	payload, _ := json.Marshal(LuaPayload{Script: "aa_dur"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{ActorKind: "character", ActorID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if seenDur != 50 {
		t.Fatalf("duration override not propagated: got %d, want 50", seenDur)
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

// Phase F #32 slice 5a — V5a combat + inventory mutation surface
// dispatches through the trigger lua action with no actor-kind guard
// (unlike quest / apply_affect / give_item). The DropItem adapter
// resolves the target room from EventCtx, so a mob-template on_tick
// with RoomID==0 refuses; happy paths confirm the hook closures
// receive the correct args from Lua.

func TestLuaAction_DealDamage_DispatchesToHook(t *testing.T) {
	cat := loadCatalog(t, "dd", `deal_damage(7, 11, "fire_trap")`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	type call struct {
		Target int64
		Amount int32
		Source string
	}
	var seen call
	hooks := LuaHooks{
		DealDamage: func(_ context.Context, target int64, amount int32, source string) error {
			seen = call{Target: target, Amount: amount, Source: source}
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)
	payload, _ := json.Marshal(LuaPayload{Script: "dd"})
	// Mob actor on a player-targeted damage call is legitimate (no
	// actor-kind guard for V5a mutations).
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{Kind: OwnerMobTemplate, RoomID: 50},
		EventCtx{Event: EventOnAttack, ActorKind: "mob", ActorID: 99, RoomID: 50}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if seen.Target != 7 || seen.Amount != 11 || seen.Source != "fire_trap" {
		t.Fatalf("hook args: %+v", seen)
	}
}

func TestLuaAction_Heal_DispatchesToHook(t *testing.T) {
	cat := loadCatalog(t, "hl", `heal(42, 8)`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var sawTarget int64
	var sawAmount int32
	hooks := LuaHooks{
		Heal: func(_ context.Context, target int64, amount int32) error {
			sawTarget = target
			sawAmount = amount
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)
	payload, _ := json.Marshal(LuaPayload{Script: "hl"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{ActorKind: "character", ActorID: 1, RoomID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if sawTarget != 42 || sawAmount != 8 {
		t.Fatalf("hook args: target=%d amount=%d", sawTarget, sawAmount)
	}
}

func TestLuaAction_TransferItem_DispatchesToHook(t *testing.T) {
	cat := loadCatalog(t, "ti", `transfer_item(101, 7)`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var sawItem, sawTo int64
	hooks := LuaHooks{
		TransferItem: func(_ context.Context, item, to int64) error {
			sawItem = item
			sawTo = to
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)
	payload, _ := json.Marshal(LuaPayload{Script: "ti"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{ActorKind: "character", ActorID: 1, RoomID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if sawItem != 101 || sawTo != 7 {
		t.Fatalf("hook args: item=%d to=%d", sawItem, sawTo)
	}
}

func TestLuaAction_DropItem_ResolvesRoomFromEvent(t *testing.T) {
	cat := loadCatalog(t, "di", `drop_item(55)`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	var sawItem, sawRoom int64
	hooks := LuaHooks{
		DropItem: func(_ context.Context, item, room int64) error {
			sawItem = item
			sawRoom = room
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)
	payload, _ := json.Marshal(LuaPayload{Script: "di"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{RoomID: 300, ActorKind: "character", ActorID: 1}, payload)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if sawItem != 55 || sawRoom != 300 {
		t.Fatalf("hook args: item=%d room=%d (want 55, 300)", sawItem, sawRoom)
	}
}

func TestLuaAction_DropItem_ZeroRoomFaults(t *testing.T) {
	cat := loadCatalog(t, "di_zero", `drop_item(1)`)
	runner := intlua.NewRunner(cat, nil)
	defer runner.Stop()

	hooks := LuaHooks{
		DropItem: func(context.Context, int64, int64) error {
			t.Fatal("hook should not fire when ev.RoomID is 0")
			return nil
		},
	}
	reg := NewActionRegistry()
	RegisterLuaAction(reg, runner, cat, hooks)
	payload, _ := json.Marshal(LuaPayload{Script: "di_zero"})
	err := reg.Lookup("lua")(context.Background(), ActionDeps{}, OwnerRef{},
		EventCtx{Event: EventOnTick, BucketName: "phase"}, payload)
	if !errors.Is(err, ErrActionFaulted) ||
		!strings.Contains(err.Error(), "drop_item requires a room context") {
		t.Fatalf("err = %v; want room-context refusal", err)
	}
}
