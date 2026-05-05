# Accessibility & clients

## Test matrix

Every UI change should be sanity-checked across this matrix before
landing. The chargen-render PR specifically demands the 3×3 matrix
embedded in the test file (`chargen_render_test.go`).

### Color levels

| Level | Source | Behavior |
|---|---|---|
| `ColorLevelNone` | `dumb`, `unknown` | cfmt strips all SGR |
| `ColorLevel16` | `vt100`, `ansi`, `linux`, `xterm` | downsamples to 30-37 / 90-97 |
| `ColorLevel256` | `xterm-256color`, `mudlet` | xterm 6×6×6 cube |
| `ColorLevelTrueColor` | `iterm2`, `xterm-truecolor` | 24-bit verbatim |

(See `telnet/color.go::DetectColorLevel`.)

### Widths

- 60 cols (mobile telnet client floor)
- 80 cols (target — most desktop clients)
- 120 cols (wide terminal — should not look sparse)

### Real clients

| Client | Notes |
|---|---|
| Mudlet | xterm-256color, NAWS, MCCP — full color, full width detection |
| TinTin++ | xterm-256color typical, NAWS supported, headless friendly |
| MUSHclient (Win) | NAWS supported, 256-color since 4.x |
| Mukluk (Android) | reports narrow widths; vt100/ANSI; flaky on truecolor |
| BlowTorch (Android) | similar to Mukluk |
| `telnet` (BSD/macOS) | dumb-ish; treat as ColorLevel16 worst-case |
| `nc` / `netcat` | no IAC negotiation; `dumb` color level |
| `tmux + telnet` | passes IAC oddly — treat as 16-color, NAWS may lie |

## Accessibility rules

### 1. Don't rely on color alone

Every meaningful state has its color **and** a text marker:

| State | Color | Text marker |
|---|---|---|
| Closed exit | `gray` | `(closed)` suffix |
| Locked exit | `gray` | `(locked)` suffix |
| Over-budget abilities | `red` | the word `over-budget` |
| Channeler class | `cyan` accent | the word `channeler` |
| Pitch black | `gray` | `it is pitch black` |
| Disabled channel | `gray` | `off` suffix |

Screen-reader users and color-blind users get full information.

### 2. ASCII-safe defaults

- Section dividers: `---` or `===`. Long form: U+2500 `─` only when
  cleanly supported. NEVER use box-drawing `┌─┐│└─┘` for boxes — they
  read aloud as garbage and break in some clients.
- Bullets: `*` or `-`, never `•` or `▶`.
- Confirmation glyph: `✓` is acceptable (widely supported, screen
  readers say "check"). Always pair with text: `{{✓ Saved.}}::green`.
- Error glyph: `>>` (literal angle brackets). Avoid `⚠` / `❌`.

### 3. Avoid ambient blink / reverse / strikethrough

cfmt supports them; don't use them. Blink is hostile to attention; reverse
breaks copy/paste; strikethrough is unsupported in many clients.

### 4. Prompt audibility

Screen readers re-read the line on prompt arrival. Keep prompts short
and meaningful — `[step 5/8 abilities] » ` is good; a 40-char prompt
rebuild every keystroke is hostile.

## NAWS / width edge cases

- Width can change mid-session. Helpers should always re-read
  `Session.Width` at render time, not cache it.
- Some clients send NAWS as `0 0` to mean "unset" — the session falls
  back to 80 in that case. Don't try to design for `Width == 0`.
- Very wide windows (200+ cols) should not stretch tables to fill —
  cap the Name column at a sensible max (e.g. 24) and add right-margin
  whitespace.

## What "tested on dumb term" means

```bash
TERM=dumb telnet localhost 2323
```

Connect, run through the screen, verify:

- No raw `\x1b[` escapes leak into output.
- No empty colored padding (e.g. red space implying error).
- All meaningful state still readable in text alone.
- Prompts and dividers still render — even if `─` falls back to `-`.

The chargen-render PR's test file should include a `ColorLevelNone`
fixture that scans the rendered output for `"\x1b["` and fails if found.
