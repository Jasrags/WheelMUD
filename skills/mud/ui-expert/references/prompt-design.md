# Prompt design

WheelMUD has four distinct prompt contexts. Each behaves differently and
must be driven through `Session.WritePrompt` so the dispatcher, mode
stack, and `WriteAsync` repaint logic stay coherent.

## The four contexts

| Context | Source | Example |
|---|---|---|
| **Chargen** | per-substep, pushed by chargen render helpers | `[step 5/8 abilities] » ` |
| **Password** | `internal/mode/login.go` under `Session.SetPasswordMode(true)` | `Password: ` |
| **Pager** | (deferred — see `pager_followups.md` memory) | `--more-- (q/space)` |
| **Game** | per-character `prompt_template` from migration 0023, expanded by `internal/prompt/` | `[100/100] >` |

## Game-prompt grammar

Source: `internal/cmd/prompt.go` help block (keep this reference in
lock-step with that help text):

| Token | Meaning | Status |
|---|---|---|
| `%h` / `%H` | current / max hit points | live |
| `%r` | current room name | live |
| `%g` | carried coin (e.g. `5gc 2sp`) | live |
| `%%` | literal `%` | live |
| `%m` / `%M` | mana / saidin / saidar pool | **reserved — renders 0** |
| `%v` / `%V` | move / fatigue | **reserved — renders 0** |
| `%t` | combat target | **reserved — empty** |

`prompt set [{{%h}}::red/%H] %r$ ` is the example shipped with the help
block — colors come from cfmt tags, not new prompt grammar.

### Wiring reserved tokens

When the underlying system lands:

- `%m/%M` — wire from `creature.Channeling.PoolCurrent / .PoolMax`
  during Phase D (one-power-systems).
- `%v/%V` — wire from a fatigue field on `creature.Core` once Phase C
  combat introduces it.
- `%t` — wire from a combat-state field on the character once the
  combat round model exists.

Don't add new tokens here without first extending the grammar in
`internal/prompt/`.

## Default game prompt

Recommendation, after chargen polish lands:

```
[{{%h}}::green/{{%H}}::green|bold] »
```

The static template stays plain. Color transitions for low-HP warnings
(yellow ≤ 50%, red ≤ 25%) belong in render code that runs after token
expansion — not in the template string. Keep the template grammar
predictable.

## Chargen prompt

```
{{[step %d/%d %s]}}::cyan »
```

Pushed via `writeStepPrompt` immediately after each substep menu render.
Mode transitions clear the cache automatically (`PushMode` / `ReplaceMode`
call `ClearLastPrompt`); the helper just re-pushes on the new step.

## Password prompt

`internal/mode/login.go` calls `Session.SetPasswordMode(true)` then
prompts. Don't decorate this with cfmt — keep it minimal and let the
session's password-mode echo suppression work without surprises.

## Pager prompt

Deferred. When the pager lands (see `pager_followups.md`), the prompt
will look like `--more-- ({{space}}::yellow next, {{q}}::yellow quit)`.

## Cross-session repaint contract

`WriteAsync` (incoming tell, channel msg, MOTD on switch) emits a CR+EL
prefix to clear the current line, writes the async message, then
replays the cached prompt and the in-progress edit buffer. This relies
on `Session.WritePrompt` having been called for the current context —
without it, `WriteAsync` repaints nothing and the player loses their
prompt cue mid-line.

**Rule:** every time a mode emits a "now waiting on input" state, call
`Session.WritePrompt` with the appropriate prompt string. Chargen
substeps included.
