package cmd

import (
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// parseOrdinal splits a "<n>.<keyword>" target into the 1-based index
// and the bare keyword. "sword" → (1, "sword"). "2.sword" → (2, "sword").
// "0.sword" or negative ordinals fall back to index 1 — there is no
// zeroth match in MUD parlance and a leading zero is almost always a
// typo.
func parseOrdinal(target string) (int, string) {
	dot := strings.IndexByte(target, '.')
	if dot <= 0 || dot == len(target)-1 {
		return 1, target
	}
	n, err := strconv.Atoi(target[:dot])
	if err != nil || n < 1 {
		return 1, target
	}
	return n, target[dot+1:]
}

// MatchItem finds the nth item in list whose name token-prefix matches
// keyword. n is 1-based; "2.sword" picks the second matching sword.
// Returns the zero Item and false when no match exists.
func MatchItem(target string, list []repo.Item) (repo.Item, bool) {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return repo.Item{}, false
	}
	n, kw := parseOrdinal(target)
	hit := 0
	for _, it := range list {
		if nameMatches(it.Name, kw) {
			hit++
			if hit == n {
				return it, true
			}
		}
	}
	return repo.Item{}, false
}

// MatchMob is the mob-list equivalent of MatchItem. Same ordinal rules.
func MatchMob(target string, list []creature.MobInstance) (creature.MobInstance, bool) {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return creature.MobInstance{}, false
	}
	n, kw := parseOrdinal(target)
	hit := 0
	for _, m := range list {
		if nameMatches(m.Core.Name, kw) {
			hit++
			if hit == n {
				return m, true
			}
		}
	}
	return creature.MobInstance{}, false
}
