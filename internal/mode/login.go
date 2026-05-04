package mode

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/auth"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// LockoutThreshold is the number of failed attempts that trigger a
// temporary account lock. Tune via Login struct fields if needed.
const LockoutThreshold = 5

// LockoutDuration is how long an account stays locked after the
// threshold is hit.
const LockoutDuration = 15 * time.Minute

// loginStep names the current question the user is answering.
type loginStep int

const (
	stepUsername loginStep = iota
	stepPassword
)

// Login handles authentication. The state machine is two-step
// (username → password); on success it bumps the session's AuthLevel,
// stamps Session.AccountID, and hands off to postAuth which routes to
// CharacterSelect / CharacterCreate / game depending on character
// count. Each connection gets its own *Login because the captured
// username is per-session.
type Login struct {
	accounts   repo.AccountRepo
	characters repo.CharacterRepo
	sessions   *session.Registry
	game       telnet.Mode
	now        func() time.Time

	// lockoutThreshold and lockoutDuration are mutable so tests can
	// shrink them. Production callers leave the defaults.
	lockoutThreshold int
	lockoutDuration  time.Duration

	step     loginStep
	username string
	account  *repo.Account // resolved after step 1; nil when no such user
}

// NewLogin returns a fresh Login bound to accounts and characters.
// sessions enforces the single-session-per-account policy: a successful
// login disconnects the prior session for the same account. Pass a
// non-nil registry; tests that don't care about multi-session can pass
// session.NewRegistry().
// game is the in-world mode to promote successful logins into.
func NewLogin(accounts repo.AccountRepo, characters repo.CharacterRepo, sessions *session.Registry, game telnet.Mode) *Login {
	return &Login{
		accounts:         accounts,
		characters:       characters,
		sessions:         sessions,
		game:             game,
		now:              time.Now,
		lockoutThreshold: LockoutThreshold,
		lockoutDuration:  LockoutDuration,
		step:             stepUsername,
	}
}

func (l *Login) Prompt(_ context.Context, _ *telnet.Session) string {
	switch l.step {
	case stepUsername:
		return "Username (or 'new' to create an account): "
	case stepPassword:
		return "Password: "
	}
	return ""
}

func (l *Login) OnEnter(s *telnet.Session) error {
	s.InPasswordMode = false
	s.AuthLevel = telnet.AuthGuest
	return nil
}

func (l *Login) OnExit(s *telnet.Session) error {
	// Defensive: ensure password masking doesn't leak into the next mode.
	s.InPasswordMode = false
	return nil
}

func (l *Login) Handle(ctx context.Context, s *telnet.Session, line string) error {
	switch l.step {
	case stepUsername:
		return l.handleUsername(ctx, s, line)
	case stepPassword:
		return l.handlePassword(ctx, s, line)
	}
	return nil
}

func (l *Login) handleUsername(ctx context.Context, s *telnet.Session, line string) error {
	username := strings.TrimSpace(line)
	if username == "" {
		return nil
	}
	if strings.EqualFold(username, "new") {
		// Hand off to account-create mode. Login is replaced; if create
		// is canceled, the user reconnects.
		return s.ReplaceMode(NewCreate(l.accounts, l.characters, l.sessions, l.game))
	}

	// Resolve the account up front so we can check lockout *before*
	// burning bcrypt cycles. Whether the account exists must NOT alter
	// the prompt or response — see handlePassword for the symmetric
	// no-such-user path.
	a, err := l.accounts.FindByUsername(ctx, username)
	switch {
	case err == nil:
		l.account = &a
	case errors.Is(err, repo.ErrAccountNotFound):
		l.account = nil
	default:
		return s.WriteRaw([]byte("Login system unavailable. Try again later.\r\n"))
	}

	l.username = username
	l.step = stepPassword
	s.InPasswordMode = true
	return nil
}

func (l *Login) handlePassword(ctx context.Context, s *telnet.Session, line string) error {
	s.InPasswordMode = false

	if l.account == nil {
		// No such user. Sleep-equivalent: still write a uniform failure
		// so timing differences from the not-found short-circuit aren't
		// trivially observable. (We accept a small enumeration window;
		// see auth_followups.md.)
		return l.fail(s, "")
	}

	// Re-fetch the account so a parallel session that locked it between
	// our username step and password step is honored. The cached
	// l.account avoids leaking existence in the not-found branch above;
	// once we know the user exists, freshness wins.
	fresh, err := l.accounts.FindByUsername(ctx, l.username)
	switch {
	case err == nil:
		l.account = &fresh
	case errors.Is(err, repo.ErrAccountNotFound):
		// Account was deleted between steps. Treat as fail.
		return l.fail(s, "")
	default:
		return s.WriteRaw([]byte("Login system unavailable. Try again later.\r\n"))
	}

	if l.account.IsLockedAt(l.now()) {
		// Locked accounts skip the verify step entirely (saves bcrypt
		// CPU for repeated probes). Be explicit so users with a valid
		// password understand why they can't log in.
		return l.fail(s, "Account temporarily locked. Try again later.")
	}

	if !auth.Verify(l.account.PasswordHash, line) {
		newCount := l.account.FailedLoginCount + 1
		var lockedUntil time.Time
		var msg string
		if newCount >= l.lockoutThreshold {
			lockedUntil = l.now().Add(l.lockoutDuration)
			msg = "Too many failures. Account temporarily locked."
		}
		// Best-effort: a DB error here is logged via the wrapped error
		// path but should not block the login response.
		_ = l.accounts.RecordLoginFailure(ctx, l.account.ID, lockedUntil)
		return l.fail(s, msg)
	}

	// Success.
	if err := l.accounts.RecordLoginSuccess(ctx, l.account.ID, l.now()); err != nil {
		return s.WriteRaw([]byte("Login system unavailable. Try again later.\r\n"))
	}
	s.AccountID = l.account.ID
	// Login no longer earns a privilege — the session stays at
	// AuthGuest until postauth.promoteToGame stamps the chosen
	// character's auth_level. This lets one account own admin and
	// player characters side-by-side.
	// Single-session-per-account: bind this session in the registry
	// and disconnect any prior occupant. The previous session's read
	// loop will EOF on Conn.Close and tear down via the existing path.
	if prev := l.sessions.Bind(l.account.ID, s); prev != nil && prev != s {
		if err := prev.WriteRaw([]byte("\r\nDisconnected: logged in elsewhere.\r\n")); err != nil {
			slog.Debug("kick notice write failed", "remote", prev.RemoteAddress, "error", err)
		}
		if err := prev.Conn.Close(); err != nil {
			slog.Debug("kick close failed", "remote", prev.RemoteAddress, "error", err)
		}
	}
	if err := s.WriteRaw([]byte("Welcome, " + l.account.Username + ".\r\n")); err != nil {
		return err
	}
	return postAuth(ctx, s, l.characters, l.game)
}

// fail resets to the username step and writes a uniform failure
// message. extra is an optional second line (e.g. lockout notice).
func (l *Login) fail(s *telnet.Session, extra string) error {
	l.step = stepUsername
	l.account = nil
	l.username = ""
	out := "Login failed.\r\n"
	if extra != "" {
		out += extra + "\r\n"
	}
	return s.WriteRaw([]byte(out))
}
