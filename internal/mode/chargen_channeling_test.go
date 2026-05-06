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
	f.feed("borderlander")
	f.feed("armsman")
	f.feed("done")   // abilities (defaults)
	f.feed("done")   // identity
	f.feed("pick 1") // first eligible feat
	f.feed("done")   // feat → skills
	f.feed("done")   // skills → review (skipped channeling)
	f.feed("yes")    // commit

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
	f.feed("midlander")
	f.feed("initiate")
	f.feed("done")              // abilities
	f.feed("gender female")     // override default male so source=saidar
	f.feed("done")              // identity
	f.feed("pick 1")            // feat
	f.feed("done")              // feat → skills
	f.feed("done")              // skills → channeling (channeler branch)
	f.feed("affinities fire spirit")
	f.feed("weaves spark warmth steady_hand")
	f.feed("done")  // channeling → review
	f.feed("yes")   // commit

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

// TestChannelingStep_DoneRequiresFullSelection asserts "done"
// refuses to advance until both 2 affinities and 3 weaves are set.
func TestChannelingStep_DoneRequiresFullSelection(t *testing.T) {
	f := newChannelingFixture(t)

	f.captured.Reset()
	f.feed("done")
	if !strings.Contains(f.captured.String(), "Pick exactly 2 affinities") {
		t.Fatalf("done with 0 affinities should refuse:\n%s", f.captured.String())
	}

	f.feed("affinities fire spirit")
	f.captured.Reset()
	f.feed("done")
	if !strings.Contains(f.captured.String(), "Pick exactly 3 starting weaves") {
		t.Fatalf("done with 0 weaves should refuse:\n%s", f.captured.String())
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

// TestChannelingStep_BackFromReviewSkipsChannelingForNonChanneler
// asserts the back handler honors the conditional substep — pressing
// `back` from review for an armsman returns to skills (not the
// channeling step the armsman never visited).
func TestChannelingStep_BackFromReviewSkipsChannelingForNonChanneler(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Lan")
	f.feed("human")
	f.feed("borderlander")
	f.feed("armsman")
	f.feed("done")   // abilities
	f.feed("done")   // identity
	f.feed("pick 1") // feat
	f.feed("done")   // feat → skills
	f.feed("done")   // skills → review (skipped channeling)

	mode := f.session.CurrentMode().(*CharacterCreate)
	if mode.step != chargenStepReview {
		t.Fatalf("expected step=Review; got %d", mode.step)
	}
	f.feed("back")
	if mode.step != chargenStepSkills {
		t.Fatalf("back from review (non-channeler) should reach Skills; got %d", mode.step)
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
	f.feed("midlander")
	f.feed("initiate")
	f.feed("done")          // abilities
	f.feed("gender female") // override default; saidar source
	f.feed("done")          // identity
	f.feed("pick 1")        // feat
	f.feed("done")          // feat → skills
	f.feed("done")          // skills → channeling

	mode := f.session.CurrentMode().(*CharacterCreate)
	if mode.step != chargenStepChanneling {
		t.Fatalf("setup: expected step=Channeling, got %d", mode.step)
	}
	return f
}
