package affects

import (
	"reflect"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

func newCore() creature.Core {
	return creature.Core{
		Abilities: creature.Abilities{
			Str: creature.AbilityScore{Current: 12, Max: 12},
			Dex: creature.AbilityScore{Current: 14, Max: 14},
			Con: creature.AbilityScore{Current: 10, Max: 10},
			Int: creature.AbilityScore{Current: 10, Max: 10},
			Wis: creature.AbilityScore{Current: 10, Max: 10},
			Cha: creature.AbilityScore{Current: 10, Max: 10},
		},
		Defense: 14,
		Saves:   creature.Saves{Fort: 1, Ref: 2, Will: 3},
		Speed:   creature.Speed{BaseFt: 30},
		BAB:     4,
	}
}

func TestEffective_ZeroAffects_Passthrough(t *testing.T) {
	c := newCore()
	got := Effective(c)
	if !reflect.DeepEqual(got, c) {
		t.Fatalf("zero-affect Effective should equal input; got %+v", got)
	}
}

func TestEffective_AllFieldBranches(t *testing.T) {
	cases := []struct {
		field  string
		delta  int16
		check  func(creature.Core) int16
		expect int16
	}{
		{FieldStrCurrent, +2, func(c creature.Core) int16 { return int16(c.Abilities.Str.Current) }, 14},
		{FieldDexCurrent, -3, func(c creature.Core) int16 { return int16(c.Abilities.Dex.Current) }, 11},
		{FieldConCurrent, +1, func(c creature.Core) int16 { return int16(c.Abilities.Con.Current) }, 11},
		{FieldIntCurrent, +1, func(c creature.Core) int16 { return int16(c.Abilities.Int.Current) }, 11},
		{FieldWisCurrent, +1, func(c creature.Core) int16 { return int16(c.Abilities.Wis.Current) }, 11},
		{FieldChaCurrent, +1, func(c creature.Core) int16 { return int16(c.Abilities.Cha.Current) }, 11},
		{FieldDefense, +3, func(c creature.Core) int16 { return c.Defense }, 17},
		{FieldSavesFort, +2, func(c creature.Core) int16 { return c.Saves.Fort }, 3},
		{FieldSavesRef, +2, func(c creature.Core) int16 { return c.Saves.Ref }, 4},
		{FieldSavesWill, -1, func(c creature.Core) int16 { return c.Saves.Will }, 2},
		{FieldSpeedBase, -10, func(c creature.Core) int16 { return c.Speed.BaseFt }, 20},
		{FieldBAB, +1, func(c creature.Core) int16 { return c.BAB }, 5},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			c := newCore()
			c.Affects = []creature.Affect{{
				Name:          "test",
				DurationTicks: 5,
				Modifiers:     []creature.StatMod{{Field: tc.field, Delta: tc.delta}},
			}}
			got := tc.check(Effective(c))
			if got != tc.expect {
				t.Fatalf("%s: want %d, got %d", tc.field, tc.expect, got)
			}
		})
	}
}

func TestEffective_DoesNotMutateInput(t *testing.T) {
	c := newCore()
	c.Affects = []creature.Affect{{
		Modifiers: []creature.StatMod{{Field: FieldDefense, Delta: +5}},
	}}
	before := c.Defense
	_ = Effective(c)
	if c.Defense != before {
		t.Fatalf("Effective mutated input: before=%d after=%d", before, c.Defense)
	}
}

func TestEffective_StackingMultipleAffects(t *testing.T) {
	c := newCore()
	c.Affects = []creature.Affect{
		{Name: "blessed", Modifiers: []creature.StatMod{{Field: FieldDefense, Delta: +2}}},
		{Name: "shielded", Modifiers: []creature.StatMod{{Field: FieldDefense, Delta: +3}}},
		{Name: "weakened", Modifiers: []creature.StatMod{{Field: FieldStrCurrent, Delta: -2}}},
	}
	got := Effective(c)
	if got.Defense != 19 {
		t.Fatalf("Defense stack: want 19, got %d", got.Defense)
	}
	if got.Abilities.Str.Current != 10 {
		t.Fatalf("Str.Current after debuff: want 10, got %d", got.Abilities.Str.Current)
	}
}

func TestEffective_UnknownFieldIgnored(t *testing.T) {
	c := newCore()
	c.Affects = []creature.Affect{{
		Modifiers: []creature.StatMod{{Field: "Nonsense.Field", Delta: 99}},
	}}
	got := Effective(c)
	if !reflect.DeepEqual(got, c) {
		t.Fatalf("unknown field should be ignored; got %+v", got)
	}
}

func TestEffective_SpeedClampsAtZero(t *testing.T) {
	c := newCore()
	c.Speed.BaseFt = 5
	c.Affects = []creature.Affect{{
		Modifiers: []creature.StatMod{{Field: FieldSpeedBase, Delta: -50}},
	}}
	if got := Effective(c).Speed.BaseFt; got != 0 {
		t.Fatalf("Speed.BaseFt clamp: want 0, got %d", got)
	}
}

func TestEffective_AbilityCurrentClampsToInt8(t *testing.T) {
	c := newCore()
	c.Abilities.Str.Current = 120
	c.Affects = []creature.Affect{{
		Modifiers: []creature.StatMod{{Field: FieldStrCurrent, Delta: 100}},
	}}
	if got := Effective(c).Abilities.Str.Current; got != 127 {
		t.Fatalf("Str clamp upper: want 127, got %d", got)
	}
	c2 := newCore()
	c2.Abilities.Str.Current = -120
	c2.Affects = []creature.Affect{{
		Modifiers: []creature.StatMod{{Field: FieldStrCurrent, Delta: -100}},
	}}
	if got := Effective(c2).Abilities.Str.Current; got != -128 {
		t.Fatalf("Str clamp lower: want -128, got %d", got)
	}
}

func TestTick_Empty(t *testing.T) {
	out, exp := Tick(nil)
	if out != nil || exp != nil {
		t.Fatalf("empty Tick: want (nil, nil), got (%v, %v)", out, exp)
	}
}

func TestTick_DecrementAndExpiry(t *testing.T) {
	in := []creature.Affect{
		{Name: "long", DurationTicks: 5},
		{Name: "short", DurationTicks: 1}, // expires this tick
		{Name: "mid", DurationTicks: 2},
		{Name: "stale", DurationTicks: 0}, // already expired
	}
	out, exp := Tick(in)

	if len(out) != 2 {
		t.Fatalf("kept count: want 2, got %d (%+v)", len(out), out)
	}
	if out[0].Name != "long" || out[0].DurationTicks != 4 {
		t.Fatalf("long: %+v", out[0])
	}
	if out[1].Name != "mid" || out[1].DurationTicks != 1 {
		t.Fatalf("mid: %+v", out[1])
	}
	wantExp := []string{"short", "stale"}
	if !reflect.DeepEqual(exp, wantExp) {
		t.Fatalf("expired: want %v, got %v", wantExp, exp)
	}
}

func TestTick_DoesNotMutateInput(t *testing.T) {
	in := []creature.Affect{{Name: "x", DurationTicks: 3}}
	_, _ = Tick(in)
	if in[0].DurationTicks != 3 {
		t.Fatalf("Tick mutated input duration: %d", in[0].DurationTicks)
	}
}

func TestApply_AppendNew(t *testing.T) {
	in := []creature.Affect{{Source: 1, Name: "blessed", DurationTicks: 5}}
	out := Apply(in, creature.Affect{Source: 2, Name: "hasted", DurationTicks: 3})
	if len(out) != 2 {
		t.Fatalf("want 2, got %d", len(out))
	}
	if out[1].Name != "hasted" {
		t.Fatalf("appended: %+v", out[1])
	}
}

func TestApply_RefreshSameSourceAndName(t *testing.T) {
	in := []creature.Affect{
		{Source: 1, Name: "blessed", DurationTicks: 1},
		{Source: 2, Name: "hasted", DurationTicks: 5},
	}
	out := Apply(in, creature.Affect{Source: 1, Name: "blessed", DurationTicks: 10})
	if len(out) != 2 {
		t.Fatalf("dedup len: want 2, got %d", len(out))
	}
	if out[0].DurationTicks != 10 {
		t.Fatalf("refresh duration: want 10, got %d", out[0].DurationTicks)
	}
	if out[1].Name != "hasted" || out[1].DurationTicks != 5 {
		t.Fatalf("untouched entry mutated: %+v", out[1])
	}
}

func TestApply_DistinctSourcesSameNameCoexist(t *testing.T) {
	in := []creature.Affect{{Source: 1, Name: "poison", DurationTicks: 5}}
	out := Apply(in, creature.Affect{Source: 2, Name: "poison", DurationTicks: 3})
	if len(out) != 2 {
		t.Fatalf("want both poisons; got %d entries: %+v", len(out), out)
	}
}

func TestApply_DistinctNamesSameSourceCoexist(t *testing.T) {
	in := []creature.Affect{{Source: 1, Name: "blessed", DurationTicks: 5}}
	out := Apply(in, creature.Affect{Source: 1, Name: "shielded", DurationTicks: 3})
	if len(out) != 2 {
		t.Fatalf("want 2, got %d", len(out))
	}
}

func TestApply_DoesNotMutateInput(t *testing.T) {
	in := []creature.Affect{{Source: 1, Name: "x", DurationTicks: 5}}
	_ = Apply(in, creature.Affect{Source: 1, Name: "x", DurationTicks: 99})
	if in[0].DurationTicks != 5 {
		t.Fatalf("Apply mutated input: %d", in[0].DurationTicks)
	}
}

func TestEffective_FoldsConditionMask(t *testing.T) {
	c := newCore()
	c.Conditions = creature.CondProne
	c.Affects = []creature.Affect{
		{Name: "blinding_dust", DurationTicks: 5, ConditionMask: creature.CondBlinded},
		{Name: "stunned", DurationTicks: 3, ConditionMask: creature.CondStunned | creature.CondDazed},
	}
	got := Effective(c)
	want := creature.CondProne | creature.CondBlinded | creature.CondStunned | creature.CondDazed
	if got.Conditions != want {
		t.Fatalf("Conditions: want %032b, got %032b", want, got.Conditions)
	}
	// input must not be mutated
	if c.Conditions != creature.CondProne {
		t.Fatalf("input Conditions mutated: %032b", c.Conditions)
	}
}

func TestApply_StackingCapEvictsShortest(t *testing.T) {
	src := func(s int64, dur int32) creature.Affect {
		return creature.Affect{Source: s, Name: "poison", DurationTicks: dur}
	}
	in := []creature.Affect{src(1, 10), src(2, 5), src(3, 20), src(4, 15)}
	if len(in) != MaxAffectsPerName {
		t.Fatalf("test premise: want full slice (%d), got %d", MaxAffectsPerName, len(in))
	}
	// 5th source pushes count over cap; victim is Source=2 (dur=5).
	out := Apply(in, src(5, 30))
	if len(out) != MaxAffectsPerName {
		t.Fatalf("post-cap len: want %d, got %d", MaxAffectsPerName, len(out))
	}
	for _, a := range out {
		if a.Source == 2 {
			t.Fatalf("Source=2 (shortest) should have been evicted; out=%+v", out)
		}
	}
	var foundNew bool
	for _, a := range out {
		if a.Source == 5 && a.DurationTicks == 30 {
			foundNew = true
		}
	}
	if !foundNew {
		t.Fatalf("new source not present: %+v", out)
	}
}

func TestApply_RefreshBypassesStackingCap(t *testing.T) {
	src := func(s int64, dur int32) creature.Affect {
		return creature.Affect{Source: s, Name: "poison", DurationTicks: dur}
	}
	in := []creature.Affect{src(1, 10), src(2, 5), src(3, 20), src(4, 15)}
	// Refreshing Source=2 must NOT evict — it just rewrites that slot.
	out := Apply(in, src(2, 99))
	if len(out) != MaxAffectsPerName {
		t.Fatalf("refresh shouldn't grow or shrink; got %d", len(out))
	}
	for _, a := range out {
		if a.Source == 2 && a.DurationTicks != 99 {
			t.Fatalf("Source=2 not refreshed: %+v", a)
		}
	}
}

func TestApply_StackingCapDifferentNamesUnaffected(t *testing.T) {
	in := []creature.Affect{
		{Source: 1, Name: "poison", DurationTicks: 5},
		{Source: 2, Name: "poison", DurationTicks: 5},
		{Source: 3, Name: "poison", DurationTicks: 5},
		{Source: 4, Name: "poison", DurationTicks: 5},
		{Source: 5, Name: "blessed", DurationTicks: 5},
	}
	// New "blessed" entry should append cleanly — different Name.
	out := Apply(in, creature.Affect{Source: 6, Name: "blessed", DurationTicks: 5})
	if len(out) != 6 {
		t.Fatalf("cross-name shouldn't evict; got %d", len(out))
	}
}
