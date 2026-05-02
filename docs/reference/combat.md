# Combat

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 8: Combat, pp. 130–153) for use in WheelMUD implementation.

## Overview

Combat resolves around six-second rounds, an initiative order, and four action types (attack, move, full-round, free). Each combatant gets one attack action and one move action per round (in either order), or two move actions, or one full-round action — plus any free actions.

## Combat Sequence

1. Every combatant starts **flat-footed**.
2. GM determines awareness. If awareness is asymmetric, run a **surprise round**.
3. Aware combatants roll initiative and take **one** action (attack or move, not both) in initiative order.
4. Unaware combatants do not act and remain flat-footed.
5. After the surprise round, anyone who hasn't rolled initiative does so. First regular round begins.
6. Rounds repeat in the same initiative order until combat ends.

A round (`6 seconds`) is treated like a calendar interval: timed effects expire just before the same initiative count in the future round.

### Initiative

- **Check:** `1d20 + Dex modifier` (+ Improved Initiative if any).
- Ties: higher Dex first; otherwise reroll.
- GM usually rolls a single initiative for an opposing group, optionally splitting groups.
- Joining mid-fight: roll initiative on entry; act when your turn comes around.

### Surprise

- **Surprised side:** Aware of nothing — does not act, remains flat-footed.
- **Aware side:** Acts in surprise round (one action), then loses flat-footed status.
- If everyone or no one is surprised, no surprise round.

### Flat-Footed

Before your first action: cannot apply Dex bonus to Defense (still keep class bonus). Some abilities (algai'd'siswai uncanny dodge) bypass this.

## Combat Statistics

### Attack Roll

```
Melee:  1d20 + base attack bonus + Str mod + size mod
Ranged: 1d20 + base attack bonus + Dex mod + size mod + range penalty
```

- Hit if result ≥ Defense.
- **Natural 1** = automatic miss. **Natural 20** = automatic hit and a *threat*.
- **Range increment:** -2 cumulative per increment past the first. Thrown max 5 increments; projectile max 10.

### Defense

```
Defense = 10 + class bonus (or equipment bonus) + Dex mod + size mod
```

- **Class bonus** vs. **equipment bonus** (armor + shield) — take the **higher**, not both.
- Class bonus applies even when flat-footed.
- Dex bonus to Defense lost when flat-footed, climbing without guard, helpless, etc.
- **Touch attacks** ignore equipment bonus and natural armor (Dex and size still count).
- **Dodge bonuses** (e.g. Dodge feat, fighting defensively, total defense, Mobility) stack with each other; lost whenever Dex bonus to Defense is lost. Wearing armor does not cap dodge bonuses the way it caps Dex.

### Damage

- Roll the weapon's damage die plus modifiers.
- **Strength** to damage:
  - Melee / thrown: full Str modifier.
  - Two-handed melee: 1.5× Str (positive only); light weapons used two-handed do not get the 1.5×.
  - Off-hand: ½ Str (positive only).
  - Bow / sling: only Str penalty applies (no bonus).
  - Crossbow: no Str modifier at all.
- **Minimum:** any successful hit deals at least 1 HP damage.
- **Critical hits:** roll a second attack with the same modifiers; if it also hits, multiply damage by the weapon's crit multiplier (default x2). Bonus dice (sneak attack, etc.) **do not** multiply.
- Threat range expansions (e.g. 19-20) only become crits if the threat-range roll itself would have hit normally; only natural 20 is an auto-hit.
- Some weaves can crit (those that require an attack roll); area weaves cannot.

### Size Modifier (attack and Defense)

| Size | Modifier |
|------|----------|
| Colossal | -8 |
| Gargantuan | -4 |
| Huge | -2 |
| Large | -1 |
| Medium | +0 |
| Small | +1 |
| Tiny | +2 |
| Diminutive | +4 |
| Fine | +8 |

### Saving Throws

`1d20 + base save bonus + ability modifier`

| Save | Ability | Used vs. |
|------|---------|----------|
| Fortitude | Con | Poison, disease, paralysis, instant death |
| Reflex | Dex | Fireball, traps, falling |
| Will | Wis | Compulsion, illusion, mental influence |

### Hit Points

| HP | State |
|----|-------|
| > 0 | OK |
| 0 | **Disabled** — one move OR attack action/round; another HP lost after any strenuous action |
| -1 to -9 | **Dying** — unconscious; lose 1 HP/round; 10% chance per round to stabilize |
| -10 or below | **Dead** |

- **Massive damage:** any single hit dealing ≥ 50 HP damage forces a Fort DC 15 or die outright.
- **Stabilizing:** Heal DC 15, or 1 HP healed by the One Power, halts the bleed-out.

### Recovery

- **With help (tended):** Each hour after stabilization, 10% to wake (then disabled). Hit points recover naturally even while unconscious.
- **Without help, stable:** Each hour, 10% to wake. Each hour you don't wake costs 1 HP. No natural healing until awake. Once natural healing starts, no more HP loss.

### Natural Healing

- **Light rest:** 1 HP per character level per day. No combat or channeling.
- **Bed rest (full day):** 1.5× character level HP per day.
- **Ability damage:** 1 point/day rest, 2/day bed rest.
- **Cap:** never exceed normal HP total.

### Subdual Damage

- Tracked separately from real HP. Does **not** subtract from HP.
- **Staggered:** subdual damage *equals* current HP — one move OR attack action per round only.
- **Unconscious:** subdual *exceeds* current HP. While unconscious you are helpless. Each minute, 10% chance to wake (still staggered).
- **Heals:** 1 HP/hr per character level.
- **Crossover:** healing weaves convert real damage into subdual damage at a 1:1 rate; subdual damage cannot be regenerated as real HP.
- A normal-damage weapon used to deal subdual takes a **-4** attack penalty; a subdual weapon used for normal damage takes the same -4.
- Objects are immune to subdual damage.

### Temporary Hit Points

- Stacked separately. Lost first when damage is taken.
- Can drop you below current HP without becoming "real" damage.
- Cannot be restored once removed.
- Constitution-score increases give regular HP, **not** temporary HP.

## Action Types (Table 8-1)

### Attack Actions

| Action | AoO? | Movement |
|--------|------|----------|
| Attack (melee) | No | — |
| Attack (ranged) | Yes | — |
| Attack (unarmed) | Maybe | — |
| Cast 1-action weave | Yes | — |
| Feint (Bluff) | No | — |
| Heal a friend | Yes | — |
| Strike an object | Yes | — |
| Total defense | No | — |
| Use a 1-action skill | Maybe | — |

### Move Actions (`* = 5-foot step allowed`)

| Action | AoO? | Movement |
|--------|------|----------|
| Move | No | x1 speed |
| Climb | No | ¼ speed |
| Draw / sheath weapon† | No | * |
| Extinguish flames | No | * |
| Light a torch | Yes | * |
| Open a door | No | * |
| Pick up an item | Yes | * |
| Retrieve a stored item | Yes | * |
| Move a heavy object | Yes | x1 speed |
| Stand up | No | * |
| Load a weapon | Yes | * |
| Use a full-round skill | Maybe | * |

† Reducible to a free action with the right feat.

### Full-Round Actions

| Action | AoO? | Movement |
|--------|------|----------|
| Charge | No | 2x speed |
| Coup de grace | No | 5-ft step |
| Full attack | No | 5-ft step |
| Run | Yes | 4x speed |
| Cast a full-round weave | Yes | 5-ft step |
| Concentrate to maintain a weave | No | 5-ft step |

### Free Actions

| Action | AoO? |
|--------|------|
| Activate an item | Yes |
| Drop an item | No |
| Drop to the floor | No |
| Ready (special) | No |
| Speak | No |

### Special Actions (offload to dedicated rules)

| Action | AoO? |
|--------|------|
| Bull rush (charge) | No |
| Disarm | Maybe |
| Grapple | Maybe |
| Trip | Maybe |
| Use skill or feat | Maybe |
| Miscellaneous | Maybe |

If your action involves no movement, you may take a **free 5-foot step** (cannot be combined with any actual movement that round).

## Attacks of Opportunity (AoO)

- You **threaten** the area within 5 ft (or 10 ft with a reach weapon).
- Three triggers:
  1. Moving **out** of a threatened area (5-ft step is exempt).
  2. Moving **within** a threatened area more than 5 ft.
  3. Taking a "distracting" action while threatened (ranged attack, weave casting, healing, etc.).
- An AoO is one melee attack at your **normal** attack bonus, max one per round (Combat Reflexes raises the per-round cap).
- Disengage (full-round): leave the threatened area in your first 5 ft of movement, then move up to 2x speed without provoking AoO from the foe(s) you disengaged from. Other foes can still AoO if you move through their threatened areas.

## Attack Actions in Detail

### Melee Attack

- 5-ft reach by default; reach weapons strike 10 ft only (cannot hit adjacent foes).
- AoO triggers above apply.

### Ranged Attack

- Line of sight required.
- **-4** to attack a target adjacent to one of your allies (engaged in melee), unless target is helpless or you have **Precise Shot**.
- Adjacent enemy → must move or melee instead.
- Improvised thrown items: -4 (non-proficient), 10 ft increment.

### Casting Weaves

- 1-action weave = attack action; full-round weave = full-round action; longer weaves require continuous full-round actions for their duration. The weave resolves just **before** your turn after the casting completes.
- Casting in a threatened area provokes AoO unless **casting on the defensive** (Concentration DC 15 + weave level; failure loses the weave).
- Distraction → Concentration check or lose weave (still counts vs. daily cap).
- **Touch weaves:** cast then touch (same round or later). Touching a friend or yourself is automatic; touching a foe is a melee touch attack. Counts as an "armed" attack — no AoO from delivering it.
- Maintaining a weave is a separate full-round action.

### Total Defense

Attack action: forfeit your attack to gain **+4 dodge** to Defense for one round.

### Fighting Defensively

- During a normal attack or full attack: **-4** to all attacks → **+2 dodge** to Defense for that round.

### Multiple Attacks

- Multiple attacks (high BAB, two weapons, double weapon) require a **full attack** (full-round). Only a 5-ft step is permitted.
- BAB iterative attacks resolve from highest bonus to lowest. After the first attack, you may swap remaining attacks for a move action.

### Two-Weapon Fighting Penalties (Table 8-2)

| Circumstance | Primary | Off |
|---|---|---|
| Normal | -6 | -10 |
| Off-hand light | -4 | -8 |
| Ambidexterity | -6 | -6 |
| Two-Weapon Fighting | -4 | -8 |
| Light + Ambidexterity | -4 | -4 |
| Light + Two-Weapon Fighting | -2 | -6 |
| Ambidexterity + Two-Weapon Fighting | -4 | -4 |
| Light + Ambidexterity + Two-Weapon Fighting | -2 | -2 |

Unarmed strikes always count as light. Double weapons are treated as light off-hand.

### Charge

- Full-round; minimum 10 ft, max 2x speed; straight line; must stop on reaching target.
- **+2** charge bonus to single attack; **-2** Defense penalty until your next action.
- Even with multiple attacks, charge yields exactly one strike.
- Set piercing weapons (`Ready vs. charge`) deal **double damage** if they hit a charging foe.

### Run

- Full-round; 4x speed (3x in heavy armor) in a straight line; lose Dex bonus to Defense.
- After Con-score-many rounds, Con check DC 10 each additional round (DC +1 each subsequent check). On failure, must rest 1 minute before running again. ~12 mph for unencumbered humans.

### Disengage

Full-round. First 5 ft must clear the threatened area; up to 2x speed total. Avoids AoO from those whose area you escaped — but other AoOs still trigger.

## Movement and Position

- **Scale:** 1 inch = 5 ft. Round = 6 seconds. Human-size = 5 ft square.
- Tactical speed by race/armor (Table 8-4):

| Race | Light/None | Medium/Heavy |
|------|-----------|--------------|
| Human | 30 ft | 20 ft |
| Ogier | 40 ft | 30 ft |

### Passing Through

- Friendly creature: free.
- Helpless/cowering enemy: free.
- Resisting enemy: must overrun (charge action).
- Tumble skill can pass through a threatened square.
- Fine/Diminutive/Tiny creatures may always enter occupied squares.
- Anyone may pass through a creature ≥ 3 size categories larger or smaller.

### Flanking

When you and an ally threaten a foe from directly opposite sides: **+2** to melee attack rolls. Wanderers may sneak attack a flanked target.

## Modifiers (Table 8-5)

| Circumstance | Melee | Ranged |
|--------------|-------|--------|
| Attacker flanking defender | +2 | — |
| Attacker on higher ground | +1 | +0 |
| Attacker prone | -4 | * |
| Attacker invisible | +2† | +2† |
| Defender sitting/kneeling | +2 | -2 |
| Defender prone | +4 | -4 |
| Defender stunned/cowering/off balance | +2† | +2† |
| Defender climbing (no shield) | +2† | +2† |
| Defender surprised or flat-footed | +0† | +0† |
| Defender running | +0† | -2 |
| Defender grappling (attacker not) | +0† | +0†† |
| Defender pinned | +4† | -4† |
| Defender has cover | (see Cover) | (see Cover) |
| Defender concealed/invisible | (see Concealment) | (see Concealment) |
| Defender helpless | (see Helpless) | (see Helpless) |

† Defender loses Dex bonus to Defense.
†† Random target among grapplers; defender loses Dex bonus.
\* Most ranged weapons cannot be used prone (crossbow can).

### Cover (Table 8-6)

| Cover | Example | Defense | Reflex |
|-------|---------|---------|--------|
| One-quarter | Behind 3-ft wall | +2 | +1 |
| One-half | Around corner / tree / open window / behind same-size creature | +4 | +2 |
| Three-quarters | Peering around corner / tree | +7 | +3 |
| Nine-tenths | Behind arrow slit / cracked door | +10 | +4* |
| Total | Solid wall between you | — | — |

\* Half damage on failed save; none on success (improved evasion).
Cover bonus does not stack with kneeling, etc. — take the better.

- Reach-weapon attacks pass through intervening creatures: same-size creature in the way grants +4 cover; if your shot strikes that creature it takes no damage (you hit with the haft).
- A miss that would have hit if not for cover may instead hit the cover; if a creature is the cover and the attack roll exceeds its Defense, it takes the damage (covering creature may forgo Dex/dodge to deliberately take the hit).

### Concealment (Table 8-7)

| Concealment | Example | Miss % |
|-------------|---------|--------|
| One-quarter | Light fog / moderate darkness / light foliage | 10% |
| One-half | Channeling effect / dense fog at 5 ft | 20% |
| Three-quarters | Dense foliage | 30% |
| Nine-tenths | Near total darkness | 40% |
| Total | Invisible / blind / total darkness / dense fog at 10 ft | 50% + must guess location |

Use the highest applicable miss chance — they do not stack.

### Helpless Defenders

- Dex score effectively 0; Dex modifier to Defense is **-5**.
- Melee attack vs. helpless: **+4** circumstance bonus (ranged: no special bonus).
- Wanderers can sneak attack helpless foes.
- **Coup de grace:** full-round, melee weapon (or bow/crossbow if adjacent). Auto-hit and auto-crit. If the target survives, Fort DC `10 + damage` or die. Cannot be used vs. crit-immune creatures.

## Special Initiative Actions

### Ready

- Attack action. Specify the action and trigger.
- Triggered action resolves **before** the triggering action.
- Your initiative becomes the count on which the readied action fired.
- If you don't fire it before your next regular turn, the readied action lapses (you may re-ready).
- "Ready vs. charge" with a piercing/set weapon: hit deals **double damage**.
- Ready against a channeler ("when she casts") to force a Concentration check on damage.

## Special Attacks and Damage

### Aid Another

Attack action; melee attack roll vs. Defense 10 to give an adjacent ally either **+2 attack** or **+2 Defense** vs. one specific opponent.

### Attack an Object (Tables 8-8, 8-9)

| Size | Defense Mod |
|------|-------------|
| Colossal | -8 |
| Gargantuan | -4 |
| Huge | -2 |
| Large | -1 |
| Medium | +0 |
| Small | +1 |
| Tiny | +2 |
| Diminutive | +4 |
| Fine | +8 |

- Inanimate immobile object Defense = `5 + size mod`.
- Melee vs. immobile: +4 to attack; full-round line-up = auto-hit (melee) or +5 (ranged). Objects are crit-immune and subdual-immune.
- Held/carried: object Defense = bearer's Defense + size mod (+5 if held in hand).
- Animated objects = creatures.
- Damage halved by ranged weapons and by fire/lightning/etc. before applying hardness.

#### Hardness & HP (Table 8-9)

| Substance | Hardness | HP |
|-----------|---------|----|
| Paper | 0 | 2 / inch |
| Rope | 0 | 2 / inch |
| Glass | 1 | 1 / inch |
| Ice | 0 | 3 / inch |
| Wood | 5 | 10 / inch |
| Stone | 8 | 15 / inch |
| Iron | 10 | 30 / inch |

| Object | Hardness | HP | Break DC |
|--------|----------|----|----------|
| Rope (1 in diam) | 0 | 2 | 23 |
| Simple wooden door | 5 | 10 | 13 |
| Spear | 5 | 2 | 14 |
| Small chest | 5 | 1 | 17 |
| Good wooden door | 5 | 15 | 18 |
| Treasure chest | 5 | 15 | 23 |
| Strong wooden door | 5 | 20 | 23 |
| Masonry wall (1 ft) | 8 | 90 | 35 |
| Hewn stone (3 ft) | 8 | 540 | 50 |
| Chain | 10 | 5 | 26 |
| Manacles | 10 | 10 | 26 |
| Masterwork manacles | 10 | 10 | 28 |
| Iron door (2 in) | 10 | 60 | 28 |

#### Break DCs (Table 8-10)

| Action | DC |
|--------|----|
| Break down simple door | 13 |
| Break down good door | 18 |
| Break down strong door | 23 |
| Burst rope bonds | 23 |
| Bend iron bars | 24 |
| Break down barred door | 25 |
| Burst chain bonds | 26 |
| Break down iron door | 28 |

If item is at half HP or less, break DC drops by 2.

#### Saving Throws

- Unattended: auto-fail.
- Attended: use the wielder's saves.
- *Ter'angreal/angreal/sa'angreal:* save bonus = `2 + ½ caster level`; use the better of bearer or item.

### Bull Rush

- Attack action or part of a charge.
- Target must be at most one size larger than you.
- Move into the defender's space (provokes AoO from threats including the defender).
- **Opposed Strength check.** Modifiers:
  - +4 per size category above Medium / -4 per below.
  - +2 charge bonus if charging.
  - Defender +4 stability if more than two legs / exceptionally stable.
- Win: push 5 ft + 1 ft per point of margin (capped at remaining movement). Lose: bounce back 5 ft (prone if blocked).

### Disarm

- Replaces a melee attack.
- Opposed attack rolls. Modifiers:
  - +4 per size-category difference (larger weapon wins).
  - +4 if defender wields the weapon two-handed.
  - Unarmed disarm attempt: -4 (typical for grapple-style disarm).
- Win: weapon dropped at defender's feet; if you attempted unarmed, you take it.
- Lose: defender immediately gets a free counter-disarm attempt.
- See Boarspear, Whip, Swordbreaker for weapon bonuses to disarm.

### Grapple

```
Grapple bonus = base attack bonus + Str mod + special size mod
```

| Size | Special Size Mod |
|------|------------------|
| Colossal | +16 |
| Gargantuan | +12 |
| Huge | +8 |
| Large | +4 |
| Medium | +0 |
| Small | -4 |
| Tiny | -8 |
| Diminutive | -12 |
| Fine | -16 |

#### Starting a Grapple

1. **Grab:** melee touch attack (provokes AoO).
2. **Hold:** opposed grapple check; deal unarmed-strike damage on success. Auto-fail vs. anything 2+ sizes larger (you can grab but not hold).
3. **Move in:** move into target's space.

#### Joining

To join an existing grapple: grab as above (auto-success on the grab); win an opposed grapple check to actually engage.

#### Grappling Options (each costs an attack)

- **Damage:** unarmed-strike damage (1d4 Large / 1d3 Medium / 1d2 Small + Str). For normal damage instead of subdual, take -4.
- **Pin:** hold immobile for 1 round. Opponents other than the pinner get +4 to attack a pinned target.
- **Break a pin:** free an ally.
- **Escape:** must beat all grapplers' results to escape; can move normally afterward.

#### While Pinned

Opposed grapple check as a melee attack to break the pin (you're still grappled).

#### Other Options

- Light weapon attacks while grappling (not pinning/pinned). No two-weapon fighting.
- Cast 1-action weave with Concentration DC `20 + casting level`.
- Escape Artist (opposed by grapple) to wriggle free.

#### Multiple Grapplers

Up to four equivalents on one target per round.

| Relative Size | Counts as |
|---------------|-----------|
| One smaller | ½ |
| Same size | 1 |
| One larger | 2 |
| Two+ larger | 4 |

#### Consequences

Lose Dex bonus to Defense vs. anyone you aren't grappling. Keep it vs. grapple partners.

### Grenadelike Weapons

- Ranged touch attack at the target's square.
- Direct hit: full damage. Splash: 5 ft radius from the landing square.
- Miss: roll `1d6` for distance away (+1 ft per range increment thrown), `1d8` for direction (1 long, 2 long+right, 3 right, 4 short+right, 5 short, 6 short+left, 7 left, 8 long+left).
- See `equipment.md` Table 7-10 for direct/splash/range damage values.

### Mounted Combat

- **Warhorses** are battle-trained; **light/heavy horses and ponies** require Ride DC 20 (move action) each round to keep under control. Failure escalates to a full-round action (no other action).
- The mount uses its action to move; you act on your initiative.
- Horse occupies a 5×10 ft space (you ride the rear half).
- Two-handed weapon use while mounted: Ride DC 5 (knees).
- If your mount moves > 5 ft you can only make **one** melee attack against any single target (full-attack against a single foe is impossible) — but extra attacks may be spent on additional targets the mount passes.
- +1 attack vs. on-foot Medium-or-smaller (higher-ground bonus).
- Mounted charge with a lance: **double damage**.
- Ranged attacks while mount is double-moving: **-4**; running mount: **-8**. Resolution at the mount's mid-move point. Full-attack possible during movement.
- **Channeling on horseback:** normal speed move OK before *or* after the cast. Both before and after → Concentration `DC 10 + casting level`. Mount running → can cast within 2x speed motion at Concentration `DC 15 + casting level`.
- **Mount drops:** Ride DC 15 to fall softly; failure = 1d6 damage.
- **You drop unconscious:** 50% to stay seated (75% in a military saddle); failure = fall + 1d6.

### Overrun

- During a charge.
- Target must be ≤ one size larger.
- Move ≥10 ft straight into target's space (provokes AoO).
- Defender chooses to **avoid** (you keep moving) or **block** (resolve a Trip).
- Trip win: continue charge straight. Trip loss & you're tripped: prone in defender's space. Loss without being tripped: bounced back 5 ft (prone if blocked).

### Trip

- Melee touch attack.
- Opposed Str check (defender uses higher of Str/Dex). Size: **+4** per size above Medium / **-4** per below; **+4** stability for >2 legs.
- Win: defender prone (Table 8-5 modifiers apply; standing up = move action).
- Lose: defender gets immediate counter-trip attempt.
- **Mounted target:** defender may use Ride in place of Str/Dex. Successful trip pulls rider from saddle.

### Unarmed Attacks

- Cannot crit.
- Damage: `1d3` Medium, `1d2` Small, `1d4` Large unarmed strike (subdual; +Str). Counts as a light weapon for two-weapon penalties.
- "Armed" unarmed attacks (Improved Unarmed Strike feat, channeler delivering touch weave, claws/fangs) avoid AoO from armed foes and provoke AoO from those foes' unarmed strikes vs. them.
- To deal **normal** damage with an unarmed strike instead of subdual: declare before the roll, take **-4**.

## Implementation Notes (WheelMUD)

- **Round/tick alignment:** existing tick scheduler should expose 6-second combat-round buckets. Initiative order is a sorted slice keyed on `(initiative, dexMod, tieBreaker)`. Effects expiring "in N rounds" should compare against the current `(roundIndex, initiativeCount)` rather than wall time, matching the "next-tick-on-same-count" rule.
- **Action accounting:** model the per-round budget as `(attackActions, moveActions, fullRound, freeActions)` flags rather than a generic counter — many rules pivot on action *type* and `5-ft step` eligibility. Track whether the character has already moved (no 5-ft step possible) or already taken any 5-ft step (no more movement).
- **Attack pipeline:** centralize one `resolveAttack(attacker, defender, weapon, modifiers)` function that walks through attack roll → threat → critical confirmation → damage roll → multiplier (excluding bonus dice). Pass options for `touchAttack`, `unarmedNormalDamage`, `chargeBonus`, `defensive`, etc.
- **Defense composition:** a single `computeDefense(state)` reading class-vs-equipment, Dex (subject to flat-footed/Dex-loss flags), size, dodge bonuses (separate stack), cover, and concealment. Keep dodge bonuses in their own slot so they vanish atomically when Dex is denied.
- **Threatened areas / AoO:** weapons need a `reach` flag (5/10 ft) plus a `noAdjacent` flag for true reach weapons. AoO trigger taxonomy: `MoveOut`, `MoveWithin`, `Distract` — emit them from the action dispatcher and a per-character listener spends one Combat-Reflexes-modulated AoO budget per round.
- **Surprise / flat-footed:** track on the combatant: `awareOf : Set<combatant>`. Awareness asymmetry triggers a surprise-round phase. Algai'd'siswai uncanny dodge clears the `flatFooted` Dex-loss flag without clearing the action restriction.
- **HP states:** cache `state ∈ {Healthy, Disabled, Dying, Dead}` derived from current HP and update on every damage event. Dying: schedule a per-round 10% stabilize roll on the round-end hook; Disabled: enforce single-action restriction in the action dispatcher.
- **Subdual:** independent counter; comparison vs. current HP determines `Staggered`/`Unconscious`. On unconscious, schedule a per-minute 10% wake check.
- **Massive damage:** any single damage event ≥ 50 triggers a Fort DC 15 immediately.
- **Weave casting:** the existing channeling system (Chapter 9 to come) needs hooks for `castingTime`, `concentrationDC`, `provokesAoO`, plus a `defensiveCast` mode. AoO-on-cast is a special case of `Distract`.
- **Two-weapon penalty matrix:** look up via `(offHandLight, ambidexterity, twoWeaponFighting)` rather than computing arithmetic — Table 8-2 has irregularities (Ambidexterity alone gives -6/-6 instead of -6/-6).
- **Cover/Concealment:** room-graph metadata already supports environmental flags; introduce per-attacker/defender pair lookups that pick the highest-priority cover and the highest miss chance (no stacking).
- **Object damage:** generic `Breakable` interface from `equipment.md` carries hardness, HP, and Break DC; combat damage applies `max(0, damage - hardness)`. Halve damage from ranged or energy sources before applying hardness.
- **Grapple state machine:** combatant gets a `GrappleState ∈ {None, Grabbing, Grappling, Pinning, Pinned}`; transitions via opposed grapple checks. Track grappling clusters with a multiset and the size-weight rules above.
- **Bull rush / overrun / trip:** all three are opposed-check actions with size and stability modifiers. Implement as a shared `OpposedManeuver` resolver parameterized on the abilities being checked.
- **Mounted combat:** mount becomes a movement source on the rider's turn; the rider's action dispatcher consults `mount.controlClass` and `mount.movedThisRound` for the various caps (single melee on movement, ranged penalties, casting concentration DCs).
- **Ready actions:** queue the readied trigger on the world event bus; on fire, splice the readied character into the initiative order at the trigger's count and consume their normal turn for that round.
- **Coup de grace:** combine helpless flag, full-round action, auto-hit + auto-crit, and a Fort save proportional to damage dealt. Skip for crit-immune creatures.
- **Healing pipeline:** convert real damage to subdual on healing-weave application; honor "cannot exceed normal HP" cap globally.
- **Temporary HP:** attach as a separate buffer with an expiration condition; consumed first when damage is taken; no rollover into real damage.
- **Critical confirmation:** weapon table provides threat range + multiplier; bonus dice (sneak attack) bypass multiplication and must be additive after the multiplied damage roll. Objects are crit-immune (skip the confirmation step).
- **Splash weapons / grenades:** place a `splash` event at the resolved square (direct hit or scatter via 1d6 distance / 1d8 direction); damage all creatures within 5 ft of that square.
- **Adjacent-to-melee ranged penalty:** when scheduling a ranged attack, scan the target's adjacent threats for an ally of the attacker and apply -4 unless the attacker has Precise Shot or the target is helpless.
