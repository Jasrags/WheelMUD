# Classes

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 3: Classes, pp. 44–63) for use in WheelMUD implementation.

## The Seven Hero Classes

| Class | Abbrev | Role |
|---|---|---|
| *Algai'd'siswai* | Alg | Aiel desert warrior using agility, skill, and spearfighting |
| Armsman | Arm | Heavy combat and weapon mastery |
| Initiate | Ini | Trained channeler of the One Power |
| Noble | Nbl | Status, education, and influence |
| Wanderer | Wan | Stealth, wits, skills, and luck |
| Wilder | Wil | Untrained natural channeler |
| Woodsman | Wds | Hunter and warrior of the wilderness |

Initiates and wilders are collectively called **channelers**.

## Class and Level Bonuses

A roll = 1d20 + ability modifier + class/level bonus.

### Base Attack Bonus

Cumulative across multiclass. A "+12/+7/+2" entry means three attacks per round at those bonuses. Ability/racial/weapon bonuses do not grant extra attacks; only base attack ≥ +6 does.

### Base Save Bonus

Each class has Fortitude, Reflex, and Will save bonuses by level. Multiclass: sum the per-class base bonuses.

## Level-Dependent Benefits (Table 3-1)

| Char Lvl | XP | Class Skill Max Ranks | Cross-Class Max Ranks | Feats | Ability Increases |
|---|---|---|---|---|---|
| 1 | 0 | 4 | 2 | 1st | — |
| 2 | 1,000 | 5 | 2½ | — | — |
| 3 | 3,000 | 6 | 3 | 2nd | — |
| 4 | 6,000 | 7 | 3½ | — | 1st |
| 5 | 10,000 | 8 | 4 | — | — |
| 6 | 15,000 | 9 | 4½ | 3rd | — |
| 7 | 21,000 | 10 | 5 | — | — |
| 8 | 28,000 | 11 | 5½ | — | 2nd |
| 9 | 36,000 | 12 | 6 | 4th | — |
| 10 | 45,000 | 13 | 6½ | — | — |
| 11 | 55,000 | 14 | 7 | — | — |
| 12 | 66,000 | 15 | 7½ | 5th | 3rd |
| 13 | 78,000 | 16 | 8 | — | — |
| 14 | 91,000 | 17 | 8½ | — | — |
| 15 | 105,000 | 18 | 9 | 6th | — |
| 16 | 120,000 | 19 | 9½ | — | 4th |
| 17 | 136,000 | 20 | 10 | — | — |
| 18 | 153,000 | 21 | 10½ | 7th | — |
| 19 | 171,000 | 22 | 11 | — | — |
| 20 | 190,000 | 23 | 11½ | — | 5th |

Notes:
- **Class skill max ranks** = character level + 3.
- **Cross-class max ranks** = ½ class max. Half-ranks (.5) cost a skill point but don't improve checks; they're partial purchases toward the next full rank.
- **Feats:** 1st level + every 3 levels thereafter. Class bonus feats are *additional*.
- **Ability increases:** +1 to one ability at 4th level and every 4 levels thereafter (permanent).
- **For multiclass:** XP, feats, and ability increases all key off **character level** (sum), not individual class levels.

### Hit Dice by Class

| Hit Die | Classes |
|---|---|
| d4 | Initiate |
| d6 | Wanderer, Wilder |
| d8 | Noble |
| d10 | *Algai'd'siswai*, Armsman, Woodsman |

1st-level character takes **maximum** HP from first Hit Die. Each subsequent level: roll, add Con modifier, minimum 1 HP per level.

### Skill Points

- 1st level: `(per-level value + Int mod) × 4`.
- Each additional level: `per-level value + Int mod` (minimum 1).
- Class skill: 1 skill point = 1 rank (max = char level + 3).
- Cross-class skill: 1 skill point = ½ rank (max = ½ class max).
- **Exclusive class skills** cannot be bought by other classes at all (e.g., Animal Empathy is exclusive to woodsmen).

## Class Tables

### Table 3-2: *Algai'd'siswai*

| Lvl | BAB | Fort | Ref | Will | Def | Rep | Special |
|---|---|---|---|---|---|---|---|
| 1 | +1 | +0 | +1 | +0 | +4 | 0 | Fast movement, Weapon Focus (shortspear) |
| 2 | +2 | +0 | +2 | +0 | +5 | 1 | Dance the spears (+2 init) |
| 3 | +3 | +1 | +2 | +1 | +5 | 1 | Uncanny dodge (retain Dex to Defense) |
| 4 | +4 | +1 | +2 | +1 | +6 | 1 | — |
| 5 | +5 | +1 | +3 | +1 | +6 | 2 | Stealthy movement |
| 6 | +6/+1 | +2 | +3 | +2 | +7 | 2 | Uncanny dodge (can't be flanked) |
| 7 | +7/+2 | +2 | +4 | +2 | +7 | 3 | — |
| 8 | +8/+3 | +2 | +4 | +2 | +8 | 3 | Dance the spears (+4 init) |
| 9 | +9/+4 | +3 | +4 | +3 | +8 | 3 | — |
| 10 | +10/+5 | +3 | +5 | +3 | +9 | 4 | — |
| 11 | +11/+6/+1 | +3 | +5 | +3 | +9 | 4 | — |
| 12 | +12/+7/+2 | +4 | +6 | +4 | +10 | 5 | Uncanny dodge (+2 vs traps) |
| 13 | +13/+8/+3 | +4 | +6 | +4 | +10 | 5 | — |
| 14 | +14/+9/+4 | +4 | +6 | +4 | +11 | 5 | Dance the spears (+6 init) |
| 15 | +15/+10/+5 | +5 | +7 | +5 | +11 | 6 | — |
| 16 | +16/+11/+6/+1 | +5 | +7 | +5 | +12 | 6 | — |
| 17 | +17/+12/+7/+2 | +5 | +8 | +5 | +12 | 6 | — |
| 18 | +18/+13/+8/+3 | +6 | +8 | +6 | +13 | 7 | — |
| 19 | +19/+14/+9/+4 | +6 | +8 | +6 | +13 | 7 | — |
| 20 | +20/+15/+10/+5 | +6 | +9 | +6 | +14 | 8 | Dance the spears (+8 init) |

- **Hit Die:** d10
- **Abilities:** Str, Dex, Con, Wis
- **Class Skills:** Balance (Dex), Climb (Str), Craft (Int), Hide (Dex), Intimidate (Cha), Intuit Direction (Wis), Jump (Str), Listen (Wis), Move Silently (Dex), Wilderness Lore (Wis)
- **Skill Points:** 1st = (4 + Int) × 4; per level = 4 + Int
- **Weapon/Armor Proficiency:** All simple weapons **except sword** (despise swords). No armor proficiency. **Wearing armor → loses all class abilities and gains no XP.** **Using a sword → gains no XP** for that encounter.
- **Fast Movement:** Speed +10 ft over racial norm (40 ft for human).
- **Weapon Focus (shortspear):** at 1st level.
- **Dance the Spears:** +2 initiative at 2nd; +4 at 8th; +6 at 14th; +8 at 20th.
- **Uncanny Dodge:** retain Dex to Defense vs flat-footed/invisible attackers (3rd); cannot be flanked (6th); +2 Reflex and +2 dodge to Defense vs traps (12th). A wanderer ≥4 levels higher than the *algai'd'siswai* can still flank/sneak.
- **Stealthy Movement (5th):** add Reflex save bonus to all Move Silently and Hide checks.

### Table 3-3: Armsman

| Lvl | BAB | Fort | Ref | Will | Def | Rep | Special |
|---|---|---|---|---|---|---|---|
| 1 | +1 | +2 | +1 | +0 | +2 | 0 | Bonus feat |
| 2 | +2 | +3 | +2 | +0 | +2 | 1 | — |
| 3 | +3 | +3 | +2 | +1 | +3 | 1 | Armor compatibility |
| 4 | +4 | +4 | +2 | +1 | +3 | 1 | Bonus feat, Weapon Specialization |
| 5 | +5 | +4 | +3 | +1 | +3 | 2 | — |
| 6 | +6/+1 | +5 | +3 | +2 | +4 | 2 | Bonus feat |
| 7 | +7/+2 | +5 | +4 | +2 | +4 | 3 | — |
| 8 | +8/+3 | +6 | +4 | +2 | +4 | 3 | — |
| 9 | +9/+4 | +6 | +4 | +3 | +5 | 3 | — |
| 10 | +10/+5 | +7 | +5 | +3 | +5 | 4 | Bonus feat |
| 11 | +11/+6/+1 | +7 | +5 | +3 | +5 | 4 | — |
| 12 | +12/+7/+2 | +8 | +6 | +4 | +6 | 5 | Bonus feat |
| 13 | +13/+8/+3 | +8 | +6 | +4 | +6 | 5 | — |
| 14 | +14/+9/+4 | +9 | +6 | +4 | +6 | 5 | — |
| 15 | +15/+10/+5 | +9 | +7 | +5 | +7 | 6 | — |
| 16 | +16/+11/+6/+1 | +10 | +7 | +5 | +7 | 6 | Bonus feat |
| 17 | +17/+12/+7/+2 | +10 | +8 | +5 | +7 | 7 | — |
| 18 | +18/+13/+8/+3 | +11 | +8 | +6 | +8 | 7 | Bonus feat |
| 19 | +19/+14/+9/+4 | +11 | +8 | +6 | +8 | 7 | — |
| 20 | +20/+15/+10/+5 | +12 | +9 | +6 | +8 | 8 | Bonus feat |

- **Hit Die:** d10
- **Abilities:** Str, Con, Dex
- **Class Skills:** Climb (Str), Craft (Int), Handle Animal (Cha), Intimidate (Cha), Jump (Str), Ride (Dex), Swim (Str)
- **Skill Points:** 1st = (4 + Int) × 4; per level = 4 + Int
- **Weapon/Armor:** All simple + martial weapons; all armor (light, medium, heavy); shields.
- **Bonus Feats (1, 4, 6, 8, 10, 12, 14, 16, 18, 20):** Choose from: Ambidexterity, Blind-Fight, Combat Expertise (Improved Disarm, Improved Trip, Whirlwind Attack), Combat Reflexes, Dodge (Mobility, Spring Attack), Exotic Weapon Proficiency*, Improved Critical*, Improved Initiative, Improved Unarmed Strike, Mounted Combat (Mounted Archery, Trample, Ride-By Attack, Spirited Charge), Point Blank Shot (Far Shot, Precise Shot, Rapid Shot, Shot on the Run), Power Attack (Cleave, Improved Bull Rush, Sunder, Great Cleave), Quick Draw, Two-Weapon Fighting (Improved Two-Weapon Fighting), Weapon Finesse*, Weapon Focus*, Weapon Specialization*. Asterisked feats may be taken multiple times for different weapons. All standard prerequisites apply.
- **Armor Compatibility (3rd):** Class Defense bonus stacks with armor + shield equipment bonuses (normally Defense doesn't stack with armor). Multiclass: only the armsman levels' Defense bonus stacks; the −2 multiclass Defense penalty does not affect armor compatibility.
- **Weapon Specialization (4th+):** +2 damage with chosen weapon (must already have Weapon Focus in it). For ranged weapons, only applies within 30 ft. May be taken as bonus or regular feat. Other classes cannot take Weapon Specialization unless explicitly noted.

### Table 3-4: Initiate

| Lvl | BAB | Fort | Ref | Will | Def | Rep | Special |
|---|---|---|---|---|---|---|---|
| 1 | +0 | +2 | +1 | +2 | +2 | 1 | Bonus channeling feat, Weavesight |
| 2 | +1 | +3 | +2 | +3 | +2 | 1 | Bonus channeling feat |
| 3 | +1 | +3 | +2 | +3 | +3 | 2 | Slow aging |
| 4 | +2 | +4 | +2 | +4 | +3 | 2 | Bonus channeling feat |
| 5 | +2 | +4 | +3 | +4 | +3 | 3 | — |
| 6 | +3 | +5 | +3 | +5 | +4 | 3 | Bonus channeling feat |
| 7 | +3 | +5 | +4 | +5 | +4 | 4 | — |
| 8 | +4 | +6 | +4 | +6 | +4 | 4 | Bonus channeling feat |
| 9 | +4 | +6 | +4 | +6 | +5 | 5 | — |
| 10 | +5 | +7 | +5 | +7 | +5 | 5 | Bonus channeling feat |
| 11 | +5 | +7 | +5 | +7 | +5 | 6 | — |
| 12 | +6/+1 | +8 | +6 | +8 | +6 | 6 | Bonus channeling feat |
| 13 | +6/+1 | +8 | +6 | +8 | +6 | 7 | — |
| 14 | +7/+2 | +9 | +6 | +9 | +6 | 7 | Bonus channeling feat |
| 15 | +7/+2 | +9 | +7 | +9 | +7 | 8 | — |
| 16 | +8/+3 | +10 | +7 | +10 | +7 | 8 | Bonus channeling feat |
| 17 | +8/+3 | +10 | +8 | +10 | +7 | 9 | — |
| 18 | +9/+4 | +11 | +8 | +11 | +8 | 9 | Bonus channeling feat |
| 19 | +9/+4 | +11 | +8 | +11 | +8 | 10 | — |
| 20 | +10/+5 | +12 | +9 | +12 | +8 | 10 | Bonus channeling feat |

- **Hit Die:** d4
- **Abilities:** Int (weave power & weaves/day), Wis (DC of saves vs weaves & weaves/day), Dex, Con
- **Class Skills:** Composure (Wis), Concentration (Wis), Decipher Script (Int), Diplomacy (Cha), Gather Information (Cha), Heal (Wis), Intimidate (Cha), Invert (Int), Knowledge (varies) (Int), Sense Motive (Wis), Weavesight (Int)
- **Skill Points:** 1st = (4 + Int) × 4; per level = 4 + Int
- **Weapon/Armor:** Club + dagger only. **Not** proficient with armor or shields.
- **Affinity:** Begins with one Affinity (Air, Earth, Fire, Spirit, Water). Female: Air, Spirit, Water. Male: Earth, Fire, Spirit. Additional Affinity feat grants more.
- **Talents and Weaves:** 1st level: 1 common Talent + 8 common weaves of 0 or 1st level. May only learn/cast weaves within their Talents (except 0-level weaves, which any initiate can learn).
- **Cross-Talent Weaves:** Initiates may cast 0-level weaves of any Power; 1st-level+ weaves require Talent.
- **Weaves/Day:** See Table 3-5 below; bonus weaves from Int and Wis stack.
- **Casting Threshold:** Int ≥ 10 + weave level required to cast a weave.
- **Save DC** vs initiate's weave = 10 + weave level + Int modifier.
- **Overchanneling:** Possible (no bonus, unlike wilders).
- **Bonus Channeling Feat:** 1st level + every 2 levels thereafter (2nd, 4th, 6th, ...). May choose any channeling feat meeting prereqs. Stacks with normal feat slots.
- **Weavesight (1st):** +4 competence bonus on Weavesight checks.
- **Slow Aging (3rd):** Age 1 year per (level / 2) years.
- **Tradition & Mentor:** Aes Sedai, Wise Ones (Aiel only, female only), Atha'an Miere Windfinders (female only), Asha'man (male only). 1st-level rank varies: Aes Sedai = Accepted; Asha'man = Dedicated; Wise Ones / Windfinders = senior apprentice. Multiclassing into the prestige class for full status (Aes Sedai, etc.) typically becomes possible around 6th level. Failing to obey mentor = potentially dangerous confrontation.
- **Always human, never Ogier.**

### Table 3-5: Initiate Weaves per Day

| Lvl | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 4 | 1 | — | — | — | — | — | — | — | — |
| 2 | 4 | 2 | — | — | — | — | — | — | — | — |
| 3 | 4 | 2 | 1 | — | — | — | — | — | — | — |
| 4 | 4 | 3 | 2 | — | — | — | — | — | — | — |
| 5 | 4 | 3 | 2 | 1 | — | — | — | — | — | — |
| 6 | 4 | 3 | 3 | 2 | — | — | — | — | — | — |
| 7 | 4 | 3 | 3 | 2 | 1 | — | — | — | — | — |
| 8 | 4 | 3 | 3 | 3 | 2 | — | — | — | — | — |
| 9 | 4 | 4 | 3 | 3 | 2 | 1 | — | — | — | — |
| 10 | 4 | 4 | 3 | 3 | 3 | 2 | — | — | — | — |
| 11 | 4 | 4 | 4 | 3 | 3 | 2 | 1 | — | — | — |
| 12 | 4 | 4 | 4 | 3 | 3 | 3 | 2 | — | — | — |
| 13 | 4 | 4 | 4 | 4 | 3 | 3 | 2 | 1 | — | — |
| 14 | 4 | 4 | 4 | 4 | 3 | 3 | 3 | 2 | — | — |
| 15 | 4 | 4 | 4 | 4 | 4 | 3 | 3 | 2 | 1 | — |
| 16 | 4 | 4 | 4 | 4 | 4 | 3 | 3 | 3 | 2 | — |
| 17 | 4 | 4 | 4 | 4 | 4 | 4 | 3 | 3 | 2 | 1 |
| 18 | 4 | 4 | 4 | 4 | 4 | 4 | 3 | 3 | 3 | 2 |
| 19 | 4 | 4 | 4 | 4 | 4 | 4 | 4 | 3 | 3 | 3 |
| 20 | 4 | 4 | 4 | 4 | 4 | 4 | 4 | 4 | 4 | 4 |

A higher-level slot may be used to cast a lower-level weave; the weave is still treated as its actual level, not the slot's level.

### Table 3-6: Noble

| Lvl | BAB | Fort | Ref | Will | Def | Rep | Special |
|---|---|---|---|---|---|---|---|
| 1 | +0 | +0 | +1 | +2 | +3 | 3 | Bonus class skill, Call in a favor |
| 2 | +1 | +0 | +2 | +3 | +4 | 4 | Inspire confidence +1 |
| 3 | +2 | +1 | +2 | +3 | +4 | 4 | Call in a favor |
| 4 | +3 | +1 | +2 | +4 | +4 | 5 | — |
| 5 | +3 | +1 | +3 | +4 | +5 | 5 | Call in a favor |
| 6 | +4 | +2 | +3 | +5 | +5 | 6 | Inspire confidence +2 |
| 7 | +5 | +2 | +4 | +5 | +6 | 6 | Call in a favor |
| 8 | +6/+1 | +2 | +4 | +6 | +6 | 7 | Command +4 |
| 9 | +6/+1 | +3 | +4 | +6 | +6 | 7 | Call in a favor |
| 10 | +7/+2 | +3 | +5 | +7 | +7 | 8 | Inspire confidence +3 |
| 11 | +8/+3 | +3 | +5 | +7 | +7 | 8 | Call in a favor |
| 12 | +9/+4 | +4 | +6 | +8 | +8 | 9 | Command +6 |
| 13 | +9/+4 | +4 | +6 | +8 | +8 | 9 | Call in a favor |
| 14 | +10/+5 | +4 | +6 | +9 | +8 | 10 | Inspire confidence +4 |
| 15 | +11/+6/+1 | +5 | +7 | +9 | +9 | 10 | Call in a favor |
| 16 | +12/+7/+2 | +5 | +7 | +10 | +9 | 11 | Command +8 |
| 17 | +12/+7/+2 | +5 | +8 | +10 | +10 | 11 | Call in a favor |
| 18 | +13/+8/+3 | +6 | +8 | +11 | +10 | 12 | Inspire confidence +5 |
| 19 | +14/+9/+4 | +6 | +8 | +11 | +10 | 12 | Call in a favor |
| 20 | +15/+10/+5 | +6 | +9 | +12 | +11 | 13 | Command +10 |

(Command +2 at 4th, then +4/+6/+8/+10 every 4 levels per the description; Inspire Confidence increases +1 every 4 levels after 2nd.)

- **Hit Die:** d8
- **Abilities:** Cha (primary), Wis, Int
- **Class Skills:** Appraise (Int), Bluff (Cha), Diplomacy (Cha), Gather Information (Cha), Innuendo (Cha), Intimidate (Cha), Knowledge (all, taken individually) (Int), Listen (Wis), Perform (Cha), Ride (Dex), Sense Motive (Wis), Speak Language (none)
- **Skill Points:** 1st = (4 + Int) × 4; per level = 4 + Int
- **Weapon/Armor:** All simple + martial weapons; light armor; shields.
- **Bonus Class Skill (1st):** Designate any one cross-class skill (except a channeling skill) as a class skill — represents an "illicit" or "unapproved" expertise.
- **Call in a Favor (1st, 3rd, 5th, ...):** Special Charisma check (+ noble's level), GM-set DC (10 simple, 25+ illegal). Cannot take 10 or 20. No multiple attempts at the same favor (different favor allowed). Can stockpile, max 5 stored. Failed favor is gone forever.
- **Inspire Confidence (2nd):** Speak ≥1 round; allies hearing must be in earshot. Diplomacy DC = 10 + 1 per 5 allies (incl. noble). Allies get +1 attacks, +1 skill, +1 Will (competence). Lasts 10 min/round spoken (max 5 hr/30 rd). Once/day; failure or success blocks reuse for 24 hr. Bonus increases +1 every 4 levels after 2nd (max +5 at 18th).
- **Command (4th):** Cha check (DC 15 + chars commanded) → cooperation bonus +2 (Cooperation, p. 70). +2 every 4 levels (max +10 at 20th). Full-round action minimum.

### Table 3-7: Wanderer

| Lvl | BAB | Fort | Ref | Will | Def | Rep | Special |
|---|---|---|---|---|---|---|---|
| 1 | +0 | +0 | +2 | +1 | +3 | 0 | Illicit barter |
| 2 | +1 | +0 | +3 | +2 | +4 | 1 | The Dark One's Own Luck |
| 3 | +2 | +1 | +3 | +2 | +4 | 1 | — |
| 4 | +3 | +1 | +4 | +2 | +4 | 1 | Skill Emphasis |
| 5 | +3 | +1 | +4 | +3 | +5 | 2 | Sneak attack +2d6 |
| 6 | +4 | +2 | +5 | +3 | +5 | 2 | — |
| 7 | +5 | +2 | +5 | +4 | +6 | 3 | — |
| 8 | +6/+1 | +2 | +6 | +4 | +6 | 3 | Skill Emphasis |
| 9 | +6/+1 | +3 | +6 | +4 | +6 | 3 | — |
| 10 | +7/+2 | +3 | +7 | +5 | +7 | 4 | Bonus feat |
| 11 | +8/+3 | +3 | +7 | +5 | +7 | 4 | — |
| 12 | +9/+4 | +4 | +8 | +6 | +8 | 5 | Skill Emphasis |
| 13 | +9/+4 | +4 | +8 | +6 | +8 | 5 | — |
| 14 | +10/+5 | +4 | +9 | +6 | +8 | 5 | Sneak attack +4d6 |
| 15 | +11/+6/+1 | +5 | +9 | +7 | +9 | 6 | — |
| 16 | +12/+7/+2 | +5 | +10 | +7 | +9 | 6 | Skill Emphasis |
| 17 | +12/+7/+2 | +5 | +10 | +8 | +10 | 7 | — |
| 18 | +13/+8/+3 | +6 | +11 | +8 | +10 | 7 | — |
| 19 | +14/+9/+4 | +6 | +11 | +8 | +10 | 7 | — |
| 20 | +15/+10/+5 | +6 | +12 | +9 | +11 | 8 | Skill Emphasis |

- **Hit Die:** d6
- **Abilities:** Dex, Int, Wis
- **Class Skills:** Appraise (Int), Balance (Dex), Bluff (Cha), Climb (Str), Craft (Int), Diplomacy (Cha), Disable Device (Int), Disguise (Cha), Escape Artist (Dex), Forgery (Int), Gather Information (Cha), Hide (Dex), Innuendo (Wis), Intimidate (Cha), Intuit Direction (Wis), Jump (Str), Knowledge (varies) (Int), Listen (Wis), Move Silently (Dex), Open Lock (Dex), Perform (Cha), Pick Pocket (Dex), Profession (Wis), Read Lips (Int), Search (Int), Sense Motive (Wis), Spot (Wis), Swim (Str), Tumble (Dex), Use Rope (Dex)
- **Skill Points:** 1st = (8 + Int) × 4; per level = 8 + Int (highest of any class)
- **Weapon/Armor:** Club, crossbow, dagger (any), dart, mace (light/heavy), morningstar, quarterstaff, rapier, sap, shortbow (normal/composite), short sword. Light armor; **no** shields.
- **Illicit Barter (1st):** +5 competence bonus on Diplomacy buying/selling illicit goods.
- **The Dark One's Own Luck (2nd):** Bonus feat.
- **Skill Emphasis (4th, 8th, 12th, 16th, 20th):** Bonus feat applied to any class skill (no skill twice).
- **Sneak Attack (5th):** +2d6 vs targets denied Dex bonus or flanked. +4d6 at 14th. Ranged sneak attack only within 10 paces. Cannot sneak attack: targets with concealment, beyond reach, immune to crits, lacking discernible anatomy. Wanderer must see vital spot. (Note: an *algai'd'siswai* ≥6th level cannot be flanked normally; wanderers ≥4 levels higher than the *algai'd'siswai* can still flank/sneak.)
- **Bonus Feat (10th):** Choose from Alertness, Dodge, Fame, Heroic Surge, Improved Initiative, Infamy, Low Profile, Weapon Finesse, Weapon Focus.

### Table 3-8: Wilder

| Lvl | BAB | Fort | Ref | Will | Def | Rep | Special |
|---|---|---|---|---|---|---|---|
| 1 | +0 | +1 | +2 | +2 | +3 | 0 | Block |
| 2 | +1 | +2 | +3 | +3 | +4 | 0 | Bonus channeling feat |
| 3 | +1 | +2 | +3 | +3 | +4 | 1 | Slow aging |
| 4 | +2 | +2 | +4 | +4 | +4 | 1 | — |
| 5 | +2 | +3 | +4 | +4 | +5 | 1 | Bonus channeling feat |
| 6 | +3 | +3 | +5 | +5 | +5 | 2 | — |
| 7 | +3 | +4 | +5 | +5 | +6 | 2 | — |
| 8 | +4 | +4 | +6 | +6 | +6 | 2 | Bonus channeling feat |
| 9 | +4 | +4 | +6 | +6 | +6 | 3 | — |
| 10 | +5 | +5 | +7 | +7 | +7 | 3 | — |
| 11 | +5 | +5 | +7 | +7 | +7 | 4 | Bonus channeling feat |
| 12 | +6/+1 | +6 | +8 | +8 | +8 | 4 | — |
| 13 | +6/+1 | +6 | +8 | +8 | +8 | 4 | — |
| 14 | +7/+2 | +6 | +9 | +9 | +9 | 5 | Bonus channeling feat |
| 15 | +7/+2 | +7 | +9 | +9 | +9 | 5 | — |
| 16 | +8/+3 | +7 | +10 | +10 | +9 | 5 | — |
| 17 | +8/+3 | +8 | +10 | +10 | +10 | 6 | Bonus channeling feat |
| 18 | +9/+4 | +8 | +11 | +11 | +10 | 6 | — |
| 19 | +9/+4 | +8 | +11 | +11 | +10 | 6 | — |
| 20 | +10/+5 | +9 | +12 | +12 | +11 | 6 | Bonus channeling feat |

- **Hit Die:** d6
- **Abilities:** Wis (weave power & DC), Cha (weaves/day with Wis), Dex, Con
- **Class Skills:** Composure (Wis), Concentration (Wis), Craft (Int), Gather Information (Cha), Heal (Wis), Intimidate (Cha), Invert (Int), Knowledge (varies) (Int), Profession (Wis), Sense Motive (Wis), Weavesight (Int)
- **Skill Points:** 1st = (4 + Int) × 4; per level = 4 + Int
- **Weapon/Armor:** All simple weapons; light armor only.
- **Affinity:** As initiate. Female: Air, Spirit, Water. Male: Earth, Fire, Spirit.
- **Talents and Weaves:** 1st level: 1 common Talent + 6 common weaves of 0 or 1st level. Starting weaves need not be within Talent. Each level: learn 1 weave of any level she can cast (i.e., learnable up to current max castable level). May learn any 0/1st/2nd weave regardless of Talent; 3rd+ requires Talent.
- **Casting Threshold:** Wis ≥ 10 + weave level required.
- **Save DC** vs wilder's weave = 10 + weave level + Wis modifier.
- **Block:** Emotional trigger required to channel. Composure check DC 15 to attain emotional state (DC 20 if currently in opposed emotion, DC 10 if already similar). Once attained, can hold state for 1 hour without re-rolling. Eliminate Block feat removes it (female wilders: only at 3rd level+; male: any time, may even take it at 1st level).
- **Overchanneling:** Wilders are more practiced; +5 competence on Concentration to overchannel and +5 competence on Fortitude saves vs failed overchannels.
- **Cross-Talent Weaves:** May freely cast 0/1st/2nd-level weaves; 3rd+ requires Talent.
- **Bonus Channeling Feat:** 2nd level + every 3 levels (5, 8, 11, 14, 17, 20).
- **Slow Aging (3rd):** Age 1 year per (level / 2) years.
- **Always human, never Ogier.**

### Table 3-9: Wilder Weaves per Day

| Lvl | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 2 | 1 | — | — | — | — | — | — | — | — |
| 2 | 3 | 1 | 1 | — | — | — | — | — | — | — |
| 3 | 3 | 2 | 1 | 1 | — | — | — | — | — | — |
| 4 | 4 | 2 | 2 | 1 | — | — | — | — | — | — |
| 5 | 4 | 2 | 2 | 1 | 1 | — | — | — | — | — |
| 6 | 5 | 3 | 2 | 2 | 1 | — | — | — | — | — |
| 7 | 5 | 3 | 3 | 2 | 1 | 1 | — | — | — | — |
| 8 | 6 | 3 | 3 | 2 | 2 | 1 | — | — | — | — |
| 9 | 6 | 4 | 3 | 3 | 2 | 1 | 1 | — | — | — |
| 10 | 6 | 4 | 4 | 3 | 2 | 2 | 1 | — | — | — |
| 11 | 6 | 4 | 4 | 3 | 3 | 2 | 1 | 1 | — | — |
| 12 | 6 | 4 | 4 | 4 | 3 | 2 | 2 | 1 | — | — |
| 13 | 6 | 5 | 4 | 4 | 3 | 3 | 2 | 1 | 1 | — |
| 14 | 6 | 5 | 5 | 4 | 4 | 3 | 2 | 2 | 1 | — |
| 15 | 6 | 5 | 5 | 4 | 4 | 4 | 2 | 1 | 1 | 1 |
| 16 | 6 | 5 | 5 | 5 | 4 | 4 | 3 | 2 | 1 | 1 |
| 17 | 6 | 6 | 5 | 5 | 4 | 4 | 3 | 2 | 1 | 1 |
| 18 | 6 | 6 | 6 | 5 | 5 | 4 | 4 | 2 | 1 | 1 |
| 19 | 6 | 6 | 6 | 6 | 5 | 4 | 4 | 3 | 2 | 1 |
| 20 | 6 | 6 | 6 | 6 | 5 | 5 | 4 | 3 | 2 | 1 |

### Table 3-10: Woodsman

| Lvl | BAB | Fort | Ref | Will | Def | Rep | Special |
|---|---|---|---|---|---|---|---|
| 1 | +1 | +1 | +0 | +0 | +3 | 0 | Nature's warrior (one env), Track |
| 2 | +2 | +2 | +0 | +0 | +4 | 0 | Partial Improved Initiative |
| 3 | +3 | +2 | +1 | +1 | +4 | 1 | Woodland stealth +2 |
| 4 | +4 | +2 | +1 | +1 | +4 | 1 | Bonus feat |
| 5 | +5 | +3 | +1 | +1 | +5 | 1 | — |
| 6 | +6/+1 | +3 | +2 | +2 | +5 | 2 | Weapon Specialization |
| 7 | +7/+2 | +4 | +2 | +2 | +6 | 2 | — |
| 8 | +8/+3 | +4 | +2 | +2 | +6 | 2 | Nature's warrior (two envs) |
| 9 | +9/+4 | +4 | +3 | +3 | +6 | 3 | Bonus feat |
| 10 | +10/+5 | +5 | +3 | +3 | +7 | 3 | — |
| 11 | +11/+6/+1 | +5 | +3 | +3 | +7 | 3 | — |
| 12 | +12/+7/+2 | +6 | +4 | +4 | +8 | 4 | Woodland stealth +4 |
| 13 | +13/+8/+3 | +6 | +4 | +4 | +8 | 4 | — |
| 14 | +14/+9/+4 | +6 | +4 | +4 | +8 | 4 | Bonus feat |
| 15 | +15/+10/+5 | +7 | +5 | +5 | +9 | 5 | — |
| 16 | +16/+11/+6/+1 | +7 | +5 | +5 | +9 | 5 | Nature's warrior (three envs) |
| 17 | +17/+12/+7/+2 | +8 | +5 | +5 | +10 | 5 | — |
| 18 | +18/+13/+8/+3 | +8 | +6 | +6 | +10 | 6 | — |
| 19 | +19/+14/+9/+4 | +8 | +6 | +6 | +10 | 6 | Bonus feat |
| 20 | +20/+15/+10/+5 | +9 | +6 | +6 | +11 | 6 | — |

- **Hit Die:** d10
- **Abilities:** Dex, Str, Con, Wis
- **Class Skills:** Animal Empathy (Cha) **[exclusive]**, Climb (Str), Craft (Int), Handle Animal (Cha), Heal (Wis), Hide (Dex), Intimidate (Cha), Intuit Direction (Wis), Jump (Str), Knowledge (nature) (Int), Listen (Wis), Move Silently (Dex), Profession (Wis), Ride (Dex), Search (Int), Spot (Wis), Swim (Str), Use Rope (Dex), Wilderness Lore (Wis)
- **Skill Points:** 1st = (6 + Int) × 4; per level = 6 + Int
- **Weapon/Armor:** All simple + martial weapons; light + medium armor; shields.
- **Nature's Warrior (1st, 8th, 16th):** In a chosen natural environment (forest, swamp, plains, mountains, Waste, Blight), add ½ Dex bonus to attack rolls vs humanoids (humans, Trollocs, Myrddraal, etc.) — added to Str bonus. Choose one environment at 1st, second at 8th, third at 16th. May *also* apply to wild beasts in any environment.
- **Track (1st):** Bonus feat.
- **Partial Improved Initiative (2nd):** Improved Initiative bonus when wearing light or no armor; lost in medium/heavy.
- **Woodland Stealth (3rd):** +2 Hide and Move Silently in forests/natural environments. Increases to +4 at 12th.
- **Bonus Feats (4th, 9th, 14th, 19th):** Same list as armsman.
- **Weapon Specialization (6th):** Same as armsman; requires Weapon Focus.

## Multiclassing

### Adding a Second Class

When a single-class character gains a level, they may:
- Increase their existing class by 1, **or**
- Pick up a new class at 1st level (subject to GM approval).

**Picking up a new class** grants:
- All 1st-level base attack, base saves, class skills, class features.
- A Hit Die of the new class's type.
- Per-level skill points (not 4× quadrupled).

But NOT (these are exclusive to actually starting in that class):
- Maximum HP from first Hit Die.
- Quadruple skill points.
- Starting equipment.
- Starting gold.

### Multiclass Mechanics

- **Character level:** Sum of all class levels. Drives XP, feats, ability increases (per Table 3-1).
- **Channeler level:** Sum of all channeling class levels (initiate + wilder).
- **Class level:** Per-class level (drives class-table lookups).
- **Hit Dice:** One per class level, rolled per its die type.
- **Base Attack Bonus:** Sum each class's BAB. Iterative attacks (extra attacks at +6+ BAB) come from the *combined* total. Example: 6th wanderer/4th armsman = +4 + +4 = +8/+3 (gets a second attack at +3 even though neither single class would).
- **Saves:** Sum all per-class base saves.
- **Defense Bonus:** Sum all class Defense bonuses, then **subtract 2 for every additional class beyond the first**. (Armor compatibility is unaffected by this −2 penalty.)
  - Example: 4th noble/1st armsman = +3 + +2 − 2 = +3.
  - Example: + a wanderer level on top → +3 + +3 − 2 (one additional class beyond the second class? RAW reads: subtract 2 *for each class after the first*, so 3 classes = −4). Re-applying example given: 4th noble/1st armsman has Def +3 + +2 − 2 = +3; adding a wanderer level → +3 + +3 + +2 − 4 = +4 per the example given on p. 63 ("she would add +3 and subtract 2 (for having yet another class), for a total Defense bonus of +4"). So the penalty is **−2 per *additional* class beyond the first** (i.e., −2 once for two classes, −4 for three classes).
- **Reputation:** Sum across all classes.
- **Skills:** Skill points per level use the class just leveled in. A skill is class-skill if it's class-skill for **any** of the character's classes. Max ranks = char level + 3 (class) or ½ that (cross-class). **Exclusive skills** (e.g., Animal Empathy for woodsmen) — if a class doesn't have access, levels in that class don't increase the max-rank cap; only the levels in the access-granting class do.
- **Starting feats:** Get all 1st-level starting feats of every class taken (ignoring redundancy).
- **Class features:** All of them — but also all restrictions stack (e.g., heavy armor still penalizes channelers' ability to function as such).
- **Feats:** Awarded by character level (every 3 levels per Table 3-1), independent of class.
- **Ability increases:** Awarded by character level (every 4 levels), independent of class.
- **Caster level (weaves):** Sum of all channeling class levels.

### Multiclass Restrictions and Pitfalls

- **Wilders & Initiates** are always human. Never Ogier.
- A wilder/armsman gets armsman armor proficiency, but heavy armor causes spellcasting issues for the wilder side (and skill penalties).
- An *algai'd'siswai* with armor levels still loses *algai'd'siswai* class abilities while wearing armor. Sword use → no XP for that encounter.
- An *algai'd'siswai* with sword-using class levels (e.g., armsman) still cannot use a sword and gain XP from the encounter.
- A wilder who multiclasses into a non-channeling class still gets the +1 to Madness rating per level (males only — see "Men Who Can Channel" below).

## Men Who Can Channel

(Cross-cutting rule for both initiate and wilder classes.)

- **Madness:** Every male channeler has a secret Madness rating. GM secretly rolls 1d6 at character creation. Madness adds:
  - +1 every time the character overchannels.
  - +1d6 every level gained (any class), as long as the character has a channeling class or adds one.
- **Madness effects:** Will saves to suppress outbursts; eventually permanent insanity and the rotting taint disease.
- **Mental Stability feat** reduces Madness rating.
- **Bonus Weaves:** Male channelers gain 5 bonus weaves: 1 each at 1st, 2nd, 3rd, 4th, and 5th level (usable when the channeler is high enough level to cast them).
- **Block:** Male wilders **can** take Eliminate Block at 1st level (effectively starting without a Block). Female wilders cannot take it until 3rd level.
- **Linking:** While female channelers can link without men, men cannot link without including women in the group.

## Implementation Notes for WheelMUD

```go
type ClassID string

const (
    ClassAlgaiDSiswai ClassID = "algai_dsiswai"
    ClassArmsman      ClassID = "armsman"
    ClassInitiate     ClassID = "initiate"
    ClassNoble        ClassID = "noble"
    ClassWanderer     ClassID = "wanderer"
    ClassWilder       ClassID = "wilder"
    ClassWoodsman     ClassID = "woodsman"
)

type ClassLevel struct {
    Class    ClassID
    Level    int
    HitDie   int  // 4, 6, 8, or 10
}

type Character struct {
    ClassLevels []ClassLevel  // ordered by acquisition for "first class" determination
    XP          int
    // derived:
    CharLevel     int  // sum of ClassLevels[*].Level — drives feats, ability bumps
    ChannelerLvl  int  // sum of initiate+wilder levels — drives weave caster level
}

func (c *Character) BaseAttackBonus() int { /* sum per class table */ }
func (c *Character) BaseSave(s SaveType) int { /* sum per class table */ }
func (c *Character) DefenseBonus() int {
    // sum class def bonuses, then subtract 2*(numClasses - 1)
    return classDefSum - 2*(len(c.ClassLevels)-1)
}
```

### Engine Invariants

1. **Iterative attacks** come from cumulative BAB ≥ 6, not per-class. A 6-wand/4-arm gets +8/+3 even though each individually would only get one attack.
2. **Skill rank caps** are character-level-driven (lvl + 3 for class skills, half that for cross-class), but **exclusive skills** require at least 1 class level granting access; otherwise the cap stays at the level when access stopped applying. (For the first MVP, treat exclusive skills as "only purchasable if you currently have a class level granting them.")
3. **Defense bonus −2 per additional class beyond the first.** Track `len(ClassLevels)` distinct classes. Armor compatibility is exempt from this penalty.
4. **Channeling caster level = total levels in channeling classes only.**
5. **Weaves/day** drawn from per-class table; bonus weaves from ability scores stack on top (cap at 9th level even with very high stats — see abilities reference doc).
6. **Initiate Talent restriction:** initiates can only learn/cast weaves of their Talent(s), except 0-level weaves. Wilders: Talent restriction kicks in only at 3rd-level weaves.
7. **Block (wilder):** require an emotional state via Composure check before the wilder can channel. Track current emotion vs block-emotion; block is per-character, eliminable via feat (with female-3rd-level restriction).
8. **Madness rating (male channelers):** secret int per male-channeler character. Increment +1d6 per level (any class) and +1 per overchannel. Required as a hidden field for each male character with at least one channeler class level. Mental Stability feat decrements it.
9. **Slow Aging (initiate/wilder, 3rd+):** age tick = real-world or in-game year passes ⇒ apply (level/2) divisor on aging effects.
10. **Algai'd'siswai sword/armor restrictions:** flag during combat resolution and XP grant. If sword used in encounter → grant 0 XP. If armor worn → suppress *algai'd'siswai* class features (AC bonus, fast move, uncanny dodge, etc.) and grant 0 XP.
11. **Noble "Call in a favor"** is GM-mediated; in MUD terms this maps to scripted favor-tokens with cooldowns and DC checks against npc-arranged outcomes. Stored max 5; failed favor permanently consumes it.
12. **Inspire Confidence / Command** are buff effects with timer/duration semantics — model as a buff applied to allies-in-earshot for a duration.
13. **Wanderer Sneak Attack** requires line-of-sight to a vital area, target with "discernible anatomy", and either denied-Dex-bonus or flanked. Range cap 10 paces (~30 ft) for ranged.
14. **Woodsman Nature's Warrior** environment-bound: store a chosen list of envs on the character; check current room's environment tag at attack-resolution time.
15. **Weave slot upcasting:** higher-level slot can cast lower-level weave; weave still treated as its actual level (DC, save, etc.).
16. **Multiclass entry restriction:** no quadruple skill points, no max HP, no starting equipment/gold for the new class.
17. **Initiate / wilder are always human.** Block Ogier from selecting these classes at character creation and at multiclass-add-time.

### Open Questions

- **Multiclass Defense penalty:** the example on p. 63 implies linear stacking (−2 per additional class), so 3 classes = −4, 4 classes = −6. Confirm before building. Implementation should match RAW exactly.
- **Madness rating:** does it persist during sleep/quit? Yes — it's a permanent character stat. Where does it surface in UI? GM-secret, so probably never to the player (only via scripted madness-event triggers).
- **Talent assignment:** initiate/wilder's first Talent is chosen at creation. Need a Talent catalog. (Defer to Chapter 9 reference doc.)
- **Reputation Score** drives social mechanics (noble's "Call in a favor", Fame/Infamy feats). Needs its own system; treat as a stat for now.
- **Affinity** affects weave power and difficulty. Need data model for which weaves use which Powers and how Affinity adjusts them. (Defer to Chapter 9 reference doc.)
- **Track feat (woodsman):** maps to a `track` command with skill-check resolution against time/terrain DC.
