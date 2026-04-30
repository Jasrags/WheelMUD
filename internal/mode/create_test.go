package mode

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/auth"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

type createFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	repo     *repo.MemoryAccountRepo
	create   *Create
	game     *stubMode
	captured *safeBuf
}

func newCreateFixture(t *testing.T, seed ...repo.Account) *createFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	r := repo.NewMemoryAccountRepo()
	for _, a := range seed {
		if _, err := r.Create(context.Background(), a); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	game := &stubMode{name: "game"}
	c := NewCreate(r, game)

	s := telnet.NewSession(server)
	if err := s.PushMode(c); err != nil {
		t.Fatalf("push create: %v", err)
	}

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	return &createFixture{t: t, session: s, peer: client, repo: r, create: c, game: game, captured: captured}
}

func (f *createFixture) feed(line string) {
	f.t.Helper()
	if err := f.create.Handle(context.Background(), f.session, line); err != nil {
		f.t.Fatalf("Handle %q: %v", line, err)
	}
	time.Sleep(20 * time.Millisecond)
}

func TestCreate_HappyPath(t *testing.T) {
	f := newCreateFixture(t)
	f.feed("Bob")
	if f.create.step != createStepPassword {
		t.Fatalf("step = %d, want password", f.create.step)
	}
	if !f.session.InPasswordMode {
		t.Fatal("password mode should be on after username")
	}
	f.feed("hunter2-password")
	if f.create.step != createStepConfirm {
		t.Fatalf("step = %d, want confirm", f.create.step)
	}
	f.feed("hunter2-password")

	if f.session.AuthLevel != telnet.AuthPlayer {
		t.Fatalf("AuthLevel = %d, want AuthPlayer", f.session.AuthLevel)
	}
	if f.session.CurrentMode() != f.game {
		t.Fatal("expected ReplaceMode to game")
	}
	if f.session.InPasswordMode {
		t.Fatal("password mode must clear after success")
	}
	got, err := f.repo.FindByUsername(context.Background(), "Bob")
	if err != nil {
		t.Fatalf("account not persisted: %v", err)
	}
	if !auth.Verify(got.PasswordHash, "hunter2-password") {
		t.Fatal("stored hash does not verify against original password")
	}
}

func TestCreate_PasswordMismatchResetsToPasswordStep(t *testing.T) {
	f := newCreateFixture(t)
	f.feed("Bob")
	f.feed("hunter2-password")
	f.feed("different-password")

	if f.create.step != createStepPassword {
		t.Fatalf("step = %d, want password (mismatch should re-ask)", f.create.step)
	}
	if f.create.username != "Bob" {
		t.Fatalf("username lost on mismatch: %q", f.create.username)
	}
	if !f.session.InPasswordMode {
		t.Fatal("password mode should remain on after mismatch")
	}
	if !strings.Contains(f.captured.String(), "did not match") {
		t.Fatalf("expected mismatch message: %q", f.captured.String())
	}
}

func TestCreate_DuplicateUsernameResets(t *testing.T) {
	hash, _ := auth.Hash("seedseed")
	f := newCreateFixture(t, repo.Account{Username: "Taken", PasswordHash: hash})
	f.feed("taken")
	f.feed("hunter2-password")
	f.feed("hunter2-password")

	if f.create.step != createStepUsername {
		t.Fatalf("step = %d, want username (duplicate should restart)", f.create.step)
	}
	if !strings.Contains(f.captured.String(), "already taken") {
		t.Fatalf("expected duplicate message: %q", f.captured.String())
	}
}

func TestCreate_RejectsShortUsername(t *testing.T) {
	f := newCreateFixture(t)
	f.feed("ab")
	if f.create.step != createStepUsername {
		t.Fatal("short username should keep step at username")
	}
	if !strings.Contains(f.captured.String(), "too short") {
		t.Fatalf("expected too short: %q", f.captured.String())
	}
}

func TestCreate_RejectsBadCharacters(t *testing.T) {
	f := newCreateFixture(t)
	f.feed("bad name")
	if f.create.step != createStepUsername {
		t.Fatal("bad chars should keep step at username")
	}
	if !strings.Contains(f.captured.String(), "letters, digits") {
		t.Fatalf("expected charset message: %q", f.captured.String())
	}
}

func TestCreate_RejectsShortPassword(t *testing.T) {
	f := newCreateFixture(t)
	f.feed("Bob")
	f.feed("short")
	if f.create.step != createStepPassword {
		t.Fatal("short password should re-ask password")
	}
	if !strings.Contains(f.captured.String(), "too short") {
		t.Fatalf("expected too short: %q", f.captured.String())
	}
}
