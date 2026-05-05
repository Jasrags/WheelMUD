# Chargen flow skin

The action-reference. Each substep names the file:line range, the current
look, the proposed skin, and any callouts.

## Substep map (current code)

| # | Step | File | Lines (approx) |
|---|---|---|---|
| 1 | Name | `character_create.go` | 280-303 |
| 2 | Race | `character_create.go` | 305-315 |
| 3 | Background | `character_create.go` | 317-403 |
| 4 | Class | `character_create.go` | 423-494 |
| 5 | Abilities | `character_create.go` | 496-617 |
| 6 | Identity | `chargen_identity.go` | full file |
| 7 | Feats / Skills | `chargen_features.go` | full file |
| 8 | Review / commit | `character_create.go` | 649-720 |

## Per-step skin

### Step 1 — Name

Bare prompt today. Skin:

```
─── {{Step 1/8 — Choose a name}}::cyan|bold ──────────────────────────

  {{Name}}::yellow|bold must be 3-24 letters, unique, not a reserved word.
  ({{back}}::gray and {{cancel}}::gray are reserved.)

{{[step 1/8 name]}}::cyan »
```

On invalid input, errors via `writeError`, then re-prompt.

### Step 2 — Race

```
─── {{Step 2/8 — Race}}::cyan|bold ───────────────────────────────────

  {{1.}}::gray  human   — Common across the Westlands.
  {{2.}}::gray  ogier   — Long-lived, builders of the steddings.

{{[step 2/8 race]}}::cyan »
```

### Step 3 — Background (`character_create.go:317-403`)

Today:

```
Backgrounds:
   1. andoran          Andoran Commoner       Common (1 feats, 2 skills, 2 outfits)
   2. cairhienin       Cairhienin Noble       Common (0 feats, 3 skills, 1 outfits)
Type 'info <id|#>' for full details.
```

Skin (80 cols):

```
─── {{Step 3/8 — Background}}::cyan|bold ─────────────────────────────

  {{ #}}::gray  {{id              }}::yellow|bold  {{name                  }}::yellow|bold  summary
   {{1}}::gray  andoran          Andoran Commoner       Common · 1 feats, 2 skills, 2 outfits
   {{2}}::gray  cairhienin       Cairhienin Noble       Common · 0 feats, 3 skills, 1 outfits

  Choose by {{<id>}}::yellow or {{<#>}}::yellow, {{info <id>}}::yellow to inspect, {{back}}::gray to revise.
{{[step 3/8 background]}}::cyan »
```

Skin (60 cols, stacked):

```
─── {{Step 3/8 — Background}}::cyan|bold ───────────────────

  {{1.}}::gray {{andoran}}::yellow|bold — Andoran Commoner
       Common · 1 feats, 2 skills, 2 outfits

  {{2.}}::gray {{cairhienin}}::yellow|bold — Cairhienin Noble
       Common · 0 feats, 3 skills, 1 outfits

  {{<id>}}::yellow / {{<#>}}::yellow / {{info <id>}}::yellow / {{back}}::gray
{{[step 3/8 background]}}::cyan »
```

### Step 3a — Background info (`:366-403`)

The full descriptor. Today: bare labels, prose dumped via `WriteRaw`.

Skin: every label through `writeFieldRow` (14-col gutter); description
through `WriteWrapped`; opening + closing `writeRule(s, "")` divider.

```
─── {{andoran (Andoran Commoner)}}::cyan|bold ────────────────────────

  {{Home language}}::yellow|bold  Common
  {{Bonus feats}}::yellow|bold    Athletic
  {{Background skills}}::yellow|bold Climb, Jump
  {{Height mod}}::yellow|bold     +0 in

  {{Equipment options}}::yellow|bold
    {{1.}}::gray  Sword + leather armor
    {{2.}}::gray  Bow + 20 arrows + dagger

  Stout Andoran stock; Two Rivers folk and Caemlyn citizens alike count
  themselves Andoran-born. They favor the bow and a plain word over a
  fancy one.

──────────────────────────────────────────────────────────────────────
```

### Step 4 — Class (`:423-494`)

Same pattern as background. Class info adds a **channeler accent** when
`cl.Channeler` is true:

```
{{Channeler}}::yellow|bold      yes ({{saidin}}::blue|bold) — male
```

(Replace `saidin` with `saidar` and `blue|bold` with `white|bold` on
female-channeler classes when the catalog grows beyond Asha'man.)

### Step 5 — Abilities (`:527-543`)

Today:

```
Point-buy ability scores:
  STR  8 (mod -1, cost 0)
  DEX  8 (mod -1, cost 0)
  ...
  budget 25 / spent 0 / remaining 25
  set <abil> <n>   change one score (8..18)
  reset            send all scores back to 8
  done             accept and continue
```

Skin:

```
─── {{Step 5/8 — Ability scores}}::cyan|bold ─────────────────────────

  {{ABL  Score  Mod  Cost}}::yellow|bold
   STR    {{ 8}}::yellow   -1     0
   DEX    {{ 8}}::yellow   -1     0
   CON    {{ 8}}::yellow   -1     0
   INT    {{ 8}}::yellow   -1     0
   WIS    {{ 8}}::yellow   -1     0
   CHA    {{ 8}}::yellow   -1     0

  Budget {{25}}::yellow|bold · Spent {{0}}::yellow · Remaining {{25}}::green|bold

  {{set}}::yellow <abil> <n> · {{reset}}::yellow · {{done}}::green|bold
{{[step 5/8 abilities]}}::cyan »
```

**Remaining color rule:** green when ≥ 0, yellow at 0, red if over.
The current code rejects on assignment so over-budget only renders on
re-entry from `back` — keep the red branch anyway, defensively.

### Step 6 — Identity (`chargen_identity.go`)

Use `writeFieldRow` for every field. Render height + weight in **both
units** — feet/inches and cm; pounds and kg — regardless of the player's
locale.

```
  {{Height}}::yellow|bold      5'10" (178 cm)
  {{Weight}}::yellow|bold      176 lb (80 kg)
```

### Step 7 — Feats / Skills (`chargen_features.go`)

Section header + two sub-rules — one for feats, one for skill ranks.
Cross-class skills annotated `(cross-class)` in `gray`.

### Step 8 — Review (`:649-720`)

Today: one flat block.

Skin: three groups, each with its own subheader, ending with the
confirmation line.

```
─── {{Step 8/8 — Review}}::cyan|bold ─────────────────────────────────

{{Identity}}::cyan|bold
  {{Name}}::yellow|bold        Tam al'Thor
  {{Race}}::yellow|bold        human
  {{Gender}}::yellow|bold      male
  {{Age}}::yellow|bold         42
  {{Height}}::yellow|bold      6'0" (183 cm)
  {{Weight}}::yellow|bold      190 lb (86 kg)
  {{Handed}}::yellow|bold      right
  {{Alignment}}::yellow|bold   neutral

{{Build}}::cyan|bold
  {{Background}}::yellow|bold  Andoran Commoner (andoran)
  {{Class}}::yellow|bold       Soldier (soldier)
  {{Abilities}}::yellow|bold   STR 14  DEX 12  CON 13  INT 10  WIS 12  CHA 10

{{Loadout}}::cyan|bold
  {{Feats}}::yellow|bold       Athletic, Weapon Focus
  {{Skills}}::yellow|bold      Climb 4, Jump 4, Spot 2

──────────────────────────────────────────────────────────────────────
  Type {{yes}}::green|bold to confirm, {{back}}::yellow to revise, {{cancel}}::red to abort.
{{[step 8/8 review]}}::cyan »
```

## Migration map (raw → helper)

These bare-`WriteRaw` callsites become the targets of the chargen-render
PR. Counts are approximate from inspection at time of writing — verify
with `grep -n WriteRaw internal/mode/character_create.go
internal/mode/chargen_features.go internal/mode/chargen_identity.go`
before opening the PR.

| File | Approx callsites | Replacement |
|---|---|---|
| `character_create.go` | ~14 | `WriteString` + helpers |
| `chargen_features.go` | several | same |
| `chargen_identity.go` | several | same |

## Per-step prompt

Push via `Session.WritePrompt` after each menu render so:

- The line-edit caret has a visible cue.
- Async output (incoming tell, channel msg, MOTD on character switch)
  repaints cleanly via `WriteAsync`'s prompt-replay path.

Format: `{{[step %d/%d %s]}}::cyan » ` with the trailing space.

## Don'ts

- Don't add ASCII art splash screens to chargen substeps.
- Don't change the wording of validation errors except for severity
  prefix (`>> `).
- Don't introduce localization here. English-only baseline.
- Don't depend on Unicode symbols for meaning (✓, ─). The text content
  must stand alone if the symbol is unsupported.
