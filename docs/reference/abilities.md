# Abilities

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 1: Abilities, pp. 16–21) for use in WheelMUD implementation.

## Overview

Every character has six abilities:

| Ability | Abbrev |
|---------|--------|
| Strength | Str |
| Dexterity | Dex |
| Constitution | Con |
| Intelligence | Int |
| Wisdom | Wis |
| Charisma | Cha |

Each ability provides a modifier applied to die rolls and other derived numbers. Positive modifiers are *bonuses*; negative modifiers are *penalties*. Monsters and creatures also have ability scores.

## Score Generation

- **Roll method:** roll 4d6, drop the lowest die, sum the highest three. Repeat six times. Assign the six results to abilities.
- **Alternatives:** standard array, or point-buy (referenced in book introduction; not specified on these pages).
- **Reroll rule:** if the sum of all modifiers is 0 or lower, *or* the highest score is 13 or lower, the player may reroll all six.
- **Typical scores:** 3 (horrible) to 18 (tremendous). Average inhabitant: 10–11. Average PC: 12–13.

## Ability Modifiers and Bonus Weaves (Table 1-1)

| Score | Modifier | Bonus Weaves by Weave Level (0/1/2/3/4/5/6/7/8/9) |
|-------|----------|----------------------------------------------------|
| 1     | -5 | Can't cast weaves tied to this ability |
| 2-3   | -4 | Can't cast weaves tied to this ability |
| 4-5   | -3 | Can't cast weaves tied to this ability |
| 6-7   | -2 | Can't cast weaves tied to this ability |
| 8-9   | -1 | Can't cast weaves tied to this ability |
| 10-11 | +0  | — / — / — / — / — / — / — / — / — / — |
| 12-13 | +1  | — / 1 / — / — / — / — / — / — / — / — |
| 14-15 | +2  | — / 1 / 1 / — / — / — / — / — / — / — |
| 16-17 | +3  | — / 1 / 1 / 1 / — / — / — / — / — / — |
| 18-19 | +4  | — / 1 / 1 / 1 / 1 / — / — / — / — / — |
| 20-21 | +5  | — / 2 / 1 / 1 / 1 / 1 / — / — / — / — |
| 22-23 | +6  | — / 2 / 2 / 1 / 1 / 1 / 1 / — / — / — |
| 24-25 | +7  | — / 2 / 2 / 2 / 1 / 1 / 1 / 1 / — / — |
| 26-27 | +8  | — / 2 / 2 / 2 / 2 / 1 / 1 / 1 / 1 / — |
| 28-29 | +9  | — / 3 / 2 / 2 / 2 / 2 / 1 / 1 / 1 / 1 |
| 30-31 | +10 | — / 3 / 3 / 2 / 2 / 2 / 2 / 1 / 1 / 1 |
| 32-33 | +11 | — / 3 / 3 / 3 / 2 / 2 / 2 / 2 / 1 / 1 |
| 34-35 | +12 | — / 3 / 3 / 3 / 3 / 2 / 2 / 2 / 2 / 1 |
| 36-37 | +13 | — / 4 / 3 / 3 / 3 / 3 / 2 / 2 / 2 / 2 |
| 38-39 | +14 | — / 4 / 4 / 3 / 3 / 3 / 3 / 2 / 2 / 2 |
| 40-41 | +15 | — / 4 / 4 / 4 / 3 / 3 / 3 / 3 / 2 / 2 |
| 42-43 | +16 | — / 4 / 4 / 4 / 4 / 3 / 3 / 3 / 3 / 2 |
| 44-45 | +17 | — / 5 / 4 / 4 / 4 / 4 / 3 / 3 / 3 / 3 |
| ...   | ...  | (pattern continues) |

### Computing the Modifier

`modifier = floor((score - 10) / 2)` matches every row in the table. Implementation can derive modifiers arithmetically rather than from a lookup.

### Channelers and Bonus Weaves

- **Initiates** use **Intelligence** + **Wisdom** for channeling (training-based).
- **Wilders** use **Charisma** + **Wisdom** (self-taught/willpower-based).
- A channeler must have a sufficiently high score in the relevant ability to cast a weave of a given level:
  - Initiate: `Intelligence >= 10 + weave_level`
  - Wilder: `Wisdom >= 10 + weave_level` (and `Charisma >= 10 + weave_level` to cast at all)
- Bonus weaves come from *both* relevant abilities (initiate: Int + Wis; wilder: Cha + Wis), summed per weave level.
- **Cap:** ability-score bonuses do **not** grant bonus weaves beyond 9th level, even at very high scores (11+ modifier).
- **Floor:** if a relevant channeling ability drops to 9 or lower (e.g., from temporary ability damage), the channeler **cannot cast any weaves** until it recovers — even if the other relevant ability is unaffected.

## The Six Abilities

### Strength (Str)

Muscle and physical power. Most important for armsmen, woodsmen, *algai'd'siswai*, Trollocs, and melee combatants.

Applies to:
- Melee attack rolls.
- Damage rolls for melee weapons and thrown weapons.
  - Off-hand attacks: half Str modifier.
  - Two-handed attacks: 1.5× Str modifier.
  - Bow/sling attacks: Str **penalty** applies, but **not** Str bonus.
- Climb, Jump, and Swim checks (Str is the key ability for these skills).
- Strength checks (e.g., breaking down doors).

**Example scores (Table 1-2):** Rat 2-3 (-4), Egwene al'Vere 9 (-1), Nynaeve al'Meara 10-11 (+0), Typical human 10-11 (+0), Mat Cauthon 12-13 (+1), Dain Bornhald 14-15 (+2), Perrin Aybara 16-17 (+3), Myrddraal 18-19 (+4), Loial 20-21 (+5), To'raken 22-23 (+6).

### Dexterity (Dex)

Hand-eye coordination, agility, reflexes, balance. Important for wanderers, light/no armor wearers, woodsmen, Aiel, channelers, and skilled archers.

Applies to:
- Ranged attack rolls (bows, throwing weapons, etc.).
- Defense (when the character can react to the attack).
- Reflex saving throws.
- Balance, Escape Artist, Hide, Move Silently, Open Lock, Pick Pocket, Ride, Tumble, Use Rope checks.

**Example scores (Table 1-3):** Loial 8-9 (-1), Typical human 10-11 (+0), Perrin Aybara 12-13 (+1), Rand al'Thor 14-15 (+2), Aviendha 16-17 (+3), Min Farshaw 18-19 (+4), Mat Cauthon 20-21 (+5), Myrddraal 22-23 (+6).

### Constitution (Con)

Health, stamina, endurance, fortitude. Important for everyone.

Applies to:
- Each Hit Die rolled. A Con penalty can **never** drop a Hit Die roll below 1 — the character always gains at least 1 hp per level.
- Fortitude saving throws (resist poison and similar threats).
- Concentration checks (key ability for the Concentration skill — important for channelers).

**Retroactive HP:** if Con changes enough to alter the Con modifier, current/max HP adjust retroactively across all levels gained so far. Example: a 4th-level armsman raising Con from 15→16 gains +4 HP (one per level).

**Example scores (Table 1-4):** Thomdril Merrilin 8-9 (-1), Min Farshaw 10-11 (+0), Typical human 10-11 (+0), Mat Cauthon 12-13 (+1), Rand al'Thor 14-15 (+2), Loial 16-17 (+3), Perrin Aybara 18-19 (+4), Gholam 20-21 (+5).

### Intelligence (Int)

Learning, reasoning, deduction. Important for initiates and skill-heavy characters.

Applies to:
- Number of languages known at start.
- Skill points per level (minimum 1 per level even with a penalty).
- Appraise, Craft, Decipher Script, Disable Device, Forgery, Invert, Knowledge, Read Lips, Search, Weavesight checks.
- For initiates: bonus weaves and weave-casting threshold (`Int >= 10 + weave_level`).

Animals: typically Int 1 or 2. Creatures of humanlike intelligence: Int >= 3.

**Example scores (Table 1-5):** Horse 2-3 (-4), Torm 4-5 (-3), Wolf 6-7 (-2), Trolloc 8-9 (-1), Perrin Aybara 10-11 (+0), Typical human 10-11 (+0), al'Lan Mandragoran 12-13 (+1), Matrim Cauthon 14-15 (+2), Rand al'Thor 16-17 (+3), Loial 18-19 (+4), Elayne Trakand 20-21 (+5).

### Wisdom (Wis)

Willpower, common sense, perceptiveness, intuition. Important for perceptive characters, channelers (especially wilders/initiates), and those who read people.

Applies to:
- Will saving throws (resist harmful weaves and similar effects).
- Composure, Heal, Innuendo, Intuit Direction, Listen, Profession, Sense Motive, Spot, Wilderness Lore checks.
- For channelers: bonus weaves (both wilders and initiates).

Animals: typically Wis 1 or 2; humanlike insight/cunning ≥ 3.

**Example scores (Table 1-6):** Dain Bornhald 8-9 (-1), Matrim Cauthon 10-11 (+0), Typical human 10-11 (+0), Min Farshaw 12-13 (+1), Aviendha 14-15 (+2), Perrin Aybara 16-17 (+3), Rand al'Thor 18-19 (+4), Egwene al'Vere 20-21 (+5).

### Charisma (Cha)

Force of personality, persuasiveness, magnetism, leadership, willpower in social contests. Includes some physical attractiveness but represents *strength of personality* primarily. Important for nobles, armsmen-leaders, and especially wilders.

Applies to:
- Animal Empathy, Bluff, Diplomacy, Disguise, Gather Information, Handle Animal, Intimidate, Perform checks.
- All checks representing attempts to influence others.
- For wilders: bonus weaves and casting threshold (`Cha >= 10 + weave_level`).

Even creatures have Cha; ugly/unimpressive creatures (toads, vermin) have low scores. A high-Cha beast resists scare/break attempts more easily.

**Example scores (Table 1-7):** Rat 2-3 (-4), Grolm 4-5 (-3), Torm 6-7 (-2), Loial 10-11 (0), Typical human 10-11 (0), Min Farshaw 14-15 (+2), Mat Cauthon 16-17 (+3), Egwene al'Vere 18-19 (+4), al'Lan Mandragoran 20-21 (+5), Rand al'Thor 22-23 (+6).

## Changing Ability Scores

Scores can increase without limit:

- **Level-up bonus:** +1 to any one score at 4th level and every four levels thereafter (8th, 12th, 16th, 20th, ...).
- **Inherent bonus:** rare *ter'angreal* can permanently boost a score. Inherent bonus capped at +5 per ability.
- **Ability damage (temporary):** poisons, diseases, and similar effects. Damaged points return at **1 point per day per damaged ability**.
- **Ability drain (Shadowspawn):** can be temporary or permanent. Permanent drain does not recover naturally; Healing weaves or extraordinary methods may restore it.

When an ability score changes, all derived attributes recalculate:

- HP recomputes retroactively across all levels (Con).
- Bonus weaves and casting thresholds re-evaluate (Int / Wis / Cha for channelers).
- Skill points per level update going forward — but **not** retroactively (Int).
  - Example: an initiate raising Int from 15→16 at 4th level gets her 3rd-level bonus weave at 5th level, and from this level forward gains 7 skill points/level instead of 6, but doesn't recoup the missed point on levels 1–3.

## Implementation Notes for WheelMUD

Suggested data shape (not prescriptive):

```go
type Abilities struct {
    Str, Dex, Con, Int, Wis, Cha int
}

func Modifier(score int) int { return (score - 10) / 2 } // floor for negatives via int math; verify for negatives if scores can be < 1

// Weave-casting eligibility
func CanCastWeave(channelerType ChannelerType, ab Abilities, level int) bool {
    switch channelerType {
    case Initiate:
        return ab.Int >= 10+level && ab.Wis >= 10 // and >9 floor on both Int and Wis
    case Wilder:
        return ab.Cha >= 10+level && ab.Wis >= 10+level
    }
    return false
}
```

Key invariants to enforce:

1. Modifier is derived, never stored independently — recompute from score.
2. Score floor of 10 (modifier ≥ +0) is required to cast any weave tied to that ability; below that, **no weaves at all** for that channeler.
3. HP recalculates retroactively when Con modifier changes.
4. Skill-points-per-level changes apply prospectively only.
5. Bonus-weave table caps at 9th-level weaves regardless of how high the modifier goes.
6. Off-hand and two-handed melee damage modifiers apply fractional Str (×0.5 and ×1.5 respectively); bows/slings use Str penalty only (no bonus).
7. Inherent bonuses cap at +5 per ability; level-up bonuses are uncapped.
8. Ability damage recovers at 1 point/day/ability; permanent drain requires Healing.

Open questions to resolve in design:

- How do we represent ability damage vs. drain vs. inherent bonus distinctly (current/max/inherent triple)?
- Where do skill key-ability mappings live (per-skill metadata)?
- How does the engine recompute HP on Con change without double-counting historical levels?
