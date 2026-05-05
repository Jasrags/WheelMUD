---
name: world-builder
description: Translates Wheel of Time geography into `data/world/<continent>/<nation>/<region>/<settlement>/<building>/zone.yaml` content — zones, rooms, exits, items, mob placements, shops, bankers. Honors the column lock-step rules from `wheelmud-architecture` and the voice from `lore-writing`.
triggers:
  - zone
  - room
  - exit
  - sector
  - climate
  - ambient
  - nation
  - region
  - settlement
  - continent
  - level_range
  - reset_interval
  - reset_mode
  - builder
  - shop block
  - banker block
  - coords_auto
  - anchor
  - BFS coords
  - extra_descs
  - door flags
  - item placement
  - mob placement
---

# world-builder

## Role

Take a piece of WoT geography and produce zone YAML that loads cleanly
into the DB on next boot. Stays inside the schema enforced by
`internal/world/loader.go` and `data/world/README.md`. Cross-checks
column lock-step against `wheelmud-architecture`. Pulls voice/idiom
from `lore-writing`.

## Core expertise

- **Hierarchy** — `data/world/<continent>/<nation>/<region>/<settlement>/<building>/zone.yaml`.
  Continents already on disk: `westlands`, `aiel_waste`, `seanchan`,
  `shara`, `tremalking`, `oceans`.
- **zone.yaml schema** — `id`, `name`, `builder`, `level_range`,
  `reset_interval_s`, `reset_mode`, `climate`, `ambient` (per-sector
  phase ambients), plus optional per-mob `shop:` and `banker:` sub-blocks.
- **Room ids** — `<zone-id>:<room-suffix>`, kebab-case both halves.
- **Exits** — direction (N/S/E/W/U/D plus diagonals/inout), to-room id,
  optional door flags, key, lock difficulty, description.
- **Sector + climate** — full sector enum (migration 0025); pick the
  matrix entry that fits the location.
- **Coords anchors** — set `coords_auto: 0` on a small set of anchor
  rooms; the BFS pass fills the rest. Leave most rooms at default
  (auto).
- **Item placement** — typed-stat YAML from migration 0015; weight,
  value (currency string), flags, container parent_item_id.
- **Mob placement** — `mob_templates` (catalog) + `mob_instances`
  (placements), `wander_chance`, optional `shop:` / `banker:`
  sub-block.
- **Reset semantics** — `reset_mode` controls how stale instances
  recycle; `reset_interval_s` paces the area-reset bucket.

## Approach

When invoked:

1. Confirm scope — single room, single building, settlement,
   region, nation? Don't accept a vague "build me a city."
2. Pull voice from `lore-writing` (descriptions) before YAML
   structure (so prose doesn't read as a debug dump).
3. Sketch zone topology first: anchor rooms, exit graph, level
   range, sector mix.
4. Write `zone.yaml`. Validate against `data/world/README.md`.
5. For new columns on `rooms` / `items`, route through
   `wheelmud-architecture` — both the loader-side INSERT and the
   repo `Create` need the column.
6. Confirm at least one `coords_auto: 0` anchor per disconnected
   sub-graph so BFS has a starting point.

## Clarifying questions

- Which continent / nation / region / settlement is the target?
- Player level range?
- Approximate room count?
- Any shops / bankers / scripted mobs?
- Indoor / outdoor mix? (drives sector + ambient choice)
- Does this slot under an existing zone or stand up a new one?

## Output formats

- **zone.yaml** — full file, ready to drop in.
- **Room map** — ASCII directional sketch of the exit graph (just for
  PR review; not committed).
- **Lock-step note** — when adding a column not yet in the schema,
  list every file that needs the change (cite
  `wheelmud-architecture`).

## Dependencies

- `wheelmud-architecture` — column lock-step (`rooms`, `items`),
  loader transactionality.
- `lore-writing` — room/item description voice.
- `data/world/README.md` — schema source of truth.

## Anti-triggers

- Does NOT invent new YAML keys without going through
  `wheelmud-architecture` to add columns + migration.
- Does NOT bypass `coords_auto` for entire zones (anchors only).
- Does NOT write combat or quest scripting (Phase C / E content
  skills).
- Does NOT build mob templates without
  `mob-designer`'s subtype catalog (deferred until Phase E).
- Does NOT pick zone names without `lore-writing`.

## References

- `references/wot-geography-master.md` — **stub**, port from prior
  session.
- `references/room-description-style.md` — second-person present,
  sensory layering, length.
- `references/zone-yaml-cheatsheet.md` — links to
  `data/world/README.md`; flags the dual raw-SQL / Create column
  lock-step.
- `references/sector-and-climate-matrix.md` — which combos make
  sense.
- `references/coords-anchor-rules.md` — when to set `coords_auto: 0`,
  how the BFS pass fills the rest.

## Agents

- `agents/room-writer.md` — produces room descriptions to spec.
- `agents/zone-planner.md` — plans room counts, exit topology,
  anchor placement.
