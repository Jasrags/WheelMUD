package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// adminPair is the admin/player analogue of commPair: alice is admin,
// bob is a player. Both share a registry, fresh world, and live in
// room 1 by default. Returns everything tests poke (registry, both
// sessions, both bufConns, the seeded repos, the character repo).
func adminPair(t *testing.T) (
	sessions *session.Registry,
	admin, player *telnet.Session,
	aOut, pOut *bufConn,
	rooms *repo.MemoryRoomRepo,
	exits *repo.MemoryExitRepo,
	items *repo.MemoryItemRepo,
	mobs *repo.MemoryMobInstanceRepo,
	chars *repo.MemoryCharacterRepo,
) {
	t.Helper()
	rooms, exits, items, mobs = seedWorld(t)
	chars = repo.NewMemoryCharacterRepo()
	sessions = session.NewRegistry()

	a, aConn := bufSession(t)
	a.AccountID = 100
	a.AuthLevel = telnet.AuthAdmin
	a.CharacterID = 1
	a.CharacterName = "Admin"
	a.CurrentRoomID = 1
	sessions.Bind(a.AccountID, a)

	p, pConn := bufSession(t)
	p.AccountID = 200
	p.AuthLevel = telnet.AuthPlayer
	p.CharacterID = 2
	p.CharacterName = "Player"
	p.CurrentRoomID = 2
	sessions.Bind(p.AccountID, p)

	return sessions, a, p, aConn, pConn, rooms, exits, items, mobs, chars
}

func TestGoto_ToOnlinePlayer(t *testing.T) {
	sessions, admin, player, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	g := NewGoto(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	if admin.CurrentRoomID == player.CurrentRoomID {
		t.Fatalf("test setup: admin and player should start apart")
	}
	runCmd(t, g, admin, "Player")

	if admin.CurrentRoomID != player.CurrentRoomID {
		t.Fatalf("goto did not move admin: admin=%d player=%d", admin.CurrentRoomID, player.CurrentRoomID)
	}
}

func TestGoto_PlayerNameWinsOverRoom(t *testing.T) {
	// If a player's name happens to match a room id/external id, the
	// player lookup wins (per command's documented contract).
	sessions, admin, player, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	g := NewGoto(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	// Move player to a freshly-inserted room, then create a room whose
	// external id matches the player's name. Goto should still land in
	// the player's room, not the trap-named one.
	rooms.Insert(repo.Room{ID: 7, Name: "Player's Hideout"})
	rooms.Insert(repo.Room{ID: 8, Name: "Decoy", ExternalID: "player"})
	player.CurrentRoomID = 7

	runCmd(t, g, admin, "Player")
	if admin.CurrentRoomID != 7 {
		t.Fatalf("goto landed in %d, expected player's room 7", admin.CurrentRoomID)
	}
}

func TestGoto_RoomFallback(t *testing.T) {
	sessions, admin, _, aOut, _, rooms, exits, items, mobs, chars := adminPair(t)
	g := NewGoto(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	runCmd(t, g, admin, "2")
	if admin.CurrentRoomID != 2 {
		t.Fatalf("goto by id did not move admin: %d", admin.CurrentRoomID)
	}
	if aOut.String() == "" {
		t.Fatalf("expected room render output")
	}
}

func TestGoto_SelfTargetIsFriendly(t *testing.T) {
	sessions, admin, _, aOut, _, rooms, exits, items, mobs, chars := adminPair(t)
	g := NewGoto(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	runCmd(t, g, admin, "Admin")
	if !strings.Contains(aOut.String(), "already there") {
		t.Fatalf("missing self-target message; got %q", aOut.String())
	}
}

func TestGoto_PlayerNotInWorldYet(t *testing.T) {
	sessions, admin, player, aOut, _, rooms, exits, items, mobs, chars := adminPair(t)
	g := NewGoto(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	// Player is bound to the registry but has no room (e.g. still in
	// character-select). Goto should refuse rather than teleport admin
	// to "room 0".
	player.CurrentRoomID = 0

	startRoom := admin.CurrentRoomID
	runCmd(t, g, admin, "Player")
	if admin.CurrentRoomID != startRoom {
		t.Fatalf("admin moved despite peer.CurrentRoomID == 0: %d -> %d", startRoom, admin.CurrentRoomID)
	}
	if !strings.Contains(aOut.String(), "not in the world yet") {
		t.Fatalf("missing not-in-world message; got %q", aOut.String())
	}
}

func TestGoto_NoTeleportRoomBlocks(t *testing.T) {
	sessions, admin, player, aOut, _, rooms, exits, items, mobs, chars := adminPair(t)
	rooms.Insert(repo.Room{ID: 99, Name: "Sealed Vault", Flags: repo.RoomFlags{NoTeleport: true}})
	player.CurrentRoomID = 99
	g := NewGoto(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	runCmd(t, g, admin, "Player")
	if admin.CurrentRoomID == 99 {
		t.Fatalf("goto into NoTeleport room succeeded")
	}
	if !strings.Contains(aOut.String(), "Pattern resists") {
		t.Fatalf("missing resist message; got %q", aOut.String())
	}
}

func TestTransfer_OneArgPullsToCallerRoom(t *testing.T) {
	sessions, admin, player, aOut, pOut, rooms, exits, items, mobs, chars := adminPair(t)
	tr := NewTransfer(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	runCmd(t, tr, admin, "Player")

	if player.CurrentRoomID != admin.CurrentRoomID {
		t.Fatalf("player not pulled to admin's room: admin=%d player=%d", admin.CurrentRoomID, player.CurrentRoomID)
	}
	if !strings.Contains(aOut.String(), "Transferred Player") {
		t.Fatalf("missing caller ack; got %q", aOut.String())
	}
	if !strings.Contains(pOut.String(), "world ripples") {
		t.Fatalf("missing target ripple notice; got %q", pOut.String())
	}
}

func TestTransfer_TwoArgSendsToRoom(t *testing.T) {
	sessions, admin, player, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	tr := NewTransfer(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	player.CurrentRoomID = 1
	runCmd(t, tr, admin, "Player 2")
	if player.CurrentRoomID != 2 {
		t.Fatalf("transfer Player 2 left them in %d", player.CurrentRoomID)
	}
}

func TestTransfer_UnknownPlayerFriendly(t *testing.T) {
	sessions, admin, _, aOut, _, rooms, exits, items, mobs, chars := adminPair(t)
	tr := NewTransfer(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	runCmd(t, tr, admin, "Ghost")
	if !strings.Contains(aOut.String(), "No such player online") {
		t.Fatalf("missing offline error; got %q", aOut.String())
	}
}

func TestSummon_PullsToCallerRoom(t *testing.T) {
	sessions, admin, player, aOut, pOut, rooms, exits, items, mobs, chars := adminPair(t)
	s := NewSummon(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	runCmd(t, s, admin, "Player")

	if player.CurrentRoomID != admin.CurrentRoomID {
		t.Fatalf("summon did not move player: admin=%d player=%d", admin.CurrentRoomID, player.CurrentRoomID)
	}
	if !strings.Contains(aOut.String(), "Transferred Player") {
		t.Fatalf("missing caller ack; got %q", aOut.String())
	}
	if !strings.Contains(pOut.String(), "world ripples") {
		t.Fatalf("missing target notice; got %q", pOut.String())
	}
}

func TestSummon_NoTeleportRoomBlocks(t *testing.T) {
	sessions, admin, _, aOut, _, rooms, exits, items, mobs, chars := adminPair(t)
	rooms.Insert(repo.Room{ID: 50, Name: "Warded Sanctum", Flags: repo.RoomFlags{NoTeleport: true}})
	admin.CurrentRoomID = 50
	s := NewSummon(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)

	runCmd(t, s, admin, "Player")
	if !strings.Contains(aOut.String(), "Pattern resists") {
		t.Fatalf("missing resist message; got %q", aOut.String())
	}
}

func TestSummon_PersistsRoomViaCharacterRepo(t *testing.T) {
	sessions, admin, player, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	// Seed a character row matching the player's CharacterID so
	// RecordRoom has something to update.
	c, _ := chars.Create(context.Background(), repo.Character{AccountID: 200, Name: "Player"})
	player.CharacterID = c.ID

	s := NewSummon(rooms, exits, items, mobs, chars, sessions, noonClock(t), nil)
	runCmd(t, s, admin, "Player")

	loaded, err := chars.FindByName(context.Background(), c.Name)
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if loaded.CurrentRoomID != admin.CurrentRoomID {
		t.Fatalf("repo not updated: row=%d admin=%d", loaded.CurrentRoomID, admin.CurrentRoomID)
	}
}
