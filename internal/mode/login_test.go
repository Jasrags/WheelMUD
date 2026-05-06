package mode

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Jasrags/WheelMUD/internal/auth"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

func TestMain(m *testing.M) {
	prev := auth.SetCost(bcrypt.MinCost)
	defer auth.SetCost(prev)
	m.Run()
}

type loginFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	repo     *repo.MemoryAccountRepo
	chars    *repo.MemoryCharacterRepo
	sessions *session.Registry
	login    *Login
	game     *stubMode
	captured *safeBuf
}

// newLoginFixture seeds an account "Alice" with password "correct-horse"
// and (by default) one character "Hero" — single-character so the
// happy-path tests exercise the auto-promote-to-game path. Tests that
// need different character counts should use newLoginFixtureChars.
func newLoginFixture(t *testing.T) *loginFixture {
	return newLoginFixtureChars(t, []string{"Hero"})
}

func newLoginFixtureChars(t *testing.T, charNames []string) *loginFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	ar := repo.NewMemoryAccountRepo()
	cr := repo.NewMemoryCharacterRepo()
	hash, err := auth.Hash("correct-horse")
	if err != nil {
		t.Fatalf("seed hash: %v", err)
	}
	acc, err := ar.Create(context.Background(), repo.Account{Username: "Alice", PasswordHash: hash})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	// Burn the first-character-on-this-server admin bootstrap on a
	// throwaway so Alice's named characters land at the player tier
	// the happy-path tests assert on. CharacterRepo.Create promotes
	// the very first character atomically; any character after that
	// honors the caller's AuthLevel.
	bootstrapAcc, err := ar.Create(context.Background(), repo.Account{Username: "BootstrapAcct", PasswordHash: hash})
	if err != nil {
		t.Fatalf("seed bootstrap account: %v", err)
	}
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: bootstrapAcc.ID, Name: "BootstrapChar"}); err != nil {
		t.Fatalf("seed bootstrap character: %v", err)
	}
	for _, name := range charNames {
		if _, err := cr.Create(context.Background(), repo.Character{AccountID: acc.ID, Name: name, AuthLevel: repo.AuthLevelPlayer}); err != nil {
			t.Fatalf("seed character %q: %v", name, err)
		}
	}

	game := &stubMode{name: "game"}
	sessions := session.NewRegistry()
	login := NewLogin(ar, cr, sessions, game)

	s := telnet.NewSession(server)
	if err := s.PushMode(login); err != nil {
		t.Fatalf("push login: %v", err)
	}

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	return &loginFixture{t: t, session: s, peer: client, repo: ar, chars: cr, sessions: sessions, login: login, game: game, captured: captured}
}

func (f *loginFixture) feed(line string) {
	f.t.Helper()
	if err := f.login.Handle(context.Background(), f.session, line); err != nil {
		f.t.Fatalf("Handle %q: %v", line, err)
	}
	// Give the drain goroutine a moment to absorb writes from Handle so
	// captured.String() reflects them at assertion time.
	time.Sleep(20 * time.Millisecond)
}

func TestLogin_HappyPath(t *testing.T) {
	f := newLoginFixture(t)
	f.feed("alice")
	if f.login.step != stepPassword {
		t.Fatalf("step = %d, want stepPassword", f.login.step)
	}
	if !f.session.InPasswordMode {
		t.Fatal("expected password mode after username step")
	}

	f.feed("correct-horse")

	// Post-§6: a successful login lands on the AccountMenu, not in
	// game. AuthLevel is stamped from the character row only when
	// the player picks a character (`play`).
	if f.session.InPasswordMode {
		t.Fatal("password mode must clear after auth")
	}
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("CurrentMode = %T, want *AccountMenu", f.session.CurrentMode())
	}
	// `play` with no arg auto-picks the only character (the
	// "one keystroke into the world" affordance the menu preserves).
	menu := f.session.CurrentMode().(*AccountMenu)
	if err := menu.Handle(context.Background(), f.session, "play"); err != nil {
		t.Fatalf("play: %v", err)
	}
	if f.session.AuthLevel != telnet.AuthPlayer {
		t.Fatalf("AuthLevel = %d, want AuthPlayer after play", f.session.AuthLevel)
	}
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %v, want gameMode", f.session.CurrentMode())
	}

	got, _ := f.repo.FindByUsername(context.Background(), "alice")
	if got.LastLoginAt == nil {
		t.Fatal("LastLoginAt not set after success")
	}
	if got.FailedLoginCount != 0 {
		t.Fatalf("failed count = %d, want 0", got.FailedLoginCount)
	}
}

func TestPostAuth_AdminFromCharacter(t *testing.T) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	ar := repo.NewMemoryAccountRepo()
	cr := repo.NewMemoryCharacterRepo()
	hash, _ := auth.Hash("correct-horse")
	acc, _ := ar.Create(context.Background(), repo.Account{Username: "Admin", PasswordHash: hash})
	// Burn the first-character bootstrap on a throwaway so the Hero
	// character below is admin only because we explicitly set its
	// AuthLevel — proving the level travels with the character row.
	burnAcc, _ := ar.Create(context.Background(), repo.Account{Username: "Burn", PasswordHash: hash})
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: burnAcc.ID, Name: "BurnChar"}); err != nil {
		t.Fatalf("seed bootstrap character: %v", err)
	}
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: acc.ID, Name: "Hero", AuthLevel: repo.AuthLevelAdmin}); err != nil {
		t.Fatalf("seed character: %v", err)
	}
	game := &stubMode{name: "game"}
	login := NewLogin(ar, cr, session.NewRegistry(), game)
	s := telnet.NewSession(server)
	if err := s.PushMode(login); err != nil {
		t.Fatalf("push: %v", err)
	}
	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := login.Handle(context.Background(), s, "admin"); err != nil {
		t.Fatalf("username step: %v", err)
	}
	if err := login.Handle(context.Background(), s, "correct-horse"); err != nil {
		t.Fatalf("password step: %v", err)
	}

	// AccountMenu is the post-login landing; AuthLevel is stamped
	// when the menu's `play` verb runs promoteToGame.
	menu, ok := s.CurrentMode().(*AccountMenu)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *AccountMenu", s.CurrentMode())
	}
	if err := menu.Handle(context.Background(), s, "play"); err != nil {
		t.Fatalf("play: %v", err)
	}
	if s.AuthLevel != telnet.AuthAdmin {
		t.Fatalf("AuthLevel = %d, want AuthAdmin (character-level admin should restore via postauth)", s.AuthLevel)
	}
}

func TestLogin_WrongPasswordIncrementsCounter(t *testing.T) {
	f := newLoginFixture(t)
	f.feed("alice")
	f.feed("nope")

	if f.session.AuthLevel == telnet.AuthPlayer {
		t.Fatal("AuthLevel must not be elevated after failure")
	}
	if f.session.CurrentMode() != f.login {
		t.Fatal("must remain in login mode after failure")
	}
	if f.login.step != stepUsername {
		t.Fatal("must reset to username step after failure")
	}

	got, _ := f.repo.FindByUsername(context.Background(), "alice")
	if got.FailedLoginCount != 1 {
		t.Fatalf("failed count = %d, want 1", got.FailedLoginCount)
	}
	if !strings.Contains(f.captured.String(), "Login failed") {
		t.Fatalf("expected 'Login failed' in output: %q", f.captured.String())
	}
}

func TestLogin_UnknownUserBehavesLikeWrongPassword(t *testing.T) {
	f := newLoginFixture(t)
	f.feed("ghost")
	if f.login.step != stepPassword {
		t.Fatalf("step = %d, want stepPassword (no enumeration leak)", f.login.step)
	}
	f.feed("anything")

	if f.session.CurrentMode() != f.login {
		t.Fatal("must remain in login mode")
	}
	if f.login.step != stepUsername {
		t.Fatal("must reset to username step after failure")
	}
	if !strings.Contains(f.captured.String(), "Login failed") {
		t.Fatalf("expected 'Login failed': %q", f.captured.String())
	}
}

func TestLogin_LockoutAfterThreshold(t *testing.T) {
	f := newLoginFixture(t)
	f.login.lockoutThreshold = 3
	f.login.lockoutDuration = 10 * time.Minute

	for i := 0; i < 3; i++ {
		f.feed("alice")
		f.feed("nope")
	}

	got, _ := f.repo.FindByUsername(context.Background(), "alice")
	if got.LockedUntil == nil {
		t.Fatal("LockedUntil not set after threshold")
	}
	if got.FailedLoginCount != 3 {
		t.Fatalf("failed count = %d, want 3", got.FailedLoginCount)
	}
	if !strings.Contains(f.captured.String(), "locked") {
		t.Fatalf("expected lockout message: %q", f.captured.String())
	}
}

func TestLogin_LockedAccountRejectsBeforeVerify(t *testing.T) {
	f := newLoginFixture(t)
	a, _ := f.repo.FindByUsername(context.Background(), "alice")
	future := time.Now().Add(time.Hour)
	_ = f.repo.RecordLoginFailure(context.Background(), a.ID, future)

	f.feed("alice")
	f.feed("correct-horse")
	if f.session.AuthLevel == telnet.AuthPlayer {
		t.Fatal("locked account must not authenticate even with correct password")
	}
	if !strings.Contains(f.captured.String(), "locked") {
		t.Fatalf("expected lockout message: %q", f.captured.String())
	}
}

func TestLogin_LockoutExpires(t *testing.T) {
	f := newLoginFixture(t)
	a, _ := f.repo.FindByUsername(context.Background(), "alice")
	past := time.Now().Add(-time.Minute)
	_ = f.repo.RecordLoginFailure(context.Background(), a.ID, past)

	f.feed("alice")
	f.feed("correct-horse")
	// Login lands on AccountMenu; auth lift happens on `play`.
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("expired lockout should not block valid login: CurrentMode = %T", f.session.CurrentMode())
	}
}

func TestLogin_NewRoutesToCreateMode(t *testing.T) {
	f := newLoginFixture(t)
	f.feed("new")
	current := f.session.CurrentMode()
	if _, ok := current.(*Create); !ok {
		t.Fatalf("CurrentMode = %T, want *Create", current)
	}
}

func TestLogin_LockoutDuringPasswordStepIsHonored(t *testing.T) {
	// Regression: account gets locked between the username step and
	// the password step (parallel session). The handlePassword path
	// must re-check rather than trust the cached state.
	f := newLoginFixture(t)
	f.feed("alice")

	a, _ := f.repo.FindByUsername(context.Background(), "alice")
	future := time.Now().Add(time.Hour)
	_ = f.repo.RecordLoginFailure(context.Background(), a.ID, future)

	f.feed("correct-horse")
	if f.session.AuthLevel == telnet.AuthPlayer {
		t.Fatal("locked account must not authenticate (TOCTOU regression)")
	}
	if !strings.Contains(f.captured.String(), "locked") {
		t.Fatalf("expected lockout message: %q", f.captured.String())
	}
}

func TestLogin_KicksPriorSessionForSameAccount(t *testing.T) {
	// Two sessions for the same account; the second login must close
	// the first.
	f1 := newLoginFixture(t)

	// Build a second session that shares the same registry / repos.
	server2, client2 := net.Pipe()
	t.Cleanup(func() { server2.Close(); client2.Close() })
	s2 := telnet.NewSession(server2)
	captured2 := &safeBuf{}
	drainPeer(t, client2, captured2)
	login2 := NewLogin(f1.repo, f1.chars, f1.sessions, f1.game)
	if err := s2.PushMode(login2); err != nil {
		t.Fatalf("push login2: %v", err)
	}

	// Log in on session 1.
	f1.feed("alice")
	f1.feed("correct-horse")
	if _, ok := f1.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("first login did not authenticate: CurrentMode = %T", f1.session.CurrentMode())
	}
	if f1.sessions.Lookup(f1.session.AccountID) != f1.session {
		t.Fatal("registry did not bind first session")
	}

	// Log in on session 2 — should kick session 1.
	if err := login2.Handle(context.Background(), s2, "alice"); err != nil {
		t.Fatalf("session2 username: %v", err)
	}
	if err := login2.Handle(context.Background(), s2, "correct-horse"); err != nil {
		t.Fatalf("session2 password: %v", err)
	}

	if f1.sessions.Lookup(s2.AccountID) != s2 {
		t.Fatal("registry did not switch to second session")
	}

	// Drain a beat for the kick notice + Conn.Close to fire on session 1.
	time.Sleep(50 * time.Millisecond)
	if !strings.Contains(f1.captured.String(), "logged in elsewhere") {
		t.Fatalf("first session did not receive kick notice: %q", f1.captured.String())
	}
}

func TestLogin_FailedLoginDoesNotDisturbExistingSession(t *testing.T) {
	f1 := newLoginFixture(t)
	f1.feed("alice")
	f1.feed("correct-horse")
	if _, ok := f1.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("first login did not authenticate: CurrentMode = %T", f1.session.CurrentMode())
	}
	priorBaseline := f1.captured.String()

	// Second session attempts and fails.
	server2, client2 := net.Pipe()
	t.Cleanup(func() { server2.Close(); client2.Close() })
	s2 := telnet.NewSession(server2)
	captured2 := &safeBuf{}
	drainPeer(t, client2, captured2)
	login2 := NewLogin(f1.repo, f1.chars, f1.sessions, f1.game)
	if err := s2.PushMode(login2); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := login2.Handle(context.Background(), s2, "alice"); err != nil {
		t.Fatalf("user: %v", err)
	}
	if err := login2.Handle(context.Background(), s2, "wrong"); err != nil {
		t.Fatalf("pass: %v", err)
	}

	// Registry must still point at the original session; original
	// must NOT have received a kick notice.
	if f1.sessions.Lookup(f1.session.AccountID) != f1.session {
		t.Fatal("failed login disturbed the existing binding")
	}
	time.Sleep(50 * time.Millisecond)
	delta := strings.TrimPrefix(f1.captured.String(), priorBaseline)
	if strings.Contains(delta, "logged in elsewhere") {
		t.Fatalf("failed login leaked kick notice: %q", delta)
	}
}

func TestLogin_OnExitClearsPasswordMode(t *testing.T) {
	f := newLoginFixture(t)
	f.feed("alice")
	f.feed("correct-horse")
	if f.session.InPasswordMode {
		t.Fatal("InPasswordMode leaked into post-login mode")
	}
}
