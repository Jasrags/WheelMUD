package cmd

import (
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
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

// completerSlot turns the rest-after-verb buffer into the 0-based arg
// slot the trailing partial occupies plus the partial itself. Slot
// counting matches Tokenize semantics so quoted args ("foo bar") count
// as one slot. When the buffer ends in whitespace the partial is empty
// and the cursor sits in a fresh slot.
//
// Returns slot=-1 on Tokenize error (unbalanced quote) so completers
// can bail out instead of guessing.
func completerSlot(rest string) (slot int, partial string) {
	partial, _ = telnet.CompletionPartial(rest)
	toks, err := telnet.Tokenize(rest)
	if err != nil {
		return -1, partial
	}
	if partial == "" {
		return len(toks), ""
	}
	return len(toks) - 1, partial
}

// itemKeywordCandidates returns one Candidate per (item, name-token)
// pair where the token has the partial as a prefix. Order is item-id
// stable so the listing doesn't shuffle between calls. Help is the
// item's full display name. When partial is `<n>.<keyword>`, the
// ordinal is preserved on each Candidate.Text so an in-place
// replacement keeps `2.sword` syntax intact.
func itemKeywordCandidates(items []repo.Item, partial string) []telnet.Candidate {
	prefix, kw := splitOrdinalPartial(partial)
	if kw == "" {
		// Empty keyword: list every item once (first token of name).
		out := make([]telnet.Candidate, 0, len(items))
		seen := make(map[string]bool, len(items))
		for _, it := range items {
			tok := firstNameToken(it.Name)
			if tok == "" || seen[tok] {
				continue
			}
			seen[tok] = true
			out = append(out, telnet.Candidate{Text: prefix + tok, Help: it.Name})
		}
		return out
	}
	return collectKeywordCandidates(itemNames(items), prefix, kw)
}

// mobKeywordCandidates is the mob-list counterpart to itemKeywordCandidates.
func mobKeywordCandidates(mobs []creature.MobInstance, partial string) []telnet.Candidate {
	prefix, kw := splitOrdinalPartial(partial)
	if kw == "" {
		out := make([]telnet.Candidate, 0, len(mobs))
		seen := make(map[string]bool, len(mobs))
		for _, m := range mobs {
			tok := firstNameToken(m.Core.Name)
			if tok == "" || seen[tok] {
				continue
			}
			seen[tok] = true
			out = append(out, telnet.Candidate{Text: prefix + tok, Help: m.Core.Name})
		}
		return out
	}
	return collectKeywordCandidates(mobNames(mobs), prefix, kw)
}

// splitOrdinalPartial peels a leading `<digit>.` off partial so the
// rest can be prefix-matched as a bare keyword. The returned prefix is
// the textual ordinal that needs to be re-prepended to each candidate
// so the in-place replacement keeps the ordinal the user typed.
// "2.swo" → ("2.", "swo"). "swo" → ("", "swo"). "12." → ("12.", "").
// A leading dot or non-numeric prefix is treated as a plain keyword.
func splitOrdinalPartial(partial string) (prefix, keyword string) {
	dot := strings.IndexByte(partial, '.')
	if dot <= 0 {
		return "", strings.ToLower(partial)
	}
	for i := 0; i < dot; i++ {
		if partial[i] < '0' || partial[i] > '9' {
			return "", strings.ToLower(partial)
		}
	}
	return partial[:dot+1], strings.ToLower(partial[dot+1:])
}

// firstNameToken returns the first whitespace-separated token of name,
// lowercased. "" if name has no non-empty tokens.
func firstNameToken(name string) string {
	for _, tok := range strings.Fields(strings.ToLower(name)) {
		return tok
	}
	return ""
}

// itemNames extracts (display, full) pairs for keyword expansion.
// display is the full Name surfaced as Help text on each Candidate;
// full is the same string and is split into tokens during expansion.
func itemNames(items []repo.Item) []namedThing {
	out := make([]namedThing, len(items))
	for i, it := range items {
		out[i] = namedThing{display: it.Name, source: it.Name}
	}
	return out
}

func mobNames(mobs []creature.MobInstance) []namedThing {
	out := make([]namedThing, len(mobs))
	for i, m := range mobs {
		out[i] = namedThing{display: m.Core.Name, source: m.Core.Name}
	}
	return out
}

type namedThing struct {
	display string
	source  string
}

// collectKeywordCandidates walks each named thing, splits the source
// on whitespace, and emits one Candidate per token whose lowercase
// form starts with kw. Tokens are deduped across the whole list so
// "iron sword" and "iron shield" together yield one "iron" candidate,
// not two. Order: thing index, then token order within the thing.
func collectKeywordCandidates(things []namedThing, prefix, kw string) []telnet.Candidate {
	if len(things) == 0 {
		return nil
	}
	out := make([]telnet.Candidate, 0, len(things))
	seen := make(map[string]bool, len(things)*2)
	for _, t := range things {
		for _, tok := range strings.Fields(strings.ToLower(t.source)) {
			if !strings.HasPrefix(tok, kw) {
				continue
			}
			if seen[tok] {
				continue
			}
			seen[tok] = true
			out = append(out, telnet.Candidate{Text: prefix + tok, Help: t.display})
		}
	}
	return out
}
