<!-- Generated: 2026-04-30 | Files scanned: 0 (no persistence yet) | Token estimate: ~150 -->

# Data

**No persistence layer exists yet.** All session state is in-memory and dies with the goroutine.

## In-process state

| Owner | Lifetime | Notes |
|---|---|---|
| `telnet.Session` (per conn) | accept → disconnect | `InputBuffer` (read-goroutine-owned), `inbox` chan (cap 16), `modes` stack, `Width/Height/TerminalType/ColorLevel` |
| `telnet.Registry` (process) | server lifetime | Built once in `buildRegistry`, shared read-only across sessions; mutations are mutex-guarded but no runtime mutation happens today |

## Pending

When persistence lands, this codemap should grow tables for: `accounts`, `characters`, `rooms`, `exits`, `items`, `mobs`, plus migration history. `ROADMAP.md` §7 ("Persistence") tracks the open work — pick a backing store (SQLite first), add a `Repository` per aggregate, and wire migrations (`goose` / `golang-migrate`).
