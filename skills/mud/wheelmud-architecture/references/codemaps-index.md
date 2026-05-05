# Codemaps index

Token-lean architecture maps for AI context live in `docs/CODEMAPS/`.
Cite these from skill output instead of pasting large code excerpts.

| File | Covers |
|---|---|
| `docs/CODEMAPS/architecture.md` | Top-level wiring (`server` struct, long-lived deps, mode stack) |
| `docs/CODEMAPS/commands.md` | Verb catalog + factory signatures |
| `docs/CODEMAPS/data.md` | Repo/migration map; column lock-step lists |
| `docs/CODEMAPS/dependencies.md` | Third-party module surface (`cfmt`, `golang.org/x/crypto`, `gopkg.in/yaml.v3`, `modernc.org/sqlite`) |
| `docs/CODEMAPS/telnet.md` | Telnet protocol + session + write paths |

## Refresh policy

When a phase finishes or a structural change lands, regenerate the
relevant codemap. They are AI-context artifacts, not human docs — keep
them token-lean and structural, not narrative.

## Source priority

When a skill's reference and the codemap disagree about *current* state:
- Codemap wins for "what's wired today."
- Skill reference wins for "the rule that governs adding the next one."
- `CLAUDE.md` "Things to watch when editing" wins for invariants.
