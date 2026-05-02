package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewExamine builds the `examine <target>` command. It resolves the
// target against the room's mobs first, then the room's items, and
// renders a detail block for the match. Inventory and equipment
// resolution lands once §14 introduces those repos.
func NewExamine(items repo.ItemRepo, mobs repo.MobInstanceRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "examine",
		Aliases: []string{"exa", "ex"},
		Help:    "Examine <target> for a closer look",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			if c.Session.CurrentRoomID == 0 {
				return c.Session.WriteString("{{There is nothing here to examine.}}::yellow\r\n")
			}
			// Join all args so multi-word targets like
			// `examine town crier` work. Whitespace inside the
			// target collapses to single spaces.
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			if target == "" {
				return c.Session.WriteString("{{Examine what?}}::yellow\r\n")
			}

			mobsList, err := mobs.ListInRoom(c.Ctx, c.Session.CurrentRoomID)
			if err != nil {
				slog.Error("examine: list mobs", "room", c.Session.CurrentRoomID, "error", err)
				return c.Session.WriteString("{{You cannot focus on anything right now.}}::red\r\n")
			}
			if m, ok := matchMob(mobsList, target); ok {
				return c.Session.WriteString(renderMob(m))
			}

			itemsList, err := items.ListInRoom(c.Ctx, c.Session.CurrentRoomID)
			if err != nil {
				slog.Error("examine: list items", "room", c.Session.CurrentRoomID, "error", err)
				return c.Session.WriteString("{{You cannot focus on anything right now.}}::red\r\n")
			}
			if it, ok := matchItem(itemsList, target); ok {
				return c.Session.WriteString(renderItem(it))
			}

			return c.Session.WriteString("{{You don't see anything like that here.}}::yellow\r\n")
		},
	}
}

// matchMob returns the first mob whose lowercased name contains the
// target as a whitespace-separated word prefix. Falls back to a plain
// substring match so `examine crier` finds "a town crier".
func matchMob(list []creature.MobInstance, target string) (creature.MobInstance, bool) {
	for _, m := range list {
		if nameMatches(m.Core.Name, target) {
			return m, true
		}
	}
	return creature.MobInstance{}, false
}

func matchItem(list []repo.Item, target string) (repo.Item, bool) {
	for _, it := range list {
		if nameMatches(it.Name, target) {
			return it, true
		}
	}
	return repo.Item{}, false
}

// nameMatches checks whether target matches name on a token-prefix
// basis — classic MUD keyword matching. "a town crier" matches
// "town", "crier", or "tow"; it does NOT match a substring like
// "own" because that would make every short keyword wildly ambiguous.
func nameMatches(name, target string) bool {
	lower := strings.ToLower(name)
	for _, tok := range strings.Fields(lower) {
		if strings.HasPrefix(tok, target) {
			return true
		}
	}
	return false
}

// renderItem / renderMob interpolate operator-authored Item.Name and
// Core.Name straight into cfmt {{...}}::style tags. World data comes
// from YAML in WORLD_DIR (operator-controlled), so a `}}` in a name
// would be a builder bug, not an attack surface. If/when player-
// editable names land (e.g. branded weapons), sanitize here the way
// sanitizeChat does in comm.go.
func renderItem(it repo.Item) string {
	var b strings.Builder
	b.WriteString("{{")
	b.WriteString(it.Name)
	b.WriteString("}}::green|bold\r\n")
	if desc := strings.TrimSpace(it.ShortDesc); desc != "" {
		b.WriteString(toCRLF(desc))
		b.WriteString("\r\n")
	}
	// Taxonomy fields (§9 migration 0015). Hidden when the item is
	// trash and weightless and worthless — the common case for the
	// pre-taxonomy "a small pebble" rows; surfaced once a builder
	// has actually filled in stats so a player can see what it is.
	if it.Type != "" && it.Type != repo.ItemTypeTrash {
		b.WriteString("{{Type:}}::yellow|bold ")
		b.WriteString("{{")
		b.WriteString(string(it.Type))
		b.WriteString("}}::white\r\n")
	}
	if it.Quality != "" && it.Quality != repo.QualityNormal {
		b.WriteString("{{Quality:}}::yellow|bold ")
		b.WriteString("{{")
		b.WriteString(string(it.Quality))
		b.WriteString("}}::white\r\n")
	}
	if it.Weight > 0 {
		b.WriteString("{{Weight:}}::yellow|bold ")
		b.WriteString(fmt.Sprintf("{{%g lb}}::white\r\n", it.Weight))
	}
	if it.Value > 0 {
		b.WriteString("{{Value:}}::yellow|bold ")
		b.WriteString("{{")
		b.WriteString(it.Value.Format())
		b.WriteString("}}::white\r\n")
	}
	if it.ShortDesc == "" && it.Type == repo.ItemTypeTrash {
		b.WriteString("{{You see nothing special about it.}}::gray\r\n")
	}
	return b.String()
}

func renderMob(m creature.MobInstance) string {
	var b strings.Builder
	b.WriteString("{{")
	b.WriteString(m.Core.Name)
	b.WriteString("}}::magenta|bold\r\n")
	b.WriteString("{{")
	b.WriteString(hpDescriptor(m.Core.HPCurrent, m.Core.HPMax))
	b.WriteString("}}::white\r\n")
	if names := conditionNames(m.Core.Conditions); len(names) > 0 {
		b.WriteString("{{Conditions:}}::yellow|bold ")
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("\r\n")
	}
	if len(m.Core.Affects) > 0 {
		b.WriteString("{{Affects:}}::cyan|bold ")
		for i, a := range m.Core.Affects {
			if i > 0 {
				b.WriteString(", ")
			}
			if a.Name == "" {
				b.WriteString("(unnamed)")
			} else {
				b.WriteString(a.Name)
			}
		}
		b.WriteString("\r\n")
	}
	return b.String()
}

// hpDescriptor renders a coarse HP band so players can size up a mob
// without seeing exact numbers. Mirrors the classic `consider` output.
// Returns "in unknown shape" when HPMax is zero (template not yet
// populating stats).
func hpDescriptor(cur, max int32) string {
	if max <= 0 {
		return "is in unknown shape."
	}
	if cur <= 0 {
		return "is at death's door."
	}
	pct := float64(cur) / float64(max)
	switch {
	case pct >= 1.0:
		return "is in perfect health."
	case pct >= 0.75:
		return "has a few scratches."
	case pct >= 0.5:
		return "is wounded."
	case pct >= 0.25:
		return "is badly wounded."
	default:
		return "is barely standing."
	}
}

// conditionNames decodes the Condition bitset into player-facing
// labels. Order matches the bit order so output is stable.
func conditionNames(c creature.Condition) []string {
	if c == 0 {
		return nil
	}
	type entry struct {
		bit  creature.Condition
		name string
	}
	table := []entry{
		{creature.CondAbilityDamaged, "ability-damaged"},
		{creature.CondAbilityDrained, "ability-drained"},
		{creature.CondBlinded, "blinded"},
		{creature.CondChecked, "checked"},
		{creature.CondCowering, "cowering"},
		{creature.CondDazed, "dazed"},
		{creature.CondDeafened, "deafened"},
		{creature.CondDisabled, "disabled"},
		{creature.CondDying, "dying"},
		{creature.CondEntangled, "entangled"},
		{creature.CondExhausted, "exhausted"},
		{creature.CondFatigued, "fatigued"},
		{creature.CondFlatFooted, "flat-footed"},
		{creature.CondFrightened, "frightened"},
		{creature.CondGrappled, "grappled"},
		{creature.CondHeld, "held"},
		{creature.CondHelpless, "helpless"},
		{creature.CondPanicked, "panicked"},
		{creature.CondParalyzed, "paralyzed"},
		{creature.CondPinned, "pinned"},
		{creature.CondProne, "prone"},
		{creature.CondShaken, "shaken"},
		{creature.CondStable, "stable"},
		{creature.CondStaggered, "staggered"},
		{creature.CondStunned, "stunned"},
		{creature.CondUnconscious, "unconscious"},
	}
	var out []string
	for _, e := range table {
		if c&e.bit != 0 {
			out = append(out, e.name)
		}
	}
	return out
}
