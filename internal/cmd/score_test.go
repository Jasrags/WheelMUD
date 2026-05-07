package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// scoreSeedChar inserts a fully-populated character through the
// memory repo so FindByName resolves it. Returns the inserted row.
func scoreSeedChar(t *testing.T, chars repo.CharacterRepo) repo.Character {
	t.Helper()
	core := creature.Core{
		HPCurrent: 9,
		HPMax:     12,
		Defense:   14,
		Saves:     creature.Saves{Fort: 2, Ref: 1, Will: 0},
		Speed:     creature.Speed{BaseFt: 30},
		Gender:    creature.GenderFemale,
		Alignment: creature.PostureGood,
	}
	core.Abilities = creature.Abilities{
		Str: creature.AbilityScore{Current: 14, Max: 14},
		Dex: creature.AbilityScore{Current: 12, Max: 12},
		Con: creature.AbilityScore{Current: 10, Max: 10},
		Int: creature.AbilityScore{Current: 13, Max: 13},
		Wis: creature.AbilityScore{Current: 11, Max: 11},
		Cha: creature.AbilityScore{Current: 9, Max: 9},
	}
	in := repo.Character{
		AccountID:   100,
		Name:        "Lan",
		Race:        creature.RaceHuman,
		Background:  creature.BackgroundBorderlander,
		ClassLevels: map[creature.Class]int8{creature.ClassArmsman: 3},
		Core:        core,
		HeightCm:    188,
		WeightKg:    91,
		Age:         24,
		Handedness:  creature.HandRight,
		Coin:        currency.Amount(523), // 5 silver 23 copper
		BankBalance: currency.Amount(10000),
		XP:          250,
		AuthLevel:   repo.AuthLevelPlayer,
	}
	out, err := chars.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	return out
}

func TestScore_RendersIdentityVitalsAbilitiesWealth(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	scoreSeedChar(t, chars)

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Lan"
	s.Width = 80

	cmd := NewScore(chars, nil)
	runCmd(t, cmd, s, "")

	got := out.String()
	wants := []string{
		"Character — Lan",
		"Identity",
		"Race:",
		"human",
		"Class:",
		"Armsman",
		"lvl 3",
		"Background:",
		"Borderlander",
		"Gender:",
		"female",
		"Age:",
		"24",
		"Height:",
		"6 ft 2 in (188 cm)",
		"Weight:",
		"kg",
		"Handed:",
		"right",
		"Alignment:",
		"good",
		"Vitals",
		"Hit points:",
		"9 / 12",
		"Defense:",
		"14",
		"Saves:",
		"Fort +2",
		"Ref +1",
		"Will +0",
		"Speed:",
		"30 ft",
		"Abilities",
		"STR",
		"14",
		"(+2)",
		"DEX",
		"12",
		"(+1)",
		"Wealth",
		"Carried:",
		"Bank:",
		"XP:",
		"250",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Fatalf("missing %q in score output:\n%s", w, got)
		}
	}
}

func TestScore_LoadFailureWritesError(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Ghost"
	s.Width = 80

	cmd := NewScore(chars, nil)
	runCmd(t, cmd, s, "")

	if !strings.Contains(out.String(), "Could not load") {
		t.Fatalf("expected load-failure message, got %q", out.String())
	}
}

func TestScore_HonorsColorLevelNone(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	scoreSeedChar(t, chars)

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Lan"
	s.Width = 80
	s.ColorLevel = telnet.ColorLevelNone

	runCmd(t, NewScore(chars, nil), s, "")

	if strings.ContainsRune(out.String(), 0x1b) {
		t.Fatalf("ColorLevelNone leaked SGR:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Lan") {
		t.Fatalf("payload missing on ColorLevelNone:\n%s", out.String())
	}
}

func TestScoreAbilityMod_Floor(t *testing.T) {
	tests := []struct {
		score, mod int
	}{
		{1, -5}, {2, -4}, {3, -4}, {8, -1}, {9, -1},
		{10, 0}, {11, 0}, {12, 1}, {18, 4}, {20, 5},
	}
	for _, tc := range tests {
		if got := scoreAbilityMod(tc.score); got != tc.mod {
			t.Errorf("scoreAbilityMod(%d) = %d, want %d", tc.score, got, tc.mod)
		}
	}
}

func TestClassLine_MultiClassOrderingByLevelDesc(t *testing.T) {
	got := classLine(map[creature.Class]int8{
		creature.ClassArmsman:  2,
		creature.ClassWoodsman: 5,
	}, nil)
	if !strings.HasPrefix(got, "Woodsman lvl 5") {
		t.Fatalf("expected highest level first, got %q", got)
	}
	if !strings.Contains(got, "Armsman lvl 2") {
		t.Fatalf("missing lower-level class, got %q", got)
	}
}

func TestClassLine_EmptyOrZeroLevels(t *testing.T) {
	if got := classLine(nil, nil); got != "(unset)" {
		t.Errorf("nil levels: got %q want (unset)", got)
	}
	if got := classLine(map[creature.Class]int8{creature.ClassArmsman: 0}, nil); got != "(unset)" {
		t.Errorf("zero levels: got %q want (unset)", got)
	}
}
