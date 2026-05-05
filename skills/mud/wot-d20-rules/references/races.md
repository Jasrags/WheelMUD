# Races

V1 ships Human and Ogier. (Trolloc / Myrddraal / etc. are mob-only;
not playable.)

## Schema mapping

- `creature.Race` enum — `RaceHuman`, `RaceOgier` (see
  `internal/creature/race.go`).
- `Character.Race` — stamped at chargen, immutable thereafter.

## Human

- Base height ~5'6" (male) / ~5'2" (female).
- Base weight tracks height per book table.
- No racial ability modifier.
- 1 bonus feat at 1st level (RAW). **Not yet wired into chargen** —
  chargen #15 slice 1 grants the standard 1st-level feat but does not
  add the human-extra slot. See `chargen_features_followups.md`.

## Ogier

- Base height ~9'-10'.
- Substantially heavier.
- +2 Str / -2 Dex (RAW). **Not yet applied** — see
  `ability-scores-and-modifiers.md` channeler/racial-mod note.
- Restricted backgrounds: Ogier-specific backgrounds are not yet in
  the chargen catalog. See `chargen_identity_followups.md` memory
  ("ogier-race backgrounds").

## Background height/weight modifier

`chargen.Background.HeightModIn` is added to race base height in
chargen identity rendering. Weight scales proportionally per book
(loose tracking, not strict).

## Channeler gender gate

In WoT, only women channel saidar; only men channel saidin. The
chargen channeler branch must check character gender before allowing
the channeler flag. **Not yet enforced** in chargen — see
`chargen_identity_followups.md` ("channeler gender gate").
