package cmd

// Score renders the player's character sheet — identity, stats, and
// wealth — using the shared internal/display helpers so the cyan|bold
// header and yellow|bold field rows match the chargen review pane.
//
// V1 surfaces the data that's already populated by chargen + the live
// Core fields. Reputation, status effects, channeling block, and
// equipment summary slot in as those subsystems land (Phase D/E).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// scoreLabelGutter aligns the Identity/Vitals/Wealth label column.
// 14 cols matches the chargen review screen's "Background:" alignment.
const scoreLabelGutter = 14

// NewScore builds the score command. Catalog may be nil — the command
// degrades to enum-fallback names for race/class/background. items
// may be nil — the Combat speed line falls back to the unarmed/naked
// baseline (1.00×) instead of resolving wielded/worn gear.
func NewScore(characters repo.CharacterRepo, items repo.ItemRepo, cat *chargen.Catalog) *telnet.Command {
	return &telnet.Command{
		Name:    "score",
		Aliases: []string{"sc", "stat", "stats", "sheet"},
		Help:    "Show your character sheet",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			ch, err := characters.FindByName(c.Ctx, c.Session.CharacterName)
			if err != nil {
				return c.Session.WriteString(
					"{{Could not load your character.}}::red\r\n")
			}
			gear := combat.ResolveGearFactors(c.Ctx, items, ch.Equipment)
			return renderScore(c.Session, ch, cat, gear)
		},
	}
}

// renderScore writes the full sheet to s. Broken out so tests can
// drive it without the registry / context machinery.
func renderScore(s *telnet.Session, ch repo.Character, cat *chargen.Catalog, gear combat.GearFactors) error {
	if err := display.SectionHeader(s, "Character — "+ch.Name); err != nil {
		return err
	}

	if err := display.Subsection(s, "Identity"); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Race", raceLabel(ch.Race), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Class", classLine(ch.ClassLevels, cat), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Background", backgroundLine(ch.Background, cat), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Gender", scoreGenderLabel(ch.Core.Gender), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Age", fmt.Sprintf("%d", ch.Age), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Height", scoreHeight(ch.HeightCm), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Weight", scoreWeight(ch.WeightKg), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Handed", scoreHandLabel(ch.Handedness), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Alignment", scorePostureLabel(ch.Core.Alignment), scoreLabelGutter); err != nil {
		return err
	}

	if err := display.Subsection(s, "Vitals"); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Hit points",
		fmt.Sprintf("%d / %d", ch.Core.HPCurrent, ch.Core.HPMax),
		scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Defense",
		fmt.Sprintf("%d", ch.Core.Defense), scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Saves",
		fmt.Sprintf("Fort %+d  Ref %+d  Will %+d",
			ch.Core.Saves.Fort, ch.Core.Saves.Ref, ch.Core.Saves.Will),
		scoreLabelGutter); err != nil {
		return err
	}
	if ch.Core.Speed.BaseFt > 0 {
		if err := display.FieldRow(s, "Speed",
			fmt.Sprintf("%d ft", ch.Core.Speed.BaseFt),
			scoreLabelGutter); err != nil {
			return err
		}
	}
	if ch.Core.StaminaMax > 0 {
		effRegen := combat.EffectiveStaminaRegen(ch.Core.StaminaRegen, gear.ArmorWeightClass)
		if err := display.FieldRow(s, "Stamina",
			fmt.Sprintf("%d / %d  (+%d/pulse)",
				ch.Core.StaminaCurrent, ch.Core.StaminaMax, effRegen),
			scoreLabelGutter); err != nil {
			return err
		}
	}
	if err := display.FieldRow(s, "Combat",
		fmt.Sprintf("%.2fx (%.2f weapon x %.2f armor)",
			gear.Multiplier(), gear.WeaponFactor, gear.ArmorFactor),
		scoreLabelGutter); err != nil {
		return err
	}

	if err := display.Subsection(s, "Abilities"); err != nil {
		return err
	}
	if err := writeAbilities(s, ch.Core.Abilities); err != nil {
		return err
	}

	if ch.Channeling != nil {
		if err := writeChannelingBlock(s, ch); err != nil {
			return err
		}
	}

	if err := display.Subsection(s, "Wealth"); err != nil {
		return err
	}
	carried := ch.Coin.Format()
	if carried == "" {
		carried = "0gc"
	}
	bank := ch.BankBalance.Format()
	if bank == "" {
		bank = "0gc"
	}
	if err := display.FieldRow(s, "Carried", carried, scoreLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Bank", bank, scoreLabelGutter); err != nil {
		return err
	}
	if ch.XP > 0 {
		if err := display.FieldRow(s, "XP",
			fmt.Sprintf("%d", ch.XP), scoreLabelGutter); err != nil {
			return err
		}
	}

	return display.Rule(s)
}

// writeChannelingBlock renders the Channeling subsection — Source,
// per-level slot pools, Madness, practice points, and the Stilled /
// Embraced flags. Phase E #27 added the dynamic state; #28 added
// the practice-points line.
func writeChannelingBlock(s *telnet.Session, ch repo.Character) error {
	if err := display.Subsection(s, "Channeling"); err != nil {
		return err
	}
	source := "Saidar"
	if ch.Channeling.GenderSource == creature.SourceSaidin {
		source = "Saidin"
	}
	if err := display.FieldRow(s, "Source", source, scoreLabelGutter); err != nil {
		return err
	}

	var b strings.Builder
	for i := range ch.Channeling.Slots {
		fmt.Fprintf(&b, "L%d %d/%d", i, ch.Channeling.Slots[i].Cur, ch.Channeling.Slots[i].Max)
		if i < len(ch.Channeling.Slots)-1 {
			b.WriteString("  ")
		}
	}
	if err := display.FieldRow(s, "Slots", b.String(), scoreLabelGutter); err != nil {
		return err
	}

	if err := display.FieldRow(s, "Madness",
		fmt.Sprintf("%d", ch.Channeling.Madness), scoreLabelGutter); err != nil {
		return err
	}

	if err := display.FieldRow(s, "Practice",
		fmt.Sprintf("%d", ch.PracticePoints), scoreLabelGutter); err != nil {
		return err
	}

	flags := []string{}
	if ch.Channeling.Stilled {
		flags = append(flags, "{{stilled}}::red|bold")
	}
	if ch.Channeling.Embraced {
		flags = append(flags, "{{embraced}}::cyan|bold")
	}
	if len(flags) > 0 {
		if err := display.FieldRow(s, "State", strings.Join(flags, " "), scoreLabelGutter); err != nil {
			return err
		}
	}
	return nil
}

// writeAbilities renders the six-stat block as two indented rows so
// the modifier column is easy to scan:
//
//	STR 14 (+2)   DEX 12 (+1)   CON 10 (+0)
//	INT 13 (+1)   WIS 11 (+0)   CHA  9 (-1)
func writeAbilities(s *telnet.Session, a creature.Abilities) error {
	type row struct {
		key   string
		score int8
	}
	rows := []row{
		{"STR", a.Str.Current},
		{"DEX", a.Dex.Current},
		{"CON", a.Con.Current},
		{"INT", a.Int.Current},
		{"WIS", a.Wis.Current},
		{"CHA", a.Cha.Current},
	}
	var b strings.Builder
	for i, r := range rows {
		if i%3 == 0 {
			if i > 0 {
				b.WriteString("\r\n")
			}
			b.WriteString("  ")
		} else {
			b.WriteString("   ")
		}
		fmt.Fprintf(&b, "{{%s}}::yellow|bold {{%2d}}::yellow (%+d)",
			r.key, r.score, scoreAbilityMod(int(r.score)))
	}
	b.WriteString("\r\n")
	return s.WriteString(b.String())
}

// scoreAbilityMod mirrors the chargen helper: floor((s-10)/2). For
// scores outside the chargen range Go's truncate-toward-zero diverges
// from floor for negatives, so handle that explicitly.
func scoreAbilityMod(score int) int {
	delta := score - 10
	if delta < 0 && delta%2 != 0 {
		return delta/2 - 1
	}
	return delta / 2
}

func raceLabel(r creature.Race) string {
	switch r {
	case creature.RaceOgier:
		return "ogier"
	default:
		return "human"
	}
}

// classLine renders the highest-level class as "Name (id) lvl N",
// falling back to enum-name if the catalog can't resolve it. Multi-
// class characters get a comma-separated list ordered by level desc.
func classLine(levels map[creature.Class]int8, cat *chargen.Catalog) string {
	if len(levels) == 0 {
		return "(unset)"
	}
	type entry struct {
		cls creature.Class
		lvl int8
	}
	rows := make([]entry, 0, len(levels))
	for k, v := range levels {
		if v <= 0 {
			continue
		}
		rows = append(rows, entry{k, v})
	}
	if len(rows) == 0 {
		return "(unset)"
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].lvl != rows[j].lvl {
			return rows[i].lvl > rows[j].lvl
		}
		return rows[i].cls < rows[j].cls
	})
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, fmt.Sprintf("%s lvl %d", classDisplay(r.cls, cat), r.lvl))
	}
	return strings.Join(parts, ", ")
}

func classDisplay(cls creature.Class, cat *chargen.Catalog) string {
	if cat != nil {
		for _, c := range cat.Classes() {
			if c.Enum == cls {
				return fmt.Sprintf("%s (%s)", c.Name, c.ID)
			}
		}
	}
	return classFallback(cls)
}

func backgroundLine(bg creature.Background, cat *chargen.Catalog) string {
	if cat != nil {
		for _, b := range cat.Backgrounds() {
			if b.Enum == bg {
				return fmt.Sprintf("%s (%s)", b.Name, b.ID)
			}
		}
	}
	return backgroundFallback(bg)
}

func classFallback(cls creature.Class) string {
	switch cls {
	case creature.ClassAlgaiDSiswai:
		return "Algai'd'Siswai"
	case creature.ClassArmsman:
		return "Armsman"
	case creature.ClassInitiate:
		return "Initiate"
	case creature.ClassNoble:
		return "Noble"
	case creature.ClassWanderer:
		return "Wanderer"
	case creature.ClassWilder:
		return "Wilder"
	case creature.ClassWoodsman:
		return "Woodsman"
	}
	return "(unknown)"
}

func backgroundFallback(bg creature.Background) string {
	switch bg {
	case creature.BackgroundAiel:
		return "Aiel"
	case creature.BackgroundAthaanMiere:
		return "Atha'an Miere"
	case creature.BackgroundBorderlander:
		return "Borderlander"
	case creature.BackgroundCairhienin:
		return "Cairhienin"
	case creature.BackgroundDomani:
		return "Domani"
	case creature.BackgroundEbouDari:
		return "Ebou Dari"
	case creature.BackgroundIllianer:
		return "Illianer"
	case creature.BackgroundMidlander:
		return "Midlander"
	case creature.BackgroundTaraboner:
		return "Taraboner"
	case creature.BackgroundTairen:
		return "Tairen"
	case creature.BackgroundTarValoner:
		return "Tar Valoner"
	}
	return "(unknown)"
}

// scoreGenderLabel / scoreHandLabel / scorePostureLabel mirror the
// chargen identity helpers without taking a runtime dep on
// internal/mode (would invite cycles via the chargen catalog).
func scoreGenderLabel(g creature.Gender) string {
	switch g {
	case creature.GenderFemale:
		return "female"
	case creature.GenderMale:
		return "male"
	}
	return "unspecified"
}

func scoreHandLabel(h creature.Hand) string {
	switch h {
	case creature.HandLeft:
		return "left"
	case creature.HandAmbidextrous:
		return "ambidextrous"
	}
	return "right"
}

func scorePostureLabel(p creature.Posture) string {
	switch p {
	case creature.PostureBad:
		return "bad"
	case creature.PostureEvil:
		return "evil"
	}
	return "good"
}

// scoreHeight / scoreWeight render in-world units (foot/inch + cm,
// stone+lb + kg). Mirrors the chargen identity helpers — when those
// helpers move into internal/display these wrappers go away.
func scoreHeight(cm int16) string {
	if cm <= 0 {
		return "(unset)"
	}
	totalIn := int((float64(cm) / 2.54) + 0.5)
	ft := totalIn / 12
	in := totalIn % 12
	return fmt.Sprintf("%d ft %d in (%d cm)", ft, in, cm)
}

func scoreWeight(kg int16) string {
	if kg <= 0 {
		return "(unset)"
	}
	lb := int((float64(kg) / 0.4535924) + 0.5)
	stone := lb / 10
	rem := lb % 10
	if rem == 0 {
		return fmt.Sprintf("%d stone (%d lb / %d kg)", stone, lb, kg)
	}
	return fmt.Sprintf("%d stone %d lb (%d lb / %d kg)", stone, rem, lb, kg)
}
