# The One Power

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 9: The One Power, pp. 154–185) for use in WheelMUD implementation.

## Overview

The One Power is drawn from the True Source. It is mechanically modeled as a slot-based casting system tied to character class (initiate, wilder), Affinities with the Five Powers, and a library of Talents (groups of related weaves).

## Setting Background

### Saidin and Saidar

- **True Source** = `saidin` (male) + `saidar` (female).
- Only women touch *saidar*; only men touch *saidin*.
- Cross-gender perception is asymmetric — neither gender can see/sense the other half. Men can *feel* another male channeler holding *saidin* within ~15 ft.
- Mechanically the two halves use the same rules — store one weave description per spell.
- *Saidin* is tainted; male channelers risk madness.

### Five Powers

`Air`, `Earth`, `Fire`, `Water`, `Spirit`. Every channeler has one or more Affinities. Weaves call for one or more Powers (their "Affinities" tag).

### Traditions (flavor for NPCs and admin tooling)

- **White Tower (Aes Sedai):** organized; bound by Three Oaths via Oath Rod ter'angreal; seven Ajahs; Hall of Sitters elects Amyrlin Seat. Bond Warders.
- **Aiel Wise Ones:** consensus-led; all female channelers join. Trained in Rhuidean ter'angreal; preserve dreamwalking lore.
- **Atha'an Miere Windfinders:** ship-based hierarchy; specialize in weather; secretive.
- **Seanchan damane / sul'dam:** a'dam ter'angreal binds channeler to controller; channeler held as property.
- **Asha'man:** male channelers under Rand al'Thor; Soldier → Dedicated → Asha'man; combat-focused.
- **Forsaken:** ancient Dreadlords with lost weaves. Not a unified force.
- **Wilders:** untrained channelers; can cast 0/1/2-level weaves outside their Talents.

### Lost / Rare / Common Weaves

| Tier | Learnable by |
|------|--------------|
| Common | Any channeler (subject to level/Affinity/Talent) |
| Rare | Must observe another channeler cast it (Weavesight). Wilders cannot reverse-engineer. |
| Lost | Must learn from a master (Forsaken, Dragon Reborn) or by observing a single use |

## Mechanics

### Talents and Weaves

- A **Talent** is a set of related weaves.
- A channeler may freely learn weaves from her Talents.
- Weaves *outside* her Talents have caps:
  - **Wilders:** 0/1/2-level only.
  - **Initiates:** 0-level only.
- Weaves have **levels 0–9** (and beyond via angreal/sa'angreal/linking/overchannel).

### Affinities and Effective Casting Level

When casting weave `X` with affinity tag `A`:

| You have... | Effective level |
|-------------|------------------|
| All listed Affinities | level − 1 |
| Some listed Affinities | level (as listed) |
| None of listed Affinities | level + 1 (fail if exceeds your max castable) |

Some weaves expose `+1 Casting Level:` modifications that augment the effect; others can be cast at multiple base levels with progressive effects.

### Embracing the True Source

- **Action:** full-round to embrace; persists until released.
- Gender-channelers see the embracer's aura (women) or sense saidin within ~15 ft (men). Some Shadowspawn see the saidar glow.
- While embraced you cannot rest, sleep, recover from fatigue, or heal subdual damage.
- Embracing is addictive; male channelers may accumulate Madness (GM discretion).

### Slots and Casting

- Initiates (Int-keyed) and wilders (Wis-keyed) get fixed daily slots per Table 3-4 / 3-8 (see `classes.md`).
- A higher-level slot may always be filled by a lower-level weave.
- A character without enough ability score loses high-level *castability* but keeps the slot count for lower-level weaves.

### Casting Sequence

1. Embrace the True Source (full-round, once).
2. Choose a known weave you can cast at the chosen casting level.
3. Apply Affinity adjustment to determine slot needed.
4. Decide all parameters at start of casting (range, area, target, version).
5. Spend casting time (action / full-round / minutes); hold concentration.
6. Apply effect on resolution.

### Casting Times

| Casting Time | Action |
|--------------|--------|
| 1 action | Attack action |
| 1 full round | Full-round action; resolves just before your next turn |
| 1 minute | 10 full-round actions; resolves just before your turn 1 minute later |

Concentration must persist for the entire casting time. Distraction → Concentration check or lose the weave (still consumes the slot).

### Range Bands

| Range | Distance |
|-------|----------|
| Personal | Self only |
| Touch | Touch a creature/object |
| Close | 25 ft + 5 ft / 2 levels |
| Medium | 100 ft + 10 ft / level |
| Long | 400 ft + 40 ft / level |
| Unlimited | Anywhere within the same realm |

### Aiming

- **Target / Targets** — must see or touch; specific identification required ("the leader" requires recognition).
- **Effect** — creates/summons something at a designated location; may move thereafter.
- **Beam** — ranged touch attack; hits intervening creatures (instant effect).
- **Area** — caster picks origin; weave defines shape.
- **Cone** — starts at caster, widens equal to range (5×5 at 5 ft, 25×25 at 25 ft).

### Saving Throws

- **DC** = `10 + weave level + caster's relevant ability mod` (Int initiate / Wis wilder).
- **Negates** = no effect on save.
- **Partial** = lesser effect on save.
- **Half** = half damage on save (round down).
- **None** = no save.
- **(Object)** — only attended objects save (using bearer's bonus).
- **(Harmless)** — beneficial weave; target may save anyway.
- **Voluntary forgo:** may willingly accept the effect.
- **Items destroyed on natural-1 save:** roll vs. Table 9-2 list.

### Items Affected by Channeling Attacks (Table 9-2)

Determine the four most-likely items struck and randomly pick one when the bearer rolls a natural 1 on a save against an object-affecting attack:

| Order | Item |
|-------|------|
| 1 | Shield |
| 2 | Armor |
| 3 | Helmet |
| 4 | Item in hand |
| 5 | Cloak / clothing |
| 6 | Stowed / sheathed weapon |
| 7 | Backpack / scrip |
| 8 | Coin purse |
| 9 | Jewelry |
| 10 | Anything else |

Angreal / ter'angreal / sa'angreal **always** save.

### Holding a Weave

| Duration | Behavior |
|----------|----------|
| Instantaneous | Resolves immediately; consequences may persist |
| Concentration | Persists while caster concentrates (full-round each round). Distraction ends it. |
| Fixed (X rounds, etc.) | Auto-runs for the listed duration |

- Maintaining a weave does **not** provoke AoOs.
- Cannot cast another weave while holding one without **Multiweave** feat.
- **Tie Off Weave** feat lets you release concentration and have it persist (with a finite tied-off duration — see `feats.md`).
- A non-channeling target who saves successfully feels nothing; same-gender channelers detect a tingle and may identify it via Weavesight.

### Distractions

Concentration DCs are detailed in the Concentration skill (Chapter 4). Sources include motion, weather, damage, hostile weaves, and casting on the defensive.

### Overchanneling

Allows casting beyond normal slot capacity. **Concentration check** to succeed; failure forces a **Fortitude save** vs. injury / stilling / death.

| Concentration DC | Attempt | Fort DC |
|------------------|---------|---------|
| 15 | 0-level weave with no slots left | 15 |
| 20 | 1st-level weave with no slots left | 25 |
| 25 | 2nd-level weave with no slots left | 35 |
| 20 | Cast weave 1 level higher than slot | 15 + weave level |
| 25 | Cast weave 2 levels higher than slot | 25 + weave level |
| 30 | Cast weave 3 levels higher than slot | 35 + weave level |

You must always use your highest available slot when overchanneling. You may not overchannel if a normal cast would work.

### Failure Cascade (Fort save margin)

| DC missed by | Penalty | Damage | Recovery |
|--------------|---------|--------|----------|
| 1–5 | -1 to all rolls | — | 6 hr rest |
| 6–10 | -2 | 1d6 | 6 hr rest; cannot overchannel until then |
| 11–15 | -3 | 2d6 | Cannot channel for 24 hr |
| 16–20 | -4 | 3d6 | Cannot channel for 48 hr |
| 21–25 | -5 | 4d6 | Cannot channel for 2 weeks |
| 25+ | -6 + **stilled** | 4d6 | Permanent (until `restore the power` weave) |

### Weave Failure

Casting fails (and the slot/concentration is wasted) if range/area/target conditions cannot be met, or if concentration is broken.

### Angreal and Sa'angreal

- Pure level boost: rated 1–3 (angreal) or 4–10 (sa'angreal). Same scale; rating = extra weave levels added to the slot used.
- Gender-tuned: a saidar device is invisible/inert to a male channeler and vice versa.
- Use only requires touching the device while casting.

### Linking (Table 9-1)

Joined channelers stand in a circle; each makes Concentration to maintain the link. Same-gender circles must include women; women-only circles cap at 13. Above 13, ≥1 man required.

| Circle Size | Min Men | Max Men | Bonus Levels |
|-------------|---------|---------|--------------|
| 2–3 | 0 | 1 | +1 |
| 4 | 0 | 2 | +1 |
| 5–6 | 0 | < ½ total | +1 |
| 7–13 | 0 | < ½ total | +2 |
| 14–27 | 1 | < ½ total | +3 |
| 28–36 | 2 | < ½ total | +4 |
| 37–45 | 3 | < ½ total | +5 |
| 46–54 | 4 | < ½ total | +6 |
| 55–63 | 5 | < ½ total | +7 |
| 64–72 | 6 | < ½ total | +8 |

Leadership: must be male in 1m/1w circles, in 13-or-fewer circles with ≥2 men, and in 72-channeler circles. Otherwise either gender. Link breaks when leader is distracted or chooses to dissolve it.

## Weave Catalog

The catalog is grouped by Talent. Each entry lists Affinities, rarity, level range, casting time, range, duration, save, and a one-line summary. Full numeric tables retained where they drive mechanics.

### Balefire (Lost)

- **Balefire** [Air, Earth, Fire, Spirit, Water]; Level 9; 1 action; Beam; instant; Reflex negates. Beam of white-hot light; living creatures struck are erased and their actions retroactively undone (`backburn`). Cannot be tied off.

| Casting Level | Range | Backburn |
|---|---|---|
| 9 | 25 ft | 5 sec |
| 10 | 75 ft | 1 min |
| 11 | 150 ft | 10 min |
| 12 | 300 ft | 2 hr |
| 13 | 600 ft | 1 day |
| 14 | 1,200 ft | 10 days |

`+2 Casting Levels:` duration → concentration; sweep beam, one creature/round.

### Cloud Dancing

| Weave | Affinities | Tier | Level | Cast | Range | Duration | Save | Effect |
|-------|------------|------|-------|------|-------|----------|------|--------|
| Foretell Weather | Air, Water | C | 0–3 | 1 min | Close | Instant | None | Sense future weather (4 hr / 2 d / 2 wk / 1 season) |
| Harness the Wind | Air, Water | C | 0–7 | 1 act | Long | Concentration | None | Conjure wind, level sets strength + area |
| Lightning | Air, Fire | C | 5–9 | 1 fr | Long | Instant | Reflex half | Call lightning bolts; brew time at higher levels |
| Raise Fog | Air, Water | C | 2–8 | 1 fr | Medium | Instant | None | Dense 20-ft-tall fog; ½ conceal at 5 ft, total beyond |
| Warmth | Air, Fire | C | 0–3 | 1 act | Close | Instant | None | Warm/cool air to comfortable temperature in 15-ft circle |

### Conjunction

| Weave | Affinities | Tier | Level | Cast | Range | Save | Effect |
|-------|------------|------|-------|------|-------|------|--------|
| Bond Warder | Spirit | C | 5 | 1 min | Touch | Will negates | Permanent bond; aging slow, compel-to-obey, distance/emotion sense, energy lend, proximity pull, shared saves; broken by death/stilling/release |
| Compulsion | Air, Earth, Fire, Spirit, Water | Lost | 3–5 | 1 act | Close | Will negates | "Influence" (3rd) = trusted master; "Command" (5th) = devoted slave; +1 CL = embed durable command per +1 |
| False Trail | Air, Earth, Spirit | C | 0–8 | 1 fr | See text | Will negates (harmless) | Lay diversionary scent/footprints; affects more creatures and longer trails at higher levels |
| Pass Bond | Spirit | C | 7 | 10 min | Touch | Will negates | Designate a successor channeler to inherit your Warder bond on your death |
| Sense Shadowspawn | Spirit | C | 0 | 1 act | 50 ft / level | See text | Sense Shadowspawn presence within range |
| Trace | Spirit | C | 0–4 | 1 act | See text | None | Sense known person's presence/direction; range x2 if intense emotion, x100 if carrying a gift you gave |

### Earth Singing

| Weave | Affinities | Tier | Level | Cast | Range | Save | Effect |
|-------|------------|------|-------|------|-------|------|--------|
| Earth Delving | Earth | C | 0–3 | 1 act | Medium | None | Sense mineral concentrations; radius 5 / 25 / 150 / 750 ft |
| Earthquake | Earth | C | 7–12 | 1 fr | Long | See text | Shock wave; collapses cave/cliff/structure; 8d6 cave-in, fissures; areas 50 ft → 5 mi |
| Grenade | Earth, Fire | C | 0–4 | 1 fr | Touch | Reflex half | Imbue stone with explosive (sling-stone 1d8 contact / fist 3d6 10 ft / catapult 5d6 20 ft) |
| Polish | Earth | C | 0–2 | 1 act | Touch | None | Remove rust/corrosion from metal; restores pitted weapons / hinges |
| Riven Earth | Earth, Fire | C | 4–6 | 1 fr | See text | Reflex half | Erupt earth at a point; 3d10 blast + violent throw of objects/creatures |

### Elementalism

| Weave | Affinities | Tier | Level | Cast | Range | Duration | Save | Effect |
|-------|------------|------|-------|------|-------|----------|------|--------|
| Arms of Air | Air | C | 0–12 | 1 act | Medium | Concentration | None | Telekinetic lift up to weight by level (5 / 25 / 100 / 200 / 400 / 800 / 1,500 / ... / 100,000 lb) |
| Blade of Fire | Air, Fire | C | 1–5 | 1 act | Touch | Concentration | None | Cutting torch on held tool (5 in / 1 ft / 2 ft); 2d6 touch attack |
| Create Fire | Fire | C | 0–6 | 1 act | Medium | Concentration | Will half | Create or alter fire (candle → firestorm); damage 1 → 5d8/round |
| Cutting Lines of Fire | Air, Fire | Lost | 7–9 | 1 act | Cone (30 / 50 / 70 ft) | Instant | Reflex half | 2d12 to all in cone; cuts material |
| Current | Spirit, Water | C | 0–7 | 1 act | Long | Concentration | None | Conjure water current; floods downstream |
| Dry | Water | C | 1 | 1 act | Close | Instant | Will negates (harmless) | Squeeze water out of object; +2 CL keeps target dry vs. ongoing water |
| False Wall | Air, Earth | C | 1–6 | 1 act | Medium | Concentration | None | As `harden air` but appears to be solid rock; +1 CL color/texture |
| Fiery Sword | Air, Fire, Spirit | C | 2–4 | 1 act | Touch | Concentration | None | Create fire weapon (any shape; 2d8 / 2d10 / 2d12); proficiencies apply |
| Fireball | Air, Fire | C | 2–6 | 1 act | Medium | Instant | Reflex half | Burst (5–50 ft); damage NdN+channeler level |
| Fly | Air, Spirit | Lost | 5 | 1 act | Touch | Concentration | Will negates (harmless) | 90 ft (60 if armored) flight; falls 60 ft/round on dispel |
| Harden Air | Air | C | 0–5 | 1 act | Medium | Concentration | Reflex | Solidify air space; freezes creatures, builds bridges |
| Immolate | Fire, Spirit | C | 4–7 | 1 act | Medium | Instant | Will half | Ignite target; 1d6/level (max 20d6); flammable/non-flammable, Medium/Large by level |
| Light | Air, Fire | C | 0–3 | 1 act | Personal | Concentration | See text | Globe of light; at lvl 3 a Reflex save vs blindness; +1 CL fixed location |
| Move Water | Water | C | 3 | 1 act | Close | Concentration | None | Move 50 gal/level at 20 ft/round |
| Tool of Air | Air | C | 0–4 | 1 act | Close | Concentration | Will half | Invisible tool; at high CL becomes weapon (sap → sword) |
| Wand of Fire | Earth, Fire | C | 1 | 1 act | Touch | Concentration | None | Imbue wand/branch with fire (touch attack 1d8 + 1/lvl, max +20) |
| Whirlpool | Spirit, Water | C | 3–7 | 1 fr | Medium | Concentration | None | Whirlpool in body of water; size by level; sinks ships |

### Healing

| Weave | Affinities | Tier | Level | Cast | Range | Save | Effect |
|-------|------------|------|-------|------|-------|------|--------|
| Delve | Spirit | C | 0–3 | 1 min | Touch | Will negates (harmless) | Diagnose wounds (0) / disease (1) / poison (2) / supernatural (3); +5 to Heal aid checks |
| Heal | Air, Spirit, Water | C | 0–8 | varies | Touch | Will negates (harmless) | Convert HP damage to subdual; 1 / 1d8+lvl / 2d8+lvl ... 8d8+lvl. Once/target/day |
| Heal the Mind | Air, Spirit, Water | C | 1–4 | 1 min | Touch | Will negates (harmless) | Reduce Madness rating temporarily (1d6 / 2d6 / 3d6) |
| Rend | Air, Spirit, Water | Rare | 0–4 | 1 act | Close | Fortitude half | Rip flesh; 1 / 1d8+lvl ... 4d8+lvl |
| Renew | Air, Spirit, Water | C | 0–4 | 1 fr | Touch | Will negates (harmless) | Suspend subdual damage; debt returns + extra subdual on expiry |
| Restore the Power | Air, Earth, Fire, Spirit, Water | Lost | 6–12 | 10 min | Touch | Will negates (harmless) | Reverse stilling/gentling (CL 6: 1/3 weaves / 8: ½ / 10: ¾ / 12: full) |
| Sever | Spirit | C | 6 | 1 act | Close | Will negates | Permanently still/gentle a same-gender target; +6 CL = opposite gender |
| Touch of Death | Earth, Fire, Spirit, Water | Lost | 5–8 | 1 fr | Close | Fortitude (varies) | Internal damage (chokes 4d8 / crushes 6d8 / boils blood 8d8 / 8th = stops heart, save loses ½ HP) |

### Illusion

| Weave | Affinities | Tier | Level | Cast | Range | Duration | Save | Effect |
|-------|------------|------|-------|------|-------|----------|------|--------|
| Disguise | Air, Fire, Spirit | C | 1–4 | 1 fr | Touch | Concentration | Will negates | Visual disguise (+2 minor / +10 major); self vs. other |
| Distant Eye | Air, Spirit | Lost | 3 | 1 fr | Medium | Concentration | — | Remote-view tendril; squeezes through ¼-in gaps |
| Eavesdrop | Air, Spirit | C | 1 | 1 fr | Medium | Concentration | — | Remote-listen tendril; same gap rules |
| Folded Light | Air, Fire | C | 1–4 | 1 act | Close | Concentration | Will negates (harmless) | 10-ft-tall invisibility screen for object/person/group; movement = Spot 15/20/25 |
| Mirror of Mists | Air, Fire, Spirit | C | 0–2 | 1 act | Personal | Concentration | — | Grow + intimidating presence (+2 / +4 / +8); 2nd level dazes Medium-or-smaller (Will negates daze) |
| Voice of Power | Air, Fire | C | 0–1 | 1 act | Touch | Concentration | Will negates (harmless) | Project voice up to 4 mi; +1 Intimidate; CL 1 = others |

### Traveling (Lost)

| Weave | Affinities | Tier | Level | Cast | Range | Effect |
|-------|------------|------|-------|------|-------|--------|
| Bridge Between Worlds | Earth, Spirit | Lost | 7–11 | 1 fr | Close | Gateway between real world and Tel'aran'rhiod (size + duration scale with CL) |
| Create Gateway | Spirit | Lost | 4–8 | 1 fr | Close | Gateway into the space between places; +3 CL = direct gateway to a known location |
| Skimming | Air, Earth, Spirit | Lost | 4–8 | 1 fr | Close | Travel platform through space-between; size and capacity scale; 1d6+1 min per 100 mi |
| Use Portal Stone | Spirit | Rare | 4–7 | 1 fr | Touch | Trigger Portal Stone for instant travel; +1 CL → mirror worlds |

#### Bridge Between Worlds — Gateway Size

| CL | Max Size | Stays Open |
|----|----------|-----------|
| 7 | 5×10 ft | 2 rounds |
| 9 | 10×15 ft | 1 round/level |
| 11 | 30×20 ft | 3 rounds/level |

#### Create Gateway — Gateway Size

| CL | Max Size | Stays Open |
|----|----------|-----------|
| 4 | 5×10 ft | 2 rounds |
| 5 | 10×15 ft | 1 round/level |
| 6 | 30×20 ft | 3 rounds/level |
| 7 | 100×25 ft | 5 rounds/level |
| 8 | 300×30 ft | 1 min/level |

#### Skimming — Platform Capacity

| CL | Platform | Passengers |
|----|----------|-----------|
| 4 | 5-ft sq | 0 |
| 5 | 10-ft sq | 4 |
| 6 | 15-ft sq | 25 |
| 7 | 25-ft sq | 120 |
| 8 | 35-ft sq | 200 |

#### Use Portal Stone — Capacity

| CL | Creatures |
|----|-----------|
| 4 | 5 |
| 5 | 50 |
| 6 | 100 |
| 7 | 500 |

### Warding

Wards are dome-shaped (or volume-equivalent); cannot overlap, contain, or be contained within other wards.

| Weave | Affinities | Tier | Level | Cast | Range | Effect |
|-------|------------|------|-------|------|-------|--------|
| Barrier to Sight | Air, Fire, Spirit | C | 1–10 | 1 fr | Close | Visually opaque dome; people pass through |
| Circle of Silence | Air, Fire, Water | C | 0–9 | 1 fr | Close | Sound-blocking dome |
| Dream Shielding | Spirit | C | 1–11 | 1 fr | Close | Prevents dreamwalkers / dream observation; tie off before sleep to ward yourself |
| Fire Trap | Air, Fire, Spirit | Rare | 3–5 | 1 fr | Touch | On-trigger fire burst (3 / 4 / 5 dN+lvl); 5 / 10 / 15 ft radius |
| Master Ward | Air, Earth, Fire, Spirit, Water | C | 4–12 | 1 fr | Close | Dome that blocks all matter and One Power; suffocation hazard; +1 CL opaque |
| Seal | Air, Fire, Spirit | C | 2–4 | 1 fr | Touch | Ward inside a container; explodes on opening (1d4 / 1d6 / 1d8); +1 CL trigger word |
| Shield | Spirit | C | 3–7 | 1 act | Close | Cut a same-gender channeler off from the Source; CL set by target level differential; +1 CL opposite gender; -2 CL if not embracing |
| Strike of Death | Air, Fire, Spirit | C | 8–12 | 1 fr | See text | Lightning bolts kill all creatures of a named Shadowspawn type within range (Will partial = ½ HP); +2 CL = +1 type; range 30 ft → 100 mi |
| Ward Against People | Air, Fire, Spirit | C | 2–11 | 1 fr | Close | Dome blocks people not present at casting; +1 CL = trigger for tied-off second weave |
| Ward Against Shadowspawn | Air, Fire, Spirit | C | 1–10 | 1 fr | Close | Block Shadowspawn from entering; +1 CL also affects Shadow-linked vermin |
| Ward Against the One Power | Air, Earth, Fire, Spirit, Water | C | 3–12 | 1 fr | Close | One Power cannot pass dome boundary |
| Ward Bore | Air, Earth, Fire, Spirit, Water | Lost | 4 | 5 min | Medium | Weavesight check (DC 20 + target weave level) to bore a hole through a ward; failure by ≥10 alerts the warder |

#### Standard Warding Areas (Master Ward / Barrier to Sight / Circle of Silence / Ward Against People / Ward Against Shadowspawn / Dream Shielding / Ward Against the One Power)

Per-level area progression generally follows:

| CL | Area |
|----|------|
| (base) | 5-ft-radius circle |
| +1 | 10-ft-radius circle / small room |
| +2 | 25-ft-radius circle / large room |
| +3 | 50-ft-radius circle / moderate building |
| +4 | 150-ft-radius circle / large building |
| +5 | 300-ft-radius circle / very large building |
| +6 | 750-ft-radius circle |
| +7 | 1,500-ft-radius circle |
| +8 | 1-mile circle |
| +9 | 5-mile circle |

(Each weave's table lists its actual base level.)

## Implementation Notes (WheelMUD)

- **Channeler model:** distinct fields for `gender`, `affinities: Set<Power>`, `talents: Set<Talent>`, `weavesKnown: List<WeaveID>`, `slots: Map<level, current/max>`, `embraced: bool`, `madness: int` (men only), `stilledAt: Option<RestoreLevel>`. Slot caps come from the class table (`classes.md`); recompute on level up.
- **Affinity adjustment:** when resolving a cast, compute `effectiveLevel = baseLevel + (none ? 1 : 0) - (all ? 1 : 0)`; reject if exceeds slot capacity.
- **Source state machine:** `Disengaged → Embracing (full-round) → Embraced → Releasing (free)`. Embraced flag drives passive perception (same-gender detection within 15 ft), saidar aura visibility, and the rest/heal blockers.
- **Madness counter** ticks while embraced (admin-tunable rate). Mental Stability feat reduces it (`feats.md`).
- **Casting pipeline:** unify with the combat attack pipeline (`combat.md`) — channelers schedule a `CastingAction` with start/finish initiative counts; concentration check listeners hook into the AoO/distract event taxonomy.
- **Concentration** is its own check (Chapter 4 skill). DC formulas for "casting on the defensive" (15 + level), distractions (motion, weather, damage), and grappling (20 + casting level) live alongside the skill module.
- **Range** computation is a runtime function of `casterLevel` for Close/Medium/Long; bake into a helper.
- **Aiming targets:** four shapes (`Target`, `Effect`, `Beam`, `Area-Cone`/`Area-Sphere`/`Area-Dome`/`Area-Burst`); each weave supplies a shape descriptor used by the resolver to enumerate hit creatures and apply saves.
- **Saving throws:** DC = `10 + level + abilityMod`. Persist `weaveResistance: bool` per weave. Provide hooks for `(Object)`, `(Harmless)`, voluntary forgo, and the items-affected list (Table 9-2) on natural-1.
- **Ties off / holds:** weave instances are first-class objects with `state ∈ {Casting, Held, TiedOff, Resolved}`; the channeler holds a list of held weaves whose count is gated by `Multiweave` purchases.
- **Concentration channel:** the existing tick scheduler should expose a per-channeler "concentration check" event each round for held weaves (free, no AoO).
- **Overchannel:** wrap each `cast` call in a `tryOverchannel(weave, slotLevel)` that runs Concentration → Fortitude on failure → applies the cascade penalties via a status-effect system (`-1 to -6` modifiers, damage, channeling lockouts, stilling).
- **Linking:** model as a `Circle` aggregate with leader, members, `requiredMen` per Table 9-1, and a `bonusLevels` field added to the leader's slot for the cast. Break on leader distraction.
- **Angreal/sa'angreal:** items carry `(power: 1..10, gender: M/F, attuned: bool)`. While held, add `power` to the slot level used. Cross-gender devices appear inert.
- **Talent / weave catalog:** load weaves from a YAML or SQL table keyed by `Talent` with columns for affinities, tier (Common/Rare/Lost), base level, casting time, range band, area shape, duration kind, save type, and a free-form effect script. Lost/rare gating happens at learn time, not cast time.
- **Wilders vs. initiates:** enforce out-of-Talent caps (0/1/2 for wilders, 0 for initiates) in the learn/cast validators; Mental Stability is the only General feat a male channeler may take with a channeling slot (`feats.md`).
- **Bond Warder pipeline:** persistent link object keyed to channeler↔Warder; exposes hooks for compel-to-obey (Will save), HP-lend ledger (300 ft cap), proximity pull, slow-aging on Warder, shared save bonuses, and on-death damage propagation. Pass Bond is a delayed-trigger inheritance.
- **Tel'aran'rhiod / space-between:** these are alternate world contexts; create gateway / skimming / bridge between worlds need world-graph adapters that can move actors between contexts and re-enter at coordinates. Trapped creatures stay there.
- **Wards as room shells:** dome wards are room-overlay objects with non-overlap invariants. Master Ward effectively turns a region into an air-tight bubble — schedule suffocation timers when sealed.
- **Strike of Death etc.:** type-targeted weaves need a `creatureType` enum (Shadowspawn, Darkbound, Shadow-linked vermin) on every NPC.
- **Ter'angreal interactions:** Oath Rod (binding speech), a'dam (mind-leash), Portal Stones, Rhuidean test, Warder bond — all listed elsewhere; flag them as scriptable items rather than raw weaves.
- **Catalog hygiene:** Lost/Rare weave acquisition requires a Weavesight observation event (`feats.md` Sense Residue interplay) — store provenance per character so admins can audit how a forbidden weave entered a character sheet.
