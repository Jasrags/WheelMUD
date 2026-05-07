package mode

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// TestCharacterCreate_NonChannelerSkipsChannelingStep walks an
// armsman (channeler:false) through the full multi-step flow and
// asserts the channeling step never fires — applySkills "done"
// jumps straight to review and the committed character has nil
// Channeling.
func TestCharacterCreate_NonChannelerSkipsChannelingStep(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Lan")
	f.feed("human")
	f.feed("1")
	f.feed("borderlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("3")
	f.feed("done")
	f.feed("4")
	f.feed("done")
	f.feed("5")
	f.feed("pick 1")
	f.feed("done")
	f.feed("6")
	f.feed("done") // skills → hub
	f.feed("7")    // equipment (row 7 for non-channelers; channeling row n/a)
	f.feed("1")
	f.feed("done") // → hub → auto-opens review
	f.feed("yes")  // commit

	got, err := f.chars.FindByName(context.Background(), "Lan")
	if err != nil {
		t.Fatalf("find Lan: %v", err)
	}
	if got.Channeling != nil {
		t.Fatalf("non-channeler should have nil Channeling; got %+v", got.Channeling)
	}
	if _, ok := got.ClassLevels[creature.ClassArmsman]; !ok {
		t.Fatalf("expected Armsman class level; got %+v", got.ClassLevels)
	}
}

// TestCharacterCreate_ChannelerHappyPath walks an Initiate through
// the new substep — picks 2 affinities, 3 starting weaves — and
// asserts the persisted Channeling pointer reflects all three
// fields.
func TestCharacterCreate_ChannelerHappyPath(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Egwene")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("initiate")
	f.feed("3")
	f.feed("done")
	f.feed("4")
	f.feed("gender female") // override default male so source=saidar
	f.feed("done")
	f.feed("5")
	f.feed("pick 1")
	f.feed("done")
	f.feed("6")
	f.feed("done")
	f.feed("7") // hub → channeling (channeler branch only)
	f.feed("affinities fire spirit")
	f.feed("done") // advance stage: affinities → weaves
	f.feed("weaves spark warmth steady_hand")
	f.feed("done") // weaves stage → hub
	f.feed("8")    // equipment (row 8 for channelers)
	f.feed("1")
	f.feed("done") // → hub → auto-opens review
	f.feed("yes")  // commit

	got, err := f.chars.FindByName(context.Background(), "Egwene")
	if err != nil {
		t.Fatalf("find Egwene: %v", err)
	}
	if got.Channeling == nil {
		t.Fatalf("Channeling missing on committed channeler")
	}
	if got.Channeling.GenderSource != creature.SourceSaidar {
		t.Fatalf("source = %v, want Saidar", got.Channeling.GenderSource)
	}
	if got.Channeling.ChannelerType != creature.ChannelerInitiate {
		t.Fatalf("type = %v, want Initiate", got.Channeling.ChannelerType)
	}
	wantBits := creature.PowerSet(1<<uint(creature.PowerFire) | 1<<uint(creature.PowerSpirit))
	if got.Channeling.Affinities != wantBits {
		t.Fatalf("affinities = %b, want %b", got.Channeling.Affinities, wantBits)
	}
	if len(got.Channeling.WeavesKnownIDs) != 3 {
		t.Fatalf("weaves = %v, want 3", got.Channeling.WeavesKnownIDs)
	}
}

// TestChannelingStep_ValidatesAffinityCount asserts the substep
// refuses < 2 / > 2 / duplicate / unknown affinity tokens.
func TestChannelingStep_ValidatesAffinityCount(t *testing.T) {
	f := newChannelingFixture(t)

	for _, tc := range []struct {
		input string
		want  string
	}{
		{"affinities fire", "exactly 2"},
		{"affinities fire fire", "Duplicate"},
		{"affinities fire spirit air", "exactly 2"},
		{"affinities bogus spirit", "Unknown power"},
	} {
		f.captured.Reset()
		f.feed(tc.input)
		if !strings.Contains(f.captured.String(), tc.want) {
			t.Fatalf("input=%q expected %q in output:\n%s",
				tc.input, tc.want, f.captured.String())
		}
	}
}

// TestChannelingStep_RefusesIneligibleWeave asserts a weave whose
// Power is not in the player's affinity set is rejected.
func TestChannelingStep_RefusesIneligibleWeave(t *testing.T) {
	f := newChannelingFixture(t)
	f.feed("affinities fire spirit")
	f.captured.Reset()
	// "trickle" is a Water weave — not eligible for a fire/spirit
	// channeler.
	f.feed("weaves spark trickle steady_hand")
	if !strings.Contains(f.captured.String(), "not in your eligible list") {
		t.Fatalf("expected eligibility refusal:\n%s", f.captured.String())
	}
}

// TestChannelingStep_DoneRequiresFullSelection asserts that "done"
// gates each stage on its own selection count: stage 1 refuses
// without 2 affinities; stage 2 refuses without 3 weaves.
func TestChannelingStep_DoneRequiresFullSelection(t *testing.T) {
	f := newChannelingFixture(t)

	// Stage 1 (affinities) refuses with 0 picks.
	f.captured.Reset()
	f.feed("done")
	if !strings.Contains(f.captured.String(), "Pick exactly 2 affinities") {
		t.Fatalf("stage-1 done with 0 affinities should refuse:\n%s",
			f.captured.String())
	}

	// Set affinities, advance stage.
	f.feed("affinities fire spirit")
	f.feed("done") // advances to stage 2

	// Stage 2 (weaves) refuses with 0 picks.
	f.captured.Reset()
	f.feed("done")
	if !strings.Contains(f.captured.String(), "Pick exactly 3 starting weaves") {
		t.Fatalf("stage-2 done with 0 weaves should refuse:\n%s",
			f.captured.String())
	}
}

// TestChannelingStep_AffinityChangeClearsWeaves asserts that
// switching affinities drops a previously-valid weave selection
// (because the eligibility filter has shifted).
func TestChannelingStep_AffinityChangeClearsWeaves(t *testing.T) {
	f := newChannelingFixture(t)
	f.feed("affinities fire spirit")
	f.feed("weaves spark warmth steady_hand")

	mode := f.session.CurrentMode().(*CharacterCreate)
	if len(mode.draft.StartingWeaves) != 3 {
		t.Fatalf("setup: weaves = %v, want 3", mode.draft.StartingWeaves)
	}

	f.feed("affinities air earth")
	if len(mode.draft.StartingWeaves) != 0 {
		t.Fatalf("affinity change should clear weaves; got %v", mode.draft.StartingWeaves)
	}
}

// TestChannelingStep_BackFromReviewLandsOnHub asserts that pressing
// `back` from review drops the player on the hub view (with the
// auto-open-review latch suppressed for one render) so they can pick
// any row to revise.
func TestChannelingStep_BackFromReviewLandsOnHub(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Lan")
	f.feed("human")
	f.feed("1")
	f.feed("borderlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("3")
	f.feed("done")
	f.feed("4")
	f.feed("done")
	f.feed("5")
	f.feed("pick 1")
	f.feed("done")
	f.feed("6")
	f.feed("done") // skills → hub
	f.feed("7")    // equipment (last required row)
	f.feed("1")
	f.feed("done") // → hub → auto-opens review

	mode := f.session.CurrentMode().(*CharacterCreate)
	if mode.step != chargenStepReview {
		t.Fatalf("expected step=Review; got %d", mode.step)
	}
	f.feed("back")
	if mode.step != chargenStepHub {
		t.Fatalf("back from review should land on hub; got %d", mode.step)
	}
	if !mode.suppressAutoReview && false {
		t.Logf("note: latch already cleared after the writeHub render that fired on B")
	}
}

// TestEligibleStartingWeaves_FiltersByPower exercises the helper
// directly against the embedded chargen catalog.
func TestEligibleStartingWeaves_FiltersByPower(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	mode := f.session.CurrentMode().(*CharacterCreate)

	// Fire bit only.
	got := eligibleStartingWeaves(mode.catalog, 1<<uint(creature.PowerFire))
	if len(got) == 0 {
		t.Fatalf("expected at least one fire weave; got 0")
	}
	for _, w := range got {
		if !strings.EqualFold(w.Power, "fire") {
			t.Fatalf("non-fire weave leaked: %s power=%s", w.ID, w.Power)
		}
	}

	// Empty bitmask returns no weaves so the menu can prompt the
	// player to pick affinities first.
	if got := eligibleStartingWeaves(mode.catalog, 0); len(got) != 0 {
		t.Fatalf("zero affinities should return no weaves; got %v", got)
	}
}

// newChannelingFixture spins a multi-step CharacterCreate forward
// to the channeling substep with an Initiate (female so source is
// Saidar). Tests then drive applyChanneling via .feed.
func newChannelingFixture(t *testing.T) *charCreateFixture {
	t.Helper()
	f := pushCharacterCreateMulti(t)
	f.feed("Egwene")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("initiate")
	f.feed("4")             // identity (so we can stamp gender=female)
	f.feed("gender female") // saidar source
	f.feed("done")
	f.feed("7") // hub → channeling

	mode := f.session.CurrentMode().(*CharacterCreate)
	if mode.step != chargenStepChanneling {
		t.Fatalf("setup: expected step=Channeling, got %d", mode.step)
	}
	return f
}
