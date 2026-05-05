# Feats and skills

## Feat slots

- 1 feat at 1st level (every character).
- +1 feat every 3 levels thereafter (3rd, 6th, 9th, ...).
- Human bonus feat at 1st level — **not yet wired** (see
  `races.md`).
- Class bonus feats (e.g. Armsman fighter-style chains) —
  `not-yet-modeled` (Phase D — level-up).

## Schema mapping

- `chargen.Feat` (catalog) — `internal/chargen/feats.go`.
- `Character.Feats` — JSON list of feat ids.
- Feat slot count derives from level + race + class; no separate
  column.

## Skill ranks

- Cap: `level + 3` for class skills.
- Cross-class cap: `(level + 3) / 2`, costs 2 points per rank.
- Cross-class is **deferred** to level-up (see
  `chargen_features_followups.md`). Chargen V1 only buys class
  skills.

## Schema mapping

- `chargen.Skill` (catalog) — `internal/chargen/skills.go`.
- `Character.SkillRanks` — JSON `map[string]int` keyed by skill id.
- `Character.Skills` derives mod from rank + ability mod + misc.

## Talents

Talents are channeler-only weave-flavor specialisations
(`not-yet-modeled` until Phase D). The chargen catalog carries the
list under `weaves.yaml` but the choice slot doesn't render yet.

## Background-driven skill grants

Each `chargen.Background` carries a `Skills` list (free ranks) and
a `Feats` list (free feat slot). These bypass the normal point
budget. See `chargen_bg_class_followups.md` for label-alignment
followups.
