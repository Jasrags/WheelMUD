package mob

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// fixture stages a small in-memory world: zone 1 with rooms 1 and 2
// linked north/south, zone 2 with room 3 reachable east from room 1
// (so the cross-zone gate can be exercised). One mob template is
// created; the caller spawns instances per test as needed.
type fixture struct {
	templates *repo.MemoryMobTemplateRepo
	mobs      *repo.MemoryMobInstanceRepo
	rooms     *repo.MemoryRoomRepo
	exits     *repo.MemoryExitRepo
	tplID     int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, ZoneID: 1, Name: "Plaza"})
	rooms.Insert(repo.Room{ID: 2, ZoneID: 1, Name: "North Road"})
	rooms.Insert(repo.Room{ID: 3, ZoneID: 2, Name: "Foreign Town"})

	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirSouth})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 3, Direction: repo.DirEast}) // cross-zone

	templates := repo.NewMemoryMobTemplateRepo()
	tpl, err := templates.Create(ctx, creature.MobTemplate{
		ExternalID:   "rat.basic",
		Core:         creature.Core{Name: "a brown rat", HPMax: 4, Defense: 12},
		WanderChance: 1.0,
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	return &fixture{
		templates: templates,
		mobs:      repo.NewMemoryMobInstanceRepo(),
		rooms:     rooms,
		exits:     exits,
		tplID:     tpl.ID,
	}
}

func (f *fixture) spawn(t *testing.T, roomID int64) creature.MobInstance {
	t.Helper()
	m, err := f.mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: f.tplID,
		Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	return m
}

// stableRand returns an RNG that always picks index 0 from any slice
// and rolls 0.0 from Float64, so chance=1.0 always succeeds and
// candidate selection is deterministic.
func stableRand() *rand.Rand { return rand.New(rand.NewSource(1)) }

func TestWander_RoomZeroSkipped(t *testing.T) {
	f := newFixture(t)
	m := f.spawn(t, 1)
	if err := f.mobs.UpdateRoom(context.Background(), m.ID, 0); err != nil {
		t.Fatalf("despawn: %v", err)
	}
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(context.Background())
	live, err := f.mobs.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if live.Core.CurrentRoomID != 0 {
		t.Fatalf("despawned mob moved to %d", live.Core.CurrentRoomID)
	}
}

func TestWander_SentinelDoesNotMove(t *testing.T) {
	f := newFixture(t)
	// Replace template with a Sentinel one.
	ctx := context.Background()
	sentTpl, err := f.templates.Create(ctx, creature.MobTemplate{
		ExternalID:    "guard",
		Core:          creature.Core{Name: "a stoic guard", HPMax: 8},
		BehaviorFlags: creature.BehavSentinel,
		WanderChance:  1.0, // proves Sentinel gates regardless of chance
	})
	if err != nil {
		t.Fatalf("create sentinel tpl: %v", err)
	}
	m, err := f.mobs.Create(ctx, creature.MobInstance{
		TemplateID: sentTpl.ID,
		Core:       creature.Core{HPCurrent: 8, CurrentRoomID: 1},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(ctx)
	live, _ := f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 1 {
		t.Fatalf("sentinel wandered to %d", live.Core.CurrentRoomID)
	}
}

func TestWander_ChanceZeroNoMove(t *testing.T) {
	f := newFixture(t)
	m := f.spawn(t, 1)
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(0.0), WithRand(stableRand()))
	h.Tick(context.Background())
	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID != 1 {
		t.Fatalf("mob moved despite chance=0")
	}
	trails, _ := f.mobs.RecentTrails(context.Background(), m.ID, 4)
	if len(trails) != 0 {
		t.Fatalf("trail recorded with chance=0: %+v", trails)
	}
}

func TestWander_EligibleMoveRecordsTrail(t *testing.T) {
	f := newFixture(t)
	m := f.spawn(t, 1)
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	ctx := context.Background()
	h.Tick(ctx)
	live, _ := f.mobs.GetByID(ctx, m.ID)
	// Dest is room 2 (north, in-zone) — never room 3 (cross-zone).
	if live.Core.CurrentRoomID != 2 {
		t.Fatalf("CurrentRoomID = %d, want 2", live.Core.CurrentRoomID)
	}
	trails, err := f.mobs.RecentTrails(ctx, m.ID, 4)
	if err != nil {
		t.Fatalf("RecentTrails: %v", err)
	}
	if len(trails) != 1 || trails[0].RoomID != 2 {
		t.Fatalf("trails = %+v, want one entry for room 2", trails)
	}
}

func TestWander_BlockedExitsSkipped(t *testing.T) {
	cases := []struct {
		name  string
		flags repo.ExitFlags
	}{
		{"hidden", repo.ExitFlags{Hidden: true}},
		{"closed", repo.ExitFlags{Closed: true}},
		{"locked", repo.ExitFlags{Locked: true}},
		{"nopass", repo.ExitFlags{NoPass: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			// Replace north exit with a blocked one (still in-zone).
			f.exits = repo.NewMemoryExitRepo()
			f.exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth, Flags: tc.flags})
			f.exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 3, Direction: repo.DirEast}) // cross-zone
			m := f.spawn(t, 1)
			h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
				WithChance(1.0), WithRand(stableRand()))
			h.Tick(context.Background())
			live, _ := f.mobs.GetByID(context.Background(), m.ID)
			if live.Core.CurrentRoomID != 1 {
				t.Fatalf("mob crossed %s exit, now at room %d", tc.name, live.Core.CurrentRoomID)
			}
		})
	}
}

func TestWander_CrossZoneExitSkipped(t *testing.T) {
	f := newFixture(t)
	// Drop the in-zone north exit; only the cross-zone east exit remains.
	f.exits = repo.NewMemoryExitRepo()
	f.exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 3, Direction: repo.DirEast})
	m := f.spawn(t, 1)
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(context.Background())
	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID != 1 {
		t.Fatalf("mob crossed zone boundary to room %d", live.Core.CurrentRoomID)
	}
}

func TestWander_PerPulseCapHonored(t *testing.T) {
	f := newFixture(t)
	// Spawn 5 mobs in room 1; cap to 2.
	ids := make([]int64, 5)
	for i := range ids {
		ids[i] = f.spawn(t, 1).ID
	}
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithCap(2), WithRand(stableRand()))
	h.Tick(context.Background())

	moved := 0
	for _, id := range ids {
		live, _ := f.mobs.GetByID(context.Background(), id)
		if live.Core.CurrentRoomID != 1 {
			moved++
		}
	}
	if moved != 2 {
		t.Fatalf("moved = %d, want 2 (cap)", moved)
	}
}

func TestWander_TemplateChanceZeroNeverMoves(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	idleTpl, err := f.templates.Create(ctx, creature.MobTemplate{
		ExternalID:   "couch.potato",
		Core:         creature.Core{Name: "a lazy slug", HPMax: 1},
		WanderChance: 0,
	})
	if err != nil {
		t.Fatalf("create idle tpl: %v", err)
	}
	m, err := f.mobs.Create(ctx, creature.MobInstance{
		TemplateID: idleTpl.ID,
		Core:       creature.Core{HPCurrent: 1, CurrentRoomID: 1},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Multiplier is 1.0 (default); template chance 0 must still gate.
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil, WithRand(stableRand()))
	h.Tick(ctx)
	live, _ := f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 1 {
		t.Fatalf("template chance 0 mob moved to room %d", live.Core.CurrentRoomID)
	}
}

func TestWander_MultiplierZeroDisablesAllTemplates(t *testing.T) {
	f := newFixture(t)
	m := f.spawn(t, 1) // tpl WanderChance = 1.0
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(0.0), WithRand(stableRand()))
	h.Tick(context.Background())
	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID != 1 {
		t.Fatalf("multiplier 0 still moved mob to room %d", live.Core.CurrentRoomID)
	}
}

func TestWander_BroadcastReachesSourceAndDest(t *testing.T) {
	f := newFixture(t)
	m := f.spawn(t, 1)

	sessions := session.NewRegistry()
	src, srcOut := bufSession(t)
	src.AccountID = 1
	src.CharacterName = "Alice"
	src.CurrentRoomID = 1
	sessions.Bind(src.AccountID, src)

	dst, dstOut := bufSession(t)
	dst.AccountID = 2
	dst.CharacterName = "Bob"
	dst.CurrentRoomID = 2
	sessions.Bind(dst.AccountID, dst)

	bystander, byOut := bufSession(t)
	bystander.AccountID = 3
	bystander.CharacterName = "Carol"
	bystander.CurrentRoomID = 3 // unrelated room
	sessions.Bind(bystander.AccountID, bystander)

	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, sessions,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(context.Background())

	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID != 2 {
		t.Fatalf("mob did not move; got room %d", live.Core.CurrentRoomID)
	}
	if !strings.Contains(srcOut.String(), "leaves north") {
		t.Fatalf("source room missed leave broadcast: %q", srcOut.String())
	}
	if !strings.Contains(dstOut.String(), "arrives from the south") {
		t.Fatalf("dest room missed arrive broadcast: %q", dstOut.String())
	}
	if byOut.String() != "" {
		t.Fatalf("bystander received broadcast: %q", byOut.String())
	}
}

// ---- minimal local bufConn helper (mirrors internal/cmd/look_test.go) ----

type bufConn struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed chan struct{}
	once   sync.Once
}

func newBufConn() *bufConn { return &bufConn{closed: make(chan struct{})} }

func (b *bufConn) Read(_ []byte) (int, error) {
	<-b.closed
	return 0, errClosed
}
func (b *bufConn) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *bufConn) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}
func (b *bufConn) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
func (b *bufConn) LocalAddr() net.Addr                { return fakeAddr{} }
func (b *bufConn) RemoteAddr() net.Addr               { return fakeAddr{} }
func (b *bufConn) SetDeadline(_ time.Time) error      { return nil }
func (b *bufConn) SetReadDeadline(_ time.Time) error  { return nil }
func (b *bufConn) SetWriteDeadline(_ time.Time) error { return nil }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake:0" }

var errClosed = errors.New("buf conn closed")

func bufSession(t *testing.T) (*telnet.Session, *bufConn) {
	t.Helper()
	c := newBufConn()
	s := telnet.NewSession(c)
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	t.Cleanup(func() { c.Close() })
	return s, c
}
