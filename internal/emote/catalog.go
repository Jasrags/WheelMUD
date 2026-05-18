package emote

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Catalog is the in-memory social-verb index. Built once at boot via
// Load (or LoadFS via SourceFS); §M.6 added in-place mutation via
// Replace for hot-reload, so all read paths now acquire mu.RLock.
type Catalog struct {
	mu     sync.RWMutex
	byID   map[string]Social // id → Social
	byName map[string]string // id OR alias (lowercased) → owning id
	order  []string          // ids in load order, for stable iteration
}

// Get returns the social with the given id and a presence bool.
// Lookup is exact (id only); aliases are reflected in the registered
// telnet.Command set, not here.
func (c *Catalog) Get(id string) (Social, bool) {
	if c == nil {
		return Social{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.byID[id]
	return s, ok
}

// All returns every social in load order. The returned slice is a
// fresh copy; callers may mutate without affecting the catalog.
func (c *Catalog) All() []Social {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Social, 0, len(c.order))
	for _, id := range c.order {
		out = append(out, c.byID[id])
	}
	return out
}

// IDs returns every id in sorted order.
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.byID))
	for id := range c.byID {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Replace atomically swaps this catalog's contents for those of `other`.
// Used by §M.6 hot-reload: the caller loads a fresh catalog via
// Load(SourceFS()), validates it, then calls Replace on the live
// catalog so all readers (NewSocialsList, future reload-aware
// consumers) see the new state.
//
// The same `*Catalog` pointer survives — callers that captured it at
// boot (e.g. internal/cmd/socials_list.go) continue to work without
// any indirection layer. Callers that captured `Social` values by
// snapshot (e.g. internal/cmd/social.go::buildSocialCommand) keep
// their old values; the reload verb is responsible for Unregister +
// Register'ing per-social commands to pick up the new contents.
//
// Nil `other` clears the catalog. Self-replace is a no-op.
func (c *Catalog) Replace(other *Catalog) {
	if c == nil || c == other {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if other == nil {
		c.byID = map[string]Social{}
		c.byName = map[string]string{}
		c.order = nil
		return
	}
	other.mu.RLock()
	defer other.mu.RUnlock()
	c.byID = make(map[string]Social, len(other.byID))
	for k, v := range other.byID {
		c.byID[k] = v
	}
	c.byName = make(map[string]string, len(other.byName))
	for k, v := range other.byName {
		c.byName[k] = v
	}
	c.order = append(c.order[:0], other.order...)
}

// Load parses every *.yaml file under root and returns a populated
// Catalog. Errors are wrapped with the offending filename so a typo
// fails the boot loudly. Within a file, entries are processed in
// document order; across files, files are processed in lexical name
// order so reload semantics are deterministic.
func Load(root fs.FS) (*Catalog, error) {
	cat := &Catalog{
		byID:   map[string]Social{},
		byName: map[string]string{},
	}
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, fmt.Errorf("emote: read root: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		n := ent.Name()
		if !hasYAMLSuffix(n) {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := fs.ReadFile(root, name)
		if err != nil {
			return nil, fmt.Errorf("emote: read %s: %w", name, err)
		}
		var batch struct {
			Socials []Social `yaml:"socials"`
		}
		if err := yaml.Unmarshal(data, &batch); err != nil {
			return nil, fmt.Errorf("emote: parse %s: %w", name, err)
		}
		for _, s := range batch.Socials {
			s.ID = strings.ToLower(strings.TrimSpace(s.ID))
			normAliases := make([]string, 0, len(s.Aliases))
			for _, a := range s.Aliases {
				a = strings.ToLower(strings.TrimSpace(a))
				if a == "" {
					continue
				}
				normAliases = append(normAliases, a)
			}
			s.Aliases = normAliases
			if err := s.validate(); err != nil {
				return nil, fmt.Errorf("emote: %s: %w", name, err)
			}
			if owner, dup := cat.byName[s.ID]; dup {
				return nil, fmt.Errorf("emote: %s: id %q collides with %q", name, s.ID, owner)
			}
			cat.byName[s.ID] = s.ID
			for _, a := range s.Aliases {
				if a == s.ID {
					return nil, fmt.Errorf("emote: %s: %s: alias %q equals id", name, s.ID, a)
				}
				if owner, dup := cat.byName[a]; dup {
					return nil, fmt.Errorf("emote: %s: alias %q collides with %q", name, a, owner)
				}
				cat.byName[a] = s.ID
			}
			cat.byID[s.ID] = s
			cat.order = append(cat.order, s.ID)
		}
	}
	return cat, nil
}

func hasYAMLSuffix(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
