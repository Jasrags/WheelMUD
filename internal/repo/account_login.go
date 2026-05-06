package repo

import (
	"context"
	"time"
)

// AccountLoginEntry is one row in the per-account authentication-event
// log (migration 0036). Append-only: callers Record on every login
// outcome (success / failure / lockout) and on a kick-sessions action
// from the account menu.
//
// RemoteAddress is the host portion of the connecting socket address;
// the dialler-side port is stripped before Record (see
// internal/mode.remoteHost). Outcome is one of the OutcomeXxx constants
// below; Info is a short fixed-vocabulary note (e.g. "wrong password",
// "kicked by other-session") that NEVER contains the typed password.
type AccountLoginEntry struct {
	ID            int64
	AccountID     int64
	At            time.Time
	RemoteAddress string
	Outcome       string
	Info          string
}

// Outcome values stored in account_logins.outcome.
const (
	LoginOutcomeSuccess = "success"
	LoginOutcomeFailure = "failure"
	LoginOutcomeLockout = "lockout"
	LoginOutcomeKick    = "kick"
)

// DefaultAccountLoginListLimit caps an unbounded ListRecentByAccount.
// The §6 security view passes 10; this default exists so a misuse with
// limit <= 0 doesn't accidentally page in the whole table.
const DefaultAccountLoginListLimit = 50

// AccountLoginRepo persists per-account authentication events.
// Append-only: no Update, no Delete. Read paths return newest first.
type AccountLoginRepo interface {
	Record(ctx context.Context, e AccountLoginEntry) error
	ListRecentByAccount(ctx context.Context, accountID int64, limit int) ([]AccountLoginEntry, error)
}
