package mode

import (
	"strings"
	"testing"
)

// abilitiesFixture spins a fresh draft up to the abilities substep
// for the slice-3 shorthand tests.
func abilitiesFixture(t *testing.T) *charCreateFixture {
	t.Helper()
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("human")
	f.feed("1")
	f.feed("midlander")
	f.feed("2")
	f.feed("armsman")
	f.feed("3") // hub → abilities
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepAbilities {
		t.Fatalf("abilitiesFixture: step = %d, want chargenStepAbilities", mc.step)
	}
	return f
}

// TestChargenAbilities_PlusMinusBumpsScore exercises the slice-3
// shorthand: `<n>+` increments row n by 1, `<n>-` decrements.
func TestChargenAbilities_PlusMinusBumpsScore(t *testing.T) {
	f := abilitiesFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)

	// Row 1 = Strength, floor 8.
	f.feed("1+")
	if mc.draft.Abilities[0] != 9 {
		t.Fatalf("after 1+, Str = %d, want 9", mc.draft.Abilities[0])
	}
	f.feed("1 +")
	f.feed("1 +")
	if mc.draft.Abilities[0] != 11 {
		t.Fatalf("after three +s, Str = %d, want 11", mc.draft.Abilities[0])
	}
	f.feed("1-")
	if mc.draft.Abilities[0] != 10 {
		t.Fatalf("after 1-, Str = %d, want 10", mc.draft.Abilities[0])
	}
}

// TestChargenAbilities_FloorRefusal asserts a `-` bump at the
// point-buy floor surfaces a focused error rather than mutating
// state.
func TestChargenAbilities_FloorRefusal(t *testing.T) {
	f := abilitiesFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.captured.Reset()
	f.feed("2-")
	if !strings.Contains(f.captured.String(), "floor") {
		t.Fatalf("expected floor refusal:\n%s", f.captured.String())
	}
	if mc.draft.Abilities[1] != pointBuyMinScore {
		t.Fatalf("floor bump leaked: Dex = %d", mc.draft.Abilities[1])
	}
}

// TestChargenAbilities_OverBudgetRollsBack asserts a `+` that would
// push the spent total past the budget refuses without committing.
func TestChargenAbilities_OverBudgetRollsBack(t *testing.T) {
	f := abilitiesFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	// Spend exactly to budget via the legacy set verb so we can
	// exercise the bump-rollback path independently.
	f.feed("set str 16") // cost 10
	f.feed("set dex 14") // +6 → 16
	f.feed("set con 14") // +6 → 22
	f.feed("set int 11") // +3 → 25 (budget)
	if mc.pointBuySpent() != pointBuyBudget {
		t.Fatalf("setup: spent = %d, want %d",
			mc.pointBuySpent(), pointBuyBudget)
	}
	f.captured.Reset()
	f.feed("5+") // would bump Wis 8→9, costing 1 — over budget
	if !strings.Contains(f.captured.String(), "Not enough points") {
		t.Fatalf("expected over-budget refusal:\n%s", f.captured.String())
	}
	if mc.draft.Abilities[4] != pointBuyMinScore {
		t.Fatalf("over-budget bump leaked: Wis = %d", mc.draft.Abilities[4])
	}
}

// TestChargenAbilities_ResetAndDoneShorthand asserts the slice-3
// `r` and `d` shorthand match `reset` and `done`.
func TestChargenAbilities_ResetAndDoneShorthand(t *testing.T) {
	f := abilitiesFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.feed("1+")
	f.feed("1+")
	if mc.draft.Abilities[0] == pointBuyMinScore {
		t.Fatalf("setup bumps did not stick: Str = %d", mc.draft.Abilities[0])
	}
	f.feed("r")
	if mc.draft.Abilities[0] != pointBuyMinScore {
		t.Fatalf("r should reset; Str = %d", mc.draft.Abilities[0])
	}
	f.feed("d")
	if mc.step != chargenStepHub {
		t.Fatalf("d should return to hub; step = %d", mc.step)
	}
}

// TestChargenAbilities_BadRowNumber asserts a row token outside
// 1..6 surfaces an error and leaves state alone.
func TestChargenAbilities_BadRowNumber(t *testing.T) {
	f := abilitiesFixture(t)
	mc := f.session.CurrentMode().(*CharacterCreate)
	f.captured.Reset()
	f.feed("9+")
	if !strings.Contains(f.captured.String(), "1..6") {
		t.Fatalf("expected row-range hint:\n%s", f.captured.String())
	}
	for i, sc := range mc.draft.Abilities {
		if sc != pointBuyMinScore {
			t.Fatalf("Abilities[%d] = %d, want %d (no mutation on bad row)",
				i, sc, pointBuyMinScore)
		}
	}
}
