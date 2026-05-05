# zone.yaml cheatsheet

The full schema lives in `data/world/README.md`. This file flags the
gotchas world-builders trip on most.

## Required top-level keys

- `id` — kebab-case, unique across all zones.
- `name` — display name.
- `builder` — credit.
- `level_range` — e.g. `"1-5"`.
- `reset_interval_s` — area-reset cadence in seconds.
- `reset_mode` — see `data/world/README.md`.
- `climate` — sectorial; drives ambient defaults.

## Optional

- `ambient` — per-sector phase ambient strings (overrides defaults).
- `rooms`, `exits`, `items`, `mob_templates`, `mob_instances` — the
  content body.

## Per-mob optional sub-blocks

```yaml
mob_templates:
  - id: caemlyn:innkeeper-merrilor
    shop:
      sell_markup: 1.20
      buy_markdown: 0.50
      buy_types: [drink, food, tradegood]
      stock:
        - { item_external_id: caemlyn:ale-mug, qty: -1, qty_max: -1 }
    banker:
      hours_open: 8
      hours_close: 20
```

Either or both blocks may be present. Shop with infinite stock uses
sentinel `qty: -1, qty_max: -1`.

## Column lock-step (CRITICAL)

`internal/world/loader.go` writes raw SQL inside one transaction. New
columns on `rooms` / `items` / `characters` need to land in **both**
the loader column list **and** the typed repo `Create` lists in the
same PR. See `wheelmud-architecture/references/repo-and-migration-rules.md`.

## Currency strings

`"5g 3s 7c"` — gold / silver / copper. Parse via `internal/coin/`.

## Item stat shape

Typed JSON columns from migration 0015 — weapons, armor, containers,
consumables each have a fixed schema. Don't free-form stat data into
description text.
