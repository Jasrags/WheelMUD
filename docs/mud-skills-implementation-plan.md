# WheelMUD — Skills Implementation Plan

## Overview

This document defines the Claude Code skills to build for WheelMUD. Skills live under
`skills/mud/` in this repo and follow the standard structure:

```
skills/mud/<skill>/
  SKILL.md                  # entry point: triggers, role, approach, anti-triggers
  references/*.md           # supporting reference docs
  agents/*.md (optional)    # subagent prompts when the skill delegates
```

The original draft of this plan was written for a generic ROM/CircleMUD codebase.
WheelMUD is **custom Go (1.25)** with its own primitives, so the foundation tier looks
different — there is no `.are` format, no vnum allocation, no `wiz` command tree. The
"codebase expert" role here is really about *our* layered architecture: telnet/session,
mode stack, repos + migrations, YAML world loader, chargen catalog, eventbus/tick.
Treat `CLAUDE.md`, `ROADMAP.md`, `docs/PLAN.md`, and `docs/CODEMAPS/*` as the source
documents these skills index — not invented from training data.

`ROADMAP.md` is status. `docs/PLAN.md` is sequencing. Skills cite both.

---

## What changed from the generic plan

| Dropped / Re-scoped | Reason |
|---|---|
| `codebase-expert` "area file format / vnum ranges / room flag table" | We don't have those. Replaced with `wheelmud-architecture` — indexes our actual modules. |
| `admin-designer` immortal/wiz tree | We have `internal/cmd/` with AuthLevel + `admin_audit`. Folded into `wheelmud-architecture`. |
| Generic combat rules | WoT d20 (book Table 8-X) is already partially implemented in `internal/creature/`. Skills reference book sections, not invent. |
| ANSI color guide as a standalone reference | We use `cfmt` tags + a level-aware downsampler (`telnet/color.go`). The skill teaches *our* tag vocabulary, not raw SGR. |

| Promoted | Reason |
|---|---|
| `ui-expert` (was Tier 3) | Most immediate need — chargen just landed and is plain ASCII. See §10. |
| `wot-d20-rules` reference catalog | Unique to this codebase; the source-of-truth rules conversion lives nowhere else. |

---

## Build sequencing (revised)

```
Phase 1 (Foundation — build first)
├── wheelmud-architecture    (replaces codebase-expert)
└── wot-d20-rules            (replaces systems-designer; pure rules reference)

Phase 2 (Active needs — build next, can parallelize)
├── ui-expert                ← PROMOTED — chargen polish blocks player retention
├── world-builder            ← already partially drafted (data/world/README.md)
└── lore-writing             ← supports world-builder + chargen flavor

Phase 3 (Content systems — when their roadmap phases land)
├── one-power-systems        (Phase C/D in docs/PLAN.md)
├── combat-designer          (Phase C)
├── mob-designer             (Phase E)
├── quest-designer           (Phase E)
└── economy-designer         (Phase B — partially done; shops + bankers exist)

Phase 4 (Operations — ongoing)
├── onboarding-designer
├── balancing-analyst
└── events-designer
```

---

## Tier 1 — Foundation

### 1. `wheelmud-architecture` (was `codebase-expert`)

**Purpose:** Authoritative reference for *our* code. Other skills that produce code or
schema changes must cross-check against this skill.

**Triggers:** session, mode, repo, migration, eventbus, tick, persist, telnet, cfmt,
WriteAsync, AuthLevel, admin_audit, world loader, chargen catalog, MOTD, escape sequences

**References to build (each ~1 page, mirrors a `docs/CODEMAPS/*` file):**
- `architecture-tour.md` — top-level wiring (`cmd/server/main.go::server`),
  what's a long-lived dep vs per-connection state, where to add the next.
- `session-and-write-paths.md` — `WriteString` / `WriteWrapped` / `WriteAsync` /
  `WritePrompt` / `EditAndWrite` rules. Cross-session output rule. `crossMu` fields.
- `mode-stack.md` — `Mode` interface, `PushMode` / `ReplaceMode` / `PopMode`,
  `ClearLastPrompt` discipline, ctx-cancel handling.
- `repo-and-migration-rules.md` — column-list lock-step (rooms / items / characters
  examples), optimistic-lock columns (`coin_version`, `version`), forward-only
  migrations, `ItemRepo.Transfer*` ownership invariant.
- `command-authoring.md` — `*telnet.Command` shape, `MinArgs`+`Long` usage block,
  `audit.Record` discipline, `safego.Go` for goroutines, semicolon-chaining
  (`SplitOnSemicolon`).
- `world-yaml-conventions.md` — links to `data/world/README.md`, room-id form,
  currency strings, typed item stats, shop/banker sub-blocks.
- `auth-and-audit.md` — `AuthLevel` lives on character; promotion in
  `mode/postauth.promoteToGame`; refusal paths MUST NOT audit.

**Anti-triggers:** does NOT design new mechanics, does NOT write room descriptions,
does NOT pick names. Pure structural reference.

---

### 2. `wot-d20-rules` (was `systems-designer`)

**Purpose:** Single source of truth mapping the printed WoT d20 rules to our
schema. When `internal/creature/`, `internal/chargen/`, combat verbs, or weave
mechanics need to extend, this skill names the book section *and* the column /
struct field that holds the result.

**References:**
- `ability-scores-and-modifiers.md` — point-buy (we use 25; book RAW differs),
  `creature.AbilityScore{Current,Max,Inherent}`, modifier formula.
- `races.md` — Human / Ogier; how `chargen.Background.HeightModIn` interacts
  with race base height.
- `classes-and-bab.md` — `BAB` strings ("good"/"avg"/"poor"), saves, hit dice,
  skill points; channeler flag and source.
- `feats-and-skills.md` — feat slot rules, skill-rank cap = level + 3,
  cross-class half-rate (deferred — see `chargen_features_followups.md`).
- `the-one-power.md` — saidin/saidar split, weave categories, taint, talents.
  Stub now; expand when Phase D lands.
- `combat-round.md` — initiative, AC composition, full-attack rules, damage types.
  Stub now; expand when Phase C lands.

**Approach:** every reference cites the book page **and** the struct/column it maps to,
or marks it `not-yet-modeled`. This skill is read-mostly; updates happen only when a
new mechanic ships.

**Anti-triggers:** does NOT decide *whether* to implement a rule (PLAN.md does).
Does NOT write Go.

---

## Tier 2 — Active Needs

### 10. `ui-expert` (PROMOTED — top priority)

This is the skill the user explicitly cares about right now. Treating it in detail.

**Purpose:** Make every player-facing screen — chargen, character select, MOTD,
score, who, look, channels, prompts — feel intentional, readable, and on-brand for
WoT. The character creation flow we just shipped (`internal/mode/character_create.go`,
`chargen_features.go`, `chargen_identity.go`) is the immediate target.

**Triggers:** chargen polish, character creation UI, menu, prompt, score screen, who
list, motd, splash, color theme, header, divider, narrow terminal, mobile telnet
client, ANSI 16 fallback, cfmt tag, width, NAWS, layout, table

**Dependencies:** `wheelmud-architecture` (write paths, color level, width)

#### What's actually wrong with chargen today

Concrete observations from the current code:

1. **No color, no hierarchy.** Every menu uses `s.WriteRaw([]byte(b.String()))`
   with bare ASCII. Backgrounds, classes, ability tables, review screen all read
   like a debug dump. cfmt is fully wired in `Session.WriteString`; chargen
   bypasses it.
2. **Hard-coded 80-col layout.** `fmt.Fprintf(&b, "  %2d. %-16s %-22s %s\r\n", …)`
   in `writeBackgroundMenu` and `writeClassMenu` assumes ≥80 cols. On a 60-col
   mobile telnet client (Mukluk, BlowTorch) the third column wraps and the table
   shears.
3. **No section header / progress.** A player on the abilities step has no
   visual cue that they're on step 5 of 8. The roadmap-equivalent UI affordance
   ("Step 5/8 — Abilities") is missing.
4. **Long descriptors aren't reflowed.** `writeBackgroundInfo` emits
   `bg.Description` verbatim with `WriteRaw`. We have `Session.WriteWrapped`
   (uses `Session.Width` from NAWS) and aren't using it.
5. **Inconsistent label gutter.** `"Home language:"` (14 cols incl. colon),
   `"Background skills:"` (18), `"Skill restriction:"` (18), `"Required skill:  "`
   (16, with one trailing space). Reads as if three different authors wrote it.
   Memory `chargen_bg_class_followups.md` already flags this.
6. **No prompt during chargen.** Players don't see a `>` cue — they just type
   into a void. The prompt cache (`WritePrompt`) is unused in chargen modes.
7. **Review screen is flat.** No grouping (Identity / Build / Loadout), no
   highlighting, "Yes / Back / Cancel" instructions are missing on first render.
8. **Errors are unstyled.** `"Score must be an integer in [8..18]."` in raw white
   reads identical to the menu — should be a noticeable warn-tone.

These are fixable without touching flow logic. The skill turns them into a
checklist with concrete cfmt patterns.

#### References to build

- `theme-and-cfmt-vocabulary.md`

  Codify the palette **already in use** by `internal/cmd/look.go` and
  `internal/news/render.go` so chargen and future screens match. Do not
  invent a new palette — extend this one.

  | Role | cfmt tag | Used today by |
  |---|---|---|
  | Section header / title | `cyan\|bold` | room title (`look.go:124`) |
  | Label (followed by value) | `yellow\|bold` | `{{Exits:}}::yellow\|bold` |
  | Value (item, exit, name) | `yellow` / `green` | exit names, item names |
  | Muted / disabled | `gray` | closed exits, "none", pitch-black room |
  | Notice banner | `yellow\|bold` | `{{[news]}}::yellow\|bold` |
  | Error | `red` | "Could not look around right now." |
  | OK / confirm | (not yet defined — propose `green\|bold`) | — |
  | Warn (over-budget) | (not yet defined — propose `yellow`) | — |
  | Accent — saidin | (propose `blue\|bold` on dark, fallback `cyan`) | — |
  | Accent — saidar | (propose `white\|bold`) | — |

  **Escaping rule:** any string sourced from a player or world author that's
  embedded inside `{{...}}::style` MUST go through a defanger
  (cf. `defangWorldField` in `look.go`). Chargen draft `Name`, ability
  values, etc. are author-controlled but free-form — defang before
  splicing into a tag. A stray `}}::red` in a name otherwise repaints
  the rest of the menu.

  **Banned:** hardcoded `\x1b[…m` outside `telnet/color.go`. Always go
  through cfmt so `Session.colorLevel` downsampling kicks in.

- `width-and-wrapping.md`
  - Always size tables off `Session.Width` (NAWS-driven). Provide a `tableCols`
    helper sketch: take available width, list of fixed-width columns, return
    a render plan.
  - Long prose goes through `WriteWrapped`, not `WriteRaw`.
  - Minimum supported width = 60. Fall back to a stacked layout below that.

- `chargen-flow-skin.md` — **the immediate-action reference**.

  Per-substep diff guide, keyed to current line ranges:

  | Substep | File:lines | Today | Skin |
  |---|---|---|---|
  | Backgrounds menu | `character_create.go:317-332` | bare `Backgrounds:` header, hardcoded `%-16s %-22s` | `{{Step 3/8 — Background}}::cyan\|bold` rule, table renders against `Session.Width`, id column dim-grey |
  | Background info | `:366-403` | raw description via `WriteRaw` | wrap prose via `WriteWrapped`; label gutter standard (14 cols + `: `); divider line above + below |
  | Classes menu | `:423-437` | same shape as backgrounds | same skin pattern; channeler tag in `cyan` accent |
  | Class info | `:471-494` | same | same |
  | Abilities menu | `:527-543` | flat list, no highlight on over/under budget | budget line goes `green` if remaining ≥ 0, `yellow` if 0, `red` if over (current code rejects on assignment, so the warn case only shows on entry from `back`) |
  | Identity menu | `chargen_identity.go` | (similar bare layout) | reuse the same row helper — height/weight rendered in feet/in *and* cm so the player sees both units |
  | Feats / Skills | `chargen_features.go` | bare | section header, list `(takes -1 cross-class)` notes in `gray` |
  | Review | `:649-705` | one flat block | three groups with subheaders: `{{Identity}}::cyan\|bold`, `{{Build}}::cyan\|bold`, `{{Loadout}}::cyan\|bold`; trailing line `Type {{yes}}::green\|bold to confirm, {{back}}::yellow to revise, {{cancel}}::red to abort.` |

  **Label gutter standard:** 14 cols, single trailing space, then colon, then
  one space, then value. Apply uniformly — kills the inconsistency flagged
  in `chargen_bg_class_followups.md`.

  **Error styling:** errors emitted from `apply*` validators go through
  `writeError(s, msg)` which renders as `{{>> }}::red\|bold {{<msg>}}::red`.
  Currently 11 raw `WriteRaw([]byte("..."))` error sites in
  `character_create.go` migrate to this helper.

  **Chargen prompt:** push a per-step prompt via `Session.WritePrompt`
  before reading next line. Format: `{{[step %d/%d %s]}}::cyan » `. This
  also makes `WriteAsync` cross-session messages (a tell arriving mid-
  chargen) repaint cleanly instead of clobbering the menu — the
  WriteAsync rule in CLAUDE.md depends on a cached prompt being present.

- `screen-patterns.md` — **document existing styled screens first, then
  align future ones to them.** Don't redesign what works.
  - `look` rendering: title `cyan|bold`, exits labeled `yellow|bold`
    with values `yellow`, closed/locked dimmed to `gray`, items
    labeled `green|bold`. Already in `internal/cmd/look.go`.
  - MOTD / news block: `internal/news/render.go` — `[news]` banner is
    `yellow|bold`; news bodies pass through cfmt, must escape author
    input (see `news.go` doc-comment).
  - Score / who / channels — **not yet styled.** Skill produces a
    proposal for each, matching the `look.go` palette, when those
    screens come up for re-skinning.

- `accessibility-and-clients.md`
  - Test matrix: Mudlet, TinTin++, Mukluk (Android), BlowTorch (Android),
    raw `telnet`, raw `nc`, `tmux + telnet` (passes IAC differently).
  - "No-color" mode: NEVER rely on color to convey meaning that isn't also
    in text (over-budget warning has both red AND the word "over-budget").
  - Screen-reader friendliness: avoid box-drawing characters that read aloud
    as gibberish; prefer `---` or `===` rules.

- `prompt-design.md`
  - The four prompt contexts: **chargen** (per-step, see above),
    **password** (rendered by login mode under `SetPasswordMode`),
    **pager** (deferred — see `pager_followups.md`), **game** (per-character
    `prompt_template` column from migration 0023).
  - Token vocabulary today (see `internal/cmd/prompt.go` help block —
    keep this skill's reference in lock-step with that help text):
    `%h/%H` HP, `%r` room, `%g` coin, `%%` literal. Reserved-but-stubbed:
    `%m/%M` mana (saidin/saidar pool), `%v/%V` move/fatigue, `%t`
    combat target — these render `0` / empty today and wire up when
    those systems land. Don't invent new tokens here; extend the
    grammar in `internal/prompt/` first.
  - Default template recommendation: `{{[%h/%H]}}::green » ` (color
    transitions to yellow ≤ 50%, red ≤ 25% — driven by render code,
    not the template string).

#### Approach (what the skill does when invoked)

1. Read `Session.WriteString` / `WriteWrapped` / `WriteRaw` rules from
   `wheelmud-architecture`.
2. Read `theme-and-cfmt-vocabulary.md`.
3. For chargen requests specifically: open `chargen-flow-skin.md`, walk the
   8-step list, propose a concrete diff against the named file:line. Do not
   restructure flow logic — only swap render paths.
4. Confirm with the user which terminal width to demo against (default 80,
   also show 60).
5. Produce one screenshot-quality before/after block per substep before
   writing code.

#### Clarifying questions the skill always asks

- Which screens are in scope this round? (chargen subset, or whole UI sweep)
- Color level to optimize for? (we ship for 256, must look OK on 16 and none)
- Width baseline? (80 default; 60 floor)
- Are we adding new flow or only re-skinning existing?

#### Output formats

- **before/after blocks** in fenced code with cfmt tags visible
- **diff-style** edits keyed to `internal/mode/character_create.go:NNN`
- **rendered preview** under each color level

#### Anti-triggers

- Does NOT change chargen flow / step order / validation rules.
- Does NOT touch `creature.*` schema or `chargen.Catalog` structs.
- Does NOT design new UI for systems that don't exist yet (no score-with-
  channeling layouts until `Channeling` model is wired into combat).
- Does NOT invent ASCII art splash screens larger than 12 lines without
  passing the 60-col / no-color test.

#### Before / after (Backgrounds menu)

Today (`character_create.go:317-332`):

```
Backgrounds:
   1. andoran          Andoran Commoner       Common (1 feats, 2 skills, 2 outfits)
   2. cairhienin       Cairhienin Noble       Common (0 feats, 3 skills, 1 outfits)
Type 'info <id|#>' for full details.
```

After:

```
─── {{Step 3/8 — Background}}::cyan|bold ──────────────────────────────────

  {{ #}}::gray|bold  {{id              }}::yellow|bold  {{name                  }}::yellow|bold  {{summary}}::yellow|bold
  {{ 1}}::gray  andoran          Andoran Commoner       Common · 1 feats, 2 skills, 2 outfits
  {{ 2}}::gray  cairhienin       Cairhienin Noble       Common · 0 feats, 3 skills, 1 outfits

  Type {{<id>}}::yellow or {{<#>}}::yellow to choose, {{info <id>}}::yellow to inspect, {{back}}::gray to revise.
{{[step 3/8 background]}}::cyan »
```

(Width stays under 80 cols; under 60 cols, summary column wraps to a
second indented line and the rule shortens.)

#### Concrete first deliverable when this skill ships

One PR, scoped:

1. Add `internal/mode/chargen_render.go` (new file — keeps
   `character_create.go` from growing past its current 821 lines):
   - `writeStepHeader(s, step, total int, label string) error`
   - `writeFieldRow(s, label, value string) error` (14-col gutter)
   - `writeError(s, msg string) error`
   - `writeOK(s, msg string) error`
   - `writeRule(s, label string) error` (uses `Session.Width`)
   - `writeStepPrompt(s, step, total int, label string) error`
     (caches via `Session.WritePrompt`)
   - `defangChargenField(s string) string` (mirror of
     `look.go::defangWorldField` — keep the impl local to mode/ for now,
     promote later if a third caller appears)
2. Migrate the bare `s.WriteRaw(...)` and `s.WriteRaw([]byte(b.String()))`
   callsites in `character_create.go`, `chargen_features.go`,
   `chargen_identity.go` to use the helpers + `s.WriteString` (cfmt).
3. Add `chargen_render_test.go` with table-driven tests at three widths
   (60 / 80 / 120) and three color levels
   (`ColorLevelNone` / `ColorLevel16` / `ColorLevel256`) using the
   `bufSession` helper from `telnet/command_test.go`.
4. PR body shows the Mudlet (256) + raw telnet (16) + dumb (none)
   renderings as fenced code blocks.

**Out of scope for this PR** (deferred to follow-on skill invocations):
the look/score/who/channels/MOTD skin sweep. Chargen first; prove the
helpers; then propagate.

---

### 3. `world-builder`

**Purpose:** Translates WoT geography into `data/world/<continent>/<nation>/…`
zone YAML. Already partially specified in `data/world/README.md`.

**Triggers:** zone, room, exit, sector, climate, ambient, nation, region,
settlement, continent, level_range, reset_interval, builder

**References:**
- `wot-geography-master.md` — port from prior session.
- `room-description-style.md` — second person present, sensory layering, length.
- `zone-yaml-cheatsheet.md` — links to `data/world/README.md`; flags the dual
  raw-SQL / Create column lock-step (rooms + items).
- `sector-and-climate-matrix.md` — which combos make sense.
- `coords-anchor-rules.md` — when to set `coords_auto: 0` (anchors), how the
  BFS pass fills the rest.

**Agents:**
- `room-writer.md` — produces room descriptions to spec.
- `zone-planner.md` — plans room counts, exit topology, anchor placement.

**Dependencies:** `wheelmud-architecture` (loader column lock-step), `lore-writing`.

---

### 4. `lore-writing`

**Purpose:** Voice consistency across rooms, item descriptions, NPC dialogue,
help topics, MOTD entries.

**Triggers:** description, dialogue, help topic, voice, flavor, in-game book,
faction speech

**References:**
- `wot-voice-guide.md` — Jordan prose tells, common idioms, what to avoid.
- `nation-flavor.md` — Andoran, Cairhienin, Aiel, Saldaean, Tairen, Sea Folk.
- `faction-speech.md` — Aes Sedai formality, Whitecloak zealotry, Asha'man
  curtness, Forsaken cadence.
- `oaths-and-idioms.md` — "Light burn me," "Blood and ashes," "honor of"…

**Agents:**
- `room-description-reviewer.md`
- `dialogue-writer.md`

---

## Tier 3 — Content systems (build when their phase lands)

### 5. `one-power-systems` (Phase D)
Already covered structurally by `internal/creature/Channeling`. Skill defines
the weave catalog and the strength/talent/taint mechanical model when Phase D
opens.

### 6. `combat-designer` (Phase C)
Defines damage types, AC composition, special moves, Warder bond bonuses.
Cross-references `wot-d20-rules/combat-round.md`.

### 7. `mob-designer` (Phase E)
Stat baselines, special procs, Shadowspawn flavor. Port the ~60-subtype catalog
from prior session into `references/mob-subtypes.md`.

### 8. `quest-designer` (Phase E)
Quest flag system + WoT story arcs. Depends on the not-yet-built quest engine.

### 9. `economy-designer` (Phase B — partial)
Shops + bankers exist; this skill defines pricing curves, regional flavor, loot
tables when loot tables ship.

---

## Tier 4 — Operations

| Skill | When |
|---|---|
| `onboarding-designer` | After ui-expert skin + tutorial zone exist |
| `balancing-analyst` | After Phase C combat is playable |
| `events-designer` | After calendar/clock matures past `Clock.HourOfDay` |

---

## Skill file standards (unchanged from original draft)

Each `SKILL.md` MUST contain:
1. Frontmatter — `name`, `description`, `triggers`
2. Role definition
3. Core expertise
4. Approach (numbered steps)
5. Clarifying questions
6. Output formats
7. Dependencies
8. Anti-triggers

---

## Documents to port immediately

| Document | Source | Destination |
|---|---|---|
| WoT Geography Master | prior conversation | `world-builder/references/wot-geography-master.md` |
| Mob Subtype Catalog | prior conversation | `mob-designer/references/mob-subtypes.md` |
| Existing zone YAML schema | `data/world/README.md` | linked from `world-builder/references/zone-yaml-cheatsheet.md` |
| CLAUDE.md "Things to watch" | `CLAUDE.md` | extracted into `wheelmud-architecture/references/*` |
| `docs/CODEMAPS/*.md` | repo | linked from `wheelmud-architecture/SKILL.md` |

---

## Next steps (concrete)

1. **Scaffold `skills/mud/ui-expert/SKILL.md`** with the §10 contents,
   plus the four references called out (`theme-and-cfmt-vocabulary.md`,
   `width-and-wrapping.md`, `chargen-flow-skin.md`, `prompt-design.md`,
   `screen-patterns.md`, `accessibility-and-clients.md`).
2. **Ship the chargen-render PR** described in §10's "Concrete first
   deliverable" — this is the proof the skill works against real code.
3. **Add a follow-up memory** when the PR lands so future sessions
   know the helpers exist and shouldn't re-add bare `WriteRaw` chargen
   sites. Memory name: `chargen_render_helpers.md`.
4. **Then build `wheelmud-architecture`** — every other skill cites it,
   but it's lower urgency than the ui-expert payoff.
5. **Defer Tier 3 content skills** until their `docs/PLAN.md` phase opens.

### Acceptance criteria for "ui-expert is built"

- `skills/mud/ui-expert/SKILL.md` passes the §"Skill file standards" checklist.
- All six reference files exist (even if some are stubs marked
  `not-yet-applicable`, they exist so the SKILL.md links don't 404).
- The `mode/chargen_render.go` helper PR is merged.
- A second invocation of the skill against a different screen (e.g. score)
  produces a proposal in the same shape without re-reading the codebase
  from scratch — proves the skill captured enough.
