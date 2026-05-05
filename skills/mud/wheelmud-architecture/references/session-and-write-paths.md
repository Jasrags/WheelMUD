# Session and write paths

Five write helpers on `telnet.Session`. Pick the right one or you will
clobber a player's prompt or race a goroutine.

| Helper | Use when | Notes |
|---|---|---|
| `WriteString(msg)` | Synchronous reply to the dispatcher's own session | Renders cfmt `{{...}}::style` tags; downsamples via `Session.colorLevel` |
| `WriteRaw(b)` | Bytes already escaped/styled | Holds `writeMu`; the only safe path to `Conn.Write` |
| `WriteWrapped(msg)` | Long prose body | Reflows to `Session.Width` (NAWS-driven) |
| `WriteAsync(msg)` | **Cross-session** output (broadcasts, channel fanout, mob arrival/depart, phase ambients, anything to a session that isn't the current dispatcher's `c.Session`) | Wraps with CR+EL erase prefix + replays cached prompt + line-edit buffer |
| `WritePrompt(msg)` | Caches the per-step / per-mode prompt | `WriteAsync` reads this cache when repainting |
| `EditAndWrite(fn)` | Read-goroutine paths that mutate `Session.Input` | Runs `fn` under `writeMu`; serializes against `WriteAsync`/`WritePrompt`/`listAndRedraw` |

## The cross-session output rule

If your command emits text to **anyone other than the player who typed
it** — peers in the room, channel subscribers, the target of a tell —
that emission goes through `WriteAsync`, not `WriteString`. Otherwise a
mid-line broadcast clobbers the recipient's prompt + in-progress
input.

The dispatcher's own reply (`c.Session.WriteString(...)` after handling
the verb) stays on `WriteString` because the dispatcher repaints the
prompt immediately after `Mode.Handle` returns.

## crossMu-guarded fields

These live on `Session` and are written by one goroutine, read by
another. Always go through the `Set/Get/Toggle/Snapshot` helpers — never
touch the unexported field:

- `lastTellFrom`
- `lastInputAt`
- `channelMuted`

In-world fields (`CharacterID`, `CharacterName`, `CurrentRoomID`) are
dispatcher-owned. Snapshots returned from
`session.Registry.Snapshot()` are values — they can change underfoot.

## Password mode

`InPasswordMode` writes from mode handlers go through
`Session.SetPasswordMode(bool)` (under `writeMu`). Top-level
fast-path reads in `handleEscape` / `handleTab` race with
mode-handler writes; the inner `EditAndWrite` paths re-read under the
lock so the race only widens the bell-vs-dispatch decision by one
keystroke.

## Banned

- Calling `Conn.Write` directly. Always go through `WriteRaw` or one
  of its callers.
- Emitting raw `\x1b[...m` outside `telnet/color.go`.
- Reading `lastTellFrom` / `lastInputAt` / `channelMuted` without the
  helper.
