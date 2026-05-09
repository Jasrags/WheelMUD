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

// LuaQuestHooks bundles the optional V2 mutation closures the cmd-
// layer wires when a trigger script needs to drive quest state. nil
// hooks register the corresponding Lua-side stub that raises a
// classified error — the trigger fault budget catches misuse and
// auto-disables the offending row after FaultThreshold strikes.
//
// The closures resolve the calling character from the EventCtx
// passed to each invocation; that's why they take a CharacterID
// rather than capturing one. The handler refuses to fire a quest
// API if ev.ActorKind != "character" so a misconfigured `on_tick`
// trigger doesn't silently no-op a quest.advance call.
type LuaQuestHooks struct {
	Accept  func(ctx context.Context, charID int64, questID string) error
	Advance func(ctx context.Context, charID int64, questID string) error
}

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
func RegisterLuaAction(reg *ActionRegistry, runner *intlua.Runner, _ *scripts.Catalog, hooks LuaQuestHooks) {
	if reg == nil || runner == nil {
		return
	}
	reg.Register("lua", luaActionHandler(runner, hooks))
}

func luaActionHandler(runner *intlua.Runner, hooks LuaQuestHooks) ActionHandler {
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
