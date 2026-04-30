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

type charFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	chars    *repo.MemoryCharacterRepo
	game     *stubMode
	captured *safeBuf
}

// pushCharacterSelect builds a fixture in CharacterSelect mode for
// account 1 with the given seeded character names.
func pushCharacterSelect(t *testing.T, names []string) *charFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	var seeded []repo.Character
	for _, n := range names {
		c, err := cr.Create(context.Background(), repo.Character{AccountID: 1, Name: n})
		if err != nil {
			t.Fatalf("seed %q: %v", n, err)
		}
		seeded = append(seeded, c)
	}
	game := &stubMode{name: "game"}
	mode := NewCharacterSelect(seeded, cr, game)

	s := telnet.NewSession(server)
	s.AccountID = 1

	// Start drain BEFORE PushMode — OnEnter writes the character list
	// and would otherwise block on the unbuffered net.Pipe.
	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}

	return &charFixture{t: t, session: s, peer: client, chars: cr, game: game, captured: captured}
}

func (f *charFixture) feed(line string) {
	f.t.Helper()
	if err := f.session.CurrentMode().Handle(context.Background(), f.session, line); err != nil && err != telnet.ErrSessionEnded {
		f.t.Fatalf("Handle %q: %v", line, err)
	}
	time.Sleep(20 * time.Millisecond)
}

func TestCharacterSelect_ListsOnEntry(t *testing.T) {
	f := pushCharacterSelect(t, []string{"Alpha", "Beta"})
	time.Sleep(20 * time.Millisecond)
	out := f.captured.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("expected both characters in listing: %q", out)
	}
}

func TestCharacterSelect_PickPromotes(t *testing.T) {
	f := pushCharacterSelect(t, []string{"Alpha", "Beta"})
	f.feed("alpha") // case-insensitive
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %v, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Alpha" {
		t.Fatalf("CharacterName = %q, want Alpha", f.session.CharacterName)
	}
}

func TestCharacterSelect_RejectsUnknown(t *testing.T) {
	f := pushCharacterSelect(t, []string{"Alpha", "Beta"})
	f.feed("ghost")
	if f.session.CurrentMode() != f.session.CurrentMode().(*CharacterSelect) {
		// Stay in select on unknown name.
	}
	if _, ok := f.session.CurrentMode().(*CharacterSelect); !ok {
		t.Fatal("unknown name should stay in CharacterSelect")
	}
	if !strings.Contains(f.captured.String(), "No such character") {
		t.Fatalf("expected reject message: %q", f.captured.String())
	}
}

func TestCharacterSelect_OtherAccountsCharacterRejected(t *testing.T) {
	// Seed a character that exists in the repo but belongs to a
	// different account — must not be playable from this session.
	f := pushCharacterSelect(t, []string{"Alpha"})
	if _, err := f.chars.Create(context.Background(), repo.Character{AccountID: 999, Name: "Stranger"}); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	f.feed("Stranger")
	if _, ok := f.session.CurrentMode().(*CharacterSelect); !ok {
		t.Fatal("foreign character must not promote — must stay in select")
	}
	if !strings.Contains(f.captured.String(), "No such character on this account") {
		t.Fatalf("expected reject message: %q", f.captured.String())
	}
}

func TestCharacterSelect_CreateRoutesToCharacterCreate(t *testing.T) {
	f := pushCharacterSelect(t, []string{"Alpha", "Beta"})
	f.feed("create")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
}

// CharacterCreate tests.

type charCreateFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	chars    *repo.MemoryCharacterRepo
	game     *stubMode
	captured *safeBuf
}

func pushCharacterCreate(t *testing.T) *charCreateFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	game := &stubMode{name: "game"}
	mode := NewCharacterCreate(cr, game)

	s := telnet.NewSession(server)
	s.AccountID = 1

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}

	return &charCreateFixture{t: t, session: s, peer: client, chars: cr, game: game, captured: captured}
}

func (f *charCreateFixture) feed(line string) {
	f.t.Helper()
	if err := f.session.CurrentMode().Handle(context.Background(), f.session, line); err != nil {
		f.t.Fatalf("Handle %q: %v", line, err)
	}
	time.Sleep(20 * time.Millisecond)
}

func TestCharacterCreate_HappyPath(t *testing.T) {
	f := pushCharacterCreate(t)
	f.feed("Hero")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %v, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Hero" {
		t.Fatalf("CharacterName = %q, want Hero", f.session.CharacterName)
	}
	got, err := f.chars.FindByName(context.Background(), "Hero")
	if err != nil {
		t.Fatalf("character not persisted: %v", err)
	}
	if got.AccountID != 1 {
		t.Fatalf("AccountID = %d, want 1", got.AccountID)
	}
}

func TestCharacterCreate_RejectsReservedNames(t *testing.T) {
	for _, name := range []string{"create", "quit", "QUIT"} {
		t.Run(name, func(t *testing.T) {
			f := pushCharacterCreate(t)
			f.feed(name)
			if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
				t.Fatal("reserved name must keep us in CharacterCreate")
			}
			if !strings.Contains(f.captured.String(), "reserved") {
				t.Fatalf("expected reserved message: %q", f.captured.String())
			}
		})
	}
}

func TestCharacterCreate_RejectsBadCharset(t *testing.T) {
	f := pushCharacterCreate(t)
	f.feed("bad name")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatal("invalid name must keep us in CharacterCreate")
	}
}

func TestCharacterCreate_DuplicateNameStays(t *testing.T) {
	f := pushCharacterCreate(t)
	if _, err := f.chars.Create(context.Background(), repo.Character{AccountID: 999, Name: "Hero"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f.feed("Hero")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatal("duplicate name must keep us in CharacterCreate")
	}
	if !strings.Contains(f.captured.String(), "already taken") {
		t.Fatalf("expected duplicate message: %q", f.captured.String())
	}
}

// End-to-end through Login: the auto-promote-to-game path covered by
// the existing TestLogin_HappyPath uses a single-character account. Add
// the multi-character case here.

func TestLogin_MultiCharacterRoutesToCharacterSelect(t *testing.T) {
	f := newLoginFixtureChars(t, []string{"Alpha", "Beta"})
	f.feed("alice")
	f.feed("correct-horse")
	if _, ok := f.session.CurrentMode().(*CharacterSelect); !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterSelect (account has 2 chars)", f.session.CurrentMode())
	}
}

func TestLogin_ZeroCharacterRoutesToCharacterCreate(t *testing.T) {
	f := newLoginFixtureChars(t, nil)
	f.feed("alice")
	f.feed("correct-horse")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate (account has 0 chars)", f.session.CurrentMode())
	}
}
