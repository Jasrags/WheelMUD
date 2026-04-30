package mode

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/Jasrags/WheelMUD/internal/auth"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// Username policy — kept conservative to dodge Unicode lookalike issues
// until NFKC normalization lands (see persistence_followups.md).
const (
	minUsernameLen = 3
	maxUsernameLen = 24
)

type createStep int

const (
	createStepUsername createStep = iota
	createStepPassword
	createStepConfirm
)

// Create is the account-creation mode reachable from Login when the
// user types "new". On success it inserts the account, bumps the
// session's AuthLevel, and replaces itself with the next mode.
type Create struct {
	accounts repo.AccountRepo
	next     telnet.Mode

	step     createStep
	username string
	hash     string
}

// NewCreate returns a fresh account-creation mode. next is the mode to
// switch to after the account is persisted (typically Game).
func NewCreate(accounts repo.AccountRepo, next telnet.Mode) *Create {
	return &Create{accounts: accounts, next: next}
}

func (c *Create) Prompt(_ *telnet.Session) string {
	switch c.step {
	case createStepUsername:
		return "Choose a username: "
	case createStepPassword:
		return "Choose a password: "
	case createStepConfirm:
		return "Confirm password: "
	}
	return ""
}

func (c *Create) OnEnter(s *telnet.Session) error {
	s.InPasswordMode = false
	return nil
}

func (c *Create) OnExit(s *telnet.Session) error {
	s.InPasswordMode = false
	return nil
}

func (c *Create) Handle(ctx context.Context, s *telnet.Session, line string) error {
	switch c.step {
	case createStepUsername:
		return c.handleUsername(s, line)
	case createStepPassword:
		return c.handlePassword(s, line)
	case createStepConfirm:
		return c.handleConfirm(ctx, s, line)
	}
	return nil
}

func (c *Create) handleUsername(s *telnet.Session, line string) error {
	username := strings.TrimSpace(line)
	if err := validateUsername(username); err != nil {
		return s.WriteRaw([]byte(err.Error() + "\r\n"))
	}
	c.username = username
	c.step = createStepPassword
	s.InPasswordMode = true
	return nil
}

func (c *Create) handlePassword(s *telnet.Session, line string) error {
	hash, err := auth.Hash(line)
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		return s.WriteRaw([]byte("Password too short (minimum 8 characters).\r\n"))
	case errors.Is(err, auth.ErrPasswordTooLong):
		return s.WriteRaw([]byte("Password too long (maximum 72 bytes).\r\n"))
	case err != nil:
		return s.WriteRaw([]byte("Could not process password. Try again.\r\n"))
	}
	c.hash = hash
	c.step = createStepConfirm
	// InPasswordMode remains true through the confirm step.
	return nil
}

func (c *Create) handleConfirm(ctx context.Context, s *telnet.Session, line string) error {
	s.InPasswordMode = false
	if !auth.Verify(c.hash, line) {
		// Reset to the password step; keep the chosen username.
		c.hash = ""
		c.step = createStepPassword
		s.InPasswordMode = true
		return s.WriteRaw([]byte("Passwords did not match. Try again.\r\n"))
	}

	a, err := c.accounts.Create(ctx, repo.Account{
		Username:     c.username,
		PasswordHash: c.hash,
	})
	switch {
	case errors.Is(err, repo.ErrDuplicateUsername):
		// Reset to username step.
		c.username = ""
		c.hash = ""
		c.step = createStepUsername
		return s.WriteRaw([]byte("Username already taken. Choose another.\r\n"))
	case err != nil:
		return s.WriteRaw([]byte("Account creation failed. Try again later.\r\n"))
	}

	s.AuthLevel = telnet.AuthPlayer
	if err := s.WriteRaw([]byte("Account created. Welcome, " + a.Username + ".\r\n")); err != nil {
		return err
	}
	return s.ReplaceMode(c.next)
}

// reservedUsernames are case-insensitively forbidden. "new" routes to
// account-create from Login mode, so allowing it as an account name
// would create an unloggable-into account.
var reservedUsernames = map[string]bool{
	"new": true,
}

// validateUsername enforces ASCII-only [A-Za-z0-9_-] usernames within
// the configured length window, and rejects names that conflict with
// login-mode keywords. Unicode policy (NFKC, lookalikes) is tracked
// under persistence_followups.md.
func validateUsername(name string) error {
	if name == "" {
		return errors.New("Username cannot be empty")
	}
	n := len(name)
	if n < minUsernameLen {
		return errors.New("Username too short (minimum 3 characters)")
	}
	if n > maxUsernameLen {
		return errors.New("Username too long (maximum 24 characters)")
	}
	for _, r := range name {
		if r > unicode.MaxASCII {
			return errors.New("Username may only contain ASCII letters, digits, _ or -")
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return errors.New("Username may only contain letters, digits, _ or -")
		}
	}
	if reservedUsernames[strings.ToLower(name)] {
		return errors.New("Username is reserved. Choose another")
	}
	return nil
}
