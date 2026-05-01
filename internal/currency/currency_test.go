package currency

import (
	"errors"
	"math"
	"testing"
)

func TestNew_TableValues(t *testing.T) {
	cases := []struct {
		name           string
		gc, mk, sp, cp int64
		want           Amount
	}{
		{"all zero", 0, 0, 0, 0, 0},
		{"single cp", 0, 0, 0, 1, 1},
		{"single sp", 0, 0, 1, 0, 10},
		{"single mk", 0, 1, 0, 0, 100},
		{"single gc", 1, 0, 0, 0, 1000},
		{"mixed", 1, 2, 3, 4, 1234},
		// Counts are independent — 12 silver pennies stays 120 cp,
		// not promoted into "1 mk 2 sp".
		{"twelve sp", 0, 0, 12, 0, 120},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := New(tc.gc, tc.mk, tc.sp, tc.cp)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNew_OverflowGuarded(t *testing.T) {
	if _, err := New(math.MaxInt64, 0, 0, 0); !errors.Is(err, ErrOverflow) {
		t.Fatalf("expected ErrOverflow, got %v", err)
	}
}

func TestIn_TruncatesTowardZero(t *testing.T) {
	a := Amount(1234) // 1gc 2mk 3sp 4cp
	if got := a.In(GC); got != 1 {
		t.Errorf("In(GC) = %d, want 1", got)
	}
	if got := a.In(MK); got != 12 {
		t.Errorf("In(MK) = %d, want 12", got)
	}
	if got := a.In(SP); got != 123 {
		t.Errorf("In(SP) = %d, want 123", got)
	}
	if got := a.In(CP); got != 1234 {
		t.Errorf("In(CP) = %d, want 1234", got)
	}
}

func TestSplit_GreedyLargestFirst(t *testing.T) {
	parts := Amount(1234).Split()
	want := []Part{
		{GC, 1}, {MK, 2}, {SP, 3}, {CP, 4},
	}
	if len(parts) != len(want) {
		t.Fatalf("len = %d, want %d", len(parts), len(want))
	}
	for i, p := range parts {
		if p != want[i] {
			t.Errorf("parts[%d] = %+v, want %+v", i, p, want[i])
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		amt  Amount
		want string
	}{
		{0, "0cp"},
		{1, "1cp"},
		{10, "1sp"},
		{100, "1mk"},
		{1000, "1gc"},
		{1234, "1gc 2mk 3sp 4cp"},
		{1004, "1gc 4cp"},   // skipped denominations omitted
		{305, "3mk 5cp"},    // ditto, with leading mk
		{-15, "-1sp 5cp"},   // sign on the whole expression
	}
	for _, tc := range cases {
		if got := tc.amt.Format(); got != tc.want {
			t.Errorf("Format(%d) = %q, want %q", tc.amt, got, tc.want)
		}
	}
}

func TestShort(t *testing.T) {
	cases := []struct {
		amt  Amount
		want string
	}{
		{0, "0cp"},
		{9, "9cp"},
		{10, "1sp"},
		{99, "9sp"},
		{100, "1mk"},
		{999, "9mk"},
		{1000, "1gc"},
		{1234, "1gc"},
		{-50, "-5sp"},
	}
	for _, tc := range cases {
		if got := tc.amt.Short(); got != tc.want {
			t.Errorf("Short(%d) = %q, want %q", tc.amt, got, tc.want)
		}
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Amount
	}{
		{"0", 0},
		{"50", 50},                // bare integer = cp
		{"5cp", 5},
		{"10sp", 100},
		{"1mk", 100},
		{"1gc", 1000},
		{"1gc 2mk 3sp 4cp", 1234}, // canonical form round-trips
		{"  1gc   4cp ", 1004},    // collapsed whitespace
		{"3MK 5cp", 305},          // case-insensitive suffix
		{"-3sp", -30},             // negative term for transfer flows
	}
	for _, tc := range cases {
		got, err := Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("Parse(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParse_Errors(t *testing.T) {
	cases := []struct {
		in     string
		target error
	}{
		{"", ErrEmpty},
		{"   ", ErrEmpty},
		{"5xx", ErrInvalidFormat},   // unknown denomination
		{"abc", ErrInvalidFormat},   // no number
		{"sp", ErrInvalidFormat},    // suffix only
		{"5sp 5sp", ErrInvalidFormat}, // duplicate denomination
		{"1.5mk", ErrInvalidFormat}, // no fractions
	}
	for _, tc := range cases {
		_, err := Parse(tc.in)
		if !errors.Is(err, tc.target) {
			t.Errorf("Parse(%q) err = %v, want errors.Is(%v) = true", tc.in, err, tc.target)
		}
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	values := []Amount{0, 1, 9, 10, 99, 100, 999, 1000, 1234, 9999, 1_000_000}
	for _, v := range values {
		s := v.Format()
		got, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q) for %d: %v", s, v, err)
		}
		if got != v {
			t.Errorf("round-trip %d -> %q -> %d", v, s, got)
		}
	}
}

func TestAddSub(t *testing.T) {
	a := Amount(150)
	b := Amount(75)

	sum, err := a.Add(b)
	if err != nil || sum != 225 {
		t.Fatalf("Add: got %d err=%v", sum, err)
	}

	diff, err := a.Sub(b)
	if err != nil || diff != 75 {
		t.Fatalf("Sub: got %d err=%v", diff, err)
	}

	if _, err := b.Sub(a); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	if _, err := Amount(math.MaxInt64).Add(1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("expected ErrOverflow on Add, got %v", err)
	}
}

func TestSub_OverflowOnNegativeSubtrahend(t *testing.T) {
	// Guards against silently wrapping when b < 0 makes (a - b) exceed
	// MaxInt64. Previously Sub only checked the insufficient-funds path
	// and let signed wrap through.
	if _, err := Amount(math.MaxInt64).Sub(-1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("expected ErrOverflow, got %v", err)
	}
	if _, err := Amount(math.MinInt64).Sub(1); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestSafeMul_MinInt64Edge(t *testing.T) {
	// Go wraps MinInt64 * -1 to MinInt64 instead of panicking, so the
	// canonical c/b != a check would miss this overflow without the
	// explicit guard.
	if _, err := safeMul(math.MinInt64, -1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("expected ErrOverflow for MinInt64 * -1, got %v", err)
	}
	if _, err := safeMul(-1, math.MinInt64); !errors.Is(err, ErrOverflow) {
		t.Fatalf("expected ErrOverflow for -1 * MinInt64, got %v", err)
	}
}

func TestCoinHelpers(t *testing.T) {
	if GC.Code() != "gc" || MK.Code() != "mk" || SP.Code() != "sp" || CP.Code() != "cp" {
		t.Fatalf("Code() mismatch")
	}
	if GC.Value() != 1000 || MK.Value() != 100 || SP.Value() != 10 || CP.Value() != 1 {
		t.Fatalf("Value() mismatch")
	}
	// Out-of-range Coin values must not panic.
	bad := Coin(99)
	if bad.Code() != "" || bad.Value() != 0 {
		t.Fatalf("expected zero values for unknown Coin")
	}
}
