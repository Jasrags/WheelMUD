package telnet

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	ColorLevelNone      = 0        // No color support
	ColorLevelBasic     = 8        // Basic ANSI colors (8 colors)
	ColorLevel16        = 16       // ANSI colors (16 colors)
	ColorLevel256       = 256      // xterm-256 colors
	ColorLevelTrueColor = 16777216 // 24-bit RGB
)

// ANSI SGR style codes.
//
//	\x1b[38;5;⟨n⟩m  - 8-bit foreground
//	\x1b[48;5;⟨n⟩m  - 8-bit background
//	\x1b[38;2;⟨r⟩;⟨g⟩;⟨b⟩m  - truecolor foreground
//	\x1b[48;2;⟨r⟩;⟨g⟩;⟨b⟩m  - truecolor background
const (
	ANSI_RESET            = 0
	ANSI_BOLD             = 1
	ANSI_DIM              = 2
	ANSI_ITALIC           = 3
	ANSI_UNDERLINE        = 4
	ANSI_BLINK            = 5
	ANSI_REVERSE          = 7
	ANSI_STRIKE           = 9
	ANSI_DOUBLE_UNDERLINE = 21
	ANSI_NORMAL_WEIGHT    = 22
	ANSI_NO_ITALIC        = 23
	ANSI_NO_UNDERLINE     = 24
	ANSI_NO_BLINK         = 25
	ANSI_NO_REVERSE       = 27
	ANSI_NO_STRIKE        = 29
)

// DetectColorLevel maps a TERM string to one of the ColorLevel* constants.
// Telnet does not give us the COLORTERM hint that local shells get, so the
// truecolor branch only fires for terminal types that explicitly advertise
// 24-bit support in their name.
func DetectColorLevel(term string) int {
	t := strings.ToLower(term)
	switch t {
	case "xterm-direct", "xterm-truecolor", "iterm", "iterm2":
		return ColorLevelTrueColor
	case "xterm-256color", "rxvt-unicode-256color", "screen-256color", "mudlet":
		return ColorLevel256
	case "xterm", "vt100", "ansi", "linux":
		return ColorLevel16
	case "dumb", "unknown":
		return ColorLevelNone
	}
	if strings.Contains(t, "truecolor") || strings.Contains(t, "24bit") || strings.Contains(t, "direct") {
		return ColorLevelTrueColor
	}
	if strings.Contains(t, "256") {
		return ColorLevel256
	}
	if strings.Contains(t, "color") {
		return ColorLevel16
	}
	return ColorLevel16
}

// StripANSI removes CSI escape sequences (ESC `[` … final-byte) from b,
// returning a new slice. Used by the cfmt-rendering write paths to
// downsample to plain text for ColorLevelNone clients — cfmt has no
// level-awareness, so the session layer scrubs after rendering.
//
// CSI final bytes are 0x40..0x7E (ANSI X3.64 / ECMA-48); cfmt only
// emits SGR (`m`), but any final byte ends a sequence safely. A bare
// ESC (no `[`) or an unterminated sequence is dropped — the wire is
// already corrupt at that point and we'd rather not re-emit the
// fragments.
func StripANSI(b []byte) []byte {
	if !bytes.Contains(b, []byte{0x1b}) {
		return b
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if b[i] == 0x1b {
			if i+1 < len(b) && b[i+1] == '[' {
				i += 2
				for i < len(b) {
					c := b[i]
					i++
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
				continue
			}
			// Bare ESC or non-CSI escape — skip the ESC and any
			// single follow-up byte if present.
			i++
			if i < len(b) {
				i++
			}
			continue
		}
		out = append(out, b[i])
		i++
	}
	return out
}

// SGR returns "\x1b[<codes>m" for the given numeric SGR codes. Zero codes
// produces a reset.
func SGR(codes ...int) string {
	if len(codes) == 0 {
		return "\x1b[0m"
	}
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = fmt.Sprintf("%d", c)
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

// RenderRGBFG returns a foreground SGR for the given 24-bit color, downsampled
// to the session's advertised level. ColorLevelNone returns "".
func RenderRGBFG(level, r, g, b int) string {
	switch level {
	case ColorLevelNone:
		return ""
	case ColorLevelTrueColor:
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", clamp8(r), clamp8(g), clamp8(b))
	case ColorLevel256:
		return fmt.Sprintf("\x1b[38;5;%dm", rgbTo256(r, g, b))
	default:
		return fmt.Sprintf("\x1b[%dm", rgbTo16(r, g, b, false))
	}
}

// RenderRGBBG returns a background SGR for the given 24-bit color, downsampled
// to the session's advertised level. ColorLevelNone returns "".
func RenderRGBBG(level, r, g, b int) string {
	switch level {
	case ColorLevelNone:
		return ""
	case ColorLevelTrueColor:
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", clamp8(r), clamp8(g), clamp8(b))
	case ColorLevel256:
		return fmt.Sprintf("\x1b[48;5;%dm", rgbTo256(r, g, b))
	default:
		return fmt.Sprintf("\x1b[%dm", rgbTo16(r, g, b, true))
	}
}

func clamp8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// rgbTo256 maps an RGB triple to the xterm 6x6x6 color cube (16-231).
// Values 0-255 are bucketed into one of six rungs; this is the standard
// approximation used by terminal libraries.
func rgbTo256(r, g, b int) int {
	rr := rgb6(clamp8(r))
	gg := rgb6(clamp8(g))
	bb := rgb6(clamp8(b))
	return 16 + 36*rr + 6*gg + bb
}

func rgb6(v int) int {
	// Bucket boundaries: 0, 95, 135, 175, 215, 255 (xterm levels).
	switch {
	case v < 48:
		return 0
	case v < 115:
		return 1
	case v < 155:
		return 2
	case v < 195:
		return 3
	case v < 235:
		return 4
	default:
		return 5
	}
}

// rgbTo16 returns the SGR code (30-37 fg / 40-47 bg, plus bright variants
// 90-97 / 100-107) closest to the given RGB. Brightness is decided by max
// channel value; hue by which channel(s) dominate.
func rgbTo16(r, g, b int, bg bool) int {
	r = clamp8(r)
	g = clamp8(g)
	bb := clamp8(b)
	mx := r
	if g > mx {
		mx = g
	}
	if bb > mx {
		mx = bb
	}
	bright := mx >= 128
	const thr = 64
	hi := func(v int) bool { return v+thr >= mx && mx > 0 }
	rH, gH, bH := hi(r), hi(g), hi(bb)

	var idx int
	switch {
	case !rH && !gH && !bH:
		idx = 0 // black
	case rH && gH && bH:
		idx = 7 // white
	case rH && gH:
		idx = 3 // yellow
	case gH && bH:
		idx = 6 // cyan
	case rH && bH:
		idx = 5 // magenta
	case rH:
		idx = 1 // red
	case gH:
		idx = 2 // green
	case bH:
		idx = 4 // blue
	default:
		idx = 7
	}

	base := 30
	if bg {
		base = 40
	}
	if bright {
		base += 60
	}
	return base + idx
}
