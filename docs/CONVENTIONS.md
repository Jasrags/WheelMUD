# Conventions

Code-level recipes and invariants. CLAUDE.md links here instead of inlining
these; update this file when a pattern changes.

## Write paths (telnet)

- `Session.WriteRaw` is the only safe write path; it holds `writeMu`. Layer
  helpers on top of it rather than calling `Conn.Write` directly.
- **Synchronous dispatcher output** (a command's own response to
  `c.Session`) uses `WriteString` — the dispatcher repaints the prompt
  immediately after `Mode.Handle` returns.
- **Cross-session output** (broadcasts, channel fanout, mob arrival/
  departure, phase ambients — anything writing to a session that isn't
  the dispatcher's `c.Session`) MUST use `Session.WriteAsync`. It wraps
  with a CR+EL erase prefix and replays the cached prompt + line-edit
  buffer so a mid-line broadcast doesn't clobber input.
- Prompt cache is managed by `WritePrompt`; mode transitions clear via
  `ClearLastPrompt` (`PushMode`/`PopMode` handle it).
- Read-goroutine keystroke handlers wrap "decide echo + mutate Input +
  emit echo" in `Session.EditAndWrite(fn)`. `fn` runs under `writeMu`
  and returns the bytes to write — serializes against `WriteAsync`,
  `WritePrompt`, and `listAndRedraw`. Password-mode toggles go through
  `Session.SetPasswordMode(bool)` (also under `writeMu`).
- **Flood gate (§M.2):** `Session.WriteRaw` consults a per-session
  token bucket BEFORE the kernel write. Configured at session
  construction via `Session.SetFloodGate(rate, burst)` and wired in
  `cmd/server/main.go` acceptLoop from `cfg.Server.FloodBytesPerSec` /
  `FloodBurstBytes` (defaults 64 KiB/s sustained, 128 KiB burst).
  Policy is **drop-silent**: when the bucket refuses, `WriteRaw`
  returns nil and logs the dropped byte count at Debug. The session
  stays open and the next allowed write resumes cleanly. This is the
  only place output can be dropped — every other write helper layers
  on `WriteRaw`.

## Dispatcher anti-abuse (§M.2 BadInputTracker)

- The dispatcher in `telnet/command.go::Dispatch` records every
  unknown-verb and AuthLevel-denied attempt through
  `Registry.tracker.Record(s)` BEFORE writing the identical "Unknown
  command\r\n" response. Both code paths do the same work in the same
  order so a probe can't time-distinguish "verb doesn't exist" from
  "verb exists but I'm not privileged".
- A session that exceeds the burst (default 20 hits per 30s) gets
  silent drops on subsequent unknowns until the window resets — the
  connection stays open but the probe loop produces no observable
  output. `Forget(s)` runs in `handleConnection`'s defer to clean up
  on disconnect.
- Construct via `telnet.NewBadInputTracker(window, maxInBurst)` and
  install with `registry.SetBadInputTracker(t)`. A nil tracker is a
  valid no-op — Record returns true.

## Visibility (wizinvis)

- The canonical "can viewer see target?" check is
  `internal/visibility.CanSee(viewer, target)`. Do NOT reimplement
  `peer.IsHidden() && viewer.AuthLevel < telnet.AuthAdmin` — that
  pattern drifted across half a dozen callsites before being
  centralized.
- For broadcasts, `internal/visibility.VisiblePeers(viewer, peers)`
  returns a filtered slice. The shared room-broadcast helper
  `cmd.broadcastRoom`/`broadcastRoomExcept2` filters automatically
  when its `except` (or `a`) argument is the actor — pass actor
  there, peers who can't see actor are skipped.
- Speaker side: hidden admin's say/shout/yell is silent to non-admin
  peers (slot the check after the room/zone gate but before
  `peer.WriteAsync`).
- Viewer side: hidden admin sees other hidden admins (admins
  transitively see each other through wizinvis); non-admins see only
  non-hidden peers.

## Session field ownership

- `Session.Input` is owned by the read goroutine inside `RunSession`.
  Do not mutate it from another goroutine.
- Cross-goroutine fields (`lastTellFrom`, `lastInputAt`, `channelMuted`,
  `charset`) MUST go through the `Set/Get/Toggle/Snapshot` helpers —
  they take `crossMu`.
- In-world fields (`CharacterID`, `CharacterName`, `CurrentRoomID`) are
  dispatcher-owned; treat `session.Registry.Snapshot()` results as
  values that can change underfoot.

## Telnet option negotiation

- Responses go through `telnet/iac.go::handleOptionNegotiation` (WILL/
  WONT/DO/DONT) and the `HandleSubnegotiation` switch (SB…SE).
- New options follow the CHARSET/MSSP pattern: add the option constant
  + sub-codes, append a `WILL <opt>` to `NegotiateTelnet`, write the
  response via `s.WriteRaw` from `handleOptionNegotiation`.
- MSSP variables come from a `Session.MSSPProvider` closure wired in
  `cmd/server/main.go::msspVars`; provider == nil silently no-ops.

## GMCP (option 201)

- Same closure-injection pattern as MSSP. `Session.GMCPHandler` is set
  at session construction to `internal/gmcp.Manager.Handle`; the
  manager dispatches Core.* and installs per-session eventbus
  subscriptions on Core.Supports.Set.
- Subscription handles live on `Session.gmcpSubs` (via `AddGMCPSub` /
  `TakeGMCPSubs`) and MUST be cancelled in `handleConnection`'s defer
  via `gmcp.Manager.UnwireSession(s)` — otherwise eventbus handlers
  leak across reconnects.
- Outbound frames go through `Session.WriteGMCP(pkg, body)`, a silent
  no-op when the client hasn't negotiated GMCP. GMCP bytes are
  out-of-band telnet, so `WriteGMCP` uses `WriteRaw` rather than
  `WriteAsync` — no prompt repaint.
- Field names the Mudlet Lua scripts read are exactly those declared
  on `internal/gmcp/packages.go`; renaming a Go field requires a
  matching package update + `clients/mudlet/src/config.lua` version
  bump.

## Dispatch / commands

- `Mode.Handle(ctx, *Session, line)` is invoked synchronously by
  `runDispatcher`. ctx is canceled on EOF / idle / flood; handlers
  doing blocking I/O must observe it. A slow handler stalls input
  for that session.
- `Registry.Dispatch` enforces `Command.Auth` against
  `Session.AuthLevel`. Privilege-denied lookups return the same
  `Unknown command` text as a missing verb (no enumeration).
- `Command.Lag` is the per-verb global cooldown stamped on
  `Session.nextReady` via `s.StampLag(cmd.Lag)` after a successful
  `cmd.Run`. The gate lives in `Registry.dispatchOne` (per-segment),
  so chained `;` inputs gate independently. Stamp on success only.
- Segment-aware: top-level `;` outside quotes splits via
  `telnet.SplitOnSemicolon` (mirrors `Tokenize`'s quote/escape rules).
  Hard cap `maxSegmentsPerLine = 16`; alias expansion bounded at
  `maxAliasDepth = 3`.
- Item/mob keyword resolution (incl. ordinal `2.sword`) goes through
  `keyword.go::MatchItem` / `MatchMob`.
- Required-args commands declare `MinArgs` + `Long`; dispatcher emits
  Long-aware usage on too-few-args.

## Items: 3-location invariant

Exactly one of `room_id`, `owner_character_id`, or `parent_item_id` is
set. Use `ItemRepo.SetOwner` / `SetRoom` / `SetParent` or the
`Transfer*` family — they flip atomically and clear the other two.

`Transfer*` guards on prior location, so concurrent
`get`/`give`/`put` surfaces as `ErrItemMoved` instead of silent
overwrite. `Character.Inventory` (`inventory_json`) is display
ordering only — SQL `owner_character_id` is the source of truth;
`inventory.go::orderInventory` self-heals. Items inside containers
are NOT in `inventory_json`; encumbrance reads them via
`ListAllOwnedTransitive` (BFS through `parent_item_id`).

## `characters` column lock-step

New columns land in BOTH `charPlayerColumns` AND `charPlayerValues`
AND `charPlayerScanDest` (`internal/repo/character_sql.go`); ordering
is load-bearing. `auth_level` MUST stay the very last entry — the
SQLite first-character bootstrap CASE expression in
`CharacterRepo.Create` consumes it as the trailing placeholder.

JSON columns also need a `characterJSON` field plus marshal/unmarshal
lines in `character_sqlite.go::marshalCharacterJSON` and
`(characterJSON).unmarshalInto`.

## World loader lock-step

New columns on `rooms` / `items` / `exits` land in the repo's
`*SelectCols` + `Create` INSERT + scan path AND in the loader's raw-
SQL INSERT in `internal/world/loader.go` (`roomInsertValues` /
`insertItems` / `insertExits`). The loader writes raw SQL inside one
transaction rather than going through repo `Create`, so the column
lists are duplicated.

## Auth promotion

AuthLevel lives on the character row, not the account. Session stays
at `AuthGuest` through login + account-create; stamped by
`mode/postauth.promoteToGame` from `Character.AuthLevel` once a
character is selected. `CharacterRepo.Create` atomically promotes the
very first character to `AuthAdmin` (fresh-deploy bootstrap).

## Admin audit rule

Privileged verbs (`spawn`, `teleport`, `goto`, `transfer`, `summon`,
`wizinvis`, `shutdown`, `reboot`) record one `admin_audit` row per
successful invocation via `internal/audit.Record(c.Ctx, audits,
c.Session, verb, target, args)`.

**Refusal paths MUST NOT audit** — the row represents "this side
effect actually happened." Synchronous by design so `shutdown` rows
commit before drain begins.

## Progression spend verb pattern

Applies to `learn`, `feat`, `bump`, `learn weave`.

- Per-verb repo method `RecordX` takes the absolute new pending value
  + per-pool upsert entry (mirrors `RecordCoin`/`RecordXP`, not
  `RecordLevelUp`-widening).
- Cmd-layer computes cap + budget guards before the call.
- Refusals do NOT mutate or audit; success writes one
  `audit.Record(verb=X, target=<id>, args=<n>)`.
- Catalog string ids → int32 via `chargen.HashID(id)`.

## Shutdown / reboot

`shutdown` / `reboot` drive teardown by calling the same `stop` cancel
`signal.NotifyContext` returns; the watcher goroutine then closes the
listener and runs `srv.shutdown()` → `persist.FlushAll`. `reboot`
flips `srv.rebootOnExit`; `main()` ends with `syscall.Exec`
(POSIX-only). Countdown goroutine uses `Session.WriteAsync`;
interruptible via `RequestAbort`.

## Long-lived goroutines

Spawn via `safego.Go("name", fn)` for panic-safe long-lived
goroutines. Logging uses `slog`; level set in `main.go` from
`LOG_LEVEL`.

## Migrations

Forward-only, embedded under `internal/db/migrations/`. Read the SQL
directly when you need the schema. Key invariants codified by recent
migrations:

- 0017, 0028 — items 3-location invariant.
- 0019 — `auth_level` lives on character, not account.
- 0029, 0036, 0052 — append-only forensic logs (`admin_audit`,
  `account_logins`, `character_audit`).
- 0032 — `characters.coin_version` optimistic-lock token bumped by
  `RecordCoin`; mismatched writes return `ErrCoinConflict`.
- 0046 — `triggers.consecutive_faults` / `disabled` auto-disable a
  trigger after 5 faults; `world` loader resets via
  `TriggerRepo.ResetAllFaults` at boot.

## Lua sandbox

`internal/lua.NewSandboxedState()` strips dangerous globals (`os`,
`io`, `debug`, `package`, `dofile`, `loadfile`, `loadstring`, `load`).
`Runner` pre-allocates an LState pool (size 8) served via a buffered
channel — NOT `sync.Pool`, which can synthesize states at Stop.
`Runner.Run` wraps the parent ctx with `CallTimeout = 50ms` and
propagates via `SetContext`. `Runner.Stop()` closes every LState —
must run BEFORE `bus.Stop()` in shutdown.

API surface: `say`, `emote`, `log`, read-only `ctx`;
`quest.accept/advance`; `push_mode`; `apply_affect`, `give_item`;
read-only `target` / `room` (resolved at bind time from
`b.Ctx.RoomID`) / `clock`. nil-bound hooks register classified-error
stubs so misuse trips the trigger fault budget instead of a generic
nil call.

## World loader (`LoadAndSync`)

Additive resync only — no updates, no deletes. Parses + validates
YAML, then runs `resyncWorld` inside a single transaction:
per-table pre-load probe selects existing `external_id`s; rows not
yet in DB land; existing rows are left exactly as they are.

Boot log emits a `world: resync complete` line with per-table
new-row counts (`zones_new`, `rooms_new`, etc.) plus total YAML row
counts so an operator can see exactly what landed.

`LoadedWorld.ItemSpecsByZone` is built from parsed YAML regardless of
insert outcome, so `ZoneResetter` always has the recipe list.

Bootstrap starter: when no row sits at `repo.StarterRoomID` (id=1),
the YAML's `starter: true` row is inserted there FIRST (before any
auto-increment rooms grab id=1). When the slot is taken, the YAML
starter lands as a regular auto-increment row — first-loaded starter
wins; operators who want to swap starters must wipe id=1.

Mob_templates are gated as a bundle: if the `external_id` already
exists, template + instance + shop / banker / trainer / weave_teacher
/ dialogue / triggers are all skipped (refreshing aux blocks would
stomp operator edits or duplicate UNIQUE rows).

Also hosts `Restocker` (refills sub-max `shop_stock`, on
`tick.Buckets.AreaReset`), `ZoneResetter` (mob respawn from anchored
templates → door restoration via `ExitRepo.RestoreAuthored` → item
respawn via `ItemRepo.FindByExternalID` + `Create`), and
`Clock.HourOfDay`.

See `data/world/README.md` for the full zone.yaml schema.

## Mudlet client

Drop-in package under `clients/mudlet/`. `make mudlet-package` zips
the Lua files into `dist/mudlet/wheelmud.mpackage` and substitutes
HOST/PORT placeholders into `dist/mudlet/wheelmud.profile`.
`WHEELMUD_HOST`/`WHEELMUD_PORT` override the stamped profile
defaults. GMCP contract above governs Lua field names.
