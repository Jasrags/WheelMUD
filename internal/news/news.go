// Package news exposes the embedded splash banner, MOTD, and dated
// news entries shown on connect and login.
//
// Layout:
//
//	assets/splash.txt        — banner shown before the login prompt.
//	assets/motd.md           — short blurb shown on game entry.
//	assets/news/YYYY-MM-DD-*.md — dated entries; filename prefix is
//	                              the canonical id and sort key.
//
// Splash and MOTD are inert text. News entries are sorted descending
// by date so the most recent shows first; UnreadCount(after) drives
// the login banner. Watermark persistence lives on the character row
// (last_news_seen, migration 0027); this package is read-only.
package news

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed assets/splash.txt assets/motd.md assets/news/*.md
var assets embed.FS

// idDateFormat is the canonical date format for news entry filenames
// and entry ids ("YYYY-MM-DD"). Filenames may carry a -slug suffix
// (`2026-05-04-launch.md`) but the id is always just the date.
const idDateFormat = "2006-01-02"

// Entry is one dated news item. ID is the YYYY-MM-DD prefix from the
// filename; Title is the first markdown heading (`# ...`) when
// present, otherwise the file's slug; Body is the full file contents
// rendered verbatim through cfmt.
//
// TRUST BOUNDARY: Body is operator-curated content embedded at build
// time. It is rendered with cfmt tags interpreted ({{...}}::style),
// so it MUST NOT include strings derived from player input. Any
// future path that wants to surface player-supplied text through the
// news/MOTD pipeline must escape cfmt's `{{`/`}}::` syntax first.
type Entry struct {
	ID    string
	Date  time.Time
	Title string
	Body  string
}

// Catalog bundles the parsed splash/MOTD/news set. Construct once
// with Load() at startup and share across goroutines: all fields are
// immutable after Load returns.
type Catalog struct {
	splash  string
	motd    string
	entries []Entry // sorted by Date descending
}

// Load parses the embedded assets. Returns an error if a news entry
// filename can't be parsed as YYYY-MM-DD-*.md, since a typo there
// would silently misorder the catalog.
func Load() (*Catalog, error) {
	splash, err := readString("assets/splash.txt")
	if err != nil {
		return nil, fmt.Errorf("news: load splash: %w", err)
	}
	motd, err := readString("assets/motd.md")
	if err != nil {
		return nil, fmt.Errorf("news: load motd: %w", err)
	}
	entries, err := loadEntries()
	if err != nil {
		return nil, err
	}
	return &Catalog{splash: splash, motd: motd, entries: entries}, nil
}

// Splash returns the banner rendered at TCP-accept time.
func (c *Catalog) Splash() string { return c.splash }

// MOTD returns the message-of-the-day rendered at game entry.
func (c *Catalog) MOTD() string { return c.motd }

// Entries returns the news catalog, newest first. The returned
// slice is shared — callers must not mutate it.
func (c *Catalog) Entries() []Entry { return c.entries }

// Entry resolves an entry by id (YYYY-MM-DD) or by 1-based index
// from the descending list ("1" = newest). Returns ok=false when the
// argument matches neither.
func (c *Catalog) Entry(arg string) (Entry, bool) {
	if arg == "" {
		return Entry{}, false
	}
	for _, e := range c.entries {
		if e.ID == arg {
			return e, true
		}
	}
	// Numeric index fallback: 1-based, descending order. strconv.Atoi
	// rejects non-digits, leading whitespace, and overflows; the
	// bounds check below handles the negative/zero cases for us.
	if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= len(c.entries) {
		return c.entries[n-1], true
	}
	return Entry{}, false
}

// UnreadCount returns the number of entries dated strictly after
// the given watermark. A zero watermark (never seen) marks every
// entry unread.
func (c *Catalog) UnreadCount(after time.Time) int {
	n := 0
	for _, e := range c.entries {
		if e.Date.After(after) {
			n++
		}
	}
	return n
}

// LatestDate returns the newest entry's date, or zero time when the
// catalog is empty. Useful as the watermark to stamp after a player
// reads the full list.
func (c *Catalog) LatestDate() time.Time {
	if len(c.entries) == 0 {
		return time.Time{}
	}
	return c.entries[0].Date
}

func readString(name string) (string, error) {
	b, err := assets.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func loadEntries() ([]Entry, error) {
	dirEntries, err := fs.ReadDir(assets, "assets/news")
	if err != nil {
		return nil, fmt.Errorf("news: read entries: %w", err)
	}
	out := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
			continue
		}
		entry, err := parseEntry(de.Name())
		if err != nil {
			return nil, fmt.Errorf("news: %s: %w", de.Name(), err)
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.After(out[j].Date)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func parseEntry(filename string) (Entry, error) {
	if len(filename) < len(idDateFormat) {
		return Entry{}, fmt.Errorf("filename too short for YYYY-MM-DD prefix")
	}
	idPart := filename[:len(idDateFormat)]
	date, err := time.Parse(idDateFormat, idPart)
	if err != nil {
		return Entry{}, fmt.Errorf("bad date prefix: %w", err)
	}
	body, err := readString("assets/news/" + filename)
	if err != nil {
		return Entry{}, err
	}
	title := firstHeading(body)
	if title == "" {
		// Fall back to the filename slug (after the date prefix and
		// the joining `-`, before `.md`).
		slug := strings.TrimSuffix(filename[len(idDateFormat):], ".md")
		title = strings.TrimPrefix(slug, "-")
	}
	return Entry{ID: idPart, Date: date, Title: title, Body: body}, nil
}

func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

