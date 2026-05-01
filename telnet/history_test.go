package telnet

import "testing"

func TestHistoryAddBasic(t *testing.T) {
	var h History
	h.Add("one")
	h.Add("two")
	h.Add("three")
	if h.Len() != 3 {
		t.Fatalf("Len = %d, want 3", h.Len())
	}
}

func TestHistoryDedupesAdjacent(t *testing.T) {
	var h History
	h.Add("ls")
	h.Add("ls")
	h.Add("ls")
	if h.Len() != 1 {
		t.Fatalf("adjacent dupes should collapse, Len = %d", h.Len())
	}
}

func TestHistorySkipsEmpty(t *testing.T) {
	var h History
	h.Add("")
	h.Add("x")
	h.Add("")
	if h.Len() != 1 {
		t.Fatalf("empty lines must not be stored, Len = %d", h.Len())
	}
}

func TestHistoryPrevNextWalk(t *testing.T) {
	var h History
	h.Add("a")
	h.Add("b")
	h.Add("c")

	if v, ok := h.Prev("draft"); !ok || v != "c" {
		t.Fatalf("first Prev = (%q,%v), want (c,true)", v, ok)
	}
	if v, ok := h.Prev(""); !ok || v != "b" {
		t.Fatalf("second Prev = (%q,%v), want (b,true)", v, ok)
	}
	if v, ok := h.Prev(""); !ok || v != "a" {
		t.Fatalf("third Prev = (%q,%v), want (a,true)", v, ok)
	}
	// Past the top — clamps to oldest.
	if v, ok := h.Prev(""); !ok || v != "a" {
		t.Fatalf("clamped Prev = (%q,%v), want (a,true)", v, ok)
	}
	if v, ok := h.Next(); !ok || v != "b" {
		t.Fatalf("Next = (%q,%v), want (b,true)", v, ok)
	}
	if v, ok := h.Next(); !ok || v != "c" {
		t.Fatalf("Next = (%q,%v), want (c,true)", v, ok)
	}
	// Bottom — restore the original draft.
	if v, ok := h.Next(); !ok || v != "draft" {
		t.Fatalf("Next at bottom = (%q,%v), want (draft,true)", v, ok)
	}
	// Past the bottom — bell.
	if _, ok := h.Next(); ok {
		t.Fatalf("Next past bottom should return ok=false")
	}
}

func TestHistoryEmptyPrev(t *testing.T) {
	var h History
	if _, ok := h.Prev("anything"); ok {
		t.Fatalf("empty history Prev should be ok=false")
	}
}

func TestHistoryRingWraps(t *testing.T) {
	var h History
	for i := 0; i < historyCap+5; i++ {
		h.Add(string(rune('a' + (i % 26))))
	}
	if h.Len() != historyCap {
		t.Fatalf("ring must cap at %d, got %d", historyCap, h.Len())
	}
}

func TestHistoryAddResetsNavigation(t *testing.T) {
	var h History
	h.Add("a")
	h.Add("b")
	if v, _ := h.Prev("d"); v != "b" {
		t.Fatalf("Prev = %q, want b", v)
	}
	h.Add("c")
	// After Add, navigation is back at the bottom; first Prev returns newest.
	if v, ok := h.Prev("d2"); !ok || v != "c" {
		t.Fatalf("Prev after Add = (%q,%v), want (c,true)", v, ok)
	}
}
