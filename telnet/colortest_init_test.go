package telnet

import "github.com/gookit/color"

// Force gookit/color (cfmt's underlying color renderer) to emit ANSI
// escapes during tests regardless of stdout TTY state. CI runs tests
// without a TTY, so gookit's SupportColor() returns false and every
// cfmt-rendered payload comes out plain — which would fail the
// session-level assertions that check for SGR escapes when
// ColorLevel != None.
func init() { color.ForceOpenColor() }
