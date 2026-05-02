# Equipment

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 7: Equipment, pp. 112–129) for use in WheelMUD implementation. Ter'angreal and One Power items live in Chapter 14 and are out of scope here.

## Starting a Character

A starting character receives random starting money based on class, plus one free clothing outfit (artisan's, cadin'sor [Aiel only], explorer's, peasant's, scholar's, or traveler's). Background may also confer items (Chapter 2).

### Random Starting Money (Table 7-1)

| Class | Amount |
|-------|--------|
| Algai'd'siswai | 3d4 × 10 mk |
| Armsman | 5d4 × 10 mk |
| Initiate | 3d4 × 10 mk |
| Noble | 5d6 × 10 mk |
| Wanderer | 4d4 × 10 mk |
| Wilder | 3d4 × 10 mk |
| Woodsman | 4d4 × 10 mk |

### Availability

- Anything ≤ 300 gc is generally purchasable wherever the character can find merchants.
- More expensive items require travel to a major city, custom orders, or a price premium.
- GM gates availability by location; small towns cannot supply, e.g., full plate.

## Wealth and Money

### Coins

| Coin | Abbrev | Notes |
|------|--------|-------|
| Copper penny | cp | candle, torch, chalk |
| Silver penny | sp | day's labor, common lamp, poor meal |
| Silver mark | mk | belt pouch, 50 ft hemp rope, goat; standard wealth unit |
| Gold crown | gc | nobility/banker unit; aka gold mark |

Standard coin weight: ~1/3 oz (50 to the pound).

### Exchange (Table 7-2)

| | cp | sp | mk | gc |
|--|----|----|----|----|
| 1 cp | 1 | 1/10 | 1/100 | 1/1,000 |
| 1 sp | 10 | 1 | 1/10 | 1/100 |
| 1 mk | 100 | 10 | 1 | 1/10 |
| 1 gc | 1,000 | 100 | 10 | 1 |

### Wealth Other Than Coins

Most wealth is non-coin: livestock, grain, land, taxation rights, mineral/forest access, gems, jewelry. Guilds, nobles, and royalty regulate trade; merchants frequently barter trade goods directly.

### Trade Goods (Table 7-3)

| Commodity | Cost |
|-----------|------|
| Chicken, 1 | 2 cp |
| Copper, 1 lb | 5 sp |
| Cow, 1 | 10 mk |
| Dog, 1 | 25 mk |
| Flour, 1 lb | 2 cp |
| Ice peppers, 1 lb | 5 mk |
| Goat, 1 | 1 mk |
| Gold, 1 lb | 5 gc |
| Iron, 1 lb | 1 sp |
| Ivory, 1 lb | 6 gc |
| Kaf, 1 lb | 3 gc |
| Linen, 1 lb (sq yd) | 4 mk |
| Ox, 1 | 15 mk |
| Pig, 1 | 3 mk |
| Salt, 1 lb | 5 mk |
| Sea Folk porcelain (1 piece) | 10 mk |
| Sheep, 1 | 2 mk |
| Silk, 1 lb (2 sq yd) | 20 mk |
| Silver, 1 lb | 10 mk |
| Exotic spices, 1 lb | 15 mk |
| Tabac, 1 lb | 2 gc |
| Tea leaves, 1 lb | 2 sp |
| Wheat, 1 lb | 1 cp |

### Selling Loot

- General rule: sell anything for **half** its listed price.
- Commodities (Table 7-3) are exception — they exchange near full value because they are functionally cash.

## Weapons

### Weapon Categories

- **Proficiency tier:** Simple / Martial / Exotic. Non-proficient use → -4 to attack rolls. Class proficiency:
  - All except initiates and wanderers → all simple weapons.
  - Armsmen, nobles, and woodsmen → all simple + all martial weapons.
  - Other classes → assorted simple plus a few martial/exotic.
  - Feats: Simple Weapon Proficiency, Martial Weapon Proficiency, Exotic Weapon Proficiency.
- **Range:** Melee vs. Ranged (thrown or projectile). Strength bonus to damage applies to melee and thrown, **not** projectile (with negative-Str penalty applying to bow/sling damage).
- **Size vs. wielder:**
  - Smaller than wielder → **light** (off-hand friendly, usable while grappling).
  - Same size → **one-handed** (two-handed melee use grants 1.5× Str to damage).
  - One step larger → **two-handed** (1.5× Str to damage in two hands; one-handed throw possible at full-round cost).
  - Two+ steps larger → **too large to use**.
  - Unarmed strike is two size categories smaller than the wielder.

### Weapon Qualities (Table 7-4 columns)

- **Cost:** in mk or sp; includes scabbard/quiver/etc.
- **Damage:** § = subdual, dual entries (e.g. 1d6/1d6) = double weapon (use second figure for the off-hand attack).
- **Critical:** x2/x3/x4 multiplier; 19-20/x2 means threat range 19-20.
- **Range Increment:** -2 cumulative attack penalty per increment past the first. Thrown max 5 increments; projectile max 10 increments.
- **Weight:** lbs.
- **Type:** Bludgeoning / Piercing / Slashing (some weapons are dual-typed).
- **Special:** reach, double weapon, set-vs-charge, etc.

### Weapon Table (Table 7-4)

#### Simple — Melee

| Weapon | Cost | Dmg | Crit | Rng | Wt | Type |
|--------|------|-----|------|-----|-----|------|
| Gauntlet | 2 mk | as unarmed | as unarmed | — | 2 | B |
| Strike, unarmed (Medium) | — | 1d3§ | x2 | — | — | B |
| **Tiny** | | | | | | |
| Dagger (thrown 10 ft) | 2 mk | 1d4 | 19-20/x2 | 10 ft | 1 | P |
| **Small** | | | | | | |
| Mace, light | 5 mk | 1d6 | x2 | — | 6 | B |
| Sickle | 6 mk | 1d6 | x2 | — | 3 | S |
| **Medium** | | | | | | |
| Club (thrown) | — | 1d6 | x2 | 10 ft | 3 | B |
| Spear, Aiel (thrown) | 5 mk | 1d6 | x3 | 20 ft | 3 | P |
| Mace, heavy | 12 mk | 1d8 | x2 | — | 12 | B |
| Morningstar | 8 mk | 1d8 | x2 | — | 8 | B+P |
| **Large** | | | | | | |
| Quarterstaff (double) | — | 1d6/1d6 | x2 | — | 4 | B |
| Spear, Seanchan (thrown) | 10 mk | 1d8 | x3 | 20 ft | 5 | P |

#### Simple — Ranged

| Weapon | Cost | Dmg | Crit | Rng | Wt | Type |
|--------|------|-----|------|-----|-----|------|
| Crossbow, light | 35 mk | 1d8 | 19-20/x2 | 80 ft | 6 | P |
| Bolts (10) | 1 mk | — | — | — | 1 | — |
| Sling | — | 1d4 | x2 | 50 ft | 0 | B |
| Bullets, sling (10) | 1 sp | — | — | — | 5 | — |
| Crossbow, heavy | 50 mk | 1d10 | 19-20/x2 | 120 ft | 9 | P |
| Bolts (10) | 1 mk | — | — | — | 1 | — |

#### Martial — Melee

| Weapon | Cost | Dmg | Crit | Rng | Wt | Type |
|--------|------|-----|------|-----|-----|------|
| **Small** | | | | | | |
| Handaxe | 6 mk | 1d6 | x3 | — | 5 | S |
| Lance, light (set vs charge) | 6 mk | 1d6 | x3 | — | 5 | P |
| Sap | 1 mk | 1d6§ | x2 | — | 3 | B |
| Sword, short | 10 mk | 1d6 | 19-20/x2 | — | 3 | P |
| **Medium** | | | | | | |
| Battleaxe | 10 mk | 1d8 | x3 | — | 7 | S |
| Lance, heavy (set, reach) | 10 mk | 1d8 | x3 | — | 10 | P |
| Longsword | 15 mk | 1d8 | 19-20/x2 | — | 4 | S |
| Rapier (Weapon Finesse) | 20 mk | 1d6 | 18-20/x2 | — | 3 | P |
| Scimitar, Seanchan | 15 mk | 1d6 | 18-20/x2 | — | 4 | S |
| Trident (thrown) | 15 mk | 1d8 | x2 | 10 ft | 5 | P |
| Hammer, smith's | 12 mk | 1d8 | x3 | — | 8 | B |
| **Large** | | | | | | |
| Axe, hafted | 20 mk | 1d12 | x3 | — | 20 | S |
| Bill (reach, trip-set) | 9 mk | 2d4 | x3 | — | 15 | S |
| Greatclub | 5 mk | 1d10 | x2 | — | 10 | B |
| Pike (reach, set) | 5 mk | 1d8 | x3 | — | 9 | P |
| Poleaxe (set, trip) | 10 mk | 1d10 | x3 | — | 15 | P+S |
| Scythe | 18 mk | 2d4 | x4 | — | 12 | P+S |
| Boarspear (reach, set, +2 disarm) | 10 mk | 2d4 | x3 | — | 15 | P |

#### Martial — Ranged

| Weapon | Cost | Dmg | Crit | Rng | Wt | Type |
|--------|------|-----|------|-----|-----|------|
| Shortbow | 30 mk | 1d6 | x3 | 60 ft | 2 | P |
| Arrows (20) | 1 mk | — | — | — | 3 | — |
| Shortbow, Aiel (recurve) | 75 mk | 1d6 | x3 | 70 ft | 2 | P |
| Arrows (20) | 1 mk | — | — | — | 3 | — |
| Longbow | 75 mk | 1d8 | x3 | 100 ft | 3 | P |
| Arrows (20) | 1 mk | — | — | — | 3 | — |
| Longbow, Two Rivers | 100 mk | 1d8 | x3 | 110 ft | 3 | P |
| Arrows (20) | 1 mk | — | — | — | 3 | — |

#### Exotic — Melee

| Weapon | Cost | Dmg | Crit | Rng | Wt | Type |
|--------|------|-----|------|-----|-----|------|
| Ashandarei (double, special crafting) | 80 mk | 1d6/1d8 | 19-20/x2 | — | 15 | B+S |
| Sword, Warder's (hand-and-a-half) | 35 mk | 1d10 | 19-20/x2 | — | 10 | S |
| Swordbreaker (+3 disarm; 2d6 vs swords) | 25 mk | 1d6 | 19-20/x2 | — | 3 | P |
| Scythesword, Trolloc (Str 16+; -2 Rep) | 75 mk | 2d4 | 18-20/x2 | — | 16 | S |

#### Exotic — Ranged

| Weapon | Cost | Dmg | Crit | Rng | Wt | Type |
|--------|------|-----|------|-----|-----|------|
| Whip | 1 mk | 1d2§ | x2 | 15 ft (no penalty) | 2 | S |
| Net | 20 mk | special | special | 10 ft | 10 | special |

`§` = subdual damage. Trolloc scythesword and ashandarei are not openly sold.

### Selected Weapon Notes

- **Arrows / Bolts as melee weapons:** Tiny, 1d4 P, x2 crit; non-proficient (-4); 50% chance to break or be lost on miss.
- **Bullets, Sling:** 10 per pouch; bullet hits → destroyed; misses → 50% lost/destroyed.
- **Ashandarei:** double weapon. Two-weapon penalties as one-handed + light. Large creature using it one-handed cannot use it as a double.
- **Crossbow, Heavy:** 2-handed; load = full-round (provokes AoO); shoot 1-handed at -4; dual-wield -6/-10 (Two-Weapon Fighting does not apply, Ambidexterity removes off-hand -4 → -6/-6).
- **Crossbow, Light:** 2-handed; load = move action (provokes AoO); shoot 1-handed at -4.
- **Dagger:** Weapon Finesse-eligible.
- **Gauntlet:** lets unarmed strikes deal normal damage; included with most medium/heavy armors except breastplate.
- **Lance:** double damage on charge from a mount; heavy lance has reach.
- **Net:** ranged touch attack, max 10 ft, no range penalty. Hit → entangled (-2 attack, -4 effective Dex, half speed, no charge/run; Concentration DC 15 to cast a weave). Escape Artist DC 20 (full-round) or burst DC 25 (5 HP). Net is only useful vs. Tiny–Large. After unfolding, attacks at -4 until refolded (2 rounds proficient, 4 non).
- **Pike / Bill / Poleaxe:** reach (10 ft only). Bill and Poleaxe also let user trip and drop weapon to avoid being tripped on a fail.
- **Boarspear:** +2 to disarm rolls; reach.
- **Quarterstaff:** double weapon; same constraints as ashandarei for big creatures.
- **Rapier / Whip / Dagger / Unarmed strike:** Weapon Finesse-eligible.
- **Scythesword, Trolloc:** Str 16+ for proficiency. Open carry by non-Trolloc → -2 Reputation.
- **Sling:** ordinary stones do 1d3 and -1 to attack.
- **Spear, Aiel:** Medium-sized but usable by Small chars without difficulty.
- **Spear, Boar / Seanchan / etc.:** see notes above.
- **Sword, Warder's:** exotic 1-handed; martial when 2-handed for Medium creatures (or 1-handed for Large).
- **Swordbreaker:** breaks weapons on hit (2d6 vs weapon). May be used in off-hand to disarm without two-weapon penalty (gives +3 to disarm); held shield-like.
- **Whip:** deals subdual; ineffective vs. anything with armor +1 or natural armor +3. Treat as projectile, 15 ft no-penalty range. Trip-eligible; +2 to disarm.
- **Improvised Thrown Weapons:** range 10 ft, non-proficient (-4); GM adjudicates damage.

## Armor

### Armor Qualities

- **Cost / Armor Bonus / Max Dex / Check Penalty / Speed / Weight** per Table 7-5. Armor and shield bonuses stack with each other but not with effects that add to the same `armor bonus`.
- Heavy and certain medium armors limit Dex bonus to Defense.
- Even with Dex bonus reduced to 0, you do **not** count as having lost Dex bonus.
- Shields do not affect Max Dex or speed; their armor check penalty stacks with body armor.
- Wearing non-proficient armor: armor check penalty applies to attack rolls and movement skills (incl. Ride).
- **Sleeping in armor** with check penalty -5 or worse → fatigued next day (-2 Str/Dex, no charge/run).

### Armor Table (Table 7-5, Medium-size)

| Armor | Cost | Bonus | Max Dex | Check | Speed | Weight |
|-------|------|-------|---------|-------|-------|--------|
| **Light** | | | | | | |
| Padded | 5 mk | +1 | +8 | 0 | 30 | 10 |
| Leather | 10 mk | +2 | +6 | 0 | 30 | 15 |
| Studded leather | 25 mk | +3 | +5 | -1 | 30 | 20 |
| Mail shirt | 10 gc | +4 | +4 | -2 | 30 | 25 |
| **Medium** | | | | | | |
| Hide | 15 mk | +3 | +4 | -3 | 20 | 25 |
| Brigandine shirt | 5 gc | +4 | +3 | -4 | 20 | 30 |
| Full mail | 15 gc | +5 | +2 | -5 | 20 | 40 |
| Breastplate | 20 gc | +5 | +3 | -4 | 20 | 30 |
| Lacquered plate (Seanchan, +1 Rep) | 25 gc | +5 | +3 | -3 | 20 | 35 |
| **Heavy** (run x3 instead of x4) | | | | | | |
| Full brigandine | 20 gc | +6 | +0 | -7 | 20 | 45 |
| Banded mail | 25 gc | +6 | +1 | -6 | 20 | 35 |
| Plate-and-mail | 60 gc | +7 | +0 | -7 | 20 | 50 |
| Full plate | 150 gc | +8 | +1 | -6 | 20 | 50 |
| **Shields** | | | | | | |
| Buckler, Aiel | 25 mk | +1 | — | 0 | — | 2 |
| Shield, small wood | 3 mk | +1 | — | -1 | — | 5 |
| Shield, small steel | 9 mk | +1 | — | -1 | — | 6 |
| Shield, large wood | 7 mk | +2 | — | -2 | — | 10 |
| Shield, large steel | 20 mk | +2 | — | -2 | — | 15 |

### Donning Armor (Table 7-6)

| Armor Type | Don | Don Hastily | Remove |
|------------|-----|------------|--------|
| Padded, leather, hide, studded leather, chain shirt | 1 min | 5 rounds | 1 min* |
| Breastplate, scale mail, full mail, banded mail, splint mail | 4 min* | 1 min | 1 min* |
| Half-plate or full plate | 4 min** | 4 min* | 1d4+1 min* |

\* Halved with help (a single helper, can serve up to two adjacent characters; helpers cannot mutually assist simultaneously).
\** Requires help; without help can only be donned hastily.

Hastily donned armor: armor bonus and armor check penalty each 1 worse than normal.

### Armor for Unusual Creatures

| Size | Humanoid Price / Weight | Nonhumanoid Price / Weight |
|------|-------------------------|----------------------------|
| Up to Tiny* | x½ / x1/10 | x1 / x1/10 |
| Small | x1 / x½ | x2 / x½ |
| Medium | x1 / x1 | x2 / x1 |
| Large | x2 / x2 | x4 / x2 |
| Huge | x4 / x5 | x8 / x5 |

\* Divide armor bonus by 2.
Gargantuan and Colossal armor is bespoke; no standard price/weight.

### Shield Bash

Shield used as off-hand weapon: Medium char does 1d4 (large) or 1d3 (small), x2 crit; Small char does 1d3 / 1d2. Tower shields cannot bash. Treated as a martial bludgeoning weapon, light for two-weapon penalty purposes. Loses Defense bonus until next action when used to attack.

### Buckler

Strap-on. -1 attack penalty if you also wield an off-hand weapon (stacks with two-weapon penalties); using an off-hand weapon forfeits buckler Defense bonus that round. Cannot bash.

## Goods and Services

### Adventuring Gear (Table 7-7 highlights)

| Item | Cost | Wt |
|------|------|----|
| Backpack (empty) | 2 mk | 2 |
| Barrel (empty) | 2 mk | 30 |
| Basket (empty) | 4 sp | 1 |
| Bedroll | 1 sp | 5 |
| Bell | 1 mk | * |
| Blanket, winter | 5 sp | 3 |
| Block and tackle | 5 mk | 5 |
| Bottle, wine, glass | 2 mk | * |
| Bucket (empty) | 5 sp | 2 |
| Caltrops (5 ft sq, +0 atk vs steppers) | 1 mk | 2 |
| Candle | 1 cp | * |
| Canvas (sq yd) | 1 sp | 1 |
| Case, map or scroll | 1 mk | ½ |
| Chain (10 ft, hardness 10, 5 HP, burst DC 26) | 30 mk | 2 |
| Chalk | 1 cp | * |
| Chest (empty) | 2 mk | 25 |
| Crowbar | 2 mk | 5 |
| Dice (5) | 5 mk | * |
| Firewood (per day) | 1 cp | 20 |
| Fishhook | 1 sp | * |
| Fishing net (25 sq ft) | 4 mk | 5 |
| Flask | 3 cp | * |
| Flint and steel | 1 mk | * |
| Grappling hook | 1 mk | 4 |
| Hammer | 5 sp | 2 |
| Ink (1 oz vial) | 8 mk | * |
| Inkpen | 1 sp | * |
| Jug, clay (1 gal) | 3 cp | 9 |
| Ladder, 10-ft | 5 cp | 20 |
| Lamp, common (15 ft radius, 6 hr/pt oil) | 1 sp | 1 |
| Lantern, hooded (30 ft radius, 6 hr/pt oil) | 7 mk | 2 |
| Lock, very simple (Open Lock DC 20) | 2 gc | 1 |
| Lock, average (DC 25) | 4 gc | 1 |
| Lock, good (DC 30) | 8 gc | 1 |
| Lock, amazing (DC 40) | 15 gc | 1 |
| Looking glass | 100 gc | 1 |
| Manacles (Escape Artist DC 30, break DC 26, 10 HP, hardness 10) | 15 mk | 2 |
| Manacles, masterwork (DC 35 / DC 28) | 5 gc | 2 |
| Mirror, small steel | 10 mk | ½ |
| Mug/tankard, clay | 2 cp | 1 |
| Oil (1 pt flask; grenade) | 3 mk | 1 |
| Paper (sheet) | 4 sp | * |
| Parchment (sheet) | 2 sp | * |
| Pick, miner's | 3 mk | 10 |
| Pitcher, clay | 2 cp | 5 |
| Piton | 1 sp | ½ |
| Playing cards | 10 mk | ¼ |
| Pole, 10-ft | 2 sp | 8 |
| Pot, iron | 5 sp | 10 |
| Pouch, belt | 1 mk | 1 |
| Ram, portable (+2 Str break, helper +2) | 10 mk | 20 |
| Rations, trail (per day) | 5 sp | 1 |
| Rope, hemp (50 ft; 2 HP, burst DC 23) | 1 mk | 10 |
| Rope, silk (50 ft; 4 HP, burst DC 24, +2 Use Rope) | 10 mk | 5 |
| Sack (empty) | 1 sp | ½ |
| Sealing wax | 1 mk | 1 |
| Sewing needle | 5 sp | * |
| Signal whistle | 8 sp | * |
| Signet ring | 5 mk | * |
| Sledge | 1 mk | 10 |
| Soap (per lb) | 5 sp | 1 |
| Spade or shovel | 2 mk | 8 |
| Tent (sleeps 2) | 10 mk | 20 |
| Torch (20 ft radius, 1 hr) | 1 cp | 1 |
| Vial (ink/potion, 1 oz) | 1 mk | * |
| Waterskin | 1 mk | 4 |
| Whetstone | 2 cp | 1 |

\* No weight worth noting.

#### Caltrops

- Each 2-lb bag covers 5 ft sq.
- On entry / round of fighting: caltrops attack at +0 vs. each creature in the area; armor, shield, deflection do **not** count; footwear gives +2 armor bonus.
- Hit: 1 HP damage; speed halved until 1 day passes, Heal DC 15 succeeds, or 1 HP of One Power healing applied.
- Charging or running creature must stop on a hit. Half-speed creatures pass through safely.

#### Oil (as grenade)

- Full-round to fuse a flask; 50% chance to ignite.
- Round after a direct hit: target takes another 1d6 damage. Full-round to extinguish (Reflex DC 15; +2 if rolling on ground; auto-success in water or via One Power).
- Pouring 1 pt = 5 ft sq. Lit floor oil: 2 rounds, 1d3 damage to creatures in area.

### Class Tools and Skill Kits

| Item | Cost | Wt | Notes |
|------|------|----|------|
| Artisan's tools | 5 mk | 5 | Required to avoid -2 improvised penalty on Craft |
| Artisan's tools, masterwork | 55 mk | 5 | +2 Craft |
| Climber's kit | 80 mk | 5 | +2 Climb |
| Disguise kit | 50 mk | 8 | +2 Disguise; 10 uses |
| Healer's kit | 50 mk | 1 | +2 Heal; 10 uses; includes 2 doses healer's balm |
| Hourglass | 25 mk | 1 | |
| Magnifying glass | 10 gc | * | +2 Appraise (small/detailed) |
| Musical instrument, common | 5 mk | 3 | |
| Musical instrument, masterwork | 10 gc | 3 | +2 Perform |
| Scale, merchant's | 2 mk | 1 | +2 Appraise (by-weight) |
| Thieves' tools | 30 mk | 1 | Avoid -2 improvised penalty on Disable Device / Open Lock |
| Thieves' tools, masterwork | 10 gc | 2 | +2 Disable Device / Open Lock |

### Clothing

| Outfit | Cost | Wt |
|--------|------|----|
| Artisan's | 1 mk | 4 |
| Cold weather (+5 Fort vs cold) | 8 mk | 7 |
| Courtier's | 30 mk | 6 |
| Gleeman's | 3 mk | 4 |
| Explorer's | 10 mk | 8 |
| Cadin'sor (Aiel; rarely sold outside) | 8 mk | 2 |
| Noble's | 8 gc | 10 |
| Peasant's | 1 sp | 2 |
| Royal | 20 gc | 15 |
| Scholar's | 5 mk | 6 |
| Traveler's | 1 mk | 5 |

The first outfit is free at character creation and does not count toward carry weight.

### Food, Drink, Lodging

| Item | Cost | Wt |
|------|------|----|
| Ale, gallon | 2 sp | 8 |
| Ale, mug | 4 cp | 1 |
| Banquet (per person) | 10 mk | — |
| Bread, per loaf | 2 cp | ½ |
| Cheese, hunk | 1 sp | ½ |
| Inn — good (per day) | 2 mk | — |
| Inn — common | 5 sp | — |
| Inn — poor | 2 sp | — |
| Meals — good (per day) | 5 sp | — |
| Meals — common | 3 sp | — |
| Meals — poor | 1 sp | — |
| Meat, chunk | 3 sp | ½ |
| Oosquai (Aiel corn liquor), jug | 5 mk | 4 |
| Rations, trail (per day) | 5 sp | 1 |
| Wine, common pitcher | 2 sp | 6 |
| Wine, fine bottle | 10 mk | 1½ |

### Mounts and Related Gear

| Item | Cost | Wt |
|------|------|----|
| Barding (Medium creature) | x2 cost / x1 wt | — |
| Barding (Large creature) | x4 cost / x2 wt | — |
| Bit and bridle | 2 mk | 1 |
| Cart | 15 mk | 200 |
| Donkey or mule | 8 mk | — |
| Feed (per day) | 5 cp | 10 |
| Horse, heavy | 20 mk | — |
| Horse, light | 10 mk | — |
| Pony | 5 mk | — |
| Warhorse, heavy | 40 gc | — |
| Warhorse, light | 15 gc | — |
| Saddle, military | 20 mk | 30 |
| Saddle, pack | 5 mk | 15 |
| Saddle, riding | 10 mk | 25 |
| Saddle, exotic — military | 60 mk | 40 |
| Saddle, exotic — pack | 15 mk | 20 |
| Saddle, exotic — riding | 30 mk | 30 |
| Saddlebags | 4 mk | 8 |
| Sled | 20 mk | 300 |
| Stabling (per day) | 5 sp | — |
| Wagon | 35 mk | 400 |

#### Barding Speed Penalty

| Barding | Mount speed 40 / 50 / 60 |
|---------|--------------------------|
| Medium | 30 / 35 / 40 |
| Heavy | 30* / 35* / 40* (run x3 only) |

Heavy barding disables flight on flying mounts. Donning/removing barding takes 5x the human Table 7-6 time. Barded animals carry only the rider plus normal saddlebags, so warriors typically lead a second mount for cargo.

#### Mount Notes

- Warhorses can be ridden into combat normally; light/heavy/pony are hard to control (Ride checks).
- Donkeys/mules are hardy and willing to enter dangerous places; horses are not.
- Riding mounts must be fed at least some meat; cost varies.

### Containers and Carriers (Table 7-8)

Hauling vehicles (cost / empty wt / capacity):

| Item | Cost | Wt | Carries |
|------|------|----|---------|
| Cart | 15 mk | 200 | ½ ton |
| Sled | 20 mk | 300 | 1 ton |
| Wagon | 35 mk | 400 | 2 tons |

Dry goods containers:

| Item | Cost | Wt | Carries |
|------|------|----|---------|
| Backpack | 2 mk | 2† | 1 cu ft |
| Barrel | 2 mk | 30 | 10 cu ft |
| Basket | 4 sp | 1 | 2 cu ft |
| Bucket | 5 sp | 2 | 1 cu ft |
| Chest | 2 mk | 25 | 2 cu ft |
| Pouch, belt | 1 mk | ½† | 1/5 cu ft |
| Sack | 1 sp | ½† | 1 cu ft |
| Saddlebags | 4 mk | 8 | 5 cu ft |

Liquids:

| Item | Carries |
|------|---------|
| Bottle, wine, glass | 1½ pint |
| Flask | 1 pint |
| Jug, clay | 1 gallon |
| Mug/tankard, clay | 1 pint |
| Pitcher, clay | ½ gallon |
| Pot, iron | 1 gallon |
| Vial, ink or potion | 1 oz |
| Waterskin | ½ gallon |

† Item weighs ¼ this amount and carries ¼ when made for Small chars.

## Special and Superior Items

### Pricing (Table 7-9)

| Item | Cost |
|------|------|
| +1 Power-wrought blade | +200 gc * |
| +2 Power-wrought blade | +800 gc * |
| +3 Power-wrought blade | +1,800 gc * |
| Weapon, masterwork | +300 mk * |
| Weapon, masterpiece | +600 mk * |
| Arrow / bolt / bullet, masterwork | 7 mk |
| Armor or shield, masterwork | +150 mk * |
| Armor or shield, masterpiece | +300 mk * |
| Tool, masterwork | +50 mk * |
| Acid (flask) | 25 mk (1 lb) |
| Illuminator's flare | 5 mk (½ lb) |
| Antitoxin (vial) | 5 gc |
| Healer's balm | 15 mk (¼ lb) |
| Warder's cloak | 1,000 gc (1 lb) |

\* Plus base item cost. Masterwork double weapons cost +600 mk. If the total exceeds 300 gc, the item is generally unavailable on the open market.

### Item Effects

- **Weapon, masterwork** — +1 attack; visible carry → +1 Reputation.
- **Weapon, masterpiece** — +2 attack; visible carry → +2 Reputation.
- **Arrow/Bolt/Bullet, masterwork** — +1 attack; stacks with masterwork bow/crossbow/sling; destroyed on use.
- **Armor or Shield, masterwork** — armor check penalty -1 better; +1 Reputation while worn.
- **Armor or Shield, masterpiece** — armor check penalty -2 better; +2 Reputation while worn.
- **Tool, masterwork** — +2 to relevant skill check; multiple masterwork tools toward the same check do not stack.
- **Acid (flask)** — grenadelike, 1d6 direct + 1 pt splash, 10-ft increment.
- **Antitoxin** — drink for +5 Fort vs. poison for 1 hour.
- **Healer's Balm** — application = 1 full round; converts 1d4 HP of damage to subdual (heals at subdual rate). 1/hour cap. Also stabilizes a character at negative HP (1/day per character). Heal-skilled user can add HP recovery to either use.
- **Illuminator's Flare / Rocket** — see grenadelike rules + rocket-stack notes below.
- **Power-wrought Blade** — unbreakable, never needs sharpening; +bonus to all attack and damage rolls. Heron-marked (often, not always). No open market.
- **Warder's Cloak** — fancloth: -10 to Spot checks vs. wearer; +2 circumstance bonus to Defense. Property of the White Tower; not legitimately available; possessing one risks Aes Sedai retaliation.

### Grenadelike Weapons (Table 7-10)

| Weapon | Cost | Direct | Splash | Range | Wt |
|--------|------|--------|--------|-------|----|
| Acid (flask) | 25 mk | 1d6 | 1 pt** | 10 ft | 1 |
| Oil (flask) | 3 mk | 1d6 | 1 pt** | 10 ft | 1 |
| Illuminator's rocket | 100 mk | 2d6 | 1d6** | 40 ft | 2 |

\** Splash hits all creatures within 5 ft of landing point. No proficiency required unless the weapon's description says otherwise.

#### Illuminator's Rocket

- As a ranged weapon: -4 to attack unless you have Exotic Weapon Proficiency (fireworks); requires flint and steel or open flame.
- Demolition: full-round to plant 1 rocket (up to 10 stacked). Ignition = attack action; explosion at start of next turn (move action to retreat). Direct damage doubled on stacked use; splash radius +5 ft per added rocket. With ≥5 ranks Knowledge (architecture & engineering) and a man-made target, damage tripled instead of doubled. Renegade Illuminators only — Guild retaliation risk.

## Implementation Notes (WheelMUD)

- **Currency model:** existing `currency` work already supports cp/sp/mk/gc; persist costs internally as cp and convert at display time. Half-price selling is a single multiplier; commodities (Table 7-3) need a "trade-good" flag exempting them from the rule.
- **Item taxonomy:** model `Weapon`, `Armor`, `Shield`, `Container`, `Tool`, `Consumable`, and `Mount` as distinct types; weapons need fields for proficiency tier, size, melee/ranged, range increment, double-weapon flag, special tags (reach, set-vs-charge, trip, disarm-bonus, finesse-eligible), and damage type bitmask (B/P/S).
- **Wielding rules:** compute light/one-handed/two-handed per (weapon size − wielder size) lookup. Strength bonus to damage handled in the attack pipeline (1.5× for two-handed, none for projectile, full for thrown).
- **Critical / threat:** store threat range (low–20) and multiplier separately; bonus dice (sneak, flame) bypass multiplication.
- **Armor stack:** per-character cache of `(armorBonus, shieldBonus, maxDex, checkPenalty, speed, runMult)` recomputed on equip/unequip. Layer with encumbrance from `heroic-characteristics.md` — take the worse of the two for each cap.
- **Don timers:** events scheduled on the tick scheduler; helper-assist halves time (cap one helper for two adjacent characters).
- **Special weapons:** Net, Whip, Swordbreaker, Boarspear, Trolloc scythesword, Lance, Crossbows all need bespoke handlers — codify as weapon "tags" the combat pipeline can switch on.
- **Light sources:** torches/lamps/lanterns are radius + duration timers; tie into room-level visibility checks if/when implemented.
- **Locks/manacles/chain:** all expose hardness, HP, and burst DC; implement as a generic "breakable" interface so Open Lock and break-DC code reuse it.
- **Caltrops & oil pools:** room hazards with on-enter triggers; persist over reboots when scattered/poured.
- **Mounts:** treat as a separate `Mount` entity owning encumbrance + barding + saddle slots; integrate with movement system (`heroic-characteristics.md`).
- **Reputation hooks:** masterwork/masterpiece weapons & armor, lacquered plate, and Trolloc scythesword carry passive Reputation deltas while equipped/visible. Implement as tag-driven modifiers on the Reputation calculation.
- **Power-wrought / Warder's cloak / fireworks:** gate behind an item-rarity flag and admin spawn rather than open commerce. Power-wrought blades should be unbreakable (skip durability checks).
- **Starting equipment:** at character creation roll Table 7-1 by class, grant one free outfit, and merge with background-derived gear (Chapter 2). Carry weight excludes the starting outfit.
- **Grenadelike attacks:** unify acid / oil / fireworks under a `GrenadeLikeWeapon` type with direct/splash damage, range, and ignition state. Rocket-stack demolition multiplier is a function of `count`, with a Knowledge (arch/eng) gate for x3 vs. man-made structures.
