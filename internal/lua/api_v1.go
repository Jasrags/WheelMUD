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
// as a Lua global (or table entry for namespaced APIs). nil function
// fields register a stub that raises a classified Lua error so the
// fault budget increments — that is, every API the script *might*
// call is always callable; the binding decides whether the call is
// honored or refused with a meaningful message.
//
// Broadcast / EmoteBroadcast take the script-supplied text string.
// The caller's closure decides how to format and route — the trigger
// handler routes through broadcastToRoom + resolveSpeaker, so the
// cross-package import graph stays clean (internal/lua has no
// dependency on internal/trigger or internal/repo).
//
// V2 mutation surface (Slice 2 of Phase F #32):
//
//   - QuestAccept / QuestAdvance enroll-or-advance the calling
//     character in a quest by id. Closures resolve the character id
//     from the surrounding context (dialogue session, trigger event
//     actor) so internal/lua stays oblivious to repo / engine types.
//   - PushMode hands off to a sibling telnet mode by name. nil
//     means "this context cannot push modes" and the bound stub
//     errors loudly.
type APIBindings struct {
	Broadcast      func(text string)
	EmoteBroadcast func(text string)
	Logger         *slog.Logger
	Ctx            CtxView

	QuestAccept  func(questID string) error
	QuestAdvance func(questID string) error
	PushMode     func(mode string) error

	// V3 surface (Phase F #32 slice 3) — content-author hooks that
	// compose existing producer pipelines. All take explicit
	// targetID / externalID args; the consumer-injection pattern
	// (cmd/server/main.go closures) bakes context.Context + repos.
	//
	// ApplyAffect's third arg (durationOverride int32) is a slice-4
	// addition: when > 0, overrides the catalog's authored
	// DurationTicks; 0 means "use catalog default" so existing
	// 2-arg Lua callers keep working.
	ApplyAffect func(targetID int64, effectID string, durationOverride int32) error
	GiveItem    func(targetID int64, externalID string) error
	TargetHP    func(targetID int64) (cur, max int32, err error)
	TargetLevel func(targetID int64) (int, error)

	// V4 surface (Phase F #32 slice 4) — read-only world queries.
	// Room hooks resolve their roomID from b.Ctx.RoomID at bind
	// time, NOT from a Lua-side argument, so scripts can't snoop
	// on rooms they don't own.
	RoomPlayers   func() ([]int64, error)
	RoomMobs      func() ([]int64, error)
	ClockHour     func() int
	ClockDay      func() int64
	TargetClasses func(targetID int64) (map[string]int, error)
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

	// V2 mutation surface. We always register the surface even when
	// hooks are nil — the stub errors with a classified message so
	// the trigger fault budget catches misuse. This is friendlier
	// than "attempt to call nil" and makes nil-bound contexts (e.g.
	// dialogue scripts that lack PushMode) self-describing in logs.
	questTbl := l.NewTable()
	questTbl.RawSetString("accept", l.NewFunction(makeQuestFn("quest.accept", b.QuestAccept)))
	questTbl.RawSetString("advance", l.NewFunction(makeQuestFn("quest.advance", b.QuestAdvance)))
	l.SetGlobal("quest", questTbl)

	l.SetGlobal("push_mode", l.NewFunction(func(L *gluua.LState) int {
		mode := L.CheckString(1)
		if b.PushMode == nil {
			L.RaiseError("push_mode not bound in this context")
			return 0
		}
		if err := b.PushMode(mode); err != nil {
			L.RaiseError("push_mode %q failed: %s", mode, err.Error())
			return 0
		}
		return 0
	}))

	// V3 surface (Phase F #32 slice 3).
	// Slice 4 added an optional 3rd arg (duration override). 0 (or
	// omitted) means "use the catalog's authored DurationTicks".
	l.SetGlobal("apply_affect", l.NewFunction(func(L *gluua.LState) int {
		targetID := L.CheckInt64(1)
		effectID := L.CheckString(2)
		durationOverride := int32(L.OptInt(3, 0))
		if b.ApplyAffect == nil {
			L.RaiseError("apply_affect not bound in this context")
			return 0
		}
		if err := b.ApplyAffect(targetID, effectID, durationOverride); err != nil {
			L.RaiseError("apply_affect(%d, %q, %d) failed: %s", targetID, effectID, durationOverride, err.Error())
			return 0
		}
		return 0
	}))

	l.SetGlobal("give_item", l.NewFunction(func(L *gluua.LState) int {
		targetID := L.CheckInt64(1)
		externalID := L.CheckString(2)
		if b.GiveItem == nil {
			L.RaiseError("give_item not bound in this context")
			return 0
		}
		if err := b.GiveItem(targetID, externalID); err != nil {
			L.RaiseError("give_item(%d, %q) failed: %s", targetID, externalID, err.Error())
			return 0
		}
		return 0
	}))

	targetTbl := l.NewTable()
	targetTbl.RawSetString("hp", l.NewFunction(func(L *gluua.LState) int {
		id := L.CheckInt64(1)
		if b.TargetHP == nil {
			L.RaiseError("target.hp not bound in this context")
			return 0
		}
		cur, max, err := b.TargetHP(id)
		if err != nil {
			L.RaiseError("target.hp(%d) failed: %s", id, err.Error())
			return 0
		}
		L.Push(gluua.LNumber(cur))
		L.Push(gluua.LNumber(max))
		return 2
	}))
	targetTbl.RawSetString("level", l.NewFunction(func(L *gluua.LState) int {
		id := L.CheckInt64(1)
		if b.TargetLevel == nil {
			L.RaiseError("target.level not bound in this context")
			return 0
		}
		lvl, err := b.TargetLevel(id)
		if err != nil {
			L.RaiseError("target.level(%d) failed: %s", id, err.Error())
			return 0
		}
		L.Push(gluua.LNumber(lvl))
		return 1
	}))
	targetTbl.RawSetString("classes", l.NewFunction(func(L *gluua.LState) int {
		id := L.CheckInt64(1)
		if b.TargetClasses == nil {
			L.RaiseError("target.classes not bound in this context")
			return 0
		}
		m, err := b.TargetClasses(id)
		if err != nil {
			L.RaiseError("target.classes(%d) failed: %s", id, err.Error())
			return 0
		}
		out := L.NewTable()
		for cls, lvl := range m {
			out.RawSetString(cls, gluua.LNumber(lvl))
		}
		L.Push(out)
		return 1
	}))
	l.SetGlobal("target", targetTbl)

	// V4 surface (Phase F #32 slice 4) — room + clock read APIs.
	roomTbl := l.NewTable()
	roomTbl.RawSetString("players", l.NewFunction(func(L *gluua.LState) int {
		if b.RoomPlayers == nil {
			L.RaiseError("room.players not bound in this context")
			return 0
		}
		ids, err := b.RoomPlayers()
		if err != nil {
			L.RaiseError("room.players failed: %s", err.Error())
			return 0
		}
		L.Push(int64SliceToTable(L, ids))
		return 1
	}))
	roomTbl.RawSetString("mobs", l.NewFunction(func(L *gluua.LState) int {
		if b.RoomMobs == nil {
			L.RaiseError("room.mobs not bound in this context")
			return 0
		}
		ids, err := b.RoomMobs()
		if err != nil {
			L.RaiseError("room.mobs failed: %s", err.Error())
			return 0
		}
		L.Push(int64SliceToTable(L, ids))
		return 1
	}))
	l.SetGlobal("room", roomTbl)

	clockTbl := l.NewTable()
	clockTbl.RawSetString("hour", l.NewFunction(func(L *gluua.LState) int {
		if b.ClockHour == nil {
			L.RaiseError("clock.hour not bound in this context")
			return 0
		}
		L.Push(gluua.LNumber(b.ClockHour()))
		return 1
	}))
	clockTbl.RawSetString("day", l.NewFunction(func(L *gluua.LState) int {
		if b.ClockDay == nil {
			L.RaiseError("clock.day not bound in this context")
			return 0
		}
		L.Push(gluua.LNumber(b.ClockDay()))
		return 1
	}))
	l.SetGlobal("clock", clockTbl)
}

// int64SliceToTable converts a Go []int64 into a 1-indexed Lua
// table so iterating with `ipairs` returns the same order. Empty
// slice yields an empty table (never pushes nil — Lua iterators
// expect a value).
func int64SliceToTable(L *gluua.LState, ids []int64) *gluua.LTable {
	t := L.NewTable()
	for i, id := range ids {
		t.RawSetInt(i+1, gluua.LNumber(id))
	}
	return t
}

// makeQuestFn produces the Lua-callable closure for quest.accept /
// quest.advance. nil hook → registered stub that raises a
// classified error so the fault budget logs *which* API was missing
// instead of a generic "attempt to call nil".
func makeQuestFn(name string, hook func(string) error) gluua.LGFunction {
	return func(L *gluua.LState) int {
		questID := L.CheckString(1)
		if hook == nil {
			L.RaiseError("%s not bound in this context", name)
			return 0
		}
		if err := hook(questID); err != nil {
			L.RaiseError("%s(%q) failed: %s", name, questID, err.Error())
			return 0
		}
		return 0
	}
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
