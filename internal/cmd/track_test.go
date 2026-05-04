package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// trackFixture stages a 3-room world (1↔2 north/south, 1→3 east) plus
// one mob template. Tests spawn instances and stamp trails directly.
type trackFixture struct {
	rooms     *repo.MemoryRoomRepo
	exits     *repo.MemoryExitRepo
	mobs      *repo.MemoryMobInstanceRepo
	templates *repo.MemoryMobTemplateRepo
	tplID     int64
}

func newTrackFixture(t *testing.T) *trackFixture {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Plaza"})
	rooms.Insert(repo.Room{ID: 2, Name: "North Road"})
	rooms.Insert(repo.Room{ID: 3, Name: "East Lane"})

	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirSouth})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 3, Direction: repo.DirEast})

	templates := repo.NewMemoryMobTemplateRepo()
	tpl, err := templates.Create(context.Background(), creature.MobTemplate{
		ExternalID: "rat.basic",
		Core:       creature.Core{Name: "a brown rat", HPMax: 4, Defense: 12},
	})
	if err != nil {
		t.Fatalf("create tpl: %v", err)
	}

	return &trackFixture{
		rooms:     rooms,
		exits:     exits,
		mobs:      repo.NewMemoryMobInstanceRepo(),
		templates: templates,
		tplID:     tpl.ID,
	}
}

func (f *trackFixture) spawn(t *testing.T, roomID int64) creature.MobInstance {
	t.Helper()
	m, err := f.mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: f.tplID,
		Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID, Name: "a brown rat"},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	return m
}

func adminSession(t *testing.T) (*telnet.Session, *bufConn) {
	t.Helper()
	s, c := bufSession(t)
	s.AccountID = 1
	s.AuthLevel = telnet.AuthAdmin
	s.CharacterID = 1
	s.CharacterName = "Admin"
	s.CurrentRoomID = 1
	return s, c
}

func TestTrack_UnknownNameYieldsNoTrail(t *testing.T) {
	f := newTrackFixture(t)
	cmd := NewTrack(f.mobs, f.rooms, f.exits)
	s, out := adminSession(t)
	runCmd(t, cmd, s, "ghost")
	if !strings.Contains(out.String(), "no trail of that name") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestTrack_NoMovementRecorded(t *testing.T) {
	f := newTrackFixture(t)
	f.spawn(t, 1)
	cmd := NewTrack(f.mobs, f.rooms, f.exits)
	s, out := adminSession(t)
	runCmd(t, cmd, s, "rat")
	o := out.String()
	if !strings.Contains(o, "Plaza") {
		t.Fatalf("missing room: %q", o)
	}
	if !strings.Contains(o, "no movement recorded") {
		t.Fatalf("missing no-movement note: %q", o)
	}
}

func TestTrack_SingleTrailReportsCurrentRoom(t *testing.T) {
	f := newTrackFixture(t)
	m := f.spawn(t, 1)
	// One UpdateRoom yields one trail row; direction inference still
	// can't fire (need ≥2 points).
	if err := f.mobs.UpdateRoom(context.Background(), m.ID, 2); err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	cmd := NewTrack(f.mobs, f.rooms, f.exits)
	s, out := adminSession(t)
	runCmd(t, cmd, s, "rat")
	o := out.String()
	if !strings.Contains(o, "North Road") {
		t.Fatalf("missing current room: %q", o)
	}
	if !strings.Contains(o, "no movement recorded") {
		t.Fatalf("expected single-trail still treated as no-history: %q", o)
	}
}

func TestTrack_TwoTrailsReportDirection(t *testing.T) {
	f := newTrackFixture(t)
	m := f.spawn(t, 1)
	ctx := context.Background()
	if err := f.mobs.UpdateRoom(ctx, m.ID, 2); err != nil {
		t.Fatalf("UpdateRoom 2: %v", err)
	}
	if err := f.mobs.UpdateRoom(ctx, m.ID, 1); err != nil {
		t.Fatalf("UpdateRoom 1: %v", err)
	}
	cmd := NewTrack(f.mobs, f.rooms, f.exits)
	s, out := adminSession(t)
	runCmd(t, cmd, s, "rat")
	o := out.String()
	// Most recent step: room 2 → room 1, which is the south exit.
	if !strings.Contains(o, "Plaza") {
		t.Fatalf("missing current room: %q", o)
	}
	if !strings.Contains(o, "south") {
		t.Fatalf("missing direction: %q", o)
	}
	if !strings.Contains(o, "North Road") {
		t.Fatalf("missing previous room name: %q", o)
	}
}

func TestTrack_OrdinalDisambiguates(t *testing.T) {
	f := newTrackFixture(t)
	a := f.spawn(t, 1)
	b := f.spawn(t, 1)
	ctx := context.Background()
	// Move B once so its trail row distinguishes it from A.
	if err := f.mobs.UpdateRoom(ctx, b.ID, 2); err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	if err := f.mobs.UpdateRoom(ctx, b.ID, 1); err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	cmd := NewTrack(f.mobs, f.rooms, f.exits)
	s, out := adminSession(t)
	runCmd(t, cmd, s, "2.rat")
	_ = a
	o := out.String()
	// 2.rat is the second match (mobB) — it has trails.
	if !strings.Contains(o, "south") {
		t.Fatalf("expected 2.rat (mobB) trail with direction: %q", o)
	}
}

func TestTrack_ElapsedRendered(t *testing.T) {
	f := newTrackFixture(t)
	m := f.spawn(t, 1)
	ctx := context.Background()
	if err := f.mobs.UpdateRoom(ctx, m.ID, 2); err != nil {
		t.Fatalf("UpdateRoom 2: %v", err)
	}
	if err := f.mobs.UpdateRoom(ctx, m.ID, 1); err != nil {
		t.Fatalf("UpdateRoom 1: %v", err)
	}
	// Pin "now" 90 seconds after the latest trail row; the memory repo
	// stamps trails with time.Now().UTC(), so we resolve from that.
	trails, _ := f.mobs.RecentTrails(ctx, m.ID, 1)
	if len(trails) == 0 {
		t.Fatal("expected a trail row")
	}
	frozen := trails[0].At.Add(90 * time.Second)
	cmd := newTrackAt(f.mobs, f.rooms, f.exits, func() time.Time { return frozen })
	s, out := adminSession(t)
	runCmd(t, cmd, s, "rat")
	if !strings.Contains(out.String(), "1m30s") {
		t.Fatalf("expected '1m30s' elapsed: %q", out.String())
	}
}

func TestTrack_DefangsCfmtInNames(t *testing.T) {
	f := newTrackFixture(t)
	// Replace room 1 with a hostile name and spawn a hostile-named mob.
	f.rooms = repo.NewMemoryRoomRepo()
	f.rooms.Insert(repo.Room{ID: 1, Name: "Plaza}}::red|bold"})
	f.rooms.Insert(repo.Room{ID: 2, Name: "North Road"})
	f.rooms.Insert(repo.Room{ID: 3, Name: "East Lane"})
	ctx := context.Background()
	tpl, err := f.templates.Create(ctx, creature.MobTemplate{
		ExternalID: "evil.rat",
		Core:       creature.Core{Name: "rat::magenta\x1bbad"},
	})
	if err != nil {
		t.Fatalf("create tpl: %v", err)
	}
	if _, err := f.mobs.Create(ctx, creature.MobInstance{
		TemplateID: tpl.ID,
		Core:       creature.Core{HPCurrent: 1, CurrentRoomID: 1, Name: "rat::magenta\x1bbad"},
	}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	cmd := NewTrack(f.mobs, f.rooms, f.exits)
	s, out := adminSession(t)
	runCmd(t, cmd, s, "rat")
	o := out.String()
	if strings.Contains(o, "}}::red") {
		t.Fatalf("room name }}:: not defanged: %q", o)
	}
	if strings.Contains(o, "::magenta") {
		t.Fatalf("mob name :: not defanged: %q", o)
	}
	if strings.Contains(o, "\x1b") {
		// only the surrounding cfmt-rendered escapes are allowed; raw
		// 0x1b coming from the mob name is the failure.
		// out also contains ANSI from cfmt rendering, so look for the
		// specific injected sentinel byte mid-name.
		if strings.Contains(o, "\x1bbad") {
			t.Fatalf("control byte from mob name leaked: %q", o)
		}
	}
}

func TestFormatTrackElapsed(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{500 * time.Millisecond, "less than a second"},
		{1 * time.Second, "1s"},
		{45 * time.Second, "45s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m30s"},
		{3600 * time.Second, "1h00m"},
		{3725 * time.Second, "1h02m"},
	}
	for _, tc := range cases {
		got := formatTrackElapsed(tc.in)
		if got != tc.want {
			t.Errorf("formatTrackElapsed(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
