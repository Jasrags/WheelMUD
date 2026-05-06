package mode

import (
	"errors"
	"strconv"
	"strings"
)

// parsePositiveIndex interprets `s` as a 1-based list index in
// [1, n] and returns the 0-based slot. Anything non-numeric or out of
// range returns an error so the caller can render a "bad choice"
// message and re-prompt.
//
// Shared by every numbered-picker UI in the mode package: the post-
// login account menu, its drilldowns, and the chargen build hub. Stays
// in this small companion file so neither caller carries the others'
// imports.
func parsePositiveIndex(s string, n int) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v < 1 || v > n {
		return 0, errors.New("out of range")
	}
	return v - 1, nil
}
