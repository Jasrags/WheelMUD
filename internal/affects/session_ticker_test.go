package affects

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

type fakeFights struct {
	active map[int64]bool
}

func (f *fakeFights) Active(roomID int64) bool { return f.active[roomID] }

type fakeChars struct {
	mu       sync.Mutex
	rows     map[int64]Character
	loadErr  map[int64]error
	writeErr map[int64]error
	writes   map[int64][][]creature.Affect
}

func (f *fakeChars) GetByID(_ context.Context, id int64) (Character, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.loadErr[id]; ok {
		return Character{}, err
	}
	c, ok := f.rows[id]
	if !ok {
		return Character{}, errors.New("not found")
	}
	out := make([]creature.Affect, len(c.Affects))
	copy(out, c.Affects)
	return Character{Affects: out}, nil
}

func (f *fakeChars) RecordAffects(_ context.Context, id int64, a []creature.Affect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.writeErr[id]; ok {
		return err
	}
	if f.writes == nil {
		f.writes = map[int64][][]creature.Affect{}
	}
	cp := make([]creature.Affect, len(a))
	copy(cp, a)
	f.writes[id] = append(f.writes[id], cp)
	row := f.rows[id]
	row.Affects = cp
	f.rows[id] = row
	return nil
}

type fakeBus struct {
	mu     sync.Mutex
	events []any
}

func (b *fakeBus) Publish(_ context.Context, ev any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func newFakes(rows map[int64]Character, fightRooms []int64) (*fakeChars, *fakeFights, *fakeBus) {
	chars := &fakeChars{rows: rows}
	fights := &fakeFights{active: map[int64]bool{}}
	for _, r := range fightRooms {
		fights.active[r] = true
	}
	return chars, fights, &fakeBus{}
}

func TestSessionTicker_TicksOutOfCombatCharacter(t *testing.T) {
	rows := map[int64]Character{
		7: {Affects: []creature.Affect{
			{Source: 1, Name: "blessed", DurationTicks: 2},
			{Source: 2, Name: "weakened", DurationTicks: 1},
		}},
	}
	chars, fights, bus := newFakes(rows, nil)
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	chars.mu.Lock()
	got := chars.rows[7].Affects
	chars.mu.Unlock()
	if len(got) != 1 || got[0].Name != "blessed" || got[0].DurationTicks != 1 {
		t.Fatalf("post-tick affects: %+v", got)
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.events) != 1 {
		t.Fatalf("event count: want 1, got %d", len(bus.events))
	}
	ev, ok := bus.events[0].(Expired)
	if !ok || ev.CharacterID != 7 || ev.RoomID != 100 || len(ev.Names) != 1 || ev.Names[0] != "weakened" {
		t.Fatalf("expired event: %+v", bus.events[0])
	}
}

func TestSessionTicker_SkipsInFightCharacter(t *testing.T) {
	rows := map[int64]Character{
		7: {Affects: []creature.Affect{{Name: "blessed", DurationTicks: 1}}},
	}
	chars, fights, bus := newFakes(rows, []int64{100})
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	if len(chars.writes) != 0 {
		t.Fatalf("in-fight character must not be written: %+v", chars.writes)
	}
	if len(bus.events) != 0 {
		t.Fatalf("in-fight character must not publish: %+v", bus.events)
	}
}

func TestSessionTicker_NoAffectsNoWrite(t *testing.T) {
	rows := map[int64]Character{7: {Affects: nil}}
	chars, fights, bus := newFakes(rows, nil)
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	if len(chars.writes) != 0 {
		t.Fatalf("empty-affects character must not be written: %+v", chars.writes)
	}
	if len(bus.events) != 0 {
		t.Fatalf("empty-affects character must not publish: %+v", bus.events)
	}
}

func TestSessionTicker_TickWithoutExpiryStillWritesButNoEvent(t *testing.T) {
	// Single affect at duration 5 → ticks to 4, no expiry. RecordAffects
	// is still called (caller can't pre-detect "did Tick change
	// anything" without re-running it; fine for now — write is cheap).
	rows := map[int64]Character{
		7: {Affects: []creature.Affect{{Name: "blessed", DurationTicks: 5}}},
	}
	chars, fights, bus := newFakes(rows, nil)
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	chars.mu.Lock()
	got := chars.rows[7].Affects
	chars.mu.Unlock()
	if len(got) != 1 || got[0].DurationTicks != 4 {
		t.Fatalf("decrement: %+v", got)
	}
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.events) != 0 {
		t.Fatalf("no expiry → no event; got %+v", bus.events)
	}
}

func TestSessionTicker_LoadErrorContinues(t *testing.T) {
	rows := map[int64]Character{
		7: {Affects: []creature.Affect{{Name: "x", DurationTicks: 1}}},
	}
	chars, fights, bus := newFakes(rows, nil)
	chars.loadErr = map[int64]error{8: errors.New("boom")}
	cand := func() []Candidate {
		return []Candidate{
			{CharacterID: 8, RoomID: 100},
			{CharacterID: 7, RoomID: 100},
		}
	}

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	if len(bus.events) != 1 {
		t.Fatalf("char 7 should still tick despite char 8 failure: %+v", bus.events)
	}
}

func TestSessionTicker_NilSafe(t *testing.T) {
	var tk *SessionTicker
	tk.Tick(context.Background()) // must not panic
}
