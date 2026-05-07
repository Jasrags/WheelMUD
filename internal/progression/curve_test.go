package progression

import "testing"

func TestXPForLevel_Boundaries(t *testing.T) {
	tests := []struct {
		n    int
		want int64
	}{
		{0, 0},
		{1, 0},
		{2, 1000},
		{3, 3000},
		{4, 6000},
		{5, 10000},
		{10, 45000},
		{20, 190000},
	}
	for _, tt := range tests {
		if got := XPForLevel(tt.n); got != tt.want {
			t.Errorf("XPForLevel(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestXPForLevel_BelowOneAndAboveCap(t *testing.T) {
	if got := XPForLevel(-5); got != 0 {
		t.Errorf("XPForLevel(-5) = %d, want 0", got)
	}
	cap := XPForLevel(MaxLevel)
	if got := XPForLevel(MaxLevel + 5); got != cap {
		t.Errorf("XPForLevel(MaxLevel+5) = %d, want %d (cap)", got, cap)
	}
}

func TestLevelForXP_Boundaries(t *testing.T) {
	tests := []struct {
		xp   int64
		want int
	}{
		{-100, 1},
		{0, 1},
		{1, 1},
		{999, 1},
		{1000, 2},
		{2999, 2},
		{3000, 3},
		{45000, 10},
		{190000, 20},
		{999999, 20},
	}
	for _, tt := range tests {
		if got := LevelForXP(tt.xp); got != tt.want {
			t.Errorf("LevelForXP(%d) = %d, want %d", tt.xp, got, tt.want)
		}
	}
}

func TestLevelForXP_BoundaryExact(t *testing.T) {
	for n := 1; n <= MaxLevel; n++ {
		if got := LevelForXP(XPForLevel(n)); got != n {
			t.Errorf("LevelForXP(XPForLevel(%d)) = %d, want %d", n, got, n)
		}
	}
}

func TestXPToNext_Mid(t *testing.T) {
	// At 1500 XP: level 2 (since 1000 <= 1500 < 3000), toNext = 1500.
	level, toNext := XPToNext(1500)
	if level != 2 || toNext != 1500 {
		t.Fatalf("XPToNext(1500) = (%d, %d), want (2, 1500)", level, toNext)
	}
}

func TestXPToNext_AtThreshold(t *testing.T) {
	// At exactly XPForLevel(5): just turned 5; toNext = XPForLevel(6) - that.
	level, toNext := XPToNext(XPForLevel(5))
	if level != 5 {
		t.Fatalf("level = %d, want 5", level)
	}
	want := XPForLevel(6) - XPForLevel(5)
	if toNext != want {
		t.Fatalf("toNext = %d, want %d", toNext, want)
	}
}

func TestXPToNext_AtCap(t *testing.T) {
	level, toNext := XPToNext(XPForLevel(MaxLevel))
	if level != MaxLevel || toNext != 0 {
		t.Fatalf("XPToNext(cap) = (%d, %d), want (%d, 0)", level, toNext, MaxLevel)
	}
	level, toNext = XPToNext(XPForLevel(MaxLevel) + 999999)
	if level != MaxLevel || toNext != 0 {
		t.Fatalf("XPToNext(over-cap) = (%d, %d), want (%d, 0)", level, toNext, MaxLevel)
	}
}

func TestRoundtrip_LevelForXP_Of_XPForLevel(t *testing.T) {
	for n := 1; n <= MaxLevel; n++ {
		got := LevelForXP(XPForLevel(n))
		if got != n {
			t.Errorf("roundtrip n=%d: got %d", n, got)
		}
	}
}

func TestCurve_Monotonic(t *testing.T) {
	prev := int64(-1)
	for n := 1; n <= MaxLevel; n++ {
		x := XPForLevel(n)
		if x <= prev {
			t.Fatalf("XPForLevel not strictly increasing at n=%d: %d <= %d", n, x, prev)
		}
		prev = x
	}
}
