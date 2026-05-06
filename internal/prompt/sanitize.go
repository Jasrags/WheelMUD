package prompt

import "strings"

// MaxTemplateLen caps the per-character template length. A longer
// prompt would push command output off the right margin on standard
// 80-col terminals; 120 leaves room for color tags + a few
// placeholders. Shared by the in-game `prompt` verb and the §6
// account-menu settings sub-menu so both apply the same hardening.
const MaxTemplateLen = 120

// SanitizeTemplate strips control bytes first and then enforces the
// length cap so a template that fits after stripping is accepted.
// Unlike a chat sanitizer it does NOT defang cfmt syntax — color
// tags in a player's own prompt are intentional. Returns ok=false
// on empty (post-strip) or oversized input.
//
// Control bytes that would otherwise reach the terminal include
// ESC (0x1b — raw SGR sequences that bypass the cfmt rendering
// path) and IAC (0xff — telnet command byte that desynchronises
// the protocol parser when echoed). Everything below 0x20 plus 0x7f
// is dropped.
func SanitizeTemplate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "", false
	}
	if len(out) > MaxTemplateLen {
		return "", false
	}
	return out, true
}
