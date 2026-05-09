// Package lua wraps gopher-lua with a sandbox + Runner suitable for
// running authored scripts on the bus goroutine. The package owns
// the LState pool, instruction cap, ctx-bound timeout, and the V1
// API binders (`say`, `emote`, `log`, `ctx`).
//
// Phase F #32 slice 1. Slice 2+ adds richer mutation primitives via
// closure-injected hook bags (mirrors the dialogue / quest pattern
// from #30 / #31).
package lua

import (
	"time"

	gluua "github.com/yuin/gopher-lua"
)

// CallTimeout caps wall-clock time per invocation. The Runner
// wraps every Run call with `context.WithTimeout(parent,
// CallTimeout)` and propagates it via gopher-lua's SetContext —
// the VM checks the ctx between instructions, so a tight loop
// like `while true do local x=1 end` aborts within the timeout
// window.
//
// Note: gopher-lua's SetMx is a wall-clock millisecond deadline
// (it spawns a watchdog goroutine that sleeps mx ms then cancels),
// NOT an instruction-count cap. We don't use it — SetContext
// gives us the same protection without leaking watchdog
// goroutines per LState borrow.
const CallTimeout = 50 * time.Millisecond

// disabledGlobals lists every standard-library entry the sandbox
// strips before handing an LState to a script. The list is
// intentionally aggressive — the V1 API surface adds back exactly
// what scripts need (say/emote/log/ctx) and nothing else.
//
// `loadstring` is named as `loadstring` in Lua 5.1 (gopher-lua's
// dialect); `load` is the 5.2+ equivalent. We strip both since
// gopher-lua exposes both depending on compile flags.
var disabledGlobals = []string{
	"os", "io", "debug", "package",
	"dofile", "loadfile", "loadstring", "load",
}

// NewSandboxedState builds a fresh gopher-lua LState with every
// dangerous global stripped. Callers (the Runner pool) borrow
// these and reset them between invocations rather than
// re-creating per call.
//
// SkipOpenLibs is intentionally NOT set — we want the safe
// libraries (string / table / math / coroutine) available. We
// strip the dangerous ones individually so the whitelist stays
// explicit at this file.
func NewSandboxedState() *gluua.LState {
	l := gluua.NewState(gluua.Options{
		// CallStackSize / RegistrySize: defaults are fine for V1.
		// IncludeGoStackTrace is off — Lua errors should not leak
		// Go-side internals into builder-visible logs.
		IncludeGoStackTrace: false,
	})
	for _, g := range disabledGlobals {
		l.SetGlobal(g, gluua.LNil)
	}
	return l
}

// resetSandbox re-strips the disabled globals after a script
// returns. Cheap belt-and-braces for pooled LStates: even if a
// script somehow re-bound `os` to a table during its run, the
// next borrow gets a fresh blank.
func resetSandbox(l *gluua.LState) {
	for _, g := range disabledGlobals {
		l.SetGlobal(g, gluua.LNil)
	}
}
