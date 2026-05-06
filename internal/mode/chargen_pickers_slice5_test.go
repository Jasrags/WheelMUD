package mode

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// TestChargenFeat_PickByNumberStillWorks confirms slice 5's render
// polish didn't break the bare-numeric pick path used by every multi-
// step happy-path test.
func TestChargenFeat_PickByNumberStillWorks(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("5") // hub → feat
	f.feed("1") // first feat in list
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.draft.SelectedFeatID == "" {
		t.Fatal("bare-number pick did not set SelectedFeatID")
	}
}

// TestChargenFeat_InfoShowsMetadata asserts the slice-2/B
// feat-info screen renders Type and Available-to rows alongside
// the description. Bullheaded is offered to Aiel and Midlander
// backgrounds, so both names should appear in the available-to
// list.
func TestChargenFeat_InfoShowsMetadata(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("5") // hub → feat
	f.captured.Reset()
	f.feed("info bullheaded")
	out := f.captured.String()
	for _, want := range []string{"Bullheaded", "Type", "Background feat", "Available to", "Aiel", "Midlander"} {
		if !strings.Contains(out, want) {
			t.Fatalf("feat info missing %q:\n%s", want, out)
		}
	}
}

// TestChargenFeat_DoneShorthand asserts `d` is accepted as `done`.
func TestChargenFeat_DoneShorthand(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("5") // feat
	f.feed("1") // pick
	f.feed("d")
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepHub {
		t.Fatalf("`d` should return to hub; step = %d", mc.step)
	}
}

// TestChargenChanneling_NumberedAffinityToggle exercises the slice-5
// numbered checklist: each row toggles on, can be toggled back off,
// and a 3rd pick is refused while two are already set.
func TestChargenChanneling_NumberedAffinityToggle(t *testing.T) {
	f := newChannelingFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.channelingStage != channelingStageAffinities {
		t.Fatalf("setup: stage = %d, want affinities", mc.channelingStage)
	}

	f.feed("3") // toggle Fire on (powers: Air=1, Earth=2, Fire=3)
	if mc.draft.Affinities&(1<<uint(creature.PowerFire)) == 0 {
		t.Fatalf("toggle Fire failed: %05b", mc.draft.Affinities)
	}
	f.feed("5") // toggle Spirit on
	if mc.draft.Affinities&(1<<uint(creature.PowerSpirit)) == 0 {
		t.Fatalf("toggle Spirit failed: %05b", mc.draft.Affinities)
	}

	// 3rd pick refused.
	f.captured.Reset()
	f.feed("1")
	if !strings.Contains(f.captured.String(), "Already at") {
		t.Fatalf("3rd toggle should refuse:\n%s", f.captured.String())
	}

	// Toggle Fire back off.
	f.feed("3")
	if mc.draft.Affinities&(1<<uint(creature.PowerFire)) != 0 {
		t.Fatalf("Fire toggle-off failed: %05b", mc.draft.Affinities)
	}
}

// TestChargenChanneling_DoneAdvancesStage asserts a successful "done"
// from affinities advances to the weaves stage and the next render
// shows the weave checklist.
func TestChargenChanneling_DoneAdvancesStage(t *testing.T) {
	f := newChannelingFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("3") // Fire
	f.feed("5") // Spirit
	f.captured.Reset()
	f.feed("d") // shorthand for done
	if mc.channelingStage != channelingStageWeaves {
		t.Fatalf("stage did not advance: %d", mc.channelingStage)
	}
	if !strings.Contains(f.captured.String(), "Starting weaves") {
		t.Fatalf("expected weaves render after done:\n%s", f.captured.String())
	}
}

// TestChargenChanneling_NumberedWeaveToggle exercises stage-2's
// numbered checklist over the affinity-filtered weave list.
func TestChargenChanneling_NumberedWeaveToggle(t *testing.T) {
	f := newChannelingFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("3")    // Fire
	f.feed("5")    // Spirit
	f.feed("done") // advance to weaves stage

	weaves := eligibleStartingWeaves(mc.catalog, mc.draft.Affinities)
	if len(weaves) < 3 {
		t.Skipf("catalog has %d eligible weaves; need 3 to test toggle", len(weaves))
	}

	f.feed("1")
	f.feed("2")
	f.feed("3")
	if len(mc.draft.StartingWeaves) != 3 {
		t.Fatalf("after 3 toggles, picked = %v", mc.draft.StartingWeaves)
	}

	// 4th pick refused.
	if len(weaves) >= 4 {
		f.captured.Reset()
		f.feed("4")
		if !strings.Contains(f.captured.String(), "Already at 3") {
			t.Fatalf("4th toggle should refuse:\n%s", f.captured.String())
		}
	}

	// Toggle one back off.
	f.feed("1")
	if len(mc.draft.StartingWeaves) != 2 {
		t.Fatalf("after toggle-off, picked = %v", mc.draft.StartingWeaves)
	}
}

// TestChargenChanneling_PrevReturnsToAffinities exercises the [P]
// shorthand in stage 2 — rolls the stage back so the player can
// revise affinities without leaving the substep.
func TestChargenChanneling_PrevReturnsToAffinities(t *testing.T) {
	f := newChannelingFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("3")
	f.feed("5")
	f.feed("done") // → weaves
	if mc.channelingStage != channelingStageWeaves {
		t.Fatalf("setup: stage = %d", mc.channelingStage)
	}
	f.feed("p")
	if mc.channelingStage != channelingStageAffinities {
		t.Fatalf("p should roll back to affinities; stage = %d", mc.channelingStage)
	}
}

// TestChargenChanneling_AffinityToggleClearsBadWeaves asserts that
// untoggling an affinity drops any picked weave whose Power was
// keyed to it (mirrors the verb-form contract).
func TestChargenChanneling_AffinityToggleClearsBadWeaves(t *testing.T) {
	f := newChannelingFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("3") // Fire
	f.feed("5") // Spirit
	f.feed("done")
	weaves := eligibleStartingWeaves(mc.catalog, mc.draft.Affinities)
	if len(weaves) < 1 {
		t.Skip("catalog has no eligible weaves to test filtering")
	}
	f.feed("1") // pick first
	if len(mc.draft.StartingWeaves) != 1 {
		t.Fatalf("setup: picked = %v", mc.draft.StartingWeaves)
	}

	// Roll back, untoggle Fire (3) — picked Fire weaves should drop.
	f.feed("p")
	first := mc.draft.StartingWeaves[0]
	w, _ := mc.catalog.Weave(first)
	if w == nil {
		t.Fatalf("test fixture broken: weave %q missing", first)
	}
	if !strings.EqualFold(w.Power, "Fire") {
		t.Skipf("first eligible weave %q is %s, not Fire — can't exercise drop",
			first, w.Power)
	}
	f.feed("3") // toggle Fire off
	for _, id := range mc.draft.StartingWeaves {
		if id == first {
			t.Fatalf("Fire weave %q survived affinity drop: %v",
				first, mc.draft.StartingWeaves)
		}
	}
}

// TestChargenChanneling_FullPickerHappyPath drives the full numbered
// flow end-to-end: affinity toggles, advance, weave toggles, exit.
func TestChargenChanneling_FullPickerHappyPath(t *testing.T) {
	f := newChannelingFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("3") // Fire
	f.feed("5") // Spirit
	f.feed("done")
	weaves := eligibleStartingWeaves(mc.catalog, mc.draft.Affinities)
	if len(weaves) < 3 {
		t.Skipf("need 3 eligible weaves; catalog has %d", len(weaves))
	}
	f.feed("1")
	f.feed("2")
	f.feed("3")
	f.feed("done")
	if mc.step != chargenStepHub {
		t.Fatalf("done from full weaves should exit to hub; step = %d", mc.step)
	}
}
