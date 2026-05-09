package scripts

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// compile parses Lua source into a reusable *lua.FunctionProto. The
// boot path runs every authored script through this so a syntax
// error surfaces at LoadAndSync — never at the bus goroutine where
// it would arrive as a fault on first fire.
//
// gopher-lua's parse errors include line + column. We prefix with
// the script name so a multi-script catalog points at the right
// file when the boot log surfaces the failure.
//
// Source-level keyword checks (e.g. "scripts that mention os.execute")
// are intentionally NOT performed. The runtime sandbox in
// internal/lua already strips the dangerous globals — a script that
// references `os` after the strip just gets `nil` and either errors
// or silently no-ops. A boot-time substring scan is fragile (it
// rejects probe scripts that legitimately type-check `os`) and
// duplicates work the sandbox does correctly.
func compile(l *lua.LState, name, source string) (*lua.FunctionProto, error) {
	chunk, err := l.LoadString(source)
	if err != nil {
		return nil, fmt.Errorf("scripts: %s: %w", name, err)
	}
	if chunk.Proto == nil {
		return nil, fmt.Errorf("scripts: %s: nil function prototype", name)
	}
	return chunk.Proto, nil
}
