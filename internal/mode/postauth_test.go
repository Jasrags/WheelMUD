package mode

import (
	"context"
	"net"
	"testing"

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

	if err := promoteToGame(context.Background(), s, c, cr, game); err != nil {
		t.Fatalf("promoteToGame: %v", err)
	}
	if s.AuthLevel != telnet.AuthPlayer {
		t.Fatalf("session AuthLevel = %d, want AuthPlayer (floor must clamp)", s.AuthLevel)
	}
}
