package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// whoTestChars seeds a memory CharacterRepo with rows for the
// commPair fixtures (Alice id=1, Bob id=2) so collectWhoRows can
// resolve PvP state. Returns the repo so tests can flip PvP via
// RecordPvP.
func whoTestChars(t *testing.T) repo.CharacterRepo {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	if _, err := chars.Create(context.Background(), repo.Character{ID: 1, AccountID: 100, Name: "Alice"}); err != nil {
		t.Fatalf("seed Alice: %v", err)
	}
	if _, err := chars.Create(context.Background(), repo.Character{ID: 2, AccountID: 200, Name: "Bob"}); err != nil {
		t.Fatalf("seed Bob: %v", err)
	}
	return chars
}

// TestWho_RendersHeaderAndSelfMarker drives the verb end-to-end and
// checks the section header, the (you) marker, and the styled name
// payload. ColorLevelNone strips ANSI so substring assertions are
// stable.
func TestWho_RendersHeaderAndSelfMarker(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	alice.ColorLevel = telnet.ColorLevelNone
	alice.Width = 80

	runCmd(t, NewWho(sessions, whoTestChars(t)), alice, "")

	got := aOut.String()
	for _, want := range []string{
		"Players online (2)",
		"Alice",
		"(you)",
		"Bob",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in who output:\n%s", want, got)
		}
	}
}

// TestCollectWhoRows_HidesWizinvisFromNonAdmin / showsToAdmin
// directly exercises the visibility rule without going through the
// dispatcher.
func TestCollectWhoRows_HidesWizinvisFromNonAdmin(t *testing.T) {
	sessions, alice, bob, _, _ := commPair(t)
	bob.SetHidden(true)

	rows := collectWhoRows(context.Background(), alice, sessions.Snapshot(), time.Now().UTC(), whoTestChars(t))
	if len(rows) != 1 || rows[0].name != "Alice" {
		t.Fatalf("non-admin viewer should only see self; got %+v", rows)
	}
}

func TestCollectWhoRows_ShowsWizinvisToAdminWithMarker(t *testing.T) {
	sessions, alice, bob, _, _ := commPair(t)
	alice.AuthLevel = telnet.AuthAdmin
	bob.SetHidden(true)

	rows := collectWhoRows(context.Background(), alice, sessions.Snapshot(), time.Now().UTC(), whoTestChars(t))
	if len(rows) != 2 {
		t.Fatalf("admin should see both, got %d rows", len(rows))
	}
	var bobRow *whoRow
	for i := range rows {
		if rows[i].name == "Bob" {
			bobRow = &rows[i]
		}
	}
	if bobRow == nil {
		t.Fatal("Bob missing from admin rows")
	}
	if !bobRow.hidden {
		t.Fatalf("admin should see hidden marker on Bob; row=%+v", *bobRow)
	}
}

func TestCollectWhoRows_PreCharacterShowsConnecting(t *testing.T) {
	sessions := session.NewRegistry()
	guest, _ := bufSession(t)
	guest.AccountID = 1
	guest.AuthLevel = telnet.AuthGuest
	// CharacterName intentionally left empty to simulate login phase.
	sessions.Bind(guest.AccountID, guest)

	rows := collectWhoRows(context.Background(), guest, sessions.Snapshot(), time.Now().UTC(), repo.NewMemoryCharacterRepo())
	if len(rows) != 1 || rows[0].name != "(connecting)" {
		t.Fatalf("expected single (connecting) row; got %+v", rows)
	}
}

func TestFormatWhoRow_DefangsHostileName(t *testing.T) {
	got := formatWhoRow(whoRow{name: "Lan}}::red OWNED"})
	if strings.Contains(got, "}}::red") {
		t.Fatalf("defang failed, raw }}:: leaked: %q", got)
	}
}

func TestFormatWhoRow_IdleAndMarkers(t *testing.T) {
	got := formatWhoRow(whoRow{name: "Lan", you: true, hidden: true, idle: "3m"})
	for _, want := range []string{"Lan", "(you)", "*", "idle 3m"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

// TestFormatWhoRow_PvPTag pins the rendered tag for opted-in rows
// and confirms it stays absent otherwise.
func TestFormatWhoRow_PvPTag(t *testing.T) {
	on := formatWhoRow(whoRow{name: "Lan", pvp: true})
	if !strings.Contains(on, "[PvP]") {
		t.Fatalf("missing [PvP] tag on opted-in row: %q", on)
	}
	off := formatWhoRow(whoRow{name: "Lan"})
	if strings.Contains(off, "[PvP]") {
		t.Fatalf("unexpected [PvP] tag on opted-out row: %q", off)
	}
}

// TestCollectWhoRows_PvPTagFromCharacterRepo wires collectWhoRows
// against a CharacterRepo with one peer flagged PvP and asserts the
// row carries the flag while the other row does not. Source-of-truth
// is the same repo `attack` reads, so toggling pvp on/off is visible
// to peers without a session-side cache.
func TestCollectWhoRows_PvPTagFromCharacterRepo(t *testing.T) {
	sessions, alice, _, _, _ := commPair(t)
	chars := whoTestChars(t)
	if err := chars.RecordPvP(context.Background(), 2, true); err != nil {
		t.Fatalf("RecordPvP Bob: %v", err)
	}

	rows := collectWhoRows(context.Background(), alice, sessions.Snapshot(), time.Now().UTC(), chars)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		switch r.name {
		case "Alice":
			if r.pvp {
				t.Fatalf("Alice should be PvP-off; row=%+v", r)
			}
		case "Bob":
			if !r.pvp {
				t.Fatalf("Bob should be PvP-on; row=%+v", r)
			}
		}
	}
}
