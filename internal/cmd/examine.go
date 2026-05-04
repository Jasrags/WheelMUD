package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewExamine builds the `examine <target>` command. Lookup order:
//  1. mobs in the current room
//  2. items on the floor of the current room
//  3. items in the player's inventory (§14)
//
// Equipment-slot resolution will land alongside the §9 equipment-slots
// bullet. Targets accept the ordinal `<n>.<keyword>` syntax via
// MatchItem / MatchMob.
func NewExamine(items repo.ItemRepo, mobs repo.MobInstanceRepo) *telnet.Command {
	return &telnet.Command{
		Name:      "examine",
		Aliases:   []string{"exa", "ex"},
		Help:      "Examine <target> for a closer look",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeExamineTargets(items, mobs),
		Run: func(c *telnet.Context) error {
			if c.Session.CurrentRoomID == 0 {
				return c.Session.WriteString("{{There is nothing here to examine.}}::yellow\r\n")
			}
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			if target == "" {
				return c.Session.WriteString("{{Examine what?}}::yellow\r\n")
			}

			mobsList, err := mobs.ListInRoom(c.Ctx, c.Session.CurrentRoomID)
			if err != nil {
				slog.Error("examine: list mobs", "room", c.Session.CurrentRoomID, "error", err)
				return c.Session.WriteString("{{You cannot focus on anything right now.}}::red\r\n")
			}
			if m, ok := MatchMob(target, mobsList); ok {
				return c.Session.WriteString(renderMob(m))
			}

			itemsList, err := items.ListInRoom(c.Ctx, c.Session.CurrentRoomID)
			if err != nil {
				slog.Error("examine: list items", "room", c.Session.CurrentRoomID, "error", err)
				return c.Session.WriteString("{{You cannot focus on anything right now.}}::red\r\n")
			}
			if it, ok := MatchItem(target, itemsList); ok {
				return c.Session.WriteString(renderItem(it))
			}

			if c.Session.CharacterID != 0 {
				inv, err := items.ListInInventory(c.Ctx, c.Session.CharacterID)
				if err != nil {
					slog.Error("examine: list inventory", "char", c.Session.CharacterID, "error", err)
					return c.Session.WriteString("{{You cannot focus on anything right now.}}::red\r\n")
				}
				if it, ok := MatchItem(target, inv); ok {
					return c.Session.WriteString(renderItem(it))
				}
			}

			return c.Session.WriteString("{{You don't see anything like that here.}}::yellow\r\n")
		},
	}
}

// completeExamineTargets unions the three places examine looks: mobs
// in the current room, items on the floor, and items in the actor's
// inventory. Slot 0 only — examine takes exactly one target.
//
// Look order matches the runtime resolver: mobs first, then floor
// items, then inventory. Tokens are deduped across sources so a
// "rusty sword" on the floor and "rusty sword" in inventory yield
// one candidate.
func completeExamineTargets(items repo.ItemRepo, mobs repo.MobInstanceRepo) func(s *telnet.Session, args string) []telnet.Candidate {
	return func(s *telnet.Session, args string) []telnet.Candidate {
		slot, partial := completerSlot(args)
		if slot != 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var out []telnet.Candidate
		if s.CurrentRoomID != 0 {
			if list, err := mobs.ListInRoom(ctx, s.CurrentRoomID); err == nil {
				out = append(out, mobKeywordCandidates(list, partial)...)
			}
			if list, err := items.ListInRoom(ctx, s.CurrentRoomID); err == nil {
				out = append(out, itemKeywordCandidates(list, partial)...)
			}
		}
		if s.CharacterID != 0 {
			if list, err := items.ListInInventory(ctx, s.CharacterID); err == nil {
				out = append(out, itemKeywordCandidates(list, partial)...)
			}
		}
		return dedupCandidates(out)
	}
}

// dedupCandidates collapses Candidates that share Text. The first
// occurrence wins so look-order (mobs → floor → inventory) is
// preserved when a player has e.g. an inventory item that shares a
// token with a room mob.
func dedupCandidates(in []telnet.Candidate) []telnet.Candidate {
	if len(in) == 0 {
		return nil
	}
	out := in[:0:0]
	seen := make(map[string]bool, len(in))
	for _, c := range in {
		if seen[c.Text] {
			continue
		}
		seen[c.Text] = true
		out = append(out, c)
	}
	return out
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
