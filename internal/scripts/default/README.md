# Default Lua script catalog

Drop one `*.lua` file per script in this directory; the embedded
loader walks every `*.lua` file at the root and assembles
`scripts.Catalog`. The base filename (without `.lua`) is the script
name triggers / dialogue effects / quest steps reference.

Override the embedded set with the `SCRIPT_DIR` environment variable
to point at a directory of `.lua` files outside the binary.

## Slice 1 API

Every script runs in a sandboxed Lua environment. The following
globals are bound by the runner per call:

### Functions

- `say(text)` — broadcast a `says, "text"` line to every session
  in the trigger's room. Cross-session output rule applies (uses
  `Session.WriteAsync` under the hood).
- `emote(text)` — broadcast a third-person line (e.g.
  `"the warden eyes you suspiciously"`) to every session in the
  room.
- `log(level, msg)` — emit a structured log entry through the
  server's `slog` logger. `level` is one of `"debug"`, `"info"`,
  `"warn"`, `"error"`. Useful for builder debugging.

### `ctx` table (read-only)

Populated from the trigger's `EventCtx` for each invocation:

| Field            | Type    | Meaning                                                       |
|------------------|---------|---------------------------------------------------------------|
| `ctx.event`      | string  | `"on_enter"`, `"on_say"`, `"on_attack"`, `"on_death"`, `"on_tick"` |
| `ctx.room_id`    | number  | Room id where the event fired (0 if not room-scoped)          |
| `ctx.actor_id`   | number  | Acting actor id (player on enter/say, attacker on attack)     |
| `ctx.actor_kind` | string  | `"character"` or `"mob"`                                      |
| `ctx.target_id`  | number  | Target actor id (defender on attack, victim on death)         |
| `ctx.target_kind`| string  | `"character"` or `"mob"`                                      |
| `ctx.text`       | string  | Player text on `on_say` (empty otherwise)                     |
| `ctx.bucket`     | string  | Tick bucket name for `on_tick` (`"phase"` etc.)               |

### Sandbox

The following standard libraries are NOT available:

- `os` (no shell-out)
- `io` (no filesystem)
- `debug` (no introspection / source mutation)
- `package` (no module loading)
- `dofile`, `loadfile`, `loadstring`, `load` (no dynamic code loading)

`math`, `string`, `table`, and core builtins (`tostring`, `tonumber`,
`pairs`, `ipairs`, etc.) are available.

### Resource caps

- Per-call instruction limit: 100,000 (gopher-lua `SetMx`).
- Per-call timeout: 50ms (`context.WithTimeout`).
- A script that hits either cap returns a fault-budget hit.

### Fault budget

Each trigger tracks consecutive script faults
(`triggers.consecutive_faults`). At 5 faults the trigger
auto-disables (`triggers.disabled = 1`) and stops firing until an
operator resets the counter:

```sql
UPDATE triggers SET consecutive_faults = 0, disabled = 0 WHERE id = ?;
```

A successful invocation resets the counter to 0. World re-deploys
also reset both columns.

## Example

`warden_alert.lua`:

```lua
-- Fire whenever a player enters the warden's tower.
say("Halt! State your business.")
log("info", string.format("warden_alert: actor=%d kind=%s",
    ctx.actor_id, ctx.actor_kind))
```

YAML to attach (in `data/world/<zone>/mobs.yaml`):

```yaml
- id: tr.warden
  room: tr.tower.gate
  name: the Tower Warden
  triggers:
    - event: on_enter
      action: lua
      payload:
        script: warden_alert
```
