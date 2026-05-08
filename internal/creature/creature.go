// Package creature models the shared core that mobs (NPCs) and
// player characters both build on, plus the WoT-flavored extensions
// (channeling, conditions, damage types, equipment slots).
//
// Layout:
//
//	Core         — fields every living thing has (ability scores, HP,
//	               defense, saves, speed, conditions). Embedded by both
//	               MobInstance and Character.
//	MobTemplate  — immutable archetype ("orc warrior"). Authored by
//	               builders; one row per kind.
//	MobInstance  — live spawn in a room. References a template id and
//	               carries its own mutable Core.
//	Character    — a player. Embeds Core, adds account fk, class levels,
//	               race, background, feats, skills, reputation, etc.
//	Channeling   — optional sub-record attached to either side; same
//	               struct used by PCs, Aes Sedai, Forsaken, damane.
//
// This file is a type-only skeleton. Persistence (repos), behavior
// (combat math, weave resolution), and validation live elsewhere.
package creature

import (
	"time"

	"github.com/Jasrags/WheelMUD/internal/currency"
)

// --- Enums ----------------------------------------------------------

// Size is the creature size category. Drives reach, attack/defense
// size mods, carrying capacity, and weapon usability.
type Size int8

const (
	SizeFine Size = iota
	SizeDiminutive
	SizeTiny
	SizeSmall
	SizeMedium
	SizeLarge
	SizeHuge
	SizeGargantuan
	SizeColossal
)

// Type is the broad creature category. Shadowspawn is its own
// category because several features (taint immunity, fade-on-link,
// Power perception) key off it.
type Type int8

const (
	TypeHumanoid Type = iota
	TypeAnimal
	TypeExotic
	TypeShadowspawn
)

// Gender is recorded on every creature; channelers also key Saidin
// vs Saidar off this.
type Gender int8

const (
	GenderNone Gender = iota
	GenderMale
	GenderFemale
)

// Posture is the narrative alignment tag (Good / Bad / Evil). It is
// not a mechanical Law/Chaos axis; it gates a handful of features
// (the Reputation system, certain feats).
type Posture int8

const (
	PostureGood Posture = iota
	PostureBad
	PostureEvil
)

// Race is a player-facing race choice. NPCs use it loosely.
type Race int8

const (
	RaceHuman Race = iota
	RaceOgier
)

// Class is one of the seven WoT base classes. Multiclass = sum of
// ClassLevels values.
type Class int8

const (
	ClassAlgaiDSiswai Class = iota
	ClassArmsman
	ClassInitiate
	ClassNoble
	ClassWanderer
	ClassWilder
	ClassWoodsman
)

// Background is selected at character creation; supplies starting
// gear, languages, and height/weight modifiers.
type Background int8

const (
	BackgroundAiel Background = iota
	BackgroundAthaanMiere
	BackgroundBorderlander
	BackgroundCairhienin
	BackgroundDomani
	BackgroundEbouDari
	BackgroundIllianer
	BackgroundMidlander
	BackgroundTaraboner
	BackgroundTairen
	BackgroundTarValoner
)

// Stance is the character's body position. Combat math reads this
// (prone = -4 melee, +4 vs ranged, etc.).
type Stance int8

const (
	StanceStanding Stance = iota
	StanceSitting
	StanceSleeping
	StanceFighting
)

// Hand records dominant-hand for narrative/equipment flavor.
type Hand int8

const (
	HandRight Hand = iota
	HandLeft
	HandAmbidextrous
)

// Load is the encumbrance band the character is currently in.
type Load int8

const (
	LoadLight Load = iota
	LoadMedium
	LoadHeavy
	LoadOverloaded
)

// FlyManeuver describes flight agility. Zero value (None) means the
// creature cannot fly.
type FlyManeuver int8

const (
	FlyNone FlyManeuver = iota
	FlyClumsy
	FlyPoor
	FlyAverage
	FlyGood
	FlyPerfect
)

// --- Ability scores -------------------------------------------------

// AbilityScore captures the WoT ability-score triple. The three
// values are not redundant: damage reduces Current; drain reduces
// Max (harder to recover); ter'angreal and racial floors set
// Inherent.
type AbilityScore struct {
	Current  int8
	Max      int8
	Inherent int8
}

// Abilities is the canonical six-stat block.
type Abilities struct {
	Str AbilityScore
	Dex AbilityScore
	Con AbilityScore
	Int AbilityScore
	Wis AbilityScore
	Cha AbilityScore
}

// AbilityMod returns the d20 ability modifier floor((s-10)/2).
// Go integer division truncates toward zero, so an explicit branch
// for odd negative scores keeps -1 from rounding up to 0.
// Matches the floor convention used throughout the chargen pipeline.
func AbilityMod(score int8) int16 {
	diff := int16(score) - 10
	if diff < 0 && diff%2 != 0 {
		return (diff - 1) / 2
	}
	return diff / 2
}

// StrMod / DexMod / ConMod / IntMod / WisMod / ChaMod return the d20
// ability modifier for that ability's Current score. Combat / skills
// / weave-resolution all read these.
func (a Abilities) StrMod() int16 { return AbilityMod(a.Str.Current) }
func (a Abilities) DexMod() int16 { return AbilityMod(a.Dex.Current) }
func (a Abilities) ConMod() int16 { return AbilityMod(a.Con.Current) }
func (a Abilities) IntMod() int16 { return AbilityMod(a.Int.Current) }
func (a Abilities) WisMod() int16 { return AbilityMod(a.Wis.Current) }
func (a Abilities) ChaMod() int16 { return AbilityMod(a.Cha.Current) }

// Saves are the d20 saving throws.
type Saves struct {
	Fort int16
	Ref  int16
	Will int16
}

// Speed bundles every movement mode. Zero on a mode means the
// creature lacks it.
type Speed struct {
	BaseFt      int16
	ClimbFt     int16
	FlyFt       int16
	FlyManeuver FlyManeuver
	SwimFt      int16
	BurrowFt    int16
}

// --- Damage / resistance / conditions -------------------------------

// DamageType is one of the WoT damage kinds. Physical types are
// each tagged on a single weapon entry (a sword is Slash); One-Power
// weaves carry their elemental type; subdual is a separate pool;
// taint bypasses most resists.
type DamageType int8

const (
	DamageSlash DamageType = iota
	DamagePierce
	DamageBludgeon
	DamageFire
	DamageCold
	DamageLightning
	DamageAir
	DamageEarth
	DamageSpirit
	DamageSubdual
	DamageTaint
)

// Resist is a percentage modifier vs a damage type. Negative = vuln.
type Resist struct {
	Type DamageType
	Pct  int16
}

// DamageReduction is flat DR applied before resists. If Bypass is
// non-empty, attacks with that keyword (e.g. "magic", "cold-iron")
// ignore the DR.
type DamageReduction struct {
	Amount int16
	Bypass string
}

// Condition is the WoT-flavored condition enum. Stored as a bitset
// on Core for fast membership tests; durations live on Affect.
type Condition uint32

const (
	CondAbilityDamaged Condition = 1 << iota
	CondAbilityDrained
	CondBlinded
	CondChecked
	CondCowering
	CondDazed
	CondDeafened
	CondDisabled
	CondDying
	CondEntangled
	CondExhausted
	CondFatigued
	CondFlatFooted
	CondFrightened
	CondGrappled
	CondHeld
	CondHelpless
	CondPanicked
	CondParalyzed
	CondPinned
	CondProne
	CondShaken
	CondStable
	CondStaggered
	CondStunned
	CondUnconscious
)

// PositionFlags are environmental, driven by the surrounding combat
// state rather than by Affects.
type PositionFlags uint16

const (
	PosFlanked PositionFlags = 1 << iota
	PosCharging
	PosTotalDefense
	PosFightingDefensively
	PosConcealed
	PosCover
)

// SpecialQuality flags innate senses / traits.
type SpecialQuality uint32

const (
	QualBlindsight SpecialQuality = 1 << iota
	QualLowLightVision
	QualScent
	QualDarkvision
	QualTremorsense
)

// Affect is a timed buff/debuff on a creature. Source identifies the
// caster/item so refresh/stack rules can dedupe by (Source, Name).
type Affect struct {
	Source        int64
	Name          string
	Modifiers     []StatMod
	DurationTicks int32
	TickEffect    string // script ref; empty for pure stat mods
}

// StatMod is a single numeric tweak applied while an Affect is live.
type StatMod struct {
	Field string // "Str.Current", "Defense", "Saves.Will", "Speed.BaseFt"
	Delta int16
}

// --- Core ----------------------------------------------------------

// Core holds every field shared by mobs and player characters.
// Embedded by MobInstance and Character; persistence layers expand
// each field into its own column.
type Core struct {
	ID        int64
	Name      string
	Size      Size
	Type      Type
	Gender    Gender
	Alignment Posture

	Abilities Abilities

	HPCurrent int32
	HPMax     int32
	Subdual   int32 // separate non-lethal pool

	HitDice string // "4d8+8" — text for now; parser later

	// Defense replaces D&D's AC: class bonus + Dex + size + armor +
	// shield + dodge + misc. Every combat formula in §11 reads this.
	Defense int16
	Saves   Saves
	InitMod int16
	Speed   Speed
	BAB     int16 // drives iterative attacks at +6/+11/+16

	ReachFt  int16
	FaceFt   int16
	ThreatFt int16

	Conditions Condition
	Position   PositionFlags
	Specials   SpecialQuality

	DR      []DamageReduction
	Resists []Resist
	Affects []Affect

	CurrentRoomID int64
}

// --- Equipment & inventory -----------------------------------------

// Slot is a wear/wield position. WoT uses discrete slots, not a
// D&D wear-bitmask; two-handed weapons consume both PrimaryWield
// and OffHand.
type Slot int8

const (
	SlotArmor Slot = iota
	SlotShield
	SlotPrimaryWield
	SlotOffHand
	SlotOutfit
	SlotCloak
	SlotBackpack
	SlotHeldInHand // transient — torch/lantern; competes with weapon
	SlotMount
	// BeltPouches and WornMisc are slices on Equipment, not single
	// Slots, since they admit multiple items.
)

// Equipment is the per-creature wear/wield state. Slot fields hold
// the equipped item id (0 = empty); slice fields hold any number.
type Equipment struct {
	Armor        int64
	Shield       int64
	PrimaryWield int64
	OffHand      int64
	Outfit       int64
	Cloak        int64
	Backpack     int64
	HeldInHand   int64
	Mount        int64
	BeltPouches  []int64
	WornMisc     []int64 // signet rings, Aiel buckler-strap, etc.
}

// --- Channeling -----------------------------------------------------

// Source is the half of the True Source the channeler draws from.
type Source int8

const (
	SourceSaidin Source = iota // male
	SourceSaidar               // female
)

// ChannelerType drives which abilities key the casting thresholds.
type ChannelerType int8

const (
	ChannelerInitiate ChannelerType = iota // Int+Wis-keyed
	ChannelerWilder                        // Cha+Wis-keyed
)

// Power is one of the Five Powers. A channeler's Affinities is a
// subset; weaves require those Powers to cast unmodified.
type Power int8

const (
	PowerAir Power = iota
	PowerEarth
	PowerFire
	PowerWater
	PowerSpirit
)

// PowerSet is a bitmask over Power.
type PowerSet uint8

// SlotPool is one row of the per-level slot table (levels 0–9).
type SlotPool struct {
	Cur int8
	Max int8
}

// OathFlag is the Aes Sedai three-oath bitmask.
type OathFlag uint8

const (
	OathTruth    OathFlag = 1 << iota // speak no word that is not true
	OathNoWeapon                      // make no weapon for one man to kill another
	OathNoPower                       // use the Power as a weapon only against Shadowspawn etc.
)

// Channeling is the optional sub-record. nil = not a channeler.
// The same struct hangs off MobTemplate / MobInstance / Character.
type Channeling struct {
	GenderSource  Source
	ChannelerType ChannelerType
	Affinities    PowerSet
	Talents       []TalentID
	WeavesKnown   []WeaveRef
	// WeavesKnownIDs is a transitional sibling carrying chargen-
	// catalog string ids ("spark", "steady_hand", …) until §12
	// authors a numeric weave table that lets WeavesKnown []WeaveRef
	// carry the same data as int32 ids. Both fields stay populated
	// once the catalog lands — chargen commits write only the
	// string list, §12 reconciliation will fill WeavesKnown.
	WeavesKnownIDs []string
	Slots          [10]SlotPool // index = weave level

	// LastSlotRefreshAt stamps the last time the per-tick driver
	// refilled Slots[*].Cur. Zero value = "never refreshed" → the
	// next refresh pulse refills immediately, which is the right
	// behavior for chargen-fresh characters whose slots already
	// land at full from the catalog.
	LastSlotRefreshAt time.Time

	Embraced      bool
	EmbracedSince time.Time
	Madness       int16 // men only; Mental Stability slows accrual
	Stilled       bool

	BondedWarderID   int64
	BondedAesSedaiID int64
	HeldAngrealID    int64 // adds 1–10 to slot levels
	HeldSaangrealID  int64
	CircleID         int64

	AesSedaiOaths OathFlag
	Ageless       bool

	DamaneCollarTo int64 // a'dam binding NPC id; 0 if free
}

// TalentID and WeaveRef are placeholders — the real catalogs land
// in §12. Keeping them as ids decouples this package from the weave
// table layout.
type (
	TalentID int32
	WeaveRef struct {
		ID     int32
		Rarity Rarity
	}
)

// Rarity tags how widely a weave is known.
type Rarity int8

const (
	RarityCommon Rarity = iota
	RarityRare
	RarityLost
)

// --- Mob template + instance --------------------------------------

// ChallengeCode is the A–I difficulty code from the source material.
type ChallengeCode rune

// BehaviorFlags are the pack-AI hints used by the mob runner.
type BehaviorFlags uint16

const (
	BehavAggressive BehaviorFlags = 1 << iota
	BehavWimpy
	BehavSentinel
	BehavScavenger
	BehavAssistSameRace
	BehavHelper
)

// Attack is one natural or wielded attack profile.
type Attack struct {
	Name       string
	HitBonus   int16
	DamageDice string // "1d8+3"
	DamageType DamageType
	ReachFt    int16
}

// SpecialAttack is a named ability with its own resolution script.
type SpecialAttack struct {
	Name      string
	ScriptRef string
}

// AdvanceRule describes how a template gains HD as it scales up.
type AdvanceRule struct {
	HDRange    string // "5-8"
	SizeChange Size   // 0 = no change
}

// ShopConfig is the shopkeeper subtype's extra data; nil otherwise.
type ShopConfig struct {
	BuyTypes     []ItemType
	SellMarkup   float32 // 1.20 = +20%
	BuyMarkdown  float32 // 0.50 = pays half
	OpenHour     int8
	CloseHour    int8
	InventoryIDs []int64 // restocking wares
}

// ItemType is referenced here so ShopConfig can declare which kinds
// it deals in; the real item type enum lives in the item package
// once §9 lands its full item model.
type ItemType int8

// DefaultWanderChance is the per-template default for the §10
// wander tick. Templates loaded from YAML without an explicit value
// inherit this number. The default is 0 — random wandering is
// opt-in per template (set `wander_chance` in YAML for mobs that
// should drift, e.g. village dogs). Pre-authored worlds that relied
// on the old 0.25 baseline must declare it explicitly. Mirrored in
// migration 0022 as the column DEFAULT (still 0.25 there for legacy
// rows; the loader rewrites every boot from YAML so the column
// default only matters for hand-inserted rows).
const DefaultWanderChance = 0.0

// MobTemplate is the immutable archetype. Builders edit these; live
// mobs are MobInstances spawned from them.
type MobTemplate struct {
	ID         int64
	ExternalID string // YAML-stable id used by the world loader
	Core       Core   // base stats; copied to instance on spawn

	ChallengeCode ChallengeCode
	// XPValue is the per-template XP override (Phase D §19 polish).
	// Zero means "use the ChallengeCode → XP fallback table"
	// (combat.xpValueForChallenge); non-zero is the absolute XP
	// awarded on this template's death, before the damage-tally
	// weighting + group split. Persisted as INTEGER NOT NULL
	// DEFAULT 0 (migration 0040). Optional `xp_value:` YAML key.
	XPValue       int64
	Organization  string   // "solitary", "pack (3-6)", …
	Climate       []string // "temperate", "cold", …
	Terrain       []string // "forest", "mountain", …
	Advancement   []AdvanceRule

	BehaviorFlags BehaviorFlags
	// WanderChance is the per-mob, per-pulse probability that the
	// wander tick relocates instances of this template. Clamped to
	// [0, 1] at the storage layer (CHECK in 0022). Zero (the
	// default — see DefaultWanderChance) disables wandering for this
	// template; 1.0 forces every eligible pulse to move. Random
	// wandering is opt-in: set `wander_chance` in the mob's YAML for
	// mobs that should drift (village animals, drifters). Mobs with
	// scheduled routes (planned future feature) will keep
	// WanderChance at 0 and use the route system instead.
	WanderChance   float64
	NaturalAttacks []Attack
	SpecialAttacks []SpecialAttack
	Traits         []int32

	LootTableID    int64
	GoldDice       string // "2d10"
	DialogueTreeID int64
	TriggerScripts []string // script refs

	ShopkeeperConfig *ShopConfig // nil if not a vendor

	CorpseDecayTicks   int32
	RespawnZoneResetID int64
	// HomeRoomID is the room the loader originally spawned this
	// template into. The §9 Respawner uses it as the anchor location
	// when topping up a zone's mob population. 0 means "not
	// respawnable" — admin spawns via the `spawn` verb leave it 0.
	HomeRoomID int64

	// Shadowspawn-only fields. Zero values for non-Shadowspawn.
	ShadowLinkMyrddraalID int64
	TaintImmune           bool
	FadeOnLinkMasterTimer time.Duration

	ShortDesc string
	LongDesc  string

	// nil for non-channelers.
	Channeling *Channeling
}

// NewInstanceFromTemplate constructs a fresh, full-HP MobInstance
// for the given template, anchored at roomID. boundResetID identifies
// the zone-reset that produced the spawn (0 = manual / admin spawn).
// Used by the spawn admin verb and the §9 Respawner so the
// template→instance copy stays in one place.
func NewInstanceFromTemplate(tpl MobTemplate, roomID, boundResetID int64) MobInstance {
	return MobInstance{
		TemplateID:   tpl.ID,
		BoundResetID: boundResetID,
		Core: Core{
			HPCurrent:     tpl.Core.HPMax,
			CurrentRoomID: roomID,
		},
	}
}

// MobInstance is a live spawned mob in the world. Stat mutations
// (HP loss, conditions, looted inventory) happen here, never on the
// template.
type MobInstance struct {
	ID         int64
	TemplateID int64
	Core       Core // copy-on-spawn of MobTemplate.Core

	Equipment Equipment
	Inventory []int64 // item instance ids

	SpawnedAt    time.Time
	BoundResetID int64 // which zone reset spawned it; 0 = manual

	// Per-instance channeling state (slot pool, embrace, madness).
	// Pulled from the template on spawn for channeler templates.
	Channeling *Channeling
}

// --- Player character ----------------------------------------------

// SkillRanks is one row of the character's skill list.
type SkillRanks struct {
	Ranks        int8
	IsClassSkill bool // true ⇒ cap level+3; false ⇒ cap (level+3)/2
}

// QuestProgress is per-character quest state. The full quest engine
// lands in §15; this is enough to schema characters today.
type QuestProgress struct {
	QuestID     int64
	StepIndex   int16
	StateJSON   string // opaque to creature; engine parses
	CompletedAt time.Time
}

// DialogueCursor is per-NPC remembered dialogue state.
type DialogueCursor struct {
	TreeID int64
	NodeID int64
}

// Character is the player aggregate. Persisted in the characters
// table (extended by future migration); held in memory while online.
type Character struct {
	ID        int64
	AccountID int64
	Core      Core // shared with mobs

	Race        Race
	ClassLevels map[Class]int8 // multiclass = sum of values
	Background  Background

	XP             int64
	Feats          []int32
	Skills         map[int32]SkillRanks
	PracticePoints int16
	ClassFeatures  []int32

	HeightCm   int16
	WeightKg   int16
	Age        int16
	Handedness Hand

	// Reputation. InfamyShare is the running ratio of vicious gains
	// to total fame events: ≥ 0.5 flips the character to Infamous,
	// gating the fame/infamy feat lines.
	Fame        int32
	Infamy      int32
	InfamyShare float32

	Followers []int64 // unlocked at lvl 10, capped by Reputation

	Coin        currency.Amount
	BankBalance currency.Amount

	Encumbrance  Load
	FatigueUntil time.Time
	Position     Stance
	IdleSince    time.Time

	BoundRoomID   int64 // respawn point
	PlayedSeconds int64
	LastLogin     time.Time

	QuestLog      []QuestProgress
	DialogueState map[int64]DialogueCursor

	Equipment Equipment
	Inventory []int64

	// nil if not a channeler.
	Channeling *Channeling
}
