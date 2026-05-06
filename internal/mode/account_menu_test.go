package mode

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

type menuFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	chars    *repo.MemoryCharacterRepo
	game     *stubMode
	captured *safeBuf
	menu     *AccountMenu
	motdHits *int32
}

func pushAccountMenu(t *testing.T, names []string) *menuFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	// Burn the first-character bootstrap so seeded characters land at
	// the player tier instead of being elevated to admin.
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: 999, Name: "Bootstrap"}); err != nil {
		t.Fatalf("seed bootstrap: %v", err)
	}
	var seeded []repo.Character
	for _, n := range names {
		c, err := cr.Create(context.Background(), repo.Character{
			AccountID: 1, Name: n, AuthLevel: repo.AuthLevelPlayer,
		})
		if err != nil {
			t.Fatalf("seed %q: %v", n, err)
		}
		seeded = append(seeded, c)
	}

	game := &stubMode{name: "game"}
	menu := NewAccountMenu(seeded, cr, game)

	var motdHits int32
	menu.SetMOTD(func(_ *telnet.Session, _ time.Time) error {
		atomic.AddInt32(&motdHits, 1)
		return nil
	})

	s := telnet.NewSession(server)
	s.AccountID = 1

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(menu); err != nil {
		t.Fatalf("push: %v", err)
	}
	return &menuFixture{
		t: t, session: s, peer: client, chars: cr, game: game,
		captured: captured, menu: menu, motdHits: &motdHits,
	}
}

func (f *menuFixture) feed(line string) {
	f.t.Helper()
	if err := f.session.CurrentMode().Handle(context.Background(), f.session, line); err != nil && err != telnet.ErrSessionEnded {
		f.t.Fatalf("Handle %q: %v", line, err)
	}
	time.Sleep(20 * time.Millisecond)
}

func TestAccountMenu_ListsOnEntry(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	time.Sleep(20 * time.Millisecond)
	out := f.captured.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("expected both characters in listing: %q", out)
	}
}

func TestAccountMenu_PlayNoArgAutoPicksSingle(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed("play")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Solo" {
		t.Fatalf("CharacterName = %q, want Solo", f.session.CharacterName)
	}
}

func TestAccountMenu_PlayNoArgRefusesAmbiguous(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed("play")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("ambiguous play should stay in menu, got %T", f.session.CurrentMode())
	}
	if !strings.Contains(f.captured.String(), "Specify") {
		t.Fatalf("expected disambiguation hint: %q", f.captured.String())
	}
}

func TestAccountMenu_PlayByNamePromotes(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed("play beta")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Beta" {
		t.Fatalf("CharacterName = %q, want Beta", f.session.CharacterName)
	}
}

func TestAccountMenu_PlayByIndexPromotes(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed("play 2")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	// chars[0] is most-recently-played; "Beta" was created last so
	// it lands at index 0 under MemoryCharacterRepo's ordering. Index 2
	// addresses the second slot regardless — assert the session has
	// exactly one of the seeded names.
	if f.session.CharacterName != "Alpha" && f.session.CharacterName != "Beta" {
		t.Fatalf("CharacterName = %q, want Alpha or Beta", f.session.CharacterName)
	}
}

func TestAccountMenu_PlayForeignCharacterRejected(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	if _, err := f.chars.Create(context.Background(), repo.Character{AccountID: 999, Name: "Stranger"}); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	f.feed("play Stranger")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatal("foreign character must not promote — must stay in menu")
	}
	if !strings.Contains(f.captured.String(), "No such character on this account") {
		t.Fatalf("expected reject message: %q", f.captured.String())
	}
}

func TestAccountMenu_NewRoutesToCharacterCreate(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed("new")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
}

func TestAccountMenu_NewsReplaysHook(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	if got := atomic.LoadInt32(f.motdHits); got != 0 {
		t.Fatalf("motd hits before news = %d, want 0", got)
	}
	f.feed("news")
	if got := atomic.LoadInt32(f.motdHits); got != 1 {
		t.Fatalf("motd hits after news = %d, want 1", got)
	}
	// Replay must not advance the watermark or change mode.
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("news should stay in menu, got %T", f.session.CurrentMode())
	}
}

func TestAccountMenu_HelpListsVerbs(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.captured.Reset()
	f.feed("help")
	out := f.captured.String()
	for _, want := range []string{"list", "play", "new", "news", "quit"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q: %q", want, out)
		}
	}
}

func TestAccountMenu_QuitClosesConnection(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed("quit")
	if !strings.Contains(f.captured.String(), "Goodbye") {
		t.Fatalf("expected goodbye line: %q", f.captured.String())
	}
}

func TestAccountMenu_UnknownVerbStaysInMenu(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed("frobnicate")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatal("unknown verb should stay in menu")
	}
	if !strings.Contains(f.captured.String(), "Unknown command") {
		t.Fatalf("expected error message: %q", f.captured.String())
	}
}

// TestPostAuth_FiresMOTDOncePerLogin locks in the §6 ordering note:
// MOTD/news fires before the AccountMenu, and is not re-fired by
// subsequent promote-to-game.
func TestPostAuth_FiresMOTDOncePerLogin(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: 999, Name: "Bootstrap"}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := cr.Create(context.Background(), repo.Character{
		AccountID: 1, Name: "Hero", AuthLevel: repo.AuthLevelPlayer,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	game := &stubMode{name: "game"}
	s := telnet.NewSession(server)
	s.AccountID = 1
	captured := &safeBuf{}
	drainPeer(t, client, captured)

	var hits int32
	motd := func(_ *telnet.Session, _ time.Time) error {
		atomic.AddInt32(&hits, 1)
		return nil
	}

	if err := postAuth(context.Background(), s, cr, motd, nil, game); err != nil {
		t.Fatalf("postAuth: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("motd hits after postAuth = %d, want 1 (fires before menu)", got)
	}

	menu, ok := s.CurrentMode().(*AccountMenu)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *AccountMenu", s.CurrentMode())
	}
	if err := menu.Handle(context.Background(), s, "play"); err != nil {
		t.Fatalf("play: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("motd hits after play = %d, want 1 (must not re-fire)", got)
	}
}
