# Theme & cfmt vocabulary

This is the palette **already in use** in the codebase. Match it; don't fork.

## Source of truth

- `internal/cmd/look.go` — room title, exits, items
- `internal/news/render.go` — `[news]` banner

## Palette

| Role | cfmt tag | Used today by |
|---|---|---|
| Section header / title | `cyan\|bold` | room title (`look.go:124`) |
| Sub-header (group label in review screen) | `cyan\|bold` | (proposed; matches title) |
| Label (followed by a value) | `yellow\|bold` | `{{Exits:}}::yellow\|bold` (`look.go:135`) |
| Item-list label | `green\|bold` | `{{You see:}}::green\|bold` (`look.go:164`) |
| Value — neutral | `yellow` | exit names |
| Value — item | `green` | item names |
| Muted / disabled / placeholder | `gray` | closed/locked exits, "none", pitch-black |
| Notice banner | `yellow\|bold` | `{{[news]}}::yellow\|bold` |
| Error | `red` | "Could not look around right now." |
| Confirm / OK | `green\|bold` | **proposed** — not yet in repo |
| Warn (over-budget / cautionary) | `yellow` | **proposed** |
| Accent — saidin (male One Power) | `blue\|bold` | **proposed** — Phase D |
| Accent — saidar (female One Power) | `white\|bold` | **proposed** — Phase D |
| Accent — Shadow / taint | `red\|bold` | **proposed** — Phase D |

## Rules

### 1. Always go through cfmt

`Session.WriteString(text)` parses cfmt tags AND downsamples by
`Session.colorLevel`. Bypass it (raw `\x1b[...m`) and:
- `dumb` clients see escape sequences as garbage
- 16-color clients see truecolor escape sequences they don't understand
- you lose the per-session color preference (`colors` verb)

`telnet/color.go` is the only place raw SGR is allowed to appear.

### 2. Always defang interpolated strings

Player- or world-author input embedded inside a `{{...}}::style` block
MUST be defanged. A character named `Lan}}::red\r\n{{` would otherwise
repaint everything below in red.

`internal/cmd/look.go` defines `defangWorldField` for room/item names.
For chargen, the helper `defangChargenField` lives in
`internal/mode/chargen_render.go` and applies the same rule to draft
`Name` and any free-form fields.

Verbatim cfmt is fine **only** for builder-authored prose that's
trusted to color itself — e.g. `room.LongDesc` is rendered with
`toCRLF` and no defang in `look.go:130`. Treat that as the exception,
not the pattern.

### 3. Don't rely on color alone

Every meaningful state must also be in text:

- Over-budget warning: red AND the word `over-budget`.
- Closed exit: gray AND the literal `(closed)` suffix.
- Channeler tag: cyan AND the word `channeler`.

This protects `ColorLevelNone` clients and screen-reader users.

### 4. Use the lightest tag that works

Prefer `yellow` over `yellow|bold` for values; reserve bold for labels
and headers. Bold-everything reads as flat as plain.

## Style examples

```
{{Exits:}}::yellow|bold north, south, {{west (locked)}}::gray
{{You see:}}::green|bold {{a sword}}::green, {{a torch}}::green
{{>> }}::red|bold {{Score must be in [8..18].}}::red
{{✓ }}::green|bold {{Saved.}}::green
─── {{Step 3/8 — Background}}::cyan|bold ──────────────────────────────
{{[step 3/8 background]}}::cyan »
```

## Anti-patterns

```go
// BAD — bypasses downsampling, breaks on dumb clients
s.WriteRaw([]byte("\x1b[1;33mExits:\x1b[0m north\r\n"))

// BAD — author input not defanged
fmt.Fprintf(&b, "{{%s}}::cyan|bold\r\n", playerName)

// BAD — color carrying meaning text doesn't carry
"{{ }}::red"  // empty red space implying error

// GOOD
s.WriteString("{{Exits:}}::yellow|bold north\r\n")
fmt.Fprintf(&b, "{{%s}}::cyan|bold\r\n", defangChargenField(playerName))
```
