# Command authoring

How to add a new verb to `internal/cmd/`.

## Shape

Each command factory takes its dependencies (repos, registry, sessions,
bus, audits) by parameter and returns a `*telnet.Command`:

```go
func NewLook(rooms repo.RoomRepo, items repo.ItemRepo, ...) *telnet.Command {
    return &telnet.Command{
        Name:    "look",
        Aliases: []string{"l"},
        Auth:    telnet.AuthPlayer,
        Short:   "Look around or at something.",
        Long:    "Usage: look [<target>]\n  look in <container>",
        MinArgs: 0,
        Run: func(c *telnet.CmdCtx) error { ... },
    }
}
```

Wire the factory in `cmd/server/main.go::buildRegistry`.

## MinArgs + Long

If the verb requires arguments, set `MinArgs >= 1` AND populate
`Long`. The dispatcher emits a Long-aware usage block on too-few-args
automatically; you do not write the usage string by hand inside `Run`.

## Auth gating

`Registry.Dispatch` enforces `Command.Auth` against
`Session.AuthLevel`. Privilege-denied lookups return the same
`Unknown command` text as a missing verb so the prompt cannot
enumerate privileged commands.

`AuthLevel` lives on the `characters` row. The session is `AuthGuest`
through login + account-create; it's stamped by
`mode/postauth.promoteToGame`.

## Audit on success only

Privileged verbs (`spawn`, `teleport`, `goto`, `transfer`, `summon`,
`wizinvis`, `shutdown`, `reboot`, …) record one `admin_audit` row per
**successful** invocation:

```go
audit.Record(c.Ctx, audits, c.Session, verb, target, args)
```

Refusal paths (auth denied, bad target, NoTeleport, controller error,
unknown template) **MUST NOT** audit — the row represents "this side
effect actually happened." Synchronous by design so a `shutdown` row
commits before drain begins.

Tests that don't care about the audit assertion pass `nil` for
`audits`.

## Goroutines

Spawn long-lived goroutines via `safego.Go("name", fn)` so panics
surface as warnings instead of taking down the process. Never bare
`go fn()` for anything that lives past the current handler.

## Cross-session output

Reread the WriteAsync rule before emitting any text to a session
other than `c.Session`. See `session-and-write-paths.md`.

## Semicolon chaining

`Registry.Dispatch` is segment-aware: a top-level `;` outside quotes
splits input via `telnet.SplitOnSemicolon`. Commands that consume
`c.Raw` (e.g. `say`, `tell`, `shout`) get the same `Raw` they would
have without chaining — `say "hello; world"` stays one command.
Lookup errors and Run errors don't abort the chain; the first Run
error is returned. Hard caps: `maxSegmentsPerLine = 16`,
`maxAliasDepth = 3`.

## Keyword resolution

Item/mob keyword resolution (including ordinal `2.sword`) goes through
`internal/cmd/keyword.go::MatchItem` / `MatchMob`. Do not roll your
own substring match.
