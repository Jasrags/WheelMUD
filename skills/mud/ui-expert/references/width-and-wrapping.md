# Width & wrapping

## Width comes from NAWS

Every `telnet.Session` carries a `Width` field populated by the NAWS
(window-size) telnet sub-negotiation during connect, and updated mid-
session if the client resizes. Default is 80 when NAWS is silent.

## Floor and target

- **Target:** 80 columns. Most desktop telnet clients and Mudlet default
  here.
- **Floor:** 60 columns. Mobile clients (Mukluk, BlowTorch) commonly
  start narrower.
- **Below 60:** clamp to 60 for layout purposes; the user's terminal
  will soft-wrap. Don't actively design for sub-60.

## Long prose

Use `Session.WriteWrapped(text)`, never `WriteRaw` for multi-sentence
bodies. WriteWrapped reflows to `Session.Width`. Today's chargen
violates this — `bg.Description` and `cl.Description` go through
`WriteRaw` (see `character_create.go:399` and `:490`).

## Tables

Helper sketch for the chargen render layer:

```go
// tableCols returns the actual column widths to use given the session
// width and the requested column widths. Trailing columns absorb
// remainder; if total exceeds available, the LAST column shrinks to
// fit (down to a minimum), and if even that doesn't fit, returns
// stack=true signaling the caller to render rows as stacked label/value
// pairs instead of a table.
func tableCols(sessionWidth int, requested []int, minLast int) (cols []int, stack bool)
```

The Backgrounds menu today (`character_create.go:328`) is:

```go
fmt.Fprintf(&b, "  %2d. %-16s %-22s %s\r\n",
    i+1, bg.ID, bg.Name, backgroundSummary(bg))
```

That's `2 + 2 + 16 + 1 + 22 + 1 + len(summary)`. The first three columns
total 44; the summary needs ~30 to render comfortably. At 60 cols, the
summary has only 16 — it shears.

Re-skin pattern:

- 80 cols: keep three-column table.
- 60-79 cols: shrink Name col to fit, truncate or wrap summary on a
  second indented line (`    {{...}}::gray`).
- <60 cols: stack — one row per field per item.

## Line endings

Always `\r\n`. The dispatcher and most clients tolerate `\n` alone but
some IAC-strict clients glitch. Match the existing `\r\n` everywhere.

## Don't break mid-tag

cfmt parses `{{...}}::style` greedily; if your wrap inserts a newline
between `{{` and `}}::style`, the tag breaks and the literal text leaks.
`WriteWrapped` is tag-aware and safe; manual splitting is not. If you
must hand-split, split outside tags only.

## Trailing whitespace

Strip trailing spaces from each line. Some terminals render them as
visible cursor artifacts when the line is colored.

## Helper signatures (target)

```go
func writeRule(s *telnet.Session, label string) error
// renders: "─── {label} ─" padded to Session.Width with `─` runes.

func writeStepHeader(s *telnet.Session, step, total int, label string) error
// wraps writeRule with "Step N/M — Label" formatting in cyan|bold.

func writeFieldRow(s *telnet.Session, label, value string) error
// "  {label:14} {value}\r\n" with label in yellow|bold, value plain.

func writeError(s *telnet.Session, msg string) error
// ">> {msg}" with >> in red|bold and msg in red.

func writeOK(s *telnet.Session, msg string) error
// "✓ {msg}" with ✓ in green|bold and msg in green.
```

The 14-col gutter is the existing-flow-friendly choice: `Background:` is
11 chars, `Background skills:` is 18 — the longest current label —
so 14 leaves a single trailing space for some, two for others. If we
go to 19 we waste a column for the rest. 14 + value works.
