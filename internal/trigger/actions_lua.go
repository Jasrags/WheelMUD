package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	gluua "github.com/yuin/gopher-lua"

	intlua "github.com/Jasrags/WheelMUD/internal/lua"
	"github.com/Jasrags/WheelMUD/internal/scripts"
	"github.com/Jasrags/WheelMUD/telnet"
)

// LuaPayload is the on-disk JSON shape for a `lua` trigger action.
// Phase F #32 slice 1: only Script (catalog name) is consumed.
// Future slices may add per-trigger options without breaking
// existing rows.
type LuaPayload struct {
	Script string `json:"script"`
}

// LuaHooks bundles the optional mutation + read closures the
// cmd-layer wires when a trigger script needs to drive quest state
// (V2, slice 2) or compose the producer pipelines (V3, slice 3 —
// apply_affect / give_item / target reads). nil hooks register the
// corresponding Lua-side stub that raises a classified error — the
// trigger fault budget catches misuse and auto-disables the
// offending row after FaultThreshold strikes.
//
// The closures resolve the calling character from the EventCtx
// passed to each invocation; that's why they take a CharacterID
// rather than capturing one. The handler refuses to fire a
// character-bound API if ev.ActorKind != "character" so a
// misconfigured `on_tick` trigger doesn't silently no-op a
// quest.advance call or stash an item on a mob.
type LuaHooks struct {
	// V2 (Phase F #32 slice 2)
	Accept  func(ctx context.Context, charID int64, questID string) error
	Advance func(ctx context.Context, charID int64, questID string) error

	// V3 (Phase F #32 slice 3) — read APIs are character-bound, so
	// they take a targetID directly rather than a charID-from-ctx.
	// Mutations (ApplyAffect, GiveItem) are character-bound; the
	// handler still requires a character actor on the firing event.
	//
	// ApplyAffect's third arg (durationOverride) is a slice-4
	// addition; > 0 overrides the catalog's authored duration, 0
	// means "use catalog default".
	ApplyAffect func(ctx context.Context, targetID int64, effectID string, durationOverride int32) error
	GiveItem    func(ctx context.Context, targetID int64, externalID string) error
	TargetHP    func(ctx context.Context, targetID int64) (cur, max int32, err error)
	TargetLevel func(ctx context.Context, targetID int64) (int, error)

	// V4 (Phase F #32 slice 4) — world read APIs. Room hooks take
	// the roomID resolved from the firing EventCtx so a script
	// can't snoop on rooms it doesn't own. Clock hooks are pure
	// (no args). TargetClasses returns the multiclass map.
	RoomPlayers   func(ctx context.Context, roomID int64) ([]int64, error)
	RoomMobs      func(ctx context.Context, roomID int64) ([]int64, error)
	ClockHour     func() int
	ClockDay      func() int64
	TargetClasses func(ctx context.Context, targetID int64) (map[string]int, error)

	// V5a (Phase F #32 slice 5a) — combat + inventory mutations.
	// Unlike the V2/V3 character-bound mutation APIs, these have
	// NO actor-kind guard at the trigger layer: scripts firing from
	// mob-template triggers legitimately damage / heal players, and
	// a script dispensed by an `on_say` row legitimately confiscates
	// contraband. The cmd-layer closure resolves target kind by
	// trying CharacterRepo first then MobInstanceRepo, and any
	// failure surfaces as a classified Lua error that trips the
	// fault budget. DropItem takes a separate currentRoomID arg
	// resolved by the trigger-layer adapter from ev.RoomID; the
	// adapter refuses with a classified error when ev.RoomID == 0
	// (mob-template `on_tick`) so a context-less drop doesn't
	// dump items into room 0.
	DealDamage   func(ctx context.Context, targetID int64, amount int32, source string) error
	Heal         func(ctx context.Context, targetID int64, amount int32) error
	TransferItem func(ctx context.Context, itemID, toOwnerID int64) error
	DropItem     func(ctx context.Context, itemID, currentRoomID int64) error
}

// LuaQuestHooks is the legacy slice-2 alias. Kept as a type alias
// so existing call sites compile; new code should use LuaHooks
// directly.
type LuaQuestHooks = LuaHooks

// RegisterLuaAction installs the `lua` action kind on reg, wired to
// runner. The handler resolves the script name from the payload,
// invokes the runner with a per-call API binding, and wraps any
// classified Lua error in ErrActionFaulted so the trigger Runner's
// fault budget increments. Successful runs return nil so the
// counter resets.
//
// catalog is needed only for cleaner errors when a trigger names a
// missing script; the runner already checks the catalog itself.
//
// hooks supplies the V2 mutation closures (Phase F #32 slice 2).
// Pass an empty LuaQuestHooks to register the stubs that raise
// classified errors — handy for tests / boots that disable quest
// scripting.
func RegisterLuaAction(reg *ActionRegistry, runner *intlua.Runner, _ *scripts.Catalog, hooks LuaHooks) {
	if reg == nil || runner == nil {
		return
	}
	reg.Register("lua", luaActionHandler(runner, hooks))
}

func luaActionHandler(runner *intlua.Runner, hooks LuaHooks) ActionHandler {
	return func(ctx context.Context, deps ActionDeps, owner OwnerRef, ev EventCtx, payload json.RawMessage) error {
		var p LuaPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			// Authoring error — JSON shape is wrong. Treat as a
			// fault so the trigger auto-disables instead of
			// repeatedly faulting on every event.
			return fmt.Errorf("%w: lua payload decode: %v", ErrActionFaulted, err)
		}
		if strings.TrimSpace(p.Script) == "" {
			return fmt.Errorf("%w: lua payload missing script name", ErrActionFaulted)
		}

		bindings := intlua.APIBindings{
			Logger:         loggerOr(deps),
			Ctx:            ctxViewFromEvent(ev),
			Broadcast:      makeSayBroadcaster(ctx, deps, owner),
			EmoteBroadcast: makeEmoteBroadcaster(ctx, deps, owner),
			QuestAccept:    makeQuestHook(ctx, ev, "quest.accept", hooks.Accept),
			QuestAdvance:   makeQuestHook(ctx, ev, "quest.advance", hooks.Advance),
			ApplyAffect:    makeApplyAffectHook(ctx, ev, hooks.ApplyAffect),
			GiveItem:       makeGiveItemHook(ctx, ev, hooks.GiveItem),
			TargetHP:       makeTargetHPHook(ctx, hooks.TargetHP),
			TargetLevel:    makeTargetLevelHook(ctx, hooks.TargetLevel),
			TargetClasses:  makeTargetClassesHook(ctx, hooks.TargetClasses),
			RoomPlayers:    makeRoomPlayersHook(ctx, ev, hooks.RoomPlayers),
			RoomMobs:       makeRoomMobsHook(ctx, ev, hooks.RoomMobs),
			ClockHour:      hooks.ClockHour,
			ClockDay:       hooks.ClockDay,
			DealDamage:     makeDealDamageHook(ctx, hooks.DealDamage),
			Heal:           makeHealHook(ctx, hooks.Heal),
			TransferItem:   makeTransferItemHook(ctx, hooks.TransferItem),
			DropItem:       makeDropItemHook(ctx, ev, hooks.DropItem),
		}
		// PushMode stays unbound on triggers — there's no surrounding
		// session to push a mode onto. The classified Lua error is
		// the right outcome for misuse.
		bind := func(l *gluua.LState) { bindings.Bind(l) }

		err := runner.Run(ctx, p.Script, bind)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrActionFaulted, err)
		}
		return nil
	}
}

// makeQuestHook adapts a (charID, questID) hook onto the Lua-side
// (questID) closure. The character id comes from the trigger's
// EventCtx; an event with no character actor (e.g. on_tick) returns
// a closure that errors with a classified message so misuse trips
// the fault budget instead of silently mutating no one's state.
func makeQuestHook(ctx context.Context, ev EventCtx, name string, hook func(context.Context, int64, string) error) func(string) error {
	if hook == nil {
		return nil // bind-side stub will surface "<name> not bound"
	}
	return func(questID string) error {
		if ev.ActorKind != "character" || ev.ActorID == 0 {
			return fmt.Errorf("%s requires a character actor (got %q)", name, ev.ActorKind)
		}
		return hook(ctx, ev.ActorID, questID)
	}
}

// makeApplyAffectHook adapts a (ctx, charID, effectID, durationOverride)
// hook onto the Lua-side (targetID, effectID, durationOverride) closure.
// Mob-fired triggers (no character actor) get a closure that errors
// with a classified message so misuse trips the fault budget. The
// targetID arg is taken from Lua, NOT from the EventCtx — scripts
// can apply affects to any character, not just the actor — but a
// non-character actor still indicates a misconfigured trigger and is
// refused. durationOverride > 0 overrides the catalog's authored
// duration; 0 means "use catalog default".
func makeApplyAffectHook(ctx context.Context, ev EventCtx, hook func(context.Context, int64, string, int32) error) func(int64, string, int32) error {
	if hook == nil {
		return nil
	}
	return func(targetID int64, effectID string, durationOverride int32) error {
		if ev.ActorKind != "character" || ev.ActorID == 0 {
			return fmt.Errorf("apply_affect requires a character actor (got %q)", ev.ActorKind)
		}
		return hook(ctx, targetID, effectID, durationOverride)
	}
}

// MaxGiveItemsPerInvocation caps the number of items a single
// script invocation can spawn via give_item. Without a cap, a tight
// loop could spawn dozens of items inside the 50ms ctx timeout.
// The wrapper closure resets the counter implicitly on each new
// trigger fire (a fresh closure is built per APIBindings construction).
const MaxGiveItemsPerInvocation = 8

// makeGiveItemHook mirrors makeApplyAffectHook for give_item, with
// a per-invocation spawn cap (MaxGiveItemsPerInvocation). The cap
// counter is captured by the returned closure, so each trigger
// fire gets its own fresh budget.
func makeGiveItemHook(ctx context.Context, ev EventCtx, hook func(context.Context, int64, string) error) func(int64, string) error {
	if hook == nil {
		return nil
	}
	count := 0
	return func(targetID int64, externalID string) error {
		if ev.ActorKind != "character" || ev.ActorID == 0 {
			return fmt.Errorf("give_item requires a character actor (got %q)", ev.ActorKind)
		}
		count++
		if count > MaxGiveItemsPerInvocation {
			return fmt.Errorf("give_item exceeded per-invocation cap of %d", MaxGiveItemsPerInvocation)
		}
		return hook(ctx, targetID, externalID)
	}
}

// makeTargetHPHook + makeTargetLevelHook are read-only — no
// actor-kind guard needed; a script firing from any owner can
// query a character's HP / level.
func makeTargetHPHook(ctx context.Context, hook func(context.Context, int64) (int32, int32, error)) func(int64) (int32, int32, error) {
	if hook == nil {
		return nil
	}
	return func(targetID int64) (int32, int32, error) {
		return hook(ctx, targetID)
	}
}

func makeTargetLevelHook(ctx context.Context, hook func(context.Context, int64) (int, error)) func(int64) (int, error) {
	if hook == nil {
		return nil
	}
	return func(targetID int64) (int, error) {
		return hook(ctx, targetID)
	}
}

// V4 adapters (Phase F #32 slice 4).
//
// makeTargetClassesHook is read-only — no actor-kind guard. Mirrors
// makeTargetHPHook / makeTargetLevelHook.
func makeTargetClassesHook(ctx context.Context, hook func(context.Context, int64) (map[string]int, error)) func(int64) (map[string]int, error) {
	if hook == nil {
		return nil
	}
	return func(targetID int64) (map[string]int, error) {
		return hook(ctx, targetID)
	}
}

// makeRoomPlayersHook resolves the room from the firing EventCtx,
// NOT from a Lua-side argument, so scripts can't snoop on rooms
// they don't own. Read-only — no actor-kind guard. The Lua-side
// `room.players()` call is no-arg in the V4 surface.
//
// Refuses with a classified error when ev.RoomID is zero — this
// happens for mob-template-owned `on_tick` triggers where the
// dispatcher leaves owner.RoomID unset. Without the guard, the
// hook would silently filter for sRoom==0 and return an always-
// empty table; the explicit error trips the trigger fault budget
// so a misconfigured author sees the failure instead of debugging
// a "list is always empty" mystery.
func makeRoomPlayersHook(ctx context.Context, ev EventCtx, hook func(context.Context, int64) ([]int64, error)) func() ([]int64, error) {
	if hook == nil {
		return nil
	}
	return func() ([]int64, error) {
		if ev.RoomID == 0 {
			return nil, fmt.Errorf("room.players requires a room context (event %q has no room)", ev.Event)
		}
		return hook(ctx, ev.RoomID)
	}
}

func makeRoomMobsHook(ctx context.Context, ev EventCtx, hook func(context.Context, int64) ([]int64, error)) func() ([]int64, error) {
	if hook == nil {
		return nil
	}
	return func() ([]int64, error) {
		if ev.RoomID == 0 {
			return nil, fmt.Errorf("room.mobs requires a room context (event %q has no room)", ev.Event)
		}
		return hook(ctx, ev.RoomID)
	}
}

// V5a adapters (Phase F #32 slice 5a).
//
// makeDealDamageHook / makeHealHook / makeTransferItemHook have no
// actor-kind guard — the trigger layer trusts content authors to
// scope script effects appropriately. makeDropItemHook does have a
// room-context guard: the drop target is "the room the trigger fired
// in", so a mob-template `on_tick` (RoomID == 0) refuses with a
// classified error rather than silently dropping into room 0.
func makeDealDamageHook(ctx context.Context, hook func(context.Context, int64, int32, string) error) func(int64, int32, string) error {
	if hook == nil {
		return nil
	}
	return func(targetID int64, amount int32, source string) error {
		return hook(ctx, targetID, amount, source)
	}
}

func makeHealHook(ctx context.Context, hook func(context.Context, int64, int32) error) func(int64, int32) error {
	if hook == nil {
		return nil
	}
	return func(targetID int64, amount int32) error {
		return hook(ctx, targetID, amount)
	}
}

func makeTransferItemHook(ctx context.Context, hook func(context.Context, int64, int64) error) func(int64, int64) error {
	if hook == nil {
		return nil
	}
	return func(itemID, toOwnerID int64) error {
		return hook(ctx, itemID, toOwnerID)
	}
}

// makeDropItemHook resolves the room from the firing EventCtx, NOT
// from a Lua-side argument. Mirrors makeRoomPlayersHook's posture:
// scripts can only drop into the room they own. ev.RoomID == 0 (mob-
// template `on_tick`) refuses with a classified error.
func makeDropItemHook(ctx context.Context, ev EventCtx, hook func(context.Context, int64, int64) error) func(int64) error {
	if hook == nil {
		return nil
	}
	return func(itemID int64) error {
		if ev.RoomID == 0 {
			return fmt.Errorf("drop_item requires a room context (event %q has no room)", ev.Event)
		}
		return hook(ctx, itemID, ev.RoomID)
	}
}

// ctxViewFromEvent maps the trigger EventCtx onto the read-only
// Lua `ctx` table. Numeric zero / empty string fields stay zero —
// the script observes them as `0` / `""` rather than nil.
func ctxViewFromEvent(ev EventCtx) intlua.CtxView {
	return intlua.CtxView{
		Event:      string(ev.Event),
		RoomID:     ev.RoomID,
		ActorID:    ev.ActorID,
		ActorKind:  string(ev.ActorKind),
		TargetID:   ev.TargetID,
		TargetKind: string(ev.TargetKind),
		Text:       ev.Text,
		Bucket:     ev.BucketName,
	}
}

// makeSayBroadcaster returns a closure the Lua API binder calls when
// the script invokes `say(text)`. The closure captures the trigger
// owner so the rendered line names the right speaker, and routes
// through broadcastToRoom — same path SayAction uses, so the cross-
// session output rule (WriteAsync) is honored automatically.
func makeSayBroadcaster(ctx context.Context, deps ActionDeps, owner OwnerRef) func(string) {
	return func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		speaker := resolveSpeaker(ctx, deps, owner)
		line := fmt.Sprintf("{{%s says,}}::cyan \"{{%s}}::white\"\r\n",
			defangCfmt(speaker), defangCfmt(text))
		broadcastToRoom(deps, owner.RoomID, line)
	}
}

func makeEmoteBroadcaster(ctx context.Context, deps ActionDeps, owner OwnerRef) func(string) {
	return func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		speaker := resolveSpeaker(ctx, deps, owner)
		line := fmt.Sprintf("{{%s %s}}::cyan\r\n",
			defangCfmt(speaker), defangCfmt(text))
		broadcastToRoom(deps, owner.RoomID, line)
	}
}

// compile-time guard: the telnet package is imported transitively
// via broadcastToRoom; we keep an explicit reference here so future
// refactors don't accidentally drop the import.
var _ = telnet.AuthGuest
