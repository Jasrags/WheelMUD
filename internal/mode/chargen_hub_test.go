package mode

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

// TestChargenHub_RendersAfterRace asserts that finishing the race
// step lands the player on the hub view (not on the legacy linear
// background substep).
func TestChargenHub_RendersAfterRace(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")

	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepHub {
		t.Fatalf("step = %d, want chargenStepHub after race", mc.step)
	}
	out := f.captured.String()
	for _, want := range []string{"Character build", "Background", "Class", "Channeling"} {
		if !strings.Contains(out, want) {
			t.Fatalf("hub render missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, notChosen) {
		t.Fatalf("expected %q on a fresh hub render:\n%s", notChosen, out)
	}
}

// TestChargenHub_ChannelingNAForNonChanneler asserts the hub renders
// channeling as "n/a" without a selectable number when the chosen
// class isn't a channeler.
func TestChargenHub_ChannelingNAForNonChanneler(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.captured.Reset()
	// Re-render the hub (empty input on the hub repaints).
	f.feed("")
	out := f.captured.String()
	if !strings.Contains(out, "n/a") {
		t.Fatalf("expected n/a marker on non-channeler hub:\n%s", out)
	}
	if !strings.Contains(out, "[1-6]") {
		t.Fatalf("expected [1-6] range hint (no row 7) on non-channeler hub:\n%s", out)
	}
}

// TestChargenHub_ChannelingSlotForChanneler asserts the row shifts
// from "n/a" to "— not chosen —" once the player picks a channeler
// class.
func TestChargenHub_ChannelingSlotForChanneler(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("initiate") // channeler
	f.captured.Reset()
	f.feed("")
	out := f.captured.String()
	if strings.Contains(out, "n/a") {
		t.Fatalf("channeler hub should not show n/a:\n%s", out)
	}
	if !strings.Contains(out, "[1-7]") {
		t.Fatalf("expected [1-7] range hint on channeler hub:\n%s", out)
	}
}

// TestChargenHub_NonChannelerCannotEnterChanneling exercises the
// guarded numeric route: row 7 is non-selectable for non-channelers
// and produces a focused error rather than entering the substep.
func TestChargenHub_NonChannelerCannotEnterChanneling(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.captured.Reset()
	f.feed("7")
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepHub {
		t.Fatalf("picking row 7 on a non-channeler hub should not route; step=%d", mc.step)
	}
	if !strings.Contains(f.captured.String(), "[1-6]") {
		t.Fatalf("expected range hint on bad pick:\n%s", f.captured.String())
	}
}

// TestChargenHub_RestartConfirmRoundtrip asserts [R] enters the
// restart confirm, [N] returns to hub with draft intact, and [Y]
// wipes the draft back to the name step.
func TestChargenHub_RestartConfirmRoundtrip(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")

	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("r")
	if mc.step != chargenStepHubConfirm || mc.pendingConfirm != confirmRestart {
		t.Fatalf("restart did not enter confirm: step=%d kind=%d",
			mc.step, mc.pendingConfirm)
	}
	f.feed("n")
	if mc.step != chargenStepHub {
		t.Fatalf("declined restart should return to hub; step=%d", mc.step)
	}
	if mc.draft.BackgroundID != "midlander" {
		t.Fatalf("declined restart wiped draft: %+v", mc.draft)
	}

	f.feed("r")
	f.feed("y")
	if mc.step != chargenStepName {
		t.Fatalf("confirmed restart should reach Name; step=%d", mc.step)
	}
	if mc.draft.Name != "" || mc.draft.BackgroundID != "" {
		t.Fatalf("restart did not clear draft: %+v", mc.draft)
	}
}

// TestChargenHub_QuitConfirmCallsOnCancel verifies that [Q] confirmed
// Y delegates to the wired onCancel hook (the path AccountMenu uses
// to ReplaceMode back into itself).
func TestChargenHub_QuitConfirmCallsOnCancel(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	called := 0
	mc.SetOnCancel(func(s *telnet.Session) error {
		called++
		return nil
	})
	f.feed("Hero")
	f.feed("human")
	f.feed("q")
	if mc.step != chargenStepHubConfirm || mc.pendingConfirm != confirmCancel {
		t.Fatalf("quit did not enter cancel-confirm: step=%d kind=%d",
			mc.step, mc.pendingConfirm)
	}
	f.feed("y")
	if called != 1 {
		t.Fatalf("onCancel called %d times, want 1", called)
	}
}

// TestChargenHub_QuitWithoutOnCancelFallsBackToRestart asserts that
// when no onCancel is wired (post-auth direct-to-chargen path on
// fresh accounts), [Q]/Y collapses to the in-place restart so the
// player isn't stranded.
func TestChargenHub_QuitWithoutOnCancelFallsBackToRestart(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("q")
	f.feed("y")
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepName {
		t.Fatalf("quit fallback should reach Name; step=%d", mc.step)
	}
	if mc.draft.Name != "" {
		t.Fatalf("fallback did not clear draft: %+v", mc.draft)
	}
}

// TestChargenHub_AutoOpensReviewWhenComplete walks the full hub and
// asserts the review screen fires automatically once the last row is
// filled.
func TestChargenHub_AutoOpensReviewWhenComplete(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	walkHubToReview(t, f)
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepReview {
		t.Fatalf("expected auto-open to land on review; step=%d", mc.step)
	}
}

// TestChargenHub_BackFromReviewSuppressesAutoOpenForOneRender asserts
// that pressing [B] on the review screen drops to the hub view (so
// the player can pick a row to revise) instead of immediately
// re-auto-opening review.
func TestChargenHub_BackFromReviewSuppressesAutoOpenForOneRender(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	walkHubToReview(t, f)
	f.captured.Reset()
	f.feed("back")
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepHub {
		t.Fatalf("back from review should reach hub; step=%d", mc.step)
	}
	if !strings.Contains(f.captured.String(), "Character build") {
		t.Fatalf("expected hub header after back-from-review:\n%s",
			f.captured.String())
	}
}

// TestChargenHub_ReviewNumberedJumpBack asserts the review screen
// routes 2..N (offset by one for the "1) Confirm" row) to the
// matching hub row's substep.
func TestChargenHub_ReviewNumberedJumpBack(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	walkHubToReview(t, f)
	// Review row 2 = revise Background (hub row 1).
	f.feed("2")
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepBackground {
		t.Fatalf("review 2 should enter Background substep; step=%d", mc.step)
	}
}

// TestChargenHub_ReviewConfirmFinalises asserts the review screen
// accepts numeric "1" as a confirm (not just "yes"/"y").
func TestChargenHub_ReviewConfirmFinalises(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	walkHubToReview(t, f)
	f.feed("1") // confirm and finalise
	if f.session.CurrentMode() != f.game {
		t.Fatalf("review 1 did not finalise; CurrentMode = %T", f.session.CurrentMode())
	}
}

// TestChargenHub_ChangingClassResetsChanneling asserts that flipping
// from a channeler class back to a non-channeler class clears the
// channeling state so the hub re-renders the row as n/a.
func TestChargenHub_ChangingClassResetsChanneling(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("initiate") // channeler

	mc := f.session.CurrentMode().(*CharacterCreate)
	mc.draft.ChannelingInit = true
	mc.draft.Affinities = 0b11
	mc.draft.StartingWeaves = []string{"a", "b", "c"}

	f.feed("2") // re-enter class
	f.feed("armsman")
	if mc.draft.ChannelingInit || mc.draft.Affinities != 0 || len(mc.draft.StartingWeaves) != 0 {
		t.Fatalf("class flip did not clear channeling state: %+v", mc.draft)
	}
}
