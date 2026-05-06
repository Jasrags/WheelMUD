package mode

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// security view constants. recentLoginsLimit caps the history block at
// a screen-friendly count; the underlying table is unbounded, so this
// is purely a presentation choice.
const (
	recentLoginsLimit = 10
	securityTSFormat  = "2006-01-02 15:04"
)

// handleSecurityEnter pivots to the security sub-step and renders the
// view. The substep dispatcher (handleSecurityLine) owns the numbered
// picker — typed verbs are gone in the slice-4 UI rework.
//
// The root verb dispatch lives in account_menu.go and doesn't have ctx
// in scope, so the entry render uses context.Background(). Subsequent
// renders triggered from handleSecurityLine propagate the dispatcher
// ctx so a session teardown mid-read cancels the SQLite query.
func (m *AccountMenu) handleSecurityEnter(s *telnet.Session) error {
	m.step = accountStepSecurity
	return m.writeSecurityView(context.Background(), s)
}

// handleSecurityLine dispatches commands inside the security sub-step.
//
//	1                 refresh
//	2                 kick other sessions
//	[B]ack / blank    return to root
func (m *AccountMenu) handleSecurityLine(ctx context.Context, s *telnet.Session, line string) error {
	if isBack(line) {
		return m.returnToRoot(s)
	}
	idx, err := parsePositiveIndex(strings.TrimSpace(line), 2)
	if err != nil {
		if werr := writeError(s, "Invalid choice. [1-2] or [B]ack."); werr != nil {
			return werr
		}
		return nil
	}
	switch idx {
	case 0:
		return m.writeSecurityView(ctx, s)
	case 1:
		return m.handleSecurityKick(ctx, s)
	}
	return nil
}

// writeSecurityView renders two sections: recent activity (last
// recentLoginsLimit entries from account_logins) and the live active
// sessions for this account from the registry snapshot. Both sections
// degrade gracefully when their backing data is empty or the
// dependency wasn't wired.
func (m *AccountMenu) writeSecurityView(ctx context.Context, s *telnet.Session) error {
	if err := display.SectionHeader(s, "Account security"); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("  Recent activity:\r\n")); err != nil {
		return err
	}
	if err := m.writeRecentLogins(ctx, s); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("\r\n  Active sessions:\r\n")); err != nil {
		return err
	}
	if err := m.writeActiveSessions(s); err != nil {
		return err
	}
	if err := display.Rule(s); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("    1) Refresh\r\n    2) Kick other sessions\r\n\r\n  [B]ack\r\n")); err != nil {
		return err
	}
	return s.WriteRaw([]byte("[1-2] >\r\n"))
}

func (m *AccountMenu) writeRecentLogins(ctx context.Context, s *telnet.Session) error {
	if m.logins == nil || s.AccountID == 0 {
		return s.WriteRaw([]byte("    (history unavailable)\r\n"))
	}
	entries, err := m.logins.ListRecentByAccount(ctx, s.AccountID, recentLoginsLimit)
	if err != nil {
		slog.Warn("account_menu: list recent logins failed",
			"account", s.AccountID, "err", err)
		return s.WriteRaw([]byte("    (could not load history)\r\n"))
	}
	if len(entries) == 0 {
		return s.WriteRaw([]byte("    (no recent activity recorded)\r\n"))
	}
	loc := m.displayLocation()
	var b strings.Builder
	for _, e := range entries {
		ip := e.RemoteAddress
		if ip == "" {
			ip = "(unknown)"
		}
		extra := ""
		if e.Info != "" {
			extra = "  " + e.Info
		}
		fmt.Fprintf(&b, "    %s   %-7s %s%s\r\n",
			e.At.In(loc).Format(securityTSFormat),
			e.Outcome, ip, extra)
	}
	return s.WriteRaw([]byte(b.String()))
}

func (m *AccountMenu) writeActiveSessions(s *telnet.Session) error {
	if m.session == nil || s.AccountID == 0 {
		return s.WriteRaw([]byte("    (registry unavailable)\r\n"))
	}
	others := m.peerSessions(s)
	if err := s.WriteRaw([]byte(fmt.Sprintf(
		"    %s   (this session)\r\n", remoteHost(s.RemoteAddress)))); err != nil {
		return err
	}
	if len(others) == 0 {
		return s.WriteRaw([]byte("    (no other sessions)\r\n"))
	}
	var b strings.Builder
	for _, peer := range others {
		fmt.Fprintf(&b, "    %s\r\n", remoteHost(peer.RemoteAddress))
	}
	return s.WriteRaw([]byte(b.String()))
}

// peerSessions returns every bound session whose AccountID matches the
// caller's, excluding the caller itself. With single-session-per-account
// enforced by Login/Create today this is always empty; the path is
// forward-wired for the multi-session future.
func (m *AccountMenu) peerSessions(s *telnet.Session) []*telnet.Session {
	if m.session == nil || s.AccountID == 0 {
		return nil
	}
	var out []*telnet.Session
	for _, peer := range m.session.Snapshot() {
		if peer == nil || peer == s {
			continue
		}
		if peer.AccountID != s.AccountID {
			continue
		}
		out = append(out, peer)
	}
	return out
}

// handleSecurityKick disconnects every other bound session for this
// account, recording one account_logins row per kicked session and
// one admin_audit (account-mode) row summarising the action. With
// single-session-per-account this is currently a no-op; the path is
// forward-wired so multi-session work doesn't need a second pass.
func (m *AccountMenu) handleSecurityKick(ctx context.Context, s *telnet.Session) error {
	peers := m.peerSessions(s)
	if len(peers) == 0 {
		if err := s.WriteRaw([]byte("No other sessions.\r\n")); err != nil {
			return err
		}
		return m.writeSecurityView(ctx, s)
	}
	for _, peer := range peers {
		if err := peer.WriteRaw([]byte("\r\nDisconnected: kicked from account menu.\r\n")); err != nil {
			slog.Debug("kick notice write failed",
				"remote", peer.RemoteAddress, "err", err)
		}
		if err := peer.Conn.Close(); err != nil {
			slog.Debug("kick close failed",
				"remote", peer.RemoteAddress, "err", err)
		}
		if m.logins != nil {
			if rerr := m.logins.Record(ctx, repo.AccountLoginEntry{
				AccountID:     s.AccountID,
				RemoteAddress: remoteHost(peer.RemoteAddress),
				Outcome:       repo.LoginOutcomeKick,
				Info:          "kicked by other-session",
			}); rerr != nil {
				slog.Warn("account_menu: record kick login failed",
					"account", s.AccountID, "err", rerr)
			}
		}
	}
	audit.RecordAccount(ctx, m.audits, s.AccountID, m.accountUsername,
		"kick-sessions", "", fmt.Sprintf("count=%d", len(peers)))
	plural := "session"
	if len(peers) != 1 {
		plural = "sessions"
	}
	if err := display.OK(s, fmt.Sprintf(
		"Kicked %d other %s.", len(peers), plural)); err != nil {
		return err
	}
	return m.writeSecurityView(ctx, s)
}
