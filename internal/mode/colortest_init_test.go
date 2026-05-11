package mode

import "github.com/fatih/color"

// Force cfmt's underlying fatih/color package to emit ANSI escapes
// during tests regardless of stdout TTY state. See telnet/colortest_init_test.go
// for the rationale.
func init() { color.NoColor = false }
