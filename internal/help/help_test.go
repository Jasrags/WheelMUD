package help

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoad_EmbeddedTopics(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := make([]string, 0, len(cat.All()))
	for _, tp := range cat.All() {
		got = append(got, tp.ID)
	}
	want := []string{"channeling", "combat", "currency"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("topic ids = %v, want %v (sorted)", got, want)
	}
}

func TestParseFrontMatter(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantErr   string // substring; "" means success
		wantID    string
		wantTitle string
		wantKW    []string
		wantBody  string
	}{
		{
			name:   "ok minimal",
			raw:    "---\nid: foo\ntitle: Foo\n---\nbody\n",
			wantID: "foo", wantTitle: "Foo", wantBody: "body\n",
		},
		{
			name:   "ok keywords",
			raw:    "---\nid: combat\ntitle: Combat\nkeywords: fight, attack ,KILL\n---\nb\n",
			wantID: "combat", wantTitle: "Combat",
			wantKW:   []string{"fight", "attack", "kill"},
			wantBody: "b\n",
		},
		{
			name:    "missing fence",
			raw:     "id: foo\ntitle: Foo\n",
			wantErr: "missing front-matter fence",
		},
		{
			name:    "unterminated fence",
			raw:     "---\nid: foo\ntitle: Foo\nbody...",
			wantErr: "unterminated",
		},
		{
			name:    "missing id",
			raw:     "---\ntitle: Foo\n---\nb",
			wantErr: "missing id",
		},
		{
			name:    "missing title",
			raw:     "---\nid: foo\n---\nb",
			wantErr: "missing title",
		},
		{
			name:    "unknown key",
			raw:     "---\nid: foo\ntitle: Foo\nbogus: 1\n---\nb",
			wantErr: "unknown front-matter key",
		},
		{
			name:    "bad id charclass",
			raw:     "---\nid: Foo Bar\ntitle: x\n---\nb",
			wantErr: "invalid topic id",
		},
		{
			name:    "bad keyword charclass",
			raw:     "---\nid: foo\ntitle: x\nkeywords: ok, BAD CASE\n---\nb",
			wantErr: "invalid keyword",
		},
		{
			name:    "bad line",
			raw:     "---\nid foo\ntitle: x\n---\nb",
			wantErr: "bad front-matter line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := parseFrontMatter(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tp.ID != tt.wantID {
				t.Errorf("id = %q, want %q", tp.ID, tt.wantID)
			}
			if tp.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", tp.Title, tt.wantTitle)
			}
			if strings.Join(tp.Keywords, ",") != strings.Join(tt.wantKW, ",") {
				t.Errorf("keywords = %v, want %v", tp.Keywords, tt.wantKW)
			}
			if tp.Body != tt.wantBody {
				t.Errorf("body = %q, want %q", tp.Body, tt.wantBody)
			}
		})
	}
}

// catalogFromTopics builds a Catalog directly from in-memory topics,
// bypassing the embedded fs. Reuses Load's validateAndIndex helper so
// collision rules stay in sync automatically.
func catalogFromTopics(t *testing.T, topics []*Topic) (*Catalog, error) {
	t.Helper()
	cp := append([]*Topic(nil), topics...)
	byID, byKeyword, err := validateAndIndex(cp)
	if err != nil {
		return nil, err
	}
	return &Catalog{sorted: cp, byID: byID, byKeyword: byKeyword}, nil
}

func mustCatalog(t *testing.T, topics []*Topic) *Catalog {
	t.Helper()
	c, err := catalogFromTopics(t, topics)
	if err != nil {
		t.Fatalf("catalogFromTopics: %v", err)
	}
	return c
}

func TestLookup(t *testing.T) {
	c := mustCatalog(t, []*Topic{
		{ID: "channeling", Title: "Channeling", Keywords: []string{"weave"}, Body: "x"},
		{ID: "channels", Title: "Channels", Body: "x"},
		{ID: "combat", Title: "Combat", Keywords: []string{"fight"}, Body: "x"},
	})

	tests := []struct {
		q       string
		wantID  string
		wantErr error
	}{
		{q: "combat", wantID: "combat"},         // exact id
		{q: "fight", wantID: "combat"},          // keyword
		{q: "weave", wantID: "channeling"},      // keyword
		{q: "comb", wantID: "combat"},           // unique prefix
		{q: "channeli", wantID: "channeling"},   // unique prefix
		{q: "ch", wantErr: ErrAmbiguousTopic},   // ambiguous
		{q: "nosuch", wantErr: ErrUnknownTopic}, // unknown
		{q: "", wantErr: ErrUnknownTopic},       // empty
	}
	for _, tt := range tests {
		t.Run(tt.q, func(t *testing.T) {
			got, err := c.Lookup(tt.q)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.ID != tt.wantID {
				t.Errorf("id = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestLookup_AmbiguousErrorListsIDs(t *testing.T) {
	c := mustCatalog(t, []*Topic{
		{ID: "channeling", Title: "x"},
		{ID: "channels", Title: "x"},
	})
	_, err := c.Lookup("ch")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "channeling") || !strings.Contains(msg, "channels") {
		t.Errorf("error %q should list both ids", msg)
	}
	if !strings.Contains(msg, ", ") {
		t.Errorf("error %q should comma-join", msg)
	}
}

func TestPrefix(t *testing.T) {
	c := mustCatalog(t, []*Topic{
		{ID: "alpha", Title: "x"},
		{ID: "alphabet", Title: "x"},
		{ID: "beta", Title: "x"},
	})
	got := c.Prefix("alp")
	if len(got) != 2 || got[0].ID != "alpha" || got[1].ID != "alphabet" {
		t.Errorf("Prefix(alp) = %+v", ids(got))
	}
	if all := c.Prefix(""); len(all) != 3 {
		t.Errorf("Prefix(\"\") count = %d, want 3", len(all))
	}
	if none := c.Prefix("zzz"); len(none) != 0 {
		t.Errorf("Prefix(zzz) = %+v, want empty", ids(none))
	}
}

func TestLookupExact_AndKeyword(t *testing.T) {
	c := mustCatalog(t, []*Topic{
		{ID: "combat", Title: "x", Keywords: []string{"fight"}},
	})
	if _, ok := c.LookupExact("combat"); !ok {
		t.Errorf("LookupExact(combat) = false")
	}
	if _, ok := c.LookupExact("fight"); ok {
		t.Errorf("LookupExact(fight) = true; should not match keyword")
	}
	if _, ok := c.LookupExact("comb"); ok {
		t.Errorf("LookupExact(comb) = true; should not match prefix")
	}
	if _, ok := c.LookupKeyword("fight"); !ok {
		t.Errorf("LookupKeyword(fight) = false")
	}
	if _, ok := c.LookupKeyword("combat"); ok {
		t.Errorf("LookupKeyword(combat) = true; should not match id")
	}
}

func TestValidTopicID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"lowercase", "combat", true},
		{"with dash", "combat-basics", true},
		{"with digits", "v2", true},
		{"uppercase", "Combat", false},
		{"space", "two words", false},
		{"tab", "a\tb", false},
		{"nul", "a\x00b", false},
		{"del 0x7F", "a\x7fb", false},
		{"high bit 0x80", "a\x80b", false},
		{"non-ascii utf-8", "café", false},
		{"low control 0x01", "a\x01b", false},
		{"carriage return", "a\rb", false},
		{"newline", "a\nb", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validTopicID(tt.in); got != tt.want {
				t.Errorf("validTopicID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func ids(ts []*Topic) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

func TestReloadFS_AddedRemovedChanged(t *testing.T) {
	v1 := fstest.MapFS{
		"topics/alpha.md": &fstest.MapFile{Data: []byte("---\nid: alpha\ntitle: A1\n---\nfirst\n")},
		"topics/bravo.md": &fstest.MapFile{Data: []byte("---\nid: bravo\ntitle: B\n---\nbody\n")},
	}
	cat, err := LoadFS(v1)
	if err != nil {
		t.Fatalf("LoadFS v1: %v", err)
	}

	// v2: alpha changed body, bravo unchanged, charlie added.
	v2 := fstest.MapFS{
		"topics/alpha.md":   &fstest.MapFile{Data: []byte("---\nid: alpha\ntitle: A2\n---\nfirst\n")},
		"topics/bravo.md":   &fstest.MapFile{Data: []byte("---\nid: bravo\ntitle: B\n---\nbody\n")},
		"topics/charlie.md": &fstest.MapFile{Data: []byte("---\nid: charlie\ntitle: C\n---\nbody\n")},
	}
	added, removed, changed, err := cat.ReloadFS(v2)
	if err != nil {
		t.Fatalf("ReloadFS v2: %v", err)
	}
	if added != 1 {
		t.Errorf("added=%d want 1", added)
	}
	if removed != 0 {
		t.Errorf("removed=%d want 0", removed)
	}
	if changed != 1 {
		t.Errorf("changed=%d want 1", changed)
	}
	// New content is live.
	if got, ok := cat.LookupExact("alpha"); !ok || got.Title != "A2" {
		t.Errorf("alpha post-reload Title=%q ok=%v want A2", got, ok)
	}
	if _, ok := cat.LookupExact("charlie"); !ok {
		t.Errorf("charlie missing after reload")
	}
}

func TestReloadFS_RemovesOldTopic(t *testing.T) {
	v1 := fstest.MapFS{
		"topics/alpha.md": &fstest.MapFile{Data: []byte("---\nid: alpha\ntitle: A\n---\nbody\n")},
		"topics/bravo.md": &fstest.MapFile{Data: []byte("---\nid: bravo\ntitle: B\n---\nbody\n")},
	}
	cat, err := LoadFS(v1)
	if err != nil {
		t.Fatal(err)
	}
	v2 := fstest.MapFS{
		"topics/alpha.md": &fstest.MapFile{Data: []byte("---\nid: alpha\ntitle: A\n---\nbody\n")},
	}
	added, removed, changed, err := cat.ReloadFS(v2)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 || changed != 0 || removed != 1 {
		t.Errorf("counts add=%d rm=%d ch=%d; want 0/1/0", added, removed, changed)
	}
	if _, ok := cat.LookupExact("bravo"); ok {
		t.Errorf("bravo should be gone after reload")
	}
}

func TestReloadFS_ParseErrorLeavesCatalogIntact(t *testing.T) {
	v1 := fstest.MapFS{
		"topics/alpha.md": &fstest.MapFile{Data: []byte("---\nid: alpha\ntitle: A\n---\nbody\n")},
	}
	cat, err := LoadFS(v1)
	if err != nil {
		t.Fatal(err)
	}
	bad := fstest.MapFS{
		"topics/oops.md": &fstest.MapFile{Data: []byte("no front matter here\n")},
	}
	_, _, _, err = cat.ReloadFS(bad)
	if err == nil {
		t.Fatalf("expected parse error on malformed reload")
	}
	// Pre-reload state must survive.
	if _, ok := cat.LookupExact("alpha"); !ok {
		t.Errorf("alpha should still exist after failed reload")
	}
}

func TestReloadFS_ConcurrentReadsAreSafe(t *testing.T) {
	v1 := fstest.MapFS{
		"topics/alpha.md": &fstest.MapFile{Data: []byte("---\nid: alpha\ntitle: A\n---\nbody\n")},
	}
	cat, err := LoadFS(v1)
	if err != nil {
		t.Fatal(err)
	}
	v2 := fstest.MapFS{
		"topics/alpha.md": &fstest.MapFile{Data: []byte("---\nid: alpha\ntitle: A\n---\nedited\n")},
		"topics/beta.md":  &fstest.MapFile{Data: []byte("---\nid: beta\ntitle: B\n---\nnew\n")},
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			_ = cat.All()
			_, _ = cat.LookupExact("alpha")
		}
		close(done)
	}()
	for i := 0; i < 50; i++ {
		if _, _, _, err := cat.ReloadFS(v2); err != nil {
			t.Errorf("ReloadFS: %v", err)
		}
	}
	<-done
}
