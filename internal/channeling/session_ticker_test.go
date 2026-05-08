package channeling

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

type fakeChars struct {
	mu       sync.Mutex
	rows     map[int64]Character
	loadErr  map[int64]error
	writeErr map[int64]error
	writes   map[int64]int
}

func (f *fakeChars) GetByID(_ context.Context, id int64) (Character, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.loadErr[id]; ok {
		return Character{}, err
	}
	row, ok := f.rows[id]
	if !ok {
		return Character{}, errors.New("not found")
	}
	if row.Channeling == nil {
		return Character{}, nil
	}
	cp := *row.Channeling
	return Character{Channeling: &cp}, nil
}

func (f *fakeChars) RecordChanneling(_ context.Context, id int64, c *creature.Channeling) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.writeErr[id]; ok {
		return err
	}
	if f.writes == nil {
		f.writes = map[int64]int{}
	}
	f.writes[id]++
	row := f.rows[id]
	if c == nil {
		row.Channeling = nil
	} else {
		cp := *c
		row.Channeling = &cp
	}
	f.rows[id] = row
	return nil
}

func mkChanneler(opts func(*creature.Channeling)) Character {
	c := &creature.Channeling{
		GenderSource: creature.SourceSaidin,
		Slots: [10]creature.SlotPool{
			{Cur: 0, Max: 4},
			{Cur: 0, Max: 4},
		},
	}
	if opts != nil {
		opts(c)
	}
	return Character{Channeling: c}
}

func TestSessionTicker_RefreshesDueChanneler(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rows := map[int64]Character{
		7: mkChanneler(func(c *creature.Channeling) {
			c.LastSlotRefreshAt = now.Add(-9 * time.Hour)
		}),
	}
	chars := &fakeChars{rows: rows}
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, chars, func() time.Time { return now }, nil)
	tk.Tick(context.Background())

	chars.mu.Lock()
	defer chars.mu.Unlock()
	if got := chars.rows[7].Channeling.Slots[0].Cur; got != 4 {
		t.Fatalf("slot 0 not refilled: %d", got)
	}
	if !chars.rows[7].Channeling.LastSlotRefreshAt.Equal(now) {
		t.Fatalf("timestamp not stamped: %v", chars.rows[7].Channeling.LastSlotRefreshAt)
	}
	if chars.writes[7] != 1 {
		t.Fatalf("expected 1 write, got %d", chars.writes[7])
	}
}

func TestSessionTicker_NoWriteWhenIdle(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rows := map[int64]Character{
		7: mkChanneler(func(c *creature.Channeling) {
			c.LastSlotRefreshAt = now.Add(-1 * time.Hour) // not due
			// not embraced → no madness
		}),
	}
	chars := &fakeChars{rows: rows}
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, chars, func() time.Time { return now }, nil)
	tk.Tick(context.Background())

	if len(chars.writes) != 0 {
		t.Fatalf("idle channeler must not write: %+v", chars.writes)
	}
}

func TestSessionTicker_AccruesMadnessForEmbracedSaidin(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rows := map[int64]Character{
		7: mkChanneler(func(c *creature.Channeling) {
			c.LastSlotRefreshAt = now.Add(-1 * time.Hour) // not due for refresh
			c.Embraced = true
			c.Madness = 0
		}),
	}
	chars := &fakeChars{rows: rows}
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, chars, func() time.Time { return now }, nil)
	tk.Tick(context.Background())

	chars.mu.Lock()
	defer chars.mu.Unlock()
	if got := chars.rows[7].Channeling.Madness; got != 1 {
		t.Fatalf("madness: got %d want 1", got)
	}
	if chars.writes[7] != 1 {
		t.Fatalf("expected 1 write, got %d", chars.writes[7])
	}
}

func TestSessionTicker_SkipsNonChannelers(t *testing.T) {
	rows := map[int64]Character{
		7: {Channeling: nil},
	}
	chars := &fakeChars{rows: rows}
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, chars, nil, nil)
	tk.Tick(context.Background())

	if len(chars.writes) != 0 {
		t.Fatalf("non-channeler must not write: %+v", chars.writes)
	}
}

func TestSessionTicker_LoadErrorContinues(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rows := map[int64]Character{
		7: mkChanneler(func(c *creature.Channeling) {
			c.LastSlotRefreshAt = now.Add(-9 * time.Hour)
		}),
	}
	chars := &fakeChars{
		rows:    rows,
		loadErr: map[int64]error{8: errors.New("boom")},
	}
	cand := func() []Candidate {
		return []Candidate{
			{CharacterID: 8, RoomID: 100},
			{CharacterID: 7, RoomID: 100},
		}
	}

	tk := NewSessionTicker(cand, chars, func() time.Time { return now }, nil)
	tk.Tick(context.Background())

	if chars.writes[7] != 1 {
		t.Fatalf("char 7 should still tick: %+v", chars.writes)
	}
}

func TestSessionTicker_WriteErrorDoesNotPanic(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rows := map[int64]Character{
		7: mkChanneler(func(c *creature.Channeling) {
			c.LastSlotRefreshAt = now.Add(-9 * time.Hour)
		}),
	}
	chars := &fakeChars{
		rows:     rows,
		writeErr: map[int64]error{7: errors.New("disk full")},
	}
	cand := func() []Candidate { return []Candidate{{CharacterID: 7, RoomID: 100}} }

	tk := NewSessionTicker(cand, chars, func() time.Time { return now }, nil)
	tk.Tick(context.Background()) // must not panic
}

func TestSessionTicker_NilSafe(t *testing.T) {
	var tk *SessionTicker
	tk.Tick(context.Background()) // must not panic
}
