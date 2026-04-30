package cmd

import (
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/telnet"
)

// Colors prints a series of palettes so an operator can eyeball whether
// their client's color level matches `Session.ColorLevel`. Output is
// produced with raw SGR codes (not cfmt tags) so the rendering path under
// test is the one we ship to game code.
var Colors = &telnet.Command{
	Name:    "colors",
	Aliases: []string{"colortest", "palette"},
	Help:    "Display a color test pattern for the current session",
	Run: func(c *telnet.Context) error {
		s := c.Session
		var b strings.Builder

		fmt.Fprintf(&b, "Terminal: %s    Color level: %s\r\n",
			displayTerm(s.TerminalType), levelName(s.ColorLevel))
		fmt.Fprintf(&b, "Width: %d  Height: %d\r\n\r\n", s.Width, s.Height)

		writeBasicPalette(&b)
		b.WriteString("\r\n")
		write256Cube(&b, s.ColorLevel)
		b.WriteString("\r\n")
		writeGrayscale(&b, s.ColorLevel)
		b.WriteString("\r\n")
		writeTrueColorRamp(&b, s.ColorLevel)
		b.WriteString("\r\n")
		writeStyleSamples(&b)

		return s.WriteRaw([]byte(b.String()))
	},
}

func levelName(level int) string {
	switch level {
	case telnet.ColorLevelNone:
		return "none"
	case telnet.ColorLevelBasic:
		return "basic (8)"
	case telnet.ColorLevel16:
		return "ansi (16)"
	case telnet.ColorLevel256:
		return "xterm (256)"
	case telnet.ColorLevelTrueColor:
		return "truecolor (24-bit)"
	default:
		return fmt.Sprintf("unknown (%d)", level)
	}
}

func displayTerm(t string) string {
	if t == "" {
		return "(unset)"
	}
	return t
}

// writeBasicPalette shows the 16 ANSI base colors as fg-on-bg swatches.
// Always emitted directly; clients on `ColorLevelNone` get raw bytes that
// most terminals will render anyway, which is the point of a test pattern.
func writeBasicPalette(b *strings.Builder) {
	b.WriteString("16-color palette (foreground):\r\n  ")
	for code := 30; code <= 37; code++ {
		fmt.Fprintf(b, "\x1b[%dm %2d \x1b[0m", code, code)
	}
	b.WriteString("\r\n  ")
	for code := 90; code <= 97; code++ {
		fmt.Fprintf(b, "\x1b[%dm %2d \x1b[0m", code, code)
	}
	b.WriteString("\r\n16-color palette (background):\r\n  ")
	for code := 40; code <= 47; code++ {
		fmt.Fprintf(b, "\x1b[%dm %2d \x1b[0m", code, code)
	}
	b.WriteString("\r\n  ")
	for code := 100; code <= 107; code++ {
		fmt.Fprintf(b, "\x1b[%dm %2d \x1b[0m", code, code)
	}
	b.WriteString("\r\n")
}

// write256Cube prints the xterm 6x6x6 cube (16-231). On lower color levels
// we still emit the codes so the user can see whether their client falls
// back gracefully or shows nothing.
func write256Cube(b *strings.Builder, level int) {
	if level < telnet.ColorLevel256 {
		b.WriteString("xterm-256 cube: skipped (color level below 256)\r\n")
		return
	}
	b.WriteString("xterm-256 cube (16-231):\r\n")
	for r := 0; r < 6; r++ {
		b.WriteString("  ")
		for g := 0; g < 6; g++ {
			for blue := 0; blue < 6; blue++ {
				idx := 16 + 36*r + 6*g + blue
				fmt.Fprintf(b, "\x1b[48;5;%dm  \x1b[0m", idx)
			}
			b.WriteByte(' ')
		}
		b.WriteString("\r\n")
	}
}

// writeGrayscale prints the 24 grayscale ramp slots (232-255).
func writeGrayscale(b *strings.Builder, level int) {
	if level < telnet.ColorLevel256 {
		return
	}
	b.WriteString("xterm-256 grayscale (232-255):\r\n  ")
	for idx := 232; idx <= 255; idx++ {
		fmt.Fprintf(b, "\x1b[48;5;%dm  \x1b[0m", idx)
	}
	b.WriteString("\r\n")
}

// writeTrueColorRamp prints a smooth red→green→blue ramp using the helpers
// the rest of the engine will use, so this command also exercises
// `RenderRGBBG` downsampling on lower levels.
func writeTrueColorRamp(b *strings.Builder, level int) {
	const cells = 36
	b.WriteString("RGB ramp (downsampled to current level):\r\n  ")
	for i := 0; i < cells; i++ {
		t := float64(i) / float64(cells-1)
		r, g, blue := hueRamp(t)
		b.WriteString(telnet.RenderRGBBG(level, r, g, blue))
		b.WriteString("  ")
		b.WriteString("\x1b[0m")
	}
	b.WriteString("\r\n")
}

// hueRamp interpolates red → yellow → green → cyan → blue → magenta → red.
// Cheap and good enough for an eyeball test.
func hueRamp(t float64) (int, int, int) {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	h := t * 6
	x := int(255 * (1 - absFrac(h)))
	switch {
	case h < 1:
		return 255, int(255 * h), 0
	case h < 2:
		return x, 255, 0
	case h < 3:
		return 0, 255, int(255 * (h - 2))
	case h < 4:
		return 0, x, 255
	case h < 5:
		return int(255 * (h - 4)), 0, 255
	default:
		return 255, 0, x
	}
}

func absFrac(h float64) float64 {
	f := h - float64(int(h))
	if f < 0 {
		return -f
	}
	return f
}

func writeStyleSamples(b *strings.Builder) {
	b.WriteString("Styles: ")
	samples := []struct {
		code int
		name string
	}{
		{telnet.ANSI_BOLD, "bold"},
		{telnet.ANSI_DIM, "dim"},
		{telnet.ANSI_ITALIC, "italic"},
		{telnet.ANSI_UNDERLINE, "underline"},
		{telnet.ANSI_REVERSE, "reverse"},
		{telnet.ANSI_STRIKE, "strike"},
	}
	for i, s := range samples {
		if i > 0 {
			b.WriteString("  ")
		}
		fmt.Fprintf(b, "%s%s\x1b[0m", telnet.SGR(s.code), s.name)
	}
	b.WriteString("\r\n")
}
