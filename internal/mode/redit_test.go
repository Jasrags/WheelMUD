package mode

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// reditFixture wires the in-memory repos, a target room, and a piped
// session so subcommand assertions can read the visible output.
type reditFixture struct {
	s        *telnet.Session
	captured *safeBuf
	rooms    *repo.MemoryRoomRepo
	audits   *repo.MemoryAdminAuditRepo
	room     repo.Room
}

func newREditFixture(t *testing.T) *reditFixture {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	audits := repo.NewMemoryAdminAuditRepo()
	room := rooms.Insert(repo.Room{
		ExternalID: "test.room",
		ZoneID:     7,
		Name:       "Original Name",
		ShortDesc:  "original short",
		LongDesc:   "original long",
		Sector:     repo.SectorCity,
		LightLevel: 100,
	})

	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	captured := &safeBuf{}
	drainPeer(t, client, captured)
	s := telnet.NewSession(server)
	s.AccountID = 1
	s.CharacterID = 42
	s.CharacterName = "Builder"
	s.AuthLevel = telnet.AuthAdmin

	// Push a base game-stand-in so PopMode at `done`/`cancel` doesn't
	// leave the stack empty (which is a valid state but more annoying
	// to assert on than "one mode below redit").
	if err := s.PushMode(&stubMode{name: "game"}); err != nil {
		t.Fatalf("seed game push: %v", err)
	}
	return &reditFixture{
		s:        s,
		captured: captured,
		rooms:    rooms,
		audits:   audits,
		room:     room,
	}
}

func (f *reditFixture) enter(t *testing.T) *REdit {
	t.Helper()
	m := NewREdit(f.rooms, f.audits, f.room)
	if err := f.s.PushMode(m); err != nil {
		t.Fatalf("push redit: %v", err)
	}
	return m
}

func TestREdit_SetName_MarksDirty(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	if err := m.Handle(context.Background(), f.s, "name Fancy Name"); err != nil {
		t.Fatalf("name: %v", err)
	}
	if m.draft.Name != "Fancy Name" {
		t.Fatalf("draft.Name = %q, want Fancy Name", m.draft.Name)
	}
	if !m.dirty {
		t.Fatal("dirty = false after name change")
	}
}

func TestREdit_Flag_ToggleAndExplicit(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()

	// Toggle on
	if err := m.Handle(ctx, f.s, "flag dark"); err != nil {
		t.Fatalf("flag dark toggle: %v", err)
	}
	if !m.draft.Flags.Dark {
		t.Fatal("dark not set after toggle")
	}
	// Explicit off
	if err := m.Handle(ctx, f.s, "flag dark off"); err != nil {
		t.Fatalf("flag dark off: %v", err)
	}
	if m.draft.Flags.Dark {
		t.Fatal("dark still set after explicit off")
	}
	// Unknown flag
	f.captured.Reset()
	if err := m.Handle(ctx, f.s, "flag bogus on"); err != nil {
		t.Fatalf("unknown flag: %v", err)
	}
	if !strings.Contains(f.captured.String(), "Unknown flag") {
		t.Fatalf("missing refusal: %q", f.captured.String())
	}
}

func TestREdit_Sector_ValidatesAgainstEnum(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "sector forest"); err != nil {
		t.Fatalf("sector forest: %v", err)
	}
	if m.draft.Sector != repo.SectorForest {
		t.Fatalf("Sector = %q, want forest", m.draft.Sector)
	}

	f.captured.Reset()
	if err := m.Handle(ctx, f.s, "sector volcano"); err != nil {
		t.Fatalf("sector volcano: %v", err)
	}
	if m.draft.Sector != repo.SectorForest {
		t.Fatal("invalid sector mutated draft")
	}
	if !strings.Contains(f.captured.String(), "Unknown sector") {
		t.Fatalf("missing refusal: %q", f.captured.String())
	}
}

func TestREdit_Light_RangeCheck(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "light 42"); err != nil {
		t.Fatalf("light 42: %v", err)
	}
	if m.draft.LightLevel != 42 {
		t.Fatalf("LightLevel = %d", m.draft.LightLevel)
	}
	if err := m.Handle(ctx, f.s, "light 200"); err != nil {
		t.Fatalf("light 200: %v", err)
	}
	if m.draft.LightLevel != 42 {
		t.Fatal("out-of-range light mutated draft")
	}
}

func TestREdit_Done_PersistsAndAudits(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "name New Name"); err != nil {
		t.Fatalf("name: %v", err)
	}
	if err := m.Handle(ctx, f.s, "sector forest"); err != nil {
		t.Fatalf("sector: %v", err)
	}
	if err := m.Handle(ctx, f.s, "done"); err != nil {
		t.Fatalf("done: %v", err)
	}

	got, err := f.rooms.FindByID(ctx, f.room.ID)
	if err != nil {
		t.Fatalf("re-find: %v", err)
	}
	if got.Name != "New Name" {
		t.Errorf("Name = %q, want New Name", got.Name)
	}
	if got.Sector != repo.SectorForest {
		t.Errorf("Sector = %q, want forest", got.Sector)
	}
	if n := f.audits.Len(); n != 1 {
		t.Fatalf("audit count = %d, want 1", n)
	}
	entries, _ := f.audits.List(ctx, repo.AdminAuditFilter{})
	e := entries[0]
	if e.Verb != "redit" || e.Target != "test.room" {
		t.Fatalf("audit row = %+v", e)
	}
	// Changes list should mention exactly the fields the operator
	// touched; order is sorted so the assertion is stable.
	if e.Args != "name,sector" {
		t.Fatalf("audit args = %q, want name,sector", e.Args)
	}

	// Mode popped: top of stack is the stub game mode again.
	if got := f.s.CurrentMode(); got == nil {
		t.Fatal("stack empty after done")
	} else if _, ok := got.(*REdit); ok {
		t.Fatal("redit still on top after done")
	}
}

func TestREdit_DoneWithoutChangesIsNoOp(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "done"); err != nil {
		t.Fatalf("done: %v", err)
	}
	if f.audits.Len() != 0 {
		t.Fatal("no-op done wrote an audit row")
	}
	// Room state preserved.
	got, _ := f.rooms.FindByID(ctx, f.room.ID)
	if got.Name != "Original Name" {
		t.Fatalf("Name mutated by no-op done: %q", got.Name)
	}
}

func TestREdit_Cancel_DiscardsDraft(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "name Discarded"); err != nil {
		t.Fatalf("name: %v", err)
	}
	if err := m.Handle(ctx, f.s, "cancel"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, _ := f.rooms.FindByID(ctx, f.room.ID)
	if got.Name != "Original Name" {
		t.Fatalf("Name = %q, want Original Name (cancel must not commit)", got.Name)
	}
	if f.audits.Len() != 0 {
		t.Fatal("cancel wrote an audit row")
	}
}

func TestREdit_UnknownSubcmdRefuses(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	if err := m.Handle(context.Background(), f.s, "xyzzy"); err != nil {
		t.Fatalf("xyzzy: %v", err)
	}
	if !strings.Contains(f.captured.String(), "Unknown editor command") {
		t.Fatalf("missing refusal: %q", f.captured.String())
	}
}
