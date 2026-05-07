package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/progression"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

func xpSeedChar(t *testing.T, chars repo.CharacterRepo, name string, xp int64, classLvl int8) repo.Character {
	t.Helper()
	in := repo.Character{
		AccountID:   100,
		Name:        name,
		Race:        creature.RaceHuman,
		Background:  creature.BackgroundBorderlander,
		ClassLevels: map[creature.Class]int8{creature.ClassArmsman: classLvl},
		XP:          xp,
		AuthLevel:   repo.AuthLevelPlayer,
	}
	out, err := chars.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	return out
}

func TestXP_BrandNewCharacter(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	xpSeedChar(t, chars, "Rand", 0, 1)

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Rand"
	s.Width = 80

	cmd := NewXP(chars)
	runCmd(t, cmd, s, "")

	got := out.String()
	for _, want := range []string{
		"Experience — Rand",
		"Level:",
		"Class total:",
		"XP:",
		"Next at:",
		"1000",
		"To next:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
	if strings.Contains(got, "level-up") {
		t.Errorf("brand-new char should not show level-up line:\n%s", got)
	}
}

func TestXP_MidLevel(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	// XP 1500 → level 2; classLvl 2 → no pending.
	xpSeedChar(t, chars, "Mat", 1500, 2)

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Mat"
	s.Width = 80

	runCmd(t, NewXP(chars), s, "")

	got := out.String()
	for _, want := range []string{"1500", "3000", "Level:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "level-up") {
		t.Errorf("matched class total should not show level-up line:\n%s", got)
	}
}

func TestXP_PendingLevelUp(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	// XP 1500 → level 2; classLvl 1 → 1 pending.
	xpSeedChar(t, chars, "Perrin", 1500, 1)

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Perrin"
	s.Width = 80

	runCmd(t, NewXP(chars), s, "")

	got := out.String()
	if !strings.Contains(got, "1 level-up available") {
		t.Errorf("expected pending '1 level-up available' line:\n%s", got)
	}
	if !strings.Contains(got, "trainer") {
		t.Errorf("expected 'trainer' hint in pending line:\n%s", got)
	}
}

func TestXP_PendingPluralization(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	// XP 6000 → level 4; classLvl 1 → 3 pending.
	xpSeedChar(t, chars, "Egwene", 6000, 1)

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Egwene"
	s.Width = 80

	runCmd(t, NewXP(chars), s, "")

	got := out.String()
	if !strings.Contains(got, "3 level-ups available") {
		t.Errorf("expected '3 level-ups available':\n%s", got)
	}
}

func TestXP_AtLevelCap(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	xpSeedChar(t, chars, "Lews", progression.XPForLevel(progression.MaxLevel)+50000, int8(progression.MaxLevel))

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Lews"
	s.Width = 80

	runCmd(t, NewXP(chars), s, "")

	got := out.String()
	if !strings.Contains(got, "Next at:") || !strings.Contains(got, "—") {
		t.Errorf("expected '—' for Next at at cap:\n%s", got)
	}
	if strings.Contains(got, "level-up available") || strings.Contains(got, "level-ups available") {
		t.Errorf("at cap with classTotal == cap should not show pending line:\n%s", got)
	}
}

func TestXP_LoadFailure(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	// No character seeded — FindByName will fail.
	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Ghost"

	runCmd(t, NewXP(chars), s, "")

	got := out.String()
	if !strings.Contains(got, "Could not load your character") {
		t.Errorf("expected error message:\n%s", got)
	}
}
