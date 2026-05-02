# Contributing

## Workflow

1. Pick the next slice from [`ROADMAP.md`](ROADMAP.md). The roadmap is the
   source of truth — items are tagged `[ ]` (todo), `[~]` (in progress /
   partial), `[x]` (done).
2. Land tests with the change. The repo is on `go test -race ./...` and
   has dual-implementation tests (memory + sqlite) for every repo.
3. Keep commits focused. The history follows Conventional Commits:
   `feat(scope): ...`, `fix(scope): ...`, `refactor(...)`, `docs(...)`,
   `test(...)`, `chore(...)`. Scopes match top-level packages
   (`telnet`, `repo`, `cmd`, `mode`, `world`, `persist`, ...).
4. Update [`ROADMAP.md`](ROADMAP.md) and the relevant codemap in
   [`docs/CODEMAPS/`](docs/CODEMAPS/) when shipping a slice.

## Style

- `gofmt` + `go vet` are mandatory. Long-form style notes live in
  [`docs/CODEMAPS/`](docs/CODEMAPS/) and the `CLAUDE.md` "Things to
  watch when editing" section.
- Errors wrap with context: `fmt.Errorf("verb noun: %w", err)`.
- Repos accept `context.Context` and parameterized SQL only.
- Long-lived goroutines go through `safego.Go("name", fn)` so panics
  log instead of crashing the process.
- Cross-goroutine `*telnet.Session` fields go through the `crossMu`
  helpers (`SetLastTellFrom` / `StampInput` / `SetChannelMuted` / ...) —
  never read or write the unexported fields directly.

## Tests

```bash
go test -race ./...                 # full suite, default before commit
go test -race ./internal/repo/...   # repo-only
go test -run TestChannel ./...      # focused
```

Repo tests use a shared `runXxxRepoTests` table that runs against both
memory and sqlite implementations. New repo methods need both impls + a
case in the shared table.

## Migrations

- Add a numbered file under `internal/db/migrations/` (e.g.
  `0012_add_quests.sql`) with a `-- +migrate up` header.
- Migrations are forward-only and run at startup via
  `internal/db.Open`. There is no down path yet.
- New columns on `characters` need to land in lock-step in
  `internal/repo/character_sql.go` (`charCoreColumns` /
  `charPlayerColumns` + matching `Values` / `ScanDest` slices).
  Ordering is load-bearing — adding to one without the others
  silently corrupts scans.

## World data

Authored zone YAML lives under `data/world/`. The world loader
(`internal/world/`) reads `WORLD_DIR` (default `./data/world`) on every
boot and upserts into the DB. Adding a new zone is a YAML edit, no code
change required.

## Code review

Run `/code-review` (or invoke the `code-reviewer` agent) on uncommitted
changes before committing. Security-sensitive paths (auth, channels,
admin commands, anything taking player text into a cfmt template) get
an extra security pass.
