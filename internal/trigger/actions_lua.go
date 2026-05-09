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

// RegisterLuaAction installs the `lua` action kind on reg, wired to
// runner. The handler resolves the script name from the payload,
// invokes the runner with a per-call API binding, and wraps any
// classified Lua error in ErrActionFaulted so the trigger Runner's
// fault budget increments. Successful runs return nil so the
// counter resets.
//
// catalog is needed only for cleaner errors when a trigger names a
// missing script; the runner already checks the catalog itself.
func RegisterLuaAction(reg *ActionRegistry, runner *intlua.Runner, _ *scripts.Catalog) {
	if reg == nil || runner == nil {
		return
	}
	reg.Register("lua", luaActionHandler(runner))
}

func luaActionHandler(runner *intlua.Runner) ActionHandler {
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
			Logger:    loggerOr(deps),
			Ctx:       ctxViewFromEvent(ev),
			Broadcast: makeSayBroadcaster(ctx, deps, owner),
			EmoteBroadcast: makeEmoteBroadcaster(ctx, deps, owner),
		}
		bind := func(l *gluua.LState) { bindings.Bind(l) }

		err := runner.Run(ctx, p.Script, bind)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrActionFaulted, err)
		}
		return nil
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
