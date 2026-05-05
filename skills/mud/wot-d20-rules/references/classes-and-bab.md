# Classes and BAB

WoT d20 ships 7 base classes (Algai'd'siswai, Armsman, Initiate,
Noble, Wanderer, Wilder, Woodsman). Each has a BAB progression, save
profile, hit die, skill points, and optional channeler flag.

## Schema mapping

- `creature.Class` enum — `internal/creature/class.go`.
- `Character.Class` — stamped at chargen. Multiclassing
  `not-yet-modeled` (Phase C/D).
- `chargen.Class` (the catalog struct) carries:
  - `BAB` — `"good"` / `"avg"` / `"poor"`
  - `HitDie` — d6 / d8 / d10
  - `Saves` — which of Fort/Ref/Will is "good"
  - `SkillPoints` — base per level (× 4 at 1st)
  - `Channeler` — bool; `true` for Initiate / Wilder

## BAB progression

| Tier | Per level |
|---|---|
| good | +1 |
| avg | +0.75 (rounds: 0,1,2,3,3,4,5,6,6,7,...) |
| poor | +0.5 (rounds: 0,0,1,1,2,2,3,3,4,4,...) |

Multiple attacks at +6, +11, +16 BAB per RAW. Combat round
implementation `not-yet-modeled` (Phase C).

## Saves

Good save: `2 + level/2`. Poor save: `level/3`. Cross-class items
can boost.

## Hit dice

- d6 — Initiate, Wilder, Noble (channeler/social classes)
- d8 — Wanderer, Woodsman, Algai'd'siswai
- d10 — Armsman

1st level = max die + Con mod. Subsequent levels roll (we'll
likely use average + 1 to keep MUD progression deterministic; not
yet decided — `not-yet-modeled`, decision belongs to Phase D).

## Skill points

`(class base + Int mod) × 4` at 1st level, `class base + Int mod`
each subsequent level. Minimum 1/level after racial/Int mods.

Cross-class skills cost 2 ranks per point (deferred — see
`chargen_features_followups.md`).

## Channeler flag

Initiate (Aes Sedai trainee) and Wilder (untrained channeler) have
`Channeler: true`. Triggers the Phase-D channeling branch in
chargen and adds saidin/saidar selection. Currently the channeling
branch in chargen is `not-yet-modeled` — feature slot lands in
chargen but the channeling pool/talents don't.
