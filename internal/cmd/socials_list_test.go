package cmd

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/emote"
)

func TestSocialsList_RendersBothGroups(t *testing.T) {
	_, alice, _, aOut, _ := commPair(t)
	cat := loadTestSocials(t)

	runCmd(t, NewSocialsList(cat), alice, "")

	got := aOut.String()
	// Section header.
	if !strings.Contains(got, "Socials") {
		t.Fatalf("missing section header; got %q", got)
	}
	// Both subsections present.
	if !strings.Contains(got, "Targetable") {
		t.Fatalf("missing Targetable subsection; got %q", got)
	}
	if !strings.Contains(got, "Untargeted-only") {
		t.Fatalf("missing Untargeted-only subsection; got %q", got)
	}
	// Targetable social listed.
	if !strings.Contains(got, "smile") {
		t.Fatalf("missing smile row; got %q", got)
	}
	// Untargeted social listed.
	if !strings.Contains(got, "sigh") {
		t.Fatalf("missing sigh row; got %q", got)
	}
	// Help text from the fixture is rendered.
	if !strings.Contains(got, "small, warm smile") {
		t.Fatalf("missing smile help text; got %q", got)
	}
	// Footer pointer.
	if !strings.Contains(got, "help <verb>") {
		t.Fatalf("missing help pointer footer; got %q", got)
	}
}

func TestSocialsList_EmptyCatalog(t *testing.T) {
	_, alice, _, aOut, _ := commPair(t)
	// Load an empty YAML to get a non-nil but zero-entry catalog,
	// mirroring the "EMOTE_DIR points at an empty dir" boot case.
	cat, err := emote.Load(fstest.MapFS{
		"empty.yaml": &fstest.MapFile{Data: []byte("socials: []\n")},
	})
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}

	runCmd(t, NewSocialsList(cat), alice, "")

	if !strings.Contains(aOut.String(), "No socials configured") {
		t.Fatalf("expected empty-state line; got %q", aOut.String())
	}
}

func TestSocialsList_NilCatalog(t *testing.T) {
	_, alice, _, aOut, _ := commPair(t)
	runCmd(t, NewSocialsList(nil), alice, "")
	if !strings.Contains(aOut.String(), "No socials configured") {
		t.Fatalf("nil catalog must yield empty-state line; got %q", aOut.String())
	}
}

func TestSocialsList_DefangsCfmtHelp(t *testing.T) {
	// A hostile Help string with cfmt control tokens must not be
	// able to close the magenta wrapper or inject a competing
	// style. The defang sweep is the only guarantee — pin it here
	// so a future refactor that bypasses display.Defang regresses
	// the test rather than the security property.
	_, alice, _, aOut, _ := commPair(t)
	cat, err := emote.Load(fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: trick
    help: "{{evil}}::red"
    self: You trick.
    other: "{actor} tricks."
`)},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	runCmd(t, NewSocialsList(cat), alice, "")

	if strings.Contains(aOut.String(), "::red") {
		t.Fatalf("cfmt help-text injection not defused: %q", aOut.String())
	}
}

func TestSocialsList_AlphabeticalWithinGroup(t *testing.T) {
	// Load-order is `zap` then `aim`; rendered output must list
	// `aim` first within the untargeted group.
	_, alice, _, aOut, _ := commPair(t)
	cat, err := emote.Load(fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: zap
    self: You zap.
    other: "{actor} zaps."
  - id: aim
    self: You aim.
    other: "{actor} aims."
`)},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	runCmd(t, NewSocialsList(cat), alice, "")

	// Anchor each row at the magenta-wrapped verb glyph rather than a
	// raw substring, so a future social whose name embeds "aim" or
	// "zap" (e.g. "claim", "zapper") can't fool the assertion.
	got := aOut.String()
	aimMarker := "\x1b[35maim"
	zapMarker := "\x1b[35mzap"
	aimAt := strings.Index(got, aimMarker)
	zapAt := strings.Index(got, zapMarker)
	if aimAt < 0 || zapAt < 0 {
		t.Fatalf("missing one of the rows: aim=%d zap=%d in %q", aimAt, zapAt, got)
	}
	if aimAt > zapAt {
		t.Fatalf("expected alphabetical order (aim before zap); got %q", got)
	}
}
