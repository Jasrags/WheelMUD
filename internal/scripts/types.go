// Package scripts is the boot-time catalog of authored Lua scripts
// (§15 / Phase F #32). Triggers / dialogue effects / quest steps
// reference scripts by name (e.g. `{"script": "warden_alert"}`); the
// runtime resolves the name through this catalog and runs the
// pre-compiled body in a sandboxed `internal/lua.Runner`.
//
// Each Lua source file under `internal/scripts/default/<name>.lua`
// (or wherever `SCRIPT_DIR` points) becomes one Catalog entry. The
// loader compiles every script at boot, so a syntax error fails
// LoadAndSync loudly with the file path + gopher-lua's error.
//
// The Slice 1 API surface is documented in
// `internal/scripts/default/README.md`. Scripts may call `say`,
// `emote`, and `log`, and read fields off the bound `ctx` table.
// Mutation primitives (player / mob / quest) land in Slice 2+.
package scripts

import lua "github.com/yuin/gopher-lua"

// Script is one authored Lua file resolved by name. Source is kept
// alongside the compiled prototype so `tedit` (Slice 4) can show the
// original text without re-reading the file. Compiled is the
// pre-parsed body the runner executes — re-using it across calls
// avoids paying parse cost per invocation.
type Script struct {
	Name     string
	Source   string
	Compiled *lua.FunctionProto
}

// Catalog is the immutable boot-time set of authored scripts, keyed
// by Script.Name. Empty catalog is valid: deploys can ship without
// scripts authored, and the trigger lua action handler refuses
// unknown names with a fault-budget increment rather than crashing.
type Catalog struct {
	ByName map[string]*Script
}

// Get returns the catalog entry for name (or nil + false on miss).
// The Lua action handler uses this to refuse unknown script names
// without panicking.
func (c *Catalog) Get(name string) (*Script, bool) {
	if c == nil {
		return nil, false
	}
	s, ok := c.ByName[name]
	return s, ok
}
