# Feats

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 5: Feats, pp. 88–101) for use in WheelMUD implementation.

## Overview

A feat is a special feature that gives a character a new capability or improves an existing one. Unlike skills, feats have no ranks — a character either has the feat or does not.

## Acquiring Feats

- 1 feat at character creation, plus extra feats from background and class.
- 1 additional feat at 3rd level and every 3 levels thereafter (6th, 9th, 12th, ...). Multiclass characters track total character level for feat progression.
- Initiates and wilders gain bonus channeling feats from a restricted list (channeling feats only, with the exception of Mental Stability for male channelers).

## Prerequisites

- Some feats require a minimum ability score, another feat, a skill rank, or a base attack bonus (BAB).
- A feat may be selected at the same level its prerequisite is gained.
- Losing a prerequisite (e.g. Str drops below 13 from poison) suspends the dependent feat until restored.

## Types of Feats

- **General** — no special grouping rules.
- **Special** — restricted to a specific class (e.g. Eliminate Block for wilders, Weapon Specialization for armsmen/woodsmen). Described in the class entries.
- **Channeling** — only initiates and wilders may take these. They modify how channelers use the One Power.
- **Lost Ability** — supernatural gifts (Dreamwalking, Foretelling, Old Blood, Sniffing, Treesinging, Viewing). Each requires a `Latent <ability>` feat as a prerequisite "doorway"; the latent feat itself confers no benefit. GM approval required.

## Feat Description Format

Each feat entry in the source uses:

- **Description** — plain-language summary.
- **Prerequisite** — required score / feat / skill / BAB / level (omitted if none).
- **Benefit** — what the feat enables.
- **Normal** — restriction on characters lacking the feat (omitted if irrelevant).
- **Special** — extra notes (e.g. stackability, multi-take rules).

## Feat Index (Table 5-1)

### General Feats

| Feat | Prerequisite | Notes |
|------|--------------|-------|
| Alertness | — | |
| Ambidexterity | Dex 15+ | |
| Animal Affinity | — | |
| Armor Proficiency (light) | — | Free for most classes |
| Armor Proficiency (medium) | Armor Proficiency (light) | |
| Armor Proficiency (heavy) | Armor Proficiency (light), Armor Proficiency (medium) | |
| Athletic | — | |
| Blind-Fight | — | |
| Cleave | Str 13+, Power Attack | |
| Combat Casting | — | Channeling list (see below) |
| Combat Expertise | Int 13+ | |
| Combat Reflexes | — | |
| The Dark One's Own Luck | — | Stackable |
| Dodge | Dex 13+ | |
| Endurance | — | |
| Exotic Weapon Proficiency* | BAB +1 | Multi-take per weapon |
| Fame | — | +3 Reputation |
| Far Shot | Point Blank Shot | |
| Great Cleave | Str 13+, Power Attack, Cleave, BAB +4 | |
| Great Fortitude | — | +2 Fortitude saves |
| Heroic Surge | — | Once/day per 4 character levels |
| Improved Bull Rush | Str 13+, Power Attack | |
| Improved Critical* | Proficient w/ weapon, BAB +8 | Multi-take per weapon |
| Improved Disarm | Int 13+, Combat Expertise | |
| Improved Initiative | — | +4 initiative |
| Improved Trip | Int 13+, Combat Expertise | |
| Improved Two-Weapon Fighting | Two-Weapon Fighting, Ambidexterity, BAB +9 | |
| Improved Unarmed Strike | — | |
| Infamy | — | Reputation = Infamous |
| Iron Will | — | +2 Will saves |
| Lightning Reflexes | — | +2 Reflex saves |
| Low Profile | — | Slows Reputation gain |
| Martial Weapon Proficiency* | — | Multi-take; armsmen/woodsmen get all |
| Mental Stability** | — | Stackable; primarily channelers/wolfbrothers |
| Mimic | — | |
| Mobility | Dex 13+, Dodge | |
| Mounted Archery | Ride, Mounted Combat | |
| Mounted Combat | Ride | |
| Nimble | — | |
| Persuasive | — | |
| Point Blank Shot | — | |
| Power Attack | Str 13+ | |
| Precise Shot | Point Blank Shot | |
| Quick Draw | BAB +1 | |
| Quickness** | — | Stackable |
| Rapid Shot | Point Blank Shot, Dex 13+ | |
| Ride-By Attack | Ride, Mounted Combat | |
| Run | — | |
| Sharp-Eyed | — | |
| Shield Proficiency | — | |
| Shot on the Run | Point Blank Shot, Dex 13+, Dodge, Mobility | |
| Simple Weapon Proficiency | — | Free for all except initiates/wilders |
| Skill Emphasis* | — | Multi-take per skill |
| Spirited Charge | Ride, Mounted Combat, Ride-By Attack | |
| Spring Attack | Dex 13+, Dodge, Mobility, BAB +4 | |
| Stealthy | — | |
| Toughness** | — | +3 HP, stackable |
| Track | — | |
| Trample | Ride, Mounted Combat | |
| Trustworthy | — | |
| Two-Weapon Fighting | — | |
| Weapon Finesse* | Proficient w/ weapon, BAB +1 | Multi-take per weapon |
| Weapon Focus* | Proficient w/ weapon, BAB +1 | Multi-take per weapon |
| Whirlwind Attack | Int 13+, Combat Expertise, Dex 13+, Dodge, Mobility, BAB +4, Spring Attack | |

\* = May be taken multiple times; effects do **not** stack — each instance applies to a new weapon/skill/Affinity/Talent.
\** = May be taken multiple times; effects **do** stack.

### Special Feats (described in class entries)

| Feat | Prerequisite |
|------|--------------|
| Eliminate Block | Wilder |
| Weapon Specialization* | Armsman 4th or Woodsman 6th+ |

### Channeling Feats (Initiates / Wilders only)

| Feat | Prerequisite |
|------|--------------|
| Combat Casting | — |
| Extra Affinity* | — |
| Extra Talent* | — |
| Multiweave** | Wis 13+ |
| Power-Heightened Senses | — |
| Sense Residue | — |
| Tie Off Weave | Wis 13+ |
| Mental Stability** | — (also listed under General; only male channelers may purchase via channeling slot) |

### Lost Ability Feats

| Feat | Prerequisite |
|------|--------------|
| Latent Dreamer | — |
| Dreamwalk | Latent Dreamer |
| Bend Dream | Dreamwalk |
| Dream Jump | Dreamwalk |
| Waking Dream | Dreamwalk |
| Dreamwatch | Latent Dreamer |
| Latent Foreteller | — |
| Foreteller | Latent Foreteller |
| Latent Old Blood | — |
| Old Blood | Latent Old Blood |
| Latent Sniffer | — |
| Sniffer | Latent Sniffer |
| Latent Treesinger | — (Ogier only) |
| Treesinger | Latent Treesinger |
| Tree Warden | Latent Treesinger |
| Latent Viewer | — |
| Viewing | Latent Viewer |

## General & Special Feats — Mechanical Summary

| Feat | Effect |
|------|--------|
| **Alertness** | +2 circumstance bonus on Listen and Spot. |
| **Ambidexterity** | Removes the off-hand penalty (-4 to attack/ability/skill checks) for using either hand. Stacks with Two-Weapon Fighting. |
| **Animal Affinity** | +2 circumstance bonus on Handle Animal and Ride. |
| **Armor Proficiency (light/medium/heavy)** | While wearing armor of the listed type, the armor check penalty applies only to Balance, Climb, Escape Artist, Hide, Jump, Move Silently, Pick Pocket, Tumble. Without proficiency, penalty applies to attack rolls and all movement-based skills (incl. Ride). |
| **Athletic** | +2 circumstance bonus on Climb and Swim. |
| **Blind-Fight** | Reroll miss-chance once per missed melee attack vs. concealed foes. Invisible attackers get no melee bonus, do not strip Dex bonus. Half the usual speed reduction in poor visibility. |
| **Cleave** | If you drop a creature, gain an immediate extra melee attack against another adjacent foe at the same bonus. Once per round. No 5-ft step before the extra attack. |
| **Combat Expertise** | When using attack/full-attack action in melee, may take up to -5 attack penalty (max = BAB) and add the same number as a dodge bonus to Defense until your next action. |
| **Combat Reflexes** | Bonus AoOs per round = Dex modifier (in addition to the standard 1). Still only one AoO per enemy. May make AoOs while flat-footed. |
| **The Dark One's Own Luck** | Once/day, reroll any single die roll where luck is the primary factor (GM call). Take the higher result. Stackable: each extra purchase = +1 reroll/day. |
| **Dodge** | +1 dodge bonus to Defense vs. one designated opponent (re-designate any action). Lost when Dex bonus is lost. Dodge bonuses stack. |
| **Endurance** | +4 circumstance bonus on physical-extension checks (running, swimming, holding breath, etc.). |
| **Exotic Weapon Proficiency** | Normal attack rolls with the chosen exotic weapon. Trolloc scythesword requires Str 16+. |
| **Fame** | +3 Reputation. |
| **Far Shot** | Projectile range increment x1.5; thrown weapon range increment x2. |
| **Great Cleave** | As Cleave, but no per-round limit. |
| **Great Fortitude** | +2 circumstance bonus on Fortitude saves. |
| **Heroic Surge** | Extra move or attack action before/after your normal action. Usable once/day per 4 character levels, max once/round. |
| **Improved Bull Rush** | No AoO from defender on a bull rush. |
| **Improved Critical** | Doubles threat range with the chosen weapon (e.g. longsword 19-20 → 17-20). |
| **Improved Disarm** | No AoO when attempting to disarm; opponent gets no chance to disarm you. |
| **Improved Initiative** | +4 circumstance bonus on initiative. |
| **Improved Trip** | On a successful trip, immediately make a melee attack against the tripped foe at full BAB. |
| **Improved Two-Weapon Fighting** | Second off-hand attack at -5 (in addition to the one granted by Two-Weapon Fighting). |
| **Improved Unarmed Strike** | You count as armed when unarmed; no AoO from armed foes when you attack unarmed. You still get an AoO vs. unarmed attackers. |
| **Infamy** | Reputation rolls treat you as Infamous regardless of score. |
| **Iron Will** | +2 circumstance bonus on Will saves. |
| **Lightning Reflexes** | +2 circumstance bonus on Reflex saves. |
| **Low Profile** | Reputation gain reduced to 1 point per 5 levels. Not retroactive. |
| **Martial Weapon Proficiency** | Normal attack rolls with the chosen martial weapon. Armsmen/woodsmen are proficient with all martial weapons. |
| **Mental Stability** | -20 to Madness rating per purchase (stackable). Mainly for male channelers and wolfbrothers. |
| **Mimic** | +2 circumstance bonus on Disguise and Perform. |
| **Mobility** | +4 dodge bonus to Defense vs. AoOs caused by movement out of/within a threatened square. Lost when Dex bonus is lost. |
| **Mounted Archery** | Mounted ranged-attack penalty halved: -2 (double move) / -4 (run) instead of -4/-8. |
| **Mounted Combat** | Once per round when your mount is hit, attempt a Ride check vs. the attack roll to negate the hit. |
| **Nimble** | +2 circumstance bonus on Escape Artist and Pick Pocket. |
| **Persuasive** | +2 circumstance bonus on Bluff and Intimidate. |
| **Point Blank Shot** | +1 circumstance bonus on attack and damage with ranged weapons within 30 ft. |
| **Power Attack** | Subtract up to BAB from melee attack rolls and add the same number to melee damage; lasts until next action. |
| **Precise Shot** | No -4 penalty for ranged attacks against foes engaged in melee. |
| **Quick Draw** | Draw a weapon as a free action. |
| **Quickness** | (Stackable; entry described under wilder/initiate text — bonus speed-related.) |
| **Rapid Shot** | One extra ranged attack at highest BAB during a full-attack; all ranged attacks suffer -2. |
| **Ride-By Attack** | While mounted-charging, may move, attack, then continue moving up to 2x mounted speed. No AoO from the target. |
| **Run** | Run at x5 base speed (instead of x4). Running jumps gain +25% distance/height (capped at max). |
| **Sharp-Eyed** | +2 circumstance bonus on Search and Sense Motive. |
| **Shield Proficiency** | Use a shield with only its standard penalties. Without it: armor check penalty applies to attack and movement skills. |
| **Shot on the Run** | With ranged attack action, may move both before and after the attack (total move ≤ speed). |
| **Simple Weapon Proficiency** | Normal attack rolls with simple weapons. All classes except initiates/wilders are auto-proficient. |
| **Skill Emphasis** | +3 bonus to checks with the chosen skill. |
| **Spirited Charge** | Mounted charge deals 2x damage with melee weapons (3x with a lance). |
| **Spring Attack** | With melee attack action, may move both before and after the attack (total move ≤ speed). No AoO from defender. Cannot use in heavy armor. |
| **Stealthy** | +2 circumstance bonus on Hide and Move Silently. |
| **Toughness** | +3 HP. Stackable. |
| **Track** | Use Wilderness Lore to find/follow tracks. DC by surface (Very soft 5 / Soft 10 / Firm 15 / Hard 20). Move at half speed (or full speed at -5). Modifiers apply for group size, creature size, age of trail, weather, visibility, hidden trail. Retry after 1 hour outdoors / 10 minutes indoors. |
| **Trample** | When mounted-overrunning, target cannot avoid you; if knocked down, mount makes a hoof attack at +4 vs. prone. |
| **Trustworthy** | +2 circumstance bonus on Diplomacy and Gather Information. |
| **Two-Weapon Fighting** | Reduce two-weapon penalties by 2. |
| **Weapon Finesse** | With the selected light weapon (or rapier/Warder's sword in one hand), use Dex modifier instead of Str on attack rolls. Shield armor check penalty still applies to attacks. |
| **Weapon Focus** | +1 to attack rolls with the chosen weapon. May choose "unarmed strike" or "grapple". |
| **Whirlwind Attack** | Replace full-attack with one melee attack at full BAB against every opponent within 5 ft. |

### Track DC Modifiers

| Condition | DC Modifier |
|-----------|-------------|
| Every 3 creatures in the tracked group | -1 |
| Creature size: Fine / Diminutive / Tiny / Small / Medium / Large / Huge / Gargantuan / Colossal | +8 / +4 / +2 / +1 / +0 / -1 / -2 / -4 / -8 |
| Every 24 hours since trail was made | +1 |
| Every hour of rain since trail was made | +1 |
| Fresh snow cover since trail was made | +10 |
| Overcast or moonless night | +6 |
| Moonlight | +3 |
| Fog or precipitation | +3 |
| Tracked party hides trail (moves at half speed) | +5 |

For mixed-size groups, apply only the largest size modifier. For visibility, apply only the largest visibility modifier.

## Channeling Feats — Mechanical Summary

| Feat | Effect |
|------|--------|
| **Combat Casting** | +4 circumstance bonus on Concentration checks to cast on the defensive. |
| **Extra Affinity** | Gain an Affinity with one of the Five Powers beyond your starting Affinity. Females must take Air/Water/Spirit before Earth/Fire; males must take Earth/Fire/Spirit before Air/Water. May be taken up to 4 times. |
| **Extra Talent** | Pick a new Talent (learn and cast weaves within it). May be taken multiple times. |
| **Multiweave** | While holding one cast weave, cast a second one with a Concentration check (DC 15). Failure: cannot cast second without releasing first. Distractions force Concentration checks for both weaves. Stackable: each instance allows one more held weave. |
| **Power-Heightened Senses** | While embracing the One Power, +4 circumstance bonus on Listen and Spot. |
| **Sense Residue** | Make a Weavesight check (base DC 5) to notice residue of recently cast/released weaves. Second check to identify or learn the weave. |
| **Tie Off Weave** | "Tie off" a Concentration-duration weave so it persists without holding. To release: must see the weave. Duration once tied = (channeler level in days) − (4 × weave casting level in hours). Tying off is an attack or move action. |

## Lost Ability Feats — Mechanical Summary

### Dreamwalking line

| Feat | Effect |
|------|--------|
| **Latent Dreamer** | Prerequisite-only; no benefit. |
| **Dreamwalk** | While asleep, enter Tel'aran'rhiod. Default appearance = current real-world location and equipment. May arrive elsewhere via Concentration check (DC 15 very familiar / 20 somewhat familiar / 25 visited briefly / 30 never seen). Injuries/death in Tel'aran'rhiod carry over to the real world. Exit early via Concentration DC 15 (else wake naturally). |
| **Bend Dream** | While in Tel'aran'rhiod or another's dream, alter dream-stuff with Concentration checks. Self: DC 10 to change clothing/gear, DC 20 to change physical features (also requires Concentration if distracted, like holding a weave). Others: DC +5 if target lacks Bend Dream, otherwise opposed Concentration. Cannot change "native" dream world objects or items physically brought into Tel'aran'rhiod. |
| **Dream Jump** | Travel to any envisioned location in Tel'aran'rhiod with Concentration DC 15 (move action). +5 if extremely familiar or visible. Observers may track the jump with Spot DC 20. |
| **Waking Dream** | Concentration DC 20 to enter a sleep-like trance, partially aware of the real world. Can converse with people in both realms but cannot take other real-world actions. |
| **Dreamwatch** | Enter the space between dreams; locate, observe, or enter a specific person's dream. Concentration DC by relationship (Intense love/hate 10 / Well-known friend 15 / Acquaintance 20 / Met once 25 / Heard of 30 / Stranger 35), modified by distance (Within feet +5 / 1 mi +0 / 100 mi -5 / >100 mi -10). +5 if you've entered their dream before. Inside another's dream you become subject to the dreamer's psyche; any independent action requires a Concentration check (DC 10 simple actions, higher for substantial changes). Failing by 10+ enslaves you to the dreamer's psyche until they wake. Real-self cannot be physically harmed inside another's dream. -10 to Concentration if there's an intense emotional bond. Communicating a real-world message requires the dreamer to make Int DC 15 on waking (+5 if expecting it). May exit space-between-dreams to wake at will. |

### Foretelling line

| Feat | Effect |
|------|--------|
| **Latent Foreteller** | Prerequisite-only; no benefit. |
| **Foreteller** | Composure DC 20 to invoke the trance. GM decides whether a foretelling actually manifests. Foretold statements always true but cryptic. Once per game session. |

### Old Blood line

| Feat | Effect |
|------|--------|
| **Latent Old Blood** | Prerequisite-only; no benefit. |
| **Old Blood** | Roll 1d6; on a 1 the old blood responds. Once-per-session response per topic; once it succeeds, no further calls that session. Effects: gain 2d6 temporary ranks in an Int- or Wis-keyed skill for 10 minutes (or +2 ranks if you already have >12); receive a piece of ancient (>300-year-old, Third Age) lore via gleeman-style specialized lore; receive an insight clue based on facts you already know. |

### Sniffing line

| Feat | Effect |
|------|--------|
| **Latent Sniffer** | Prerequisite-only; no benefit. |
| **Sniffer** | Smell residue of violent acts. Intensity reflects severity (torture/murder strongest; fair fights weakest). Odors fade in ~1 week. Track perpetrator from scene with Search DC 15 (-5 for especially heinous acts; +3 per 24 hours since the act). Reveals location and identity of perpetrator but not method or participants. |

### Treesinging line (Ogier only)

| Feat | Effect |
|------|--------|
| **Latent Treesinger** | Prerequisite-only; no benefit. Ogier-only. |
| **Treesinger** | Sing wooden objects out of trees via Craft (treesinging) check (untrained allowed; ranks otherwise unbuyable). Solid wood items only — no moving parts (assemble parts separately). Items match masterwork quality and price. |
| **Tree Warden** | Heal trees (Concentration DC 15 typical, 10 minutes) or grow them. Grow DC = ¼ current height (ft); grows up to 25%/level; 10 min per 25% increase. A grown tree cannot be regrown for a month. |

### Treesinger Craft DCs

| Item | DC | Time |
|------|----|------|
| Board or plank | 5 | 1 minute |
| Simple item (staff, club, bucket) | 10 | 5 minutes |
| Modest item (stool, bow, flute) | 15 | 10 minutes |
| Complex item (chair, rowboat) | 20 | 20 minutes |
| Extremely complex (statue, ornate throne) | 25 | 30 minutes |
| Masterwork modifier | +10 | +15 minutes |

### Viewing line

| Feat | Effect |
|------|--------|
| **Latent Viewer** | Prerequisite-only; no benefit. |
| **Viewing** | Spot check (DC by subject minus subject's level): Average person 30 / Hero class 25 / Prestige class 20 / Channeler or Warder 15. Success reveals one important fact about the subject. Beat DC by 10+ for a metaphoric prophetic image. One viewing per subject — repeats automatically fail. Humans only (not creatures or Ogier). |

## Implementation Notes (WheelMUD)

- **Storage:** feats are flag-style (have / don't have); use a per-character set keyed by feat ID. Multi-take general feats (`Weapon Focus`, `Skill Emphasis`, etc.) need a parameter (weapon ID, skill ID, Affinity, Talent). Stackable feats (`Toughness`, `Mental Stability`, `Quickness`, `The Dark One's Own Luck`, `Multiweave`) need a count.
- **Prerequisites:** treat as a graph — validate at award time and at recompute (e.g. ability drain). Locking (not removing) a feat when a prerequisite is suspended preserves audit history and matches the rules text.
- **Slot accounting:** track three feat-slot pools — general (1 + 1 per 3 character levels + class/background grants), channeling (initiate/wilder class grants), and free-from-class (Armor Proficiency lines, Simple/Martial Weapon Proficiency, etc.). Channeling slots cannot consume general feats except Mental Stability for male channelers.
- **Class restriction:** mark feats with allowed-class lists for `Eliminate Block`, `Weapon Specialization`, all channeling feats, `Latent Treesinger` / `Treesinger` / `Tree Warden` (Ogier-only).
- **Combat-system hooks:** several feats modify the to-hit/damage/AoO pipeline (`Power Attack`, `Combat Expertise`, `Combat Reflexes`, `Two-Weapon Fighting` family, `Cleave`/`Great Cleave`, `Whirlwind Attack`, `Improved Critical`, `Spring Attack`, `Shot on the Run`). Implement these as middleware on attack resolution rather than per-feat special cases where possible.
- **Lost-ability feats** are GM-gated in the source; expose a config flag (`world.lost_abilities_enabled`) so admins can disable them campaign-wide.
- **Rolls/randomness** for `Old Blood` (1d6), `Foreteller` (Composure DC 20), and `Viewing` (Spot vs. table) all return narrative output — keep a hook for staff-authored result text rather than auto-generated fluff.
- **Reputation interactions:** `Fame`, `Infamy`, `Low Profile` modify the Reputation system (Chapter 6). Coordinate with that subsystem when implemented.
