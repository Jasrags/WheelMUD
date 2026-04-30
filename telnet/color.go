package telnet

import "strings"

const (
	ColorLevelNone  = 0   // No color support
	ColorLevelBasic = 8   // Basic ANSI colors (8 colors)
	ColorLevel16    = 16  // ANSI colors (16 colors)
	ColorLevel256   = 256 // xterm-256 colors
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

func DetectColorLevel(term string) int {
	switch strings.ToLower(term) {
	case "xterm-256color", "rxvt-unicode-256color", "screen-256color", "mudlet":
		return ColorLevel256
	case "xterm", "vt100", "ansi", "linux":
		return ColorLevel16
	case "dumb", "unknown":
		return ColorLevelNone
	default:
		if strings.Contains(term, "256") {
			return ColorLevel256
		}
		if strings.Contains(term, "color") {
			return ColorLevel16
		}
		return ColorLevel16
	}
}
