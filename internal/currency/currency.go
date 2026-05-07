// Package currency models in-game wealth.
//
// The setting uses four coins, all stored on the Amount type as a
// signed 64-bit count of copper pennies (the base unit):
//
//	copper penny (cp) = 1   cp     — laborer's daily candle
//	silver penny (sp) = 10  cp     — laborer's daily wage
//	silver mark  (mk) = 100 cp     — standard unit of wealth
//	gold crown   (gc) = 1000 cp    — banker / noble denomination
//
// All conversions live in the `ratio` table; use the helpers (New,
// Parse, Format, Split, In) instead of multiplying by hand so a
// future denomination change is a one-line edit.
package currency

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Coin is a denomination identifier. Values are dense and ordered
// smallest-to-largest so callers can range over coinOrder when they
// need every denomination.
type Coin int

const (
	CP Coin = iota // copper penny — base unit
	SP             // silver penny — 10 cp
	MK             // silver mark  — 100 cp (standard wealth unit)
	GC             // gold crown   — 1000 cp
)

// coinOrder is the canonical largest-first sequence used by Format
// and Split. Keep this in sync with the iota above when adding new
// denominations.
var coinOrder = [...]Coin{GC, MK, SP, CP}

// ratio is the value of one coin of each denomination expressed in
// base copper pennies. Indexed by Coin.
var ratio = [...]int64{
	CP: 1,
	SP: 10,
	MK: 100,
	GC: 1000,
}

// suffix is the short string used by Format / Parse for each
// denomination. Indexed by Coin.
var suffix = [...]string{
	CP: "cp",
	SP: "sp",
	MK: "mk",
	GC: "gc",
}

// Code returns the two-letter short code for the denomination
// (e.g. CP -> "cp"). Returns the empty string for unknown values
// rather than panicking so callers can guard at the boundary.
func (c Coin) Code() string {
	if int(c) < 0 || int(c) >= len(suffix) {
		return ""
	}
	return suffix[c]
}

// Value returns the worth of one coin of this denomination in
// base copper pennies. Returns 0 for unknown values.
func (c Coin) Value() int64 {
	if int(c) < 0 || int(c) >= len(ratio) {
		return 0
	}
	return ratio[c]
}

// Amount is wealth measured in copper pennies. The zero value is a
// valid empty purse. Negative values are accepted by arithmetic
// helpers so callers can represent debts or transfers, but Sub
// refuses to underflow when called via the safe Subtract helper.
type Amount int64

// New builds an Amount from per-denomination counts. Counts are
// independent — `New(0, 0, 12, 0)` is twelve silver marks, not
// "1 gc 2 mk", and is equivalent to `New(0, 0, 10, 0).Add(New(0,
// 0, 2, 0))`. Returns ErrOverflow if the sum would exceed int64.
func New(gc, mk, sp, cp int64) (Amount, error) {
	parts := [...]struct {
		count int64
		mult  int64
	}{
		{gc, ratio[GC]},
		{mk, ratio[MK]},
		{sp, ratio[SP]},
		{cp, ratio[CP]},
	}
	var total int64
	for _, p := range parts {
		v, err := safeMul(p.count, p.mult)
		if err != nil {
			return 0, err
		}
		next, err := safeAdd(total, v)
		if err != nil {
			return 0, err
		}
		total = next
	}
	return Amount(total), nil
}

// MustNew is the panicking variant for tests and seed data where the
// constants are known good.
func MustNew(gc, mk, sp, cp int64) Amount {
	a, err := New(gc, mk, sp, cp)
	if err != nil {
		panic(err)
	}
	return a
}

// In returns the amount expressed in the given denomination,
// truncated toward zero. `Amount(150).In(SP)` is 15 (silver
// pennies); `Amount(99).In(MK)` is 0.
func (a Amount) In(c Coin) int64 {
	v := c.Value()
	if v == 0 {
		return 0
	}
	return int64(a) / v
}

// Split decomposes a positive amount into greedy largest-first
// counts. The returned slice has one entry per denomination in
// coinOrder, including zero counts, so callers can render or
// iterate without re-checking ordering. For negative amounts the
// magnitudes are returned with the leading sign carried by the
// caller (Format handles this).
func (a Amount) Split() []Part {
	rem := int64(a)
	if rem < 0 {
		rem = -rem
	}
	out := make([]Part, 0, len(coinOrder))
	for _, c := range coinOrder {
		v := ratio[c]
		out = append(out, Part{Coin: c, Count: rem / v})
		rem %= v
	}
	return out
}

// Part is a single denomination + count pair, produced by Split.
type Part struct {
	Coin  Coin
	Count int64
}

// Format renders the amount greedy-largest-first ("1gc 2mk 3sp
// 4cp"). Zero renders as "0cp". Negative amounts get a leading
// minus on the whole expression ("-2mk 5cp"), not per term.
func (a Amount) Format() string {
	if a == 0 {
		return "0cp"
	}
	parts := a.Split()
	var b strings.Builder
	if a < 0 {
		b.WriteByte('-')
	}
	first := true
	for _, p := range parts {
		if p.Count == 0 {
			continue
		}
		if !first {
			b.WriteByte(' ')
		}
		first = false
		b.WriteString(strconv.FormatInt(p.Count, 10))
		b.WriteString(p.Coin.Code())
	}
	return b.String()
}

// Short renders only the largest non-zero denomination, truncating
// toward zero ("1gc", "3sp"). Zero renders as "0cp". Useful for
// terse displays like a `who` line that should never wrap.
func (a Amount) Short() string {
	if a == 0 {
		return "0cp"
	}
	mag := int64(a)
	if mag < 0 {
		mag = -mag
	}
	for _, c := range coinOrder {
		v := ratio[c]
		if mag >= v {
			n := mag / v
			if a < 0 {
				return "-" + strconv.FormatInt(n, 10) + c.Code()
			}
			return strconv.FormatInt(n, 10) + c.Code()
		}
	}
	return "0cp"
}

// Add returns a + b, or ErrOverflow on signed-int64 wrap.
func (a Amount) Add(b Amount) (Amount, error) {
	v, err := safeAdd(int64(a), int64(b))
	if err != nil {
		return 0, err
	}
	return Amount(v), nil
}

// Sub returns a - b. Returns ErrInsufficientFunds when subtracting
// a non-negative b would drive a below zero. Returns ErrOverflow
// when the result would wrap int64 (relevant only when b is
// negative, which is rare but legal for transfer flows).
func (a Amount) Sub(b Amount) (Amount, error) {
	if b >= 0 && b > a {
		return 0, ErrInsufficientFunds
	}
	v, err := safeSub(int64(a), int64(b))
	if err != nil {
		return 0, err
	}
	return Amount(v), nil
}

// Parse reads a wealth expression such as "1gc 2mk 3sp 4cp",
// "10sp", "5mk 3cp", or a bare integer (interpreted as cp).
// Whitespace between terms is collapsed; suffixes are case-
// insensitive. Each denomination may appear at most once, so
// "5sp 5sp" is rejected. Returns ErrEmpty for an all-whitespace
// input and ErrInvalidFormat for anything that doesn't tokenize.
func Parse(s string) (Amount, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrEmpty
	}

	// Bare integer is shorthand for copper pennies, matching
	// how players will type "give 50" at a fellow adventurer.
	if isAllDigitsOrSign(s) {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("currency: %w: %q", ErrInvalidFormat, s)
		}
		return Amount(n), nil
	}

	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return 0, ErrEmpty
	}

	seen := make(map[Coin]bool, len(tokens))
	var total Amount
	for _, tok := range tokens {
		count, code, err := splitNumberSuffix(tok)
		if err != nil {
			return 0, err
		}
		coin, ok := coinByCode(code)
		if !ok {
			return 0, fmt.Errorf("currency: %w: unknown denomination %q", ErrInvalidFormat, code)
		}
		if seen[coin] {
			return 0, fmt.Errorf("currency: %w: %s repeated", ErrInvalidFormat, code)
		}
		seen[coin] = true

		v, err := safeMul(count, ratio[coin])
		if err != nil {
			return 0, err
		}
		next, err := safeAdd(int64(total), v)
		if err != nil {
			return 0, err
		}
		total = Amount(next)
	}
	return total, nil
}

// Errors surfaced from this package. Wrapped via fmt.Errorf where
// extra context is helpful so callers can errors.Is on the sentinel.
var (
	ErrInvalidFormat     = errors.New("invalid currency format")
	ErrEmpty             = errors.New("empty currency string")
	ErrOverflow          = errors.New("currency arithmetic overflow")
	ErrInsufficientFunds = errors.New("insufficient funds")
)

// splitNumberSuffix peels the trailing letters off a single token
// like "12mk" -> (12, "mk"). The number portion may be negative
// ("-3sp" is valid for transfer flows).
func splitNumberSuffix(tok string) (int64, string, error) {
	cut := -1
	for i, r := range tok {
		if unicode.IsLetter(r) {
			cut = i
			break
		}
	}
	if cut <= 0 || cut == len(tok) {
		return 0, "", fmt.Errorf("currency: %w: %q", ErrInvalidFormat, tok)
	}
	num, suf := tok[:cut], strings.ToLower(tok[cut:])
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("currency: %w: %q", ErrInvalidFormat, tok)
	}
	return n, suf, nil
}

func coinByCode(code string) (Coin, bool) {
	for c, s := range suffix {
		if s == code {
			return Coin(c), true
		}
	}
	return 0, false
}

func isAllDigitsOrSign(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// safeAdd / safeMul guard the int64 base-unit arithmetic against
// silent wrap-around. Wealth in this game won't realistically come
// near these bounds, but the helpers cost essentially nothing and
// keep Format/Parse honest under fuzz.
func safeAdd(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

// safeSub avoids the negate-then-add dance because -MinInt64 wraps
// to MinInt64. Treat each sign of b directly so the bounds check
// stays in MaxInt64/MinInt64 terms.
func safeSub(a, b int64) (int64, error) {
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return 0, ErrOverflow
	}
	return a - b, nil
}

func safeMul(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	// Go does not panic on MinInt64 / -1 — it wraps to MinInt64 —
	// so c/b == a would falsely report "no overflow" for that input.
	// Guard explicitly.
	if (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
		return 0, ErrOverflow
	}
	c := a * b
	if c/b != a {
		return 0, ErrOverflow
	}
	return c, nil
}
