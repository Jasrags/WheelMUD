package chargen

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

// liveDir loads the real ./data/chargen tree for round-trip tests.
// The seed data is the source of truth that main.go ships with;
// any breakage here breaks boot.
func liveDir(t *testing.T) *Catalog {
	t.Helper()
	cat, err := Load(os.DirFS("../../data/chargen"))
	if err != nil {
		t.Fatalf("Load(real data): %v", err)
	}
	return cat
}

func TestLoad_RealCatalog(t *testing.T) {
	cat := liveDir(t)

	// All 11 named human backgrounds present.
	for _, id := range []string{
		"aiel", "athaan_miere", "borderlander", "cairhienin",
		"domani", "ebou_dari", "illianer", "midlander",
		"taraboner", "tairen", "tar_valoner",
	} {
		bg, ok := cat.Background(id)
		if !ok {
			t.Errorf("missing background %q", id)
			continue
		}
		if bg.Race != "human" {
			t.Errorf("%s: race=%q want human", id, bg.Race)
		}
		if len(bg.EquipmentOptions) == 0 {
			t.Errorf("%s: no equipment_options", id)
		}
	}

	// All seven hero classes present.
	for _, id := range []string{
		"algai_d_siswai", "armsman", "initiate", "noble",
		"wanderer", "wilder", "woodsman",
	} {
		cl, ok := cat.Class(id)
		if !ok {
			t.Errorf("missing class %q", id)
			continue
		}
		if cl.HitDie == 0 {
			t.Errorf("%s: hit_die not set", id)
		}
	}

	// Channelers are gated correctly.
	if ini, _ := cat.Class("initiate"); !ini.Channeler {
		t.Error("initiate.Channeler want true")
	}
	if arm, _ := cat.Class("armsman"); arm.Channeler {
		t.Error("armsman.Channeler want false")
	}
}

func TestBackgroundsForRace(t *testing.T) {
	cat := liveDir(t)
	humans := cat.BackgroundsForRace("human")
	if len(humans) != 11 {
		t.Errorf("BackgroundsForRace(human) = %d, want 11", len(humans))
	}
	if got := cat.BackgroundsForRace("ogier"); len(got) != 0 {
		t.Errorf("BackgroundsForRace(ogier) = %d, want 0 (no Ogier seed yet)", len(got))
	}
}

func TestClassesForRace_OgierExcludesChannelers(t *testing.T) {
	cat := liveDir(t)
	for _, cl := range cat.ClassesForRace("ogier") {
		if cl.Channeler {
			t.Errorf("ogier should not see channeler %q", cl.ID)
		}
	}
	human := cat.ClassesForRace("human")
	if len(human) != 7 {
		t.Errorf("ClassesForRace(human) = %d, want 7", len(human))
	}
}

func TestFeatsForBackground(t *testing.T) {
	cat := liveDir(t)
	feats := cat.FeatsForBackground("aiel")
	want := map[string]bool{
		"blooded": true, "bullheaded": true, "disciplined": true,
		"stealthy": true, "survivor": true,
	}
	for _, f := range feats {
		delete(want, f.ID)
	}
	if len(want) != 0 {
		t.Errorf("aiel missing feats: %v", want)
	}
}

func TestWeavesAtLevel(t *testing.T) {
	cat := liveDir(t)
	got := cat.WeavesAtLevel(0)
	if len(got) == 0 {
		t.Fatal("no level-0 weaves seeded")
	}
	for _, w := range got {
		if w.Level != 0 {
			t.Errorf("weave %q: level=%d", w.ID, w.Level)
		}
	}
}

func TestWeavePracticeCost_LoadsAndDefaults(t *testing.T) {
	withCost := "- {id: spark, name: Spark, level: 0, power: Fire, practice_cost: 2}\n" +
		"- {id: candle, name: Candle, level: 0, power: Fire}\n"
	cat, err := Load(mapFS(minimalBackgrounds, minimalClasses, minimalFeats, minimalSkills, withCost))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	w, _ := cat.Weave("spark")
	if w.PracticeCost != 2 {
		t.Errorf("spark PracticeCost = %d, want 2", w.PracticeCost)
	}
	c, _ := cat.Weave("candle")
	if c.PracticeCost != 0 {
		t.Errorf("candle PracticeCost = %d, want 0 (default)", c.PracticeCost)
	}
}

func TestWeavePracticeCost_NegativeRejected(t *testing.T) {
	bad := "- {id: spark, name: Spark, level: 0, power: Fire, practice_cost: -1}\n"
	_, err := Load(mapFS(minimalBackgrounds, minimalClasses, minimalFeats, minimalSkills, bad))
	if err == nil || !strings.Contains(err.Error(), "practice_cost") {
		t.Errorf("want practice_cost error, got %v", err)
	}
}

func TestEnumStamping(t *testing.T) {
	cat := liveDir(t)
	bg, _ := cat.Background("midlander")
	if int(bg.Enum) < 0 {
		t.Error("midlander enum unset")
	}
	cl, _ := cat.Class("armsman")
	if int(cl.Enum) < 0 {
		t.Error("armsman enum unset")
	}
}

// --- failure-mode coverage via in-memory fstest.MapFS ---------------

const minimalSkills = `
- {id: hide, name: Hide, ability: Dex}
`
const minimalFeats = `
- {id: stealthy, name: Stealthy, background: true, backgrounds: [aiel]}
`
const minimalBackgrounds = `
- id: aiel
  name: Aiel
  race: human
  home_language: Common (Aiel)
  bonus_languages: []
  bonus_feats: [stealthy]
  background_skills: [hide]
  equipment_options:
    - {label: "buckler", items: [buckler]}
  description: ok
`
const minimalClasses = `
- id: armsman
  name: Armsman
  abbrev: Arm
  hit_die: 10
  bab: high
  save_fort: high
  save_ref: low
  save_will: low
  skill_points: 2
  class_skills: [hide]
  key_abilities: [Str]
  channeler: false
  description: ok
`
const minimalWeaves = `
- {id: spark, name: Spark, level: 0, power: Fire}
`

const minimalItems = `
- id: buckler
  name: a buckler
  short: a small steel buckler
  type: shield
  weight: 5
  value: 15mk
  stats:
    kind: buckler
    bonus: 1
    check_penalty: -1
`

func mapFS(b, c, f, s, w string) fstest.MapFS {
	return fstest.MapFS{
		fileBackgrounds: {Data: []byte(b)},
		fileClasses:     {Data: []byte(c)},
		fileFeats:       {Data: []byte(f)},
		fileSkills:      {Data: []byte(s)},
		fileWeaves:      {Data: []byte(w)},
		fileItems:       {Data: []byte(minimalItems)},
	}
}

func TestLoad_MinimalRoundtrip(t *testing.T) {
	fs := mapFS(minimalBackgrounds, minimalClasses, minimalFeats, minimalSkills, minimalWeaves)
	cat, err := Load(fs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cat.Background("aiel"); !ok {
		t.Fatal("aiel missing")
	}
}

func TestLoad_RejectsUnknownFeat(t *testing.T) {
	bg := strings.Replace(minimalBackgrounds, "[stealthy]", "[doesnotexist]", 1)
	_, err := Load(mapFS(bg, minimalClasses, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "unknown feat") {
		t.Errorf("want unknown-feat error, got %v", err)
	}
}

func TestLoad_RejectsUnknownSkill(t *testing.T) {
	bg := strings.Replace(minimalBackgrounds, "background_skills: [hide]", "background_skills: [nope]", 1)
	_, err := Load(mapFS(bg, minimalClasses, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Errorf("want unknown-skill error, got %v", err)
	}
}

func TestLoad_RejectsBadHitDie(t *testing.T) {
	c := strings.Replace(minimalClasses, "hit_die: 10", "hit_die: 7", 1)
	_, err := Load(mapFS(minimalBackgrounds, c, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "invalid hit_die") {
		t.Errorf("want hit_die error, got %v", err)
	}
}

func TestLoad_RejectsBadRace(t *testing.T) {
	bg := strings.Replace(minimalBackgrounds, "race: human", "race: trolloc", 1)
	_, err := Load(mapFS(bg, minimalClasses, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "invalid race") {
		t.Errorf("want race error, got %v", err)
	}
}

func TestLoad_RejectsChannelerWithoutSource(t *testing.T) {
	c := strings.Replace(minimalClasses, "channeler: false", "channeler: true", 1)
	_, err := Load(mapFS(minimalBackgrounds, c, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "channel_source") {
		t.Errorf("want channel_source error, got %v", err)
	}
}

func TestLoad_RejectsDuplicateID(t *testing.T) {
	c := minimalClasses + minimalClasses
	_, err := Load(mapFS(minimalBackgrounds, c, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("want duplicate-id error, got %v", err)
	}
}

func TestLoad_RejectsMissingFile(t *testing.T) {
	fs := mapFS(minimalBackgrounds, minimalClasses, minimalFeats, minimalSkills, minimalWeaves)
	delete(fs, fileWeaves)
	_, err := Load(fs)
	if err == nil || !strings.Contains(err.Error(), "weaves.yaml") {
		t.Errorf("want weaves-missing error, got %v", err)
	}
}

func TestLoad_RejectsBadChannelSource(t *testing.T) {
	c := strings.Replace(minimalClasses, "channeler: false", "channeler: true\n  channel_source: sadin", 1)
	_, err := Load(mapFS(minimalBackgrounds, c, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "channel_source") {
		t.Errorf("want channel_source error, got %v", err)
	}
}

func TestLoad_RejectsZeroSkillPoints(t *testing.T) {
	c := strings.Replace(minimalClasses, "skill_points: 2", "skill_points: 0", 1)
	_, err := Load(mapFS(minimalBackgrounds, c, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "skill_points") {
		t.Errorf("want skill_points error, got %v", err)
	}
}

func TestLoad_RejectsUnknownEquipmentItem(t *testing.T) {
	bg := strings.Replace(minimalBackgrounds,
		"items: [buckler]", "items: [buckler, doesnotexist]", 1)
	_, err := Load(mapFS(bg, minimalClasses, minimalFeats, minimalSkills, minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "unknown item") {
		t.Errorf("want unknown-item error, got %v", err)
	}
}

func TestLoad_RejectsItemStatsOnUntypedTier(t *testing.T) {
	// Clothing rejects a stats: block per data/world/README.md.
	const badItems = `
- id: cadinsor
  name: cadin'sor
  short: an Aiel outfit
  type: clothing
  weight: 6
  stats:
    bonus: 1
- id: buckler
  name: a buckler
  short: a small buckler
  type: shield
  weight: 5
  stats:
    kind: buckler
    bonus: 1
`
	fs := mapFS(minimalBackgrounds, minimalClasses, minimalFeats, minimalSkills, minimalWeaves)
	fs[fileItems].Data = []byte(badItems)
	_, err := Load(fs)
	if err == nil || !strings.Contains(err.Error(), "does not accept a stats block") {
		t.Errorf("want stats-rejection error, got %v", err)
	}
}

func TestLoad_RejectsBadCurrency(t *testing.T) {
	const badItems = `
- id: buckler
  name: a buckler
  short: a small buckler
  type: shield
  weight: 5
  value: "not a currency"
  stats:
    kind: buckler
    bonus: 1
`
	fs := mapFS(minimalBackgrounds, minimalClasses, minimalFeats, minimalSkills, minimalWeaves)
	fs[fileItems].Data = []byte(badItems)
	_, err := Load(fs)
	if err == nil || !strings.Contains(err.Error(), "invalid value") {
		t.Errorf("want invalid-value error, got %v", err)
	}
}

func TestLoad_RealCatalogResolvesEveryEquipmentRef(t *testing.T) {
	cat := liveDir(t)
	// Spot-check each background — every items entry should resolve.
	for _, bg := range cat.Backgrounds() {
		for _, opt := range bg.EquipmentOptions {
			for _, ref := range opt.Items {
				if _, ok := cat.Item(ref); !ok {
					t.Errorf("background %q option %q references missing item %q",
						bg.ID, opt.Label, ref)
				}
			}
		}
	}
	// Sanity: at least the 50 known ids load.
	if len(cat.Items()) < 30 {
		t.Errorf("expected >=30 items in catalog, got %d", len(cat.Items()))
	}
}

func TestLoad_RejectsEmptyCatalog(t *testing.T) {
	_, err := Load(mapFS(minimalBackgrounds, minimalClasses, minimalFeats, "", minimalWeaves))
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("want empty-catalog error, got %v", err)
	}
}
