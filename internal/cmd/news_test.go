package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/news"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

func newsFixture(t *testing.T) (*news.Catalog, repo.CharacterRepo, *telnet.Session, *bufConn, repo.Character) {
	t.Helper()
	cat, err := news.Load()
	if err != nil {
		t.Fatalf("news.Load: %v", err)
	}
	cr := repo.NewMemoryCharacterRepo()
	ar := repo.NewMemoryAccountRepo()
	acc, _ := ar.Create(context.Background(), repo.Account{Username: "owner", PasswordHash: "h"})
	c, err := cr.Create(context.Background(), repo.Character{AccountID: acc.ID, Name: "Reader"})
	if err != nil {
		t.Fatalf("character create: %v", err)
	}
	s, conn := bufSession(t)
	s.AccountID = acc.ID
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterID = c.ID
	s.CharacterName = c.Name
	return cat, cr, s, conn, c
}

func TestNewsList_MarksUnreadWhenWatermarkZero(t *testing.T) {
	cat, cr, s, out, _ := newsFixture(t)
	cmd := NewNews(cat, cr)

	runCmd(t, cmd, s, "")

	body := out.String()
	if !strings.Contains(body, "[*]") {
		t.Errorf("expected [*] unread marker in output, got:\n%s", body)
	}
	if !strings.Contains(body, cat.Entries()[0].ID) {
		t.Errorf("expected entry id %q in list, got:\n%s", cat.Entries()[0].ID, body)
	}
}

func TestNewsRead_BumpsWatermarkAndClearsMarker(t *testing.T) {
	cat, cr, s, out, c := newsFixture(t)
	cmd := NewNews(cat, cr)
	first := cat.Entries()[0]

	runCmd(t, cmd, s, first.ID)

	got, err := cr.FindByName(context.Background(), c.Name)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !got.LastNewsSeen.Equal(first.Date) {
		t.Errorf("LastNewsSeen = %v, want %v", got.LastNewsSeen, first.Date)
	}
	if !s.LastNewsSeen().Equal(first.Date) {
		t.Errorf("session mirror not updated: %v", s.LastNewsSeen())
	}

	out.Reset()
	runCmd(t, cmd, s, "")
	if strings.Contains(out.String(), "[*]") {
		t.Errorf("expected no unread marker after reading newest entry, got:\n%s", out.String())
	}
}

func TestNewsRead_ByIndex(t *testing.T) {
	cat, cr, s, out, _ := newsFixture(t)
	cmd := NewNews(cat, cr)

	runCmd(t, cmd, s, "1")

	if out.String() == "" {
		t.Error("expected entry body to be written")
	}
	if !s.LastNewsSeen().Equal(cat.Entries()[0].Date) {
		t.Errorf("watermark not bumped: %v", s.LastNewsSeen())
	}
}

func TestNewsRead_UnknownEntry(t *testing.T) {
	cat, cr, s, out, _ := newsFixture(t)
	cmd := NewNews(cat, cr)

	runCmd(t, cmd, s, "9999-12-31")

	if !strings.Contains(out.String(), "No such") {
		t.Errorf("expected 'No such' error, got: %s", out.String())
	}
	if !s.LastNewsSeen().IsZero() {
		t.Errorf("watermark should not have moved on missing entry; got %v", s.LastNewsSeen())
	}
}

func TestNewsRead_StaleEntryDoesNotRegress(t *testing.T) {
	cat, cr, s, _, c := newsFixture(t)
	// Pre-stamp watermark to a future date so reading any entry would
	// otherwise regress it.
	future := time.Now().Add(48 * time.Hour)
	if err := cr.MarkNewsSeen(context.Background(), c.ID, future); err != nil {
		t.Fatalf("seed watermark: %v", err)
	}
	s.SetLastNewsSeen(future)

	cmd := NewNews(cat, cr)
	runCmd(t, cmd, s, cat.Entries()[0].ID)

	got, _ := cr.FindByName(context.Background(), c.Name)
	if !got.LastNewsSeen.Equal(future) {
		t.Errorf("stale read regressed watermark: got %v want %v", got.LastNewsSeen, future)
	}
}

func TestNewsCompleter_OffersEntryIDs(t *testing.T) {
	cat, cr, s, _, _ := newsFixture(t)
	cmd := NewNews(cat, cr)

	cands := cmd.Completer(s, "")
	if len(cands) != len(cat.Entries()) {
		t.Errorf("expected %d candidates, got %d", len(cat.Entries()), len(cands))
	}
	if cands[0].Text != cat.Entries()[0].ID {
		t.Errorf("first candidate = %q, want %q", cands[0].Text, cat.Entries()[0].ID)
	}
}
