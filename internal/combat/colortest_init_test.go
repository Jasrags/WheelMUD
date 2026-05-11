package combat

import "github.com/gookit/color"

// Force gookit/color (cfmt's underlying color renderer) to emit ANSI
// escapes during tests regardless of stdout TTY state. See
// telnet/colortest_init_test.go for the rationale.
func init() { color.ForceOpenColor() }
