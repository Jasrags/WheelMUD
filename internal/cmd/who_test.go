package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// TestWho_RendersHeaderAndSelfMarker drives the verb end-to-end and
// checks the section header, the (you) marker, and the styled name
// payload. ColorLevelNone strips ANSI so substring assertions are
// stable.
func TestWho_RendersHeaderAndSelfMarker(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	alice.ColorLevel = telnet.ColorLevelNone
	alice.Width = 80

	runCmd(t, NewWho(sessions), alice, "")

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

	rows := collectWhoRows(alice, sessions.Snapshot(), time.Now().UTC())
	if len(rows) != 1 || rows[0].name != "Alice" {
		t.Fatalf("non-admin viewer should only see self; got %+v", rows)
	}
}

func TestCollectWhoRows_ShowsWizinvisToAdminWithMarker(t *testing.T) {
	sessions, alice, bob, _, _ := commPair(t)
	alice.AuthLevel = telnet.AuthAdmin
	bob.SetHidden(true)

	rows := collectWhoRows(alice, sessions.Snapshot(), time.Now().UTC())
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

	rows := collectWhoRows(guest, sessions.Snapshot(), time.Now().UTC())
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
