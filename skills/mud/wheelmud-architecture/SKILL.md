---
name: wheelmud-architecture
description: Authoritative structural reference for the WheelMUD Go codebase — session/mode/repo/bus wiring, write paths, migration rules, command authoring, world loader, auth/audit. Indexes `CLAUDE.md` and `docs/CODEMAPS/*`. Cited by every skill that produces code or schema changes.
triggers:
  - session
  - mode stack
  - PushMode
  - PopMode
  - ReplaceMode
  - WriteString
  - WriteWrapped
  - WriteAsync
  - WritePrompt
  - EditAndWrite
  - crossMu
  - ClearLastPrompt
  - repo
  - migration
  - column lock-step
  - eventbus
  - tick
  - persist
  - safego
  - telnet
  - cfmt
  - AuthLevel
  - admin_audit
  - world loader
  - chargen catalog
  - MOTD
  - escape sequences
  - optimistic lock
  - coin_version
  - ItemRepo Transfer
  - ErrItemMoved
---

# wheelmud-architecture

## Role

The structural source of truth for **our** code. When any other skill (ui-expert,
world-builder, wot-d20-rules, content skills) is about to produce a code change,
schema change, or new module, it consults this skill first to keep the change
in lock-step with the existing layered architecture.

This skill is **read-mostly**. It does not invent mechanics, write room
descriptions, or pick names. It indexes `CLAUDE.md`, `docs/CODEMAPS/*`,
`ROADMAP.md`, and `docs/PLAN.md` and surfaces the rules that govern where
new work lands.

## Core expertise

- **Wiring** — `cmd/server/main.go::server`: which deps are long-lived,
  which are per-connection, how new repos/managers slot into the `server`
  struct.
- **Telnet/session layer** — write paths (`WriteString` cfmt vs `WriteRaw`
  vs `WriteWrapped` vs `WriteAsync` vs `WritePrompt` vs `EditAndWrite`),
  the cross-session output rule, prompt cache discipline, `crossMu`-guarded
  fields and their helpers.
- **Mode stack** — `Mode` interface contract, `PushMode` / `ReplaceMode`
  / `PopMode`, automatic `ClearLastPrompt` on transitions, ctx-cancel
  observance in slow handlers.
- **Repos + migrations** — column-list lock-step (rooms, items, characters
  raw-SQL paths in `internal/world/loader.go` and the `Create` paths in
  `internal/repo/*_sqlite.go` move together), forward-only migration
  policy, optimistic-lock columns (`coin_version`, `version`),
  `ItemRepo.Transfer*` ownership invariant (`room_id ⊕ owner_character_id
  ⊕ parent_item_id`).
- **Command authoring** — `*telnet.Command` shape, `MinArgs` + `Long`
  usage discipline, `audit.Record` discipline (only on success, never on
  refusal), `safego.Go("name", fn)` for goroutines, semicolon segment
  rules (`Tokenize` + `SplitOnSemicolon` + `maxSegmentsPerLine`).
- **World YAML loader** — links to `data/world/README.md`; raw-SQL inserts
  inside one transaction, why the column lists are duplicated.
- **Auth + audit** — `AuthLevel` lives on `characters`, not accounts;
  promotion in `mode/postauth.promoteToGame`; refusal paths MUST NOT
  audit; first-character-on-server is auto-AuthAdmin.
- **Tick / eventbus / persist / safego** — typed pub/sub, named tick
  buckets, periodic+shutdown autosave layering, panic-recovered goroutine
  wrapper.

## Approach

When invoked:

1. Identify the question class: write path? new column? new command? new
   migration? new mode? new long-lived dep? cross-session output? auth
   gating?
2. Open the matching reference file (see References below).
3. If the change involves a column on `rooms`, `items`, or `characters`,
   surface the lock-step list (raw-SQL loader cols + repo `Create` cols
   + scan dest) and verify they all change together.
4. If the change involves cross-session output, enforce the WriteAsync
   rule and confirm the prompt cache is still hot at every point.
5. If the change involves a new admin verb, confirm the `audit.Record`
   call lands on the success path only.
6. Cite `CLAUDE.md` "Things to watch when editing" line ranges and
   `docs/CODEMAPS/<area>.md` for any new contributor walking in cold.

## Clarifying questions

- Is this a new mechanic (defer to content skills + PLAN.md) or a
  structural change (this skill)?
- Does it add a new column? to which table? have you checked the
  loader-side INSERT?
- Does the new code emit text to a session other than the dispatcher's
  `c.Session`? if so, are you using `WriteAsync`?
- Does the new code spawn a goroutine? if so, are you using `safego.Go`?
- Is the verb privileged? what `AuthLevel`? does success need to audit?

## Output formats

- **File-line-keyed pointers** — `cmd/server/main.go:NNN`,
  `telnet/server.go:NNN`, `internal/repo/character_sql.go:NNN`.
- **Lock-step checklists** — when adding a column, the literal list of
  files/symbols that must change in the same PR.
- **Negative examples** — "do NOT do X here, because Y".
- **Cross-references** — to `CLAUDE.md`, `docs/CODEMAPS/*`, the relevant
  migration number.

## Dependencies

None — this is the foundation skill. Other skills depend on it.

## Anti-triggers

- Does NOT design new mechanics or content.
- Does NOT write room descriptions, names, or flavor.
- Does NOT decide *whether* to implement a feature (`docs/PLAN.md` does).
- Does NOT propose down-migrations (the project is forward-only;
  see `persistence_followups.md` memory).
- Does NOT bypass `WriteAsync` for cross-session output, even for
  "simple" notifications.

## References

- `references/architecture-tour.md` — top-level wiring; long-lived deps;
  where to add the next.
- `references/session-and-write-paths.md` — `WriteString` /
  `WriteWrapped` / `WriteAsync` / `WritePrompt` / `EditAndWrite` rules;
  cross-session output rule; `crossMu`-guarded fields.
- `references/mode-stack.md` — `Mode` interface; push/pop/replace
  semantics; ctx-cancel; `ClearLastPrompt` discipline.
- `references/repo-and-migration-rules.md` — column lock-step (rooms,
  items, characters); forward-only; optimistic-lock columns;
  `ItemRepo.Transfer*` invariant; `ErrItemMoved` / `ErrCoinConflict`
  pattern.
- `references/command-authoring.md` — `*telnet.Command` shape;
  `MinArgs`+`Long`; `audit.Record`; `safego.Go`; semicolon chaining.
- `references/world-yaml-conventions.md` — links to
  `data/world/README.md`; room-id form; currency strings; typed item
  stats; shop/banker sub-blocks.
- `references/auth-and-audit.md` — `AuthLevel` placement; promotion in
  `postauth`; refusal paths; first-admin bootstrap; audit-on-success rule.
- `references/codemaps-index.md` — pointers into `docs/CODEMAPS/*`.
