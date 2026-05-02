package repo

import "strings"

// Placeholders returns "?, ?, ?, ..." with n question marks, suitable
// for splicing into SQL VALUES clauses. Returns "" when n <= 0.
func Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

// BoolToInt maps Go bools to the 0/1 form SQLite uses for INTEGER
// boolean columns. The reverse direction is handled inline at scan
// sites (`flag != 0`) since each row decode wants the column-by-column
// shape.
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// placeholders / boolToInt remain as package-internal aliases so
// existing callsites inside repo don't have to chase the rename.
func placeholders(n int) string { return Placeholders(n) }
func boolToInt(b bool) int      { return BoolToInt(b) }
