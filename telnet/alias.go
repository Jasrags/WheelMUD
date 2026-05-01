package telnet

import (
	"sort"
	"strings"
	"sync"
)

// AliasTable holds per-session user-level aliases. Distinct from
// Registry aliases (which are baked into command definitions): these
// are runtime, user-defined, and not yet persisted across sessions.
//
// Reads and writes are mutex-guarded so the alias command (running on
// the dispatcher goroutine) and Registry.Dispatch (also dispatcher)
// can't race even if a future change moves either off-thread.
type AliasTable struct {
	mu      sync.RWMutex
	entries map[string]string
}

// NewAliasTable returns an empty table ready for use.
func NewAliasTable() *AliasTable {
	return &AliasTable{entries: make(map[string]string)}
}

// Set stores name → expansion. name is normalized to lowercase. An
// empty name or expansion is rejected so a malformed alias can't shadow
// a real verb invisibly.
func (t *AliasTable) Set(name, expansion string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	expansion = strings.TrimSpace(expansion)
	if name == "" || expansion == "" {
		return false
	}
	if !validVerb(name) {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries[name] = expansion
	return true
}

// Delete removes name. Returns true if anything was removed.
func (t *AliasTable) Delete(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.entries[name]; !ok {
		return false
	}
	delete(t.entries, name)
	return true
}

// Lookup returns the expansion for name, or "" / false if absent.
func (t *AliasTable) Lookup(name string) (string, bool) {
	name = strings.ToLower(name)
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.entries[name]
	return v, ok
}

// All returns every alias as parallel name/expansion slices, sorted by
// name. Useful for `alias` (no-args) listing output.
func (t *AliasTable) All() (names, expansions []string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	names = make([]string, 0, len(t.entries))
	for n := range t.entries {
		names = append(names, n)
	}
	sort.Strings(names)
	expansions = make([]string, len(names))
	for i, n := range names {
		expansions[i] = t.entries[n]
	}
	return names, expansions
}

// expandAlias rewrites line if its leading verb matches a user alias.
// It runs once and returns the rewritten line — Dispatch must NOT call
// it again on the result, to keep the resolution closed against
// alias-of-alias loops without needing a visited set. Returns the input
// unchanged if no alias applies or if t is nil.
func expandAlias(t *AliasTable, line string) string {
	if t == nil {
		return line
	}
	verb, rest := splitVerb(line)
	if verb == "" {
		return line
	}
	exp, ok := t.Lookup(verb)
	if !ok {
		return line
	}
	if rest == "" {
		return exp
	}
	return exp + " " + rest
}
