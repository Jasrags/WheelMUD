package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/news"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewNews builds the §18 news command. With no args it lists dated
// entries, marking unread ones with `[*]`. With an id (YYYY-MM-DD) or
// a 1-based index it renders the body and bumps the character's
// last_news_seen watermark via CharacterRepo.MarkNewsSeen.
func NewNews(catalog *news.Catalog, characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name: "news",
		Help: "List dated news entries; pass an id or index to read one",
		Long: "Usage: news               — list entries (newest first)\n" +
			"       news <id|index>    — read one (id is YYYY-MM-DD, index is 1-based)\n\n" +
			"Reading an entry advances your last-seen watermark; older\n" +
			"entries cannot regress it.",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				return writeNewsList(c.Session, catalog)
			}
			return writeNewsEntry(c, catalog, characters, c.Args[0])
		},
		Completer: func(s *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			if slot != 0 {
				return nil
			}
			out := make([]telnet.Candidate, 0, len(catalog.Entries()))
			for _, e := range catalog.Entries() {
				if !strings.HasPrefix(e.ID, partial) {
					continue
				}
				out = append(out, telnet.Candidate{Text: e.ID, Help: e.Title})
			}
			return out
		},
	}
}

func writeNewsList(s *telnet.Session, catalog *news.Catalog) error {
	entries := catalog.Entries()
	if len(entries) == 0 {
		return s.WriteString("{{No news entries.}}::yellow\r\n")
	}
	var b strings.Builder
	b.WriteString("{{== News ==}}::cyan|bold\r\n")
	for i, e := range entries {
		marker := "   "
		if e.Date.After(s.LastNewsSeen()) {
			marker = "[*]"
		}
		fmt.Fprintf(&b, "  %s %2d. %s  -  %s\r\n", marker, i+1, e.ID, e.Title)
	}
	b.WriteString("Type {{news <id>}}::yellow or {{news <n>}}::yellow to read one.\r\n")
	return s.WritePagedWrapped(b.String())
}

func writeNewsEntry(c *telnet.Context, catalog *news.Catalog, characters repo.CharacterRepo, arg string) error {
	entry, ok := catalog.Entry(arg)
	if !ok {
		return c.Session.WriteString("{{No such news entry.}}::yellow\r\n")
	}
	body := strings.TrimRight(entry.Body, "\r\n") + "\r\n"
	if err := c.Session.WritePagedWrapped(body); err != nil {
		return err
	}
	// Watermark bump (best-effort): MarkNewsSeen clamps so a stale
	// read can't regress, and the session's mirror is only updated
	// on success so a DB error keeps the unread flag in the list.
	if c.Session.CharacterID == 0 {
		return nil
	}
	if err := characters.MarkNewsSeen(c.Ctx, c.Session.CharacterID, entry.Date); err != nil {
		slog.Warn("news: MarkNewsSeen failed",
			"character", c.Session.CharacterID, "entry", entry.ID, "error", err)
		return nil
	}
	c.Session.SetLastNewsSeen(entry.Date)
	return nil
}
