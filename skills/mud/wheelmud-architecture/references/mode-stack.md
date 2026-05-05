# Mode stack

Per-session stack of `Mode` handlers. The dispatcher calls
`Mode.Handle(ctx, *Session, line)` synchronously per input line.

## Interface contract

- `Handle` is invoked by `runDispatcher`. The ctx is canceled when the
  read loop exits (EOF / idle / flood). Slow handlers must observe the
  ctx — a blocking handler stalls input for that session.
- `Handle` runs single-threaded per session; you do not need to lock
  Session fields the dispatcher owns.

## Push / Replace / Pop

- `PushMode(m)` — stack a new mode on top. Calls `ClearLastPrompt`.
- `ReplaceMode(m)` — swap the top of the stack. Calls
  `ClearLastPrompt`. Used by `postauth.promoteToGame` to swap from
  character-select into game.
- `PopMode()` — drop back to the prior mode. Calls `ClearLastPrompt`.

`ClearLastPrompt` runs on every transition because the new mode owns a
different prompt; replaying the old cache after a transition would
paint the wrong cue.

## Concrete modes

`internal/mode/`:
- `login.go` — username + password
- `account_create.go` — new account
- `character_select.go`
- `character_create.go` (+ `chargen_features.go`, `chargen_identity.go`,
  `chargen_render.go`)
- `game.go`
- `postauth.go` — promotion glue (`promoteToGame`)

## Adding a new mode

1. Define a struct implementing `Handle(ctx, *Session, line) error`.
2. Push or Replace from the current mode's `Handle`.
3. The first thing the new mode emits should be its prompt, via
   `WritePrompt`.
4. If the mode is multi-step (like chargen), the substep struct lives
   on the mode and `Handle` switches on it.
