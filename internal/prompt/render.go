// Package prompt renders a player prompt template against live
// session/character values. The grammar is intentionally tiny:
//
//	%h / %H   current / max hit points
//	%m / %M   current / max mana (One-Power pool — 0 today, reserved)
//	%v / %V   current / max move (fatigue/move pool — 0 today, reserved)
//	%r        current room name
//	%g        carried coin (currency.Amount.Short)
//	%t        combat target name (empty today, reserved)
//	%%        literal '%'
//
// Unknown `%X` passes through verbatim so a typo in a builder template
// is visible rather than silently swallowed. A trailing `%` at end of
// string also passes through.
package prompt

import (
	"strconv"
	"strings"
)

// Vars carries every value the template grammar can interpolate. The
// reserved fields (Mn*, Mv*, Target) render as their zero value until
// the corresponding game systems land; the API stays stable.
type Vars struct {
	HPCur, HPMax int32
	MnCur, MnMax int32
	MvCur, MvMax int32
	RoomName     string
	Gold         string
	Target       string
}

// Render expands tmpl against v.
func Render(tmpl string, v Vars) string {
	if tmpl == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(tmpl) + 16)
	for i := 0; i < len(tmpl); i++ {
		c := tmpl[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(tmpl) {
			b.WriteByte('%')
			continue
		}
		i++
		switch tmpl[i] {
		case '%':
			b.WriteByte('%')
		case 'h':
			b.Write(strconv.AppendInt(nil, int64(v.HPCur), 10))
		case 'H':
			b.Write(strconv.AppendInt(nil, int64(v.HPMax), 10))
		case 'm':
			b.Write(strconv.AppendInt(nil, int64(v.MnCur), 10))
		case 'M':
			b.Write(strconv.AppendInt(nil, int64(v.MnMax), 10))
		case 'v':
			b.Write(strconv.AppendInt(nil, int64(v.MvCur), 10))
		case 'V':
			b.Write(strconv.AppendInt(nil, int64(v.MvMax), 10))
		case 'r':
			b.WriteString(v.RoomName)
		case 'g':
			b.WriteString(v.Gold)
		case 't':
			b.WriteString(v.Target)
		default:
			b.WriteByte('%')
			b.WriteByte(tmpl[i])
		}
	}
	return b.String()
}

// NeedsRoom reports whether tmpl references %r and therefore requires
// a room lookup. Lets callers avoid an unnecessary RoomRepo hit when
// the active template only uses character-local placeholders.
func NeedsRoom(tmpl string) bool { return containsToken(tmpl, 'r') }

// NeedsGold reports whether tmpl references %g.
func NeedsGold(tmpl string) bool { return containsToken(tmpl, 'g') }

// containsToken returns true when tmpl contains an unescaped `%t`
// placeholder for byte t. `%%` is treated as a literal and skipped.
func containsToken(tmpl string, t byte) bool {
	for i := 0; i < len(tmpl); i++ {
		if tmpl[i] != '%' || i+1 >= len(tmpl) {
			continue
		}
		i++
		if tmpl[i] == t {
			return true
		}
		// `%%` already consumed by the i++ above; any other `%X`
		// is just skipped past.
	}
	return false
}
