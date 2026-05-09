package lua

import (
	"log/slog"

	gluua "github.com/yuin/gopher-lua"
)

// CtxView is the read-only event context every script sees as the
// global `ctx` table. The caller (trigger Lua action handler in
// Slice 1) fills this from the trigger's EventCtx; later slices
// (dialogue / quest) may extend the shape but the existing fields
// stay stable so authored scripts don't break across slices.
//
// All fields are optional: a script that reads `ctx.text` on an
// `on_enter` event correctly observes the empty string instead of
// a nil-deref. Numeric zeros likewise read back as `0`.
type CtxView struct {
	Event      string
	RoomID     int64
	ActorID    int64
	ActorKind  string
	TargetID   int64
	TargetKind string
	Text       string
	Bucket     string
}

// APIBindings is the per-call bag the runner consumer fills before
// invoking Runner.Run. The Bind method registers each non-nil field
// as a Lua global. nil fields are skipped (the script that calls
// the missing function gets a Lua "attempt to call nil" error,
// which surfaces as ErrLuaError + a fault — by design, since a
// missing API in a context that doesn't support it is a content
// authoring bug).
//
// Broadcast / EmoteBroadcast take the script-supplied text string.
// The caller's closure decides how to format and route — the trigger
// handler routes through broadcastToRoom + resolveSpeaker, so the
// cross-package import graph stays clean (internal/lua has no
// dependency on internal/trigger or internal/repo).
type APIBindings struct {
	Broadcast      func(text string)
	EmoteBroadcast func(text string)
	Logger         *slog.Logger
	Ctx            CtxView
}

// Bind registers the V1 API globals on l. Call this from the bind
// callback passed to Runner.Run. The runner's release path clears
// `say` / `emote` / `log` / `ctx` between borrows so per-call state
// doesn't leak.
func (b APIBindings) Bind(l *gluua.LState) {
	if b.Broadcast != nil {
		l.SetGlobal("say", l.NewFunction(func(L *gluua.LState) int {
			text := L.CheckString(1)
			b.Broadcast(text)
			return 0
		}))
	}
	if b.EmoteBroadcast != nil {
		l.SetGlobal("emote", l.NewFunction(func(L *gluua.LState) int {
			text := L.CheckString(1)
			b.EmoteBroadcast(text)
			return 0
		}))
	}
	logger := b.Logger
	if logger == nil {
		logger = slog.Default()
	}
	l.SetGlobal("log", l.NewFunction(func(L *gluua.LState) int {
		level := L.CheckString(1)
		msg := L.CheckString(2)
		switch level {
		case "debug":
			logger.Debug(msg, "source", "lua")
		case "warn":
			logger.Warn(msg, "source", "lua")
		case "error":
			logger.Error(msg, "source", "lua")
		default:
			logger.Info(msg, "source", "lua")
		}
		return 0
	}))
	l.SetGlobal("ctx", buildCtxTable(l, b.Ctx))
}

// buildCtxTable materializes the read-only ctx table. We don't use
// metatables to enforce read-only-ness for V1 — gopher-lua tables
// are mutable, but the runner clears `ctx` between calls so a
// script that overwrites a field can't leak state to the next
// invocation. If we observe scripts mutating ctx in practice we'll
// add a metatable __newindex that errors.
func buildCtxTable(l *gluua.LState, c CtxView) gluua.LValue {
	t := l.NewTable()
	t.RawSetString("event", gluua.LString(c.Event))
	t.RawSetString("room_id", gluua.LNumber(c.RoomID))
	t.RawSetString("actor_id", gluua.LNumber(c.ActorID))
	t.RawSetString("actor_kind", gluua.LString(c.ActorKind))
	t.RawSetString("target_id", gluua.LNumber(c.TargetID))
	t.RawSetString("target_kind", gluua.LString(c.TargetKind))
	t.RawSetString("text", gluua.LString(c.Text))
	t.RawSetString("bucket", gluua.LString(c.Bucket))
	return t
}
