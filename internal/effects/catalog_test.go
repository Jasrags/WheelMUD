package effects

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
)

func TestLoad_EmbeddedDefault(t *testing.T) {
	root, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS: %v", err)
	}
	cat, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, want := range []string{"healing_draught", "weak_poison", "bull_strength"} {
		if _, ok := cat.Get(want); !ok {
			t.Fatalf("expected catalog entry %q in %v", want, cat.IDs())
		}
	}
}

func TestIDForHash_Roundtrip(t *testing.T) {
	root, err := SourceFS()
	if err != nil {
		t.Fatalf("SourceFS: %v", err)
	}
	cat, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const id = "healing_draught"
	got, ok := cat.IDForHash(chargen.HashID(id))
	if !ok || got != id {
		t.Fatalf("IDForHash round-trip: ok=%v got=%q", ok, got)
	}
}

func TestEffect_ToAffect(t *testing.T) {
	e := Effect{
		ID: "test", Name: "Test", DurationTicks: 5,
		Modifiers:     []creature.StatMod{{Field: "Defense", Delta: 2}},
		ConditionMask: creature.CondBlinded,
		TickEffect:    "poison", TickDamage: -3,
	}
	a := e.ToAffect(42)
	if a.Source != 42 || a.Name != "Test" || a.DurationTicks != 5 ||
		a.TickEffect != "poison" || a.TickDamage != -3 ||
		a.ConditionMask != creature.CondBlinded ||
		len(a.Modifiers) != 1 || a.Modifiers[0].Delta != 2 {
		t.Fatalf("ToAffect: %+v", a)
	}
	// mutating result must not touch the source
	a.Modifiers[0].Delta = 99
	if e.Modifiers[0].Delta != 2 {
		t.Fatalf("ToAffect aliased Modifiers slice")
	}
}

func loadFromYAML(t *testing.T, body string) (*Catalog, error) {
	t.Helper()
	fsys := fstest.MapFS{"effects.yaml": &fstest.MapFile{Data: []byte(body)}}
	return Load(fsys)
}

func TestValidate_RejectsDuplicateID(t *testing.T) {
	body := `effects:
  - id: dup
    name: A
    duration_ticks: 3
  - id: dup
    name: B
    duration_ticks: 3
`
	_, err := loadFromYAML(t, body)
	if err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("want duplicate-id error, got %v", err)
	}
}

func TestValidate_RejectsBlankName(t *testing.T) {
	body := `effects:
  - id: a
    name: ""
    duration_ticks: 3
`
	_, err := loadFromYAML(t, body)
	if err == nil || !strings.Contains(err.Error(), "blank Name") {
		t.Fatalf("want blank-name error, got %v", err)
	}
}

func TestValidate_RejectsZeroDuration(t *testing.T) {
	body := `effects:
  - id: a
    name: A
    duration_ticks: 0
`
	_, err := loadFromYAML(t, body)
	if err == nil || !strings.Contains(err.Error(), "DurationTicks") {
		t.Fatalf("want duration error, got %v", err)
	}
}

func TestValidate_RejectsUnknownStatMod(t *testing.T) {
	body := `effects:
  - id: a
    name: A
    duration_ticks: 3
    modifiers:
      - field: Nonsense.Field
        delta: 1
`
	_, err := loadFromYAML(t, body)
	if err == nil || !strings.Contains(err.Error(), "StatMod field") {
		t.Fatalf("want statmod error, got %v", err)
	}
}

func TestValidate_RejectsTickDamageWithoutTickEffect(t *testing.T) {
	body := `effects:
  - id: a
    name: A
    duration_ticks: 3
    tick_damage: -2
`
	_, err := loadFromYAML(t, body)
	if err == nil || !strings.Contains(err.Error(), "TickEffect") {
		t.Fatalf("want TickEffect mismatch error, got %v", err)
	}
}
