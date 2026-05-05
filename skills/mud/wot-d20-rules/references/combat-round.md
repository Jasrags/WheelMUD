# Combat round — STUB (Phase C)

Placeholder. Combat is not yet implemented; expand when Phase C opens.

## What exists today

- `creature.Core` carries HP, AC components, damage resistances,
  position flags, conditions.
- The move family + door verbs ship; combat verbs do not.
- BAB strings on classes are persisted but not yet consulted by any
  attack roll.

## What to flesh out (Phase C)

- **Initiative** — d20 + Dex mod, ties broken by Dex score.
- **AC composition** — 10 + armor + shield + Dex (capped by armor) +
  size + dodge + natural + deflection.
- **Attack roll** — d20 + BAB + Str (or Dex for finesse) + size +
  misc.
- **Full attack** — second attack at -5 BAB starting at +6 BAB; third
  at +11; fourth at +16.
- **Damage types** — slashing / piercing / bludgeoning; channeling
  damage types from weaves.
- **DR** — `creature.Core` already has the field; not yet consulted.
- **Saves** — Fort (Con-based, poison/disease/death), Ref (Dex,
  area), Will (Wis, mind/charm).
- **Conditions** — flat-footed, prone, helpless, fatigued, exhausted.
- **Special moves** — trip, disarm, grapple, sunder; Warder bond
  bonuses.

## Damage source naming

When Phase C lands, weapons in `data/world/**/items.yaml` should carry
typed damage stats (already in the migration 0015 taxonomy). Cross-
reference the weapon catalog when designing: do not invent damage
dice that aren't already in the YAML.
