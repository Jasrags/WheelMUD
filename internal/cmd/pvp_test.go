package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// pvpFixture seeds a memory CharacterRepo with one row matching the
// commPair Alice session and returns the repo + verb. Returned ID
// matches s.CharacterID = 1 from commPair.
func pvpFixture(t *testing.T) *repo.MemoryCharacterRepo {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID: 100,
		Name:      "Alice",
	}); err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	return chars
}

func TestPvP_BareEchoesOff(t *testing.T) {
	chars := pvpFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	runCmd(t, NewPvP(chars), alice, "")
	if !strings.Contains(aOut.String(), "PvP: off") {
		t.Fatalf("missing default-off echo: %q", aOut.String())
	}
}

func TestPvP_OnPersists(t *testing.T) {
	chars := pvpFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	c := NewPvP(chars)

	runCmd(t, c, alice, "on")
	if !strings.Contains(aOut.String(), "PvP: on") {
		t.Fatalf("missing on confirm: %q", aOut.String())
	}
	got, err := chars.GetByID(context.Background(), alice.CharacterID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.PvP {
		t.Fatal("PvP flag not persisted")
	}

	aOut.Reset()
	runCmd(t, c, alice, "")
	if !strings.Contains(aOut.String(), "PvP: on") {
		t.Fatalf("re-read after toggle: %q", aOut.String())
	}
}

func TestPvP_OffFlipsBack(t *testing.T) {
	chars := pvpFixture(t)
	_, alice, _, _, _ := commPair(t)
	c := NewPvP(chars)

	runCmd(t, c, alice, "on")
	runCmd(t, c, alice, "off")
	got, _ := chars.GetByID(context.Background(), alice.CharacterID)
	if got.PvP {
		t.Fatal("PvP should be off")
	}
}

func TestPvP_BadArgRefuses(t *testing.T) {
	chars := pvpFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	runCmd(t, NewPvP(chars), alice, "wibble")
	if !strings.Contains(aOut.String(), "Usage: pvp") {
		t.Fatalf("missing usage refusal: %q", aOut.String())
	}
	got, _ := chars.GetByID(context.Background(), alice.CharacterID)
	if got.PvP {
		t.Fatal("bad arg must not flip the flag")
	}
}

func TestPvP_AcceptsSynonyms(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"on", true}, {"off", false},
		{"1", true}, {"0", false},
		{"true", true}, {"false", false},
		{"YES", true}, {"NO", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := parsePvPArg(tc.in)
			if !ok {
				t.Fatalf("parsePvPArg(%q): rejected", tc.in)
			}
			if got != tc.want {
				t.Fatalf("parsePvPArg(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
	if _, ok := parsePvPArg("nope"); ok {
		t.Fatal("nope should not parse")
	}
}
