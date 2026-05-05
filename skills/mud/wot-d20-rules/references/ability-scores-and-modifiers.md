# Ability scores and modifiers

## Scores

Six abilities: Str, Dex, Con, Int, Wis, Cha. Range 3–18 at character
creation; can rise above with level/items/inherent boosts.

## Point-buy budget

**We use 25 points** (chargen `internal/chargen/abilities.go`). Book
RAW differs (commonly 28 / 32 by tier). This is a deliberate
divergence — our 25-point pool is balanced for a tighter starting
spread.

Cost table (V1, see `chargen_abilities_followups.md` memory for
deferred alternates):

| Score | Cost |
|---|---|
| 8 | 0 |
| 9 | 1 |
| 10 | 2 |
| 11 | 3 |
| 12 | 4 |
| 13 | 5 |
| 14 | 6 |
| 15 | 8 |
| 16 | 10 |
| 17 | 13 |
| 18 | 16 |

## Modifier formula

`mod = (score - 10) / 2`, rounded toward negative infinity (i.e.
floor). 8 → -1, 9 → -1, 10 → 0, 11 → 0, 12 → +1, 13 → +1, 14 → +2, ...

## Schema mapping

- `creature.AbilityScore{Current, Max, Inherent}` — three-part struct
  so racial / item / drain effects can be modelled separately.
  - `Inherent` = base from chargen + permanent boosts (tomes,
    inherent stat increases at levels 4/8/12/16/20).
  - `Max` = `Inherent` + permanent enhancement (rarely changes).
  - `Current` = `Max` minus temporary penalties (drain, ability damage).
- `creature.Core` holds the six `AbilityScore`s.

## Racial modifiers

Not yet wired into chargen — see
`chargen_abilities_followups.md` memory ("racial mods deferred").
Ogier base is +2 Str / -2 Dex per book; not applied yet.

## Channeler floor

A character with the channeler flag should have a minimum ability
score in the channeling stat (saidin → Str-of-channel scoring varies
by source; track in `internal/creature/channeling.go` when Phase D
opens). Currently `not-yet-modeled` — chargen accepts any legal
point-buy distribution.
