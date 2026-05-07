package help

import (
	"errors"
	"strings"
	"testing"
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
// bypassing the embedded fs. Mirrors the validation Load runs after
// parsing so collision rules can be exercised without committing
// pathological fixture files.
func catalogFromTopics(t *testing.T, topics []*Topic) (*Catalog, error) {
	t.Helper()
	// Re-implement the validation half of Load on a synthetic slice.
	// This is the only place catalog construction is done outside
	// Load; if the validation rules change in Load, mirror them here.
	c := &Catalog{
		byID:      make(map[string]*Topic),
		byKeyword: make(map[string]*Topic),
	}
	cp := append([]*Topic(nil), topics...)
	c.sorted = cp
	for _, tp := range cp {
		if _, dup := c.byID[tp.ID]; dup {
			return nil, errDup("id", tp.ID)
		}
		if _, dup := c.byKeyword[tp.ID]; dup {
			return nil, errDup("id-vs-keyword", tp.ID)
		}
		c.byID[tp.ID] = tp
		for _, kw := range tp.Keywords {
			if _, dup := c.byID[kw]; dup {
				return nil, errDup("keyword-vs-id", kw)
			}
			if _, dup := c.byKeyword[kw]; dup {
				return nil, errDup("keyword", kw)
			}
			c.byKeyword[kw] = tp
		}
	}
	return c, nil
}

func errDup(kind, name string) error {
	return errors.New("dup " + kind + ": " + name)
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

func ids(ts []*Topic) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}
