package telnet

import "github.com/fatih/color"

// Force cfmt's underlying fatih/color package to emit ANSI escapes
// during tests regardless of stdout TTY state. CI runs tests without
// a TTY, which would otherwise leave fatih/color.NoColor = true and
// strip every SGR sequence — exactly the cases the cfmt-rendering
// tests in this package assert against.
func init() { color.NoColor = false }
