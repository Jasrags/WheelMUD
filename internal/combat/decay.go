package combat

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// RoomBroadcaster delivers a single line of cfmt-tagged text to every
// session whose CurrentRoomID matches roomID. Decayer takes this as a
// callback rather than depending on session.Registry directly to keep
// the combat package clear of telnet.* imports. cmd/server/main.go
// constructs the closure that walks Snapshot() and writes via
// Session.WriteAsync (the cross-session output rule).
type RoomBroadcaster func(roomID int64, msg string)

// Decayer is the in-memory queue + sweeper for time-bounded items
// (today: corpses spawned by the death pipeline). Combat.Manager
// schedules an entry on every successful corpse spawn; Decayer.Tick
// fires on tick.Buckets.Decay (30 s default) and deletes any entry
// whose At ≤ now.
//
// The queue lives in process memory only. A restart drops scheduled
// decays, leaving the items.row rows behind; an admin can purge them
// or wait for a future durable variant. V1 corpses are empty so the
// staleness is cosmetic.
type Decayer struct {
	items     repo.ItemRepo
	broadcast RoomBroadcaster // optional; nil disables in-room "crumble" line
	bus       *eventbus.Bus   // optional; CorpseDecayed publish path

	mu    sync.Mutex
	queue []decayEntry
	now   func() time.Time
}

type decayEntry struct {
	ItemID int64
	RoomID int64
	At     time.Time
}

// NewDecayer constructs a Decayer. broadcast and bus are both
// optional; pass nil to silence either side. items is required for
// the sweeper to delete due rows — a nil repo turns the Decayer into
// an inert queue (mostly useful for tests that only assert pop
// ordering).
func NewDecayer(items repo.ItemRepo, broadcast RoomBroadcaster, bus *eventbus.Bus) *Decayer {
	return &Decayer{
		items:     items,
		broadcast: broadcast,
		bus:       bus,
		now:       time.Now,
	}
}

// SetClock injects a deterministic time source. Tests use this; the
// constructor wires time.Now by default.
func (d *Decayer) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	d.mu.Lock()
	d.now = now
	d.mu.Unlock()
}

// Schedule inserts an entry, keeping the slice sorted by At ascending
// so popDue can stop scanning at the first not-yet-due element.
func (d *Decayer) Schedule(itemID, roomID int64, at time.Time) {
	if d == nil || itemID == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue = append(d.queue, decayEntry{ItemID: itemID, RoomID: roomID, At: at})
	sort.SliceStable(d.queue, func(i, j int) bool {
		return d.queue[i].At.Before(d.queue[j].At)
	})
}

// Pending returns the current queue length. Test-only accessor.
func (d *Decayer) Pending() int {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.queue)
}

// popDue removes and returns every entry whose At ≤ now. The queue is
// kept sorted by Schedule, so a single linear scan suffices.
func (d *Decayer) popDue(now time.Time) []decayEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.queue) == 0 {
		return nil
	}
	cut := 0
	for cut < len(d.queue) && !d.queue[cut].At.After(now) {
		cut++
	}
	if cut == 0 {
		return nil
	}
	due := make([]decayEntry, cut)
	copy(due, d.queue[:cut])
	d.queue = append(d.queue[:0], d.queue[cut:]...)
	return due
}

// Tick is the bucket subscription. Pops every due entry, deletes the
// item row (best-effort), broadcasts the crumble line, and publishes
// CorpseDecayed. Errors are logged and swallowed — one bad row must
// not stop the rest of the sweep.
func (d *Decayer) Tick(ctx context.Context) {
	if d == nil {
		return
	}
	due := d.popDue(d.now())
	for _, e := range due {
		if d.items != nil {
			if err := d.items.Delete(ctx, e.ItemID); err != nil {
				slog.Warn("combat: corpse decay delete failed",
					"item", e.ItemID, "room", e.RoomID, "error", err)
			}
		}
		if d.broadcast != nil && e.RoomID != 0 {
			d.broadcast(e.RoomID, "{{The corpse crumbles to dust.}}::dim")
		}
		if d.bus != nil {
			d.bus.Publish(ctx, CorpseDecayed{RoomID: e.RoomID, ItemID: e.ItemID})
		}
	}
}
