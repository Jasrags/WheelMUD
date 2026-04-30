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
	login    *Login
	game     *stubMode
	captured *safeBuf
}

func newLoginFixture(t *testing.T) *loginFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	r := repo.NewMemoryAccountRepo()
	hash, err := auth.Hash("correct-horse")
	if err != nil {
		t.Fatalf("seed hash: %v", err)
	}
	if _, err := r.Create(context.Background(), repo.Account{Username: "Alice", PasswordHash: hash}); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	game := &stubMode{name: "game"}
	login := NewLogin(r, game)

	s := telnet.NewSession(server)
	if err := s.PushMode(login); err != nil {
		t.Fatalf("push login: %v", err)
	}

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	return &loginFixture{t: t, session: s, peer: client, repo: r, login: login, game: game, captured: captured}
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

	if f.session.AuthLevel != telnet.AuthPlayer {
		t.Fatalf("AuthLevel = %d, want AuthPlayer", f.session.AuthLevel)
	}
	if f.session.InPasswordMode {
		t.Fatal("password mode must clear after auth")
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
	if f.session.AuthLevel != telnet.AuthPlayer {
		t.Fatal("expired lockout should not block valid login")
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

func TestLogin_OnExitClearsPasswordMode(t *testing.T) {
	f := newLoginFixture(t)
	f.feed("alice")
	f.feed("correct-horse")
	if f.session.InPasswordMode {
		t.Fatal("InPasswordMode leaked into post-login mode")
	}
}
