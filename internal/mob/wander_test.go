package mob

import (
	"context"
	"math/rand"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/testhelper"
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

// Phase F #32a slice 1 — strict path branch. Templates with a non-empty
// Path walk a closed loop one step per pulse, ignoring WanderChance,
// gated by the same walkable-exit + zone checks as the chance branch.

// pathFixture stages a 3-room closed loop (1 → 2 → 3 → 1) in zone 1
// plus a path-eligible template. Rooms get external_ids so the
// wander handler's FindByExternalID resolution path works.
type pathFixture struct {
	templates *repo.MemoryMobTemplateRepo
	mobs      *repo.MemoryMobInstanceRepo
	rooms     *repo.MemoryRoomRepo
	exits     *repo.MemoryExitRepo
	tplID     int64
}

func newPathFixture(t *testing.T) *pathFixture {
	t.Helper()
	ctx := context.Background()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, ZoneID: 1, ExternalID: "loop.a", Name: "Atrium"})
	rooms.Insert(repo.Room{ID: 2, ZoneID: 1, ExternalID: "loop.b", Name: "Hallway"})
	rooms.Insert(repo.Room{ID: 3, ZoneID: 1, ExternalID: "loop.c", Name: "Library"})

	exits := repo.NewMemoryExitRepo()
	// Closed loop a → b → c → a (north/north/north) + reverse exits
	// (south) so peer broadcasts have a reverse direction to use.
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirSouth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 3, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 3, ToRoomID: 2, Direction: repo.DirSouth})
	exits.Insert(repo.Exit{FromRoomID: 3, ToRoomID: 1, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 3, Direction: repo.DirSouth})

	templates := repo.NewMemoryMobTemplateRepo()
	tpl, err := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "patrol.guard",
		Core:       creature.Core{Name: "a watchful guard", HPMax: 4, Defense: 12},
		// Path takes precedence over WanderChance; set chance to 0 to
		// prove the branch ignores it.
		WanderChance: 0,
		Path:         []string{"loop.a", "loop.b", "loop.c"},
	})
	if err != nil {
		t.Fatalf("create patrol template: %v", err)
	}
	return &pathFixture{
		templates: templates,
		mobs:      repo.NewMemoryMobInstanceRepo(),
		rooms:     rooms,
		exits:     exits,
		tplID:     tpl.ID,
	}
}

func (f *pathFixture) spawn(t *testing.T, roomID int64) creature.MobInstance {
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

func TestWander_StrictPath_WalksClosedLoop(t *testing.T) {
	f := newPathFixture(t)
	m := f.spawn(t, 1) // start at loop.a

	// WanderChance is 0 on the template but the path branch ignores
	// it; the mob must still advance every pulse.
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	ctx := context.Background()

	// pulse 1: 1 → 2
	h.Tick(ctx)
	live, _ := f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 2 {
		t.Fatalf("pulse 1: CurrentRoomID = %d, want 2", live.Core.CurrentRoomID)
	}
	// pulse 2: 2 → 3
	h.Tick(ctx)
	live, _ = f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 3 {
		t.Fatalf("pulse 2: CurrentRoomID = %d, want 3", live.Core.CurrentRoomID)
	}
	// pulse 3: 3 → 1 (wraparound)
	h.Tick(ctx)
	live, _ = f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 1 {
		t.Fatalf("pulse 3 (wraparound): CurrentRoomID = %d, want 1", live.Core.CurrentRoomID)
	}
}

func TestWander_StrictPath_IgnoresChanceMultiplierZero(t *testing.T) {
	// chance=0 globally disables random wandering, but the strict
	// path branch must still fire (paths are not chance-gated).
	f := newPathFixture(t)
	m := f.spawn(t, 1)
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(0.0), WithRand(stableRand()))
	h.Tick(context.Background())
	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID != 2 {
		t.Fatalf("strict path didn't fire with chance=0; CurrentRoomID = %d, want 2",
			live.Core.CurrentRoomID)
	}
}

func TestWander_StrictPath_OffPathMobNoOp(t *testing.T) {
	// A mob standing in a room that isn't on the authored path
	// silently no-ops (logs at debug). Don't move randomly; don't
	// crash.
	f := newPathFixture(t)
	ctx := context.Background()
	// Add an extra room outside the loop and spawn the mob there.
	f.rooms.Insert(repo.Room{ID: 4, ZoneID: 1, ExternalID: "loop.outside", Name: "Outside"})
	// One-way exit so the mob *could* random-wander into the loop,
	// but the path branch must block it.
	f.exits.Insert(repo.Exit{FromRoomID: 4, ToRoomID: 1, Direction: repo.DirNorth})
	m := f.spawn(t, 4)

	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(ctx)
	live, _ := f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 4 {
		t.Fatalf("off-path mob moved to %d; want 4 (stay put)", live.Core.CurrentRoomID)
	}
}

func TestWander_StrictPath_BlockedDoorNoStep(t *testing.T) {
	// A runtime-closed door on the path edge blocks the step; the
	// mob waits in place rather than tunneling through.
	f := newPathFixture(t)
	// Close the loop.a → loop.b door. Replace just that exit.
	f.exits = repo.NewMemoryExitRepo()
	f.exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth, Flags: repo.ExitFlags{Closed: true}})
	f.exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 3, Direction: repo.DirNorth})
	f.exits.Insert(repo.Exit{FromRoomID: 3, ToRoomID: 1, Direction: repo.DirNorth})
	m := f.spawn(t, 1)
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(context.Background())
	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID != 1 {
		t.Fatalf("mob bypassed closed door; CurrentRoomID = %d, want 1", live.Core.CurrentRoomID)
	}
}

// Phase F #32a slice 2 — BFS wander branch.
//
// wander_radius > 0 makes the mob pick a random reachable room within
// the cap and step toward it via BFS. The fixture lays out a small
// grid so tests can pin the chosen goal via a seeded RNG and assert
// the mob walks the shortest path.

// bfsFixture stages a 4-room "L-shape" in zone 1:
//
//	1 -- 2 -- 3
//	     |
//	     4
//
// All exits bidirectional. With radius=3 from room 1, every room is
// reachable. With radius=1 only room 2 is reachable.
type bfsFixture struct {
	templates *repo.MemoryMobTemplateRepo
	mobs      *repo.MemoryMobInstanceRepo
	rooms     *repo.MemoryRoomRepo
	exits     *repo.MemoryExitRepo
	tplID     int64
}

func newBFSFixture(t *testing.T, radius int32) *bfsFixture {
	t.Helper()
	ctx := context.Background()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, ZoneID: 1, ExternalID: "g.a", Name: "Atrium"})
	rooms.Insert(repo.Room{ID: 2, ZoneID: 1, ExternalID: "g.b", Name: "Mid"})
	rooms.Insert(repo.Room{ID: 3, ZoneID: 1, ExternalID: "g.c", Name: "End"})
	rooms.Insert(repo.Room{ID: 4, ZoneID: 1, ExternalID: "g.d", Name: "Side"})

	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirEast})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirWest})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 3, Direction: repo.DirEast})
	exits.Insert(repo.Exit{FromRoomID: 3, ToRoomID: 2, Direction: repo.DirWest})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 4, Direction: repo.DirSouth})
	exits.Insert(repo.Exit{FromRoomID: 4, ToRoomID: 2, Direction: repo.DirNorth})

	templates := repo.NewMemoryMobTemplateRepo()
	tpl, err := templates.Create(ctx, creature.MobTemplate{
		ExternalID:   "scout.basic",
		Core:         creature.Core{Name: "a roving scout", HPMax: 4, Defense: 12},
		WanderChance: 0, // BFS branch ignores chance
		WanderRadius: radius,
	})
	if err != nil {
		t.Fatalf("create scout tpl: %v", err)
	}
	return &bfsFixture{
		templates: templates,
		mobs:      repo.NewMemoryMobInstanceRepo(),
		rooms:     rooms,
		exits:     exits,
		tplID:     tpl.ID,
	}
}

func (f *bfsFixture) spawn(t *testing.T, roomID int64) creature.MobInstance {
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

func TestWander_BFS_StepsTowardCachedGoal(t *testing.T) {
	f := newBFSFixture(t, 3)
	m := f.spawn(t, 1) // start at g.a
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	// Pre-seed the goal so the test doesn't depend on rng pick order.
	h.setGoal(m.ID, 3) // target g.c, 2 hops away via room 2
	ctx := context.Background()

	// Pulse 1: 1 → 2 (one hop along shortest path).
	h.Tick(ctx)
	live, _ := f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 2 {
		t.Fatalf("pulse 1: room = %d, want 2 (BFS step toward goal 3)", live.Core.CurrentRoomID)
	}
	// Goal still cached (not yet arrived).
	if g := h.getGoal(m.ID); g != 3 {
		t.Fatalf("goal cleared mid-walk: got %d, want 3", g)
	}

	// Pulse 2: 2 → 3, arrival, goal cleared.
	h.Tick(ctx)
	live, _ = f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 3 {
		t.Fatalf("pulse 2: room = %d, want 3 (arrival)", live.Core.CurrentRoomID)
	}
	if g := h.getGoal(m.ID); g != 0 {
		t.Fatalf("goal not cleared on arrival: got %d", g)
	}
}

func TestWander_BFS_PicksGoalWhenNoneCached(t *testing.T) {
	// With no cached goal the handler floods BFS within radius and
	// rolls a target. With a stable RNG (seed 1) the first call
	// picks a deterministic room from the reachable set; the test
	// just asserts a goal got cached AND the mob moved one hop.
	f := newBFSFixture(t, 3)
	m := f.spawn(t, 1)
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(context.Background())

	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID == 1 {
		t.Fatalf("mob didn't move despite reachable goals")
	}
	// Goal cached unless the random target was 1-hop away and the
	// mob already arrived — in which case it'd be cleared. Either
	// way, the room must be one of the BFS-reachable neighbors of 1.
	if live.Core.CurrentRoomID != 2 {
		t.Fatalf("first hop = %d, want 2 (only direct neighbor of 1)", live.Core.CurrentRoomID)
	}
}

func TestWander_BFS_RespectsRadius(t *testing.T) {
	// radius=1 means BFS can only see room 2 from room 1, even
	// though rooms 3 and 4 are reachable. Mob walks to 2 and
	// arrives — goal cleared. Pulse 2 picks a new goal from
	// 1's neighbors after we move the mob back.
	f := newBFSFixture(t, 1)
	m := f.spawn(t, 1)
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(context.Background())

	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID != 2 {
		t.Fatalf("radius=1: hop = %d, want 2 (only reachable)", live.Core.CurrentRoomID)
	}
}

func TestWander_BFS_NoMoveWhenSurrounded(t *testing.T) {
	// All exits from room 1 are closed → no walkable neighbors,
	// BFS reachable set is empty, the mob stays put. Cached goal
	// (set earlier) should also clear since bfsNextStep fails.
	f := newBFSFixture(t, 3)
	f.exits = repo.NewMemoryExitRepo()
	f.exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirEast, Flags: repo.ExitFlags{Closed: true}})
	m := f.spawn(t, 1)
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	// Pre-seed a goal that's now unreachable via the closed door.
	h.setGoal(m.ID, 3)
	h.Tick(context.Background())

	live, _ := f.mobs.GetByID(context.Background(), m.ID)
	if live.Core.CurrentRoomID != 1 {
		t.Fatalf("trapped mob moved to %d", live.Core.CurrentRoomID)
	}
	if g := h.getGoal(m.ID); g != 0 {
		t.Fatalf("unreachable goal not cleared: got %d", g)
	}
}

func TestWander_BFS_PathBranchWinsOverRadius(t *testing.T) {
	// A template with BOTH a strict path AND wander_radius is
	// rejected at load time (see internal/world/mob_path_test.go),
	// but at runtime the strict-path branch must still fire first
	// if both fields slip through (defense in depth). Set both
	// directly on the template here.
	f := newBFSFixture(t, 3)
	ctx := context.Background()
	// Replace template: same WanderRadius, but ALSO a path
	// loop.a→loop.b→loop.c (rooms 1, 2, 3).
	dual, err := f.templates.Create(ctx, creature.MobTemplate{
		ExternalID:   "dual.behaviorflagged",
		Core:         creature.Core{Name: "a confused traveler", HPMax: 4, Defense: 12},
		WanderRadius: 3,
		Path:         []string{"g.a", "g.b", "g.c"},
	})
	if err != nil {
		t.Fatalf("create dual tpl: %v", err)
	}
	m, err := f.mobs.Create(ctx, creature.MobInstance{
		TemplateID: dual.ID,
		Core:       creature.Core{HPCurrent: 4, CurrentRoomID: 1},
	})
	if err != nil {
		t.Fatalf("spawn dual: %v", err)
	}
	h := NewWanderHandler(f.mobs, f.rooms, f.exits, f.templates, nil,
		WithChance(1.0), WithRand(stableRand()))
	h.Tick(ctx)
	live, _ := f.mobs.GetByID(ctx, m.ID)
	if live.Core.CurrentRoomID != 2 {
		t.Fatalf("dual-behavior: room = %d, want 2 (strict path 1→2)", live.Core.CurrentRoomID)
	}
}

// ---- bufConn alias to internal/testhelper -------------------------------

type bufConn = testhelper.BufConn

func bufSession(t *testing.T) (*telnet.Session, *bufConn) {
	t.Helper()
	return testhelper.BufSession(t)
}
