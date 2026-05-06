package mode

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// identityFixture spins a draft up to the identity substep with a
// deterministic RNG so the slice-4 sub-hub tests can assert exact
// height/weight values.
func identityFixture(t *testing.T) *charCreateFixture {
	t.Helper()
	f := pushCharacterCreateMulti(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	mc.SetRNG(rand.New(rand.NewSource(42)))
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("3") // abilities
	f.feed("done")
	f.feed("4") // hub → identity
	if mc.step != chargenStepIdentity {
		t.Fatalf("identityFixture: step = %d, want chargenStepIdentity", mc.step)
	}
	return f
}

// TestChargenIdentity_NumberedFlowGender exercises picking row 1
// (Gender), confirming the prompt is rendered, the next line lands on
// the gender parser, and the menu re-renders cleanly afterwards.
func TestChargenIdentity_NumberedFlowGender(t *testing.T) {
	f := identityFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.captured.Reset()
	f.feed("1")
	if mc.pendingIdentityField != identityFieldGender {
		t.Fatalf("after picking 1, pendingIdentityField = %d, want gender",
			mc.pendingIdentityField)
	}
	if !strings.Contains(f.captured.String(), "Gender") {
		t.Fatalf("expected gender prompt:\n%s", f.captured.String())
	}
	f.feed("f")
	if mc.draft.Gender != creature.GenderFemale {
		t.Fatalf("Gender = %v, want Female", mc.draft.Gender)
	}
	if mc.pendingIdentityField != identityFieldNone {
		t.Fatalf("pending latch should clear after value: %d",
			mc.pendingIdentityField)
	}
}

// TestChargenIdentity_NumberedFlowAge exercises picking row 2 (Age)
// and verifies that a bad value re-prompts in place — the latch is
// retained until a value validates so the player isn't dumped back
// to the menu after a typo.
func TestChargenIdentity_NumberedFlowAge(t *testing.T) {
	f := identityFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("2")
	f.captured.Reset()
	f.feed("zero")
	if !strings.Contains(f.captured.String(), "Age must be") {
		t.Fatalf("expected age refusal:\n%s", f.captured.String())
	}
	if mc.pendingIdentityField != identityFieldAge {
		t.Fatalf("bad value should keep latch; pending = %d",
			mc.pendingIdentityField)
	}
	// Next line is parsed as the retried value, not as a menu pick.
	f.feed("25")
	if mc.draft.Age != 25 {
		t.Fatalf("Age = %d, want 25", mc.draft.Age)
	}
	if mc.pendingIdentityField != identityFieldNone {
		t.Fatalf("successful value should clear latch; pending = %d",
			mc.pendingIdentityField)
	}
}

// TestChargenIdentity_HeightWeightRowReRolls exercises picking row 3,
// which should re-roll directly without prompting for a follow-up.
func TestChargenIdentity_HeightWeightRowReRolls(t *testing.T) {
	f := identityFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	h0, w0 := mc.draft.HeightCm, mc.draft.WeightKg
	f.feed("3")
	if mc.pendingIdentityField != identityFieldNone {
		t.Fatalf("row 3 should not set a pending field: %d",
			mc.pendingIdentityField)
	}
	if mc.draft.HeightCm == h0 && mc.draft.WeightKg == w0 {
		t.Fatalf("row 3 did not re-roll: h=%d w=%d", h0, w0)
	}
}

// TestChargenIdentity_NumberedFlowHanded exercises row 4.
func TestChargenIdentity_NumberedFlowHanded(t *testing.T) {
	f := identityFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("4")
	f.feed("l")
	if mc.draft.Handedness != creature.HandLeft {
		t.Fatalf("Handedness = %v, want Left", mc.draft.Handedness)
	}
}

// TestChargenIdentity_NumberedFlowAlignment exercises row 5.
func TestChargenIdentity_NumberedFlowAlignment(t *testing.T) {
	f := identityFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("5")
	f.feed("evil")
	if mc.draft.Alignment != creature.PostureEvil {
		t.Fatalf("Alignment = %v, want Evil", mc.draft.Alignment)
	}
}

// TestChargenIdentity_RAndDShorthand asserts the slice-4 [R] / [D]
// shorthand match `roll` / `done`.
func TestChargenIdentity_RAndDShorthand(t *testing.T) {
	f := identityFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	h0, w0 := mc.draft.HeightCm, mc.draft.WeightKg
	f.feed("r")
	if mc.draft.HeightCm == h0 && mc.draft.WeightKg == w0 {
		t.Fatalf("r did not re-roll: h=%d w=%d", h0, w0)
	}
	f.feed("d")
	if mc.step != chargenStepHub {
		t.Fatalf("d should return to hub; step = %d", mc.step)
	}
}

// TestChargenIdentity_VerbFormsStillWork asserts the legacy
// `gender m` / `age 25` / `handed left` / `align bad` / `roll`
// verbs still parse at the top level (not inside a pending prompt).
func TestChargenIdentity_VerbFormsStillWork(t *testing.T) {
	f := identityFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
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
}

// TestChargenIdentity_PendingClearedOnReentry asserts re-entering
// identity from the hub doesn't strand a stale pending-field latch.
func TestChargenIdentity_PendingClearedOnReentry(t *testing.T) {
	f := identityFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	// Pick a field, then back out without supplying a value.
	f.feed("1")
	if mc.pendingIdentityField != identityFieldGender {
		t.Fatalf("setup: pending = %d", mc.pendingIdentityField)
	}
	f.feed("back")
	if mc.step != chargenStepHub {
		t.Fatalf("back should land on hub: step = %d", mc.step)
	}
	// Re-enter identity; latch must reset so the next line is parsed
	// as a menu choice rather than as a gender value.
	f.feed("4")
	if mc.pendingIdentityField != identityFieldNone {
		t.Fatalf("re-entry should clear pending; got %d", mc.pendingIdentityField)
	}
}
