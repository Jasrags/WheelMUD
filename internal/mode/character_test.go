package mode

import (
	"context"
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

type charFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	chars    *repo.MemoryCharacterRepo
	game     *stubMode
	captured *safeBuf
}

// pushCharacterSelect builds a fixture in CharacterSelect mode for
// account 1 with the given seeded character names.
func pushCharacterSelect(t *testing.T, names []string) *charFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	var seeded []repo.Character
	for _, n := range names {
		c, err := cr.Create(context.Background(), repo.Character{AccountID: 1, Name: n})
		if err != nil {
			t.Fatalf("seed %q: %v", n, err)
		}
		seeded = append(seeded, c)
	}
	game := &stubMode{name: "game"}
	mode := NewCharacterSelect(seeded, cr, game)

	s := telnet.NewSession(server)
	s.AccountID = 1

	// Start drain BEFORE PushMode — OnEnter writes the character list
	// and would otherwise block on the unbuffered net.Pipe.
	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}

	return &charFixture{t: t, session: s, peer: client, chars: cr, game: game, captured: captured}
}

func (f *charFixture) feed(line string) {
	f.t.Helper()
	if err := f.session.CurrentMode().Handle(context.Background(), f.session, line); err != nil && err != telnet.ErrSessionEnded {
		f.t.Fatalf("Handle %q: %v", line, err)
	}
	time.Sleep(20 * time.Millisecond)
}

func TestCharacterSelect_ListsOnEntry(t *testing.T) {
	f := pushCharacterSelect(t, []string{"Alpha", "Beta"})
	time.Sleep(20 * time.Millisecond)
	out := f.captured.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("expected both characters in listing: %q", out)
	}
}

func TestCharacterSelect_PickPromotes(t *testing.T) {
	f := pushCharacterSelect(t, []string{"Alpha", "Beta"})
	f.feed("alpha") // case-insensitive
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %v, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Alpha" {
		t.Fatalf("CharacterName = %q, want Alpha", f.session.CharacterName)
	}
}

func TestCharacterSelect_RejectsUnknown(t *testing.T) {
	f := pushCharacterSelect(t, []string{"Alpha", "Beta"})
	f.feed("ghost")
	if f.session.CurrentMode() != f.session.CurrentMode().(*CharacterSelect) {
		// Stay in select on unknown name.
	}
	if _, ok := f.session.CurrentMode().(*CharacterSelect); !ok {
		t.Fatal("unknown name should stay in CharacterSelect")
	}
	if !strings.Contains(f.captured.String(), "No such character") {
		t.Fatalf("expected reject message: %q", f.captured.String())
	}
}

func TestCharacterSelect_OtherAccountsCharacterRejected(t *testing.T) {
	// Seed a character that exists in the repo but belongs to a
	// different account — must not be playable from this session.
	f := pushCharacterSelect(t, []string{"Alpha"})
	if _, err := f.chars.Create(context.Background(), repo.Character{AccountID: 999, Name: "Stranger"}); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	f.feed("Stranger")
	if _, ok := f.session.CurrentMode().(*CharacterSelect); !ok {
		t.Fatal("foreign character must not promote — must stay in select")
	}
	if !strings.Contains(f.captured.String(), "No such character on this account") {
		t.Fatalf("expected reject message: %q", f.captured.String())
	}
}

func TestCharacterSelect_CreateRoutesToCharacterCreate(t *testing.T) {
	f := pushCharacterSelect(t, []string{"Alpha", "Beta"})
	f.feed("create")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
}

// CharacterCreate tests.

type charCreateFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	chars    *repo.MemoryCharacterRepo
	game     *stubMode
	captured *safeBuf
}

func pushCharacterCreate(t *testing.T) *charCreateFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	game := &stubMode{name: "game"}
	mode := NewCharacterCreate(cr, game)

	s := telnet.NewSession(server)
	s.AccountID = 1

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}

	return &charCreateFixture{t: t, session: s, peer: client, chars: cr, game: game, captured: captured}
}

func (f *charCreateFixture) feed(line string) {
	f.t.Helper()
	if err := f.session.CurrentMode().Handle(context.Background(), f.session, line); err != nil {
		f.t.Fatalf("Handle %q: %v", line, err)
	}
	time.Sleep(20 * time.Millisecond)
}

func TestCharacterCreate_HappyPath(t *testing.T) {
	f := pushCharacterCreate(t)
	f.feed("Hero")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %v, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Hero" {
		t.Fatalf("CharacterName = %q, want Hero", f.session.CharacterName)
	}
	got, err := f.chars.FindByName(context.Background(), "Hero")
	if err != nil {
		t.Fatalf("character not persisted: %v", err)
	}
	if got.AccountID != 1 {
		t.Fatalf("AccountID = %d, want 1", got.AccountID)
	}
}

func TestCharacterCreate_FirstCharacterIsAdmin(t *testing.T) {
	f := pushCharacterCreate(t)
	f.feed("Hero")
	got, err := f.chars.FindByName(context.Background(), "Hero")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.AuthLevel != repo.AuthLevelAdmin {
		t.Fatalf("first character AuthLevel = %d, want AuthLevelAdmin", got.AuthLevel)
	}
	if f.session.AuthLevel != telnet.AuthAdmin {
		t.Fatalf("session AuthLevel = %d, want AuthAdmin", f.session.AuthLevel)
	}
}

func TestCharacterCreate_SecondCharacterIsPlayer(t *testing.T) {
	f := pushCharacterCreate(t)
	// Burn the bootstrap admin slot on a throwaway character.
	if _, err := f.chars.Create(context.Background(), repo.Character{AccountID: 999, Name: "Burn"}); err != nil {
		t.Fatalf("seed bootstrap: %v", err)
	}
	f.feed("Hero")
	got, err := f.chars.FindByName(context.Background(), "Hero")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.AuthLevel != repo.AuthLevelPlayer {
		t.Fatalf("second character AuthLevel = %d, want AuthLevelPlayer", got.AuthLevel)
	}
	if f.session.AuthLevel != telnet.AuthPlayer {
		t.Fatalf("session AuthLevel = %d, want AuthPlayer", f.session.AuthLevel)
	}
}

func TestCharacterCreate_RejectsReservedNames(t *testing.T) {
	for _, name := range []string{"create", "quit", "QUIT"} {
		t.Run(name, func(t *testing.T) {
			f := pushCharacterCreate(t)
			f.feed(name)
			if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
				t.Fatal("reserved name must keep us in CharacterCreate")
			}
			if !strings.Contains(f.captured.String(), "reserved") {
				t.Fatalf("expected reserved message: %q", f.captured.String())
			}
		})
	}
}

func TestCharacterCreate_RejectsBadCharset(t *testing.T) {
	f := pushCharacterCreate(t)
	f.feed("bad name")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatal("invalid name must keep us in CharacterCreate")
	}
}

func TestCharacterCreate_DuplicateNameStays(t *testing.T) {
	f := pushCharacterCreate(t)
	if _, err := f.chars.Create(context.Background(), repo.Character{AccountID: 999, Name: "Hero"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f.feed("Hero")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatal("duplicate name must keep us in CharacterCreate")
	}
	if !strings.Contains(f.captured.String(), "already taken") {
		t.Fatalf("expected duplicate message: %q", f.captured.String())
	}
}

// End-to-end through Login: the auto-promote-to-game path covered by
// the existing TestLogin_HappyPath uses a single-character account. Add
// the multi-character case here.

func TestLogin_MultiCharacterRoutesToAccountMenu(t *testing.T) {
	f := newLoginFixtureChars(t, []string{"Alpha", "Beta"})
	f.feed("alice")
	f.feed("correct-horse")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("CurrentMode = %T, want *AccountMenu (account has 2 chars)", f.session.CurrentMode())
	}
}

func TestLogin_ZeroCharacterRoutesToCharacterCreate(t *testing.T) {
	f := newLoginFixtureChars(t, nil)
	f.feed("alice")
	f.feed("correct-horse")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate (account has 0 chars)", f.session.CurrentMode())
	}
}

// pushCharacterCreateMulti builds a CharacterCreate fixture wired to
// a real chargen.Catalog (the embedded default), driving the multi-
// step flow rather than the legacy single-name flow.
func pushCharacterCreateMulti(t *testing.T) *charCreateFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	fsys, err := chargen.SourceFS()
	if err != nil {
		t.Fatalf("chargen source: %v", err)
	}
	cat, err := chargen.Load(fsys)
	if err != nil {
		t.Fatalf("chargen load: %v", err)
	}

	cr := repo.NewMemoryCharacterRepo()
	game := &stubMode{name: "game"}
	mode := NewCharacterCreate(cr, game)
	mode.SetCatalog(cat)

	s := telnet.NewSession(server)
	s.AccountID = 1

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}
	return &charCreateFixture{t: t, session: s, peer: client, chars: cr, game: game, captured: captured}
}

func TestCharacterCreate_Multi_HappyPath(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	f.feed("set str 14")
	f.feed("done")   // abilities → identity
	f.feed("done")   // identity → feat (defaults accepted)
	f.feed("pick 1") // first background-restricted feat
	f.feed("done")   // feat → skills
	f.feed("done")   // skills → review (forfeit unspent points)
	f.feed("yes")

	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	got, err := f.chars.FindByName(context.Background(), "Hero")
	if err != nil {
		t.Fatalf("find Hero: %v", err)
	}
	if got.Race != creature.RaceHuman {
		t.Fatalf("Race = %v, want RaceHuman", got.Race)
	}
	if got.Background != creature.BackgroundMidlander {
		t.Fatalf("Background = %v, want Midlander", got.Background)
	}
	if got.ClassLevels[creature.ClassArmsman] != 1 {
		t.Fatalf("ClassLevels = %v, want Armsman:1", got.ClassLevels)
	}
	if got.Core.Abilities.Str.Current != 14 || got.Core.Abilities.Str.Max != 14 {
		t.Fatalf("Str = %+v, want Current=Max=14", got.Core.Abilities.Str)
	}
	if got.Core.Abilities.Dex.Current != 8 {
		t.Fatalf("Dex = %+v, want Current=8 (point-buy floor)", got.Core.Abilities.Dex)
	}
	// Identity defaults (no override during the happy-path) — gender
	// male, alignment good, handedness right, age and height/weight
	// non-zero (rolled from Table 6-1).
	if got.Core.Gender != creature.GenderMale {
		t.Fatalf("Gender = %v, want Male", got.Core.Gender)
	}
	if got.Core.Alignment != creature.PostureGood {
		t.Fatalf("Alignment = %v, want Good", got.Core.Alignment)
	}
	if got.Handedness != creature.HandRight {
		t.Fatalf("Handedness = %v, want Right", got.Handedness)
	}
	if got.Age == 0 || got.HeightCm == 0 || got.WeightKg == 0 {
		t.Fatalf("identity unset: age=%d height=%d weight=%d",
			got.Age, got.HeightCm, got.WeightKg)
	}
}

func TestCharacterCreate_Multi_BackRevisesPreviousStep(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("back")
	// Now back at race step; pick ogier.
	f.feed("ogier")
	// Background list rendered. Ogier-only entries are not currently
	// in the catalog, so just go back twice and confirm we don't
	// crash and end up in name step.
	f.feed("back")
	f.feed("back")
	mc, ok := f.session.CurrentMode().(*CharacterCreate)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
	if mc.step != chargenStepName {
		t.Fatalf("step = %d after 2x back, want chargenStepName(%d)", mc.step, chargenStepName)
	}
}

func TestCharacterCreate_Multi_CancelResetsDraft(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("cancel")
	mc, ok := f.session.CurrentMode().(*CharacterCreate)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
	if mc.step != chargenStepName {
		t.Fatalf("step = %d after cancel, want chargenStepName", mc.step)
	}
	if mc.draft.Name != "" || mc.draft.Race != "" || mc.draft.BackgroundID != "" {
		t.Fatalf("cancel did not clear draft: %+v", mc.draft)
	}
}

func TestCharacterCreate_Multi_RejectsBadRace(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("dragon")
	mc, ok := f.session.CurrentMode().(*CharacterCreate)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
	if mc.step != chargenStepRace {
		t.Fatalf("step = %d, want chargenStepRace after bad race", mc.step)
	}
	if !strings.Contains(f.captured.String(), "human") {
		t.Fatalf("expected race hint: %q", f.captured.String())
	}
}

func TestCharacterCreate_Multi_BackgroundByListNumber(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1") // first background in catalog
	mc, ok := f.session.CurrentMode().(*CharacterCreate)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
	if mc.step != chargenStepClass {
		t.Fatalf("step = %d, want chargenStepClass after numeric pick", mc.step)
	}
	if mc.draft.BackgroundID == "" {
		t.Fatal("BackgroundID empty after numeric pick")
	}
}

func TestCharacterCreate_Multi_BackgroundInfo(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.captured.Reset()
	f.feed("info aiel")
	out := f.captured.String()
	// info renders the descriptor; selection step is unchanged so the
	// player can still pick an option afterwards.
	for _, want := range []string{"Aiel", "Home language", "Bonus feats", "Equipment options"} {
		if !strings.Contains(out, want) {
			t.Fatalf("info output missing %q: %q", want, out)
		}
	}
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepBackground {
		t.Fatalf("step = %d after info, want chargenStepBackground", mc.step)
	}
	if mc.draft.BackgroundID != "" {
		t.Fatalf("info should not commit a selection, got %q", mc.draft.BackgroundID)
	}
	// Numeric form too: 'info 1' shows the first bg.
	f.captured.Reset()
	f.feed("info 1")
	if !strings.Contains(f.captured.String(), "Home language") {
		t.Fatalf("info 1 did not render: %q", f.captured.String())
	}
}

func TestCharacterCreate_Multi_ClassInfo(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.captured.Reset()
	f.feed("info armsman")
	out := f.captured.String()
	for _, want := range []string{"Armsman", "Hit die", "BAB", "Saves", "Class skills"} {
		if !strings.Contains(out, want) {
			t.Fatalf("class info missing %q: %q", want, out)
		}
	}
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepClass {
		t.Fatalf("step = %d after info, want chargenStepClass", mc.step)
	}
	if mc.draft.ClassID != "" {
		t.Fatalf("info should not commit a selection, got %q", mc.draft.ClassID)
	}
}

func TestCharacterCreate_Multi_OgierClassesFilterChannelers(t *testing.T) {
	// Sanity-check the race gate: ogier picks must not see Initiate /
	// Wilder. The catalog has no ogier-race backgrounds yet (deferred
	// to #14), so we only exercise ClassesForRace transitively here.
	f := pushCharacterCreateMulti(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	mc.draft.Race = "ogier"
	for _, cl := range mc.catalog.ClassesForRace("ogier") {
		if cl.Channeler {
			t.Fatalf("ogier class list contains channeler %q", cl.ID)
		}
	}
}

func TestCharacterCreate_Multi_ReviewRejectNonYes(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	f.feed("done") // abilities → identity
	f.feed("done") // identity → feat
	f.feed("pick 1")
	f.feed("done")  // feat → skills
	f.feed("done")  // skills → review
	f.feed("maybe") // anything other than yes/y/back/cancel
	mc, ok := f.session.CurrentMode().(*CharacterCreate)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
	if mc.step != chargenStepReview {
		t.Fatalf("step = %d, want chargenStepReview after non-yes", mc.step)
	}
	if _, err := f.chars.FindByName(context.Background(), "Hero"); err == nil {
		t.Fatal("character should not have been created without yes")
	}
}

func TestCharacterCreate_Multi_DuplicateNameAtReview(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	if _, err := f.chars.Create(context.Background(), repo.Character{AccountID: 999, Name: "Hero"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	f.feed("done") // abilities → identity
	f.feed("done") // identity → feat
	f.feed("pick 1")
	f.feed("done") // feat → skills
	f.feed("done") // skills → review
	f.feed("yes")
	mc, ok := f.session.CurrentMode().(*CharacterCreate)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
	if mc.step != chargenStepName {
		t.Fatalf("step = %d, want chargenStepName after duplicate", mc.step)
	}
	if !strings.Contains(f.captured.String(), "already taken") {
		t.Fatalf("expected duplicate message: %q", f.captured.String())
	}
}

func TestCharacterCreate_Multi_AbilitiesPointBuy(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")

	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepAbilities {
		t.Fatalf("step = %d, want chargenStepAbilities after class", mc.step)
	}
	// Floor is 8 across the board.
	for i, sc := range mc.draft.Abilities {
		if sc != 8 {
			t.Fatalf("Abilities[%d] = %d, want 8 floor on entry", i, sc)
		}
	}

	// Shorthand "<abil> <n>" is accepted alongside "set <abil> <n>".
	f.feed("str 16") // costs 10
	if mc.draft.Abilities[0] != 16 {
		t.Fatalf("Str = %d, want 16", mc.draft.Abilities[0])
	}
	if got := mc.pointBuySpent(); got != 10 {
		t.Fatalf("spent = %d, want 10", got)
	}

	// Out-of-range scores are rejected.
	f.feed("set dex 19")
	if mc.draft.Abilities[1] != 8 {
		t.Fatalf("Dex = %d, out-of-range write leaked", mc.draft.Abilities[1])
	}
	if !strings.Contains(f.captured.String(), "8..18") {
		t.Fatalf("expected range hint: %q", f.captured.String())
	}

	// Spend 14 more points to land exactly at budget (10 + 6 + 6 = 22, +3 = 25).
	f.feed("set dex 14") // +6 → 16
	f.feed("set con 14") // +6 → 22
	f.feed("set int 11") // +3 → 25
	if got := mc.pointBuySpent(); got != 25 {
		t.Fatalf("spent = %d, want 25 (at budget)", got)
	}

	// Going over budget is rejected without overwriting prior state.
	prev := mc.draft.Abilities[4]
	f.feed("set wis 9") // +1 → 26, over
	if mc.draft.Abilities[4] != prev {
		t.Fatalf("Wis = %d, want %d (over-budget write leaked)", mc.draft.Abilities[4], prev)
	}
	if !strings.Contains(f.captured.String(), "Not enough points") {
		t.Fatalf("expected over-budget message: %q", f.captured.String())
	}

	// Reset returns everything to the floor.
	f.feed("reset")
	for i, sc := range mc.draft.Abilities {
		if sc != 8 {
			t.Fatalf("Abilities[%d] = %d after reset, want 8", i, sc)
		}
	}

	f.feed("done")
	if mc.step != chargenStepIdentity {
		t.Fatalf("step = %d, want chargenStepIdentity after done", mc.step)
	}
}

func TestCharacterCreate_Multi_AbilitiesBackPreservesScores(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	f.feed("set str 14")
	f.feed("done") // → identity
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepIdentity {
		t.Fatalf("step = %d, want chargenStepIdentity", mc.step)
	}
	f.feed("back")
	if mc.step != chargenStepAbilities {
		t.Fatalf("step = %d, want chargenStepAbilities after back", mc.step)
	}
	if mc.draft.Abilities[0] != 14 {
		t.Fatalf("Str = %d after back, want 14 (revisit must preserve)", mc.draft.Abilities[0])
	}
}

func TestCharacterCreate_Multi_IdentityVerbs(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	mc.SetRNG(rand.New(rand.NewSource(42)))

	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	f.feed("done") // abilities → identity (defaults stamped)

	if mc.step != chargenStepIdentity {
		t.Fatalf("step = %d, want chargenStepIdentity", mc.step)
	}
	if !mc.draft.IdentitySet {
		t.Fatal("identity defaults not stamped on entry")
	}
	if mc.draft.HeightCm == 0 || mc.draft.WeightKg == 0 {
		t.Fatalf("rolled height/weight zero: h=%d w=%d",
			mc.draft.HeightCm, mc.draft.WeightKg)
	}

	// Each verb mutates one field.
	f.feed("gender f")
	if mc.draft.Gender != creature.GenderFemale {
		t.Fatalf("Gender = %v, want Female", mc.draft.Gender)
	}
	f.feed("age 25")
	if mc.draft.Age != 25 {
		t.Fatalf("Age = %d, want 25", mc.draft.Age)
	}
	f.feed("handed left")
	if mc.draft.Handedness != creature.HandLeft {
		t.Fatalf("Handedness = %v, want Left", mc.draft.Handedness)
	}
	f.feed("align bad")
	if mc.draft.Alignment != creature.PostureBad {
		t.Fatalf("Alignment = %v, want Bad", mc.draft.Alignment)
	}

	// Bad inputs leave state untouched and surface a hint.
	prevAge := mc.draft.Age
	f.captured.Reset()
	f.feed("age zero")
	if mc.draft.Age != prevAge {
		t.Fatalf("bad age leaked: %d", mc.draft.Age)
	}
	if !strings.Contains(f.captured.String(), "Age must be") {
		t.Fatalf("expected age hint: %q", f.captured.String())
	}

	// done advances to feat (next mandatory step). Walk forward to
	// review so we can assert the review block reflects identity.
	f.captured.Reset()
	f.feed("done")
	if mc.step != chargenStepFeat {
		t.Fatalf("step = %d, want chargenStepFeat after identity done", mc.step)
	}
	f.feed("pick 1")
	f.feed("done") // feat → skills
	f.feed("done") // skills → review
	if mc.step != chargenStepReview {
		t.Fatalf("step = %d, want chargenStepReview", mc.step)
	}
	out := f.captured.String()
	for _, want := range []string{"female", "Age:", "Height:", "Weight:", "left", "bad"} {
		if !strings.Contains(out, want) {
			t.Fatalf("review missing %q: %q", want, out)
		}
	}
}

func TestCharacterCreate_Multi_IdentityRollDeterministic(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	mc.SetRNG(rand.New(rand.NewSource(7)))

	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	f.feed("done") // → identity

	h1, w1 := mc.draft.HeightCm, mc.draft.WeightKg
	// Re-rolling with the same seed twice must change the values
	// (rng advances between calls).
	f.feed("roll")
	h2, w2 := mc.draft.HeightCm, mc.draft.WeightKg
	if h1 == h2 && w1 == w2 {
		t.Fatalf("roll did not change values: h=%d w=%d", h1, w1)
	}
	// Sanity: human heights land somewhere between 4 ft and 7 ft.
	for _, h := range []int16{h1, h2} {
		if h < 120 || h > 220 {
			t.Fatalf("human height %d cm out of plausible range", h)
		}
	}
}

func TestCharacterCreate_Multi_FeatSubstep(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	mc := f.session.CurrentMode().(*CharacterCreate)

	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	f.feed("done") // abilities → identity
	f.feed("done") // identity → feat
	if mc.step != chargenStepFeat {
		t.Fatalf("step = %d, want chargenStepFeat", mc.step)
	}

	// done with no pick is rejected when options exist.
	f.captured.Reset()
	f.feed("done")
	if mc.step != chargenStepFeat {
		t.Fatalf("done without pick advanced: step=%d", mc.step)
	}
	if !strings.Contains(f.captured.String(), "Pick a feat") {
		t.Fatalf("expected pick-first hint: %q", f.captured.String())
	}

	// info renders the description but doesn't commit.
	f.captured.Reset()
	f.feed("info bullheaded")
	if !strings.Contains(f.captured.String(), "Bullheaded") {
		t.Fatalf("info missing entry: %q", f.captured.String())
	}
	if mc.draft.SelectedFeatID != "" {
		t.Fatalf("info committed selection: %q", mc.draft.SelectedFeatID)
	}

	// pick by id.
	f.feed("pick bullheaded")
	if mc.draft.SelectedFeatID != "bullheaded" {
		t.Fatalf("SelectedFeatID = %q", mc.draft.SelectedFeatID)
	}

	// Bare id is also accepted.
	f.feed("luck_of_heroes")
	if mc.draft.SelectedFeatID != "luck_of_heroes" {
		t.Fatalf("bare-id pick failed: %q", mc.draft.SelectedFeatID)
	}

	f.feed("done")
	if mc.step != chargenStepSkills {
		t.Fatalf("step = %d, want chargenStepSkills", mc.step)
	}
}

func TestCharacterCreate_Multi_SkillsSubstep(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	mc := f.session.CurrentMode().(*CharacterCreate)

	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	// Int 14 → +2 mod, armsman skill_points=2 → (2+2)*4 = 16 budget.
	f.feed("set int 14")
	f.feed("done")   // abilities → identity
	f.feed("done")   // identity → feat
	f.feed("pick 1") // any
	f.feed("done")   // feat → skills

	if mc.step != chargenStepSkills {
		t.Fatalf("step = %d, want chargenStepSkills", mc.step)
	}
	if mc.draft.SkillBudget != 16 {
		t.Fatalf("SkillBudget = %d, want 16", mc.draft.SkillBudget)
	}

	// Out of range rejected.
	f.captured.Reset()
	f.feed("rank 1 5")
	if !strings.Contains(f.captured.String(), "0..4") {
		t.Fatalf("expected range hint: %q", f.captured.String())
	}

	// Spend exactly to budget.
	skills := mc.allowedSkillIDs()
	if len(skills) < 4 {
		t.Fatalf("expected ≥4 allowed skills, got %d", len(skills))
	}
	f.feed("rank 1 4")
	f.feed("rank 2 4")
	f.feed("rank 3 4")
	f.feed("rank 4 4")
	if mc.skillsSpent() != 16 {
		t.Fatalf("skillsSpent = %d, want 16", mc.skillsSpent())
	}

	// One more rank pushes over budget — must be refused, prior state
	// preserved.
	prev := mc.draft.SkillRanks[skills[0]]
	f.captured.Reset()
	f.feed("rank 5 1")
	if !strings.Contains(f.captured.String(), "Not enough points") {
		t.Fatalf("expected over-budget hint: %q", f.captured.String())
	}
	if mc.draft.SkillRanks[skills[0]] != prev {
		t.Fatalf("prior rank corrupted on over-budget rejection")
	}

	// reset clears the map.
	f.feed("reset")
	if mc.skillsSpent() != 0 {
		t.Fatalf("reset did not clear: %d", mc.skillsSpent())
	}

	// Setting to 0 keeps the map sparse.
	f.feed("rank 1 2")
	f.feed("rank 1 0")
	if _, ok := mc.draft.SkillRanks[skills[0]]; ok {
		t.Fatalf("zero rank still present in map")
	}

	f.feed("done")
	if mc.step != chargenStepReview {
		t.Fatalf("step = %d, want chargenStepReview", mc.step)
	}
}

func TestCharacterCreate_Multi_FeatsAndSkillsPersisted(t *testing.T) {
	f := pushCharacterCreateMulti(t)

	f.feed("Hero")
	f.feed("human")
	f.feed("midlander")
	f.feed("armsman")
	f.feed("done") // abilities → identity
	f.feed("done") // identity → feat
	f.feed("pick bullheaded")
	f.feed("done") // feat → skills
	f.feed("rank 1 2")
	f.feed("done") // skills → review
	f.feed("yes")

	got, err := f.chars.FindByName(context.Background(), "Hero")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got.Feats) == 0 {
		t.Fatalf("no feats persisted")
	}
	// Picked feat + 4 midlander bonus feats = at least 4 ids
	// (deduped: bullheaded is in both, so exactly 4).
	if len(got.Feats) < 4 {
		t.Fatalf("Feats = %v, want at least 4 (bonus + pick)", got.Feats)
	}
	if catalogIDInt32("bullheaded") == 0 {
		t.Fatal("hash should not collide with zero")
	}
	found := false
	for _, id := range got.Feats {
		if id == catalogIDInt32("bullheaded") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("bullheaded not in Feats: %v", got.Feats)
	}
	if len(got.Skills) != 1 {
		t.Fatalf("Skills size = %d, want 1", len(got.Skills))
	}
	for _, sr := range got.Skills {
		if sr.Ranks != 2 || !sr.IsClassSkill {
			t.Fatalf("SkillRanks = %+v, want Ranks=2 IsClassSkill=true", sr)
		}
	}
}

func TestFirstLevelSkillBudget(t *testing.T) {
	cases := []struct {
		classPoints, intMod, want int
	}{
		{2, 2, 16}, // armsman + Int 14
		{2, 0, 8},  // armsman + Int 10
		{2, -2, 4}, // armsman + Int 6 → min 1/lvl floor → 4
		{8, 2, 40}, // noble + Int 14
		{0, -3, 4}, // pathological → floor
	}
	for _, c := range cases {
		if got := firstLevelSkillBudget(c.classPoints, c.intMod); got != c.want {
			t.Errorf("firstLevelSkillBudget(%d, %d) = %d, want %d",
				c.classPoints, c.intMod, got, c.want)
		}
	}
}

func TestRollHeightWeight_OgierTaller(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	hHuman, _ := rollHeightWeight(r, "human", creature.GenderMale, 0)
	r = rand.New(rand.NewSource(1))
	hOgier, _ := rollHeightWeight(r, "ogier", creature.GenderMale, 0)
	if hOgier <= hHuman {
		t.Fatalf("ogier height %d <= human height %d (same seed)", hOgier, hHuman)
	}
}

func TestPointBuyCost(t *testing.T) {
	cases := []struct {
		score, want int
	}{
		{8, 0}, {9, 1}, {10, 2}, {11, 3}, {12, 4}, {13, 5},
		{14, 6}, {15, 8}, {16, 10}, {17, 13}, {18, 16},
		{7, -1}, {19, -1}, {0, -1},
	}
	for _, c := range cases {
		if got := pointBuyCost(c.score); got != c.want {
			t.Errorf("pointBuyCost(%d) = %d, want %d", c.score, got, c.want)
		}
	}
}
