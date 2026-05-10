package lua

import (
	"context"
	"errors"
	"strings"
	"testing"

	gluua "github.com/yuin/gopher-lua"
)

// V4 surface (Phase F #32 slice 4): room.players / room.mobs,
// clock.hour / clock.day, target.classes, and the apply_affect
// duration override (3rd arg).

func TestAPIv4_RoomPlayers_HappyPath(t *testing.T) {
	cat := loadScript(t, "rp", `
local players = room.players()
if #players ~= 3 then error("len=" .. #players) end
if players[1] ~= 7 or players[2] ~= 11 or players[3] ~= 42 then
    error("ids: " .. players[1] .. "," .. players[2] .. "," .. players[3])
end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		RoomPlayers: func() ([]int64, error) {
			return []int64{7, 11, 42}, nil
		},
	}
	if err := r.Run(context.Background(), "rp", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv4_RoomPlayers_EmptyTable(t *testing.T) {
	cat := loadScript(t, "rp_empty", `
local players = room.players()
if #players ~= 0 then error("expected empty table, got " .. #players) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		RoomPlayers: func() ([]int64, error) { return nil, nil },
	}
	if err := r.Run(context.Background(), "rp_empty", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv4_RoomPlayers_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "rp_nil", `room.players()`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "rp_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "room.players not bound") {
		t.Fatalf("err = %v; want 'room.players not bound'", err)
	}
}

func TestAPIv4_RoomMobs_HappyPath(t *testing.T) {
	cat := loadScript(t, "rm", `
local mobs = room.mobs()
if #mobs ~= 2 or mobs[1] ~= 100 or mobs[2] ~= 200 then error("ids") end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		RoomMobs: func() ([]int64, error) { return []int64{100, 200}, nil },
	}
	if err := r.Run(context.Background(), "rm", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv4_RoomMobs_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "rm_nil", `room.mobs()`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "rm_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "room.mobs not bound") {
		t.Fatalf("err = %v; want 'room.mobs not bound'", err)
	}
}

func TestAPIv4_ClockHour_ReturnsValue(t *testing.T) {
	cat := loadScript(t, "ch", `
local h = clock.hour()
if h ~= 14 then error("hour=" .. h) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{ClockHour: func() int { return 14 }}
	if err := r.Run(context.Background(), "ch", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv4_ClockDay_ReturnsValue(t *testing.T) {
	cat := loadScript(t, "cd", `
local d = clock.day()
if d ~= 42 then error("day=" .. d) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{ClockDay: func() int64 { return 42 }}
	if err := r.Run(context.Background(), "cd", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv4_ClockHour_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "ch_nil", `clock.hour()`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "ch_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "clock.hour not bound") {
		t.Fatalf("err = %v; want 'clock.hour not bound'", err)
	}
}

func TestAPIv4_TargetClasses_HappyPath(t *testing.T) {
	cat := loadScript(t, "tc", `
local classes = target.classes(7)
if classes.warrior ~= 3 then error("warrior=" .. tostring(classes.warrior)) end
if classes.rogue ~= 2 then error("rogue=" .. tostring(classes.rogue)) end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		TargetClasses: func(id int64) (map[string]int, error) {
			if id != 7 {
				t.Errorf("hook id = %d, want 7", id)
			}
			return map[string]int{"warrior": 3, "rogue": 2}, nil
		},
	}
	if err := r.Run(context.Background(), "tc", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPIv4_TargetClasses_NilBoundIsClassified(t *testing.T) {
	cat := loadScript(t, "tc_nil", `target.classes(1)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	err := r.Run(context.Background(), "tc_nil", func(l *gluua.LState) { (APIBindings{}).Bind(l) })
	if !errors.Is(err, ErrLuaError) || !strings.Contains(err.Error(), "target.classes not bound") {
		t.Fatalf("err = %v; want 'target.classes not bound'", err)
	}
}

func TestAPIv4_ApplyAffect_DurationOverride(t *testing.T) {
	cat := loadScript(t, "aa_dur", `apply_affect(7, "weak_poison", 30)`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var seenDur int32
	bindings := APIBindings{
		ApplyAffect: func(_ int64, _ string, dur int32) error {
			seenDur = dur
			return nil
		},
	}
	if err := r.Run(context.Background(), "aa_dur", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seenDur != 30 {
		t.Fatalf("duration override not propagated: got %d, want 30", seenDur)
	}
}

func TestAPIv4_ApplyAffect_NoOverrideDefaultsZero(t *testing.T) {
	cat := loadScript(t, "aa_nodur", `apply_affect(7, "weak_poison")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var seenDur int32 = -1
	bindings := APIBindings{
		ApplyAffect: func(_ int64, _ string, dur int32) error {
			seenDur = dur
			return nil
		},
	}
	if err := r.Run(context.Background(), "aa_nodur", func(l *gluua.LState) { bindings.Bind(l) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seenDur != 0 {
		t.Fatalf("2-arg call should pass durationOverride=0; got %d", seenDur)
	}
}

// Release wipes the V4 globals (room + clock) just like every other
// borrow. Bundle two scripts so we exercise multiple pool LStates.
func TestAPIv4_ReleaseClearsV4Globals(t *testing.T) {
	body := func(name, src string) (string, []byte) { return name + ".lua", []byte(src) }
	n1, b1 := body("with_v4", `room.players(); room.mobs(); clock.hour(); clock.day(); target.classes(1)`)
	n2, b2 := body("leakprobe_v4", `
if type(room) ~= "nil" then error("room should be nil, got " .. type(room)) end
if type(clock) ~= "nil" then error("clock should be nil, got " .. type(clock)) end
`)
	parser := gluua.NewState()
	defer parser.Close()
	cat := loadCatalogMulti(t, parser, map[string][]byte{n1: b1, n2: b2})

	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		RoomPlayers:   func() ([]int64, error) { return nil, nil },
		RoomMobs:      func() ([]int64, error) { return nil, nil },
		ClockHour:     func() int { return 0 },
		ClockDay:      func() int64 { return 0 },
		TargetClasses: func(int64) (map[string]int, error) { return nil, nil },
	}
	bind := func(l *gluua.LState) { bindings.Bind(l) }
	for i := 0; i < poolSize; i++ {
		if err := r.Run(context.Background(), "with_v4", bind); err != nil {
			t.Fatalf("with_v4 #%d: %v", i, err)
		}
	}
	for i := 0; i < poolSize; i++ {
		if err := r.Run(context.Background(), "leakprobe_v4", nil); err != nil {
			t.Fatalf("leakprobe_v4 #%d (release should have wiped): %v", i, err)
		}
	}
}
