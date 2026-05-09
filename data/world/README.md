# World Builder Guide

This directory holds the YAML world definition. The loader at
`internal/world/loader.go` walks the tree once on first boot, validates
every zone, and inserts rooms / exits / items / mobs into SQLite. The
runtime never reads YAML again — to pick up changes, wipe the DB
(`rm wheelmud.db` or `DB_DSN=:memory:`) and restart.

## Hierarchy

The directory tree mirrors in-world geography:

```
data/world/
├── westlands/                   # continent (Randland)
│   ├── <nation>/                # Andor, Ghealdan, Cairhien, …
│   │   └── <region>/            # e.g. two_rivers
│   │       └── <settlement>/    # e.g. emonds_field
│   │           └── <building>/  # e.g. winespring_inn  (interior sub-zone)
│   └── shared/                  # cross-nation geography (Mountains of Mist, etc.)
├── aiel_waste/                  # peer continent-roots
├── seanchan/
├── shara/
├── tremalking/
└── oceans/                      # connective tissue between landmasses
    ├── aryth_ocean/
    └── sea_of_storms/
```

**A "zone" is any directory containing a `zone.yaml`.** Pure
organisational directories (e.g. `westlands/`, `westlands/andor/`) have
no `zone.yaml` and are skipped by the loader.

## Files in a zone

Each zone directory may contain:

| File         | Required | Purpose                                       |
|--------------|----------|-----------------------------------------------|
| `zone.yaml`  | yes      | Metadata: id, name, builder, level range, reset, climate, ambient |
| `rooms.yaml` | yes      | Sequence of rooms (≥1)                        |
| `items.yaml` | no       | Sequence of items spawned in this zone        |
| `mobs.yaml`  | no       | Sequence of NPC spawns in this zone           |

### `zone.yaml`

The persisted zones row drives §9 area resets, §10 ambient/weather,
§16 builder permission scoping, and the `zones` admin command. Only
`id` and `name` are required; everything else has a documented default
applied at insert time.

```yaml
id: emonds_field                   # required — globally unique slug, ASCII, no whitespace
name: Emond's Field                # required — human-readable display name

builder: jrags                     # default ""    — author tag (§16 permission scoping)
level_range: { min: 1, max: 5 }    # default 1..60 — advisory content gating
reset_interval_s: 900              # default 600   — §9 areaReset bucket cadence (seconds)
reset_mode: empty                  # default empty — always | empty | never
climate: temperate                 # default ""    — §10 ambient/weather hint
ambient:                           # default []    — §10 ambient ticker rotates these
  - The smell of fresh bread drifts across the green.
  - A pair of doves break cover from the inn roof.
```

**Reset modes:**
- `always` — reset every interval regardless of who is in the zone
- `empty` — reset only when no players are present (default)
- `never` — disable resets entirely (use for stub zones, player housing)

**Inspect persisted state at runtime:**
```
zones                  # list every zone (admin)
zones show emonds_field
```

### `rooms.yaml`

Sequence of room objects. **Exactly one room across the entire world**
must be marked `starter: true` (currently `tr.emonds_field.green`).

```yaml
- id: tr.emonds_field.green
  starter: true                       # at most one in the world
  name: Emond's Field — the Green
  short: The heart of the Two Rivers, before the Winespring Inn.
  long: |
    Multi-line description of the room as players see it on `look`.
  exits:
    n: tr.emonds_field.forge_yard     # shorthand: target room id
    e:                                 # full form, for doors/locks
      to: tr.emonds_field.winespring_inn.common
      closed: true
      locked: true
      key: tr.iron_key
      difficulty: 15
      description: A heavy oak door bound with iron.
  flags:
    indoors: false
    nopvp: false
    noteleport: false
    dark: false
    silent: false
    peaceful: true
  sector: city                        # see Sectors below
  light_level: 7                      # 0=pitch dark, 10=full day; default 10
  coords: { x: 0, y: 0, z: 0 }        # optional, used by §10 map/track
  descriptions:                       # `look <keyword>` extras
    oak: |
      The old oak at the centre of the green is broader than three men…
```

### `items.yaml`

```yaml
- id: tr.spring_dipper                 # globally unique slug
  room: tr.emonds_field.winespring     # must reference an existing room id
  name: a wooden dipper                # short article-prefixed display name
  short: a wooden dipper, paled with use, hangs from a peg beside the spring
  type: container                      # see Item types below
  weight: 0.5                          # pounds; non-negative
  value: 2cp                           # currency string — see below
  quality: normal                      # normal | masterwork | masterpiece | power_wrought
  flags:                               # see Item flags below
    - notake
  stats:                               # type-specific sub-block; see below
    capacity_lbs: 1.0
    liquid_pints: 0.5
```

### `mobs.yaml`

```yaml
- id: tr.bran_alvere                   # globally unique slug
  room: tr.emonds_field.winespring_inn.common
  name: Bran al'Vere
  short: the round-faced mayor of Emond's Field stands behind the bar, drying a tankard
```

The v1 loader spawns each YAML mob as a one-of-a-kind template plus a
single instance into the named room. Stat blocks (HP, Defense, etc.)
get safe defaults — full template authoring lands later.

Optional death-loot fields (Phase D §19 polish):

```yaml
- id: tr.trolloc_grunt
  room: tr.blight.scout_camp
  name: a trolloc warrior
  short: a hulking shadowspawn with matted black fur
  gold_dice: "2d10"     # rolled at death → coin pile inside the corpse
  xp_value: 600         # overrides the ChallengeCode → XP table when > 0
```

`gold_dice` is parsed via `combat.rollDice` (`NdM` or `NdM±K`). On a
successful roll the result spawns a single `ItemTypeTradeGood` "a
small pile of coins" inside the corpse with `Value` set to the rolled
copper amount and `FlagTradeGood` set so the shop verbs sell it back
at full price. Empty / malformed strings produce no pile.

`xp_value > 0` is the absolute XP awarded on this template's death
(before damage-tally weighting + group split). `xp_value == 0` (the
default) falls back to the hard-coded ChallengeCode A→I curve in
`combat.xpValueForChallenge`.

#### Shopkeeper sub-block

A mob entry may carry an optional `shop:` block to mark the mob as a
§14 shopkeeper. The loader inserts a `shops` row keyed to the mob
template plus one `shop_stock` row per `stock` line.

```yaml
- id: tr.bran_alvere
  room: tr.emonds_field.winespring_inn.common
  name: Bran al'Vere
  short: the round-faced mayor of Emond's Field stands behind the bar
  shop:
    buy_types: [food, consumable, trade_good]   # ItemTypes the shop will buy back
    sell_markup: 1.0          # buy price = item.value * sell_markup (default 1.0)
    buy_markdown: 0.5         # sell price = item.value * buy_markdown for non-trade_goods (default 0.5)
    open_hour: 6              # 0..23. open_hour == close_hour means always open
    close_hour: 23            # close < open wraps midnight (e.g. 22→4 covers a tavern's late hours)
    restock_interval_s: 600   # per-line refill cadence (default 3600)
    stock:
      - item: tr.inn_ale      # external_id of an item template defined elsewhere
        qty: 12
        qty_max: 12
      - item: tr.kitchen_loaf
        qty: -1               # qty=-1 + qty_max=-1 → infinite stock (staple goods)
        qty_max: -1
```

Notes:

- Stock items must be defined as ordinary `items.yaml` entries
  somewhere in the world (placing them on the shopkeeper's room
  floor doubles as a visible "this is on the menu" cue). `buy`
  materializes a fresh copy in the buyer's inventory cloned from
  the template.
- `FlagTradeGood` items always sell back at full Value (the WoT
  trade-good rule); the half-price rule only applies to other types.
- `FlagNoSell` items are refused at sell-time regardless of
  `buy_types`.
- Items not in the shop's `buy_types` whitelist are refused at sell.
- Stock with `qty_max < 0` is infinite — the restocker leaves it
  alone, and `buy` doesn't decrement it.

A mob entry may also carry an optional `banker:` block to mark the
mob as a §14 banker. The loader inserts a `bankers` row keyed to the
mob template. V1 carries operating hours only — no fees, no
min-deposit, no item vault. Coin moves between the character's purse
and `characters.bank_balance` via the `balance` / `deposit` /
`withdraw` verbs.

```yaml
- id: tr.padan_fain
  room: tr.emonds_field.winespring_inn.common
  name: Padan Fain
  short: a sharp-eyed peddler tallies coin in a small leatherbound ledger
  banker:
    open_hour: 8     # optional, 0..23; default 0
    close_hour: 20   # optional, 0..23; default 0
```

Banker hours follow the same rules as shop hours:

- Both endpoints are integers in `[0, 23]`. There is no `24` —
  midnight is `0`.
- `open_hour == close_hour` is the **always-open sentinel**. The
  default `0/0` therefore means "24 hours". If you want a banker
  open from 8am to midnight, write `open_hour: 8, close_hour: 0`.
- Otherwise the banker is open for `[open_hour, close_hour)`. A
  wrap (e.g. `open: 22, close: 4`) covers a late-night window.

A mob entry may also carry an optional `weave_teacher:` block to mark
the mob as a Phase E #28 mid-game weave teacher. The loader inserts a
`weave_teachers` row keyed to the mob template. With a teacher in the
room, `learn weave` drains `characters.practice_points` (1 PP/level
earned at level-up) per weave's `practice_cost` instead of the
chargen `pending_weaves` pool. V1 has no fees, no time cost, and no
outside-affinity learning — the teacher's offerings intersect with
the channeler's own affinities.

```yaml
- id: tr.aes_sedai_anaiya
  room: tr.tar_valon.tower.green_ajah
  name: Sister Anaiya
  short: a kindly Aes Sedai of the Green Ajah
  weave_teacher:
    max_level_taught: 1     # 0..9, the highest weave level offered
    affinity_filter:        # optional; empty = teach any in-affinity
      - air                 # Power names: air, earth, fire, water, spirit
      - fire
```

Teacher rules:

- `max_level_taught` is `[0, 9]`. The chargen catalog is level-0 only
  today, so 0 is the conventional value; the column is in place for
  when level-1+ weaves are authored.
- `affinity_filter` is a list of Power names. An empty (or absent)
  list means "teach any weave the channeler can learn from her
  Affinities." A non-empty list restricts the teacher to those
  Powers; the verb still intersects with the channeler's own
  Affinities.
- A mob can be both a class trainer and a weave teacher
  simultaneously by carrying both `trainer:` and `weave_teacher:`
  blocks.

### Dialogue trees

A mob entry may also carry an optional `dialogue:` block to attach a
branching conversation that players reach via `talk <mob>` (Phase F
#30). The loader translates the YAML into the canonical
`internal/dialogue.Tree` shape and persists it on
`mob_templates.dialogue_json`.

```yaml
- id: tr.elder
  room: tr.commons.green
  name: village elder
  short: an aged village elder
  dialogue:
    root: greet
    nodes:
      - id: greet
        prompt: "Greetings, traveler. What brings you to Emond's Field?"
        responses:
          - match: [hello, hi, greetings]
            reply: "Well met."
            next: menu
          - match: [bye, leave, farewell]
            reply: "Travel safely."
            effects:
              - kind: end
      - id: menu
        prompt: "Did you need something?"
        responses:
          - match: [quest, work]
            reply: "Then take this charge — find the lost lamb on the Westwood path."
            effects:
              - kind: set_flag
                args:
                  name: lamb_quest_started
            next: menu
          - match: [reward]
            reply: "Your service is noted, friend."
            show:
              require_flag: lamb_quest_returned
            next: greet
          - match: [shop]
            label: "see your wares"
            effects:
              - kind: push_mode
                args:
                  mode: shop
```

Dialogue rules:

- `root` is the starting node id; it must exist in `nodes`.
- Each node has a unique `id`, a `prompt`, and an ordered `responses`
  list. Empty `nodes` or a dangling `next` reference fails the boot
  loudly.
- `match` is a list of case-insensitive substring keywords. The
  player can also pick by typing the response number (1-based on the
  visible list).
- `label`, when set, overrides the numbered-list label (otherwise the
  first `match` keyword wins, then the first sentence of `reply`).
- `next` advances to the named node. Empty `next` ends the
  conversation, equivalent to an `end` effect.
- `effects` fire in order before `next` is followed. Effect kinds:
  `set_flag` / `clear_flag` (per-session flag bag — drops on
  logout), `goto` (overrides `next`), `push_mode` (hand off to a
  sibling mode; nil in V1, log-and-noop), `end` (pop the dialogue),
  `accept_quest` / `advance_quest` (Phase F #31 — `args.quest_id`
  enrolls or advances a talk_to quest step; quest definitions live
  under `internal/quest/default/<id>.yaml`, see that directory's
  README for the schema).
- `show` gates a response's visibility on the per-session flag bag.
  `require_flag` keeps the response hidden until the flag is set;
  `forbid_flag` hides it once the flag is set.

## Conventions

### Room ID naming

Room IDs are global and must be unique across the whole world. Use a
dotted, hierarchical convention so origin is obvious from the ID alone:

| Scope                       | Pattern                                  | Example                                    |
|-----------------------------|------------------------------------------|--------------------------------------------|
| Two Rivers village interior | `tr.<village>.<spot>`                    | `tr.emonds_field.green`                    |
| Two Rivers building interior| `tr.<village>.<building>.<room>`         | `tr.emonds_field.winespring_inn.common`    |
| Two Rivers wilderness       | `tr.<feature>` or `tr.<feature>.<spot>`  | `tr.westwood.north`, `tr.mire`             |
| Cross-nation shared geo     | `shared.<feature>.<spot>`                | `shared.mountains_of_mist.foothills`       |
| Nation/region stubs         | `stub.<area>` or `<area>.gateway`        | `stub.ghealdan`, `seanchan.gateway`        |
| Oceans                      | `ocean.<sea>.<spot>`                     | `ocean.aryth.open_water`                   |

**Rules:**
- ASCII only, no whitespace, no characters ≤ 0x20 or ≥ 0x7F.
- Lowercase, underscore-separated within a segment.
- Stable forever. IDs are referenced by exits, items, mobs, persisted
  character `current_room_id`, and quest scripts.

### Zone IDs

Zone IDs (the `id:` in `zone.yaml`) must also be globally unique and
follow the same charset rules. Convention: snake_case, prefix with
nation/region when ambiguous (e.g. `andor_caemlyn`,
`shared_mountains_of_mist`, `ocean_aryth`).

#### Settlement / building hierarchy

The world tree is `continent → nation → region → settlement → building`
(see **Hierarchy** above), and **each named building is its own zone**,
not a room inside the settlement zone. This keeps reset cadence,
ambient lines, builder ownership, and mob/item spawn lists scoped to
the right granularity:

- A **settlement zone** owns the outdoor streetscape (the green, lanes,
  market, fields just outside the wall, etc.) and any unnamed
  one-room interiors that don't deserve a zone of their own.
- A **building zone** owns the interior of one named building
  (inn, smithy, mill, temple, manor) plus any tightly-bound annex
  rooms (cellar, stable yard, upstairs hall).

Use the **settlement_building** zone-id prefix so `zones list` groups
the children visually beneath their parent settlement:

```
emonds_field                      ← settlement zone (Green, lanes, fields)
emonds_field_winespring_inn      ← building zone (taproom, kitchen, rooms)
emonds_field_luhhans_forge       ← building zone (smithy, yard)
emonds_field_thanes_mill         ← building zone (mill floor, loft)
```

The settlement zone owns the doored exits *into* each building; the
building zone owns the corresponding return exit. Both sides reference
each other via cross-zone exits (see **Cross-zone exits** below).

When a building is small enough that it doesn't warrant its own zone
(a single-room shop, a watchman's hut), put it in the settlement zone
as a regular room and skip the building-zone split. The rule of thumb:
**carve out a zone if the building has 3+ interior rooms, distinct
reset cadence, distinct ownership, or its own mob/item spawn list.**

Building zones inherit the settlement's `level_range` by default; bump
them per-zone when the interior has higher-tier content (e.g., a
basement crypt under a temple, a noble's strongroom).

### Directions

Valid exit directions:

```
n s e w u d ne nw se sw
```

Anything else fails validation.

### Sectors

Valid `sector:` values (default `city` if omitted):

```
city forest field hills mountain desert water underwater air underground
blight waste stedding swamp
```

The four extension sectors (`blight`, `waste`, `stedding`, `swamp`) were
added by migration 0025 for Wheel-of-Time-flavored terrain. See
`docs/wot_geography_mud.md` for region-by-region guidance on which sector
to pick. Mechanical hooks for each (e.g., channeling suppression in
stedding, ambient horror in blight) land in later phases — today these
sectors render their own phase ambient lines but otherwise behave like
generic outdoor terrain.

### Currency strings

`value:` is parsed by `internal/currency/Parse`. **No internal
whitespace between the number and the denomination.**

| Denomination | Code | Example |
|--------------|------|---------|
| Copper penny | `cp` | `5cp`   |
| Silver penny | `sp` | `1sp`   |
| Silver mark  | `mk` | `2mk`   |
| Gold crown   | `gc` | `1gc`   |

Multi-coin: space-separated, **each denomination at most once**.

```yaml
value: 1gc 2mk 3sp 4cp     # OK
value: 5sp 5sp             # FAIL — sp repeated
value: 2 cp                # FAIL — embedded space
value: 50                  # OK — bare integer = copper pennies
```

Empty / omitted `value:` is treated as zero.

### Item types

| Type         | Has stats block? | Stats struct                 |
|--------------|------------------|------------------------------|
| `weapon`     | yes              | `repo.WeaponStats`           |
| `armor`      | yes              | `repo.ArmorStats`            |
| `shield`     | yes              | `repo.ShieldStats`           |
| `container`  | yes              | `repo.ContainerStats`        |
| `consumable` | yes              | `repo.ConsumableStats`       |
| `light`      | yes              | `repo.LightStats`            |
| `key`        | yes              | `repo.KeyStats`              |
| `tool`       | yes              | `repo.ToolStats`             |
| `clothing`   | **no**           | rejected if `stats:` present |
| `food`       | **no**           | rejected if `stats:` present |
| `trade_good` | **no**           | rejected if `stats:` present |
| `trash`      | **no**           | rejected if `stats:` present |

The `stats:` block is **flat** — fields go directly under `stats:`, not
nested under the type name.

```yaml
# CORRECT
type: weapon
stats:
  proficiency: simple
  size: medium
  range: melee
  damage: "1d6"
  damage_type: [B]

# WRONG — no `weapon:` wrapper
type: weapon
stats:
  weapon:
    damage: "1d6"
```

`damage_type` is a list of single-letter codes: `B` (bludgeoning),
`P` (piercing), `S` (slashing).

For typed items, an empty / omitted `stats:` block is allowed and
yields a zero-valued struct — handy for stubbing items during
authoring.

### Item flags

```
notake nodrop nosell bind_on_pickup magic glow hum trade_good
```

Snake-case only. Unknown names fail validation.

### Cross-zone exits

Exits are validated against the global room set, so you can freely
point an exit in one zone at a room defined in another zone. The
loader doesn't care which file the target lives in.

## Stub zones

Areas with no authored content (most nations, distant continents) ship
as a single sentinel room flagged unreachable:

```yaml
flags:
  noteleport: true
  peaceful: true
```

This lets you reference them in lore and exit descriptions without
committing real content. Replace the sentinel with real rooms when the
area is built out — the room IDs do not need to remain.

## Adding a new zone

1. Pick a directory under the appropriate hierarchy point.
2. Create `zone.yaml` with a unique `id` and human `name`.
3. Create `rooms.yaml` with at least one room. Wire its exits to
   neighbouring zones if it should be reachable from existing content.
4. Optional: `items.yaml`, `mobs.yaml`.
5. Wipe the DB (`rm wheelmud.db`) and restart the server. The loader
   logs `world: load complete zones=N rooms=M …` when validation passes.
6. Validation errors print as `<file>:<line>: <message>` so you can
   jump straight to the YAML.

## Reference template

`westlands/andor/two_rivers/emonds_field/` is the worked example:
village exterior + three building interior sub-zones, with item types,
stats blocks, currency values, NPC spawns, keyword `descriptions:`,
sector + lighting, and bidirectional cross-zone exits.
