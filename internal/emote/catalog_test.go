package emote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSocialTargetable(t *testing.T) {
	tests := []struct {
		name string
		s    Social
		want bool
	}{
		{"all set", Social{TargetSelf: "a", TargetView: "b", TargetOther: "c"}, true},
		{"none set", Social{}, false},
		{"partial", Social{TargetSelf: "a"}, false},
	}
	for _, tc := range tests {
		if got := tc.s.Targetable(); got != tc.want {
			t.Errorf("%s: Targetable=%v want %v", tc.name, got, tc.want)
		}
	}
}

func TestRenderHelpers(t *testing.T) {
	s := Social{
		Self:        "You smile.",
		Other:       "{actor} smiles.",
		TargetSelf:  "You smile at {target}.",
		TargetView:  "{actor} smiles at you.",
		TargetOther: "{actor} smiles at {target}.",
	}
	cases := map[string]string{
		s.RenderSelf("Bob"):                "{{You smile.}}::magenta\r\n",
		s.RenderOther("Bob"):               "{{Bob smiles.}}::magenta\r\n",
		s.RenderTargetSelf("Bob", "Alice"): "{{You smile at Alice.}}::magenta\r\n",
		s.RenderTargetView("Bob", "Alice"): "{{Bob smiles at you.}}::magenta\r\n",
		s.RenderTargetOther("Bob", "Alice"): "{{Bob smiles at Alice.}}::magenta\r\n",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("render = %q want %q", got, want)
		}
	}
}

func TestLoadEmbeddedDefaults(t *testing.T) {
	fsys, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS: %v", err)
	}
	cat, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load embedded: %v", err)
	}
	if len(cat.All()) < 10 {
		t.Fatalf("expected ≥10 embedded socials, got %d", len(cat.All()))
	}
	// Spot-check a few canonical entries are present.
	for _, id := range []string{"smile", "wave", "bow", "nod", "sigh"} {
		if _, ok := cat.Get(id); !ok {
			t.Errorf("embedded catalog missing %q", id)
		}
	}
	// Smile should be targetable; sigh should not.
	if smile, _ := cat.Get("smile"); !smile.Targetable() {
		t.Errorf("smile should be targetable")
	}
	if sigh, _ := cat.Get("sigh"); sigh.Targetable() {
		t.Errorf("sigh should not be targetable")
	}
}

func TestLoadDuplicateID(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    self: You smile.
    other: "{actor} smiles."
  - id: smile
    self: You smile.
    other: "{actor} smiles."
`)},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("want duplicate-id error, got %v", err)
	}
}

func TestLoadAliasCollidesAcrossFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    aliases: [grin]
    self: You smile.
    other: "{actor} smiles."
`)},
		"b.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: grin
    self: You grin.
    other: "{actor} grins."
`)},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("want alias-collision error, got %v", err)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    other: "{actor} smiles."
`)},
	}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "self is required") {
		t.Fatalf("want missing-self error, got %v", err)
	}
}

func TestLoadPartialTargetedSet(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    self: You smile.
    other: "{actor} smiles."
    target_self: You smile at {target}.
`)},
	}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "all be set or all blank") {
		t.Fatalf("want partial-targeted error, got %v", err)
	}
}

func TestLoadUnknownToken(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    self: You smile {foo}.
    other: "{actor} smiles."
`)},
	}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "{foo}") {
		t.Fatalf("want unknown-token error, got %v", err)
	}
}

func TestLoadTargetInUntargetedSlot(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    self: You smile at {target}.
    other: "{actor} smiles."
`)},
	}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "untargeted-only") {
		t.Fatalf("want target-in-untargeted error, got %v", err)
	}
}

func TestLoadBadID(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: "Big Smile"
    self: You smile.
    other: "{actor} smiles."
`)},
	}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "match [a-z]") {
		t.Fatalf("want bad-id error, got %v", err)
	}
}

func TestLoadAliasEqualsID(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    aliases: [smile]
    self: You smile.
    other: "{actor} smiles."
`)},
	}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "equals id") {
		t.Fatalf("want alias-equals-id error, got %v", err)
	}
}

func TestEnvOverride(t *testing.T) {
	dir := t.TempDir()
	yaml := `socials:
  - id: thinks
    self: You think.
    other: "{actor} thinks."
`
	if err := os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(DirEnv, dir)
	fsys, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS: %v", err)
	}
	cat, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cat.Get("thinks"); !ok {
		t.Fatalf("override catalog missing 'thinks'")
	}
	if _, ok := cat.Get("smile"); ok {
		t.Fatalf("override should not include embedded defaults")
	}
}

func TestEnvOverrideMissingDir(t *testing.T) {
	t.Setenv(DirEnv, filepath.Join(t.TempDir(), "does-not-exist"))
	if _, err := SourceFS(); err == nil {
		t.Fatalf("expected error for missing EMOTE_DIR")
	}
}

func TestCatalogAllIsCopy(t *testing.T) {
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    self: You smile.
    other: "{actor} smiles."
`)},
	}
	cat, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	all := cat.All()
	all[0].ID = "tampered"
	if got, _ := cat.Get("smile"); got.ID != "smile" {
		t.Fatalf("All() must not alias catalog storage; Get gave %q", got.ID)
	}
}
