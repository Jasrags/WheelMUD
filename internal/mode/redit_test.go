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
	exits    *repo.MemoryExitRepo
	audits   *repo.MemoryAdminAuditRepo
	room     repo.Room
}

func newREditFixture(t *testing.T) *reditFixture {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
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
		exits:    exits,
		audits:   audits,
		room:     room,
	}
}

func (f *reditFixture) enter(t *testing.T) *REdit {
	t.Helper()
	m := NewREdit(f.rooms, f.exits, f.audits, f.rooms.FindByExternalID, f.room)
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

// --- Phase G #34 slice 2: multi-line desc -----------------------------

func TestREdit_Desc_SingleLineStillWorks(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	if err := m.Handle(context.Background(), f.s, "desc One-shot description."); err != nil {
		t.Fatalf("desc: %v", err)
	}
	if m.draft.LongDesc != "One-shot description." {
		t.Fatalf("LongDesc = %q", m.draft.LongDesc)
	}
	if !m.dirty {
		t.Fatal("dirty not set")
	}
	if m.bufActive {
		t.Fatal("single-line desc must not enter buffering mode")
	}
}

func TestREdit_Desc_MultilineBuffer_FlushesOnDot(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "desc"); err != nil {
		t.Fatalf("desc enter: %v", err)
	}
	if !m.bufActive || m.bufKind != "desc" {
		t.Fatalf("expected desc buffering active, got active=%v kind=%q", m.bufActive, m.bufKind)
	}
	for _, line := range []string{"First paragraph.", "", "Second paragraph."} {
		if err := m.Handle(ctx, f.s, line); err != nil {
			t.Fatalf("buffer line %q: %v", line, err)
		}
	}
	if err := m.Handle(ctx, f.s, "."); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if m.bufActive {
		t.Fatal("buffering still active after flush")
	}
	want := "First paragraph.\n\nSecond paragraph."
	if m.draft.LongDesc != want {
		t.Fatalf("LongDesc = %q, want %q", m.draft.LongDesc, want)
	}
	if !m.dirty {
		t.Fatal("dirty not set after flush")
	}
}

func TestREdit_Desc_MultilineBuffer_AbortDiscards(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "desc"); err != nil {
		t.Fatalf("desc enter: %v", err)
	}
	_ = m.Handle(ctx, f.s, "Throwaway line.")
	if err := m.Handle(ctx, f.s, "@abort"); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if m.bufActive {
		t.Fatal("buffering still active after abort")
	}
	if m.draft.LongDesc != "original long" {
		t.Fatalf("LongDesc clobbered by abort: %q", m.draft.LongDesc)
	}
	if m.dirty {
		t.Fatal("dirty set after abort")
	}
}

// --- Phase G #34 slice 2: extras --------------------------------------

func TestREdit_Extra_SetShowDelete(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "extra plaque An old engraved plaque hangs here."); err != nil {
		t.Fatalf("extra set: %v", err)
	}
	if got := m.draft.ExtraDescs["plaque"]; got != "An old engraved plaque hangs here." {
		t.Fatalf("ExtraDescs[plaque] = %q", got)
	}
	if !m.dirty {
		t.Fatal("dirty not set after extra set")
	}
	f.captured.Reset()
	if err := m.Handle(ctx, f.s, "extra plaque"); err != nil {
		t.Fatalf("extra show: %v", err)
	}
	if !strings.Contains(f.captured.String(), "engraved plaque") {
		t.Fatalf("show output missing body: %q", f.captured.String())
	}
	if err := m.Handle(ctx, f.s, "extra plaque delete"); err != nil {
		t.Fatalf("extra delete: %v", err)
	}
	if _, ok := m.draft.ExtraDescs["plaque"]; ok {
		t.Fatal("ExtraDescs[plaque] still present after delete")
	}
}

func TestREdit_Extra_KeywordLowercased(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	if err := m.Handle(context.Background(), f.s, "extra Plaque Mixed case."); err != nil {
		t.Fatalf("extra: %v", err)
	}
	if _, ok := m.draft.ExtraDescs["plaque"]; !ok {
		t.Fatalf("ExtraDescs key not lowercased: %v", m.draft.ExtraDescs)
	}
}

func TestREdit_Extra_MultilineBuffer(t *testing.T) {
	f := newREditFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "extra plaque ."); err != nil {
		t.Fatalf("extra .: %v", err)
	}
	if !m.bufActive || m.bufKind != "extra:plaque" {
		t.Fatalf("buffering state = %v / %q", m.bufActive, m.bufKind)
	}
	for _, line := range []string{"Line one.", "Line two."} {
		_ = m.Handle(ctx, f.s, line)
	}
	if err := m.Handle(ctx, f.s, "."); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if m.draft.ExtraDescs["plaque"] != "Line one.\nLine two." {
		t.Fatalf("ExtraDescs[plaque] = %q", m.draft.ExtraDescs["plaque"])
	}
}

// --- Phase G #34 slice 2: exit <dir> ... -------------------------------

// reditExitFixture extends the base fixture with a second room and a
// pre-created exit so the exit subverbs have something to manipulate.
type reditExitFixture struct {
	*reditFixture
	dest repo.Room
	exit repo.Exit
}

func newREditExitFixture(t *testing.T) *reditExitFixture {
	t.Helper()
	base := newREditFixture(t)
	dest := base.rooms.Insert(repo.Room{
		ExternalID: "test.room.dest",
		ZoneID:     7,
		Name:       "Destination",
		Sector:     repo.SectorCity,
	})
	ex, err := base.exits.Create(context.Background(), repo.Exit{
		FromRoomID:     base.room.ID,
		ToRoomID:       dest.ID,
		Direction:      repo.DirNorth,
		Description:    "a worn footpath",
		KeyExternalID:  "",
		LockDifficulty: 0,
	})
	if err != nil {
		t.Fatalf("seed exit: %v", err)
	}
	return &reditExitFixture{reditFixture: base, dest: dest, exit: ex}
}

func TestREdit_Exit_ShowRendersFields(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	f.captured.Reset()
	if err := m.Handle(context.Background(), f.s, "exit n"); err != nil {
		t.Fatalf("exit show: %v", err)
	}
	out := f.captured.String()
	for _, want := range []string{"Exit north", "worn footpath", "difficulty", "runtime"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q in:\n%s", want, out)
		}
	}
}

func TestREdit_Exit_UnknownDirRefuses(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	f.captured.Reset()
	if err := m.Handle(context.Background(), f.s, "exit zz"); err != nil {
		t.Fatalf("exit zz: %v", err)
	}
	if !strings.Contains(f.captured.String(), "Unknown direction") {
		t.Fatalf("missing refusal: %q", f.captured.String())
	}
}

func TestREdit_Exit_MissingDirRefuses(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	f.captured.Reset()
	if err := m.Handle(context.Background(), f.s, "exit south"); err != nil {
		t.Fatalf("exit south: %v", err)
	}
	if !strings.Contains(f.captured.String(), "No exit south") {
		t.Fatalf("missing refusal: %q", f.captured.String())
	}
}

func TestREdit_Exit_DescUpdatesRepoAndAudits(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	if err := m.Handle(ctx, f.s, "exit n desc A cobbled lane heads north."); err != nil {
		t.Fatalf("exit desc: %v", err)
	}
	got, err := f.exits.FindByDirection(ctx, f.room.ID, repo.DirNorth)
	if err != nil {
		t.Fatalf("FindByDirection: %v", err)
	}
	if got.Description != "A cobbled lane heads north." {
		t.Fatalf("Description = %q", got.Description)
	}
	if f.audits.Len() == 0 {
		t.Fatal("no audit row written for exit desc")
	}
	rows, _ := f.audits.List(ctx, repo.AdminAuditFilter{Limit: 1})
	if len(rows) != 1 {
		t.Fatalf("audit list len = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Verb != "redit_exit" {
		t.Errorf("audit verb = %q, want redit_exit", row.Verb)
	}
	if !strings.Contains(row.Target, "test.room:n:desc") {
		t.Errorf("audit target = %q", row.Target)
	}
}

func TestREdit_Exit_KeyNoneClears(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	_ = m.Handle(ctx, f.s, "exit n key key.iron")
	_ = m.Handle(ctx, f.s, "exit n key none")
	got, _ := f.exits.FindByDirection(ctx, f.room.ID, repo.DirNorth)
	if got.KeyExternalID != "" {
		t.Fatalf("KeyExternalID = %q, want empty", got.KeyExternalID)
	}
}

func TestREdit_Exit_DifficultyBoundsChecked(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	f.captured.Reset()
	if err := m.Handle(ctx, f.s, "exit n difficulty 200"); err != nil {
		t.Fatalf("difficulty: %v", err)
	}
	if !strings.Contains(f.captured.String(), "integer in [0, 100]") {
		t.Fatalf("missing bounds refusal: %q", f.captured.String())
	}
	got, _ := f.exits.FindByDirection(ctx, f.room.ID, repo.DirNorth)
	if got.LockDifficulty != 0 {
		t.Fatalf("LockDifficulty mutated by failed bounds: %d", got.LockDifficulty)
	}
	if err := m.Handle(ctx, f.s, "exit n difficulty 50"); err != nil {
		t.Fatalf("difficulty 50: %v", err)
	}
	got, _ = f.exits.FindByDirection(ctx, f.room.ID, repo.DirNorth)
	if got.LockDifficulty != 50 {
		t.Fatalf("LockDifficulty = %d, want 50", got.LockDifficulty)
	}
}

func TestREdit_Exit_FlagToggle(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	ctx := context.Background()
	_ = m.Handle(ctx, f.s, "exit n flag hidden on")
	got, _ := f.exits.FindByDirection(ctx, f.room.ID, repo.DirNorth)
	if !got.Flags.Hidden {
		t.Fatal("hidden not set")
	}
	_ = m.Handle(ctx, f.s, "exit n flag hidden")
	got, _ = f.exits.FindByDirection(ctx, f.room.ID, repo.DirNorth)
	if got.Flags.Hidden {
		t.Fatal("hidden not toggled off")
	}
}

func TestREdit_Exit_ToRetargets(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	other := f.rooms.Insert(repo.Room{ExternalID: "test.room.other", ZoneID: 7, Name: "Other"})
	if err := m.Handle(context.Background(), f.s, "exit n to test.room.other"); err != nil {
		t.Fatalf("retarget: %v", err)
	}
	got, _ := f.exits.FindByDirection(context.Background(), f.room.ID, repo.DirNorth)
	if got.ToRoomID != other.ID {
		t.Fatalf("ToRoomID = %d, want %d", got.ToRoomID, other.ID)
	}
}

func TestREdit_Exit_ToUnknownRoomRefuses(t *testing.T) {
	f := newREditExitFixture(t)
	m := f.enter(t)
	f.captured.Reset()
	if err := m.Handle(context.Background(), f.s, "exit n to nonsuch"); err != nil {
		t.Fatalf("retarget: %v", err)
	}
	if !strings.Contains(f.captured.String(), "No room with external id") {
		t.Fatalf("missing refusal: %q", f.captured.String())
	}
}
