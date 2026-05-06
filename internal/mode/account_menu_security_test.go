package mode

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

func TestAccountMenu_SecurityEmptyHistory(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	logins := repo.NewMemoryAccountLoginRepo()
	f.menu.SetLogins(logins)
	time.Sleep(20 * time.Millisecond)
	f.captured.Reset()

	f.feed(rootSecurity)
	out := f.captured.String()
	if !strings.Contains(out, "Account security") {
		t.Fatalf("expected header, got %q", out)
	}
	if !strings.Contains(out, "no recent activity recorded") {
		t.Fatalf("expected empty-history notice, got %q", out)
	}
	if !strings.Contains(out, "(this session)") {
		t.Fatalf("expected this-session marker, got %q", out)
	}
	if !strings.Contains(out, "(no other sessions)") {
		t.Fatalf("expected no-other-sessions notice, got %q", out)
	}
}

func TestAccountMenu_SecurityShowsHistoryNewestFirst(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	logins := repo.NewMemoryAccountLoginRepo()
	f.menu.SetLogins(logins)

	ctx := context.Background()
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	seed := []repo.AccountLoginEntry{
		{AccountID: f.account.ID, At: base, RemoteAddress: "203.0.113.1", Outcome: repo.LoginOutcomeFailure, Info: "wrong password"},
		{AccountID: f.account.ID, At: base.Add(time.Minute), RemoteAddress: "203.0.113.1", Outcome: repo.LoginOutcomeSuccess},
		// Other-account row that must NOT leak in.
		{AccountID: f.account.ID + 99, At: base.Add(2 * time.Minute), RemoteAddress: "198.51.100.7", Outcome: repo.LoginOutcomeSuccess, Info: "other"},
	}
	for _, e := range seed {
		if err := logins.Record(ctx, e); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	time.Sleep(20 * time.Millisecond)
	f.captured.Reset()

	f.feed(rootSecurity)
	out := f.captured.String()

	// Newest-first: success (12:01) should appear before failure (12:00).
	successIdx := strings.Index(out, "success")
	failureIdx := strings.Index(out, "failure")
	if successIdx < 0 || failureIdx < 0 {
		t.Fatalf("missing outcomes in output: %q", out)
	}
	if successIdx >= failureIdx {
		t.Fatalf("expected success before failure, got %q", out)
	}
	if !strings.Contains(out, "wrong password") {
		t.Fatalf("expected info field rendered, got %q", out)
	}
	if strings.Contains(out, "198.51.100.7") {
		t.Fatalf("foreign-account row leaked into view: %q", out)
	}
}

func TestAccountMenu_SecurityKickNoOpWhenAlone(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	logins := repo.NewMemoryAccountLoginRepo()
	f.menu.SetLogins(logins)
	// Bind this session so the registry has something for Snapshot to
	// return; with only one binding, peerSessions is empty.
	f.registry.Bind(f.account.ID, f.session)
	t.Cleanup(func() { f.registry.Unbind(f.account.ID, f.session) })
	time.Sleep(20 * time.Millisecond)

	// Enter security substep.
	f.feed(rootSecurity)
	f.captured.Reset()

	f.feed("2")
	out := f.captured.String()
	if !strings.Contains(out, "No other sessions.") {
		t.Fatalf("expected no-op notice, got %q", out)
	}
	if logins.Len() != 0 {
		t.Fatalf("kick wrote login row when no peers: %d", logins.Len())
	}
	if got := f.audits.Len(); got != 0 {
		t.Fatalf("kick wrote audit row when no peers: %d", got)
	}
}

func TestAccountMenu_SecurityKickDisconnectsPeers(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	logins := repo.NewMemoryAccountLoginRepo()
	f.menu.SetLogins(logins)
	f.registry.Bind(f.account.ID, f.session)
	t.Cleanup(func() { f.registry.Unbind(f.account.ID, f.session) })

	// Stand up a second session bound to the same account.
	peerServer, peerClient := net.Pipe()
	t.Cleanup(func() { peerServer.Close(); peerClient.Close() })
	peerSession := telnet.NewSession(peerServer)
	peerSession.AccountID = f.account.ID
	peerCaptured := &safeBuf{}
	drainPeer(t, peerClient, peerCaptured)
	f.registry.Bind(f.account.ID, peerSession)

	time.Sleep(20 * time.Millisecond)
	f.feed(rootSecurity)
	f.captured.Reset()
	f.feed("2")
	time.Sleep(40 * time.Millisecond)

	out := f.captured.String()
	if !strings.Contains(out, "Kicked 1 other session.") {
		t.Fatalf("expected kick confirmation, got %q", out)
	}
	if !strings.Contains(peerCaptured.String(), "kicked from account menu") {
		t.Fatalf("peer didn't see kick notice: %q", peerCaptured.String())
	}
	// Audit row recorded.
	if got := f.audits.Len(); got != 1 {
		t.Fatalf("expected 1 audit row, got %d", got)
	}
	auditRows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{})
	if len(auditRows) != 1 || auditRows[0].Verb != "kick-sessions" {
		t.Fatalf("audit row mismatch: %+v", auditRows)
	}
	// One kick login row.
	if logins.Len() != 1 {
		t.Fatalf("expected 1 kick login row, got %d", logins.Len())
	}
	got, _ := logins.ListRecentByAccount(context.Background(), f.account.ID, 0)
	if len(got) != 1 || got[0].Outcome != repo.LoginOutcomeKick {
		t.Fatalf("login row mismatch: %+v", got)
	}
}

// TestLogin_RecordLoginEventNilLoginsIsNoOp pins the nil-guard
// behaviour of recordLoginEvent. A Login built without SetLogins must
// not panic on outcome paths and the surrounding RecordLoginFailure /
// RecordLoginSuccess must still run.
func TestLogin_RecordLoginEventNilLoginsIsNoOp(t *testing.T) {
	f := newLoginFixture(t)
	// Deliberately do NOT call f.login.SetLogins.

	f.feed("alice")
	f.feed("nope") // failure path — exercises recordLoginEvent(nil)

	got, _ := f.repo.FindByUsername(context.Background(), "alice")
	if got.FailedLoginCount != 1 {
		t.Fatalf("failure path didn't run with nil logins: count=%d", got.FailedLoginCount)
	}

	// Now succeed — exercises the success-path recordLoginEvent(nil).
	f.feed("alice")
	f.feed("correct-horse")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("expected AccountMenu after success, got %T", f.session.CurrentMode())
	}
}

func TestAccountMenu_SecurityDoneReturnsToRoot(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.menu.SetLogins(repo.NewMemoryAccountLoginRepo())
	time.Sleep(20 * time.Millisecond)

	f.feed(rootSecurity)
	f.feed("b")
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %v, want root", f.menu.step)
	}
}
