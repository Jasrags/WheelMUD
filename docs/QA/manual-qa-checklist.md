# WheelMUD Manual QA Checklist

Walkthrough of every shipped feature. Connect via `telnet localhost 2323`
(or `nc localhost 2323`) and tick each row as you go.

## How to use

- **Status:** mark one of `[ ]` PENDING / `[P]` PASS / `[F]` FAIL / `[B]` BLOCKED / `[S]` SKIP.
- **Notes:** put repro, version, observed-vs-expected, or follow-up issue link.
- One row per testable behavior. If a row needs sub-cases, add them as
  indented children with their own checkbox.
- For multi-session tests, "Session A / Session B" means two telnet
  connections. For admin tests, log in as the first-ever character on
  the server (auto-promoted to AuthAdmin) or a character SQL-promoted
  to admin/builder.

Legend: **[ ]** = not run · **[P]** = pass · **[F]** = fail · **[B]** = blocked · **[S]** = skipped

---

## 1. Network & protocol

| # | Status | Test | Notes |
|---|--------|------|-------|
| 1.1 | [ ] | TCP listener accepts connection on `:2323` (default `LISTEN_ADDR`) | |
| 1.2 | [ ] | `LISTEN_ADDR=:2400 ./server` binds the alternate port | |
| 1.3 | [ ] | Server greets with splash + login prompt within ~1s of connect | |
| 1.4 | [ ] | Telnet IAC negotiation: client `WILL NAWS` is acknowledged; window resize updates wrap width | |
| 1.5 | [ ] | `TERM_TYPE` (RFC 1091) subnegotiation classifies xterm/xterm-256color/dumb correctly (`colors` shows level) | |
| 1.6 | [ ] | Literal `0xFF` byte in input is handled (IAC IAC escape) — no disconnect | |
| 1.7 | [ ] | Disconnecting mid-line (Ctrl+] then `quit` from telnet) cleans up — server logs `disconnect`, no goroutine leak warnings | |
| 1.8 | [ ] | `LOG_LEVEL=debug` shows per-connection lifecycle events | |

## 2. Terminal & rendering

| # | Status | Test | Notes |
|---|--------|------|-------|
| 2.1 | [ ] | `colors` command reports detected color level (None/Basic/16/256/TrueColor) | |
| 2.2 | [ ] | cfmt tags render: `say {{hello}}::red` → red text on color-capable client | |
| 2.3 | [ ] | Set client to monochrome / `TERM=dumb` → output strips ANSI cleanly (no `[31m` leaks) | |
| 2.4 | [ ] | NAWS resize: shrink terminal to 60 cols → next `look` wraps at ~60 | |
| 2.5 | [ ] | Long room description word-wraps without breaking words mid-letter | |
| 2.6 | [ ] | Truecolor: 24-bit gradients render on a TrueColor-capable terminal (e.g. iTerm2) | |
| 2.7 | [ ] | Cross-session broadcast (`say` from peer) does NOT clobber your half-typed input — line restored after the message | |
| 2.8 | [ ] | Prompt appears on its own line after every command response | |

## 3. Input loop & line editing

| # | Status | Test | Notes |
|---|--------|------|-------|
| 3.1 | [ ] | Type a verb, press Enter — command dispatches | |
| 3.2 | [ ] | Backspace removes last character; cursor visually retreats | |
| 3.3 | [ ] | DEL key behaves identically to Backspace | |
| 3.4 | [ ] | Tab autocompletes a unique verb prefix (e.g. `wh<Tab>` → `who`) | |
| 3.5 | [ ] | Tab on an ambiguous prefix lists candidates without committing | |
| 3.6 | [ ] | Tab autocompletes argument: `look <Tab>` shows mob/item names | |
| 3.7 | [ ] | Tab autocompletes a direction in `open <Tab>` to door names | |
| 3.8 | [ ] | Up/Down arrows scroll command history | |
| 3.9 | [ ] | Left/Right arrows + Home/End move cursor mid-line; insertions happen at cursor | |
| 3.10 | [ ] | Quoted args: `say "hello world"` keeps the quoted span intact | |
| 3.11 | [ ] | Login + character-create echo is suppressed during password prompts | |
| 3.12 | [ ] | Multi-command line: `look ; who` runs both in order | |
| 3.13 | [ ] | Quoted `;` is not split: `say "one; two"` stays one command | |
| 3.14 | [ ] | Alias add/remove: `alias gg get gold ; drop sword` then `gg` runs the chain | |

## 4. Command system

| # | Status | Test | Notes |
|---|--------|------|-------|
| 4.1 | [ ] | `help` lists registered commands organized by category | |
| 4.2 | [ ] | Unknown verb returns `Unknown command` (no enumeration of privileged verbs) | |
| 4.3 | [ ] | Privileged verb as a non-admin returns the same `Unknown command` text (no leak) | |
| 4.4 | [ ] | `quit` disconnects the session cleanly | |
| 4.5 | [ ] | Too-few-args verb prints usage block with the command's `Long` text | |
| 4.6 | [ ] | Argument completer: `give <Tab>` shows inventory item names | |

## 5. Mode / state stack

| # | Status | Test | Notes |
|---|--------|------|-------|
| 5.1 | [ ] | Login → game mode transition stamps CharacterID/Name/CurrentRoomID on session | |
| 5.2 | [ ] | Account-create flow reachable by typing `new` at the username prompt | |
| 5.3 | [ ] | Mode push/pop preserves the underlying mode's prompt cache | |

## 6. Accounts, auth & characters

### Login / account create
| # | Status | Test | Notes |
|---|--------|------|-------|
| 6.1 | [ ] | New account: `new` → username/password/confirm → reaches character menu | |
| 6.2 | [ ] | Login with correct credentials → character menu | |
| 6.3 | [ ] | Login with wrong password 5x triggers lockout; subsequent attempts are denied | |
| 6.4 | [ ] | Lockout window expires and login works again after the cooldown | |
| 6.5 | [ ] | Multi-session policy: log in as same account from two terminals → first session is kicked, second remains | |

### Post-login account menu
| # | Status | Test | Notes |
|---|--------|------|-------|
| 6.6 | [ ] | Account menu lists existing characters with last-played dates | |
| 6.7 | [ ] | `play <name>` enters the world as that character | |
| 6.8 | [ ] | `delete <name>` requires confirmation; deleted character no longer in list | |
| 6.9 | [ ] | `password` sub-menu changes account password (verify by relogging) | |
| 6.10 | [ ] | `settings` → color override applies on next promote-to-game | |
| 6.11 | [ ] | `settings` → prompt default flows into new character's prompt template | |
| 6.12 | [ ] | `settings` → width override applies on next promote-to-game | |
| 6.13 | [ ] | `settings` → `motd-always` flag re-shows MOTD/news on every login | |
| 6.14 | [ ] | `settings` → locale changes character-list date format | |
| 6.15 | [ ] | `security` shows last 10 login attempts (success/failure/lockout/kick) | |
| 6.16 | [ ] | `security` → `kick` removes a peer session (today: no-op for self-only) | |
| 6.17 | [ ] | `news` from account menu shows MOTD entries respecting `last_news_seen_at` | |

### Character creation (chargen)
| # | Status | Test | Notes |
|---|--------|------|-------|
| 6.18 | [ ] | `create` from character menu enters chargen | |
| 6.19 | [ ] | Chargen substep — abilities (point-buy V1) accepts valid spends, rejects over-budget | |
| 6.20 | [ ] | Chargen substep — race picker shows races; `info <race>` paginates flavor | |
| 6.21 | [ ] | Chargen substep — background picker; `info <bg>` shows feats/skills granted | |
| 6.22 | [ ] | Chargen substep — class picker (plain-English, no d20 jargon on row); `info <class>` shows full block | |
| 6.23 | [ ] | Chargen substep — identity (name, gender, age, height, weight, alignment) | |
| 6.24 | [ ] | Chargen substep — feats picker enforces 1st-level cap; `info <feat>` shows metadata + flavor | |
| 6.25 | [ ] | Chargen substep — skills picker enforces class cap; `info <skill>` shows description | |
| 6.26 | [ ] | Chargen substep — channeler branch only appears for Initiate/Wilder; non-channelers skip | |
| 6.27 | [ ] | Channeler: gender source auto-derives from Gender; affinities exactly 2 picks | |
| 6.28 | [ ] | Channeler: 3 weaves chosen from level-0 catalog filtered by affinity | |
| 6.29 | [ ] | Chargen finalize writes Character row with all selections persisted | |
| 6.30 | [ ] | Chargen back/cancel returns to previous substep without losing earlier picks | |
| 6.31 | [ ] | First character ever created is auto-promoted to AuthAdmin | |

## 7. Persistence

| # | Status | Test | Notes |
|---|--------|------|-------|
| 7.1 | [ ] | Fresh boot with empty `wheelmud.db` runs all migrations 0001–0036 without error | |
| 7.2 | [ ] | Inventory changes survive `quit` + reconnect | |
| 7.3 | [ ] | Coin balance survives reconnect; bank balance survives reconnect | |
| 7.4 | [ ] | Equipment slots survive reconnect | |
| 7.5 | [ ] | `last_played_at` updates on quit (autosave Save bucket) | |
| 7.6 | [ ] | World loader: edit `data/world/.../room.yaml` description, restart → change visible in-game | |
| 7.7 | [ ] | Item moved between rooms by another player persists across that player's disconnect | |
| 7.8 | [ ] | Concurrent coin mutation (sell + give from peer) — one succeeds, the other gets "balance just changed" | |

## 8. Game loop & scheduling

| # | Status | Test | Notes |
|---|--------|------|-------|
| 8.1 | [ ] | Tick scheduler runs (visible via `LOG_LEVEL=debug`, periodic ticks logged) | |
| 8.2 | [ ] | AreaReset bucket fires shop restock on its cadence (sub-max stock refills) | |
| 8.3 | [ ] | `time` verb advances per Clock tick | |
| 8.4 | [ ] | Mob wander (templates with `wander_chance > 0`) — mob appears in different rooms over time | |
| 8.5 | [ ] | Graceful shutdown: SIGINT triggers drain, autosave, exit code 0 | |

## 9. World model

| # | Status | Test | Notes |
|---|--------|------|-------|
| 9.1 | [ ] | `look` shows room name, description, exits, items, mobs | |
| 9.2 | [ ] | `look <object>` shows extra-desc text from YAML | |
| 9.3 | [ ] | `look in <container>` lists container contents | |
| 9.4 | [ ] | `examine <item>` shows typed taxonomy stats (weapon/armor/container) | |
| 9.5 | [ ] | Dark room (`light_level=0`) hides items/mobs from a non-light-bearing character | |
| 9.6 | [ ] | Phase ambient (sector-based) appears on entry / on tick | |
| 9.7 | [ ] | Zone listing: `zones` admin verb shows all loaded zones | |

## 10. Movement, look & navigation

| # | Status | Test | Notes |
|---|--------|------|-------|
| 10.1 | [ ] | Cardinals `n s e w` move between rooms | |
| 10.2 | [ ] | Diagonals `ne nw se sw` move where exits exist | |
| 10.3 | [ ] | Vertical `u d` move where exits exist | |
| 10.4 | [ ] | Closed door blocks movement; `open <dir>` then move works | |
| 10.5 | [ ] | Locked door blocks `open`; `unlock <dir>` with correct key works | |
| 10.6 | [ ] | `pick <dir>` admin/skill path opens locked door | |
| 10.7 | [ ] | `map` renders BFS minimap depth 3 (default) | |
| 10.8 | [ ] | `map 5` renders depth 5 (max) | |
| 10.9 | [ ] | `nomap` rooms are hidden from the player-facing minimap | |
| 10.10 | [ ] | `zonemap` shows the wider zone view | |
| 10.11 | [ ] | `coords show` displays room coordinates (admin) | |
| 10.12 | [ ] | `coords rebuild` reflows auto-derived coords (admin) | |
| 10.13 | [ ] | `track <mob>` shows recent mob_trail crumbs | |

## 13. Communication

| # | Status | Test | Notes |
|---|--------|------|-------|
| 13.1 | [ ] | `say hello` emits to room; peer in same room sees it | |
| 13.2 | [ ] | `tell <name> hi` reaches target; `reply hi back` round-trips | |
| 13.3 | [ ] | `shout` / `yell` reach all sessions in the zone | |
| 13.4 | [ ] | Channel verbs (per catalog row) broadcast to subscribers; non-subscribers don't see | |
| 13.5 | [ ] | `channels` lists all channels and current mute state | |
| 13.6 | [ ] | Mute toggling persists across reconnect | |
| 13.7 | [ ] | News/MOTD: unseen entries displayed on login; `news` re-displays | |

## 14. Inventory & economy

### Inventory
| # | Status | Test | Notes |
|---|--------|------|-------|
| 14.1 | [ ] | `inventory` shows owned items with `(worn)`/`(wielded)`/`(offhand)` annotations | |
| 14.2 | [ ] | `get <item>` from room → item moves to inventory | |
| 14.3 | [ ] | `drop <item>` → item lands in current room | |
| 14.4 | [ ] | `give <item> <name>` transfers to peer in same room | |
| 14.5 | [ ] | `put <item> in <container>` respects container capacity | |
| 14.6 | [ ] | `get <item> from <container>` works (recursive ownership) | |
| 14.7 | [ ] | Encumbrance bands respect Str + transitive container contents | |
| 14.8 | [ ] | Ordinal selectors: `2.sword` selects the second matching item | |
| 14.9 | [ ] | Dropping/giving an equipped item auto-unequips it | |

### Equipment
| # | Status | Test | Notes |
|---|--------|------|-------|
| 14.10 | [ ] | `wear <armor>` fills the right slot; `equipment` shows it | |
| 14.11 | [ ] | `wield <weapon>` fills main hand; `wield <weapon> off` fills off-hand | |
| 14.12 | [ ] | `remove <slot-or-item>` returns item to inventory | |
| 14.13 | [ ] | Two-handed weapon blocks off-hand equip | |

### Shops
| # | Status | Test | Notes |
|---|--------|------|-------|
| 14.14 | [ ] | `list` at a shopkeeper shows stock with prices | |
| 14.15 | [ ] | `buy <item>` deducts coin (with markup), grants the item | |
| 14.16 | [ ] | `sell <item>` credits coin (with markdown); shopkeeper accepts | |
| 14.17 | [ ] | `value <item>` shows expected sell price without selling | |
| 14.18 | [ ] | `FlagNoSell` items refused; `FlagTradeGood` sells at full price | |
| 14.19 | [ ] | Shop refuses items outside its `BuyTypes` | |
| 14.20 | [ ] | Restocker refills sub-max lines after `restock_interval_s` | |
| 14.21 | [ ] | Concurrent sell from peer: one succeeds, other gets coin-conflict message | |

### Banks
| # | Status | Test | Notes |
|---|--------|------|-------|
| 14.22 | [ ] | `balance` at a banker shows purse + bank | |
| 14.23 | [ ] | `deposit <coin>` moves coin from purse → bank | |
| 14.24 | [ ] | `withdraw <coin>` moves bank → purse | |
| 14.25 | [ ] | Banker refuses outside operating hours | |
| 14.26 | [ ] | Concurrent deposit/withdraw conflict surfaces "balance just changed" | |

## 17. Admin & moderation

| # | Status | Test | Notes |
|---|--------|------|-------|
| 17.1 | [ ] | Non-admin: `spawn` returns `Unknown command` | |
| 17.2 | [ ] | Admin: `spawn mob <ext>` creates a mob_instance in the current room | |
| 17.3 | [ ] | Admin: `spawn mob <ext> 3` creates 3 instances | |
| 17.4 | [ ] | Admin: `spawn item <ext> [count]` creates item(s) on the floor | |
| 17.5 | [ ] | Admin: `teleport <name>` moves you to that character | |
| 17.6 | [ ] | Admin: `whereami` shows current room id, zone, coords | |
| 17.7 | [ ] | Admin: `zones` lists all zones | |
| 17.8 | [ ] | Admin verb success writes one row to `admin_audit`; refusal does not | |
| 17.9 | [ ] | Account-menu admin actions write `admin_audit` rows with `actor_type='account'` | |
| 17.10 | [ ] | `shutdown 30` broadcasts countdown via WriteAsync; commits an audit row before drain | |
| 17.11 | [ ] | `reboot 30` re-execs server; same audit + drain behavior | |
| 17.12 | [ ] | `shutdown abort` cancels an in-flight countdown | |

## 18. Help & docs

| # | Status | Test | Notes |
|---|--------|------|-------|
| 18.1 | [ ] | `help` index lists categories | |
| 18.2 | [ ] | `help <verb>` shows the `Long` text for that verb | |
| 18.3 | [ ] | `news` lists news entries; new entry on next boot is shown automatically | |

## 19. Logging & ops

| # | Status | Test | Notes |
|---|--------|------|-------|
| 19.1 | [ ] | `LOG_LEVEL=debug` shows command dispatches | |
| 19.2 | [ ] | `LOG_LEVEL=warn` suppresses debug/info | |
| 19.3 | [ ] | Panic in a goroutine wrapped by `safego.Go` logs and recovers (no crash) | |

## 20. Configuration

| # | Status | Test | Notes |
|---|--------|------|-------|
| 20.1 | [ ] | `LISTEN_ADDR` override works | |
| 20.2 | [ ] | `DB_DSN=:memory:` boots with ephemeral DB | |
| 20.3 | [ ] | `WORLD_DIR=/path/to/alt` loads alternate world tree | |
| 20.4 | [ ] | `CHARGEN_DIR=/path/to/alt` loads alternate chargen catalog | |

## 22. Packaging & deploy

| # | Status | Test | Notes |
|---|--------|------|-------|
| 22.1 | [ ] | `make build/server` produces a binary | |
| 22.2 | [ ] | `make run/server` boots and listens | |
| 22.3 | [ ] | `make run/live/server` reloads on file save (air) | |
| 22.4 | [ ] | `docker compose up` boots and exposes `:2323` | |

---

## Regression notes / blockers

Use this section for cross-cutting issues that don't fit a single row
(e.g. "all chargen substeps wrap awkwardly at 80 cols"). Reference row
numbers above so we can wire fixes back to specific test cases.

| Date | Area | Issue | Resolution |
|------|------|-------|------------|
|      |      |       |            |
