package combat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

func TestDecayer_PopDueOnlyReturnsExpired(t *testing.T) {
	d := NewDecayer(nil, nil, nil)
	t0 := time.Unix(1_000_000, 0)
	d.Schedule(1, 10, t0.Add(10*time.Second))
	d.Schedule(2, 20, t0.Add(20*time.Second))
	d.Schedule(3, 30, t0.Add(30*time.Second))

	if got := d.Pending(); got != 3 {
		t.Fatalf("pending=%d want 3", got)
	}

	due := d.popDue(t0.Add(15 * time.Second))
	if len(due) != 1 || due[0].ItemID != 1 {
		t.Fatalf("popDue@15s = %+v want [1]", due)
	}
	if got := d.Pending(); got != 2 {
		t.Fatalf("pending=%d want 2", got)
	}

	due = d.popDue(t0.Add(60 * time.Second))
	if len(due) != 2 || due[0].ItemID != 2 || due[1].ItemID != 3 {
		t.Fatalf("popDue@60s = %+v want [2 3]", due)
	}
	if got := d.Pending(); got != 0 {
		t.Fatalf("pending=%d want 0", got)
	}
}

func TestDecayer_ScheduleSortsAcrossOutOfOrderInsert(t *testing.T) {
	d := NewDecayer(nil, nil, nil)
	t0 := time.Unix(0, 0)
	d.Schedule(2, 0, t0.Add(20*time.Second))
	d.Schedule(1, 0, t0.Add(10*time.Second))
	d.Schedule(3, 0, t0.Add(30*time.Second))

	due := d.popDue(t0.Add(time.Hour))
	want := []int64{1, 2, 3}
	if len(due) != 3 {
		t.Fatalf("popDue n=%d want 3", len(due))
	}
	for i, e := range due {
		if e.ItemID != want[i] {
			t.Fatalf("popDue[%d].ItemID=%d want %d", i, e.ItemID, want[i])
		}
	}
}

func TestDecayer_TickDeletesAndBroadcasts(t *testing.T) {
	ctx := context.Background()
	items := repo.NewMemoryItemRepo()
	created, err := items.Create(ctx, repo.Item{
		ExternalID: "corpse-test-1",
		Name:       "corpse of trolloc",
		RoomID:     42,
		Type:       repo.ItemTypeContainer,
		Stats:      &repo.ContainerStats{CapacityLbs: 100, CapacityCuFt: 10},
	})
	if err != nil {
		t.Fatalf("seed item: %v", err)
	}

	var (
		mu        sync.Mutex
		broadcast []string
		broomID   int64
	)
	bus := eventbus.New()
	var gotEvents []CorpseDecayed
	eventbus.Subscribe[CorpseDecayed](bus, func(_ context.Context, e CorpseDecayed) {
		mu.Lock()
		gotEvents = append(gotEvents, e)
		mu.Unlock()
	})

	d := NewDecayer(items, func(rid int64, msg string) {
		mu.Lock()
		broadcast = append(broadcast, msg)
		broomID = rid
		mu.Unlock()
	}, bus)

	t0 := time.Unix(2_000_000, 0)
	d.SetClock(func() time.Time { return t0.Add(10 * time.Minute) })
	d.Schedule(created.ID, created.RoomID, t0.Add(5*time.Minute))

	d.Tick(ctx)

	if _, err := items.GetByID(ctx, created.ID); err != repo.ErrItemNotFound {
		t.Fatalf("item still present after decay: err=%v", err)
	}
	mu.Lock()
	if len(broadcast) != 1 {
		t.Fatalf("broadcast count=%d want 1: %+v", len(broadcast), broadcast)
	}
	if broomID != 42 {
		t.Fatalf("broadcast roomID=%d want 42", broomID)
	}
	if len(gotEvents) != 1 || gotEvents[0].ItemID != created.ID || gotEvents[0].RoomID != 42 {
		t.Fatalf("CorpseDecayed events=%+v", gotEvents)
	}
	mu.Unlock()
	if got := d.Pending(); got != 0 {
		t.Fatalf("pending=%d want 0", got)
	}
}

func TestDecayer_TickNotYetDueIsNoop(t *testing.T) {
	ctx := context.Background()
	items := repo.NewMemoryItemRepo()
	created, err := items.Create(ctx, repo.Item{
		ExternalID: "corpse-pending",
		Name:       "corpse of trolloc",
		RoomID:     7,
		Type:       repo.ItemTypeContainer,
		Stats:      &repo.ContainerStats{CapacityLbs: 100, CapacityCuFt: 10},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	var fires int
	d := NewDecayer(items, func(int64, string) { fires++ }, nil)
	t0 := time.Unix(3_000_000, 0)
	d.SetClock(func() time.Time { return t0 })
	d.Schedule(created.ID, created.RoomID, t0.Add(5*time.Minute))

	d.Tick(ctx)

	if fires != 0 {
		t.Fatalf("broadcast fired %d times before due", fires)
	}
	if got := d.Pending(); got != 1 {
		t.Fatalf("pending=%d want 1", got)
	}
	if _, err := items.GetByID(ctx, created.ID); err != nil {
		t.Fatalf("item gone before due: %v", err)
	}
}

func TestDecayer_NilDepsAreSafe(t *testing.T) {
	d := NewDecayer(nil, nil, nil)
	t0 := time.Unix(0, 0)
	d.SetClock(func() time.Time { return t0.Add(time.Hour) })
	d.Schedule(99, 1, t0)
	// Must not panic with nil items / nil broadcast / nil bus.
	d.Tick(context.Background())
	if got := d.Pending(); got != 0 {
		t.Fatalf("pending=%d want 0", got)
	}
}

func TestDecayer_ScheduleZeroIDIsIgnored(t *testing.T) {
	d := NewDecayer(nil, nil, nil)
	d.Schedule(0, 1, time.Now())
	if got := d.Pending(); got != 0 {
		t.Fatalf("pending=%d want 0 (zero id should be dropped)", got)
	}
}

func TestDecayer_RearmFromRepo(t *testing.T) {
	ctx := context.Background()
	items := repo.NewMemoryItemRepo()
	t0 := time.Unix(2_000_000, 0).UTC()

	past := t0.Add(-1 * time.Minute)
	future := t0.Add(5 * time.Minute)

	expired, err := items.Create(ctx, repo.Item{
		ExternalID:     "corpse-expired",
		Name:           "corpse of trolloc",
		RoomID:         42,
		Type:           repo.ItemTypeContainer,
		Stats:          &repo.ContainerStats{CapacityLbs: 100, CapacityCuFt: 10},
		DecayExpiresAt: &past,
	})
	if err != nil {
		t.Fatalf("seed expired: %v", err)
	}
	live, err := items.Create(ctx, repo.Item{
		ExternalID:     "corpse-live",
		Name:           "corpse of orc",
		RoomID:         43,
		Type:           repo.ItemTypeContainer,
		Stats:          &repo.ContainerStats{CapacityLbs: 100, CapacityCuFt: 10},
		DecayExpiresAt: &future,
	})
	if err != nil {
		t.Fatalf("seed live: %v", err)
	}

	d := NewDecayer(items, nil, nil)
	rescheduled, swept, err := d.RearmFromRepo(ctx, items, t0)
	if err != nil {
		t.Fatalf("RearmFromRepo: %v", err)
	}
	if rescheduled != 1 || swept != 1 {
		t.Fatalf("counts = rescheduled=%d swept=%d, want 1/1", rescheduled, swept)
	}
	if got := d.Pending(); got != 1 {
		t.Fatalf("queue len = %d, want 1", got)
	}
	if _, err := items.GetByID(ctx, expired.ID); err != repo.ErrItemNotFound {
		t.Fatalf("expired item still present: err=%v", err)
	}
	if _, err := items.GetByID(ctx, live.ID); err != nil {
		t.Fatalf("live item gone: %v", err)
	}

	// Advancing the clock past the live deadline drains it via the
	// normal Tick path — proves the Schedule actually landed.
	d.SetClock(func() time.Time { return future.Add(time.Second) })
	d.Tick(ctx)
	if _, err := items.GetByID(ctx, live.ID); err != repo.ErrItemNotFound {
		t.Fatalf("live item not swept after deadline: err=%v", err)
	}
	if got := d.Pending(); got != 0 {
		t.Fatalf("queue len after sweep = %d, want 0", got)
	}
}

func TestDecayer_RearmFromRepo_NilSafe(t *testing.T) {
	var d *Decayer
	r, s, err := d.RearmFromRepo(context.Background(), repo.NewMemoryItemRepo(), time.Now())
	if err != nil || r != 0 || s != 0 {
		t.Fatalf("nil receiver: r=%d s=%d err=%v", r, s, err)
	}
	d2 := NewDecayer(nil, nil, nil)
	r, s, err = d2.RearmFromRepo(context.Background(), nil, time.Now())
	if err != nil || r != 0 || s != 0 {
		t.Fatalf("nil items: r=%d s=%d err=%v", r, s, err)
	}
}

func TestDecayer_NilReceiverNoop(t *testing.T) {
	var d *Decayer
	d.Schedule(1, 1, time.Now()) // must not panic
	d.Tick(context.Background())
	if d.Pending() != 0 {
		t.Fatal("nil receiver Pending should be 0")
	}
}
