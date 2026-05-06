# Skills

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 4: Skills, pp. 64–87) for use in WheelMUD implementation.

## Overview

A skill represents a character's training in a specific area — climbing a wall, talking past a guard, identifying the weave another channeler just spun. Skill checks resolve as `1d20 + skill rank + ability modifier + miscellaneous modifiers` against either a Difficulty Class set by the GM or another character's opposing check.

## Skill Points

- **1st level:** `(class skill points + Int modifier) × 4`. Minimum 1 point.
- **Higher levels:** `class skill points + Int modifier` per level. Minimum 1.
- A class skill costs 1 point per rank; a cross-class skill costs 2 points per rank (recorded as half-ranks).
- Maximum rank in a class skill = `level + 3`. Maximum rank in a cross-class skill = `(level + 3) / 2` (do not round).
- Skills marked **No** under "Untrained" require at least 1 rank to use at all.

| Class | 1st-Level Skill Points | Per-Level After |
|-------|------------------------|-----------------|
| Algai'd'siswai | (4 + Int) × 4 | 4 + Int |
| Armsman | (4 + Int) × 4 | 4 + Int |
| Initiate | (4 + Int) × 4 | 4 + Int |
| Noble | (4 + Int) × 4 | 4 + Int |
| Wanderer | (8 + Int) × 4 | 8 + Int |
| Wilder | (4 + Int) × 4 | 4 + Int |
| Woodsman | (6 + Int) × 4 | 6 + Int |

## Skill Check Modifiers

- **Take 10** — when not threatened or rushed, treat the d20 roll as a 10. Useful for routine work.
- **Take 20** — when there is plenty of time and no penalty for failure, treat the roll as a 20. Takes about 20× as long as a single check.
- **Aid Another** — a helper rolls DC 10 against the same skill; success grants the lead character a +2 circumstance bonus.
- **Synergy** — 5+ ranks in one skill grants a +2 bonus on checks with synergistic skills, as noted in each skill's entry.
- **Armor Check Penalty** — applies to skills marked with `*` in tables; heavy armor reduces effective skill modifier.

## Difficulty Class Reference (Table 4-5)

| DC | Difficulty | Example |
|----|------------|---------|
| 0  | Very easy | Notice a Trolloc in plain sight |
| 5  | Easy | Climb a knotted rope |
| 10 | Average | Hear a fist of Trollocs from 300 ft |
| 15 | Tough | Learn an enemy's whereabouts |
| 20 | Challenging | Decipher an Age of Legends inscription |
| 25 | Formidable | Read lips at 30 ft |
| 30 | Heroic | Leap a 30-ft chasm |
| 35 | Super heroic | Talk past suspicious palace guards |
| 40 | Nearly impossible | Track an Aiel across the Waste after a duststorm |

## General Skills (Table 4-2)

`*` after the ability indicates an Armor Check Penalty applies. **Trained**: Yes = usable untrained; No = at least 1 rank required.

| Skill | Key Ability | Trained | Notes |
|-------|-------------|---------|-------|
| Animal Empathy | Cha | No | Woodsman only |
| Appraise | Int | Yes | |
| Balance | Dex* | Yes | |
| Bluff | Cha | Yes | |
| Climb | Str* | Yes | |
| Concentration | Con | Yes | Required for channelers |
| Craft | Int | No | Multi-take per trade |
| Decipher Script | Int | No | Initiate-only class skill |
| Diplomacy | Cha | Yes | |
| Disable Device | Int | No | |
| Disguise | Cha | Yes | |
| Escape Artist | Dex* | Yes | |
| Forgery | Int | Yes | |
| Gather Information | Cha | Yes | |
| Handle Animal | Cha | No | |
| Heal | Wis | Yes | |
| Hide | Dex* | Yes | |
| Innuendo | Wis | No | |
| Intimidate | Cha | Yes | |
| Intuit Direction | Wis | No | |
| Jump | Str* | Yes | |
| Knowledge | Int | No | Multi-take per field |
| Listen | Wis | Yes | |
| Move Silently | Dex* | Yes | |
| Open Lock | Dex | No | |
| Perform | Cha | Yes | Multi-take per art |
| Pick Pocket | Dex* | No | |
| Profession | Wis | No | Multi-take per trade |
| Read Lips | Int | No | Wanderer only |
| Ride | Dex | Yes | |
| Search | Int | Yes | |
| Sense Motive | Wis | Yes | |
| Speak Language | — | No | One language per rank |
| Spot | Wis | Yes | |
| Swim | Str* | Yes | |
| Tumble | Dex* | No | |
| Use Rope | Dex | Yes | |
| Wilderness Lore | Wis | Yes | Required for the Track feat |

## Channeler Skills (Table 4-3)

Only initiates and wilders may take these. Composure is open to other classes only as a cross-class skill where the table allows.

| Skill | Key Ability | Trained | Class List |
|-------|-------------|---------|-----------|
| Composure | Wis | No | Cross-class for everyone except initiate / wilder |
| Invert | Int | No | Lost — Initiate / Wilder only |
| Weavesight | Int | Yes | Initiate / Wilder only |

## Skill Descriptions

Concise summaries below — each captures the in-world flavor and the most-used mechanic. Refer to the source PDF for full task tables (DCs, distances, etc.).

### Animal Empathy (Cha) — *Trained Only; Woodsman Only*

Read and influence an animal's mood — calm a barking dog, soothe a startled horse, keep a grolm at bay while you back away. Works on natural animals at full effect; raken, grolm, and other beasts at a –4 penalty.

### Appraise (Int)

Estimate an item's worth, from a Tairen lute to a shipment of Sharan silks. Common goods are easy (DC 12, accurate to within 10%); rare or exotic items take a higher DC and yield a 50–150% estimate.

### Balance (Dex*)

Walk a tightrope, beam, ledge, or icy footing without falling. A failed check by 5 or more drops you. While balancing you lose your Dex bonus to Defense.

### Bluff (Cha)

Make the outrageous or untrue seem plausible — lying, conning, fast-talking, misdirection. Opposed by Sense Motive. Also used to **feint** in combat (deny an opponent their Dex to Defense for one attack) and to **create a diversion** so you can Hide while observed.

### Climb (Str*)

Scale a cliff, scramble up to a second-storey window, climb a rope or rough wall. DCs run 0 (steep slope with rope) to 25 (overhang). A failure by 5 or more drops you. Climbing is one-quarter speed (full-round), or one-half speed at –5.

### Concentration (Con)

Stay focused under pressure — keep a weave aligned while taking damage, study a clue in a moving wagon, eavesdrop while pretending to nap. Channelers use this for every difficult cast: **link**, **overchannel**, **unlace**, hold a weave through distractions.

### Craft (Int)

Make an item — armor, bows, weapons, leatherwork, pottery, calligraphy, jewellery, masonry. Each trade is a separate skill (Craft (armorsmithing), Craft (bowmaking), etc.). Successful craftsmen earn about half their check result in silver marks per week.

### Decipher Script (Int) — *Trained Only*

Puzzle out unfamiliar writing — an Age-of-Legends inscription, a coded letter, a partially-erased map. DC 20 for simple messages, 30+ for ancient or alien scripts. Failure may give a false reading you don't notice.

### Diplomacy (Cha)

Change someone's attitude — negotiate passage with an Atha'an Miere raker, calm a feud between Cairhienin houses, persuade Children of the Light to leave peacefully. Also drives haggling: each 5 points of margin shifts a price 5%.

### Disable Device (Int) — *Trained Only*

Disarm a trap, jam or rig a lock, sabotage a saddle. Simple devices are a full-round DC 10; complex traps take 2d4 rounds at DC 25 or higher. A failure by 5 sets the trap off.

### Disguise (Cha)

Change your appearance — minor details, opposite sex (–2), different age (–2 per step), different background. Opposed by Spot. Friends and close associates spot through disguises more easily.

### Escape Artist (Dex*)

Slip out of ropes, manacles, a tight crawlspace, or a grappler. Ropes are opposed by your binder's Dex check (binder gets +10); manacles are DC 35; tight spaces vary by length.

### Forgery (Int)

Fake a document — military orders, a House seal, a deed, a map purporting to be from the Age of Legends. Opposed by the reader's Forgery check, modified by their familiarity with the document type and handwriting.

### Gather Information (Cha)

Spend an evening in taverns, brothels, and markets buying drinks and asking questions. DC 10 yields the major news of the city; DC 15–25 finds specifics ("which way to the hidden bandit camp?"). Cannot be used without speaking the local language.

### Handle Animal (Cha) — *Trained Only*

Train a domestic animal, drive a team, raise a wild creature from infancy. Teaching tricks takes two months and 1 rank per trick. Cannot be used to train inherently hostile creatures.

### Heal (Wis)

Save a dying friend, help allies recover faster, treat poison or disease. First aid is DC 15; long-term care doubles natural healing. A healer's kit gives +2; a trained healer can also use **healer's balm** to restore lost hit points beyond standard rest.

### Hide (Dex*)

Sink into shadows, slip past a guard post, tail someone through a city. Opposed by Spot. Half-speed = no penalty; full speed = –5; running or charging = –20. Size shifts the result wildly (Tiny +8, Large –4, Huge –8).

### Innuendo (Wis) — *Trained Only*

Send and receive secret messages while appearing to talk about something else. DC 10 for simple meaning, 15–20 for complex. Eavesdroppers must beat your Innuendo check to read the hidden message.

### Intimidate (Cha)

Make a guard back down, force a prisoner to talk, cow a tavern brawl. Opposed by the target's level + bonuses against fear. Verbal threats and body language are both part of the skill.

### Intuit Direction (Wis) — *Trained Only*

Concentrate for a minute and find true north relative to your position (DC 15). On a natural 1 you confidently identify a wrong direction. Untrained characters cannot — they must navigate by landmarks and clues.

### Jump (Str*)

Leap pits, low fences, or up to a tree branch. Running jump = 5 ft minimum + 1 ft per check point above 10 (max = height × 6). Standing jump = 3 ft minimum, half the above. High jumps are roughly one-fifth the distance.

### Knowledge (Int) — *Trained Only*

Recall a body of lore — Arcana, the Blight, geography, history, nobility & royalty, the Age of Legends, etc. Each field is a separate skill. Untrained characters fall back on a raw Int check and only know common knowledge.

### Listen (Wis)

Hear an approaching enemy, eavesdrop, detect someone sneaking up. DCs run 0 (people talking) to 30 (a Myrddraal moving on a smooth surface). +1 DC per 10 ft from the source; +5 through a door, +15 through a stone wall.

### Move Silently (Dex*)

Sneak up behind an enemy, slip past a sleeping guard, leave a room without being heard. Opposed by Listen. Half-speed = no penalty; full speed = –5; running or charging is essentially impossible.

### Open Lock (Dex) — *Trained Only*

Pick padlocks, finesse combination locks, solve puzzle locks. Full-round action; thieves' tools mandatory (improvised tools = –2; masterwork tools = +2). DCs: very simple 20, average 25, good 30, amazing 40.

### Perform (Cha)

Entertain an audience with one specific art — ballad, dance, drums, flute, harp, lute, mime, oratory, singing, storytelling. Each art is a separate skill. Routine performance earns ~1d10 silver pennies per day; great performance can land you a noble's patronage.

### Pick Pocket (Dex*) — *Trained Only*

Lift a coin purse, plant an item on a target, perform sleight-of-hand. Coin-sized objects are DC 10. Opposed by the target's Spot if they're paying attention. A failure by 20+ has the target detect the attempt.

### Profession (Wis) — *Trained Only*

Practice a livelihood — apothecary, bookkeeper, brewer, cook, farmer, herbalist, innkeeper, sailor, scribe, siege engineer, etc. Each profession is a separate skill. Earns about half your check result in silver marks per week of dedicated work.

### Read Lips (Int) — *Trained Only; Wanderer only*

Understand a speaker by watching their lips at up to 30 ft. Requires a clear line of sight and one full minute of concentration. DC 15 base; intricate, indirect, or muffled speech raises the DC.

### Ride (Dex)

Ride one familiar mount type — horse (incl. mules and ponies), to'raken, raken, etc. Riding a different mount-type is at –2 to –5 ranks. Most casual riding requires no check; battle, leaps, and bareback riding do.

### Search (Int)

Scour a 5-ft area for hidden compartments, traps, or small clues. DC 10 to ransack; DC 20 to find a typical secret door; DC 25+ for cleverly-hidden traps. Tracking requires the Track feat — Search alone only finds tracks at DC ≤ 10.

### Sense Motive (Wis)

Read another character's body language and tell when they're bluffing, lying, or hiding ill intent. Opposed by Bluff. Also used to make a "hunch" check (DC 20) about a social situation in general.

### Speak Language — *Trained Only*

You start at 1st level fluent in your background's primary language. Each rank in this skill picks up a new language (read, write, and speak). No check is made — either you know the language or you don't. Some languages have no written form.

### Spot (Wis)

Notice opponents waiting in ambush, an assassin in the shadows, a lurking pickpocket. Opposed by Hide. –1 per 10 ft of distance, –5 if the spotter is distracted.

### Swim (Str*)

Move through water at one-quarter speed (move) or one-half speed (full-round). Calm water is DC 10; rough 15; stormy 20. Failing by 5 starts you drowning. Held breath: Con × rounds. Armor and gear = –1 Swim per 5 lb carried.

### Tumble (Dex*) — *Trained Only*

Dive, roll, somersault, and flip. DC 15 to treat a fall as 10 ft shorter, DC 15 to tumble past a single foe, DC 25 to weave through a press of enemies. Five ranks of Tumble grant a +3 dodge bonus to Defense when fighting defensively.

### Use Rope (Dex)

Tie firm knots, slip-knots, or special bindings; bind prisoners; splice ropes. A typical knot is DC 10; a tricky special-purpose knot is DC 15. When you bind someone, you get +10 inherent on the opposed Escape Artist check.

### Wilderness Lore (Wis)

Hunt and forage on the move (DC 10 keeps the party fed at half overland speed), shelter against severe weather (DC 15), follow tracks (with the Track feat), avoid natural hazards. Required for the Track feat.

## Channeler Skill Descriptions

### Composure (Wis) — *Trained Only*

Maintain inner calm in the face of fear, stress, or volatile emotion. DC 15 grants +2 on Bluff, Diplomacy, and Intimidate for ten minutes of heated social pressure. DC 20 lets a channeler fall asleep at will or enter the Dream realms without delay. DC 25 keeps you comfortable in moderate temperature extremes (and grants +5 on weather-related Concentration checks). DC 20 in combat grants +1 to attack rolls for 5 rounds.

### Invert (Int) — *Lost; Trained Only; Initiate, Wilder only*

Conceal a weave you have just cast from other channelers' Weavesight. Only the same gender can have any chance of seeing the weave (with the Sense Residue feat). Doing so disguises the weave so even other channelers can't tell its appearance is unnatural. Particularly useful for inverting a *create fire* weave to light a hearth without revealing yourself.

### Weavesight (Int) — *Initiate, Wilder only*

Identify a weave as it is cast or while held. DC 10 reveals which of the Five Powers are used. DC 15 names the weave (if you know it). DC 20 lets you learn the weave (if it's a level you can cast without overchanneling); DC 25 if you'd need to overchannel. Note that males can only see saidin weaves and females can only see saidar weaves.
