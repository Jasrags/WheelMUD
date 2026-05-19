package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const wizdemoYAML = `id: wizdemo
entry: name
steps:
  - id: name
    kind: text
    prompt: What is your name?
    store_as: name
    next: confirm
  - id: confirm
    kind: confirm
    prompt: Commit?
    on_yes: ""
    on_no: ""
`

func TestLoad_EmbeddedDefaults(t *testing.T) {
	fsys, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS: %v", err)
	}
	cat, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load embedded: %v", err)
	}
	if cat.Get("wizdemo") == nil {
		t.Fatalf("wizdemo flow missing from embedded catalog")
	}
}

func TestLoad_TaggedUnionAllKinds(t *testing.T) {
	yaml := `id: demo
entry: t
steps:
  - id: t
    kind: text
    prompt: hi
    next: c
  - id: c
    kind: choice
    prompt: pick
    options:
      - { label: A, value: a }
    next: f
  - id: f
    kind: confirm
    prompt: ok?
    on_yes: ""
`
	cat, err := Load(fstest.MapFS{
		"demo.yaml": &fstest.MapFile{Data: []byte(yaml)},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fl := cat.Get("demo")
	if fl == nil {
		t.Fatal("demo flow missing")
	}
	if _, ok := fl.Step("t").(*TextStep); !ok {
		t.Errorf("t should be *TextStep, got %T", fl.Step("t"))
	}
	if _, ok := fl.Step("c").(*ChoiceStep); !ok {
		t.Errorf("c should be *ChoiceStep, got %T", fl.Step("c"))
	}
	if _, ok := fl.Step("f").(*ConfirmStep); !ok {
		t.Errorf("f should be *ConfirmStep, got %T", fl.Step("f"))
	}
}

func TestLoad_UnknownStepKind(t *testing.T) {
	yaml := `id: x
entry: a
steps:
  - id: a
    kind: ouija
    prompt: "?"
`
	_, err := Load(fstest.MapFS{"x.yaml": &fstest.MapFile{Data: []byte(yaml)}})
	if err == nil || !strings.Contains(err.Error(), "unknown step kind") {
		t.Fatalf("want unknown-step-kind error, got %v", err)
	}
}

func TestLoad_MissingKindField(t *testing.T) {
	yaml := `id: x
entry: a
steps:
  - id: a
    prompt: "?"
`
	_, err := Load(fstest.MapFS{"x.yaml": &fstest.MapFile{Data: []byte(yaml)}})
	if err == nil || !strings.Contains(err.Error(), "missing required `kind:`") {
		t.Fatalf("want missing-kind error, got %v", err)
	}
}

func TestLoad_DuplicateFlowID(t *testing.T) {
	_, err := Load(fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(wizdemoYAML)},
		"b.yaml": &fstest.MapFile{Data: []byte(wizdemoYAML)},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate flow id") {
		t.Fatalf("want dup-id error, got %v", err)
	}
}

func TestLoad_FlowValidateErrorPropagates(t *testing.T) {
	yaml := `id: bad
entry: zzz
steps:
  - id: a
    kind: text
    prompt: hi
`
	_, err := Load(fstest.MapFS{"bad.yaml": &fstest.MapFile{Data: []byte(yaml)}})
	if err == nil || !strings.Contains(err.Error(), "not in Steps") {
		t.Fatalf("want entry-not-in-steps error, got %v", err)
	}
}

func TestLoad_PerOptionNext(t *testing.T) {
	yaml := `id: branch
entry: pick
steps:
  - id: pick
    kind: choice
    prompt: "?"
    next: fallback
    options:
      - { label: One, value: one, next: branchOne }
      - { label: Two, value: two }
  - id: fallback
    kind: text
    prompt: fb
    next: ""
  - id: branchOne
    kind: text
    prompt: b1
    next: ""
`
	cat, err := Load(fstest.MapFS{"b.yaml": &fstest.MapFile{Data: []byte(yaml)}})
	if err != nil {
		t.Fatal(err)
	}
	pick := cat.Get("branch").Step("pick").(*ChoiceStep)
	if pick.Options[0].Next != "branchOne" {
		t.Errorf("per-option Next not preserved: %+v", pick.Options[0])
	}
	if pick.Options[1].Next != "" {
		t.Errorf("blank per-option Next should fall back to step Next: got %q", pick.Options[1].Next)
	}
}

func TestEnvOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.yaml"), []byte(wizdemoYAML), 0o644); err != nil {
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
	if cat.Get("wizdemo") == nil {
		t.Fatal("FLOW_DIR override didn't load wizdemo")
	}
}

func TestEnvOverrideMissingDir(t *testing.T) {
	t.Setenv(DirEnv, filepath.Join(t.TempDir(), "does-not-exist"))
	if _, err := SourceFS(); err == nil {
		t.Fatalf("expected error for missing FLOW_DIR")
	}
}

func TestCatalogIDsSorted(t *testing.T) {
	cat, err := Load(fstest.MapFS{
		"z.yaml": &fstest.MapFile{Data: []byte(`id: zeta
entry: a
steps: [{ id: a, kind: text, prompt: x }]
`)},
		"a.yaml": &fstest.MapFile{Data: []byte(`id: alpha
entry: a
steps: [{ id: a, kind: text, prompt: x }]
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := cat.IDs()
	if len(ids) != 2 || ids[0] != "alpha" || ids[1] != "zeta" {
		t.Fatalf("IDs not sorted: %v", ids)
	}
}
