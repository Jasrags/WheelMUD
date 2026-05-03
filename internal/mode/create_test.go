package mode

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/auth"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

type createFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	repo     *repo.MemoryAccountRepo
	chars    *repo.MemoryCharacterRepo
	create   *Create
	game     *stubMode
	captured *safeBuf
}

func newCreateFixture(t *testing.T, seed ...repo.Account) *createFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	ar := repo.NewMemoryAccountRepo()
	cr := repo.NewMemoryCharacterRepo()
	for _, a := range seed {
		if _, err := ar.Create(context.Background(), a); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	game := &stubMode{name: "game"}
	sessions := session.NewRegistry()
	c := NewCreate(ar, cr, sessions, game)

	s := telnet.NewSession(server)
	if err := s.PushMode(c); err != nil {
		t.Fatalf("push create: %v", err)
	}

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	return &createFixture{t: t, session: s, peer: client, repo: ar, chars: cr, create: c, game: game, captured: captured}
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

	// Account create no longer earns a privilege — session is still
	// AuthGuest until CharacterCreate inserts a character and
	// postauth.promoteToGame stamps the level from there.
	if f.session.AuthLevel != telnet.AuthGuest {
		t.Fatalf("AuthLevel = %d, want AuthGuest pre-character", f.session.AuthLevel)
	}
	if f.session.AccountID == 0 {
		t.Fatal("AccountID not stamped after create")
	}
	// Fresh account has zero characters → postAuth pushes CharacterCreate.
	current := f.session.CurrentMode()
	cc, ok := current.(*CharacterCreate)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", current)
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

	// Drive CharacterCreate to completion to verify the full flow lands
	// in game with the character set.
	if err := cc.Handle(context.Background(), f.session, "Hero"); err != nil {
		t.Fatalf("CharacterCreate handle: %v", err)
	}
	if f.session.CurrentMode() != f.game {
		t.Fatal("expected ReplaceMode to game after character create")
	}
	if f.session.CharacterName != "Hero" {
		t.Fatalf("CharacterName = %q, want Hero", f.session.CharacterName)
	}
	// First character on this in-memory repo → bootstrap promotes Hero
	// to AuthAdmin, which postauth stamps onto the session.
	if f.session.AuthLevel != telnet.AuthAdmin {
		t.Fatalf("AuthLevel after first character = %d, want AuthAdmin", f.session.AuthLevel)
	}
	hero, err := f.chars.FindByName(context.Background(), "Hero")
	if err != nil {
		t.Fatalf("find Hero: %v", err)
	}
	if hero.AuthLevel != repo.AuthLevelAdmin {
		t.Fatalf("Hero persisted AuthLevel = %d, want AuthLevelAdmin", hero.AuthLevel)
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

func TestCreate_RejectsReservedUsername(t *testing.T) {
	for _, name := range []string{"new", "NEW", "New"} {
		t.Run(name, func(t *testing.T) {
			f := newCreateFixture(t)
			f.feed(name)
			if f.create.step != createStepUsername {
				t.Fatalf("step = %d, reserved username should be rejected", f.create.step)
			}
			if !strings.Contains(f.captured.String(), "reserved") {
				t.Fatalf("expected reserved-name message: %q", f.captured.String())
			}
		})
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
