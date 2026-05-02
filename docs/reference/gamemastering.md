# Gamemastering

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 10: Gamemastering, pp. 186–225) for use in WheelMUD implementation. The chapter mixes prose advice with hard rules; this document captures the latter and condenses the former.

## GM Duties (summary)

- Provide adventures, world, and NPCs.
- Adjudicate rules; resolve conflicts; track campaign state.
- Maintain game balance; manage player roster and table dynamics.

## Style Considerations

- Three baseline styles: **Dancing with the Dark One** (action), **A New Age of Legends** (intrigue/character-driven), **Something in Between**.
- Other axes: serious vs. humorous, naming consistency, multi-hero rule, third-person vs. first-person interaction.

## Rules Adjudication Hierarchy

1. Core rulebook overrides any other published WoT product.
2. Core rule overrides published-adventure rules unless the adventure rule is explicitly local.
3. House rule once chosen → consistent for the rest of the campaign.
4. **GM's secret rule:** favorable conditions = +2; unfavorable = -2.

## Save vs. Check

- **Saves** = avoid harm; always reflect level via base save bonus.
- **Checks** = accomplish a task; only reflect level when a skill rank applies.

## Ta'veren

Ta'veren cannot be chosen by players; the GM grants the status. Bonuses (extra Affinities/Talents for channelers, luck bonuses, skill bonuses, occasional Cha boost) are removable when the Pattern releases the character.

## NPC Interaction

### Initial Attitude (Table 10-1)

| Attitude | Means | Possible Actions |
|----------|-------|------------------|
| Hostile | Risks to hurt you | Attack, interfere, berate, flee |
| Unfriendly | Wishes you ill | Mislead, gossip, avoid, watch suspiciously, insult |
| Indifferent | Doesn't care | Socially expected interaction |
| Friendly | Wishes you well | Chat, advise, offer limited help, advocate |
| Helpful | Risks to help you | Protect, back up, heal, aid |

### Influencing Attitude (Table 10-2)

DC = Diplomacy (or Charisma) check result needed to shift attitude:

| Starting | → Hostile | Unfriendly | Indifferent | Friendly | Helpful |
|----------|----------|-----------|-------------|----------|---------|
| Hostile | <20 | 20 | 25 | 35 | 50 |
| Unfriendly | <5 | 5 | 15 | 25 | 40 |
| Indifferent | — | <1 | 1 | 15 | 30 |
| Friendly | — | — | <1 | 1 | 20 |

Heroes may not be influenced via this rule; only the player decides their attitude.

## Stacking Bonuses

| Type | Stacks? | Notes |
|------|---------|-------|
| Armor | Armor + shield | Armor bonuses with same name don't stack |
| Circumstance | Yes (different sources) | Same source → no |
| Competence | No (with same source) | Take higher |
| Dodge | Always stack | Lost when Dex bonus lost |
| Enhancement | No | Take higher |
| Morale | No | Take higher |
| Natural Armor | No (with itself) | Stacks with armor/shield |
| Racial | No | — |
| Resistance | No | — |
| Synergy | Yes (skill→skill) | — |

## Adventures

### Length Tiers

| Length | Encounters | Sessions | Base XP |
|--------|-----------|---------|---------|
| Short | 3–5 | 1 | 1,000 |
| Medium | 6–10 | 2–3 | 2,000 |
| Long | 12–15 | 4+ | 4,000 |

XP/character = `baseXP × averagePartyLevel ÷ partySize`. Roughly: 4 short / 2 medium / 1 long advances a 4-character party one level.

### Difficulty Mix

| Length | Simple : Challenging : Extreme |
|--------|-------------------------------|
| Short | ~⅓ : ~⅓ : ≤1 |
| Medium / Long | ~½ challenging; remainder split between simple and extreme |

### Environmental Modifiers (encounter-grade)

| Factor | Game Effect |
|--------|-------------|
| Pits / chasms / bridges / ledges | Push or leap actions become available |
| Fog | 20% concealment for everyone |
| Whirling blades / Age of Legends clockwork | DC 13 Dex check / round or 6d6 slashing/crushing |
| Steam vents / fire pits | Random target makes DC 15 Dex check / round or 3d6 heat |
| Raising / lowering platforms | Melee only on same level; platforms swap every other round |
| Swamp / marsh | Speed halved; dropped items may be lost |
| Ice / slippery surface | DC 10 Dex check / round or fall (move to stand) |

### In-World Units (flavor)

- 10 inches = 3 hands = 1 foot.
- 3 ft = 1 pace; 2 paces = 1 span; 1,000 spans = 1 mile; 4 mi = 1 league.
- 100 paces × 100 paces = 1 hide.
- 10 oz = 1 lb; 10 lb = 1 stone; 10 stone = 1 hundredweight; 10 hundredweight = 1 ton.

## Madness (male channelers)

### Rating

- Initial = `1d6` (rolled secretly by GM).
- +`1d6` per channeler-class level gained.
- +1 per overchannel attempt.

### Trigger Conditions (cumulative)

| Madness Rating | Trigger |
|----------------|---------|
| 0–15 | — |
| 16–30 | Injury |
| 31–40 | Will save |
| 41–50 | Casting a weave / using ter'angreal |
| 51–60 | Threat (real or imagined) |
| 61+ | Constant (permanent insanity; no further checks) |

### Check for Madness

Will save vs. DC = current Madness rating. On failure: roll `1d20`, subtract from rating, consult symptom table.

| `Rating - d20` | Symptom | Duration |
|----------------|---------|----------|
| ≤5 | Delusion | 2d6 minutes |
| 6–15 | Suspicion | 2d6 hours |
| 16–25 | Panic | 2d6 rounds |
| 26–35 | Withdrawal | 2d6 hours |
| 36–45 | Fury | 2d6 rounds |
| 46–55 | Disease | See text |
| 56+ | Dementia | See text |

- **Disease:** Fort DC 20; incubation 1 week; thereafter 1d3 temporary Con damage / day until Con 0 = death. (Wolfbrothers do **not** suffer the disease.)
- **Dementia:** PC becomes an NPC; permanent.

## Prestige Classes

### Eligibility & Mechanics

- Cannot start at 1st level — typically requires ~5th–6th character level.
- Multiclassing into a prestige class **does not** suffer the -2 Defense penalty per extra class.
- Channeling prestige classes (Aes Sedai, Asha'man, Windfinder, Wise One) do **not** track their own weaves-per-day. The character continues to use their original initiate/wilder progression but reads the table at *total* level.
- Acceptable requirement axes: background, base attack bonus, skill ranks, specific feats / proficiencies, special abilities (sneak attack, uncanny dodge), gender (for One Power gender-locks), bond status (Warder).
- Class-and-level requirements are discouraged in favor of derived metrics.

### Prestige Class Roster

| Class | HD | Base Save Profile | Skill pts |
|-------|----|------------------|-----------|
| Aes Sedai | d4 | Will-strong | 4 + Int |
| Asha'man | d6 | Fort/Will dual | 4 + Int |
| Blademaster | d10 | Reflex-strong | 2 + Int |
| Commander | d8 | Fort-strong | 6 + Int |
| Gleeman | d6 | Reflex/Will | 6 + Int |
| Thief-Taker | d8 | Reflex-strong | 6 + Int |
| Warder | d12 | All three middling | 4 + Int |
| Windfinder | d4 | Will-strong | 4 + Int |
| Wise One | d6 | Fort/Will | 4 + Int |
| Wolfbrother | d8 | Fort-strong | 2 + Int |

### Aes Sedai (Table 10-3)

- **Requirements:** female; Composure 4, Concentration 8, Weavesight 4; feats Multiweave, Sense Residue, Tie Off Weave; ≥2 Talents.
- **No weapon/armor proficiency.**
- Class skills: Composure, Concentration, Decipher Script, Diplomacy, Gather Information, Heal, Innuendo, Intimidate, Invert, Knowledge (any), Sense Motive, Weavesight.
- BAB +0 → +5 over 10; Will save strong (+2 → +7); Defense +0 → +3; Reputation gain 1/level (mostly).
- Features by level:
  1. **Iron Will** (bonus feat); **Aes Sedai Presence** (+4 competence Intimidate).
  2. **Resources** — Gather Information +2 circumstance; chance of obtaining requested resources 50%/25%/10% (city/town/countryside).
  3. **Extra Affinity** (bonus feat).
  4. **Extra Talent** (bonus feat).
  5. **Improved Resources** — +4 circumstance; 75%/50%/20%.
  6. **Control** — +5 competence Concentration when overchanneling within an Affinity.
  7. **Resolve** — treat Wis as +2 for weaves/day & bonus weaves.
  8. **Improved Control** — +10 competence Concentration when overchanneling within an Affinity.
  9. **Great Fortitude** (bonus feat).
  10. **Improved Resolve** — Wis treated as +4 for weaves/day & bonus weaves.

### Asha'man (Table 10-4)

- **Requirements:** male; BAB +2; Composure 4, Concentration 8, Weavesight 3; feats Multiweave, Sense Residue, Tie Off Weave; ≥2 Talents; sword proficiency.
- **Proficiencies:** simple + martial weapons; no armor.
- Class skills: Composure, Concentration, Diplomacy, Gather Information, Innuendo, Intimidate, Invert, Knowledge (any), Sense Motive, Spot, Weavesight.
- Features:
  1. **Iron Will**; **Asha'man Presence** (+4 competence Intimidate).
  2. **Asha'man Combat Casting** — +5 Concentration when casting/maintaining.
  3. **Offensive Control** — +5 Concentration overchannel for offensive weaves only.
  4. **Improved Initiative** (bonus feat).
  5. **Great Fortitude** (bonus feat).
  6. **Improved Offensive Control** — +10 Concentration overchannel (offensive weaves only).
  7. **Extra Affinity** (bonus feat).
  8. **Resolve** — Wis treated as +2.
  9. **Improved Asha'man Combat Casting** — +6 Concentration on offensive cast.
  10. **Improved Resolve** — Wis treated as +4.

### Blademaster (Table 10-5)

- **Requirements:** BAB +5; Balance 4, Intimidate 5; feats Combat Reflexes, Dodge, Mobility, Spring Attack, Whirlwind Attack; sword proficiency; masterwork sword.
- **Proficiencies:** all simple/martial weapons; all armor + shields.
- Class skills: Balance, Intimidate, Knowledge (weaponry), Listen, Sense Motive, Spot, Tumble.
- BAB +1 → +10; Reflex strong (+2 → +7).
- Features (signature ability **Parting the Silk** = treat next sword hit's *base* damage as maximum, no roll; bonus dice still rolled; cannot be combined with crit):
  1. **Parting the Silk** 1/day/level.
  2. **Increased Multiplier** 1/day — +1 to weapon's crit multiplier; declare before damage.
  3. **Superior Weapon Focus** — +1 attack with chosen sword (stacks with Weapon Focus).
  4. **Parting the Silk** 2/day/level.
  5. **Eyes of the Crane** — when delaying past an attacker, +2 attack/damage with sword (stacks with Parting the Silk).
  6. **Increased Multiplier** 2/day.
  7. **Hummingbird Kisses the Honeyrose** — Improved Critical (bonus feat).
  8. **Parting the Silk** 3/day/level.
  9. **Heron Spreads His Wings** — Whirlwind Attack as an attack action (1/round).
  10. **Increased Multiplier** 3/day.

### Commander (Table 10-6)

- **Requirements:** BAB +5; Diplomacy 6, Ride 5; member of organized standing force.
- **Proficiencies:** all simple/martial; all armor + shields.
- Features:
  1. **Strategy** — full-round action + Diplomacy DC `10+allies`; allies +Cha competence to skill checks for 1 minute.
  2. **Battle Cry** — 1/day/level shout; +2 morale Will vs. mind weaves, +1 attack/dmg, lasts Cha rounds.
  3. **Hard March** — +4 morale Con on forced-march checks for the commander's company.
  4. **Logistics** — requisition value `level × Cha × 2,000 mk` outstanding.
  5. **Tactics** — single ally gets +Int to attack OR +Int dodge to Defense/Reflex for `1d4 + Cha` rounds (attack action), or all allies for `Cha` rounds (full-round).
  6. **Improved Strategy** — Strategy now lasts 10 minutes.
  7. **Improved Logistics** — cap × 2 (i.e. `× 4,000 mk`).
  8. **Superior Strategy** — Strategy lasts 1 hour.
  9. **Improved Tactics** — once/round, direct one ally as a free action or all allies as an attack action.
  10. **To the Bitter End** — allies within 30 ft fight without disabled/dying penalties down to -10 HP.

### Gleeman (Table 10-7)

- **Requirements:** Human, non-Aiel; Diplomacy or Intimidate 6; Perform 10; Pick Pocket 10; feat Fame.
- **Proficiencies:** all simple weapons + one of (longbow / longsword / rapier / sap / short sword / shortbow / whip); light & medium armor; shields.
- **Gleeman's Music** — `level + Cha` uses/day; move or attack action; deaf gleeman 20% fail chance on sound effects.

| Ability | Min Perform | Effect |
|---------|-------------|--------|
| Inspire Courage | 3 | Allies hearing for full round get +2 morale Will vs. mind weaves and +1 attack/dmg; lasts while performing + 5 rounds. |
| Fascinate | 3 (Perform or Pick Pocket) | Single creature within 90 ft; Will save = check; fails → spellbound 1 round/level; -4 to Spot/Listen. |
| Inspire Competence | 6 | Ally within 30 ft, +2 competence to a chosen skill while listening (max 2 minutes). |
| Inspire Greatness | 12 | One ally + 1 per 3 levels above 1st; +2d10 temp HP, +2 attack, +1 Fort. |

- Features:
  1. **Gleeman's Music**; **Gleeman's Lore** (level + Int bonus on lore check; DC by knowledge tier 10/20/25/30; no take-10/20).
  2. **Distract** (Perform 3+) — Pick Pocket vs. Spot; +1 attack/dmg per 5 of margin (single attack).
  4. **Virtuoso Performance — Calumny** (Perform 11+) — Will vs. Perform; shifts audience attitude one step worse, +2 morale opposed social vs. target, lasts 24 hr per use spent.
  5. **Persuasive** (bonus feat).
  6. **Trustworthy** (bonus feat).
  8. **Virtuoso Performance — Jarring Song** (Perform 12+) — anyone casting nearby Concentration `15 + casting level` or lose weave; costs 3 daily uses.
  9. **Mimic** (bonus feat).
  10. **Virtuoso Performance — Mindbending Melody** (Perform 14+) — fascinated target affected by Compulsion (CL 5); Will DC `15 + Cha`; costs 2 uses.

### Thief-Taker (Table 10-8)

- **Requirements:** BAB +6; Gather Information 5, Intimidate 5, Move Silently 5, Search 5; feats Exotic Weapon Proficiency (swordbreaker), Track.
- **Proficiencies:** simple + martial + swordbreaker; light armor; no shields.
- Features:
  1. **Brotherhood Contacts**; **Traps** (Search DC > 20 traps, OnePower-trap DC `25 + weave level`; beat Disable Device DC by 10 = bypass with party); **Sneak Attack +2d6**.
  2. **Exotic Weapon Proficiency** (bonus); **Uncanny Dodge** (keep Dex to Defense vs. flat-footed/invisible).
  3. **Capture** (entangle Small/Medium foe via flexible weapon, melee touch w/ Dex; opposed Str/Escape Artist to free); **Special Ability** (see below).
  4. **Uncanny Dodge** — can't be flanked (except by wanderer/thief-taker ≥4 levels higher); **Sneak Attack +4d6**.
  5. **Weapon Specialization** (bonus feat).
  6. **Uncanny Dodge** — +1 Reflex/+1 Defense vs. traps; **Exotic Weapon Proficiency** (bonus).
  7. **Sneak Attack +6d6**; **Special Ability**.
  8. **Uncanny Dodge** — +2 vs. traps.
  9. **Bonus Feat** (any non-channeling).
  10. **Sneak Attack +8d6**; **Special Ability**.

#### Thief-Taker Special Abilities (pick at 3/7/10)

| Ability | Effect |
|---------|--------|
| Crippling Strike | Sneak attacks deal 1 temp Str dmg. |
| Defensive Roll | 1/day, when reduced to ≤0 HP by a weapon, Reflex DC=damage halves it. |
| Opportunist | 1/round AoO vs. anyone just hit in melee; counts as round's AoO. |
| Skill Mastery | Pick `3 + Int` skills; can take 10 under stress. May pick again. |
| Feat | Replace special ability with any qualifying feat. |

### Warder (Table 10-9)

- **Requirements:** BAB +6; Balance 4, Intimidate 5, Ride 4; feats Alertness, Improved Initiative, Combat Reflexes; must have been targeted by **bond Warder** weave.
- **Proficiencies:** all simple/martial; all armor + shields.
- Features:
  1. **Armor Compatibility** (class Defense bonus stacks with armor/shield); **Power Attack** (bonus); **Warder's Cloak** (issued).
  2. **Defensive Awareness** — keep Dex to Defense vs. flat-footed.
  3. **Cleave** (bonus feat).
  4. **Iron Will** (bonus feat).
  5. **Great Cleave** (bonus feat).
  6. **Defensive Awareness** — can't be flanked.
  7. **Defensive Blow** — when defending bonded Aes Sedai, +2 morale to attack/dmg.
  8. **Superior Weapon Focus** — +1 attack with chosen weapon (stacks with Weapon Focus).
  9. **Improved Reflexes** — +2 initiative (stacks with Improved Initiative).
  10. **Cleave** with intervening 5-ft step (one step/round).

### Windfinder (Table 10-10)

- **Requirements:** female; Composure 4, Concentration 8, Weavesight 4; feats Multiweave, Sense Residue, Tie Off Weave; ≥2 Talents.
- **No proficiencies.**
- Features:
  1. **Iron Will** (bonus feat); **Windfinder Presence** (+4 competence Intimidate).
  2. **Multiweave** (bonus feat).
  3. **Windfinder Control** — +5 Concentration overchannel for weather weaves.
  4. **Open Sky** — double range/area of weather weaves.
  5. **Multiweave** (bonus).
  6. **Endurance** (bonus feat).
  7. **Improved Windfinder Control** — +10 Concentration overchannel for weather weaves.
  8. **Improved Open Sky** — quadruple range/area for weather weaves.
  9. **Multiweave** (bonus).
  10. **Extra Affinity** (bonus feat).

### Wise One (Table 10-11)

- **Requirements:** female; Composure 4, Concentration 8, Weavesight 4; feats Multiweave, Sense Residue; ≥2 Talents.
- **Proficiencies:** all simple weapons; no armor.
- Features:
  1. **Iron Will** (bonus); **Wise One Presence** (+4 competence Intimidate).
  2. **Endurance** (bonus feat).
  3. **Dreamwalk** (bonus feat).
  4. **Bend Dream** (bonus feat).
  5. **Great Fortitude** (bonus feat).
  6. **Dream Jump** (bonus feat).
  7. **Control** — +5 Concentration overchannel within an Affinity.
  8. **Dreamwatch** (bonus feat).
  9. **Extra Affinity** (bonus feat).
  10. **Improved Control** — +10 Concentration overchannel within an Affinity.

### Wolfbrother (Table 10-12)

- **Requirements:** non-Ogier; Animal Empathy 8, Listen 5, Spot 5, Wilderness Lore 5; feats Animal Affinity, Latent Dreamer; must have heard the call (rare in cities). May waive up to three skill/feat requirements at the cost of +1d6 Madness rating per waiver.
- **Proficiencies:** all simple/martial; light armor; shields.
- Features:
  1. **Wolfspeech** (telepathy with wolves, range `level × 10 mi`); **Nature Sense**; **Madness** (rating starts 1d6, +1d6 per wolfbrother level; **never the rotting disease**).
  2. **Scent** (30 ft baseline; 60 ft upwind / 15 ft downwind; strong x2, overpowering x3); **Low-Light Vision**; **Yelloweyes** (+2 Intimidate).
  3. **Wolf Dream** — may enter Tel'aran'rhiod.
  4. **Alert Pack** — sense and call wolves (1d6 arrive, 10 min/mile away, stay until task done or 4 hr); 1/day; Wisdom check `DC 20 + 1/5mi` to extend reach.
  5. **Sense Emotion** — +4 competence Sense Motive; Spot DC 15 to read basic emotion.
  6. **Track by Scent** — +4 competence on Wilderness Lore/Spot/Search via smell.
  7. **Survivor** (bonus feat).
  8. **Great Health** — +2 inherent Con.
  9. **Rapid Healing** — recover 1 HP / level / day strenuous; 1.5 light; 2 full rest (doubled with Heal aid). Ability damage 2/day (3 with care).
  10. **Call Wolves** — alert pack arrives in 2d6 rounds with 1d3 extra wolves; no Animal Empathy needed; wolves obey unconditionally.

## NPC Classes

NPC classes get feats every 3 levels and ability increases every 4 (per Table 3-1). Most NPCs cap at low levels; dangerous areas push higher levels. They may multiclass.

### Commoner (Table 10-13)

- **HD** d4. **BAB** poor. **All saves** poor. **Defense** rises slowly (+0 → +6 over 20). **Reputation Score** caps at 5.
- Class skills: Climb, Craft, Handle Animal, Jump, Listen, Profession, Ride, Spot, Swim, Use Rope.
- **Skill points:** 1st level `(2 + Int) × 4`, thereafter `2 + Int`.
- **Proficiency:** one simple weapon. No armor or shields.
- Starting gear: `5d4 mk`.

### Diplomat (Table 10-14)

- **HD** d8. **BAB** as commoner. **Will** strong; Fort/Reflex weak. **Defense** +0 → +6. **Reputation Score** to 7.
- Class skills: Appraise, Bluff, Diplomacy, Gather Information, Innuendo, Knowledge, Read Lips (exclusive), Sense Motive, Speak Language.
- **Skill points:** 1st level `(4 + Int) × 4`; thereafter `4 + Int`.
- **Proficiency:** all simple weapons. No martial weapons, armor, or shields.
- Starting gear: `6d8 × 10 mk`.

### Expert (Table 10-15)

- **HD** d6. Above-average BAB (multi-attack at 8th, 15th). **Will** strong. **Defense** +0 → +6. **Reputation Score** to 5.
- Class skills: any 10 (non-channeler).
- **Skill points:** 1st level `(6 + Int) × 4`; thereafter `6 + Int`.
- **Proficiency:** all simple weapons; light armor; no shields.
- *Closest to a hero-worthy NPC class.*

### Warrior (Table 10-16)

- **HD** d8. **BAB** full progression (multi-attack at 6, 11, 16). **Fort** strong. **Defense** +0 → +6. **Reputation Score** to 5.
- Class skills: Climb, Handle Animal, Intimidate, Jump, Ride, Swim.
- **Skill points:** 1st level `(2 + Int) × 4`; thereafter `2 + Int`.
- **Proficiency:** all simple/martial; all armor + shields.
- **Armor Compatibility** — class Defense bonus stacks with armor/shield (mirrors armsman).
- Starting gear: `3d4 × 10 mk`. Used for soldiers, guards, thugs, and Trollocs.

## Implementation Notes (WheelMUD)

- **Class registry:** prestige and NPC classes belong in the same registry as base classes (`classes.md`), with extra fields:
  - `kind: base | prestige | npc`
  - `multiclassDefenseExempt: bool` (true for prestige)
  - `weavesPerDay: { mode: own | inheritFrom(class) }` (channeling prestige inherits initiate or wilder progression at total character level)
  - `requirements: Requirement[]` (BAB, skill ranks, feats, gender, background, special).
- **Channeling prestige multiclass:** when computing weaves/day, sum levels in (initiate + Aes Sedai/Wise One/Windfinder) or (wilder + Aes Sedai/Asha'man) and read the *base* class progression at the total. Slot capacity stays gender-locked.
- **Bonus feats in tables:** model as `feat-grant` events at level-up; track separately from purchased feats so they bypass the prerequisite check (still apply prerequisites where the ruleset requires them — e.g. Asha'man's Improved Initiative still requires no extra preconditions).
- **Stacking-bonus enum:** introduce a `BonusType` taxonomy (`Armor / Circumstance / Competence / Dodge / Enhancement / Morale / NaturalArmor / Racial / Resistance / Synergy`) so the bonus aggregator can apply same-type-don't-stack except for Dodge / Circumstance-from-different-source / Synergy.
- **NPC attitude:** `Attitude` enum (`Hostile / Unfriendly / Indifferent / Friendly / Helpful`) on every NPC, with a `shiftAttitude(diplomacyResult)` function that uses Table 10-2 thresholds. Heroes are *not* affected by this rule — guard the function to refuse hero targets unless explicitly compelled by a weave.
- **Reputation hooks:** prestige class entries deliver +1 Reputation gain per level on most lines; pump these into the existing Reputation system (`heroic-characteristics.md`).
- **Madness state:** keep `madnessRating: int` on every male channeler and on every wolfbrother. Expose triggers as event listeners (`onInjury`, `onWillSave`, `onCastWeave`, `onTouchTerangreal`, `onThreat`); when a trigger fires and `rating ≥ threshold`, run the Will save then the symptom roll. Wolfbrothers must skip the Disease entry entirely.
  - **Disease incubation:** schedule `1 week` then a daily 1d3 Con loss tick with `onConZero → death`.
  - **Dementia:** flip the character into NPC mode (lose player control).
- **Prestige class signature abilities:**
  - **Parting the Silk** — one-shot deterministic max-damage attack; track per-day uses by level slab.
  - **Increased Multiplier** — temporarily raises the weapon's crit multiplier for a single declared hit.
  - **Inspire Courage / Greatness / Competence / Fascinate / Calumny / Jarring Song / Mindbending Melody** — area auras keyed to "audible & visible" range with Will saves vs. Perform.
  - **Strategy / Improved / Superior** — buff effects with progressively longer duration; use a single buff record with a `duration` field.
  - **Sneak Attack progression** — already standard wanderer rule; thief-taker stacks (`+2d6` → `+8d6`).
  - **Uncanny Dodge** — multi-stage flag bag (`keepsDexFlatFooted`, `cantBeFlanked`, `+N vs traps`); ranks combine with wanderer levels for the flank-immunity threshold.
  - **Defensive Awareness** — Warder variant of uncanny dodge; fold into the same flag bag with class-source tagging.
  - **Wolfbrother scent** — perception-pipeline plug-in: range modulated by wind direction, scent strength multiplier (1× / 2× / 3×), with a "follow trail" Wisdom-or-Wilderness-Lore check ignoring surface and visibility penalties.
  - **Tel'aran'rhiod entry** (Wise One Dreamwalk, Wolfbrother Wolf Dream) — dispatches into the world-graph adapter set up for traveling weaves (`the-one-power.md`).
- **Commander logistics ledger:** track outstanding requisition value vs. cap = `level × Cha × 2,000 mk` (×2 at 7th); destroyed equipment debits the cap permanently.
- **Warder bond integration:** the `bondWarder` link (from `the-one-power.md`) must expose a "is bonded" flag — required by Warder prestige class entry and by Defensive Blow's "protecting bonded Aes Sedai" trigger.
- **Adventure XP awarder:** `awardXP(party, length)` = `baseXP[length] × avgLevel(party) ÷ size(party)`. Allow GM override per encounter. Log that an XP grant for a long adventure tends to advance ~1 level for a 4-character party.
- **Encounter environment modifiers:** capture as room-tagged hazards with the `effect` cell from the table above; tick into the combat scheduler for "every round" hazards (whirling blades, steam vents, ice).
- **Save vs. check distinction:** when a system-internal helper has to choose between calling `save()` and `check()`, default to "saves to avoid harm, checks to accomplish a task." Use checks for skill-driven actions (Climb, Open Lock); use saves for level-scaling resistance (Reflex vs. fireball).
- **Madness for wolfbrothers** must not increment on overchannel (they aren't channelers); only on level-up rolls and the optional waive-requirement penalty.
- **Reputation gain per level:** pull from each prestige class's table (varies — 0/0/+1/etc.); fold into the per-level Reputation increase calculator.
- **NPC class progression:** keep these classes in a separate "NPC tier" so adventurers can multiclass *into* them but the validator warns that experts/warriors/diplomats/commoners are weaker than hero classes (matches the book's guidance).
