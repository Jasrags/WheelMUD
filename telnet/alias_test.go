package telnet

import "testing"

func TestAliasTableSetLookup(t *testing.T) {
	tbl := NewAliasTable()
	if !tbl.Set("ll", "look") {
		t.Fatal("Set should succeed")
	}
	if v, ok := tbl.Lookup("ll"); !ok || v != "look" {
		t.Fatalf("Lookup = (%q,%v), want (look,true)", v, ok)
	}
	// Case-insensitive lookup.
	if v, ok := tbl.Lookup("LL"); !ok || v != "look" {
		t.Fatalf("Lookup uppercase = (%q,%v)", v, ok)
	}
}

func TestAliasTableRejectsInvalid(t *testing.T) {
	tbl := NewAliasTable()
	if tbl.Set("", "look") {
		t.Fatal("empty name must be rejected")
	}
	if tbl.Set("ll", "") {
		t.Fatal("empty expansion must be rejected")
	}
	if tbl.Set("has space", "look") {
		t.Fatal("name with whitespace must be rejected")
	}
	if tbl.Set("UPPER", "look") {
		// Set lowercases and revalidates — "upper" is valid, but a
		// caller giving uppercase is a smell. The current contract
		// accepts it after lowercasing; document by asserting it works.
		if v, ok := tbl.Lookup("upper"); !ok || v != "look" {
			t.Fatalf("expected lowercased entry, got (%q,%v)", v, ok)
		}
	}
}

func TestAliasTableDelete(t *testing.T) {
	tbl := NewAliasTable()
	tbl.Set("ll", "look")
	if !tbl.Delete("ll") {
		t.Fatal("Delete should report true")
	}
	if _, ok := tbl.Lookup("ll"); ok {
		t.Fatal("entry should be gone after Delete")
	}
	if tbl.Delete("nothing") {
		t.Fatal("Delete missing should report false")
	}
}

func TestAliasExpandOneLevel(t *testing.T) {
	tbl := NewAliasTable()
	tbl.Set("ll", "look")
	tbl.Set("l", "ll") // chained — must NOT recurse

	if got := expandAlias(tbl, "ll north"); got != "look north" {
		t.Fatalf("expandAlias = %q, want %q", got, "look north")
	}
	if got := expandAlias(tbl, "ll"); got != "look" {
		t.Fatalf("expandAlias bare = %q, want %q", got, "look")
	}
	// Chain stops after one expansion: `l` → `ll` (NOT `look`).
	if got := expandAlias(tbl, "l"); got != "ll" {
		t.Fatalf("alias-of-alias should stop at one level, got %q", got)
	}
}

func TestAliasExpandNilTable(t *testing.T) {
	if got := expandAlias(nil, "look north"); got != "look north" {
		t.Fatalf("nil table must pass through, got %q", got)
	}
}

func TestAliasExpandUnknown(t *testing.T) {
	tbl := NewAliasTable()
	if got := expandAlias(tbl, "look north"); got != "look north" {
		t.Fatalf("unknown verb must pass through, got %q", got)
	}
}

func TestAliasTableAll(t *testing.T) {
	tbl := NewAliasTable()
	tbl.Set("z", "zip")
	tbl.Set("a", "apple")
	tbl.Set("m", "mike")
	names, exps := tbl.All()
	want := []string{"a", "m", "z"}
	for i, n := range names {
		if n != want[i] {
			t.Fatalf("names[%d] = %q, want %q (sorted)", i, n, want[i])
		}
	}
	if exps[0] != "apple" || exps[1] != "mike" || exps[2] != "zip" {
		t.Fatalf("exps = %v, want [apple mike zip]", exps)
	}
}
