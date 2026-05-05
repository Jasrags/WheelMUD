# Screen patterns

Document existing styled screens first; align future ones to them. Don't
redesign what works.

## Already styled (palette source-of-truth)

### `look` — `internal/cmd/look.go`

```
{{Caemlyn — Queen's Gardens}}::cyan|bold
A wide flagstone path winds between rosebushes.  ...

{{Exits:}}::yellow|bold north, {{west (locked)}}::gray, east
{{You see:}}::green|bold {{a small key}}::green, {{a torch}}::green
```

Pattern: title bold-cyan, prose plain (uses cfmt verbatim from
LongDesc — author trust, not defanged), labels yellow/green-bold,
values yellow/green, disabled gray, suffix annotation `(locked)` /
`(closed)` always present alongside the gray.

### News / MOTD — `internal/news/render.go`

`{{[news]}}::yellow|bold 2 unread entries. Type {{news}}::yellow to read.`

Pattern: bracketed banner in `yellow|bold`, verb hint in `yellow`. News
bodies pass through cfmt verbatim — author input MUST be escaped at
ingest (the `news` package's responsibility, not display).

## Not yet styled — proposals

These all currently render bare. When a re-skin sweep is requested,
match the `look.go` palette.

### `score`

Two-column stat block + conditions ribbon at the bottom.

```
{{Tam al'Thor — Soldier 1}}::cyan|bold
─────────────────────────────────────────────────────────────

  {{HP}}::yellow|bold       42 / 42         {{STR}}::yellow|bold  14 (+2)
  {{AC}}::yellow|bold       15              {{DEX}}::yellow|bold  12 (+1)
  {{BAB}}::yellow|bold      +1              {{CON}}::yellow|bold  13 (+1)
  {{Init}}::yellow|bold     +1              {{INT}}::yellow|bold  10 (+0)
  {{Speed}}::yellow|bold    30 ft           {{WIS}}::yellow|bold  12 (+1)
  {{Coin}}::yellow|bold     5gc 2sp         {{CHA}}::yellow|bold  10 (+0)

  {{Saves}}::yellow|bold    fort +3  ref +1  will +1

  {{Conditions}}::yellow|bold (none)
```

### `who`

```
{{Players online}}::cyan|bold (3)

  [ 1 Soldier]   {{Tam al'Thor}}::yellow      Two Rivers
  [ 5 Aes Sedai] {{Moiraine Damodred}}::white|bold   Cairhien
  [12 Asha'man]  {{Logain Ablar}}::blue|bold    Black Tower
```

Class colors: saidin (`blue|bold`), saidar (`white|bold`), warder
(`green|bold`), ordinary (`yellow`). Two-letter class abbreviations if
width is tight; full class name preferred.

### `channels` overview

```
{{Channels}}::cyan|bold

  {{ooc}}::yellow      on   {{[OOC]}}::cyan      out-of-character chatter
  {{newbie}}::yellow   on   {{[Newbie]}}::green  questions welcome here
  {{auction}}::gray    off  {{[Auction]}}::yellow  trade and sales
```

Channel-name color reflects subscription state (yellow on, gray off).
Bracket-tag color is fixed per channel category.

### Channel message rendering

Existing format (extend, don't redesign):

```
{{[OOC]}}::cyan {{Tam}}::yellow: hello world
{{[Newbie]}}::green {{Moiraine}}::white|bold: any questions?
```

Speaker name follows the class-color rule from `who`.

### Splash / connect banner

Don't replace the existing splash unless asked. If asked: keep under
12 lines, ASCII-safe (no box-drawing), one accent color, room for the
player to see the login prompt without scrolling on a 24-row terminal.

## Pattern checklist for any new screen

Before submitting a new screen design:

- [ ] Section header in `cyan|bold`?
- [ ] Labels in `yellow|bold` (or `green|bold` for item-list labels)?
- [ ] Values in matching plain (`yellow` / `green`)?
- [ ] Disabled state in `gray` AND text-suffixed?
- [ ] Errors via the standard `>>` red pattern?
- [ ] Width-aware (renders on 60 cols)?
- [ ] Color-blind safe (every state also in text)?
- [ ] No raw `\x1b[...m`?
- [ ] All interpolated names defanged?
- [ ] Tested on `dumb` term?
