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
	mu         sync.Mutex
	rows       map[int64]Character
	loadErr    map[int64]error
	writeErr   map[int64]error
	writes     map[int64][][]creature.Affect
	coreWrites map[int64][]coreWrite
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
	return Character{
		Affects:   out,
		HPCurrent: c.HPCurrent,
		HPMax:     c.HPMax,
		Subdual:   c.Subdual,
		Condition: c.Condition,
		Position:  c.Position,
	}, nil
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

type coreWrite struct {
	HP      int32
	Subdual int32
}

func (f *fakeChars) RecordHP(_ context.Context, id int64, hp, subdual int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.coreWrites == nil {
		f.coreWrites = map[int64][]coreWrite{}
	}
	f.coreWrites[id] = append(f.coreWrites[id], coreWrite{HP: hp, Subdual: subdual})
	row := f.rows[id]
	row.HPCurrent = hp
	row.Subdual = subdual
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
	if !ok || ev.CharacterID != 7 || ev.RoomID != 100 ||
		len(ev.Entries) != 1 || ev.Entries[0].Name != "weakened" {
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

func TestSessionTicker_WriteErrorContinues(t *testing.T) {
	rows := map[int64]Character{
		7: {Affects: []creature.Affect{{Name: "x", DurationTicks: 1}}},
		8: {Affects: []creature.Affect{{Name: "y", DurationTicks: 1}}},
	}
	chars, fights, bus := newFakes(rows, nil)
	chars.writeErr = map[int64]error{7: errors.New("disk full")}
	cand := func() []Candidate {
		return []Candidate{
			{CharacterID: 7, RoomID: 100},
			{CharacterID: 8, RoomID: 100},
		}
	}

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	// char 7's write fails → no event should fire for it. char 8 still
	// ticks and emits Expired (its only affect just expired).
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.events) != 1 {
		t.Fatalf("want 1 event for char 8 only; got %d", len(bus.events))
	}
	ev, ok := bus.events[0].(Expired)
	if !ok || ev.CharacterID != 8 {
		t.Fatalf("event: %+v", bus.events[0])
	}
}

func TestSessionTicker_NilSafe(t *testing.T) {
	var tk *SessionTicker
	tk.Tick(context.Background()) // must not panic
}

func TestSessionTicker_TickEffectAppliesHPDelta(t *testing.T) {
	rows := map[int64]Character{
		7: {
			HPCurrent: 20,
			HPMax:     30,
			Affects: []creature.Affect{
				{Source: 1, Name: "weak_poison", DurationTicks: 5, TickEffect: "poison", TickDamage: -3},
			},
		},
	}
	chars, fights, bus := newFakes(rows, nil)
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	chars.mu.Lock()
	cw := chars.coreWrites[7]
	got := chars.rows[7]
	chars.mu.Unlock()
	if len(cw) != 1 || cw[0].HP != 17 {
		t.Fatalf("expected one HP write to 17, got %+v", cw)
	}
	if len(got.Affects) != 1 || got.Affects[0].DurationTicks != 4 {
		t.Fatalf("affect post-tick: %+v", got.Affects)
	}
	bus.mu.Lock()
	defer bus.mu.Unlock()
	var sawTick bool
	for _, ev := range bus.events {
		if td, ok := ev.(TickDamaged); ok {
			sawTick = true
			if td.NewHP != 17 || td.HPMax != 30 || len(td.Events) != 1 || td.Events[0].Delta != -3 {
				t.Fatalf("TickDamaged: %+v", td)
			}
		}
	}
	if !sawTick {
		t.Fatalf("expected TickDamaged event in %+v", bus.events)
	}
}

func TestSessionTicker_TickEffectKillsAndFiresDeathHook(t *testing.T) {
	rows := map[int64]Character{
		7: {
			HPCurrent: 2,
			HPMax:     30,
			Affects: []creature.Affect{
				{Source: 1, Name: "weak_poison", DurationTicks: 5, TickEffect: "poison", TickDamage: -3},
			},
		},
	}
	chars, fights, bus := newFakes(rows, nil)
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	type call struct {
		ID    int64
		Cause string
	}
	var deaths []call
	var dmu sync.Mutex
	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.SetDeathHook(func(_ context.Context, id int64, cause string) {
		dmu.Lock()
		defer dmu.Unlock()
		deaths = append(deaths, call{ID: id, Cause: cause})
	})
	tk.Tick(context.Background())

	chars.mu.Lock()
	hp := chars.rows[7].HPCurrent
	chars.mu.Unlock()
	if hp != 0 {
		t.Fatalf("HP after lethal tick should clamp at 0, got %d", hp)
	}
	dmu.Lock()
	defer dmu.Unlock()
	if len(deaths) != 1 || deaths[0].ID != 7 || deaths[0].Cause != "weak_poison" {
		t.Fatalf("death hook: %+v", deaths)
	}
}

func TestSessionTicker_HoTClampsAtMax(t *testing.T) {
	rows := map[int64]Character{
		7: {
			HPCurrent: 28,
			HPMax:     30,
			Affects: []creature.Affect{
				{Source: 1, Name: "regen", DurationTicks: 5, TickEffect: "regen", TickDamage: +5},
			},
		},
	}
	chars, fights, bus := newFakes(rows, nil)
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	chars.mu.Lock()
	hp := chars.rows[7].HPCurrent
	chars.mu.Unlock()
	if hp != 30 {
		t.Fatalf("HoT should clamp at HPMax (30), got %d", hp)
	}
}

func TestSessionTicker_DoesNotOverwriteConcurrentConditions(t *testing.T) {
	// Regression: tickOne previously called RecordCore which writes
	// hp/subdual/conditions/position_flags atomically — using the
	// snapshot's Condition value would clobber a CondProne/CondStunned
	// bit set by combat between the snapshot load and the write.
	// The fix swapped to the narrow RecordHP, which leaves conditions
	// untouched. This test verifies the fake's coreWrites only carry
	// HP+Subdual (no condition fields) and that the ticker never
	// asks the loader to write conditions.
	rows := map[int64]Character{
		7: {
			HPCurrent: 20,
			HPMax:     30,
			Affects: []creature.Affect{
				{Source: 1, Name: "weak_poison", DurationTicks: 5, TickEffect: "poison", TickDamage: -3},
			},
		},
	}
	chars, fights, bus := newFakes(rows, nil)
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	chars.mu.Lock()
	defer chars.mu.Unlock()
	cw := chars.coreWrites[7]
	if len(cw) != 1 {
		t.Fatalf("expected one HP write, got %d", len(cw))
	}
	// coreWrite struct intentionally has no Cond/Pos fields — if a
	// future change reintroduces a Conditions write here, the field
	// won't exist and this assertion (compile-time) will fail.
	if cw[0].HP != 17 || cw[0].Subdual != 0 {
		t.Fatalf("unexpected HP write payload: %+v", cw[0])
	}
}

func TestSessionTicker_ExpiredEventCarriesAuthoredMessage(t *testing.T) {
	rows := map[int64]Character{
		7: {Affects: []creature.Affect{
			{Source: 1, Name: "healing draught", DurationTicks: 1, ExpireMessage: "The healing draught's warmth fades."},
			{Source: 1, Name: "weakened", DurationTicks: 1}, // no message → fallback
		}},
	}
	chars, fights, bus := newFakes(rows, nil)
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, fights, chars, bus, nil)
	tk.Tick(context.Background())

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	ev, ok := bus.events[0].(Expired)
	if !ok {
		t.Fatalf("event type: %T", bus.events[0])
	}
	if len(ev.Entries) != 2 {
		t.Fatalf("entry count: want 2, got %d (%+v)", len(ev.Entries), ev.Entries)
	}
	if ev.Entries[0].Name != "healing draught" ||
		ev.Entries[0].Message != "The healing draught's warmth fades." {
		t.Fatalf("entry[0]: %+v", ev.Entries[0])
	}
	if ev.Entries[1].Name != "weakened" || ev.Entries[1].Message != "" {
		t.Fatalf("entry[1] should have empty Message (fallback): %+v", ev.Entries[1])
	}
}

func TestApplyTickEffects_SkipsNonTickAffects(t *testing.T) {
	c := creature.Core{
		HPCurrent: 20, HPMax: 30,
		Affects: []creature.Affect{
			{Name: "blessed", DurationTicks: 5, Modifiers: []creature.StatMod{{Field: FieldDefense, Delta: 2}}}, // no TickEffect
			{Name: "shielded", DurationTicks: 5, TickEffect: "ward", TickDamage: 0},                            // zero delta
		},
	}
	hp, evs := ApplyTickEffects(c)
	if hp != 20 {
		t.Fatalf("HP must be untouched, got %d", hp)
	}
	if len(evs) != 0 {
		t.Fatalf("no events expected: %+v", evs)
	}
}
