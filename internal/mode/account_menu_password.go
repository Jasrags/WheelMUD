package mode

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/auth"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/telnet"
)

// handlePasswordEnter opens the change-password substep flow. It
// renders the section header, enters password-mode immediately so the
// very next keystroke (the current-password line) is masked, and
// advances to the current-password step. The dispatcher repaints the
// step-aware Prompt() automatically.
//
// Refuses with a "not configured" notice when the account repo isn't
// wired (memory-only test paths). Production wiring always supplies it.
func (m *AccountMenu) handlePasswordEnter(s *telnet.Session) error {
	if m.accounts == nil {
		if werr := writeError(s, "Password change is not configured on this server."); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	if err := display.SectionHeader(s, "Change password"); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("  Type a blank line, 'cancel', or [B]ack at any prompt to abort.\r\n\r\n")); err != nil {
		return err
	}
	m.step = accountStepCurrentPassword
	s.SetPasswordMode(true)
	return nil
}

// handleCurrentPassword runs at accountStepCurrentPassword. cancel /
// blank / back → reset to root. Otherwise re-fetch the account by the
// snapshotted username (cheaper and safer than caching the hash on
// the menu) and bcrypt-verify. Mismatch returns to root with a generic
// notice; the user re-picks "Change password" to retry. Match advances
// to accountStepNewPassword keeping password mode on.
func (m *AccountMenu) handleCurrentPassword(ctx context.Context, s *telnet.Session, line string) error {
	if isCancelOrBlank(line) {
		m.resetPasswordFlow(s)
		return m.returnToRoot(s)
	}
	if m.accounts == nil {
		// Defensive: handlePasswordEnter guards on this, but if accounts is
		// cleared mid-flow, fail closed.
		m.resetPasswordFlow(s)
		if werr := writeError(s, "Password change is not configured on this server."); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	a, err := m.accounts.FindByUsername(ctx, m.accountUsername)
	if err != nil {
		slog.Warn("account_menu: find account for password change failed",
			"account", s.AccountID, "err", err)
		m.resetPasswordFlow(s)
		if werr := writeError(s, "Could not change password. Try again later."); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	if !auth.Verify(a.PasswordHash, line) {
		m.resetPasswordFlow(s)
		if werr := writeError(s, "Current password did not match."); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	m.step = accountStepNewPassword
	// Password mode stays on for the next prompt.
	return nil
}

// handleNewPassword runs at accountStepNewPassword. cancel / blank →
// reset to root. Otherwise hash via auth.Hash and stash on the menu.
// Length-policy error wording matches Create.handlePassword so a user
// who hits a too-short / too-long during chargen sees the same notice
// during rotation.
func (m *AccountMenu) handleNewPassword(s *telnet.Session, line string) error {
	if isCancelOrBlank(line) {
		m.resetPasswordFlow(s)
		return m.returnToRoot(s)
	}
	hash, err := auth.Hash(line)
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		m.resetPasswordFlow(s)
		if werr := s.WriteRaw([]byte("Password too short (minimum 8 characters).\r\n")); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	case errors.Is(err, auth.ErrPasswordTooLong):
		m.resetPasswordFlow(s)
		if werr := s.WriteRaw([]byte("Password too long (maximum 72 bytes).\r\n")); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	case err != nil:
		m.resetPasswordFlow(s)
		if werr := s.WriteRaw([]byte("Could not process password. Try again.\r\n")); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	m.pendingNewHash = hash
	m.step = accountStepConfirmNewPassword
	// Password mode stays on for the confirm prompt.
	return nil
}

// handleConfirmNewPassword runs at accountStepConfirmNewPassword.
// Mirrors Create.handleConfirm: clears password mode first thing, then
// verifies the typed line against the stashed hash. Match → persist
// + audit + "Password changed.". Any other outcome (cancel, mismatch,
// repo failure) returns to root with the appropriate notice.
func (m *AccountMenu) handleConfirmNewPassword(ctx context.Context, s *telnet.Session, line string) error {
	s.SetPasswordMode(false)
	if isCancelOrBlank(line) {
		m.resetPasswordFlow(s)
		return m.returnToRoot(s)
	}
	if !auth.Verify(m.pendingNewHash, line) {
		m.resetPasswordFlow(s)
		if werr := writeError(s, "Passwords did not match."); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	if err := m.accounts.UpdatePasswordHash(ctx, s.AccountID, m.pendingNewHash); err != nil {
		slog.Warn("account_menu: password update failed",
			"account", s.AccountID, "err", err)
		m.resetPasswordFlow(s)
		if werr := writeError(s, "Could not change password. Try again later."); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	audit.RecordAccount(ctx, m.audits, s.AccountID, m.accountUsername,
		"change-password", "", "")
	m.resetPasswordFlow(s)
	if err := display.OK(s, "Password changed."); err != nil {
		return err
	}
	return m.returnToRoot(s)
}

// resetPasswordFlow clears in-progress state and exits password mode.
// Idempotent — safe to call from any step (including from within
// handleConfirmNewPassword which already turned masking off).
func (m *AccountMenu) resetPasswordFlow(s *telnet.Session) {
	m.step = accountStepRoot
	m.pendingNewHash = ""
	s.SetPasswordMode(false)
}
