package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/chargen"
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

	cmd := NewScore(chars, nil, nil)
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

// TestScore_ShowsStaminaAndEffectiveRegen verifies the Phase L slice
// 63 stamina row renders cur/max plus the *effective* regen — i.e.
// halved when wearing heavy plate. Naked baseline shows the raw
// regen rate.
func TestScore_ShowsStaminaAndEffectiveRegen(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	in := repo.Character{
		AccountID: 300,
		Name:      "Stam",
		Race:      creature.RaceHuman,
		ClassLevels: map[creature.Class]int8{
			creature.ClassArmsman: 1,
		},
		Core: creature.Core{
			HPCurrent: 10, HPMax: 10,
			StaminaCurrent: 87, StaminaMax: 100, StaminaRegen: 4,
		},
		AuthLevel: repo.AuthLevelPlayer,
	}
	if _, err := chars.Create(context.Background(), in); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Stam"
	s.Width = 80
	runCmd(t, NewScore(chars, nil, nil), s, "")
	got := out.String()
	for _, w := range []string{"Stamina:", "87 / 100", "(+4/pulse)"} {
		if !strings.Contains(got, w) {
			t.Fatalf("missing %q in score output:\n%s", w, got)
		}
	}

	// Same character with heavy plate should show the halved regen
	// (4 → 2). Score reads the gear via items repo, so seed one.
	items := repo.NewMemoryItemRepo()
	plate, _ := items.Create(context.Background(), repo.Item{
		ExternalID: "test.plate", Name: "plate",
		Type: repo.ItemTypeArmor, Weight: 50,
		Stats: &repo.ArmorStats{WeightClass: "heavy"},
	})
	eq := creature.Equipment{}
	eq.Set(creature.SlotArmor, plate.ID)
	if err := chars.RecordEquipment(context.Background(), 1, eq); err != nil {
		t.Fatalf("equip plate: %v", err)
	}

	s2, out2 := bufSession(t)
	s2.AuthLevel = telnet.AuthPlayer
	s2.CharacterName = "Stam"
	s2.Width = 80
	runCmd(t, NewScore(chars, items, nil), s2, "")
	got2 := out2.String()
	if !strings.Contains(got2, "(+2/pulse)") {
		t.Fatalf("plate regen = no halving in score:\n%s", got2)
	}
}

func TestScore_ChannelerBlockShowsPracticeAndState(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	in := repo.Character{
		AccountID: 200,
		Name:      "Moiraine",
		ClassLevels: map[creature.Class]int8{
			creature.ClassInitiate: 1,
		},
		Core: creature.Core{HPCurrent: 8, HPMax: 8, Gender: creature.GenderFemale},
		Channeling: &creature.Channeling{
			GenderSource: creature.SourceSaidar,
			Affinities:   creature.PowerSet(1 << creature.PowerFire),
			Slots: [10]creature.SlotPool{
				{Cur: 3, Max: 4},
			},
			Embraced: true,
		},
		PracticePoints: 2,
		AuthLevel:      repo.AuthLevelPlayer,
	}
	if _, err := chars.Create(context.Background(), in); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Moiraine"
	s.Width = 80

	cmd := NewScore(chars, nil, nil)
	runCmd(t, cmd, s, "")

	got := out.String()
	wants := []string{
		"Channeling",
		"Saidar",
		"L0 3/4",
		"Madness:",
		"Practice:",
		"2",
		"embraced",
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

	cmd := NewScore(chars, nil, nil)
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

	runCmd(t, NewScore(chars, nil, nil), s, "")

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

// loadDefaultChargen loads the production chargen catalog from the
// embedded fixtures. Used by score tests that exercise feat-driven
// score rendering (Phase L slice 65).
func loadDefaultChargen(t *testing.T) *chargen.Catalog {
	t.Helper()
	fsys, err := chargen.SourceFS()
	if err != nil {
		t.Fatalf("chargen source fs: %v", err)
	}
	cat, err := chargen.Load(fsys)
	if err != nil {
		t.Fatalf("chargen load: %v", err)
	}
	return cat
}

// TestScore_CombatLineCitesActiveFeats seeds a character with the
// Blademaster feat, equips a greatsword, and asserts the score
// Combat line includes the bracketed contributor name. Verifies
// the slice-65 score-sheet feedback loop end-to-end.
func TestScore_CombatLineCitesActiveFeats(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	items := repo.NewMemoryItemRepo()
	cat := loadDefaultChargen(t)

	gs, _ := items.Create(context.Background(), repo.Item{
		ExternalID: "test.greatsword", Name: "greatsword",
		Type: repo.ItemTypeWeapon, Weight: 16.0,
		Stats: &repo.WeaponStats{Damage: "1d12"},
	})
	eq := creature.Equipment{}
	eq.Set(creature.SlotPrimaryWield, gs.ID)

	in := repo.Character{
		AccountID: 400,
		Name:      "Lan",
		Race:      creature.RaceHuman,
		ClassLevels: map[creature.Class]int8{
			creature.ClassArmsman: 1,
		},
		Core:      creature.Core{HPCurrent: 10, HPMax: 10},
		Equipment: eq,
		Feats:     []int32{chargen.HashID("feat_blademaster")},
		AuthLevel: repo.AuthLevelPlayer,
	}
	if _, err := chars.Create(context.Background(), in); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Lan"
	s.Width = 80
	runCmd(t, NewScore(chars, items, cat), s, "")
	got := out.String()

	if !strings.Contains(got, "[Blademaster]") {
		t.Fatalf("Combat line should cite [Blademaster]; got:\n%s", got)
	}
	// Sanity: the line still carries the multiplier number prefix.
	if !strings.Contains(got, "Combat") {
		t.Fatalf("Combat row missing; got:\n%s", got)
	}
}

// TestScore_FeatRegenBumpRendered seeds a character with Iron
// Constitution and a base regen of 2. Expected: the Stamina line
// shows (+3/pulse) because the +1 feat bump adds before
// EffectiveStaminaRegen.
func TestScore_FeatRegenBumpRendered(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	cat := loadDefaultChargen(t)

	in := repo.Character{
		AccountID: 401,
		Name:      "Ironside",
		Race:      creature.RaceHuman,
		ClassLevels: map[creature.Class]int8{
			creature.ClassArmsman: 1,
		},
		Core: creature.Core{
			HPCurrent: 10, HPMax: 10,
			StaminaCurrent: 50, StaminaMax: 100, StaminaRegen: 2,
		},
		Feats:     []int32{chargen.HashID("feat_iron_constitution")},
		AuthLevel: repo.AuthLevelPlayer,
	}
	if _, err := chars.Create(context.Background(), in); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterName = "Ironside"
	s.Width = 80
	runCmd(t, NewScore(chars, nil, cat), s, "")
	got := out.String()

	if !strings.Contains(got, "(+3/pulse)") {
		t.Fatalf("expected (+3/pulse) with Iron Constitution; got:\n%s", got)
	}
}
