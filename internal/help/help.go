// Package help exposes the embedded help-topic catalog rendered by the
// in-game `help <topic>` command.
//
// Layout:
//
//	assets/topics/<id>.md — one Markdown file per topic, prefixed with
//	                       a YAML-ish front-matter block:
//	                           ---
//	                           id: combat
//	                           title: Combat
//	                           keywords: combat, fight, attack
//	                           ---
//	                           Body text rendered through cfmt.
//
// Topics are an immutable companion to telnet.Registry: the help command
// merges command lookups with topic lookups so `help <name>` resolves
// uniformly across both spaces (exact → keyword → unique-prefix →
// ambiguity list).
package help

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

//go:embed assets/topics/*.md
var assets embed.FS

// Topic is a single help article. ID is the lowercase canonical name
// (also the prefix-scan key); Title is the display heading; Keywords
// are alternate exact-match aliases; Body is the article content.
//
// TRUST BOUNDARY: Body is operator-curated, embedded at build time,
// and rendered through cfmt with tags interpreted ({{...}}::style).
// It MUST NOT include strings derived from player input. Any future
// path that surfaces player text through this pipeline must escape
// cfmt's `{{`/`}}::` syntax first.
type Topic struct {
	ID       string
	Title    string
	Keywords []string
	Body     string
}

// Catalog bundles the parsed topic set. Construct once with Load() or
// LoadFS() at startup and share across goroutines: every field is
// immutable after Load returns, except that MergeGenerated may extend
// the topic set during single-threaded boot (under mu).
type Catalog struct {
	mu        sync.RWMutex // guards the maps + sorted slice during MergeGenerated
	sorted    []*Topic     // sorted by ID; supports binary-search prefix scans
	byID      map[string]*Topic
	byKeyword map[string]*Topic
}

var (
	ErrUnknownTopic   = errors.New("help: unknown topic")
	ErrAmbiguousTopic = errors.New("help: ambiguous topic prefix")
)

// Load parses the embedded topic assets. Returns an error if a file
// is missing required front-matter fields, or if two topics collide
// on id, keyword, or id-vs-keyword: collisions would silently shadow
// one topic and mask authoring mistakes.
//
// Equivalent to LoadFS(embeddedSub) where embeddedSub is fs.Sub of
// the package's bundled assets. Preserved for backwards compatibility;
// new boot paths should call LoadFS(SourceFS()) so the HELP_DIR
// override is honored.
func Load() (*Catalog, error) {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, fmt.Errorf("help: embedded sub: %w", err)
	}
	return LoadFS(sub)
}

// LoadFS parses topics from the given filesystem. The filesystem
// must expose `topics/<id>.md` files at its root; SourceFS provides
// that layout for both embedded and HELP_DIR-override modes. Returns
// an error on parse failure or topic collision (id, keyword, or
// cross-space).
func LoadFS(fsys fs.FS) (*Catalog, error) {
	dirEntries, err := fs.ReadDir(fsys, "topics")
	if err != nil {
		return nil, fmt.Errorf("help: read topics: %w", err)
	}
	topics := make([]*Topic, 0, len(dirEntries))
	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
			continue
		}
		raw, err := fs.ReadFile(fsys, "topics/"+de.Name())
		if err != nil {
			return nil, fmt.Errorf("help: %s: %w", de.Name(), err)
		}
		t, err := parseFrontMatter(string(raw))
		if err != nil {
			return nil, fmt.Errorf("help: %s: %w", de.Name(), err)
		}
		topics = append(topics, t)
	}
	sort.Slice(topics, func(i, j int) bool { return topics[i].ID < topics[j].ID })

	byID, byKeyword, err := validateAndIndex(topics)
	if err != nil {
		return nil, err
	}
	return &Catalog{sorted: topics, byID: byID, byKeyword: byKeyword}, nil
}

// validateAndIndex builds the byID / byKeyword maps and enforces the
// id/keyword collision rules. Shared by Load and test-only catalog
// builders so validation drift cannot creep in via duplicated logic.
func validateAndIndex(topics []*Topic) (byID, byKeyword map[string]*Topic, err error) {
	byID = make(map[string]*Topic, len(topics))
	byKeyword = make(map[string]*Topic)
	for _, t := range topics {
		if _, dup := byID[t.ID]; dup {
			return nil, nil, fmt.Errorf("help: duplicate topic id %q", t.ID)
		}
		if _, dup := byKeyword[t.ID]; dup {
			return nil, nil, fmt.Errorf("help: topic id %q collides with a keyword", t.ID)
		}
		byID[t.ID] = t
		for _, kw := range t.Keywords {
			if _, dup := byID[kw]; dup {
				return nil, nil, fmt.Errorf("help: keyword %q collides with a topic id", kw)
			}
			if existing, dup := byKeyword[kw]; dup {
				return nil, nil, fmt.Errorf("help: keyword %q used by both %q and %q", kw, existing.ID, t.ID)
			}
			byKeyword[kw] = t
		}
	}
	return byID, byKeyword, nil
}

// All returns every topic, sorted by id. The returned slice is a
// defensive copy so a caller's sort or append cannot corrupt the
// binary-search invariant on c.sorted (mirrors Prefix("") behaviour).
func (c *Catalog) All() []*Topic {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*Topic, len(c.sorted))
	copy(out, c.sorted)
	return out
}

// LookupExact returns the topic whose id exactly matches q. No keyword
// or prefix fallback. Used by help to give the command-registry exact
// match the same precedence as a topic-id exact match.
func (c *Catalog) LookupExact(q string) (*Topic, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.byID[strings.ToLower(strings.TrimSpace(q))]
	return t, ok
}

// LookupKeyword returns the topic whose keyword list contains q. No
// id, exact-id, or prefix fallback. Help calls this between the exact
// pass and the unique-prefix pass.
func (c *Catalog) LookupKeyword(q string) (*Topic, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.byKeyword[strings.ToLower(strings.TrimSpace(q))]
	return t, ok
}

// Lookup resolves q to a single topic via exact-id → keyword → unique
// id-prefix. Returns ErrUnknownTopic when nothing matches and
// ErrAmbiguousTopic (with comma-joined ids) when a prefix matches
// more than one. Mirrors telnet.Registry.Lookup.
func (c *Catalog) Lookup(q string) (*Topic, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil, ErrUnknownTopic
	}
	c.mu.RLock()
	if t, ok := c.byID[q]; ok {
		c.mu.RUnlock()
		return t, nil
	}
	if t, ok := c.byKeyword[q]; ok {
		c.mu.RUnlock()
		return t, nil
	}
	c.mu.RUnlock()
	matches := c.Prefix(q)
	switch len(matches) {
	case 0:
		return nil, ErrUnknownTopic
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrAmbiguousTopic, joinIDs(matches))
	}
}

// Prefix returns every topic whose id starts with p, sorted by id.
// Empty p returns the full catalog. Mirrors telnet.Registry.Prefix.
func (c *Catalog) Prefix(p string) []*Topic {
	p = strings.ToLower(p)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if p == "" {
		out := make([]*Topic, len(c.sorted))
		copy(out, c.sorted)
		return out
	}
	start := sort.Search(len(c.sorted), func(i int) bool { return c.sorted[i].ID >= p })
	var out []*Topic
	for i := start; i < len(c.sorted); i++ {
		if !strings.HasPrefix(c.sorted[i].ID, p) {
			break
		}
		out = append(out, c.sorted[i])
	}
	return out
}

// MergeGenerated extends the catalog with auto-derived topics — for
// example, per-command topics from cmd.GenerateCommandTopics. Topics
// whose ID matches an existing authored topic are skipped (authored
// wins so a hand-written article overrides the generated default).
// Topics whose ID collides with an existing keyword (in either
// direction) are also skipped to preserve the byID/byKeyword
// disjoint invariant from validateAndIndex.
//
// Returns (added, skipped). Skipped topics are logged at Debug so
// authors can audit which generated topics were shadowed.
//
// Intended for single-threaded boot use only: callers should
// MergeGenerated AFTER LoadFS and BEFORE the listener opens. The
// mutex guards reads against any concurrent caller that may have
// raced to start, but the catalog is not designed for steady-state
// reload — that's a future followup.
func (c *Catalog) MergeGenerated(gen []*Topic) (added, skipped int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range gen {
		if t == nil || t.ID == "" {
			continue
		}
		if _, dup := c.byID[t.ID]; dup {
			slog.Debug("help: generated topic shadowed by authored", "id", t.ID)
			skipped++
			continue
		}
		if _, dup := c.byKeyword[t.ID]; dup {
			slog.Debug("help: generated topic id collides with keyword", "id", t.ID)
			skipped++
			continue
		}
		// A keyword on the generated topic that collides with an
		// existing id or keyword is dropped from the topic; the
		// topic itself still lands as long as its ID is clean. This
		// matches the "additive, non-disruptive" intent — a
		// generated keyword shouldn't displace authored routing.
		clean := make([]string, 0, len(t.Keywords))
		for _, kw := range t.Keywords {
			if _, dup := c.byID[kw]; dup {
				continue
			}
			if _, dup := c.byKeyword[kw]; dup {
				continue
			}
			clean = append(clean, kw)
		}
		t.Keywords = clean
		c.byID[t.ID] = t
		for _, kw := range t.Keywords {
			c.byKeyword[kw] = t
		}
		// Insert into sorted slice maintaining order. Linear scan is
		// fine — MergeGenerated runs once at boot with O(commands)
		// topics, not in a hot path.
		idx := sort.Search(len(c.sorted), func(i int) bool { return c.sorted[i].ID >= t.ID })
		c.sorted = append(c.sorted, nil)
		copy(c.sorted[idx+1:], c.sorted[idx:])
		c.sorted[idx] = t
		added++
	}
	return added, skipped
}

func joinIDs(topics []*Topic) string {
	ids := make([]string, len(topics))
	for i, t := range topics {
		ids[i] = t.ID
	}
	return strings.Join(ids, ", ")
}

// parseFrontMatter parses a YAML-ish front-matter block from raw.
// Front-matter must be the first line, fenced by `---` lines, and
// must declare id and title. keywords is optional (comma-separated).
// Split out from parseTopic so tests can exercise edge cases without
// committing fixture files to the embedded asset tree.
func parseFrontMatter(raw string) (*Topic, error) {
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	// Front-matter must be the first line.
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing front-matter fence")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("unterminated front-matter")
	}

	t := &Topic{}
	for _, line := range lines[1:end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, fmt.Errorf("bad front-matter line %q (want key: value)", trimmed)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "id":
			t.ID = strings.ToLower(val)
		case "title":
			t.Title = val
		case "keywords":
			if val == "" {
				continue
			}
			for _, kw := range strings.Split(val, ",") {
				kw = strings.ToLower(strings.TrimSpace(kw))
				if kw != "" {
					t.Keywords = append(t.Keywords, kw)
				}
			}
		default:
			return nil, fmt.Errorf("unknown front-matter key %q", key)
		}
	}
	if t.ID == "" {
		return nil, fmt.Errorf("front-matter missing id")
	}
	if t.Title == "" {
		return nil, fmt.Errorf("front-matter missing title")
	}
	if !validTopicID(t.ID) {
		return nil, fmt.Errorf("invalid topic id %q (lowercase ASCII, no whitespace)", t.ID)
	}
	for _, kw := range t.Keywords {
		if !validTopicID(kw) {
			return nil, fmt.Errorf("invalid keyword %q (lowercase ASCII, no whitespace)", kw)
		}
	}

	body := strings.Join(lines[end+1:], "\n")
	t.Body = strings.TrimLeft(body, "\n")
	return t, nil
}

// validTopicID enforces the same shape as telnet.validVerb so a topic
// id (or keyword) can never collide ambiguously with a command verb at
// dispatch — both spaces use the same character class.
func validTopicID(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c >= 0x80 {
			return false
		}
		if c >= 'A' && c <= 'Z' {
			return false
		}
		switch c {
		case ' ', '\t', '\r', '\n', 0:
			return false
		}
		if c < 0x20 || c == 0x7F {
			return false
		}
	}
	return true
}
