# Heroic Characteristics

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 6: Heroic Characteristics, pp. 102–111) for use in WheelMUD implementation.

## Overview

This chapter covers identity details, reputation, followers, movement, and encumbrance — the non-combat systems that surround a character.

## Character Details

- **Name** — chosen to fit race/class and culture; see Chapter 2: Backgrounds for examples.
- **Gender** — primarily mechanical for channeling. No man may join the Aes Sedai or Aiel Wise Ones; male channelers face fear and prejudice. Most other roles are open to either gender, with some social friction in traditional westland societies.
- **Age** — defaulted to late teens / early twenties for the Pattern's "young heroes" assumption. Ogier are long-lived (90+ years counts as "young"). Channelers age slowly.
- **Appearance** — derived from background (Chapter 2). High Charisma implies attractive or exotic looks. Handedness is free; Ambidexterity feat removes the off-hand penalty.
- **Personality / Life Experience / Customization** — narrative; subject to GM approval.

## Measurement (in-world units)

| Unit | Value |
|------|-------|
| Inch | 1/10 of a foot (vs. 1/12 in our world; ~20% longer) |
| Foot | ≈ real-world foot |
| Pace | 3 feet |
| Span | 2 paces (6 ft) |
| Mile | 1000 spans (6,000 ft, ≈15% longer than our miles) |
| League | 4 miles |
| Stone | 10 pounds |
| Pound | ≈ real-world pound |

## Random Height and Weight (Table 6-1)

| Race / Gender | Base Height | Height Modifier | Base Weight | Weight Modifier |
|---------------|-------------|-----------------|-------------|-----------------|
| Human, male | 5 ft 4 in | +2d4 | 14 stone | 1d8+1 |
| Human, female | 5 ft 0 in | +2d4 | 10 stone | 1d8+1 |
| Ogier, male | 7 ft 0 in | +2d8 | 27 stone | 1d6+1 |
| Ogier, female | 6 ft 7 in | +2d8 | 19 stone | 1d6+1 |

### Background Height Modifiers

| Background | Modifier (in.) |
|------------|---------------|
| Aiel | +4 |
| Atha'an Miere | +2 |
| Borderlander | -1 |
| Cairhienin | -3 |
| Domani | +1 |
| Ebou Dari | (not listed) |
| Illianer | (not listed) |
| Midlander | (not listed) |
| Taraboner | (not listed) |
| Tairen | +1 |
| Tar Valoner | (not listed) |

**Procedure:**

1. Roll Height Modifier (or pick within range), add Background Height Modifier.
2. Add result (in inches) to Base Height.
3. Roll Weight Modifier; multiply by the Height + Background modifier total.
4. Add product (in pounds) to Base Weight.

## Reputation

A character's Reputation score measures fame (or infamy). It influences NPC reactions and the ability to mask identity.

### Starting Reputation

| Class | Start |
|-------|-------|
| Noble | 3 |
| Initiate | 1 |
| All others | 0 |

Reputation increases automatically with class level (per Chapter 3 class tables) and through dramatic actions.

### Gaining Reputation

- **Automatic +1** for an act of dramatic heroism with witnesses.
- **Charisma DC 20** to gain +1 for an attention-drawing-but-lesser act (e.g. escaping a Forsaken).
- Vicious or malevolent acts add Reputation just the same — Reputation does not care about morality.

### Reputation Score Examples (Table 6-2)

| Score | Description | Examples |
|-------|-------------|----------|
| 0 | Unknown | Laborer, intern, apprentice, enlisted soldier |
| 1–2 | Known in home town | Low-ranking officer, important craftsman/merchant |
| 3–5 | Known in home region | Local lord, captain, successful gleeman |
| 6–9 | Known in home domain/kingdom | Aes Sedai, high-ranking lord |
| 10–14 | Known in many domains/kingdoms | Heir to ruler, high-ranking Aes Sedai |
| 15–20 | Known throughout the land | Ruler, famous military leader, prophet |
| 21+ | Known worldwide | Amyrlin Seat, false Dragon |

### Reputation Checks

- **Roll:** 1d20 + Reputation (Reputation 0 → cannot make a check).
- **DC by location:**

| Location | DC |
|----------|----|
| Midlands, Cairhien | 25 |
| Borderlands, Illian, Ebou Dar, Tear | 30 |
| Arad Doman, Tarabon | 35 |
| Aiel Waste, Sea Folk islands | 40 |
| Seanchan, Shara | 45 |

| DC Modifier | Condition |
|-------------|-----------|
| +5 | Remote village or region |
| -5 | Large city |
| -5 | Character's home town or area |

A Reputation check happens automatically when the character's identity might be recognized — the player does not opt in.

### Fame vs. Infamy

A character whose Reputation is at least half-earned through vicious or malevolent acts is **Infamous**. Otherwise (and by default) **Famous**. The `Fame`, `Infamy`, and `Low Profile` feats override or modify this.

### Reputation Check Skill Bonuses (Table 6-3, synergy bonuses)

| Skill | Famous | Infamous |
|-------|--------|----------|
| Bluff | +5 | +5 (auto -5 when used to deny identity) |
| Diplomacy | +5 | -5 |
| Entertain | +5 | +0 |
| Gather Information | +5 | +5 |
| Intimidate | +2 | +5 |

## Followers

At 10th level and every level thereafter, a hero may attempt one **Reputation check DC 25** to attract followers (troops, servants, accomplices, trainees, etc.).

- Optional — the player can decline.
- On success: followers arrive over the next few weeks.
- On failure: cannot retry until the next level.
- After succeeding once, the player may keep trying each new level — but the maximum total stays capped.

### Cap Rules

- Total **levels** of all followers ≤ character's Reputation score.
  - Professional characters count as ½ their level.
  - Commoners count as ½ of a 1st-level character.
- No single follower may exceed ½ Reputation (rounded down) in level.
- Followers stay loyal absent extreme abuse (GM call). Departed/dead followers free up cap "open space" that can be filled with future successful checks.
- GM may disallow followers during specific adventures.

## Alignment Posture (Good/Bad/Evil)

The setting assumes "good" PCs. Truly evil PCs are problematic and discouraged: short Darkfriend life-span, hidden mechanics, and group-cohesion issues. The middle ground (Whitecloaks, Seanchan, Shaido Aiel, dabbling Darkfriends) is allowed but requires GM coordination.

## Movement

Three scales:

| Scale | Use | Unit |
|-------|-----|------|
| Tactical | Combat | feet / round |
| Local | Exploring an area | feet / minute |
| Overland | Long travel | miles / hour or day |

### Modes of Movement

| Mode | Multiplier | Notes |
|------|-----------|-------|
| Walk | x1 | ≈ 3 mph for unencumbered human |
| Hustle | x2 | Two move actions/round; ≈ 6 mph |
| Run (x3) | x3 | Heavy armor; full-round action |
| Run (x4) | x4 | Light/medium/no armor; full-round action |

### Movement and Distance (Table 6-4)

| Per Round (Tactical) | 15 ft | 20 ft | 30 ft | 40 ft |
|----------------------|------|------|------|------|
| Walk | 15 | 20 | 30 | 40 |
| Hustle | 30 | 40 | 60 | 80 |
| Run (x3) | 45 | 60 | 90 | 120 |
| Run (x4) | 60 | 80 | 120 | 160 |

| Per Minute (Local) | 15 ft | 20 ft | 30 ft | 40 ft |
|--------------------|------|------|------|------|
| Walk | 150 | 200 | 300 | 400 |
| Hustle | 300 | 400 | 600 | 800 |
| Run (x3) | 450 | 600 | 900 | 1,200 |
| Run (x4) | 600 | 800 | 1,200 | 1,600 |

| Per Hour (Overland) | 15 ft | 20 ft | 30 ft | 40 ft |
|---------------------|------|------|------|------|
| Walk | 1½ mi | 2 mi | 3 mi | 4 mi |
| Hustle | 3 mi | 4 mi | 6 mi | 8 mi |

| Per Day (Overland) | 15 ft | 20 ft | 30 ft | 40 ft |
|--------------------|------|------|------|------|
| Walk | 12 mi | 16 mi | 24 mi | 32 mi |

### Hampered Movement (Table 6-5)

| Condition | Example | Penalty |
|-----------|---------|---------|
| Moderate obstruction | Undergrowth | x¾ |
| Heavy obstruction | Thick undergrowth | x½ |
| Bad surface | Steep slope or mud | x½ |
| Very bad surface | Deep snow | x¼ |
| Poor visibility | Darkness or fog | x½ |

Multiple conditions multiply (e.g. thick undergrowth in fog = x¼).

### Local Movement Notes

- Walk and Hustle are unrestricted on the local scale.
- Run for ≈1–2 minutes requires Con 9+; then rest 1 minute.

### Overland Movement Notes

- A travel day = 8 hours walking (or 10 hours rowing, 24 hours sailing).
- **Hustle:** 1 hour free; each additional hour deals 1 subdual damage, doubling per hour thereafter.
- **Run:** cannot be sustained; treat run-rest cycles as a Hustle.
- **Forced March:** beyond 8 hours walking, Con check (DC 10 +1 per extra hour) each hour. Failure = 1d6 subdual damage; cannot be recovered until 4 hours of rest. Marching into unconsciousness is possible.
- **Mounted:** mounts can hustle (damage taken is normal damage). Force-march possible but Con checks auto-fail and damage is normal.

### Terrain Modifiers (Table 6-6)

| Terrain | Highway | Road/Trail | Trackless |
|---------|---------|-----------|-----------|
| Plains | x1 | x1 | x1 |
| Scrub, rough | x1 | x1 | x¾ |
| Forest | x1 | x1 | x½ |
| Jungle | x1 | x¾ | x¼ |
| Swamp | x1 | x¾ | x½ |
| Hills | x1 | x¾ | x½ |
| Mountains | x¾ | x½ | x¼ |
| Sandy desert | x1 | — | x½ |

### Mounts and Vehicles (Table 6-7)

| Mount/Vehicle | Per Hour | Per Day |
|---------------|---------|---------|
| Light horse / light warhorse | 6 mi | 48 mi |
| Light horse (151–450 lb load) | 4 mi | 32 mi |
| Light warhorse (231–690 lb) | 4 mi | 32 mi |
| Heavy horse | 5 mi | 40 mi |
| Heavy horse (201–600 lb) | 3½ mi | 28 mi |
| Heavy warhorse | 4 mi | 32 mi |
| Heavy warhorse (301–900 lb) | 3 mi | 24 mi |
| Pony | 4 mi | 32 mi |
| Pony (76–225 lb) | 3 mi | 24 mi |
| Donkey or mule | 3 mi | 24 mi |
| Mule (231–690 lb) | 2 mi | 16 mi |
| Cart or wagon | 2 mi | 16 mi |
| Raft or barge (poled/towed)* | ½ mi | 5 mi |
| Keelboat (rowed)* | 1 mi | 10 mi |
| Rowboat (rowed)* | 1½ mi | 15 mi |
| Mainland sailing ship | 3 mi | 48 mi |
| Atha'an Miere skimmer | 3 mi | 72 mi |
| Atha'an Miere darter / soarer | 4 mi | 96 mi |
| Atha'an Miere raker | 5 mi | 120 mi |

\* Single shift of rowers/polers. River currents (typically 2–3 mph) add or subtract from speed; rowed vessels usually cannot row upstream against significant current but can be pulled by shore animals. A rowed boat can drift downstream the remaining 14 hours/day if guided.

### Waterborne Notes

- Rowed/poled vessel: 10 hours/day (single shift).
- Drifting downstream adds ~42 mi/day in a 3 mph current.
- Larger ships handle largest rivers / open sea.

## Encumbrance

Two parts: **by armor** and **by total weight**. Use the worse penalties from each — they do not stack.

### Encumbrance by Armor

Armor (Table 7-5) sets max Dex bonus, armor check penalty, speed, and run multiplier. If the character is not weak and not heavily loaded, armor alone is the limit.

### Carrying Capacity (Table 6-8, Medium-size biped)

| Strength | Light | Medium | Heavy |
|----------|-------|--------|-------|
| 1 | ≤3 lb | 4–6 | 7–10 |
| 2 | ≤6 | 7–13 | 14–20 |
| 3 | ≤10 | 11–20 | 21–30 |
| 4 | ≤13 | 14–26 | 27–40 |
| 5 | ≤16 | 17–33 | 34–50 |
| 6 | ≤20 | 21–40 | 41–60 |
| 7 | ≤23 | 24–46 | 47–70 |
| 8 | ≤26 | 27–53 | 54–80 |
| 9 | ≤30 | 31–60 | 61–90 |
| 10 | ≤33 | 34–66 | 67–100 |
| 11 | ≤38 | 39–76 | 77–115 |
| 12 | ≤43 | 44–86 | 87–130 |
| 13 | ≤50 | 51–100 | 101–150 |
| 14 | ≤58 | 59–116 | 117–175 |
| 15 | ≤66 | 67–133 | 134–200 |
| 16 | ≤76 | 77–153 | 154–230 |
| 17 | ≤86 | 87–173 | 174–260 |
| 18 | ≤100 | 101–200 | 201–300 |
| 19 | ≤116 | 117–233 | 234–350 |
| 20 | ≤133 | 134–266 | 267–400 |
| 21 | ≤153 | 154–306 | 307–460 |
| 22 | ≤173 | 174–346 | 347–520 |
| 23 | ≤200 | 201–400 | 401–600 |
| 24 | ≤233 | 234–466 | 467–700 |
| 25 | ≤266 | 267–533 | 534–800 |
| 26 | ≤306 | 307–613 | 614–920 |
| 27 | ≤346 | 347–693 | 694–1,040 |
| 28 | ≤400 | 401–800 | 801–1,200 |
| 29 | ≤466 | 467–933 | 934–1,400 |
| **+10 Str** | x4 | x4 | x4 |

### Carrying Loads (Table 6-9)

| Load | Max Dex | Check Penalty | Speed (30 ft) | Speed (20 ft) | Run |
|------|---------|---------------|--------------|--------------|-----|
| Light | — | 0 | 30 | 20 | x4 |
| Medium | +3 | -3 | 20 | 15 | x4 |
| Heavy | +1 | -6 | 20 | 15 | x3 |

A medium or heavy load counts as medium or heavy armor for ability restrictions. Light load does not encumber.

### Lifting and Dragging

- Lift over head: up to max load.
- Lift off ground (stagger only, lose Dex to Defense, move 5 ft/round full-round): up to 2x max load.
- Push or drag along ground: up to 5x max load. Smooth/slick conditions x2; broken ground or snagging objects ≤ ½.

### Size Modifiers

**Carrying capacity multipliers by creature size (biped):**

| Size | Modifier |
|------|----------|
| Fine | x⅛ |
| Diminutive | x¼ |
| Tiny | x½ |
| Small | x¾ |
| Medium | x1 |
| Large | x2 |
| Huge | x4 |
| Gargantuan | x8 |
| Colossal | x16 |

**Quadruped multipliers:**

| Size | Modifier |
|------|----------|
| Fine | x¼ |
| Diminutive | x½ |
| Tiny | x¾ |
| Small | x1 |
| Medium | x1½ |
| Large | x3 |
| Huge | x6 |
| Gargantuan | x12 |
| Colossal | x24 |

### Tremendous Strength

For Strength 30+, find the 20–29 entry with the matching ones digit and multiply: x4 (30s), x16 (40s), x64 (50s), and so on.

## Implementation Notes (WheelMUD)

- **Reputation system:** store Reputation as an int per character; track fame/infamy as a derived flag (sum of "vicious" Reputation gains ≥ ½ total). Implement `Fame`, `Infamy`, `Low Profile` feats as modifiers to gain rate / forced infamy. NPC `recognize-on-sight` should call into a Reputation check on the location-DC table.
- **Reputation checks** are *automatic* per the rules — implement as a passive system triggered when a character speaks or acts in a populated room, not a player-initiated command.
- **Followers** map naturally to a per-character roster with a level cap = Reputation; cap-free slots reopen on departure/death and are reusable. Gate behind level ≥ 10 and a level-up hook for the DC 25 check. Provide an admin toggle to disable in specific adventures (matches GM discretion).
- **Movement scales:** WheelMUD already uses room-to-room movement; hustle/run primarily affect overland (player-initiated travel paths) and tactical (combat tick speed). Keep walk as the default room-move; expose hustle/run as commands or modes that consume stamina (subdual damage proxy).
- **Forced march & overland fatigue:** model subdual damage with a separate counter that recovers after 4 hours of rest. Tie the Constitution check schedule to the travel system rather than per-room movement.
- **Encumbrance:** compute load on inventory mutation; cache `MaxLoad` from Strength via Table 6-8 lookup. The "use worse of armor or load" rule means storing both and selecting per attribute (Dex cap, check penalty, speed, run). For Str 30+, implement the +10 Str x4 chain rather than baking each row.
- **Size:** keep a `Size` enum on creatures so capacity scaling is a single multiplier lookup. Quadruped vs. biped is a separate flag, not a size category.
- **Heights/weights:** generate at character creation via the Random Height & Weight tables; persist as static fields. Background height modifier should pull from the background profile (already partly modeled in `docs/reference/backgrounds.md`).
- **Measurement units:** internally store distance in feet and weight in pounds (standard). Display layer can optionally render spans/leagues/stone for in-world flavor.
