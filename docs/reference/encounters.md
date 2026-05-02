# Encounters

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 11: Encounters, pp. 226–259) for use in WheelMUD implementation.

## Encounter Categories

### Tailored vs. Status Quo

- **Tailored** — built around specific PCs.
- **Status Quo** — built around the world; PCs adapt or retreat.

### Difficulty Tiers

| Tier | Resource Drain | Frequency in adventure |
|------|---------------|------------------------|
| Simple | ~10% | ~25% (drops to ~10% if rest is plentiful) |
| Challenging | 20–25% | ~50% |
| Extreme | up to 50% | ≤25% (up to 50% with ample rest) |

### Sample Encounters by Challenge Code (Table 11-1)

| Code | Simple Lvl | Challenging Lvl | Extreme Lvl | Sample |
|------|------------|----------------|-------------|--------|
| A | 1–2 | — | — | 2 1st-level warriors / 1 1st-level armsman |
| B | 3–5 | 1–2 | — | 1 2nd-level wanderer / 2 2nd-level warriors |
| C | 6–8 | 3–5 | 1–2 | 1 4th-level wanderer / 2 2nd-level armsmen |
| D | 9–11 | 6–8 | 3–5 | 1 7th-level wanderer / 1 5th-level initiate |
| E | 12–14 | 9–11 | 6–8 | 1 10th-level armsman / 1 8th-level initiate |
| F | 15–17 | 12–14 | 9–11 | 4 9th-level woodsmen / 1 13th-level initiate |
| G | 18–20 | 15–17 | 12–14 | 1 16th-level initiate / 2 14th-level armsmen |
| H | — | 18–20 | 15–17 | 1 19th-level wanderer / 4 15th-level armsmen |
| I | — | — | 18–20 | 2 20th-level initiates |

### Hazard Encounters (Table 11-2)

| Code | One-time damage | Sustained damage |
|------|----------------|------------------|
| A | 1d6 | 1d6/min |
| B | 2d6 | 1d6/5 rounds |
| C | 4d6 | 1d6/round |
| D | 7d6 | 2d6/round |
| E | 10d6 | 3d6/round |
| F | 13d6 | 4d6/round |
| G | 16d6 | 5d6/round |
| H | 19d6 | 6d6/round |
| I | 22d6 | 7d6/round |

Hazards dealing < 1d6/min are environmental, not encounters.

### Miscellaneous Encounters (Table 11-3)

| Code | DC (single check) | DC (multiple checks) |
|------|------------------|----------------------|
| A | 15 | 10 |
| B | 18 | 13 |
| C | 21 | 16 |
| D | 24 | 19 |
| E | 27 | 24 |
| F | 30 | 27 |
| G | 33 | 30 |
| H | 36 | 33 |
| I | 39 | 36 |

## Environment

### Light Sources (Table 11-4)

| Item | Light radius | Duration |
|------|-------------|----------|
| Candle | 5 ft | 1 hr |
| Torch | 20 ft | 1 hr |
| Hooded lantern | 30 ft | 6 hr / pint oil |
| Glowbulb | 60 ft | Permanent |

Without light, characters are effectively blinded.

### Heat & Cold

- Hot/cold day: Fort DC 15 (+1 per previous check) hourly or take 1d6 subdual.
- Desert/arctic: Fort DC every 10 min instead.
- Air above lava: 1d6 normal damage / minute; Fort save every 5 min or 1d4 subdual.
- Heavy clothing/armor: -4 vs. heat saves, +4 vs. cold saves. Wilderness Lore can grant bonus.
- Subdual from temperature can't be healed until back in normal climate; unconscious → starts taking normal damage.

### Starvation & Thirst

- Daily needs: ~1 gallon water + 1 lb decent food (2–3× water in extreme heat).
- Without water: 1 day + Con-score hours, then Con DC `10+1/check` hourly or 1d6 subdual.
- Without food: 3 days, then Con DC daily or 1d6 subdual.
- Subdual from thirst/starvation cannot be healed (even by One Power) until needs met.

### Suffocation & Drowning

- Hold breath: 2 × Con score in rounds.
- Then Con DC `10+1/check` per round.
- On first failure: subdual = current HP (drop to 0, unconscious).
- Round 2: -1 HP and dying. Round 3: dead.

### Smoke

- Fort DC `15+1/check` each round or spend round coughing.
- 2 consecutive choke rounds: 1d6 subdual.
- Subdual from smoke → unconscious → normal damage starts.
- Smoke = ½ concealment (20% miss).

### Falling

- 1d6 / 10 ft. Deliberate jumps: first 1d6 is subdual.
- Jump or Tumble DC 15: ignore first 10 ft and convert next 10 ft to subdual.
- Soft surfaces: convert first 1d6 to subdual (cumulative with above).
- **Into water ≥10 ft deep:** first 20 ft = no damage; next 20 ft = subdual (1d3/10 ft); beyond that = normal (1d6/10 ft). Deliberate dive: Swim/Tumble DC 15 (+5 per 50 ft fallen) for no damage if water is ≥10 ft / 30 ft fallen.

### Poison (Table 11-5)

| Poison | Type | Initial | Secondary |
|--------|------|---------|-----------|
| Knockout drops | Ingested DC 12 | 1d6 Dex | Unconscious & stable |
| Sense-deadening poison | Inhaled DC 12 | 1d6 Wis | 2d6 Wis |
| Weakening poison gas | Inhaled DC 12 | 1d6 Str | 2d6 Str |
| Knockout gas | Inhaled DC 18 | 1d6 Dex | Unconscious & stable |
| Paralytic poison | Injury DC 15 | 1d6 Dex | Paralysis |
| Deadly poison | Ingested DC 15 | 1d6 Con | 2d6 Con |
| Red adder bite | Injury DC 16 | 1d6 Con | 2d6 Con |
| Blood snake bite | Injury DC 17 | 2d6 Con | 4d6 Con |
| Contact poison | Injury DC 18 | 1d4 Con | 2d4 Con |

- Secondary save 1 minute later unless specified.
- 5% chance (natural 1 on d20) to self-expose during application; natural-1 attack with poisoned weapon → Reflex DC 15 or self-poison.
- Status effects (paralysis, unconsciousness) persist 1d3 hours.

### Disease (Table 11-6)

| Disease | Type | Incubation | Initial | Secondary |
|---------|------|-----------|---------|-----------|
| Type I | Ingested/Inhaled DC 13 | 1d6 days | 1 Con | 1d2 Con & 1d2 Str* |
| Type II | Ingested/Inhaled DC 13 | 1d4 days | 1 Str | 1d2 Con* & 1d4 Str |
| Animal-based | Injury DC 15 | 1d4 days | 1 Con & 1 Str | 1d3 Con* & 1d3 Str* |
| Spore-based | Ingested/Injury DC 14 | 2d4 days | 1 Con & 1 Dex | 1d2 Con* & 1d2 Dex* |

\* Second save against starred values: failure = 1 point becomes a permanent drain.
- Two consecutive successful daily saves cure the disease.
- Heal skill aid: substitute Heal result for save if better; subject must rest most of each day.

## Character Conditions Summary

| Condition | Effect (in brief) |
|-----------|------------------|
| Ability Damaged | Temporary; heals 1/day; Str 0 = helpless prone, Dex 0 = paralyzed, Con 0 = dead, Int/Wis/Cha 0 = unconscious |
| Ability Drained | Permanent loss |
| Blinded | Full concealment to all foes; 50% miss; lose Dex to Defense; +2 to attackers; speed × ½; -4 to most Str/Dex skills |
| Checked | Forward motion blocked (e.g. by wind) |
| Cowering | Frozen; lose Dex to Defense; can't act; foes +2 attack |
| Dazed | No actions; defends normally |
| Deafened | -4 initiative; no Listen checks |
| Disabled | 1 action/round; strenuous action → 1 dmg, may revert to dying |
| Dying | Unconscious; 10%/round to stabilize, else -1 HP |
| Entangled | -2 attack, -4 effective Dex; ½ speed; no run/charge; Concentration DC 15 to cast |
| Exhausted | ½ speed; -6 effective Str/Dex; 1 hr rest → fatigued |
| Fatigued | No run/charge; -2 effective Str/Dex; 8 hr rest cures |
| Flat-Footed | Lose Dex to Defense |
| Frightened | Flee or fight; -2 attack/dmg/saves |
| Grappled | Severely restricted actions; lose Dex to Defense vs. non-grapplers |
| Held | Helpless; mental actions only |
| Helpless | Bound/asleep/paralyzed/unconscious; melee +4, Dex effective 0, modifier -5 |
| Panicked | -2 morale saves; flee; 50% drop items; cowers if cornered |
| Paralyzed | Helpless; Str/Dex effective 0; mental actions only |
| Pinned | Held immobile (not helpless) |
| Prone | -4 melee attack; only crossbow useful at range; foes +4 melee, -4 ranged |
| Shaken | -2 morale attack/dmg/saves |
| Stable | Negative HP, no longer dying |
| Staggered | Subdual = current HP; one move OR attack action/round |
| Stunned | Lose Dex to Defense; no actions; foes +2 attack |
| Unconscious | Helpless |

## Creatures

### Statblock Format

- **Name / Size & Type** — type ∈ Animal / Exotic / Shadowspawn (no multi-typing).
- **Hit Dice / Initiative / Speed** — speed entries may include `climb`, `fly` (with maneuverability tier `perfect / good / average / poor / clumsy`), or `swim`. Climb/swim grant +8 racial. Fly + dive attack works like a charge (≥30 ft, claw only, double damage).
- **Defense / Attacks / Damage / Face/Reach**.
- **Special Attacks** (Improved Grab, Gaze, etc.) and **Special Qualities** (Blindsight, Damage Reduction, Low-Light Vision, One Sense, Scent, etc.).
- **Saves / Abilities / Skills / Feats**.
- **Climate/Terrain / Organization / Challenge Code / Advancement**.

#### Size Modifiers (Table 11-7)

| Size (example) | Mod | Length | Weight |
|---------------|-----|--------|--------|
| Colossal (great whale) | -8 | ≥64 ft | >250,000 lb |
| Gargantuan (to'raken) | -4 | 32–64 ft | 32k–250k lb |
| Huge (raken) | -2 | 16–32 ft | 4k–32k lb |
| Large (lopar) | -1 | 8–16 ft | 500–4,000 lb |
| Medium (human) | +0 | 4–8 ft | 60–500 lb |
| Small (eagle) | +1 | 2–4 ft | 8–60 lb |
| Tiny (rat) | +2 | 1–2 ft | 1–8 lb |
| Diminutive (toad) | +4 | 6–12 in | ⅛–1 lb |
| Fine (fly) | +8 | ≤6 in | <⅛ lb |

### Special Abilities

- **Improved Grab** — on melee hit: free grapple, no AoO, no touch attack, no special size penalty for Tiny/Small. Works only vs. foes ≥1 size smaller. Optional "hold" mode: -20 grapple but stay non-grappled (keep Dex, threaten, attack others).
- **Gaze** — typical 30 ft range; save (usually Will or Fort) DC `10 + ½HD + Cha mod`. Each foe in range saves at start of own turn. Avoidance: avert eyes (50% chance to skip save) or blindfold (target gets total concealment vs. you). Active gaze as attack action forces an extra save (round can have two). Self-immune by default.
- **Blindsight** — fights as sighted; invisibility/darkness irrelevant; no Spot/Listen needed within range.
- **Damage Reduction X/Y** — ignore X damage from attacks unless they meet `Y` (e.g. `5/+1` requires +1 enhancement). Energy attacks bypass DR. Natural weapons of the same creature ignore its own DR.
- **Low-Light Vision** — see 2× human in starlight/moonlight/torchlight; full color/detail.
- **One Sense** — sense embracing/casting within 60 ft via Spot; casting level adds to Spot bonus.
- **Scent** — detect at 30 ft (60 upwind / 15 downwind; ×2 strong, ×3 overpowering); follow tracks via Wis check (DC 10 +2/hour cold).

### Advancement (Table 11-8)

| Type | HD | Attack Bonus | Good Saves | Skill Pts | Feats |
|------|----|-------------|-----------|-----------|-------|
| Animal | d8 | ¾ × HD (noble) | as armsman | 10–15 | — |
| Exotic | d10 | full HD (armsman) | as armsman | +1/extra HD | +1/extra HD |
| Shadowspawn | d8 | full HD (armsman) | as armsman | +2/extra HD | +1/4 HD |

Saves use armsman of equivalent level as base.

### Size Increase Effects (Table 11-9)

| New Size | Str | Dex | Con | Natural Armor | Defense/Attack |
|----------|-----|-----|-----|---------------|----------------|
| Diminutive | — | -2 | — | — | -4 |
| Tiny | +2 | -2 | — | — | -2 |
| Small | +4 | -2 | — | — | -1 |
| Medium | +4 | -2 | +2 | — | -1 |
| Large | +8 | -2 | +2 | +2 | -1 |
| Huge | +8 | -2 | +4 | +3 | -1 |
| Gargantuan | +8 | — | +4 | +4 | -2 |
| Colossal | +8 | — | +4 | +5 | -4 |

Apply repeatedly when jumping multiple categories.

### Creature Feats (unique to creatures)

- **Flyby Attack** — fly speed required; attack at any point of move action; no second move that round.
- **Multiattack** — ≥3 natural weapons; secondary natural attacks at -2 instead of -5.

## Creature Roster

Compact reference; full statblocks in source. CC = Challenge Code.

### Animals

| Creature | Size | HD | CC | Notes |
|----------|------|----|----|-------|
| Mountain Cat | Medium | 3d8+6 | C | Pounce, improved grab, rake 1d4+1; Climb 20 ft; +4 Hide/Move Silently, +8 Balance. |
| Wolf | Medium | 2d8+4 | A | Wolfspeech (telepathy ≤100 mi); +5 Hide; pack flank tactics; reincarnates in *Tel'aran'rhiod*. |
| S'redit | Huge | 11d8+55 | E | Trample 2d8+15 (Reflex DC 20 half); Seanchan domesticated "boar-horse." |
| Horse, Heavy War | Large | 4d8+12 | B | Hoof 1d6+4 / Bite 1d4+2; rider attacks need Ride DC 10. |
| Horse, Light | Large | 3d8+6 | A | Hoof 1d4+1; cannot fight while ridden. |
| Horse, Light War | Large | 3d8+9 | A | Hoof 1d4+3 / Bite 1d3+1; rider attacks need Ride DC 10. |

#### Horse Carrying Capacity

| Type | Light | Medium | Heavy | Drag |
|------|-------|--------|-------|------|
| Light horse | ≤150 lb | 151–300 | 301–450 | 2,250 |
| Light warhorse | ≤230 lb | 231–460 | 461–690 | 3,450 |
| Heavy warhorse | ≤300 lb | 301–600 | 601–900 | 4,500 |

### Exotics (Seanchan)

| Creature | Size | HD | CC | Notes |
|----------|------|----|----|-------|
| Corlm | Medium | 2d10+6 | A | Tracker; +8 Listen / Wilderness Lore; flees when injured. |
| Grolm | Medium | 3d10 | B | Bite + 2 claws; trip on hit (free, no AoO); DR 8/—; +4 Jump/Spot, +8 Sense Motive. |
| Lopar | Large | 8d10+40 | C | Improved grab; rears for reach; barding +2 (leather, 30 lb, 20 mk) or +4 (plate, 50 lb, 20 gc). |
| Raken | Huge | 5d10+10 | C | Fly 180 (good); 1 morat'raken → 400 mi range; 2 → scouting only. |
| To'raken | Gargantuan | 7d10+21 | D | Fly 120 (poor); carries 1,000 lb 200 mi or 1 rider 1,000 mi; lands if hurt. |
| Torm | Large | 6d10+18 | C | Frenzy (+4 Str/Con, +2 Will, -2 Defense; 3d6×10 min cooldown); Handle Animal DC `10+2/round` to suppress. |

### Shadowspawn

| Creature | Size | HD | CC | Notes |
|----------|------|----|----|-------|
| Darkhound (lesser) | Medium | 8d8+32 | D | Bite 1d8+6 + poison Fort DC 18; poisonous blood; never crosses running water. |
| Darkhound (greater) | Medium | 8d8+40 | E | Bite 2d6+7 + poison Fort DC 19; **Regeneration 5** (only One Power kills); poisonous blood. |
| Draghkar | Medium | 2d8 | C | Captivating Song (Will DC 19, 120 ft); Kiss → Fort DC 19 or 1d6 perm Wis drain; Wis 0 = soul destroyed. |
| Gholam | Medium | 10d8+50 | G | DR 5/+1; **Boneless** (slip through 1/16 in; immune to sneak/crit); **Weave Immunity**; vulnerable to anti-Power *ter'angreal* (touch attack 1d8). |
| Gray Man | Medium | 4d8+12 | E | Beneath notice (Hide in plain sight until acts); Sneak Attack +3d6; Death Attack (3 rounds study + sneak attack → Fort DC 14 or die). |
| Myrddraal | Medium | 9d8+36 | E | Black plate (+4 armor); Shadow-blade (1d10+4 + Fort DC 18 disease 1d6 Con); Fear Gaze (Will DC 17, 30 ft); Blindsight; Trolloc Link (1d6+20); Shadow Walk; **Dark Vitality** — won't die until next sundown unless above -10 HP. |
| Trolloc | Large | 3d8+3 | A | Scythesword 2d4+3 / Shortbow 1d6; light sensitivity -2 attack in bright light; linked Trollocs die when their Myrddraal dies; gangs (2–6) / bands (11–20 + sergeant + sometimes Myrddraal) / fists (100–200 + 5 sergeants + leader 3–5 + 1–4 Myrddraal). |
| Shadow-Linked Rat | Tiny | ¼ d8 | A | Spy; +4 Hide/Move Silently, +8 Balance. |
| Shadow-Linked Raven/Crow | Tiny | ¼ d8 | A | Spy; Fly 40 (average); +6 Listen/Spot. |

### Notable Creature Mechanics (highlights)

- **Frenzy (torm)** — bestial rage state with stat bonus and Defense penalty; stops 3d6×10 min after combat ends.
- **Regeneration (greater Darkhound)** — only weaves of the One Power deal real damage; other damage heals at 5 HP/round.
- **Trolloc/Myrddraal Link** — psychic bond giving the Myrddraal ld6+20 linked Trollocs; on the Myrddraal's death, all linked Trollocs convulse and die in a few rounds.
- **Shadow Walk (Myrddraal)** — instant teleport between shadows over miles; cannot enter own shadow.
- **Dark Vitality (Myrddraal)** — does not die until next sunset even at ≤-10 HP; can be saved by lifting above -10 with the One Power before then.
- **Captivating Song (Draghkar)** — free action targeting one creature within 120 ft; victim approaches automatically until kissed.
- **Death Attack (Gray Man)** — must study 3 rounds, then make sneak attack within next 3 rounds; failure resets the study window.
- **Boneless (gholam)** — slip through any 1/16-inch gap; immune to crit/sneak attack.
- **Weave Immunity (gholam)** — direct One Power attacks fail; secondary effects (e.g. hurled rock) still apply.

## Major NPCs (Dumai's Wells era — high-level statblock notes)

The chapter ends with full statblocks for ~15 named characters from the novels. They are model NPCs, not lookup tables, so this doc captures the rules-relevant patterns rather than every line.

### Class Builds

| Character | Build |
|-----------|-------|
| Rand al'Thor | Midlander Woodsman 1 / Armsman 4 / Wilder 12 / Blademaster 2 |
| Matrim Cauthon | Midlander Wanderer 9 / Commander 6 |
| Perrin Aybara | Midlander Woodsman 5 / Wolfbrother 10 |
| Egwene al'Vere | Midlander Initiate 8 / Aes Sedai 7 |
| Nynaeve al'Meara | Midlander Woodsman 1 / Wilder 14 |
| Elayne Trakand | Midlander Noble 1 / Initiate 8 / Aes Sedai 4 |
| Aviendha | Aiel Algai'd'siswai 4 / Initiate 6 / Wise One 3 |
| Moiraine Damodred | Cairhienin Initiate 8 / Aes Sedai 7 |
| Min Farshaw | Midlander Wanderer 9 (Latent Viewer + Viewing) |
| Loial | Ogier Wanderer 9 (Latent Treesinger + Treesinger) |
| al'Lan Mandragoran | Borderlander Armsman 10 / Warder 4 / Blademaster 2 |
| Thomdril Merrilin | Cairhienin Wanderer 7 / Gleeman 7 |
| Dain Bornhald | Midlander Armsman 9 / Commander 1 |
| Padan Fain | Midlander Wanderer 16 |

### Ta'veren Bookkeeping

- **Ta'veren bonus:** +2 Charisma (or other ability per the GM) marked in the statblock; vanishes if the Pattern releases the character.
- **Bonus feats:** several feats are granted *because* of ta'veren status (e.g. Rand's Extra Affinity (×4), Extra Talent (×4); Mat's Two-Weapon Fighting / Improved Two-Weapon Fighting / The Dark One's Own Luck ×3; Perrin's Skill Emphasis grants and Trustworthy). They must be flagged to be removable on status loss.
- **Madness rating** is on the sheet for male channelers (Rand: 31) and wolfbrothers (Perrin: 15).

### Useful Build Notes for World-Builders

- **Rand** carries a +2 Power-wrought Warder's sword and a +3 angreal; knows Balefire, Bridge Between Worlds, Create Gateway, Use Portal Stone, Strike of Death, Ward Bore — the lost-weave inventory only Rand-tier characters get.
- **Mat** carries a +2 Power-wrought *ashandarei* and the foxhead medallion *ter'angreal* (see Chapter 14: Wondrous Items).
- **Egwene** wears the Amyrlin shawl; has Dreamwalking line of feats plus Aes Sedai Extra Affinity/Extra Talent grants.
- **Nynaeve** has the Block (cannot channel unless angry); represented as a wilder feature.
- **al'Lan** has the Blooded feat tree (+2 to initiative bundled into Improved Initiative + Blooded), Warder bond, masterpiece Warder's sword.
- **Loial**'s `reach 10 ft` is intrinsic to Ogier size; treesinger feat grants Craft (treesinging).
- **Padan Fain** is a 16th-level wanderer with Ordeith/Mordeth fusion; mechanically he's a high-Dex wanderer with sneak attack +4d6 — special tainting effects are GM-narrated, not in the statblock.

## Implementation Notes (WheelMUD)

- **Encounter classification:** introduce an `Encounter` type with fields `code` (A–I), `kind` (Combat / Hazard / Misc), `tier` (Simple / Challenging / Extreme), and `expectedResourceDrain` (10/25/50). Drive XP awards (`gamemastering.md`) by `tier × baseXP` rather than per-monster bookkeeping for status-quo encounters.
- **Hazard encounters:** generic `HazardEncounter` runs either a one-shot damage event or a periodic damage tick keyed off the round/minute scheduler. Auto-promote to "encounter" only when damage ≥ 1d6/min.
- **Miscellaneous encounters:** wrap any skill-only challenge in a `SkillChallenge` aggregate that sums DC against `single` or `multiple` columns.
- **Environmental hazards:**
  - Light/dark — already a render flag; pin `lightRadius` to source items per Table 11-4.
  - Heat/cold/desert/arctic/lava — periodic Fort save tickers tied to the climate of the room/region; `subdualLockUntilLeaveClimate` flag prevents healing.
  - Starvation/thirst/suffocation/smoke — separate counters whose subdual damage cannot be healed until the prerequisite is met (food/water/breath/clean air).
  - Falling — single resolver function `resolveFall(distance, deliberate, surface, jumpSkill, water)` that applies the staircase of subdual/normal split.
  - Poison — `Poison` value object: `(deliveryType, saveDC, initial, secondary, secondaryDelay, statusOnSecondary)`; user attacks roll application self-expose (5%) and natural-1 self-poison (Reflex DC 15).
  - Disease — incubation timer + per-day Fort save loop with optional starred-secondary "permanent drain" sub-save.
- **Condition system:** standardize a `Condition` enum + applied-condition record (with source, duration, modifier set). Many entries combine multiple flags (helpless = lose Dex bonus + Dex effective 0 + sneak-attack-eligible + auto-crit on coup de grace). Centralize the rules in one resolver so combat code stays small.
- **Creature data model:** match the statblock fields one-to-one. Extra fields:
  - `creatureType ∈ Animal | Exotic | Shadowspawn` (single value; never multi-typed).
  - `subtype` for things like `Shadow-linked vermin`, used by `Strike of Death` and `Ward against Shadowspawn`.
  - `naturalAttacks: Attack[]` separate from `manufacturedWeapons` so the secondary-penalty rule can pick the right modifier (-5 default, -2 with Multiattack).
  - `specialAttacks` / `specialQualities` as named-trait records with their own resolver hooks (improved grab, gaze, regeneration, DR, etc.).
- **Movement modes:** `speed` is a map keyed by mode (`land`, `climb`, `fly{maneuverability}`, `swim`); fliers may "run" only in straight lines; climbers and swimmers always take 10. Dive attack is a Charge variant gated on `mode == fly` with min 30 ft straight-line move.
- **Improved grab:** integrate with the grapple state machine from `combat.md` — on melee-attack hit, branch to a "free grapple attempt" path that skips the touch attack and AoO; if the creature opts for "hold," set its grapple to a `-20`-bonus state and clear the standard grapple lockouts.
- **Gaze attack:** publish a per-room or per-encounter `GazeSource` event each round; subscribers (anyone within range) save at the start of their turn. Track `avertingEyes`/`blindfolded` per character for the 50%-skip / total-concealment branches.
- **Damage Reduction:** generalized `damageMitigation(damage, source) → finalDamage` on the creature; sources tagged with `enhancementBonus`, `material`, `isEnergy`, etc. so the resolver can select what bypasses DR. Energy attacks bypass DR universally.
- **Regeneration:** subtype of healing-tick listener; ignore damage source unless tag matches the regen exception (One Power for greater Darkhound).
- **Weave Immunity (gholam) / Boneless / Dark Vitality / Shadow Walk / Captivating Song / Death Attack / Trolloc Link:** model each as a named trait with explicit hooks (`onDirectWeave`, `onSqueezeThrough`, `onHpBelowThreshold`, `onTeleportRequest`, `onTurnStart`, `onStudyComplete`, `onMyrddraalDeath`). Avoid hardcoding behavior into the creature class itself.
- **Advancement:** `advanceCreature(creature, addedHD)` applies the row from Table 11-8 (BAB, saves, skill points, feats per type). Size-up applies Table 11-9 deltas, repeating per category jumped.
- **NPC class roster:** the named-character stats are seed data — they belong in a YAML/SQL fixture used to spawn NPCs, not in code. The fixture must support `taverenBonuses` (a flag-list of `feat-grant` and `ability-bonus` entries that can be flipped off if ta'veren status ends).
- **Reputation in encounters:** the chapter assumes Reputation flows during interaction encounters; tie into the existing Reputation-check pipeline from `heroic-characteristics.md`.
- **Challenge codes as XP gates:** XP isn't strictly per-creature — encounter-level award (Code A → I) maps cleanly to the adventure-tier XP table from `gamemastering.md`. Implement the Challenge-Code → expected-party-level lookup so admins can balance by intent rather than by adding up CRs.
- **Shadow-linked vermin:** these aren't full encounter combatants by default — they're spies. Wrap them in a "spy" reporting subsystem that triggers narrative consequences (Forsaken/Myrddraal hears about a target) rather than only as combat tokens.
- **Mortar between weave and creature systems:** several creature traits cross-cut with `the-one-power.md` (e.g. `Sense Shadowspawn` weave and the `Shadowspawn` creature type; `Ward against Shadowspawn` uses the same enum; gholam's Weave Immunity short-circuits the casting pipeline). Centralize the creature-type enum in shared code.
