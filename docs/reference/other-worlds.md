# Other Worlds

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 13: Other Worlds, pp. 282–289) for use in WheelMUD implementation.

## Overview

Five alternate realms intersect the natural world:

| Realm | Access | Risk profile |
|-------|--------|--------------|
| Tel'aran'rhiod (World of Dreams) | Dreaming, Dreamwalk feat, certain ter'angreal, gateway weave | Injury/death cross over to the real body |
| The Ways | Waygates only | Lost, falling, Shadowspawn, Machin Shin |
| Mirror Worlds | Portal Stones (use Portal Stone weave) | Each world has its own hazards |
| Stedding | Walk in from any direction | Safe — but the One Power is inert inside |
| Aelfinn / Eelfinn | Two specific ter'angreal + the Tower of Ghenjei | Unknown but powerful inhabitants |

## Tel'aran'rhiod (World of Dreams)

### Nature

- A reflected universe — every mountain, river, city, and palace has a `Tel'aran'rhiod` mirror.
- **Empty of living beings.** Sleepers and physical visitors appear briefly; wolves arrive on death and remain until rebirth; rare visitors like *Slayer* hunt there.
- **Permanent objects** mirror exactly. **Transient objects** drift, swap, lock/unlock, change color — purely metaphoric, never tied to real-world motion.
- Inanimate changes do not propagate to the real world. Altered objects revert to their natural-world reflection over time.
- The "space between dreams" is the dark void where most ordinary dreams live as bubble realities — separate from `Tel'aran'rhiod` proper.

### Entry Methods

| Method | Notes |
|--------|-------|
| Ordinary dreaming | Random, unreliable, indistinguishable from normal dreams. |
| **Dreamwalk** feat (Lost Ability) | Sleeper enters at will; conscious control. |
| Dreaming ter'angreal | Some aid dreamwalkers, others let non-dreamwalkers enter (sleep with item against skin). |
| Channeled gateway | Physical entry — body comes along; arrives at the same coordinate. |

### Dreamer vs. Physical Visitor

| Dreamer (asleep) | Physical visitor (gateway) |
|------------------|----------------------------|
| No food/water needed | Needs real rest, food, water — dream food provides no nourishment |
| Can wake out at will (Concentration DC 15 from Dreamwalk) | Can only leave through another gateway — can be trapped |
| No body in dream world | Body is here; injury/death cross over both ways |
| Free at exit | On every physical exit: **Fortitude DC 10** or permanently lose **1d4 Charisma** |

### Self-Customization

Visitors with sufficient practice can shape their own appearance (clothing, gear, hair, features). See **Bend Dream** feat (`feats.md`).

### Movement

- Walking is universal.
- Mounts and wagons do not exist in `Tel'aran'rhiod` — bring your own only via gateway.
- Dreamwalkers may **Dream Jump** between known points (see `feats.md`).

### Hazards

- **Carry-over damage:** anything that hits you here hits the real body.
- **Hostile visitors:** Aiel Wise Ones (Dreamwalk-trained), some Forsaken, Slayer.
- **Wolves** appear after death; not normally hostile but unpredictable.
- **Stedding regions are off-limits** in `Tel'aran'rhiod` — visitors cannot enter the dream-mirror of a stedding.

## The Ways

### Nature

- A netherworld of stone bridges, ramps, and platforms suspended in a (formerly bright, now utterly dark) void.
- Connect Waygates near every Ogier stedding and most great cities.
- Light and sound travel **half** their real-world distance (a torch lights a 10-ft radius).
- Stonework is pitted, decaying; some bridges have collapsed.

### Origin

- Built by male Aes Sedai during the Breaking as a gift to Ogier sheltered in stedding.
- Later expansions to the great cities by Ogier using the **Talisman of Growing** ter'angreal (treesinger-triggered).
- ~1,000 years ago, the saidin taint corrupted the Ways. Ogier Elders banned use.

### Waygates

- Stone wall ~8 ft × 12 ft carved with vines/leaves.
- Hidden door opened by moving the central **Avendesora leaf** to a new spot; replace it to close.
- **Knowledge (arcana) DC 22** to figure out how to open one without prior demonstration.
- Crossing the inner film feels like passing through a sheet of ice; no resistance.
- Closed doors can be opened from inside via the interior Avendesora leaf.

#### Locking a Waygate

| Action | Effect |
|--------|--------|
| Remove leaf from one side, place among carved vines on the other | Locks against re-entry from the depleted side |
| Remove leaves from both sides | Permanently sealed (until a Talisman of Growing regrows them) |
| Leaf removed for >1 day | Loses unlock power; if both gone, the gate is dead |

### Travel

- Each Waygate exits onto a stone island. A white stripe leads to the first **pedestal** — a stone column inscribed in flowing Ogier script.
- **Knowledge (arcana) DC 16** at each pedestal to interpret the route. Failure → wrong path → +1 pedestal added to the trip.
- Each pedestal has a 5% chance (`01–05` percentile) of being unreadable. Forced random choice: roll 1d6; **1** = correct, **5–6** = same as a failed check (+1 pedestal).
- Distance between pedestals averages ~4 hours of walking pace.

| Route | Pedestals |
|-------|-----------|
| Stedding → stedding | 2d4 |
| Stedding ↔ city | 2d6 |
| City → city | 2d8 |

### Time Compression

When a party exits the Ways, divide the time spent inside by **1d4** to get real-world elapsed time. Example: 72 hr forced march in the Ways with a roll of 3 → 24 hr passed outside.

### Mapping

- **Knowledge (arcana) DC 20** to research a written map (requires obscure libraries — White Tower, large stedding).
- Track the margin by which the mapmaker beat the DC.
- Each pedestal in the trip becomes a `DC 16 − margin` Knowledge (arcana) check against the map.
- Map check failure means **lost** (not "+1 pedestal" — true lost).

### Lost Wanderers

- Without map and Ogier-script literacy, navigation is impossible — characters become lost.
- Lost characters search for any Waygate: 5% chance per 4 hours; destination unknown until opened.

### The Endless Plummet

Falling from a bridge:

- 5% chance to land on a lower bridge/ramp/island. Distance: `3d6 × 10 ft`. Apply standard falling damage. Survivor is now separated from companions above.
- Otherwise: falls forever (death by thirst, starvation, or terror) or strikes a far surface and dies instantly.

### Shadowspawn Encounters

Roll **once per trip**: percentile vs. number of pedestals. Below = encounter occurs. Roll again on the table:

| d% | Encounter |
|----|-----------|
| 01–55 | A single Trolloc straggler |
| 56–70 | A gang of 2d4 Trollocs |
| 71–85 | A Myrddraal |
| 86–100 | A band of 2d8 Trollocs and 1 Myrddraal |

(Chapter prints overlapping ranges 71–90 / 86–100; treat 71–85 / 86–100 as the intended split.)

### Machin Shin (the Black Wind)

- Cold wind voicing death/decay/madness. Cannot be harmed — physical attacks fail; no weave is known to slow it.
- Slight tendency to lurk near Waygates.

| Location | Encounter chance |
|----------|------------------|
| At a Waygate | 5% |
| Within 1 pedestal of a Waygate* | 4% |
| Otherwise | 2% |

\* Between the first and second pedestals of a trip, or between the last pedestal and the destination Waygate.

- **Speed** 30 ft; covers a 30-ft area.
- **Listen DC 15** as it approaches to hear the whisper.
- Each round of pursuit: cumulative +10% chance Machin Shin loses the runners (`01–10` round 1, `01–20` round 2, ...).
- If it catches a victim, it pauses on them until devoured.
- **Will DC 20** each round inside it or **permanently drain 1d4 Wisdom and 1d4 Intelligence**. Either ability hits 0 → soulless husk (devoured).

> **GM note:** Machin Shin can wipe a party. The book recommends using it as tension/atmosphere, not a stat-block fight.

## The Mirror Worlds

### Nature

- Infinite alternate realities (some near-identical to ours, some uninhabitable).
- Many Seanchan exotic creatures are believed to have been imported from Mirror Worlds.

### Entry

- **Portal Stones** — 20 ft tall, 3 ft across stone columns covered in destination symbols.
- Channeler uses the **Use Portal Stone** weave (`the-one-power.md`) on a destination symbol.
- Not every Stone in our world has a counterpart in every Mirror World; many Mirror Worlds are entirely Stone-less.

### Mechanics in a Mirror World

- Terrain typically mirrors the real world (some near-identical, others alien).
- The One Power generally works the same.
- Portal Stones inside a Mirror World connect to each other and to other Mirror Worlds.

### Hazards

- Per-world: poisonous atmosphere, Shadowspawn occupation, exotic predators, "unimaginable chaos."
- No reliable catalog. GM seeds each Mirror World with its own threat table.

## Stedding

### Nature

- Localized regions; pleasant air, ancient flora, deep peace.
- Most stedding are ≤ 10–15 mi across; most active ones lie in mountains or remote forests.
- **The One Power is inert inside.** Channelers cannot touch, use, or sense saidar/saidin while in a stedding.
- Weaves cast from outside cannot affect targets inside.
- Stedding regions are absent from `Tel'aran'rhiod` — dreamwalkers cannot enter their reflection.

### Origin

- Pre-Breaking; existence predates Ogier memory.
- Believed not Ogier-made.
- Many were lost during the Breaking.
- Some abandoned stedding retain their properties indefinitely.

### Settlement Pattern

- Mound-homes built into the earth.
- Great Trees rising hundreds of feet with trunks "scores of feet" across.
- Often a polished tree-stump in the village center serves as a council space.

### Entry & Hazards

- No barrier — walk in from any side. The boundary is felt (peace for the welcome, unease for Shadowspawn/Darkfriends), not seen.
- Trollocs refuse to enter unless forced. Myrddraal enter only at direst need. Even committed Darkfriends are uncomfortable.
- Hazard-wise: stedding are *safer* than the surrounding world. Even abandoned stedding make solid rest stops.

## Aelfinn and Eelfinn

- Two mysterious realms inhabited by powerful, dangerous races.
- Access is **only** via specific ter'angreal and the **Tower of Ghenjei** (somewhere in the Mountains of Mist).
- The children's game **Snakes and Foxes** is hinted to encode the rules for dealing with their inhabitants.
- The book provides no mechanical detail beyond this.

## Implementation Notes (WheelMUD)

- **World-graph as the right primitive:** every alternate realm is another connected graph adjoined to the natural-world graph via gateway/portal/Waygate edges. Add a `realm` field to every room (`natural`, `telAranRhiod`, `ways`, `mirror.<id>`, `stedding`, `betweenDreams`, `aelfinn`, `eelfinn`).
- **Tel'aran'rhiod mirror:** lazily mirror real rooms when first visited. Cache transient-object snapshots that drift on each tick (object state randomly mutates; revert to canonical real-world state when the visitor leaves long enough). Permanent fixtures stay deterministic.
- **Tel'aran'rhiod entry types:** distinguish `dreamerVisit` (no body, no food/water tick, can wake out via Concentration) from `physicalVisit` (real inventory + needs + the gateway-only exit). Physical exit triggers a `Fort DC 10 → -1d4 Cha permanent` save.
- **Bend Dream / Dream Jump:** route to the same feat handlers in `feats.md`. Their Concentration DCs key off whether the visitor is the dreamer or another dreamwalker.
- **Damage carry-over:** combat in `Tel'aran'rhiod` uses the same combat pipeline as the real world; HP is shared. Object damage in the dream world does **not** propagate.
- **Stedding region in dream world:** mark `telAranRhiod` mirror rooms inside a stedding boundary as `inaccessible` and bounce attempted entries.
- **Ways:**
  - Model as a separate graph keyed by Waygate ID and pedestal sequence.
  - Each pedestal node carries a Knowledge (arcana) DC and a `degraded` flag (5% chance per visit, optional persistence).
  - Light/sound radius halved → tag `realm == ways` and apply a global `lightRange × 0.5`.
  - Pedestal route generation uses the `2d4 / 2d6 / 2d8` table at trip start; cache for the run.
  - Real-world clock advances by `(timeInWays / 1d4)`; do this once on exit, not gradually.
  - Falling: a single resolver picks `5% lower-bridge` vs. infinite-fall; otherwise `3d6 × 10 ft` with standard falling damage and party split.
  - Shadowspawn encounter: per-trip percentile + sub-table.
  - Machin Shin: encounter check at every Waygate transition and every pedestal departure with the location-specific %, then run as a fixed-stat hazard (Speed 30, area 30 ft, Listen DC 15 detection, escape +10%/round, Will DC 20 / -1d4 Wis & -1d4 Int / round).
- **Avendesora leaves as items:** model each Waygate's two leaves as inventory-like items with a 1-day "vitality" timer when removed; expose `lockGate(side)` and `sealGatePermanently()` operations.
- **Talisman of Growing:** treat as a unique ter'angreal that triggers an Ogier-treesinger script to regrow leaves on a sealed gate.
- **Portal Stones:** each Stone has a list of `(symbol, destinationStoneID, destinationRealm)` edges. The `Use Portal Stone` weave (`the-one-power.md`) consumes a symbol selection and channels into the destination realm.
- **Mirror Worlds catalog:** seed a small registry of named worlds (the book gives none, so this is GM territory). Each has its own region-danger tags from `encounters.md` and its own Stone subgraph.
- **Stedding effect:** add a room flag `noOnePower` that:
  - blocks any cast attempt by characters inside the room (returns a "the True Source slips away" failure),
  - blocks weave effects originating outside whose target is inside the stedding,
  - silences `Tel'aran'rhiod` mirror access from inside.
- **NPC behavior in stedding:** Trolloc and Myrddraal NPCs gain a strong refusal-to-enter AI flag; Darkfriend NPCs gain a discomfort modifier (-2 to social checks while inside).
- **Aelfinn / Eelfinn / Tower of Ghenjei:** model as pluggable scripted scenes, not generic rooms. The Snakes-and-Foxes hint suggests encounter-as-puzzle gameplay; leave the resolver as a custom encounter type rather than wiring rules.
- **Dream world wolves:** when a wolf NPC dies, schedule its `Tel'aran'rhiod` ghost spawn with a respawn-elsewhere timer. Wolfbrothers (`gamemastering.md`) gain access to encounter them.
- **Slayer-class wandering NPC:** support a `roamer` NPC type that traverses the `Tel'aran'rhiod` graph hunting other visitors. Hostile by default.
- **Dream-world cap on wagons/horses:** prevent spawning of mount entities within the `telAranRhiod` realm; mounts brought via gateway become tagged `dreamPersistent` and follow the visitor.
- **Knowledge (arcana) gate:** centralize the DC table — `openWaygate = 22`, `pedestalReadout = 16`, `mapResearch = 20`, `mapFollow = 16 - margin`. Expose as constants so balance tuning is a single edit.
- **Hazard catalog cross-reference:** Machin Shin's `permanent ability drain` plugs into the same drain mechanic used by Draghkar's kiss (`encounters.md`), Wisdom-zero/Int-zero death conditions in the Character Condition Summary.
- **Realm-aware travel mounts:** the existing mounted-combat rules (`combat.md`) need a `realmAllowed` filter so horses block in pure `Tel'aran'rhiod` and dream-only realms.
- **Persistent locked Waygates:** when a Waygate is sealed (both leaves gone), persist it across reboots. Talisman of Growing usage is the only un-seal path.
- **Escape clauses:** for the lost-in-Ways case, schedule a 5%-per-4-hours roll for a wandering Waygate find; on hit, the player walks to a random other Waygate (admin-visible log).
