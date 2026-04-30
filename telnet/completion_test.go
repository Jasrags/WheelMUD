package telnet

import (
	"strings"
	"testing"
)

func TestCommonPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{name: "empty", in: nil, want: ""},
		{name: "single", in: []string{"look"}, want: "look"},
		{name: "shared", in: []string{"look", "loot"}, want: "loo"},
		{name: "no overlap", in: []string{"look", "quit"}, want: ""},
		{name: "one is prefix of other", in: []string{"loo", "loot"}, want: "loo"},
		{name: "three way", in: []string{"sit", "sleep", "say"}, want: "s"},
		{name: "identical", in: []string{"go", "go", "go"}, want: "go"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := commonPrefix(tc.in); got != tc.want {
				t.Fatalf("commonPrefix(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatCandidates_Flat(t *testing.T) {
	cands := []Candidate{
		{Text: "look", Help: "Examine the room"},
		{Text: "loot", Help: "Take items from a corpse"},
		{Text: "list", Help: ""},
	}
	got := formatCandidates(cands, 80)
	wantLines := []string{
		"look  -  Examine the room",
		"loot  -  Take items from a corpse",
		"list",
	}
	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Fatalf("output missing %q\n--- output ---\n%s", line, got)
		}
	}
	if strings.Contains(got, "and ") {
		t.Fatalf("unexpected truncation marker: %q", got)
	}
}

func TestFormatCandidates_Columns(t *testing.T) {
	// 12 candidates -> exceeds helpColumnThreshold, should columnize.
	names := []string{
		"say", "scan", "search", "sell", "sense", "shout",
		"sit", "sleep", "smile", "south", "speak", "stand",
	}
	cands := make([]Candidate, len(names))
	for i, n := range names {
		cands[i] = Candidate{Text: n, Help: "help"}
	}
	got := formatCandidates(cands, 80)
	if strings.Contains(got, "  -  ") {
		t.Fatalf("columnized output should not include help dashes:\n%s", got)
	}
	for _, n := range names {
		if !strings.Contains(got, n) {
			t.Fatalf("missing candidate %q in:\n%s", n, got)
		}
	}
	// Column-major: row 0 should start with names[0] = "say".
	firstLine := strings.SplitN(got, "\r\n", 2)[0]
	if !strings.HasPrefix(firstLine, "say") {
		t.Fatalf("expected first row to start with %q, got %q", "say", firstLine)
	}
}

func TestFormatCandidates_Truncation(t *testing.T) {
	cands := make([]Candidate, 47)
	for i := range cands {
		cands[i] = Candidate{Text: "cmd"}
	}
	got := formatCandidates(cands, 80)
	want := "... and 17 more"
	if !strings.Contains(got, want) {
		t.Fatalf("expected truncation marker %q in:\n%s", want, got)
	}
}

func TestFormatCandidates_NarrowWidth(t *testing.T) {
	cands := []Candidate{
		{Text: "alphabet"}, {Text: "bravo"}, {Text: "charlie"},
		{Text: "delta"}, {Text: "echo"}, {Text: "foxtrot"},
		{Text: "golf"}, {Text: "hotel"}, {Text: "india"},
		{Text: "juliet"}, {Text: "kilo"},
	}
	// width = 1 should still produce one column without panicking.
	got := formatCandidates(cands, 1)
	for _, c := range cands {
		if !strings.Contains(got, c.Text) {
			t.Fatalf("missing %q in narrow output:\n%s", c.Text, got)
		}
	}
}

func TestCompletionPartial(t *testing.T) {
	tests := []struct {
		buffer string
		want   string
	}{
		{buffer: "", want: ""},
		{buffer: "look", want: "look"},
		{buffer: "look ", want: ""},
		{buffer: "say hello", want: "hello"},
		{buffer: "say  multi  word", want: "word"},
		{buffer: "tabbed\there", want: "here"},
	}
	for _, tc := range tests {
		t.Run(tc.buffer, func(t *testing.T) {
			if got := completionPartial(tc.buffer); got != tc.want {
				t.Fatalf("completionPartial(%q) = %q, want %q", tc.buffer, got, tc.want)
			}
		})
	}
}
