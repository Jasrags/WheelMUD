// Package display owns rendering helpers shared across cmd, mob, and
// world packages. The first export is Defang — the cfmt-syntax /
// control-byte scrubber used everywhere a YAML-authored or DB-stored
// display string lands inside a cfmt-formatted line on the wire.
//
// Two near-identical copies of this scrubber lived in
// internal/cmd/track.go (safeName) and internal/mob/wander.go
// (safeMobName) before being consolidated here — drift between them
// was tracked in world_aggregates_followups.md.
package display

import "strings"

// Defang scrubs `name` for safe inclusion inside a cfmt-formatted
// line. It strips C0 control bytes + DEL (0x00..0x1f, 0x7f) so a
// stray `\r\n` or terminal escape can't inject styling/scrolling,
// then breaks any `{{` / `}}` / `::` cfmt markers by inserting a
// space — preserving readability when a builder typed e.g.
// `the keep at A::B` while neutralising the parse-time meaning.
//
// `fallback` is returned when name is empty before scrubbing or
// becomes empty after (a control-only string). Callers pass a
// context-appropriate placeholder ("Something", "elsewhere",
// "an unknown place").
func Defang(name, fallback string) string {
	if name == "" {
		return fallback
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return fallback
	}
	return cfmtDefanger.Replace(out)
}

// cfmtDefanger is package-level so the allocation happens once at
// init time rather than on every call. The replacer is safe for
// concurrent use.
var cfmtDefanger = strings.NewReplacer("{{", "{ {", "}}", "} }", "::", ": :")
