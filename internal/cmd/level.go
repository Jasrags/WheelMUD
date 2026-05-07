package cmd

import "github.com/Jasrags/WheelMUD/internal/repo"

// NewbiePvPLevelCap is the inclusive lower bound a character must
// reach (on either side of an attack) before player-vs-player combat
// is allowed. Below this, attack <player> is refused regardless of
// the PvP opt-in flag — see internal/cmd/attack.go's PvP guard.
const NewbiePvPLevelCap = 10

// characterLevel returns the character's effective level — the sum
// of all class levels in ClassLevels. Multi-class characters add the
// values together (per CLAUDE.md §11). Returns 0 when ClassLevels is
// empty (chargen incomplete) or nil.
func characterLevel(ch repo.Character) int {
	total := 0
	for _, lvl := range ch.ClassLevels {
		total += int(lvl)
	}
	return total
}
