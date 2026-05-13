package mode

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// TestPostAuth_FloorClampsSubPlayer locks in the deliberate floor
// policy in promoteToGame: a character stored at AuthGuest (0) is
// clamped up to AuthPlayer with an slog.Error rather than refused
// promotion. See promoteToGame for the policy rationale.
func TestPostAuth_FloorClampsSubPlayer(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	// Burn the first-character bootstrap so the next Create honors
	// the caller-supplied AuthLevel verbatim.
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: 999, Name: "Bootstrap"}); err != nil {
		t.Fatalf("seed bootstrap: %v", err)
	}
	// Stored at AuthGuest (0) — the case the floor exists to defend.
	c, err := cr.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Guest",
		AuthLevel: repo.AuthLevelGuest,
	})
	if err != nil {
		t.Fatalf("seed guest: %v", err)
	}
	if c.AuthLevel != repo.AuthLevelGuest {
		t.Fatalf("seed AuthLevel = %d, want AuthLevelGuest", c.AuthLevel)
	}

	game := &stubMode{name: "game"}
	s := telnet.NewSession(server)
	captured := &safeBuf{}
	drainPeer(t, client, captured)
	if err := s.PushMode(game); err != nil {
		t.Fatalf("push game: %v", err)
	}
	// Pre-condition: session at AuthGuest before promote.
	s.AuthLevel = telnet.AuthGuest

	if err := promoteToGame(context.Background(), s, c, cr, nil, game); err != nil {
		t.Fatalf("promoteToGame: %v", err)
	}
	if s.AuthLevel != telnet.AuthPlayer {
		t.Fatalf("session AuthLevel = %d, want AuthPlayer (floor must clamp)", s.AuthLevel)
	}
}

// TestPromote_LoadsBuilderZones verifies the Phase G #33 slice 2
// promote-time cache load: a character with two builder_zones rows
// arrives in-game with IsBuilderFor returning true for both zones
// and false for an unseeded zone. A nil builders dep (back-compat
// path / tests without OLC) leaves the session's cache empty
// without erroring out the promote.
func TestPromote_LoadsBuilderZones(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	// Burn the bootstrap so the next Create stays at AuthPlayer.
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: 999, Name: "Bootstrap"}); err != nil {
		t.Fatalf("seed bootstrap: %v", err)
	}
	c, err := cr.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Builder",
		AuthLevel: repo.AuthLevelPlayer,
	})
	if err != nil {
		t.Fatalf("seed builder: %v", err)
	}

	br := repo.NewMemoryBuilderZoneRepo()
	if err := br.Grant(context.Background(), c.ID, 42, 0, time.Time{}); err != nil {
		t.Fatalf("grant 42: %v", err)
	}
	if err := br.Grant(context.Background(), c.ID, 99, 0, time.Time{}); err != nil {
		t.Fatalf("grant 99: %v", err)
	}

	game := &stubMode{name: "game"}
	s := telnet.NewSession(server)
	captured := &safeBuf{}
	drainPeer(t, client, captured)
	if err := s.PushMode(game); err != nil {
		t.Fatalf("push game: %v", err)
	}

	if err := promoteToGame(context.Background(), s, c, cr, br, game); err != nil {
		t.Fatalf("promoteToGame: %v", err)
	}
	if !s.IsBuilderFor(42) {
		t.Fatal("session: IsBuilderFor(42) = false after promote")
	}
	if !s.IsBuilderFor(99) {
		t.Fatal("session: IsBuilderFor(99) = false after promote")
	}
	if s.IsBuilderFor(7) {
		t.Fatal("session: IsBuilderFor(7) = true; not granted")
	}
}

// TestPromote_NoBuildersIsOK verifies the nil-builders back-compat
// path: promote succeeds and leaves the session's cache empty when
// no BuilderZoneRepo is wired (e.g. tests, pre-§G boot).
func TestPromote_NoBuildersIsOK(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: 999, Name: "Bootstrap"}); err != nil {
		t.Fatalf("seed bootstrap: %v", err)
	}
	c, err := cr.Create(context.Background(), repo.Character{
		AccountID: 1,
		Name:      "Plain",
		AuthLevel: repo.AuthLevelPlayer,
	})
	if err != nil {
		t.Fatalf("seed plain: %v", err)
	}

	game := &stubMode{name: "game"}
	s := telnet.NewSession(server)
	captured := &safeBuf{}
	drainPeer(t, client, captured)
	if err := s.PushMode(game); err != nil {
		t.Fatalf("push game: %v", err)
	}

	if err := promoteToGame(context.Background(), s, c, cr, nil, game); err != nil {
		t.Fatalf("promoteToGame: %v", err)
	}
	if s.IsBuilderFor(42) {
		t.Fatal("session got grants from nowhere; nil-builders path must skip the load")
	}
}
