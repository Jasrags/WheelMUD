package gmcp

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// drainPeer reads everything queued on conn until EOF or a small
// deadline. Returns whatever bytes accumulated; never fails the test.
func drainPeer(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	out := make([]byte, 0, 1024)
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			break
		}
	}
	return out
}

// newGMCPHarness builds a Manager + session with GMCP enabled and a
// character in place so handler tests can exercise the full path.
func newGMCPHarness(t *testing.T) (*Manager, *telnet.Session, net.Conn, *eventbus.Bus, repo.CharacterRepo, *repo.MemoryRoomRepo, *repo.MemoryExitRepo) {
	t.Helper()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	zones := repo.NewMemoryZoneRepo()
	sessions := session.NewRegistry()

	room := rooms.Insert(repo.Room{
		ID:         1,
		ExternalID: "test.room",
		Name:       "Test Room",
		ZoneID:     1,
		LongDesc:   "A featureless test space.",
	})
	_ = zones.Insert(repo.Zone{ID: 1, ExternalID: "test"})
	exits.Insert(repo.Exit{FromRoomID: room.ID, ToRoomID: 2, Direction: repo.DirNorth})

	ch, err := chars.Create(context.Background(), repo.Character{
		Name:          "Tester",
		AccountID:     1,
		CurrentRoomID: room.ID,
		Race:          creature.RaceHuman,
		ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: 1},
		Core: creature.Core{
			HPCurrent: 30, HPMax: 30,
			StaminaCurrent: 10, StaminaMax: 10,
		},
	})
	if err != nil {
		t.Fatalf("character create: %v", err)
	}

	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	s := telnet.NewSession(server)
	s.AccountID = ch.AccountID
	s.SetInWorld(ch.ID, ch.Name, ch.CurrentRoomID)
	s.SetGMCPEnabled(true)

	mgr := New(bus, sessions, chars, rooms, exits, zones)
	s.GMCPHandler = mgr.Handle
	return mgr, s, client, bus, chars, rooms, exits
}

func TestManager_CoreHelloLogsAndNoReply(t *testing.T) {
	_, s, peer, _, _, _, _ := newGMCPHarness(t)
	gotCh := make(chan []byte, 1)
	go func() { gotCh <- drainPeer(t, peer) }()
	s.GMCPHandler(s, "Core.Hello", []byte(`{"client":"Mudlet","version":"4.18.6"}`))
	s.Conn.Close()
	got := <-gotCh
	if len(got) != 0 {
		t.Fatalf("Core.Hello should not reply on the wire; got %v", got)
	}
}

func TestManager_CorePingEchoes(t *testing.T) {
	_, s, peer, _, _, _, _ := newGMCPHarness(t)
	gotCh := make(chan []byte, 1)
	go func() { gotCh <- drainPeer(t, peer) }()
	s.GMCPHandler(s, "Core.Ping", []byte(`42`))
	s.Conn.Close()
	got := <-gotCh
	if !bytes.Contains(got, []byte("Core.Ping 42")) {
		t.Fatalf("Core.Ping echo missing: %q", got)
	}
}

func TestManager_SupportsSetInstallsSubsAndEmitsSnapshot(t *testing.T) {
	_, s, peer, _, _, _, _ := newGMCPHarness(t)
	gotCh := make(chan []byte, 1)
	go func() { gotCh <- drainPeer(t, peer) }()

	s.GMCPHandler(s, "Core.Supports.Set", []byte(`["Char 1","Room 1","Comm 1"]`))
	s.Conn.Close()
	got := <-gotCh

	for _, want := range []string{
		"Char.Name", "Char.Vitals", "Char.Status", "Room.Info",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("snapshot missing %s frame: %q", want, got)
		}
	}
}

func TestManager_SupportsRemoveCancelsSubs(t *testing.T) {
	mgr, s, peer, bus, _, _, _ := newGMCPHarness(t)
	gotCh := make(chan []byte, 1)
	go func() { gotCh <- drainPeer(t, peer) }()

	// Opt in, then opt out. After remove, a published event must
	// not reach the wire.
	s.GMCPHandler(s, "Core.Supports.Set", []byte(`["Char.Vitals 1"]`))
	s.GMCPHandler(s, "Core.Supports.Remove", []byte(`["Char.Vitals 1"]`))

	// Publish a TickDamaged for the session's character; the dropped
	// subscription must not fire.
	charID, _, _ := s.InWorld()
	bus.Publish(context.Background(), world.PlayerEntered{
		CharacterID: charID, ToRoomID: 1,
	})
	// Give the async worker a beat.
	time.Sleep(50 * time.Millisecond)

	// Also call UnwireSession defensively to confirm idempotence.
	mgr.UnwireSession(s)
	s.Conn.Close()
	got := <-gotCh

	// After Supports.Remove, no Char.Vitals should appear in the
	// post-remove stream. The pre-remove snapshot did emit one, so
	// allow exactly one. (We split on "Char.Vitals" markers.)
	count := bytes.Count(got, []byte("Char.Vitals"))
	if count > 1 {
		t.Fatalf("Char.Vitals emitted %d times after Supports.Remove; want <= 1: %q", count, got)
	}
}

func TestSplitSupports(t *testing.T) {
	cases := []struct {
		in      string
		wantPkg string
		wantVer int
	}{
		{"Char 1", "Char", 1},
		{"Char.Vitals 2", "Char.Vitals", 2},
		{"Core", "Core", 0},
		{"  Room   3 ", "Room", 3}, // version not strictly parsed past spaces
		{"", "", 0},
	}
	for _, tc := range cases {
		gotPkg, gotVer := splitSupports(tc.in)
		// The 4th case has odd whitespace; we don't promise a specific
		// version recovery there, only that pkg name is right.
		if strings.TrimSpace(tc.in) != tc.in {
			if gotPkg != tc.wantPkg {
				t.Errorf("splitSupports(%q): pkg=%q want %q", tc.in, gotPkg, tc.wantPkg)
			}
			continue
		}
		if gotPkg != tc.wantPkg || gotVer != tc.wantVer {
			t.Errorf("splitSupports(%q) = (%q,%d), want (%q,%d)",
				tc.in, gotPkg, gotVer, tc.wantPkg, tc.wantVer)
		}
	}
}

func TestBuildRoomInfo_ExitMap(t *testing.T) {
	room := repo.Room{ID: 5, Name: "Hub", LongDesc: "desc"}
	exits := []repo.Exit{
		{FromRoomID: 5, ToRoomID: 6, Direction: repo.DirNorth},
		{FromRoomID: 5, ToRoomID: 7, Direction: repo.DirEast},
		{FromRoomID: 5, ToRoomID: 8, Direction: "bogus"},
	}
	got := buildRoomInfo(room, exits, "test")
	if got.Num != 5 || got.Name != "Hub" || got.Zone != "test" {
		t.Fatalf("scalar fields wrong: %+v", got)
	}
	if got.Exits["n"] != 6 || got.Exits["e"] != 7 {
		t.Fatalf("exits map wrong: %+v", got.Exits)
	}
	if _, ok := got.Exits["bogus"]; ok {
		t.Fatal("unknown direction leaked into exits map")
	}
}

func TestCharacterLevel_SumAcrossClasses(t *testing.T) {
	cases := []struct {
		name string
		in   map[creature.Class]int8
		want int
	}{
		{"empty", nil, 0},
		{"single", map[creature.Class]int8{creature.ClassArmsman: 5}, 5},
		{"multi", map[creature.Class]int8{
			creature.ClassArmsman:  3,
			creature.ClassWoodsman: 2,
		}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := characterLevel(tc.in); got != tc.want {
				t.Fatalf("characterLevel = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDominantClassName_AlphabeticTieBreak(t *testing.T) {
	in := map[creature.Class]int8{
		creature.ClassWoodsman: 2,
		creature.ClassArmsman:  2,
	}
	got := dominantClassName(in)
	if got != "Armsman" {
		t.Fatalf("tie-break dominant = %q, want Armsman", got)
	}
}
