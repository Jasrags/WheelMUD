---
name: wot-d20-rules
description: Source-of-truth mapping from the printed Wheel of Time d20 rules to WheelMUD's Go schema. Names the book section AND the column/struct field that holds the result. Read-mostly; updates only when a new mechanic ships.
triggers:
  - ability score
  - point buy
  - racial modifier
  - BAB
  - base attack bonus
  - hit dice
  - saves
  - fortitude
  - reflex
  - will
  - feats
  - skill ranks
  - cross-class
  - skill cap
  - the one power
  - saidin
  - saidar
  - weave
  - taint
  - talent
  - channeler
  - initiative
  - AC
  - armor class
  - full attack
  - damage type
  - encumbrance
  - carrying capacity
  - level up
  - experience
---

# wot-d20-rules

## Role

The single place that maps printed WoT d20 rules to our concrete Go
types and SQLite columns. When `internal/creature/`, `internal/chargen/`,
combat verbs, or weave mechanics need to extend, this skill names:

1. The book section the rule comes from.
2. The struct field or column that holds the result.
3. Or marks the rule `not-yet-modeled` so we know the gap exists.

This skill **does not** decide whether to implement a rule (`docs/PLAN.md`
does that) and **does not** write Go.

## Core expertise

- **Ability scores** — point-buy 25 (we use 25; book RAW is different),
  modifier formula `(score - 10) / 2` rounded toward zero,
  `creature.AbilityScore{Current, Max, Inherent}` shape.
- **Races** — Human / Ogier; how `chargen.Background.HeightModIn`
  interacts with race base height.
- **Classes** — `BAB` strings (`"good"` / `"avg"` / `"poor"`),
  saves, hit dice, skill points; channeler flag and source.
- **Feats + skills** — feat slot rules, skill-rank cap = `level + 3`,
  cross-class half-rate (deferred — see
  `chargen_features_followups.md` memory).
- **The One Power** — saidin/saidar split, weave categories, taint,
  talents. Stub now; expand in Phase D.
- **Combat round** — initiative, AC composition, full-attack rules,
  damage types. Stub now; expand in Phase C.

## Approach

When invoked:

1. Identify the rule in question.
2. Open the matching reference file.
3. Cite **both** the book page and the schema location (struct/column).
4. If the rule is `not-yet-modeled`, say so plainly and link to the
   PLAN.md phase that opens it.
5. Never invent values. If the book is silent, the answer is "ask the
   designer" — do not interpolate.

## Clarifying questions

- Is the question about RAW or about our deliberate divergence?
  (e.g. point-buy 25 is intentional non-RAW.)
- Is the rule supposed to ship now, or is it covered by a deferred
  phase?
- Does the answer need a code path or just the rule? (this skill never
  writes Go.)

## Output formats

- **Rule citation** — book section + page when available.
- **Schema mapping** — Go struct field or SQL column.
- **Divergence note** — when our impl departs from RAW, why.
- **Gap marker** — `not-yet-modeled (Phase X)` for unfinished pieces.

## Dependencies

- `wheelmud-architecture` — for the schema this skill maps onto.

## Anti-triggers

- Does NOT decide *whether* to implement a rule.
- Does NOT write Go.
- Does NOT write room/item/mob descriptions.
- Does NOT pick names or flavor.
- Does NOT invent rules the book doesn't have.

## References

- `references/ability-scores-and-modifiers.md` — point-buy 25,
  `creature.AbilityScore` shape, modifier formula.
- `references/races.md` — Human / Ogier; base height + background
  modifier.
- `references/classes-and-bab.md` — BAB / saves / hit dice / skill
  points / channeler flag.
- `references/feats-and-skills.md` — feat slot rules, skill cap,
  cross-class.
- `references/the-one-power.md` — **stub** (Phase D).
- `references/combat-round.md` — **stub** (Phase C).
- `references/encumbrance.md` — Str-keyed carrying-capacity table;
  links to `internal/cmd/encumbrance.go`.
