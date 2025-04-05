package telnet

import (
	"regexp"
	"strings"
)

const (
	ColorLevelNone  = 0   // No color support
	ColorLevelBasic = 8   // Basic ANSI colors (8 colors)
	ColorLevel16    = 16  // ANSI colors (16 colors)
	ColorLevel256   = 256 // xterm-256 colors
)

// ANSI color codes for terminal output
const (
	// \x1b is the escape character
	// 8bit support
	// \x1b[38;5;⟨n⟩m Select foreground color
	// \x1b[48;5;⟨n⟩m Select background color

	// Truecolor support
	// \x1b[38;2;⟨r⟩;⟨g⟩;⟨b⟩m Select RGB foreground color
	// \x1b[48;2;⟨r⟩;⟨g⟩;⟨b⟩m Select RGB background color

	// Reset & Style
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

var ansiColors = map[string]string{
	"reset":   "\x1b[0m",
	"red":     "\x1b[31m",
	"green":   "\x1b[32m",
	"yellow":  "\x1b[33m",
	"blue":    "\x1b[34m",
	"magenta": "\x1b[35m",
	"cyan":    "\x1b[36m",
	"white":   "\x1b[37m",
}

// // clamp ensures RGB values stay within 0–255
// func clamp(n int) int {
// 	if n < 0 {
// 		return 0
// 	}
// 	if n > 255 {
// 		return 255
// 	}
// 	return n
// }

var colorTagRegex = regexp.MustCompile(`(?i)\{(/?[a-z]+)\}`)

func RenderColorTags(text string, s *Session) string {
	if s.ColorLevel == 0 {
		return colorTagRegex.ReplaceAllString(text, "")
	}
	return colorTagRegex.ReplaceAllStringFunc(text, func(tag string) string {
		name := strings.Trim(tag, "{}")
		name = strings.ToLower(name)

		// Treat any closing tag like {/green} as {reset}
		if strings.HasPrefix(name, "/") {
			return ansiColors["reset"]
		}
		if val, ok := ansiColors[name]; ok {
			return val
		}
		return tag
	})
}

func DetectColorLevel(term string) int {
	switch strings.ToLower(term) {
	case "xterm-256color", "rxvt-unicode-256color", "screen-256color", "mudlet":
		return 256
	case "xterm", "vt100", "ansi", "linux":
		return 16
	case "dumb", "unknown":
		return 0
	default:
		if strings.Contains(term, "256") {
			return 256
		} else if strings.Contains(term, "color") {
			return 16
		}
		return 16
	}
}
