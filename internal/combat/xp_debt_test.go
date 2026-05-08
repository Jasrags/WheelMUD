package combat

import (
	"testing"

	"github.com/Jasrags/WheelMUD/internal/progression"
)

func TestDeathDebt(t *testing.T) {
	cases := []struct {
		name  string
		xp    int64
		level int
		want  int64
	}{
		{"floor of L1 is 0 sting", 0, 1, 0},
		{"sub-floor clamps to 0", -50, 1, 0}, // can't happen, but guard
		// L2 floor = 1000; into-level = xp - 1000.
		{"at L2 floor exactly", 1000, 2, 0},
		{"100 over L2 floor", 1100, 2, 10},
		{"5000 into L3", progression.XPForLevel(3) + 5000, 3, 500},
		// At MaxLevel, formula still bites the over-cap excess.
		{"MaxLevel pays on excess",
			progression.XPForLevel(progression.MaxLevel) + 9999,
			progression.MaxLevel, 999},
		// "level mismatch" defensive: caller passed too-low level.
		// DeathDebt trusts it and uses the lower floor.
		{"level lower than xp implies", 5000, 1, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeathDebt(tc.xp, tc.level)
			if got != tc.want {
				t.Errorf("DeathDebt(%d, %d) = %d, want %d",
					tc.xp, tc.level, got, tc.want)
			}
		})
	}
}

func TestApplyXPAward(t *testing.T) {
	cases := []struct {
		name        string
		award, debt int64
		wantGain    int64
		wantDebt    int64
	}{
		{"no debt, full gain", 100, 0, 100, 0},
		{"award equal to debt clears it", 100, 100, 0, 0},
		{"award smaller than debt pays only debt", 30, 100, 0, 70},
		{"award larger than debt clears it + surplus", 150, 100, 50, 0},
		{"zero award is a no-op", 0, 100, 0, 100},
		{"negative award is a no-op", -50, 100, 0, 100},
		{"both zero", 0, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gain, newDebt := ApplyXPAward(tc.award, tc.debt)
			if gain != tc.wantGain || newDebt != tc.wantDebt {
				t.Errorf("ApplyXPAward(%d, %d) = (%d, %d), want (%d, %d)",
					tc.award, tc.debt, gain, newDebt, tc.wantGain, tc.wantDebt)
			}
			// Invariant: gain + paid == award (for positive awards).
			if tc.award > 0 {
				paid := tc.debt - newDebt
				if gain+paid != tc.award {
					t.Errorf("conservation broken: gain=%d paid=%d != award=%d",
						gain, paid, tc.award)
				}
			}
			if newDebt < 0 {
				t.Errorf("newDebt = %d, must be non-negative", newDebt)
			}
			if newDebt > tc.debt {
				t.Errorf("newDebt = %d > input debt %d", newDebt, tc.debt)
			}
		})
	}
}
