package combat

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/chargen"
)

// fakeCatalog is the minimum YAML fixture the resolver needs: one
// general feat per cadence-modifier field plus a benign background
// feat (validates that "no fields set" contributes nothing).
const fakeCatalogYAML = `
- id: blademaster
  name: Blademaster
  background: false
  description: x
  weapon_weight_penalty_mul: 0.5
- id: light_step
  name: Light Step
  background: false
  description: x
  armor_weight_penalty_mul: 0.5
- id: endurance
  name: Endurance
  background: false
  description: x
  stamina_cost_mul: 0.8
- id: iron_constitution
  name: Iron Constitution
  background: false
  description: x
  stamina_regen_add: 1
- id: blooded
  name: Blooded
  background: true
  backgrounds: [borderlander]
  description: x
`

const fakeBackgroundsYAML = `
- id: borderlander
  name: Borderland
  race: human
  home_language: x
  bonus_feats: []
  background_skills: []
  equipment_options:
    - label: Default
      items: []
  description: x
`

const fakeClassesYAML = `
- id: armsman
  name: Armsman
  abbrev: ARM
  hit_die: 10
  bab: high
  save_fort: high
  save_ref: low
  save_will: low
  skill_points: 2
  class_skills: []
  key_abilities: [Str]
  description: x
`

const fakeSkillsYAML = `
- id: spot
  name: Spot
  ability: Wis
`

const fakeWeavesYAML = `
- id: spark
  name: Spark
  level: 0
  power: Fire
`

const fakeItemsYAML = `
- id: dummy
  name: dummy
  short: dummy
  type: trash
  weight: 0
  value: 0gc
  quality: normal
  flags: []
  stats: {}
`

// loadFakeCatalog stands up a chargen.Catalog from the minimal
// fixtures above. Helper for the resolver tests.
func loadFakeCatalog(t *testing.T) *chargen.Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"feats.yaml":       {Data: []byte(fakeCatalogYAML)},
		"backgrounds.yaml": {Data: []byte(fakeBackgroundsYAML)},
		"classes.yaml":     {Data: []byte(fakeClassesYAML)},
		"skills.yaml":      {Data: []byte(fakeSkillsYAML)},
		"weaves.yaml":      {Data: []byte(fakeWeavesYAML)},
		"items.yaml":       {Data: []byte(fakeItemsYAML)},
	}
	cat, err := chargen.Load(fs.FS(fsys))
	if err != nil {
		t.Fatalf("load fake catalog: %v", err)
	}
	return cat
}

func TestIdentityFeatModifiers(t *testing.T) {
	fm := IdentityFeatModifiers()
	if fm.WeaponWeightPenaltyMul != 1.0 || fm.ArmorWeightPenaltyMul != 1.0 || fm.StaminaCostMul != 1.0 {
		t.Errorf("identity *Mul fields = (%.2f, %.2f, %.2f); want all 1.0",
			fm.WeaponWeightPenaltyMul, fm.ArmorWeightPenaltyMul, fm.StaminaCostMul)
	}
	if fm.StaminaRegenAdd != 0 {
		t.Errorf("identity StaminaRegenAdd = %d; want 0", fm.StaminaRegenAdd)
	}
	if len(fm.Active) != 0 {
		t.Errorf("identity Active = %v; want empty", fm.Active)
	}
}

// isIdentity is a small helper since FeatModifiers carries a slice
// and isn't comparable with ==. Checks the scalar fields and asserts
// Active is empty.
func isIdentity(fm FeatModifiers) bool {
	return fm.WeaponWeightPenaltyMul == 1.0 &&
		fm.ArmorWeightPenaltyMul == 1.0 &&
		fm.StaminaCostMul == 1.0 &&
		fm.StaminaRegenAdd == 0 &&
		len(fm.Active) == 0
}

func TestResolveFeatModifiers_NilCatalog(t *testing.T) {
	fm := ResolveFeatModifiers([]int32{chargen.HashID("blademaster")}, nil)
	if !isIdentity(fm) {
		t.Errorf("nil catalog → fm = %+v; want identity", fm)
	}
}

func TestResolveFeatModifiers_EmptyFeats(t *testing.T) {
	cat := loadFakeCatalog(t)
	fm := ResolveFeatModifiers(nil, cat)
	if !isIdentity(fm) {
		t.Errorf("empty feats → fm = %+v; want identity", fm)
	}
}

func TestResolveFeatModifiers_UnknownHashSkipped(t *testing.T) {
	cat := loadFakeCatalog(t)
	fm := ResolveFeatModifiers([]int32{12345}, cat)
	if !isIdentity(fm) {
		t.Errorf("unknown hash → fm = %+v; want identity", fm)
	}
}

func TestResolveFeatModifiers_SingleFeat(t *testing.T) {
	cat := loadFakeCatalog(t)
	cases := []struct {
		feat string
		want FeatModifiers
	}{
		{"blademaster", FeatModifiers{
			WeaponWeightPenaltyMul: 0.5, ArmorWeightPenaltyMul: 1.0, StaminaCostMul: 1.0,
			Active: []string{"Blademaster"},
		}},
		{"light_step", FeatModifiers{
			WeaponWeightPenaltyMul: 1.0, ArmorWeightPenaltyMul: 0.5, StaminaCostMul: 1.0,
			Active: []string{"Light Step"},
		}},
		{"endurance", FeatModifiers{
			WeaponWeightPenaltyMul: 1.0, ArmorWeightPenaltyMul: 1.0, StaminaCostMul: 0.8,
			Active: []string{"Endurance"},
		}},
		{"iron_constitution", FeatModifiers{
			WeaponWeightPenaltyMul: 1.0, ArmorWeightPenaltyMul: 1.0, StaminaCostMul: 1.0,
			StaminaRegenAdd: 1, Active: []string{"Iron Constitution"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.feat, func(t *testing.T) {
			fm := ResolveFeatModifiers([]int32{chargen.HashID(tc.feat)}, cat)
			if fm.WeaponWeightPenaltyMul != tc.want.WeaponWeightPenaltyMul ||
				fm.ArmorWeightPenaltyMul != tc.want.ArmorWeightPenaltyMul ||
				fm.StaminaCostMul != tc.want.StaminaCostMul ||
				fm.StaminaRegenAdd != tc.want.StaminaRegenAdd {
				t.Errorf("fm = %+v; want %+v", fm, tc.want)
			}
			if len(fm.Active) != 1 || fm.Active[0] != tc.want.Active[0] {
				t.Errorf("Active = %v; want %v", fm.Active, tc.want.Active)
			}
		})
	}
}

func TestResolveFeatModifiers_Stacks(t *testing.T) {
	cat := loadFakeCatalog(t)
	feats := []int32{
		chargen.HashID("blademaster"),
		chargen.HashID("light_step"),
		chargen.HashID("endurance"),
		chargen.HashID("iron_constitution"),
	}
	fm := ResolveFeatModifiers(feats, cat)
	if fm.WeaponWeightPenaltyMul != 0.5 {
		t.Errorf("weapon mul = %.2f; want 0.5", fm.WeaponWeightPenaltyMul)
	}
	if fm.ArmorWeightPenaltyMul != 0.5 {
		t.Errorf("armor mul = %.2f; want 0.5", fm.ArmorWeightPenaltyMul)
	}
	if fm.StaminaCostMul != 0.8 {
		t.Errorf("stamina cost mul = %.2f; want 0.8", fm.StaminaCostMul)
	}
	if fm.StaminaRegenAdd != 1 {
		t.Errorf("regen add = %d; want 1", fm.StaminaRegenAdd)
	}
	if len(fm.Active) != 4 {
		t.Errorf("Active = %v; want 4 entries", fm.Active)
	}
}

func TestResolveFeatModifiers_BenignFeatNotInActive(t *testing.T) {
	cat := loadFakeCatalog(t)
	// Blooded sets none of the cadence fields — must NOT show up in
	// Active even though it's a known hash.
	fm := ResolveFeatModifiers([]int32{chargen.HashID("blooded")}, cat)
	if len(fm.Active) != 0 {
		t.Errorf("blooded should not contribute; Active = %v", fm.Active)
	}
}

func TestApplyFeatGearAttenuation_PenaltyOnly(t *testing.T) {
	g := GearFactors{WeaponFactor: 1.50, ArmorFactor: 1.30}
	fm := FeatModifiers{WeaponWeightPenaltyMul: 0.5, ArmorWeightPenaltyMul: 1.0, StaminaCostMul: 1.0}
	out := ApplyFeatGearAttenuation(g, fm)
	// Weapon: 1 + (1.5-1)*0.5 = 1.25
	if out.WeaponFactor != 1.25 {
		t.Errorf("weapon factor = %.3f; want 1.25", out.WeaponFactor)
	}
	if out.ArmorFactor != 1.30 {
		t.Errorf("armor factor = %.3f; want 1.30", out.ArmorFactor)
	}
}

func TestApplyFeatGearAttenuation_BonusFactorUnchanged(t *testing.T) {
	g := GearFactors{WeaponFactor: 0.80, ArmorFactor: 1.30}
	fm := FeatModifiers{WeaponWeightPenaltyMul: 0.5, ArmorWeightPenaltyMul: 1.0, StaminaCostMul: 1.0}
	out := ApplyFeatGearAttenuation(g, fm)
	// Sub-1.0 (light weapon) must pass through untouched.
	if out.WeaponFactor != 0.80 {
		t.Errorf("light weapon factor = %.3f; want 0.80 (unchanged)", out.WeaponFactor)
	}
}

func TestApplyFeatGearAttenuation_IdentityNoop(t *testing.T) {
	g := GearFactors{WeaponFactor: 1.50, ArmorFactor: 1.30}
	out := ApplyFeatGearAttenuation(g, IdentityFeatModifiers())
	if out.WeaponFactor != 1.50 || out.ArmorFactor != 1.30 {
		t.Errorf("identity attenuation altered factors: (%.2f, %.2f)",
			out.WeaponFactor, out.ArmorFactor)
	}
}

func TestApplyFeatGearAttenuation_ZeroMulIdentity(t *testing.T) {
	// Defensive: a zero modifier (e.g. caller forgot to seed identity)
	// must not collapse the factor to 1.0.
	g := GearFactors{WeaponFactor: 1.50, ArmorFactor: 1.30}
	fm := FeatModifiers{}
	out := ApplyFeatGearAttenuation(g, fm)
	if out.WeaponFactor != 1.50 || out.ArmorFactor != 1.30 {
		t.Errorf("zero-mul attenuation altered factors: (%.2f, %.2f)",
			out.WeaponFactor, out.ArmorFactor)
	}
}

func TestCatalog_FeatByHashedID(t *testing.T) {
	cat := loadFakeCatalog(t)
	want := chargen.HashID("blademaster")
	f := cat.FeatByHashedID(want)
	if f == nil {
		t.Fatalf("FeatByHashedID(%d) = nil; want blademaster", want)
	}
	if f.ID != "blademaster" {
		t.Errorf("FeatByHashedID returned %q; want blademaster", f.ID)
	}
	if cat.FeatByHashedID(12345) != nil {
		t.Errorf("FeatByHashedID(12345) should be nil for unknown hash")
	}
	var nilCat *chargen.Catalog
	if nilCat.FeatByHashedID(want) != nil {
		t.Errorf("FeatByHashedID on nil receiver should return nil")
	}
}

func TestCatalog_Feat_CadenceFieldsRoundTrip(t *testing.T) {
	cat := loadFakeCatalog(t)
	bm, ok := cat.Feat("blademaster")
	if !ok {
		t.Fatalf("blademaster missing from fake catalog")
	}
	if bm.WeaponWeightPenaltyMul != 0.5 {
		t.Errorf("WeaponWeightPenaltyMul = %.2f; want 0.5", bm.WeaponWeightPenaltyMul)
	}
	ic, _ := cat.Feat("iron_constitution")
	if ic.StaminaRegenAdd != 1 {
		t.Errorf("StaminaRegenAdd = %d; want 1", ic.StaminaRegenAdd)
	}
}

func TestCatalog_RejectsOutOfRangeMul(t *testing.T) {
	bad := strings.ReplaceAll(fakeCatalogYAML,
		"weapon_weight_penalty_mul: 0.5", "weapon_weight_penalty_mul: 3.0")
	fsys := fstest.MapFS{
		"feats.yaml":       {Data: []byte(bad)},
		"backgrounds.yaml": {Data: []byte(fakeBackgroundsYAML)},
		"classes.yaml":     {Data: []byte(fakeClassesYAML)},
		"skills.yaml":      {Data: []byte(fakeSkillsYAML)},
		"weaves.yaml":      {Data: []byte(fakeWeavesYAML)},
		"items.yaml":       {Data: []byte(fakeItemsYAML)},
	}
	_, err := chargen.Load(fs.FS(fsys))
	if err == nil {
		t.Fatalf("expected validation error for out-of-range mul")
	}
	if !strings.Contains(err.Error(), "weapon_weight_penalty_mul") {
		t.Errorf("error %v should mention weapon_weight_penalty_mul", err)
	}
}
