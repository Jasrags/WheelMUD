package flow

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Catalog is the boot-loaded set of Flow definitions, keyed by id.
// Immutable after Load returns; safe to share across goroutines
// (every read is a plain map index of a frozen map). Hot-reload
// (§O.9) will replace the whole *Catalog pointer rather than mutate.
type Catalog struct {
	byID map[string]*Flow
}

// Load parses every `*.yaml` file under root as a single flow
// definition (one Flow per file). Each parsed flow is `Validate()`d
// before being added to the catalog; any failure aborts the load
// with the offending filename in the error chain.
//
// Errors are wrapped with filename + flow id where applicable so a
// boot failure points the operator at the bad file.
func Load(root fs.FS) (*Catalog, error) {
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, fmt.Errorf("flow: read root: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !hasYAMLSuffix(n) {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)

	cat := &Catalog{byID: map[string]*Flow{}}
	for _, name := range names {
		data, err := fs.ReadFile(root, name)
		if err != nil {
			return nil, fmt.Errorf("flow: read %s: %w", name, err)
		}
		fl, err := parseFlow(data)
		if err != nil {
			return nil, fmt.Errorf("flow: parse %s: %w", name, err)
		}
		if _, dup := cat.byID[fl.ID]; dup {
			return nil, fmt.Errorf("flow: %s: duplicate flow id %q", name, fl.ID)
		}
		cat.byID[fl.ID] = fl
	}
	return cat, nil
}

// parseFlow unmarshals one file's bytes into a raw flow and projects
// it to a validated *Flow. Stays package-private because the
// rawFlow/rawStep types are loader implementation, not API.
func parseFlow(data []byte) (*Flow, error) {
	var raw rawFlow
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	steps := make([]Step, 0, len(raw.Steps))
	for i, r := range raw.Steps {
		if r.Step == nil {
			return nil, fmt.Errorf("step %d: nil after unmarshal", i)
		}
		steps = append(steps, r.Step)
	}
	fl := &Flow{
		ID:        raw.ID,
		Entry:     raw.Entry,
		Resumable: raw.Resumable,
		Steps:     steps,
	}
	if err := fl.Validate(); err != nil {
		return nil, err
	}
	return fl, nil
}

// Get returns the flow with the given id, or nil if absent. Nil
// receiver is also nil — production callers must Load before Get;
// tests routinely call Get on a non-nil Catalog so the nil-receiver
// guard is purely defensive.
func (c *Catalog) Get(id string) *Flow {
	if c == nil {
		return nil
	}
	return c.byID[id]
}

// IDs returns every flow id in sorted order. Safe for use as a
// closure to the `flow <id>` verb's introspection path; the slice
// is freshly allocated per call so callers may mutate it freely.
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.byID))
	for id := range c.byID {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// All returns every flow sorted by id. Useful for admin dumps;
// not on a hot path.
func (c *Catalog) All() []*Flow {
	ids := c.IDs()
	out := make([]*Flow, 0, len(ids))
	for _, id := range ids {
		out = append(out, c.byID[id])
	}
	return out
}

func hasYAMLSuffix(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
