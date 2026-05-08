package progression

import (
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

func loadCat(t *testing.T) *chargen.Catalog {
	t.Helper()
	fsys, err := chargen.SourceFS()
	if err != nil {
		t.Fatalf("chargen source: %v", err)
	}
	cat, err := chargen.Load(fsys)
	if err != nil {
		t.Fatalf("chargen load: %v", err)
	}
	return cat
}

func TestAbilityModifier(t *testing.T) {
	cases := []struct {
		score, want int
	}{
		{1, -5},
		{8, -1},
		{9, -1},
		{10, 0},
		{11, 0},
		{12, 1},
		{14, 2},
		{18, 4},
		{20, 5},
	}
	for _, tc := range cases {
		if got := AbilityModifier(tc.score); got != tc.want {
			t.Errorf("AbilityModifier(%d) = %d, want %d", tc.score, got, tc.want)
		}
	}
}

func TestHPDelta(t *testing.T) {
	cases := []struct {
		hitDie, conMod int
		want           int32
	}{
		{4, 0, 3},   // d4 → ceil(4/2)+1 = 3
		{6, 0, 4},   // d6 → 4
		{8, 0, 5},   // d8 → 5
		{10, 0, 6},  // d10 → 6
		{10, 2, 8},  // d10 + Con +2
		{10, -2, 4}, // d10 + Con -2
		{4, -5, 1},  // floored at 1
		{0, 0, 1},   // hitDie clamped to 1 → ceil(1/2)+1 = 2 ... actually 1+1=2; min still 1
	}
	for _, tc := range cases {
		got := HPDelta(tc.hitDie, tc.conMod)
		// Special-case the clamp: hitDie<1 floors to 1, avgUp=1+1=2
		if tc.hitDie == 0 && tc.want == 1 {
			tc.want = 2
		}
		if got != tc.want {
			t.Errorf("HPDelta(d%d, conMod=%d) = %d, want %d",
				tc.hitDie, tc.conMod, got, tc.want)
		}
	}
}

func TestBABTotal_SingleClass(t *testing.T) {
	cat := loadCat(t)
	// Armsman is BAB high.
	cases := []struct {
		lvl  int8
		want int16
	}{{1, 1}, {4, 4}, {20, 20}}
	for _, tc := range cases {
		got := BABTotal(map[creature.Class]int8{
			creature.ClassArmsman: tc.lvl,
		}, cat)
		if got != tc.want {
			t.Errorf("Armsman L%d BAB = %d, want %d", tc.lvl, got, tc.want)
		}
	}
}

func TestBABTotal_Multiclass(t *testing.T) {
	cat := loadCat(t)
	// Armsman (high) L5 = 5; Initiate (medium) L3 = floor(9/4) = 2.
	// Total = 7.
	got := BABTotal(map[creature.Class]int8{
		creature.ClassArmsman:  5,
		creature.ClassInitiate: 3,
	}, cat)
	if got != 7 {
		t.Errorf("multiclass BAB = %d, want 7", got)
	}
}

func TestBABTotal_NilCatalog(t *testing.T) {
	got := BABTotal(map[creature.Class]int8{creature.ClassArmsman: 5}, nil)
	if got != 0 {
		t.Errorf("nil cat → %d, want 0", got)
	}
}

func TestSaveTotal_SingleClass(t *testing.T) {
	cat := loadCat(t)
	// Armsman: Fort=high, Ref=low, Will=low.
	cls := map[creature.Class]int8{creature.ClassArmsman: 1}
	if got := SaveTotal(cls, cat, SaveFortOf, 0); got != 2 {
		t.Errorf("Armsman L1 Fort base = %d, want 2", got)
	}
	if got := SaveTotal(cls, cat, SaveRefOf, 0); got != 0 {
		t.Errorf("Armsman L1 Ref base = %d, want 0", got)
	}
	cls[creature.ClassArmsman] = 20
	if got := SaveTotal(cls, cat, SaveFortOf, 0); got != 12 {
		t.Errorf("Armsman L20 Fort = %d, want 12 (2 + 20/2)", got)
	}
	if got := SaveTotal(cls, cat, SaveRefOf, 0); got != 6 {
		t.Errorf("Armsman L20 Ref = %d, want 6 (20/3)", got)
	}
}

func TestSaveTotal_AbilityModAddedOnce(t *testing.T) {
	cat := loadCat(t)
	cls := map[creature.Class]int8{
		creature.ClassArmsman: 4, // Fort high → 2+2=4
		creature.ClassWilder:  4, // Fort low  → 4/3=1
	}
	// Total base Fort = 5; +Con mod once = 5+3 = 8
	if got := SaveTotal(cls, cat, SaveFortOf, 3); got != 8 {
		t.Errorf("multiclass Fort+conMod = %d, want 8", got)
	}
}

func mkChar(con, dex, wis int8) repo.Character {
	return repo.Character{
		ClassLevels: map[creature.Class]int8{creature.ClassArmsman: 1},
		Core: creature.Core{
			HPCurrent: 12,
			HPMax:     12,
			Abilities: creature.Abilities{
				Con: creature.AbilityScore{Current: con, Max: con},
				Dex: creature.AbilityScore{Current: dex, Max: dex},
				Wis: creature.AbilityScore{Current: wis, Max: wis},
			},
		},
	}
}

func TestComputeLevelUp_AdvancesExistingClass(t *testing.T) {
	cat := loadCat(t)
	ch := mkChar(14, 12, 10) // +2 Con, +1 Dex, +0 Wis

	got, err := ComputeLevelUp(ch, cat, creature.ClassArmsman)
	if err != nil {
		t.Fatalf("ComputeLevelUp: %v", err)
	}
	if got.ClassLevels[creature.ClassArmsman] != 2 {
		t.Errorf("class advancement = %d, want 2", got.ClassLevels[creature.ClassArmsman])
	}
	if got.NewLevel != 2 {
		t.Errorf("NewLevel = %d, want 2", got.NewLevel)
	}
	// Armsman is d10. avgUp = 6, +2 Con = 8.
	if got.HPDelta != 8 {
		t.Errorf("HPDelta = %d, want 8", got.HPDelta)
	}
	if got.NewHPMax != 12+8 {
		t.Errorf("NewHPMax = %d, want 20", got.NewHPMax)
	}
	if got.NewHPCurrent != 12+8 {
		t.Errorf("NewHPCurrent = %d, want 20", got.NewHPCurrent)
	}
	// Armsman L2: BAB high = 2.
	if got.NewBAB != 2 {
		t.Errorf("NewBAB = %d, want 2", got.NewBAB)
	}
	// Armsman L2: Fort high = 2 + 2/2 = 3, +Con(+2) = 5.
	if got.NewSaves.Fort != 5 {
		t.Errorf("Fort = %d, want 5", got.NewSaves.Fort)
	}
	// Ref low at L2 = 2/3 = 0; +Dex(+1) = 1.
	if got.NewSaves.Ref != 1 {
		t.Errorf("Ref = %d, want 1", got.NewSaves.Ref)
	}
}

func TestComputeLevelUp_OpensNewClassMulticlass(t *testing.T) {
	cat := loadCat(t)
	ch := mkChar(10, 10, 10)
	// Player has only Armsman; advance Wilder → opens at 1.
	got, err := ComputeLevelUp(ch, cat, creature.ClassWilder)
	if err != nil {
		t.Fatalf("ComputeLevelUp: %v", err)
	}
	if got.ClassLevels[creature.ClassWilder] != 1 {
		t.Errorf("new class level = %d, want 1", got.ClassLevels[creature.ClassWilder])
	}
	if got.ClassLevels[creature.ClassArmsman] != 1 {
		t.Errorf("old class preserved = %d, want 1", got.ClassLevels[creature.ClassArmsman])
	}
	if got.NewLevel != 1 {
		t.Errorf("NewLevel for opened class = %d, want 1", got.NewLevel)
	}
}

func TestComputeLevelUp_RejectsUnknownClass(t *testing.T) {
	cat := loadCat(t)
	ch := mkChar(10, 10, 10)
	// Class 99 is not in the catalog enum.
	_, err := ComputeLevelUp(ch, cat, creature.Class(99))
	if !errors.Is(err, ErrUnknownClass) {
		t.Fatalf("err = %v, want ErrUnknownClass", err)
	}
}

func TestComputeLevelUp_NilCatalog(t *testing.T) {
	ch := mkChar(10, 10, 10)
	_, err := ComputeLevelUp(ch, nil, creature.ClassArmsman)
	if !errors.Is(err, ErrUnknownClass) {
		t.Fatalf("nil cat err = %v, want ErrUnknownClass", err)
	}
}

func TestComputeLevelUp_HPCurrentCappedAtNewMax(t *testing.T) {
	cat := loadCat(t)
	ch := mkChar(10, 10, 10)
	ch.Core.HPCurrent = 5 // wounded
	ch.Core.HPMax = 12

	got, _ := ComputeLevelUp(ch, cat, creature.ClassArmsman)
	// d10 + Con(0) = 6 delta. Wounded char gets 5+6 = 11 (under new max 18).
	if got.NewHPCurrent != 11 {
		t.Errorf("wounded current = %d, want 11", got.NewHPCurrent)
	}
	if got.NewHPMax != 18 {
		t.Errorf("max = %d, want 18", got.NewHPMax)
	}
}

func TestComputeLevelUp_SourceCharacterUnmutated(t *testing.T) {
	cat := loadCat(t)
	ch := mkChar(14, 12, 10)
	beforeHP := ch.Core.HPMax
	beforeCL := ch.ClassLevels[creature.ClassArmsman]

	_, _ = ComputeLevelUp(ch, cat, creature.ClassArmsman)

	if ch.Core.HPMax != beforeHP {
		t.Error("source HPMax mutated")
	}
	if ch.ClassLevels[creature.ClassArmsman] != beforeCL {
		t.Error("source ClassLevels mutated")
	}
}
