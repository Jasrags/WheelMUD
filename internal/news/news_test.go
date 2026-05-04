package news

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cat.Splash() == "" {
		t.Error("splash is empty")
	}
	if cat.MOTD() == "" {
		t.Error("motd is empty")
	}
	if len(cat.Entries()) == 0 {
		t.Fatal("expected at least one news entry")
	}
}

func TestEntriesOrderedNewestFirst(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	entries := cat.Entries()
	for i := 1; i < len(entries); i++ {
		if entries[i].Date.After(entries[i-1].Date) {
			t.Fatalf("entries not in descending order: %s after %s",
				entries[i-1].ID, entries[i].ID)
		}
	}
}

func TestEntryLookup(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	first := cat.Entries()[0]

	got, ok := cat.Entry(first.ID)
	if !ok || got.ID != first.ID {
		t.Errorf("Entry(%q) = %+v ok=%v, want %+v", first.ID, got, ok, first)
	}

	got, ok = cat.Entry("1")
	if !ok || got.ID != first.ID {
		t.Errorf("Entry(\"1\") should resolve to newest entry; got %+v ok=%v", got, ok)
	}

	if _, ok := cat.Entry("9999-01-01"); ok {
		t.Error("Entry(missing) should return ok=false")
	}
	if _, ok := cat.Entry("0"); ok {
		t.Error("Entry(\"0\") should return ok=false (1-based)")
	}
	if _, ok := cat.Entry(""); ok {
		t.Error("Entry(\"\") should return ok=false")
	}
}

func TestUnreadCount(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	all := len(cat.Entries())

	if got := cat.UnreadCount(time.Time{}); got != all {
		t.Errorf("UnreadCount(zero) = %d, want %d (all unread)", got, all)
	}
	latest := cat.LatestDate()
	if got := cat.UnreadCount(latest); got != 0 {
		t.Errorf("UnreadCount(latest) = %d, want 0", got)
	}
	// One day before the newest entry → at least 1 unread.
	dayBefore := latest.Add(-24 * time.Hour)
	if got := cat.UnreadCount(dayBefore); got < 1 {
		t.Errorf("UnreadCount(latest-1d) = %d, want >= 1", got)
	}
}

func TestParseEntryRejectsBadDate(t *testing.T) {
	if _, err := parseEntry("not-a-date.md"); err == nil {
		t.Error("parseEntry should reject non-date prefix")
	}
}

func TestEntryTitleFromHeading(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, e := range cat.Entries() {
		if e.Title == "" {
			t.Errorf("entry %s has empty title", e.ID)
		}
		if strings.Contains(e.Title, "\n") {
			t.Errorf("entry %s title contains newline: %q", e.ID, e.Title)
		}
	}
}
