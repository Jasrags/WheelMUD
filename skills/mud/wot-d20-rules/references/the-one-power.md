# The One Power — STUB (Phase D)

This reference is a placeholder. The channeling subsystem is not yet
implemented; expand this file when Phase D opens and the weave catalog
ships.

## What exists today

- `creature.Channeling` struct (introduced in migration 0008) —
  carries the channeler's strength rating, taint, talents.
- `chargen/default/weaves.yaml` — catalog of weaves, validated at
  load time but not yet exposed in chargen UI.
- `wot-d20-rules` triggers on saidin/saidar/weave/taint/talent so
  questions land here even though the answer is mostly
  "not-yet-modeled."

## What to flesh out (Phase D)

- Saidin / saidar split — gender gate (see `races.md`).
- Strength rating scale and how it interacts with weave DC.
- Weave categories (the five Powers: Air, Earth, Fire, Spirit, Water,
  plus weaves that combine them).
- Taint mechanics for male channelers (pre-Cleansing era choice if
  the MUD's setting predates Knife of Dreams).
- Talent slots (Healing, Foretelling, Dreamwalking, Cloud Dancing,
  Earth Singing, Travelling, Skimming, Wolfbrother, ...).
- Burning out / stilling / gentling.
- Linking (circle formation rules, gender ratios, leader rules).

## Don't invent

If Phase D hasn't opened yet, the answer to "how does X work in our
schema" for any of the above is: **not-yet-modeled, blocked on Phase
D**. Do not interpolate from training data — the source of truth is
the printed WoT d20 sourcebook plus our explicit divergences.
