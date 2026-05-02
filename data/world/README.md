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
| `zone.yaml`  | yes      | Metadata: `id` + `name`                       |
| `rooms.yaml` | yes      | Sequence of rooms (≥1)                        |
| `items.yaml` | no       | Sequence of items spawned in this zone        |
| `mobs.yaml`  | no       | Sequence of NPC spawns in this zone           |

### `zone.yaml`

```yaml
id: emonds_field        # globally unique slug, ASCII, no whitespace
name: Emond's Field     # human-readable display name
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
```

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
