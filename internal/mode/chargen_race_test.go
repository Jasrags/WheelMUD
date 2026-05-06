package mode

import (
	"strings"
	"testing"
)

// TestChargenRace_NumberedPick covers the slice-6 numbered race
// picker: `1` selects Human, `2` selects Ogier, and the next
// rendered screen is the build hub.
func TestChargenRace_NumberedPick(t *testing.T) {
	for _, tc := range []struct {
		input    string
		wantRace string
	}{
		{"1", "human"},
		{"2", "ogier"},
	} {
		f := pushCharacterCreateMulti(t)
		f.feed("Hero")
		f.feed(tc.input)
		mc := f.session.CurrentMode().(*CharacterCreate)
		if mc.draft.Race != tc.wantRace {
			t.Fatalf("input=%q race = %q, want %q",
				tc.input, mc.draft.Race, tc.wantRace)
		}
		if mc.step != chargenStepHub {
			t.Fatalf("input=%q step = %d, want chargenStepHub",
				tc.input, mc.step)
		}
	}
}

// TestChargenRace_BareTokenStillWorks asserts the legacy
// `human` / `ogier` text input keeps working alongside the numbered
// picker (existing test suites and muscle memory).
func TestChargenRace_BareTokenStillWorks(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.feed("ogier")
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.draft.Race != "ogier" {
		t.Fatalf("race = %q, want ogier", mc.draft.Race)
	}
}

// TestChargenRace_RenderShowsBothRows asserts the picker render is
// emitted on entry, with both numbered options visible.
func TestChargenRace_RenderShowsBothRows(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	out := f.captured.String()
	for _, want := range []string{"Race", "Human", "Ogier", "[B]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("race menu missing %q:\n%s", want, out)
		}
	}
}

// TestChargenRace_BadPickStays asserts an unrecognised token leaves
// the player on the race step with a helpful error.
func TestChargenRace_BadPickStays(t *testing.T) {
	f := pushCharacterCreateMulti(t)
	f.feed("Hero")
	f.captured.Reset()
	f.feed("dragon")
	mc := f.session.CurrentMode().(*CharacterCreate)
	if mc.step != chargenStepRace {
		t.Fatalf("bad pick advanced: step = %d", mc.step)
	}
	if !strings.Contains(f.captured.String(), "human") {
		t.Fatalf("expected race hint:\n%s", f.captured.String())
	}
}
