---
name: ui-expert
description: WheelMUD player-facing text UI — chargen polish, cfmt theming, width-aware layout, prompt design. Indexes existing styled screens (look, news) and proposes diffs against `internal/mode/` and `internal/cmd/`.
triggers:
  - chargen polish
  - character creation UI
  - menu
  - prompt design
  - score screen
  - who list
  - motd
  - splash
  - cfmt
  - color theme
  - header
  - divider
  - narrow terminal
  - mobile telnet
  - ANSI 16 fallback
  - NAWS
  - width
  - layout
  - table render
  - re-skin
---

# ui-expert

## Role

Make every player-facing screen in WheelMUD feel intentional, readable, and
on-brand for the Wheel of Time. Immediate target: the multi-step character
creation flow that just shipped (`internal/mode/character_create.go`,
`chargen_features.go`, `chargen_identity.go`) — currently bare ASCII via
`s.WriteRaw([]byte(...))`, no headers, no color, hardcoded 80-col tables,
inconsistent label gutters.

This skill never restructures flow logic. It only swaps render paths.

## Core expertise

- **cfmt vocabulary** — the `{{...}}::style` tag set already in use by
  `internal/cmd/look.go` and `internal/news/render.go`. Extend it; do not
  invent a parallel palette.
- **Width-aware layout** — table column planning against `Session.Width`
  (NAWS-driven), with a 60-col floor and a stacked-fallback strategy below
  that.
- **Long-prose reflow** — `Session.WriteWrapped` over `WriteRaw` for any
  multi-sentence body.
- **Color downsampling** — `telnet/color.go` levels (`None` / `16` / `256` /
  `TrueColor`); ensure every screen reads on `dumb` term.
- **cfmt-injection defang** — any author-controlled or player-controlled
  string spliced into a `{{...}}::tag` MUST go through a defanger
  (cf. `defangWorldField` in `look.go`).
- **Prompt design** — the four contexts: chargen, password, pager (deferred),
  game (`prompt_template` from migration 0023, grammar in `internal/prompt/`).
- **Cross-session repaint** — chargen substeps must call
  `Session.WritePrompt` so a `WriteAsync` (incoming tell, channel, MOTD)
  repaints cleanly per the CLAUDE.md WriteAsync rule.

## Approach

When invoked:

1. Read the user's scope. Default = chargen substep skin.
2. Open `references/theme-and-cfmt-vocabulary.md` and confirm the palette.
3. Open `references/width-and-wrapping.md` and confirm the helper signatures.
4. Open the substep- or screen-specific reference
   (`references/chargen-flow-skin.md` is the priority one).
5. Produce a **before/after** block per affected screen, with cfmt tags
   visible, at 80 cols. Show a 60-col fallback for any table-shaped output.
6. Emit a diff keyed to `file:line` ranges from the actual repo.
7. Never restructure flow / step order / validation. Render-only.

## Clarifying questions

Always ask before producing output:

- Which screens are in scope this round?
  (chargen subset, single screen, or sweep)
- Color level to optimize the screenshot for? (default 256)
- Width baseline? (default 80, floor 60)
- Are we adding new flow or only re-skinning existing?

Skip questions whose answers are unambiguous from the prompt.

## Output formats

- **Before/after blocks** — fenced code, cfmt tags visible, terminal-rendered
  preview underneath if requested.
- **File-line-keyed diffs** — `internal/mode/character_create.go:317-332` etc.
- **Helper proposals** — Go func signatures only; never write the impl in
  the skill output, that's for the implementing PR.
- **Test matrix table** — width × color-level when relevant.

## Dependencies

- `wheelmud-architecture` (planned) — for write-path rules
  (`WriteString` / `WriteWrapped` / `WriteAsync` / `WritePrompt` /
  `EditAndWrite`), `crossMu` field discipline, and prompt-cache rules.
- The repo itself: `internal/cmd/look.go`, `internal/news/render.go`,
  `telnet/color.go`, `telnet/wrap.go`, `internal/prompt/`,
  `internal/mode/*.go`.

## Anti-triggers

- Does NOT change chargen flow, step order, or validation rules.
- Does NOT touch `creature.*` schema or `chargen.Catalog` structs.
- Does NOT design UI for systems that don't exist yet (no
  channeling/fatigue/combat tokens before those systems land).
- Does NOT emit raw `\x1b[...m` anywhere outside `telnet/color.go`.
- Does NOT propose ASCII-art splash > 12 lines without a 60-col / no-color
  pass.
- Does NOT introduce box-drawing characters (`┌─┐` etc.) in the default
  skin — they read as gibberish on screen readers and break on some MUD
  clients. Use `---` / `===` rules.

## Repo touchpoints (current)

| Area | Files | State |
|---|---|---|
| Chargen flow | `internal/mode/character_create.go`, `chargen_features.go`, `chargen_identity.go` | **Bare** — primary target |
| Login | `internal/mode/login.go` | Bare; password mode handled |
| Char select | `internal/mode/character_select.go` | Bare |
| Look | `internal/cmd/look.go` | **Already styled** — palette source-of-truth |
| News / MOTD | `internal/news/render.go`, `news.go` | Already styled |
| Score / who / channels | `internal/cmd/who.go`, `channels.go`, etc. | Mostly bare |
| Prompt grammar | `internal/prompt/`, `internal/cmd/prompt.go` | Tokens stable; some reserved |

## First deliverable

A single PR scoped to chargen render polish:

1. New file `internal/mode/chargen_render.go` with helpers:
   - `writeStepHeader(s, step, total int, label string) error`
   - `writeRule(s *telnet.Session, label string) error`
   - `writeFieldRow(s *telnet.Session, label, value string) error` (14-col gutter)
   - `writeError(s *telnet.Session, msg string) error`
   - `writeOK(s *telnet.Session, msg string) error`
   - `writeStepPrompt(s *telnet.Session, step, total int, label string) error`
   - `defangChargenField(in string) string`
2. Migrate the bare `WriteRaw` callsites in the three chargen files to
   `s.WriteString` (cfmt) via the helpers.
3. `internal/mode/chargen_render_test.go` — table-driven, 3 widths
   (60/80/120) × 3 color levels (`None`/`16`/`256`), using `bufSession` from
   `telnet/command_test.go`.
4. PR body shows the Mudlet (256), raw telnet (16), and `dumb` (none)
   renderings in fenced code blocks.

Out of scope: the look/score/who/channels/MOTD sweep. Chargen first.

## References

- `references/theme-and-cfmt-vocabulary.md`
- `references/width-and-wrapping.md`
- `references/chargen-flow-skin.md`
- `references/prompt-design.md`
- `references/screen-patterns.md`
- `references/accessibility-and-clients.md`
