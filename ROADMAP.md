# WheelMUD Roadmap

A living checklist of the components a modern MUD typically ships with, annotated
with what already exists in this repo. Status legend:

- `[x]` Done — landed on `main`
- `[~]` Partial — scaffolded or wired in a limited form, more work tracked below
- `[ ]` Not started

The "Notes" under each section call out concrete files when something exists, so
the doc stays anchored to code rather than drifting into wishlist territory.

---

## 1. Network & protocol

- [x] TCP listener on `:2323` (`LISTEN_ADDR` overridable)
- [x] Telnet IAC negotiation: `WILL/WONT/DO/DONT`, `SB ... SE`
- [x] Subnegotiation handlers for `TERM_TYPE` (RFC 1091) and `NAWS` (RFC 1073)
- [x] Per-connection goroutine + protocol parser
- [x] `IAC IAC` escape (literal 0xFF in stream) and standalone `IAC GA` / `IAC NOP` / etc.
- [ ] **MCCP2/3** (Mud Client Compression Protocol) — negotiate option 86
      (MCCP2) on connect; on `IAC SB MCCP2 IAC SE` start a `zlib.Writer`
      around `Session.WriteRaw` and flush per write. MCCP3 (option 87)
      adds upstream compression — lower priority. Track compressed-byte
      counters for telemetry.
- [x] **GMCP** (Generic MUD Communication Protocol) — Phase I #46,
      landed 2026-05-14. Option 201 negotiation in
      `telnet/iac.go`; `internal/gmcp.Manager` owns inbound Core.*
      dispatch (Hello / Ping / Supports.Set/Add/Remove) and per-
      session eventbus subscription lifecycle. V1 outbound: Char.Name,
      Char.Vitals (deduped), Char.Status, Room.Info, Comm.Channel.Text
      (covers `say`, every chat channel, and tells via new
      `world.ChannelBroadcast` + `world.PlayerTold` events). Initial
      snapshot fires on Core.Supports.Set. Char.Items.* deferred to
      V2 pending inventory-change events on `get`/`drop`/`wear`/
      `unwear`. MSSP `GMCP=1` flipped.
- [ ] **MSDP** (MUD Server Data Protocol) — option 69. Simpler key/value
      protocol than GMCP; useful for clients that don't speak JSON.
      Share the publish layer with GMCP behind a common `OOB` interface
      so handlers report variables once.
- [x] **MSSP** (MUD Server Status Protocol) — option 70. Phase I #45,
      landed 2026-05-14. `telnet/mssp.go::EncodeMSSP` builds the
      wire block; `cmd/server/main.go::msspVars` produces the full
      crawler variable set (NAME, PLAYERS, UPTIME, world stats from
      boot snapshot, capability flags, public strings from the new
      `mssp:` config block). Response wired through
      `telnet/iac.go::handleOptionNegotiation` on inbound `DO MSSP`.
- [ ] **MXP** (MUD eXtension Protocol) — clickable links, server-pushed
      UI. Negotiate option 91, wrap output in `<send>`/`<a>` tags via
      a `MXPWriter` shim that no-ops when not negotiated. Most value
      comes from making exits and `who` entries clickable.
- [x] **CHARSET / UTF-8 negotiation** (RFC 2066, option 42) — Phase I
      #44, landed 2026-05-14. Server sends `WILL CHARSET` on connect;
      on client `DO CHARSET` we offer `;UTF-8`. `Session.Charset`
      (crossMu accessor pair) drives `WrapText`'s display-cell vs.
      rune-count branch. Wide-glyph wrap (§2) closed in the same PR
      via `github.com/mattn/go-runewidth`. MNES envelope deferred.
- [ ] TLS listener (telnet-over-TLS on a second port) — second
      `net.Listener` from `tls.Listen` on `:2992`, wrap conn before
      handing to `RunSession`. Cert path via `TLS_CERT`/`TLS_KEY` env;
      auto-reload on SIGHUP for cert rotation.
- [ ] WebSocket gateway (browser clients) sharing the session layer —
      `gorilla/websocket` upgrader behind an HTTP handler, adapt to a
      `net.Conn` shim so `RunSession` is reused unchanged. Frame each
      newline-terminated chunk as one text message; binary frames
      reserved for GMCP.
- [ ] SSH listener (optional, for ops/admin shell) — `gliderlabs/ssh`
      with a public-key allowlist from `ssh_admins` table. Sessions
      open with `AuthLevel=Admin` and a special `Mode` skipping the
      login prompt; intended for emergency console only.
- [x] **Mudlet client package** (v1) — landed 2026-05-14.
      `clients/mudlet/` ships a `.mpackage` plus a connection
      profile that consume the V1 GMCP packages: HP/SP gauges
      (Char.Vitals), character header (Char.Name + Char.Status),
      auto-mapping (Room.Info), per-channel chat panes
      (Comm.Channel.Text). Build with `make mudlet-package`
      (optionally `WHEELMUD_HOST=` / `WHEELMUD_PORT=` overrides);
      drop both artifacts into Mudlet's File → Import + Package
      Manager → Install. Stretch items (triggers / alias
      hotkeys / affect tracker) deferred to v2.

Notes: parser lives in `telnet/server.go::bufferInput` and `RunSession`; option
constants and helpers in `telnet/iac.go`.

## 2. Terminal & rendering

- [x] ANSI SGR constants (`telnet/color.go`)
- [x] Color-level detection from `TERM` (`None`/`Basic`/`16`/`256`)
- [x] `cfmt`-based `{{text}}::style` rendering through `Session.WriteString`
- [x] NAWS-driven width/height tracking on `Session`
- [x] Truecolor (24-bit) path — `ColorLevelTrueColor`, `RenderRGBFG`/`RenderRGBBG`
      with 256-color and 16-color downsampling fallbacks (`telnet/color.go`)
- [x] ANSI-aware word-wrap respecting `Session.Width` (`telnet/wrap.go`,
      `Session.WriteWrapped`)
- [x] Output write lock — `Session.writeMu` serializes `WriteRaw`
- [x] Pager mode for output that exceeds `Session.Height` — Phase A
      #2 closed 2026-05-11. `telnet/pager.go` implements `pagerMode`
      pushed via `Session.WritePaged` / `WritePagedWrapped`; wired
      into `help`, `news`, `who`, `examine`, `inventory`, `quest`,
      and `zonemap`. Space/enter advances, q quits.
- [x] Prompt templating (HP/MP/room placeholders) — `internal/prompt`
      renders `%h/%H` (HP), `%r` (room), `%g` (gold), `%%` literal,
      with `%m/%M`/`%v/%V`/`%t` reserved for mana, move, and combat
      target once those systems land. `Game.Prompt` invokes it once
      per dispatch with `FindByName`-fetched character + optional
      `RoomRepo.FindByID` (only when `%r` is in the template). Server
      default: `"<%h/%H hp> "`. cfmt color tags (`{{...}}::red`) are
      rendered before write; interpolated `%r`/`%g` values are
      defanged to prevent style injection. Per-character override via
      the `prompt set/clear/show` command (migration 0023 added the
      `characters.prompt_template` column).
- [x] Width-aware **wrap** (CJK fullwidth, combining marks) — Phase
      I #44 (CHARSET), landed 2026-05-14. `telnet/wrap.go::WrapText`
      gained a `cellWidth bool` arg; when true (UTF-8 negotiated via
      CHARSET) column accounting switches to `runewidth.StringWidth`
      so CJK glyphs count as 2 cells and zero-width joiners as 0.
      `Session.WriteWrapped` / `WritePagedWrapped` pass
      `Charset() == "UTF-8"`. Regression test
      `TestWrapText_CellWidthCountsCJKAsTwo`.
- [x] Width-aware **cursor accounting** in `LineEdit` and the byte
      dispatcher — landed 2026-05-14. `telnet/lineedit.go`
      primitives (Insert / InsertRune / Backspace / Delete /
      Move* / Kill* / Replace) walk by rune via
      `utf8.DecodeRune` / `DecodeLastRune` and emit cell-count BS
      via `runewidth.RuneWidth` / `StringWidth`. The byte
      dispatcher (`telnet/server.go::dispatchByte`) accumulates
      UTF-8 continuation bytes in `Session.utf8Pending` / `utf8Have`
      and hands a complete rune to `LineEdit.InsertRune` — one
      call per glyph, not three per byte. Password mode echoes one
      `*` per glyph; `extendBuffer` (tab-completion repaint) uses
      `runewidth.StringWidth` for the BS count; `WriteAsync`'s
      masked redraw counts asterisks by rune. Coverage: 9 CJK
      lineedit tests + 6 dispatcher/echo tests + 1 combining-mark
      regression. Deferred: grapheme-cluster awareness — a `é`
      typed as `e + COMBINING ACUTE` still edits in two
      backspaces (one per codepoint). Closes
      `terminal_rendering_followups.md` item #2.
- [x] Long-token break in `WrapText` — landed 2026-05-14 alongside
      the CHARSET work. Tokens whose cell width exceeds the wrap
      width are now split into successive width-cell chunks
      separated by newlines; bare cut (no hyphen) so URLs stay
      copy-pasteable. Pathological case (width=1 with a 2-cell
      glyph) overflows by one cell to guarantee forward progress;
      documented in the godoc. Coverage:
      `TestWrapText_BreaksLongToken*` family.

## 3. Input loop & line editing

- [x] Byte-at-a-time read loop with CR/LF line termination
- [x] Backspace / DEL handling
- [x] Tab autocomplete (verb-only) via `telnet/completion.go` + `Game.Complete`
- [x] Password-mode echo suppression — driven by Login / Create mode lifecycle
      (`Session.InPasswordMode` flipped on entering / leaving password substep)
- [x] Argument-side tab completion via `Command.Completer`; `Game.Complete`
      delegates to the matched command once whitespace appears, with
      AuthLevel filtering so privileged verbs can't be enumerated
- [x] Quoted-argument tokenization (`telnet/tokenize.go`) — `Tokenize`
      replaces `strings.Fields` in `Registry.Dispatch`; supports double,
      single, and bare-backslash escapes; unbalanced quotes surface as
      "Unbalanced quote"
- [x] Command history with up/down arrows — `History` ring on `Session`
      (`telnet/history.go`), CSI parser (`telnet/csi.go`) recognizes
      ESC[A/B and SS3 ESC O A/B; password-mode lines never enter history
- [x] In-line cursor movement — `LineEdit` model (`telnet/lineedit.go`)
      handles ←/→, Home/End (ESC[H/F, ESC[1~, ESC[4~), forward-delete
      (ESC[3~), Ctrl-A/E/U/W/K, mid-line insert/backspace; password mode
      bells on every motion key
- [x] Aliases at the user level — `AliasTable` on `Session`,
      `alias`/`unalias` commands, single-pass expansion in
      `Registry.Dispatch` so chained aliases don't recurse

## 4. Command system

- [x] Command registry with aliases, prefix lookup, ambiguity detection (`telnet/command.go`)
- [x] Per-session dispatcher and `Mode` stack (`telnet/mode.go`, `telnet/session.go`)
- [x] `Game` mode wrapping the registry (`internal/mode/game.go`)
- [x] Sample commands: `quit`, `who`, `help`, `colors`
- [x] `AuthLevel` enforcement in `Registry.Dispatch` — denials render as
      `"Unknown command"` so privileged verbs can't be enumerated
- [x] Persisted AuthLevel on the **character** row (migrations 0018/0019;
      0018 introduced it on accounts, 0019 moved it to characters so a
      single account can own admin and player characters side-by-side).
      `CharacterRepo.Create` atomically promotes the very first
      character on the server to AuthAdmin; `mode/postauth.promoteToGame`
      stamps `s.AuthLevel` from the chosen character. Boot-time
      data-integrity audit added 2026-05-14:
      `CharacterRepo.ClampInvalidAuthLevels` runs after repo
      construction in `cmd/server/main.go` and clamps any row with
      `auth_level > AuthLevelMax` back into range, logging a warn
      with the row count. The 0019 CHECK constraint forbids new
      writes outside [0,2], but hand-edited DB rows or rows
      predating the constraint were tripping the post-load scan
      validator with "invalid auth_level <N>" and locking the
      owning account out of character select. Recovery hatch on
      every boot.
- [x] Per-command argument completer — `Command.Completer` field;
      `help` ships a real one (sample: tab on `help <prefix>`)
- [x] Broaden argument completers across the command catalog —
      `internal/cmd/door.go` ships `completeExits` (visible exits
      from the actor's room, hidden filtered) on `open`/`close`/
      `lock`/`unlock`/`pick`; `teleport` unions room external IDs +
      online peer names on slot 0 and room IDs only on slot 1;
      `get` completes floor item keywords, `drop` completes
      inventory keywords, `give` does inventory keyword on slot 0
      then online name on slot 1; `examine` unions room mobs +
      room items + inventory items; `tell` completes online names
      with self/higher-auth peers filtered out. Ordinal `2.sword`
      syntax flows through `splitOrdinalPartial` so the typed
      ordinal is preserved on each candidate's replacement text.
      Combat-verb completers and 3-arg "get sword from chest" forms
      will land alongside their respective features. `reply` takes
      a free-form message and bells. Channel verbs continue to
      self-name (no completer needed).
- [~] Command cooldowns / lag system (combat balance lever). Phase E
      #26 slice A landed 2026-05-09. `Command.Lag time.Duration` field
      on every verb; `Session.NextReady time.Time` (crossMu-guarded);
      gate placed in `Registry.dispatchOne` (per-segment, NOT in
      `Dispatch`) so chained `;` inputs gate correctly. Refuse-with-
      message V1 (`{{You're too busy. (~Ns)}}::yellow`) — bounded-
      queue promotion is a single dispatcher swap on the same wire
      shape. Stamp on success only (`StampLag` runs after a
      `cmd.Run` returning nil, never on Run error). Wired to
      `attack`/`kill`=3s, `flee`=2s, `parry`=1s, `shout`/`yell`=2s.
      Deferred: bounded queue, movement lag (waits on §9 sector
      cost table), say/tell lag (anti-RP — only zone broadcasts
      lag V1).
- [x] Macro / multi-command lines (`;` separator) —
      `telnet.SplitOnSemicolon` (quote/escape-aware) splits the raw
      line; `Registry.Dispatch` dispatches each segment in order via
      `dispatchOne` (alias-expand + lookup + auth + tokenize + run).
      Continues past lookup errors and Run errors (first Run error is
      returned). Cap of 16 segments per line with a truncation notice;
      alias-expansion-introduces-`;` is depth-bounded.
- [ ] User-defined aliases stored on the character — promote the
      in-memory `AliasTable` from §3 to a `character_aliases` table
      (`character_id`, `name`, `expansion`, unique on `(character_id,
      name)`), load on character promote, persist on `alias`/`unalias`,
      cap per-character count (e.g. 64) and expansion length.

## 5. Mode / state stack

- [x] `Mode` interface with `OnEnter`/`OnExit`/`Handle`/`Prompt`/`Complete`
- [x] `PushMode`, `ReplaceMode`, `PopMode` on `Session`
- [x] `PushMode` / `ReplaceMode` roll back on `OnEnter` error
- [x] `context.Context` plumbed through `Mode.Handle` — canceled when read
      loop exits so blocking I/O in handlers can observe teardown
- [x] Game mode (`internal/mode/game.go`)
- [x] Login mode (`internal/mode/login.go`) — see §6
- [x] Character-select + character-create modes (`internal/mode/character_*.go`)
- [x] Account-create mode (`internal/mode/create.go`)
- [ ] OLC editor modes (room editor, mob editor, etc.) — one mode
      per aggregate (`RoomEdit`, `MobEdit`, `ItemEdit`, `ZoneEdit`)
      sharing a small DSL: `name <text>`, `desc` (drops into the
      multi-line editor below), `flag <name>`, `set <field> <value>`,
      `show`, `done` to commit, `abort` to discard. Edits buffer in
      a working copy and only hit the repo on `done`.
- [ ] Mail / note editor mode — multi-line input mode with `.` on its
      own line to end, `~q` to abort, `~h` for help. Backed by a
      `[]string` line buffer on the mode; submits to the parent on
      `OnExit` via a `Done(text string)` callback. Used by mail,
      bulletin boards, and OLC `desc`.
- [x] Pager mode for long output (also tracked in §2) — Phase A #2
      closed 2026-05-11. Lives in `telnet/pager.go` (not
      `internal/mode/pager.go`); push helpers are
      `Session.WritePaged` / `WritePagedWrapped`.

## 6. Accounts, auth & characters

- [x] Account model — `repo.Account` + accounts table (`internal/repo/account.go`,
      `internal/db/migrations/0001_create_accounts.sql`)
- [x] Password hashing — `internal/auth.Hash` / `Verify` via `bcrypt`,
      enforcing 8-rune min / 72-byte max
- [x] Login mode — `internal/mode.Login` with username/password steps,
      lockout after 5 failures, mode-driven echo masking; replaces
      legacy `togglepassword`
- [x] Account-create mode — `internal/mode.Create` reachable via "new"
      from login; ASCII-only username policy, password confirmation
- [x] Character model — `repo.Character`, `0002_create_characters.sql`
      with FK + cascade, account isolation enforced in `CharacterSelect`
- [x] Character-select + character-create modes — auto-skip when account
      has 0 chars (forced create) or 1 char (auto-promote); 2+ shows menu
- [ ] **Post-login account menu** — new `internal/mode/account_menu.go`
      pushed by `Login.onSuccess` (and `Create.onSuccess`) *before*
      `character_select`, replacing the auto-skip-to-select shortcut.
      Becomes the single hub players land on after auth and the place
      every account-scoped action is reachable without dropping to a
      verb. Ordering note: MOTD/news currently renders after
      `character_select` inside `postauth.promoteToGame`; move that
      `news.WriteMOTDBlock` call to fire *before* the account menu is
      pushed (right after login/create success) so players see news
      once per login regardless of which character they pick — and
      so the menu itself isn't interrupted by a news block on every
      character switch. The `last_news_seen_at` gate stays on the
      character row but is stamped against the account's most-recent
      character (or skipped until a character is selected, with a
      `news` menu entry to replay). Menu items:
      - **Character management** — `play <n>` / `select <n>` enters the
        existing `character_select` → `promoteToGame` path; `new`
        ReplaceModes into `character_create`; `delete <n>` runs a
        confirm-by-typed-name guard, then `CharacterRepo.Delete`
        (cascades inventory + equipment + bank + audit refs); `rename
        <n> <name>` if §6 ever allows post-creation rename (gated off
        V1). List shows name / class / level / last-played.
      - **Account password change** — `password` substep prompts current
        → new → confirm under `Session.SetPasswordMode`, rehashes via
        `auth.Hash`, calls `AccountRepo.UpdatePassword`, invalidates
        any other bound sessions for the account.
      - **Account settings** — `settings` sub-menu (slice 3, landed)
        edits five knobs persisted to `accounts.settings_json`
        (migration 0035): `color` (override TERM-detected level),
        `prompt` (default template stamped onto new characters at
        chargen finalize), `width` (override NAWS in [40,200]),
        `locale` (IANA tz string, currently feeds the menu's date
        formatter), `motd` (replay MOTD on every login regardless of
        `last_news_seen`). Each edit writes through
        `AccountRepo.UpdateSettings` and records one
        `settings-update` row in `admin_audit` (account-mode actor).
        Color/Width apply to the session via
        `mode/postauth.applyAccountSettings` immediately before
        `promoteToGame`.
      - **Account security** (slice 4, landed) — `security` sub-menu
        renders the last 10 entries from `account_logins` (migration
        0036; outcome ∈ {success, failure, lockout, kick}) plus the
        active-session list pulled from `session.Registry.Snapshot()`.
        `kick` disconnects every peer session for the account and
        records one `account_logins(outcome=kick)` row per peer plus
        one `admin_audit(verb=kick-sessions)` account-mode row;
        single-session-per-account makes this a no-op today, but the
        path is forward-wired for multi-session work. Login outcomes
        are recorded by `mode/login.go::recordLoginEvent` and
        `mode/create.go` (success-only). `info` is a short fixed-
        vocabulary note and never carries the typed password.
      - **Email / recovery** — set/verify email, trigger password-reset
        token (depends on the §6 email-verification item; menu entry
        is dark until that lands).
      - **News / MOTD replay** — re-render the unseen-news block on
        demand without flipping `last_news_seen_at`.
      - **Help** — `help account` topic (§18) describing every menu
        verb.
      - **Quit** — drops the connection cleanly (same as game-mode
        `quit`, but from pre-game).
      Substep state machine mirrors the chargen pattern (`accountStep`
      enum, `accountDraft` for in-progress password change). Single-
      character accounts no longer auto-promote — the menu always
      shows — but `play` with no arg picks the only character if
      there's exactly one, preserving the "one keystroke into the
      world" feel. Cross-cutting: every destructive action (delete
      character, password change, kick sessions) records to
      `admin_audit` with `actor=account`, `target=character|account`.
- [~] **Character creation flow** — replace the current single-screen
      name prompt (`internal/mode/character_create.go`) with the full
      WoT chargen pipeline, driven by content catalogs in
      `data/chargen/*.yaml` loaded by a new `internal/chargen` package.
      Substeps via the existing mode stack, in order: ability scores
      (point-buy V1; standard array + 4d6-drop-lowest as alternates;
      reroll rule per *abilities.md*) → race + background (eleven WoT
      backgrounds from *backgrounds.md* with bonus feats / class skills
      / home + bonus languages / height-mod / one of three equipment
      bundles) → class (seven WoT classes from *classes.md* driving
      BAB / save progression / HD / class-skill list / level-1
      features) → channeler branch for Initiate / Wilder (Source by
      gender, affinities from the five Powers, starting weaves from
      level-0 list per *the-one-power.md*) → heroic characteristics
      (gender / age / height / weight via *heroic-characteristics.md*
      Table 6-1 with background mods, handedness, alignment posture
      [Good default; Bad / Evil hidden from menu V1], name) →
      first-level feat slot + `(4 + Int mod) × 4` class-skill ranks
      from *classes.md* Table 3-1, merged with background bonuses →
      starting equipment from chosen background bundle, spawned into
      inventory and auto-equipped (Outfit slot wired in §14) → review/
      confirm screen → single `CharacterRepo.Create`. Falls back to the
      legacy name-only flow when no chargen catalog is loaded so dev
      fixtures stay simple. Schema is already there — `creature.Race`,
      `Background`, `ClassLevels`, `Abilities`, Heroic Characteristics
      fields, and `Channeling` all round-trip through migration 0009;
      this is content + UI, not migrations. Catalogs are the same data
      level-up / weave-learning will read in §12, so loader lives once
      and serves both day-zero (this item) and over-time progression.
      **Slice 3 landed 2026-05-06** — starting-equipment bundle
      spawning + auto-equip. New `chargenStepEquipment` substep slots
      between channeling (or skills, for non-channelers) and review
      with one-line bundle picker, `info <#>` detail screen showing
      each item's name/type/weight/value, and `done` gate. Catalog
      gains `internal/chargen/default/items.yaml` with 50 starting
      templates (mirroring world item schema: type/weight/value/
      quality/flags/typed Stats). Catalog `validate()` cross-checks
      every `background.equipment_options[].items` ref against the
      templates map so a typo fails boot. At finalize,
      `CharacterCreate.applyStartingEquipment` clones each picked
      item via `ItemRepo.Create` with a unique runtime
      `external_id` (`<id>#cgen-<charID>-<i>`),
      `RecordInventory`s the id list, and auto-equips the first
      armor → SlotArmor / shield → SlotShield / clothing →
      SlotOutfit / weapon → SlotPrimaryWield. ItemRepo threads
      `Login → Create → postAuth → AccountMenu → CharacterCreate`
      via a new `SetItems` setter; nil silently skips spawning so
      legacy fixtures still work. No migration — items +
      equipment_json + inventory_json round-trip already.
- [x] Mode-driven echo masking — Login / Create / CharacterCreate flip
      `Session.InPasswordMode` via lifecycle; `togglepassword` debug
      command retired
- [x] Multi-session detection — `internal/session.Registry` keyed by
      account ID; Login + Create bind on success and disconnect any
      prior occupant with a "logged in elsewhere" notice. Compare-and-
      delete unbind in `handleConnection` defer prevents stale teardown
      from clobbering a takeover.
- [~] Lockout on failed logins — per-account 5-failure / 15-min lock
      enforced; per-connection rate limit / exponential backoff still
      pending (`login_followups.md` items 4 & 6)
- [ ] Email verification / password reset — optional `email` column
      on accounts, `email_verifications` table holding single-use
      tokens with TTL, `verify <token>` command. Reset flow: `forgot
      <email>` from login mode, mail a token, `reset <token>
      <newpass>` swaps the hash. Mail dispatch behind a small
      `mail.Sender` interface (SMTP impl + log-only fake).

## 7. Persistence

- [x] Backing store chosen: SQLite via pure-Go `modernc.org/sqlite` (no CGO)
- [x] Migration runner: embedded `internal/db/migrations/*.sql`, applied
      lexically, tracked in `schema_migrations` (`internal/db/db.go`)
- [x] First migration: `0001_create_accounts.sql` (accounts table + lockout index)
- [x] Account aggregate: `repo.AccountRepo` interface, `SQLiteAccountRepo`
      impl, `MemoryAccountRepo` fake; shared contract test exercises both
- [x] Character aggregate: `repo.CharacterRepo`, `0002_create_characters.sql`
      with FK + cascade, both impls + shared contract test
- [x] World aggregates (rooms, exits, items, mobs) — `repo.Room`/`Exit`/`Item`/`Mob`
      with SQLite + memory repos (`internal/repo/{room,exit,item,mob}_*.go`),
      `0003_create_world.sql` schema, `0004_seed_starter_zone.sql` 3-room demo
      zone (Plaza ↔ North/South Roads), and `0005_add_character_room.sql`
      adding `current_room_id` to characters. `Session.CurrentRoomID` is
      populated in `promoteToGame` and persisted across reconnects via
      `CharacterRepo.RecordRoom`. `look`, `n/s/e/w/u/d` commands wired into
      Game mode (`internal/cmd/look.go`, `internal/cmd/move.go`).
      The legacy flat `mobs` table and `repo.Mob` impls were
      retired in `0010_drop_legacy_mobs.sql`; the world loader
      now manufactures one `mob_template` per YAML mob entry and
      spawns a single `mob_instance` from it (`internal/world/
      loader.go::insertMobs`), and `look`/`move`/`teleport` read
      from `MobInstanceRepo`. A richer YAML schema for builders
      (separate `mob_templates.yaml` + spawn references with
      counts) is still pending; today every YAML mob is its own
      one-of-a-kind template.
- [x] World data on disk (YAML/JSON area files) with a loader —
      `internal/world` package: `gopkg.in/yaml.v3` parsing, strict
      cross-reference validation (unique ids, exactly-one starter, valid
      directions, all refs resolve), single-transaction sync into the
      SQLite world tables. Default world embedded via
      `//go:embed all:default` at `internal/world/default/`;
      `WORLD_DIR` env var overrides with a real filesystem path. As
      of 2026-05-14 the loader performs an **additive resync** on
      every boot: per-table pre-load probe selects existing
      `external_id`s, and rows in YAML that aren't in the DB get
      inserted; existing rows are left exactly as they are
      (`UPDATE`s and DELETEs out of scope). Boot log emits a
      structured `world: resync complete` line with per-table
      new-row counts plus YAML row totals. Starter room still
      pinned to id=1 when the slot is unoccupied; when occupied,
      the YAML starter falls through to a regular auto-increment
      row (first-loaded starter wins). The previous "no-op when
      world tables already have rows" short-circuit was retired
      because it silently dropped YAML changes; ZoneResetter then
      warned every 5min with "resolve home room" errors for items
      pointing at missing rooms.
- [~] Hot-reload of area files without restart — additive resync
      (above) covers the **adds** half of the contract: new zones
      / rooms / exits / items / mob_templates land on every boot,
      no DB wipe required. Pending: `reload world` admin verb
      (no restart, no boot delay), update support (catch renames
      and drift), soft-delete support for YAML rows that vanish
      (mark `deleted_at`, teleport players in deleted rooms to
      the starter with "the world shifts around you"), and an
      `fsnotify` watcher for fully automatic reloads.
- [~] Periodic + shutdown autosave — `internal/persist.Manager`
      hosts named `SaverFunc` registrations; the new `tick.Buckets
      .Save` bucket (default 30s, `DefaultSaveInterval`) calls
      `FlushAll` on every pulse, and `srv.shutdown()` runs a final
      `FlushAll` under a 5s context budget after the session drain
      finishes. One concrete saver landed: `savePlayTimes`
      iterates `session.Registry.Snapshot()` and stamps
      `last_played_at` on every authenticated character — so a
      crash within the 30s window now loses at most 30s of
      tracked play-time instead of the entire session.
      Pending: dirty-bit aggregate types for combat HP / mob-
      instance state / affect tick counters once §11 / §12 land.
      Most other state (room / item / character core / mob room
      moves) is already write-through, so the dirty-bit pattern
      only matters for state that ticks faster than we want to
      round-trip per mutation.
- [x] Backup rotation — `internal/backup.Manager` writes
      `VACUUM INTO` snapshots to `db.backup_dir` on a wall-clock
      cadence (decoupled from `tick.Buckets` so a paused world
      can't pause backups). Filename
      `wheelmud-YYYYMMDD-HHMMSS.db`; retention prunes by file
      prefix beyond `db.backup_retention`. Gated on a non-empty
      `db.backup_dir` + positive `db.backup_interval_hours`; the
      Manager runs under the server-lifetime ctx via
      `safego.Go("backup-manager", ...)`. Phase J slice J4 (#56),
      commit `938579f` (2026-05-12). Deferred: tiered retention
      (daily / weekly / monthly), gzip compression.

## 8. Game loop & scheduling

- [x] Tick / heartbeat goroutine — `internal/tick.Scheduler` runs a
      single 1 Hz goroutine, dispatches due `Subscribe(every, fn)` /
      `After(d, fn)` handlers fire-and-forget, recovers panics, warns
      on >50 ms fan-out. Clock seam (`WithClock`) for deterministic
      tests. Owned by `server` in `cmd/server/main.go`, started after
      DB open and stopped after the session drain.
- [x] Pulse buckets — `internal/tick.Bucket` wraps a single scheduler
      subscription and fans out to its own subscriber list;
      `tick.NewBuckets(s)` registers `combat` / `regen` / `areaReset`
      with default cadences (4 s / 30 s / 5 min). No game logic yet —
      just the plumbing for §11 / §12 to attach to.
- [x] Event bus / pub-sub for room and zone events —
      `internal/eventbus.Bus` with generic `Subscribe[T]` /
      `SubscribeAsync[T]`, sync dispatch by default, single-worker
      async path, panic-recovery per handler. `world.PlayerEntered`
      / `PlayerLeft` defined in `internal/world/events.go` and
      published from `cmd/move.go::moveDir`. Owned by `server` and
      drained alongside scheduler in `srv.shutdown()`.
- [x] Delayed / scheduled actions (`after 5s do X`) —
      `Scheduler.After` for unscoped one-shots; `tick.AfterCtx`
      auto-cancels via `context.Context` so callers can pass a
      session or room ctx and have the timer die with its scope.
      The wrapped handler also re-checks `ctx.Err()` before firing
      so a race between dispatch and cancel doesn't leak through.
- [x] Graceful shutdown — `signal.NotifyContext(SIGINT, SIGTERM)` in
      `main.go` closes the listener, accept loop exits on
      `net.ErrClosed`, `srv.shutdown()` drains in-flight sessions via
      `WaitGroup` with a 10 s bound, then stops buckets + scheduler.
      World save still pending (depends on §7 dirty-tracking).

## 9. World model

The §7 persistence layer already lands rough `Room`/`Exit`/`Item`/`Mob`
aggregates as flat rows so `look` / movement work end-to-end. This section
tracks the richer domain model the gameplay layers (combat, quests, economy)
will need on top of those tables.

- [~] **Room** — `repo.Room` covers id, zone, name, description,
      `external_id`, `RoomFlags` (`indoors` / `nopvp` / `noteleport`
      / `dark` / `silent` / `peaceful`), `Sector` enum (city, forest,
      field, hills, mountain, desert, water, underwater, air,
      underground), `LightLevel`, `coord_x/y/z` (migration 0012),
      and `ExtraDescs` keyword map (migration 0013, JSON column).
      YAML schema accepts `flags:`, `sector:`, `light_level:`,
      `coords:`, and `descriptions:` blocks; defaults are outdoors
      / city / fully lit / origin / no extras. Behavior wired:
      `look` (no args) renders the room and pitch-blacks dark+0
      light rooms; `look <noun>` resolves against `ExtraDescs`
      (case-insensitive) and falls back to "nothing special" on
      miss; `tp` refuses `noteleport` rooms; `say` is swallowed in
      `silent` rooms; `move` consults the mover's
      `creature.Speed` to gate `air`/`underwater` (FlyFt/SwimFt
      requirements; zero-speed defaults to "blocked"); `whereami`
      command surfaces id/sector/flags/light/coords/keywords for
      builders. Day/night cycling shipped via `internal/world.Clock`
      (migration 0024 + `world_state` table, 30 real-min per game
      day, 4 phases of 450 ticks each). `EffectiveLight(room)` ramps
      outdoor rooms dawn → noon → dusk → midnight; indoor /
      underground / underwater rooms ride the static baseline; the
      `Dark` flag is an explicit "always pitch black" override.
      `look` and `whereami` consult the clock; persistence runs on
      the existing Save bucket. The `time` admin command (gated at
      AuthAdmin) reports tick count, phase, phase progress, and the
      countdown to the next phase boundary so builders can sanity-
      check the cycle without scraping `whereami`. Phase-change ambient
      broadcasts ship sector defaults: a 1-second tick poll
      (`world.PhaseAmbientWatcher`) detects dawn/sunrise/dusk/nightfall
      crossings and writes a single sector-appropriate sentence to every
      session in an outdoor, non-Silent, non-Dark, non-Indoors room
      (Underground/Underwater filtered too); seed-on-construct prevents
      a spurious fire on boot, and non-adjacent phase jumps (future
      `time set` rebases) advance lastPhase silently. Pending: per-room
      reset hooks (block on §9 zone schema); `coords` consumed by
      `map`/`track` rendering (block on §10); player-carried light
      sources (block on item taxonomy + §11 combat); deferred ambient
      extensions tracked in `phase_ambients_open_questions.md` memory
      (empty-sector default policy alignment, optional `RoomFlags.
      Windowed` for shuttered indoor rooms, per-room ambient overrides
      `rooms.ambient_phase_json` for hand-built set-piece rooms, and
      zone-`climate` variants for sector-by-climate text tuples once
      overrides exist).
- [x] **Exit** — `repo.Exit` covers `from_room_id`, `direction`,
      `to_room_id`, `ExitFlags` (`Closed` / `Locked` / `Pickable` /
      `Hidden` / `NoPass`), `KeyExternalID`, `LockDifficulty`, and
      `Description` (migration 0014). YAML accepts both shorthand
      (`north: room.id`) and object form (`north: {to: ..., closed,
      locked, key, difficulty, description, ...}`); `Pickable`
      defaults to true. Behavior wired: `look` skips Hidden exits
      and annotates the rest with `(closed)` / `(locked)` in dim
      gray; `move` refuses Hidden as "you can't go that way",
      Closed as "the door is closed", and NoPass with an
      unseen-force message. One-way exits already supported via
      single-direction authoring. Diagonal directions (ne/nw/se/sw)
      shipped via migration 0007. Runtime mutation verbs
      (`open` / `close` / `lock` / `unlock` / `pick`) shipped in
      §16 against `ExitRepo.UpdateFlags`, with key resolution
      against §14 inventory and lock-difficulty gating for
      `pick`. See `door_commands_followups.md` for deferred
      polish (inventory-key swap, pick skill check, etc.).
- [x] **Area / zone** — `zones` table landed (migration 0016): id,
      external_id, name, builder, min/max_level, reset_interval_s,
      reset_mode (`always`/`empty`/`never` enforced via CHECK),
      climate, ambient_json. `rooms.zone_id` joins rooms to their
      owning zone (soft FK; default 0 keeps test fixtures buildable
      without a backing zones row). `repo.ZoneRepo` (sqlite + memory
      + shared test suite) exposes Create / GetByID /
      GetByExternalID / List; ZoneResetMode is a typed enum mirrored
      across both impls so the SQLite CHECK is a backstop, not the
      front line. The YAML loader inserts a zones row per `zone.yaml`
      and stamps `rooms.zone_id` during insertRooms; `zone.yaml`
      accepts `builder`, `level_range`, `reset_interval_s`,
      `reset_mode`, `climate`, `ambient` (all optional, defaults
      applied at insert). `validateZones` enforces external-id
      uniqueness, valid reset_mode, level-range bounds, and
      non-negative reset interval with builder-friendly file:line
      errors. `zones` admin command (AuthAdmin) lists zones and
      shows per-zone metadata + room count via
      `RoomRepo.CountByZone`. Reset pipeline shipped end-to-end:
      `internal/world/respawn.go::ZoneResetter` (renamed from
      `Respawner`) walks every zone on each `AreaReset` tick and
      runs three steps in order under one per-zone gate (honors
      `ResetMode` always/empty/never + `ResetIntervalS` +
      occupancy): mob respawn from anchored templates (0042);
      door state restoration from `exits.authored_closed` /
      `authored_locked` columns (0048) via
      `ExitRepo.RestoreAuthored`; item respawn from the loader's
      in-memory `LoadedWorld.ItemSpecsByZone` map via
      `ItemRepo.FindByExternalID` global presence check (a player
      who keeps the item suppresses respawn — even after carrying
      it into another zone). Mob equipment on reset is deferred
      until MobTemplate gains equipment fields + persistence +
      auto-equip on `NewInstanceFromTemplate`. Admin edit
      (`zedit`) lands with §16 (Phase G #34). Promotion of
      `rooms.zone_id` to a hard FK via table rebuild also lands
      with §16, once admin room-create reliably supplies a zone id.
- [~] **Item / object** — taxonomy schema landed (migration 0015).
      `repo.Item` carries `Type` enum (`weapon` / `armor` / `shield` /
      `container` / `consumable` / `light` / `key` / `tool` /
      `clothing` / `food` / `trade_good` / `trash`), `Weight` (lb),
      `Value` (`currency.Amount` in cp), `Quality` (`normal` /
      `masterwork` / `masterpiece` / `power_wrought`), `Flags` bitset
      (`notake` / `nodrop` / `nosell` / `bind_on_pickup` / `magic` /
      `glow` / `hum` / `trade_good`), and a polymorphic
      `stats_json` blob decoded into typed structs:
      `WeaponStats` (proficiency / size / range / damage dice /
      threat range / crit mult / range increment / B-P-S types /
      special tags), `ArmorStats` (weight class / bonus / max-dex /
      check penalty / speed), `ShieldStats` (kind / bonus / check
      penalty), `ContainerStats` (lb + cuft + liquid pints + depth
      cap + weight multiplier), `ConsumableStats` (charges +
      effect-id forward ref), `LightStats` (radius + fuel ticks),
      `KeyStats` (key id), `ToolStats` (skill tag + charges). YAML
      schema accepts `type:`, `weight:`, `value: "15mk"` (parsed
      via `currency.Parse`), `quality:`, `flags: [...]`, and a
      type-discriminated `stats:` sub-block; unknown types and
      flags fail at validate time. `examine` surfaces type / quality
      / weight / formatted value when set. `lock` / `unlock` prefer
      typed `KeyStats.KeyID` matches over the legacy ExternalID
      fallback. Pending: combat damage resolution against
      `WeaponStats` (§11), wear/wield slot management (§9 equipment
      slots bullet), encumbrance / carry-weight (§14), don timers,
      masterwork/masterpiece attack/AC effects, Reputation deltas,
      container nesting + `take` / `put` / `open container` (§14),
      light radius affecting room visibility (§10), Power-wrought
      unbreakable handling.
- [~] **Container semantics** — `parent_item_id` (migration 0028)
      adds nesting as a third reachable location alongside room/owner;
      `put <item> in <container>` and `get <item> from <container>`
      verbs honor `ContainerStats.CapacityLbs`, `DepthCap`, `WeightMult`
      (bag-of-holding compounds across nested bags), and reject
      self/cycle puts. `inventory` renders a tree; encumbrance counts
      transitive contents through the multiplier chain. Pending:
      open/closed/locked state sharing the door schema, content
      visibility flag (`see-inside`), slot count caps, take/put
      permission flags, liquid containers (pour/fill/drink), YAML
      `contents:` for builders, `look in <container>` verb.
- [~] **Mob / NPC** — `repo.Mob` has id, room/zone, name,
      description, `external_id`; `mob_templates` / `mob_instances`
      tables and `creature.MobTemplate` / `MobInstance` types now
      exist (see "Player character extending the mob model" below)
      but no repo or loader integration yet. Pending: stat block (level, HP/MP,
      str/dex/con/int/wis/cha, AC, hit/dam dice, attack type,
      damage type), behavior flags (`aggressive`, `wimpy`,
      `sentinel`, `scavenger`, `assist-same-race`, `helper`),
      faction / race / class, default loadout (eq + inventory),
      gold range, XP award, corpse decay timer, spawn rules
      (zone reset references), dialogue/script hooks (§15
      triggers), shopkeeper subtype (inventory list, buy/sell
      multipliers, hours), quest-giver flag.
- [~] **Player character extending the mob model** — type skeleton
      and schema landed (`internal/creature/creature.go` defines
      `Core`, `Abilities`/`AbilityScore`, `Saves`, `Speed`,
      condition/position/quality bitsets, `Affect`/`StatMod`,
      damage types + DR/resists, `Equipment` slots, `Channeling`
      sub-record, `MobTemplate`, `MobInstance`, `Character`).
      Migrations `0008_create_creatures.sql` (mob_templates,
      mob_instances, polymorphic channeling) and
      `0009_extend_characters.sql` (Core + player columns) applied.
      `MobTemplateRepo`, `MobInstanceRepo`, and `ChannelingRepo`
      (interfaces + SQLite + memory + contract tests) landed in
      `internal/repo/{mob_template,mob_instance,channeling}_*.go`;
      `ChannelingRepo` keys off a polymorphic `(OwnerKind, owner_id)`
      so the same row schema serves PCs, mob templates, and mob
      instances.
      World loader migrated to spawn from templates
      (`0010_drop_legacy_mobs.sql` removes the legacy flat table);
      `look`/`move`/`teleport` read from `MobInstanceRepo`.
      `repo.CharacterRepo` extended to load/persist the full Core
      stat block + player columns from migration 0009 (race,
      background, class levels, XP, coin/bank, stance, fame/
      infamy, JSON catalogs for feats / skills / equipment /
      inventory / quests / dialogue) plus a `RecordCore` method
      for combat HP/condition updates; round-trip test covers
      every column class.
      Pending: char-create rolling abilities
      / picking race / class / background / starting HP & defense,
      richer YAML schema for builders (template/spawn split with
      counts), `examine` rendering full mob detail, and seed catalogs
      for feats / skills / talents / weaves / classes /
      backgrounds (stubbed as `int32` ids today). Original spec:
      share a `creatures` core: `name`, `size` (Fine→Colossal), `type`
      (Humanoid / Animal / Exotic / Shadowspawn), `gender`,
      `alignment_posture` (Good/Bad/Evil narrative tag); abilities
      `Str/Dex/Con/Int/Wis/Cha` as `current/max/inherent` triples
      (drain vs damage vs ter'angreal); `hp_current/max` plus a
      separate `subdual_damage` pool; Hit Dice; **`defense`** (replaces
      AC — class+Dex+size+armor+shield+dodge); `fort/ref/will` saves;
      `init_modifier`; `base_speed_ft` (+ optional climb / fly with
      maneuverability / swim); `bab` (drives multi-attack at +6/+11/
      +16); reach/face/threat range; conditions set (§12); position
      flags (prone, flat-footed, flanked, grappling targets); damage
      reduction; resistances; special qualities (Blindsight,
      Low-Light Vision, Scent). Split into `mob_template` (immutable
      archetype — `challenge_code` A–I, `organization`, climate/
      terrain, advancement rules, behavior flags, natural attacks,
      `special_attacks`, traits, `loot_table_id`, `gold_dice`,
      `dialogue_tree_id`, `trigger_scripts[]`, `shopkeeper_config`,
      `corpse_decay_ticks`, `respawn_zone_reset_id`, Shadowspawn-
      specific `shadow_link_to_myrddraal_id` + `taint_immune` +
      fade-on-link-master timer) / `mob_instance` (in-world state) /
      `character` (persisted player). Player adds: account fk,
      `class_levels: map[Class]int` over the seven WoT classes
      (multiclass = sum), `race` (Human/Ogier), `background` (Aiel /
      Atha'an Miere / Borderlander / Cairhienin / Domani / Ebou Dari
      / Illianer / Midlander / Taraboner / Tairen / Tar Valoner —
      supplies starting gear, languages, height-mod), `feats[]`
      (general/special/channeling/lost-ability), `skills:
      map[SkillID]ranks` (class-skill cap = level+3, cross-class
      = ½), `practice_points`, `class_features[]` (Uncanny Dodge,
      Dance the Spears, Sneak Attack, etc.), appearance (height/
      weight/age/handedness), **reputation** with `infamy_share`
      (≥½ vicious gains → Infamous; gates fame/infamy feats),
      `followers[]` (unlocked lvl 10, capped by Reputation),
      `coin: Amount` (existing currency), encumbrance load +
      fatigue/exhaustion timers, condition (standing/sitting/
      sleeping/fighting), idle timer, `bound_room_id` (respawn),
      `bank_balance`, `played_seconds`, `last_login`, `quest_log`,
      per-NPC dialogue state. Channelers (PC or NPC) attach a
      sub-record — see new bullet below.
- [ ] **Channeling (One Power) sub-record** — attached to any
      channeler (PC, Aes Sedai, Wise Ones, Forsaken, damane,
      Asha'man). Fields: `gender_source` (`Saidin` male / `Saidar`
      female — same mechanics, asymmetric perception);
      `channeler_type` (`Initiate` Int+Wis-keyed / `Wilder`
      Cha+Wis-keyed); `affinities: Set[Power]` ⊆ {Air, Earth, Fire,
      Water, Spirit}; `talents: Set[TalentID]` (Healing, Traveling,
      Warding, Cloud Dancing, Earth Singing, Elementalism, Illusion,
      Conjunction, …); `weaves_known: []WeaveID` (each tagged
      Common / Rare / Lost); `slots_per_level: map[int]{cur,max}`
      (slot-based casting, **not mana** — levels 0–9). Casting
      thresholds: Initiate `Int ≥ 10+level` & `Wis ≥ 10`; Wilder
      `Cha ≥ 10+level` & `Wis ≥ 10+level`; Wilders cap at level-2
      outside their Talents, Initiates cap at level-0. State:
      `embraced` (full-round to enter; blocks rest/heal/sleep;
      addictive), `madness: int` (men only — accrues while embraced;
      Mental Stability feat slows; `Heal the Mind` weave reduces),
      `stilled_state` (recoverable via the Lost `Restore the Power`
      weave), `bonded_warder_id` / `bonded_to_aes_sedai_id` (via the
      Conjunction Talent), `held_angreal_id` / `held_saangreal_id`
      (adds power 1–10 to slot levels; cross-gender devices appear
      inert), `circle_id` (linking — leader / members / required-men
      ratio per Table 9-1), `aes_sedai_oaths` 3-oath bitmask + an
      `ageless` cosmetic flag, `damane_collar_to: NPCID` (a'dam
      binding). Data layout landed (`creature.Channeling` + the
      polymorphic `channeling` table from `0008_create_creatures
      .sql`). Phase E #27 (2026-05-08) shipped slot consumption /
      8h refresh from the `Regen` bucket via
      `internal/channeling.RefreshIfDue`, accrue-only madness for
      embraced Saidin channelers via `AccrueMadness`, the
      `Stilled` runtime gate, and the `embrace`/`release` and
      `still`/`unstill` verbs. Still pending: embrace lifecycle
      (full-round enter, blocks rest/heal/sleep — gated on a
      `rest` verb landing), Mental Stability save layered on
      madness accrual + `Heal the Mind` reduction, circle linking
      math (Table 9-1 leader/member/required-men ratios, pooled
      slot draw), a'dam bind/unbind enforcement (collar-side
      commands, suppression while collared), Warder bond effects,
      angreal/sa'angreal slot boost with cross-gender inert
      behavior.
- [~] **Equipment slots and wear/wield logic** — WoT does not use a
      D&D wear-slot bitmask. Discrete slots: `armor` (one body
      armor), `shield` (separate from armor; bonuses stack),
      `primary_wield` + `off_hand` (two-handed weapons consume both;
      double weapons like quarterstaff/ashandarei = one item with
      two attack profiles), `outfit` (Artisan's / Cadin'sor /
      Courtier's / Explorer's / Gleeman's / Noble's / Peasant's /
      Royal / Scholar's / Traveler's / Cold-weather — first free at
      creation), `cloak` (Warder fancloth canonical), `backpack`
      (primary container), `belt_pouches[]` (containers),
      `held_in_hand` (transient — torch/lantern; competes with
      weapon use), `mount` (separate aggregate with barding/saddle/
      saddlebags), `worn_misc[]` (signet ring, Aiel buckler-strap —
      no hard cap, narrative). Wear-flag validation against item
      type, swap semantics, save/restore on login, affects
      reapplied on equip and stripped on remove, container-on-belt
      vs in-inventory distinction.
      Landed 2026-05-04: V1 verbs `wear` (Armor / Shield /
      Clothing→Outfit), `wield <weapon> [off]` (PrimaryWield /
      OffHand), `remove`, and `equipment` / `eq` listing. Persisted
      via `equipment_json` (no migration). `inventory` annotates
      equipped items; `drop` / `give` / `put` auto-clear the slot.
      Pending: two-handed weapons, Cloak / Backpack / WornMisc /
      BeltPouches slot disambiguation, wear-requirements (Str /
      class / level / wear-flag), affect application on equip/
      remove, double-weapon attack profiles.
- [~] **Currency model** — `internal/currency` package: four
      denominations (`cp`/`sp`/`mk`/`gc`) at fixed ratios
      `1/10/100/1000`, all wealth stored as a signed `Amount` of
      base copper pennies. `New(gc, mk, sp, cp)`, `Parse("1gc 2mk
      3sp 4cp")`, `Format` (greedy largest-first), `Short` (largest
      non-zero), `In(coin)`, `Add`/`Sub` with overflow + insufficient-
      funds guards. Pending: a `coin` column on characters (carried
      wealth), a separate `bank_balance` for §14 vaults, gold ranges
      on mob loadouts, and `give <amt>` / shop wiring once §14
      lands.

## 10. Movement, look & navigation

- [x] `look` (current room, items, mobs, exits) — `internal/cmd/look.go`
      via `RenderRoom`; reused by every successful move
- [x] Cardinal + vertical + diagonal directions — `n/s/e/w/u/d` plus
      `ne/nw/se/sw` with short-code aliases via `NewMoveFamily`
      (`internal/cmd/move.go`). Migration `0007_widen_exit_directions.sql`
      rebuilds the `exits` table with the widened CHECK constraint
      (SQLite can't ALTER CHECK in place); `validDirections` in the
      world loader (`internal/world/validate.go`) and the look
      renderer's `directionLongName` map (`internal/cmd/look.go`)
      both accept the diagonals.
- [~] Doors, locks, keys — `open` / `close` / `lock` / `unlock` /
      `pick` shipped (`internal/cmd/door.go`) on top of
      `ExitRepo.UpdateFlags`. Direction parsing accepts long names
      (`open north`) and short codes (`open n`); Hidden exits stay
      invisible to door verbs. `open` refuses Locked, `close` refuses
      NoPass. Both broadcast to the actor's room and (when a reverse
      exit exists) the far room with the inverted direction. `lock` /
      `unlock` enforce a key match against `Exit.KeyExternalID` —
      keys must be in the actor's inventory (§14 retired the
      room-floor placeholder); `AuthAdmin` always bypasses the key
      check. `unlock` leaves the door closed-but-unlocked so the lock
      state stays observable. `pick` requires
      `Pickable=true` and is gated to `AuthAdmin` until §12 skill
      checks land — players see the "you lack the skill" flavor that
      will become a failed roll. Pending: §14 inventory swap (keyed
      item lookup against the player's bag instead of the room),
      §12 lockpicking skill check + difficulty roll against
      `Exit.LockDifficulty`, and door reset on zone reset (§9 zone
      schema).
- [~] `examine` for detailed inspection — `examine <target>` shipped
      (`internal/cmd/examine.go`). Resolves against room mobs first
      (by Core.Name token-prefix / substring), then room items (by
      Item.Name). Mob view renders name, a coarse HP descriptor
      ("perfect health" / "wounded" / "barely standing"), condition
      bitset decoded to labels, and named Affects. Item view renders
      name + ShortDesc. Pending: inventory + equipment lookup
      (waits on §14 inventory tables / repos), container contents
      (waits on §9 container semantics), keyword-disambiguation
      (`2.guard` syntax — also a §14 follow-up), and richer item
      condition/durability once items grow a stat block.
- [x] `map` / mini-map ASCII rendering — `internal/cmd/map.go`. BFS
      from the current room (default depth 3, `map <n>` clamped to
      1..5). Lays cells onto a 2D grid via per-direction unit vectors
      in `dirVec` (north decreases y, east increases x; diagonals are
      integer ±1 steps; vertical exits decorate the source cell).
      Renders `[*]` current, `[ ]` visited, `[?]` for `nomap`
      destinations + depth boundary, `[^]`/`[v]`/`[%]` for cells with
      up/down/both. Connectors (`-`, `|`, `\`, `/`) are drawn from
      actual exits, never grid adjacency, so two visited rooms that
      lack an exit between them stay disconnected. Hidden exits are
      skipped (mirrors look). NoMap rooms render `[?]` and the BFS
      does not recurse through them. First-visit-wins for cycles and
      grid conflicts. Migration `0020_room_nomap.sql` adds the flag
      column; `repo.RoomFlags.NoMap` and `world.RoomFlags.NoMap` mirror
      it. Player cells are tinted by sector via the shared
      `sectorPalette` (same palette + legend layout as admin
      `zonemap`); the player legend lists every sector and the
      `[*]`/`[?]`/`[^]`/`[v]`/`[%]` cell semantics. The current
      zone's display name (not external id) is printed above the
      grid via `lookupZoneName`; legacy ZoneID==0 rooms omit the
      header silently. Sector glyph letters, off-zone `( X )`
      markers, and flagged-room footers stay admin-only — color is
      additive. Not yet shipped: dynamic depth from `Session.Width`,
      door state in connectors, depth-boundary `[?]` hint when a
      visited cell has unrecursed exits.
- [~] Trails / `track` command — `mob_trails` ring buffer keyed by
      `(mob_id, room_id, ts)` updated on every move; `track <name>`
      finds the freshest trail entry within the last N minutes and
      reports the first-step direction. Fades with time; skill
      check determines the max staleness the tracker can read.
      Storage shipped (migration 0021, `MobInstanceRepo.UpdateRoom`
      records the row + caps at `MobTrailCap`); the wander tick
      (`internal/mob/wander.go`, `tick.Buckets.Wander` at 20 s,
      per-template `WanderChance` × global multiplier, non-Sentinel
      mobs only, in-zone walkable exits) drives organic movement
      so trails accumulate in normal play. Per-template tuning via
      migration 0022 (`mob_templates.wander_chance`, default 0.25)
      and `wander_chance` on `mobs.yaml`. Admin `track <name>` verb
      shipped (`internal/cmd/track.go`, AuthAdmin) — keyword-resolves
      across all spawned mobs, reports current room + last-step
      direction + elapsed time. Pending: §12 skill-check gate on
      staleness window so players (not just admins) can `track`.
      **Default flipped 2026-05-06**:
      `creature.DefaultWanderChance` is now `0.0` — random wandering
      is opt-in per template (set `wander_chance` on the YAML mob
      entry). **Phase F §32a landed 2026-05-12**: authored
      strict-path (`mob_templates.path`, migration 0053) and BFS
      pathfinding (`mob_templates.wander_radius`, migration 0054)
      both ride the same wander tick — see PLAN.md #32a.

- [ ] Scheduled mob routes — declarative path-following so mobs like
      a lamplighter making evening rounds, a guard checking the
      walls, or an innkeeper closing up shop move on a known
      schedule rather than (or in addition to) the random wander
      tick. Sketch: a `routes:` sub-block on the YAML mob entry
      listing one or more named routes; each route is an ordered
      list of `(room_id, action?, dwell_s)` waypoints plus a trigger
      (clock window, world-state key, or interval). The mob runner
      gains a route handler that advances the current waypoint, runs
      the optional action verb (`emote`, `say`, `light`, `lock`),
      and respects `dwell_s` before stepping. Routes coexist with
      `wander_chance`: a mob on an active route shouldn't wander;
      between routes (or off-schedule) it falls back to its base
      wander rate. New schema: `mob_routes` (route metadata) and
      `mob_route_steps` (ordered waypoints) tables, plus a
      `mob_instances.current_route_id` / `current_step_idx` pair so
      reboots resume mid-route. Audit hook to flag mobs whose routes
      cross zone boundaries or hit `nomap` rooms.

## 11. Combat

- [~] Initiative / round structure — combat ticks off the `combat`
      bucket from §8 (default 4 s). On `attack <target>` push both
      participants into a `Fight` aggregate keyed by room; each pulse
      resolves one round per combatant in initiative order
      (`d20 + init_modifier`, ties broken by Dex then random).
      Players queue one combat-mode action (`flee`, `kick`,
      `weave <name>`) per round. Multi-attack at BAB +6/+11/+16
      grants extra iterative attacks at −5 each.
      **Spine landed 2026-05-06** — new `internal/combat` package
      with per-room `Fight`, initiative roll
      (`d20 + DexMod + InitMod`, ties broken by raw-d20 then
      ActorRef.ID), `Manager` (`Start`/`End`/`Get`/`Tick`/`Stop`)
      subscribed to `tick.Buckets.Combat`, and typed
      `CombatStarted` / `RoundStarted` / `CombatEnded` events on
      the eventbus. Auto-ends fights when every mob participant
      has left the room. Pending: `attack` verb (#18), action
      queue per round (`flee`, `kick`, `weave`), iterative
      attacks at BAB +6/+11/+16, mid-fight join, character
      participant-presence check (slice 2 — needs session
      registry threading), Fight persistence across reboot
      (currently in-memory only).
- [x] Damage types and resistances — minimal slice landed
      2026-05-07 with #18: `applyDamage(target, amount, dt)` walks
      `Core.DR` (flat clamp) then `Core.Resists` (percent modifier,
      negative = vulnerability), routes subdual into `Core.Subdual`.
      `weaponPrimaryDamageType` maps WeaponStats B/P/S → enum.
      Pending: Bypass-keyword DR (magic / cold-iron tags), per-resist
      type-tag parsing, `examine` surfacing of resists/DR, weave-
      sourced energy types.
- [x] Hit/miss/dodge/parry rolls — landed 2026-05-07 (Phase D #18,
      slice 1). `internal/combat/resolution.go` — `RollAttack`
      (d20 + BAB + Str-mod vs Defense; nat-1 always misses, nat-20
      always hits; crit threshold from `WeaponStats.ThreatLow`,
      multiplier from `CritMult`) and `RollDamage` (weapon dice +
      Str-mod, multiplied on crit, floored at 1). **Crit
      confirmation landed 2026-05-11** — SRD-style second d20 +
      same modifiers vs the same Defense; `AttackRoll.Threat`
      carries the raw threat-range flag, `IsCrit` only fires on
      confirm-success. Per-`Fight`
      `Actions map[ActorRef]Action` queue with
      `Manager.EnqueueAction`; `Tick` resolves the active actor's
      queued action and writes HP back via
      `MobInstanceRepo.UpdateLive` / `CharacterRepo.RecordCore`.
      `CombatHit` / `CombatMiss` / `ActionResolved` events.
      New `attack <target>` verb (alias `kill`) — refused in
      Peaceful rooms; re-issuing while a fight is in progress
      switches the queued target without restarting initiative.
      `CharacterRepo.GetByID` added (sqlite + memory + shared
      test) so `Manager.resolveCore` can load player participants.
      Pending: parry check (needs FlatFooted state machine), crit-
      confirmation roll, fumble (drop / AoO), iterative attacks at
      BAB +6/+11/+16, two-weapon / off-hand, ranged / thrown
      weapons, `flee`, combat prompt repaint, single
      `combat.Roll` seam for deterministic tests beyond the
      stub-seed pattern used today.
- [~] Aggro / threat tables — slice 1 landed 2026-05-07.
      `Fight.Threat map[ActorRef]map[ActorRef]int32`
      (defender → attacker → cumulative). Damage adds threat 1:1
      from the same hit site that bumps `DamageTally` (post-DR /
      post-resist). `pruneDead` drops the dead actor's defender row
      AND walks every other row to drop their attacker column.
      `Fight.HighestThreat(defender)` returns the largest contributor
      with deterministic tie break (ascending ActorRef.ID), zero
      ActorRef on empty / missing rows. Pending: NPC retarget loop
      (no mob AI yet), healing-adds-threat (no heal verb / weave
      wiring), taunt verb (flat bonus), `feign death` (zero one
      source).
- [x] Death, corpses, looting, XP award — slice 1 landed
      2026-05-07 (Phase D #19, mob death only). HP ≤ 0 on a mob:
      spawn a `corpse of <name>` `ItemTypeContainer` in the room
      (currently empty — inventory transfer pending), despawn the
      mob (`UpdateRoom(0)` + `Delete`), award XP weighted by
      `Fight.DamageTally` to character attackers; killer collects
      the rounding remainder. Per-template XP value comes from a
      hard-coded `xpValueForChallenge(ChallengeCode)` table
      (A=100 → I=38400) — moving to a YAML field on MobTemplate is
      a follow-up. New `CharacterRepo.RecordXP`. `CombatDeath` /
      `CombatXPAwarded` events. `Fight.Dead` set pruned from
      `Order` at top of next `tickRoom` so ActiveIdx math observes
      a stable slice during resolution; fight auto-ends when Order
      empties. **Looting bundle landed 2026-05-07** — Phase D §19
      polish. Corpse decay landed earlier as the in-memory `Decayer`
      + `tick.Buckets.Decay` 30s sweep. Now: mob inventory transfers
      into the corpse via new `ItemRepo.SetParent(itemID, parentID)`
      (unconditional sibling to `SetOwner` / `SetRoom` — clears the
      other two location columns atomically); `MobTemplate.GoldDice`
      is parsed via the existing `combat.rollDice` helper and
      spawns a single `ItemTypeTradeGood` "a small pile of coins"
      inside the corpse with `Value = rolledCp` and `FlagTradeGood`
      so it sells back at full value through the §14 shop verbs.
      `MobTemplate.XPValue int64` (migration 0040) overrides the
      `xpValueForChallenge` table when non-zero; zero falls back to
      the A→I curve. Optional `xp_value:` and `gold_dice:` YAML
      keys on mob entries. All three additions are best-effort:
      a SetParent failure logs and continues, an empty / malformed
      `GoldDice` produces no pile, `XPValue == 0` is the silent
      fallback.
      **Player death + respawn + XP-debt landed 2026-05-08** —
      Phase D §19 player-death slice. `combat.handleCharacterDeath`
      now fires when a character's HP hits zero (resolveAction's
      death gate dispatches by ActorKind). Pipeline: heal to
      HPMax, clear `CondDying|CondUnconscious` + position_flags
      via `RecordCore`, move to `BoundRoomID` via `RecordRoom`,
      stack an XP-debt delta = `(xp - XPForLevel(level)) / 10`
      (10% of current XP-into-level, clamped at 0). Migration
      0041 adds `characters.xp_debt`. Combat events
      `CharacterDied` (death-room peer broadcast + "You die!"
      private) and `CharacterRespawned` (subscriber stamps the
      session room, renders the bound room, broadcasts to peers
      in the new room) wire it end-to-end. Future XP awards
      drain debt off the top via
      `combat.ApplyXPAward(award, debt) → (gain, newDebt)`;
      `CombatXPAwarded` carries both the net gain and the
      `DebtTaken` share. The `xp` verb shows the debt line when
      non-zero. No tick-scheduled decay — debt clears through
      play. Inventory / equipment / coin stay with the player;
      no player corpse spawned. **Durable corpse decay landed
      2026-05-11** — migration 0050 adds `items.decay_expires_at`;
      `combat.spawnCorpse` stamps the deadline inline at Create
      time; `combat.Decayer.RearmFromRepo` replays the queue at
      boot (past-deadline rows are swept on the spot, future rows
      are re-Schedule'd). **`bind` verb landed 2026-05-14** —
      migration 0056 adds `rooms.bindable`; new `bind` player
      verb (Auth=Player, no args) records the current room as
      Character.BoundRoomID via new `CharacterRepo.RecordBoundRoom`
      (mirrors RecordRoom shape, no optimistic lock). Refuses on
      non-bindable rooms with "You feel no connection to this
      place"; short-circuits when already bound. `RoomFlags.Bindable`
      wired through repo / YAML / loader / redit OLC / zonemap
      flag reporter (loader-lockstep). The existing
      `combat.handleCharacterDeath` respawn path picks up the new
      value automatically — no death-side changes needed.
      **PvP XP awards landed 2026-05-14** — `combat.handleCharacterDeath`
      now snapshots `Fight.DamageTally` under `m.mu` (mirrors
      mob_death's critical section) and, after the existing
      death/respawn events, runs the new shared `creditXPShares`
      helper. New `pvp_xp.go`: `pvpXPForKill(attackerLevel,
      victimLevel) int64` returns `PvPXPPerVictimLevel(50) *
      victimLevel`, zeroed when `attacker - victim >
      PvPLevelDiffCap(5)` so high-level alts can't farm low ones.
      Refactor extracted the per-character XP loop out of
      `awardKillXP` into shared `creditXPShares` so mob and PvP
      paths share group-expand / allocateXP / ApplyXPAward /
      CombatXPAwarded plumbing — mob path is byte-equivalent to
      before. Non-combat deaths (HandleAffectDeath, empty
      killer) and empty-tally edges short-circuit; victim is
      stripped from the tally before allocation so a self-damage
      reflect can't credit XP back. **Drop-on-death toggle landed
      2026-05-15** — Phase D §19 closer. New `CombatConfig{DropOnDeath
      bool}` block on the runtime config (env `DROP_ON_DEATH`, YAML
      `combat.drop_on_death`), threaded through
      `combat.Manager.SetDropOnDeath`. When enabled and the items repo
      is wired, `handleCharacterDeath` runs `dropCharacterLoot`
      before the heal/respawn writes: spawn a durable player-corpse
      (`pcorpse-<id>-<nano>` external id; same `corpseDecayDuration`
      and `Decayer.Schedule` as mob corpses), `TransferOwnerToContainer`
      every top-level inventory item (nested container contents follow
      their parent automatically), repeat for equipped item ids,
      clear `Equipment` via `RecordEquipment(zero)`, spawn a
      `TradeGood` coin pile inside the corpse for carried coin
      (bank preserved), zero `Character.Coin` via `RecordCoin` with
      one optimistic-lock retry on `ErrCoinConflict`. When the drop
      fires, the 10% XP-debt delta is waived — gear/coin loss replaces
      XP debt as the death cost. `CharacterDied.CorpseID` carries the
      new corpse id through the event bus for future broadcast
      variants. Affect-death path (`HandleAffectDeath`) shares the
      gate. Still pending: two-UPDATE TOCTOU on RecordXP +
      RecordXPDebt (safe under single-session-per-account today);
      per-zone room-flag override and harsher broadcast variants are
      deferred. Mob respawn via §9 zone reset already shipped
      (`internal/world/respawn.go::ZoneResetter.respawnMobs`).
- [x] PvE vs PvP rules and safe zones — `pvp` flag on character
      (opt-in) plus `nopvp` room flag (always safe). Attack between
      two non-PvP players blocked at the verb level; one-side opt-in
      still blocked. Newbie level cap (<10) immune. **Closed
      2026-05-07** — Phase D #21 across 4 slices: (1) migration
      0037 + `pvp` verb + `attack <player>` guard order (nopvp
      room → attacker newbie → target newbie → attacker opt-in →
      target opt-in); (2) `who` PvP tag; (3) defender-side reverse
      broadcast on PvP attacks; (4) `MatchPlayer` in
      `internal/cmd/keyword.go` mirroring `MatchItem`/`MatchMob`
      for ordinal targeting (`attack 2.alice`). Newbie cap is
      `cmd.NewbiePvPLevelCap = 10`; `characterLevel(ch)` sums
      `ClassLevels` values. Pending follow-ups (deferred): tab-
      completion for `attack` player targets, `consider <player>`.
- [x] Group / party mechanics — `group <name>` invites, `follow
      <name>` / `unfollow`, leader's moves auto-pull followers,
      shared XP split among in-room party members. **Closed
      2026-05-07** — Phase D #22 across 4 slices:
      (1) `internal/group` package — `Group` aggregate (Leader +
      Members map), `Manager` keyed by leader CharacterID with
      reverse `byCharacter` index, in-memory state,
      `MaxGroupSize = 6`, leader-leaves-disbands. New `group
      <invite|accept|decline|leave|kick|disband>` verb plus bare
      `group` roster. Logout cleanup wired through
      `handleConnection`'s teardown defer. (2) Same-group PvP
      refusal: `pvpRefusalReason` gains a `sameGroup bool`
      parameter and a new gate (priority 2, between NoPVP and
      newbie cap) — "X is a comrade — you won't strike them."
      (3) `follow <player>` + `unfollow` verbs plus chain-on-move.
      New `Session.followingID` (crossMu-guarded). Move verb
      re-runs `moveDir` for every co-located peer following the
      leader; recursion bounded by `followDepth`; on per-follower
      failure (locked door, sector gate, missing exit) the
      relationship clears with a "couldn't keep up" notice.
      (4) Shared XP split. New `combat.GroupResolver` typed
      callback + `Manager.SetGroupResolver` setter
      (mirrors `SetDecayer`/`SetFleeMover`); `expandTallyByGroup`
      splits each character contributor's damage equally across
      in-room party members (remainder credits to the dealer).
      Pending follow-ups (deferred): `gtell`, leader succession,
      invite expiry, cross-room follow, loot share, `assist`,
      `consider`.
- [~] **Action cost & per-actor cadence** — the current MVP (#18)
      resolves *one* action per `tick.Buckets.Combat` pulse for the
      single active actor on a strict round-robin, so a 2-combatant
      fight swings every 4 s regardless of Dex, weapon, race, or
      armor. That reads as turn-based, not real-time, and gives an
      Aiel spearman the same swing rate as a plate-armored Borderland
      heavy. The long-term model treats combat time as an **action
      budget over real wall-clock**:

      - **Per-actor cadence.** `Fight.Order` stops driving an
        `ActiveIdx` round-robin. Each `ActorEntry` carries
        `NextActAt time.Time`; the bucket pulses at ~1 s and
        `tickRoom` drains *every* actor whose `NextActAt <= now`
        in initiative order. Initiative still rolls at `Start` but
        only seeds the first `NextActAt` (low init = slight delay
        on the first swing) rather than picking who acts ever.
      - **Action cost.** Every `Action` carries a base duration
        and stamina cost. `combat.actorActionCost(core, eq, action)`
        is the central pure function — easy to test, easy to tune.
        After resolution, `NextActAt = now + cost(action) ×
        speedFactor(core, eq)` and `Core.Stamina -= action.Stamina`.
      - **Speed factor** folds in (multiplicatively): base actor
        speed (Dex, race, class), worn-armor weight class, wielded-
        weapon weight, feats that reduce specific penalties.
        Examples: Aiel-spear-leather ≈ 0.7×; Borderlander-greatsword-
        plate ≈ 1.8×; Lan-greatsword-mail ≈ 1.1× via Blademaster
        feat that halves the weapon-weight term. The "Lan exception"
        is not a special case — it's the same machinery with a feat
        modifier.
      - **Action menu broadens.** Today: `Attack | Parry | Flee`.
        Long-term: same plus `power` / `quick` variants of attack
        (damage-vs-cost trade), `throw <weapon> <target>` (ranged,
        consumes the wield slot), `dodge` (short defensive boost,
        cheap, Dex-favored — pairs with `parry` which is weapon-
        favored), and `reposition` / `sidestep` (flat-foots last
        attacker, no damage). Aiel naturally chain
        `quick → dodge → throw → reposition` because their speed
        factor + stamina pool support it; a plate Borderlander
        literally cannot — the action math prices them out even
        if they queue the verbs.
      - **Stamina pool** as the burst limiter. New `Stamina`,
        `StaminaMax`, `StaminaRegen` on `creature.Core`; rides the
        existing `tick.Buckets.Regen` pulse for refill. Without
        this, low-cost actions would spam forever. Race table
        seeds different pools (Aiel large+fast, Ogier large+slow).
        Stamina refill rate is reduced by armor weight, so plate
        not only swings slower but recovers cheap actions slower.
      - **Iterative attacks fold in** as "your action cost is low
        enough that you queue the next swing immediately." A high-
        BAB character's `tickRoom` pass drains 2–4 queued attacks
        per pulse at +0/-5/-10/-15 attack bonus, replacing the
        D&D 3.x explicit-iteratives mechanic with cadence math.
      - **Per-verb `Lag` decouples from combat pacing.** Today the
        verb's 3 s `Lag` (Phase E #26) is the dominant gate; once
        the engine drives swings, `Lag` shrinks to ~0.5 s as an
        input-fairness floor. Combat cadence becomes a property of
        the *fighter*, not of the *typist*.
      - **Persistence pressure.** A 1 s bucket × 5 simultaneous
        fights with HP write-back per swing is ~5–25 UPDATEs/s on
        `characters`/`mob_instances`. SQLite at our scale handles
        that, but a write-coalescing pass on the persist.Manager
        (dirty-bit aggregate for HP / stamina) becomes worth
        scheduling before content scales the fight count up.
      - **Echo line volume.** `CombatHit`/`CombatMiss` lines stream
        2–4×/s in a hot fight. Great for *feel*, but the per-
        character combat brief (currently one full line per event)
        needs a compact mode — likely an opt-in `combat brief`
        toggle that collapses runs of misses into a single
        "you swing wildly (×3)" rollup. Pairs with the prompt
        templating `%t` slot reserved in §2 for an in-combat HP/SP
        gauge.
      - **Balance complexity.** Once cost is a function of
        `(race × class × feats × armor × weapon × action)`, every
        new content addition needs a tuning pass and "is Aiel OP"
        becomes a real question. Mitigated by keeping
        `actorActionCost` as one pure function with a YAML tuning
        table sibling to the existing chargen catalog.

      Long-term direction lock-in: per-actor cadence drives the
      whole subsystem; everything else (action variety, stamina,
      racial speed, feats, iteratives) is downstream of it. The
      MVP `[~]` initiative/round structure item above stays
      shipped — this item *replaces its semantics* once
      cadence work begins, but the `Fight` / `Manager` / `Action`
      / event-bus surfaces all carry over with field additions,
      not a rewrite.

      **Slice 60 (Phase L foundation) landed 2026-05-10.**
      `ActorEntry.NextActAt` / `LastActedAt` plus
      `combat.DefaultActionCost(kind)` replace round-robin
      `ActiveIdx`. Combat bucket cadence dropped 4 s → 1 s; verb
      `Lag` on `attack`/`parry`/`flee` dropped to 500 ms (lag is
      now input-fairness only). `RoundStarted` fires per
      actor-act; `Fight.Round` is the monotonically-incrementing
      per-fight act counter. `ParryingUntil` stamp moves to
      `round + 1` so the stance covers exactly the next incoming
      swing. Action cost is flat-table for now (Attack 3 s,
      Parry/Flee 2 s, idle 1 s) — slices 61–66 layer variant /
      gear / race / stamina / feats on top.

      **Slice 61 (attack variants — power / quick) landed
      2026-05-10.** `combat.AttackVariant` (`VariantNormal` /
      `VariantPower` / `VariantQuick`) lives on `Action.Variant`
      (zero value = Normal, so existing call sites stay green).
      `DefaultActionCost(kind, variant)` scales the base 3 s
      Attack cost by 1.0 / 1.5 / 0.6 → 3.0 / 4.5 / 1.8 s; Parry
      and Flee ignore Variant. `RollAttack` gained a trailing
      `bonus int` parameter for the variant attack-roll modifier
      (Normal 0 / Power -2 / Quick +1) so #62 (gear), #63 (race),
      and #65 (feats) can compose onto the same term.
      `RollDamage` gained a trailing `variant AttackVariant`
      parameter — variant damage factor applies after crit-mult,
      then re-floors to 1 so Quick on a 1-base hit still deals 1
      and Power-crit chains both multipliers. `CombatHit` /
      `CombatMiss` carry `Variant` so attacker-side echo lines
      pick variant-flavored copy ("You lunge with a power
      strike", "You flick a quick jab"); defender + room
      narration stays generic for now (variant compaction is
      slice #67). Verbs: `attack <target> [power|quick]`,
      `power <target>`, `jab <target>` — all share Auth, Lag, and
      completer via a private `runAttack` helper. Re-issuing
      during a fight overwrites the queued variant.

      **Slice 62 (gear-driven cadence — weapon weight + armor
      encumbrance) landed 2026-05-10.** New `combat.ActionCost(base,
      weaponWeight, armorWeightClass)` pure fn multiplies the
      kind/variant base by a weapon factor (unarmed 0.9× ; ≤2 lb
      0.8× ; ≤10 lb 1.0× ; ≤15 lb 1.3× ; >15 lb 1.5×) and an armor
      factor (none/empty 1.0× ; light 1.05× ; medium 1.15× ; heavy
      1.3×). `combat.ResolveGearFactors(ctx, items, eq)` looks up
      `SlotPrimaryWield` (weight from `repo.Item.Weight`) and
      `SlotArmor` (class from `repo.ArmorStats.WeightClass`); nil
      items repo / lookup failures degrade to the unarmed/naked
      baseline so combat never panics on gear glitches. Manager
      gained `actorActionCost(ctx, ref, action)` plus
      `resolveEquipment(ctx, ref)`, mirroring `resolveCore`; the
      single `tickRoom` callsite swapped from `DefaultActionCost`.
      `internal/cmd/score.go` renders a new
      `Combat: 1.95x (1.50 weapon x 1.30 armor)` line in Vitals
      directly under the existing movement Speed; `NewScore` took
      an `items repo.ItemRepo` parameter (nil yields the same
      unarmed/naked baseline as the resolver). QA fixture: room
      `test.qa.speed_range` (off the hub via `u`), dummy
      `test.qa.dummy_speed`, kit `test.qa.weapon_greatsword` (16 lb
      two-handed), `test.qa.weapon_dagger` (1 lb light),
      `test.qa.armor_plate` (heavy). Mob-side equipment authoring
      deferred (no `mob_templates.equipment_json` yet) — tester
      carries the gear and the dummy just stands still.
      Variant-cadence tests adjusted to the new unarmed/naked
      baseline (3 s × 0.9 = 2.7 s for Normal; 4.5 × 0.9 = 4.05 s
      Power; 1.8 × 0.9 = 1.62 s Quick) — the base table tested in
      `TestDefaultActionCost_Variants` stays unchanged.

      **Slice 63 (racial speed + stamina pool) landed 2026-05-11.**
      Migration 0049 adds `stamina_current` / `stamina_max` /
      `stamina_regen` int32 columns to `characters` (mirroring the
      HP shape from 0009); `creature.Core` gains the same trio.
      New `creature.ProfileFor(Race) RaceProfile` is a pure-Go
      lookup table (no chargen YAML catalog yet — the live roster
      is a 2-value enum; Aiel/Trolloc/Myrddraal land when chargen
      grows the surface): Human is 1.0× / 100 / 2, Ogier is
      1.2× / 150 / 1. New `combat.ApplySpeedFactor(d, factor)` is
      folded into `actorActionCost` after gear factors so an
      Ogier's 1.2× tax stacks with weapon + armor weight. New
      `combat.DefaultActionStamina(kind, variant)` is the per-
      action cost table (Attack/Normal=5, Power=12, Quick=3,
      Parry=4, Flee=8); `EnqueueAction` refuses with
      `ErrInsufficientStamina` when a character's
      `StaminaCurrent` is below the cost. Pre-0049 characters and
      tests with `StaminaMax==0` are treated as "unconfigured
      pool" and skip the gate so existing rows still fight without
      a re-finalize. `resolveAction` calls `drainStamina(ctx, ref,
      action)` at the top so parry / flee pay their cost on
      resolution. New `repo.CharacterRepo.RecordStamina(ctx, id,
      current)` is the narrow write (mirrors `RecordHP`) used by
      both the drain and the regen ticker. New
      `combat.NewStaminaTicker(candidates, chars, items, log)`
      subscribes to `tick.Buckets.Regen` (30 s) and tops
      `StaminaCurrent` toward `StaminaMax` at `StaminaRegen` per
      pulse, halved (rounded down, floored at 1) by heavy body
      armor via `combat.EffectiveStaminaRegen(base, class)`;
      regen halts while `HPCurrent <= 0`. `internal/cmd/score.go`
      renders a new `Stamina: cur / max (+N/pulse)` row between
      `Speed` and `Combat`, showing the *effective* regen so the
      sheet matches what the player observes. Chargen finalize
      stamps the racial profile onto the new character's Core
      (pool starts full). Verbs surface refusals distinctly:
      attack → "You're too winded."; parry/flee → "You're too
      winded to parry|flee." QA fixture: the existing
      `test.qa.speed_range` placard gained a slice-63 recipe block
      (drain via repeated power swings, observe the winded
      refusal, wait one Regen pulse, then wear plate to verify the
      halved regen).

      **Slice 64 (new action verbs — dodge / throw / sidestep)
      landed 2026-05-11.** Three new `ActionKind` values
      (`ActionDodge`, `ActionThrow`, `ActionSidestep`) join the
      existing Attack/Parry/Flee with their own cadence + stamina
      entries: `DefaultActionCost` 1.0 s / 2.0 s / 0.5 s,
      `DefaultActionStamina` 3 / 6 / 2. `Fight.DodgeUntil` is the
      new round-keyed stance map (mirror of `ParryingUntil`); the
      attack resolver reads it post-snapshot, adds +4 to the
      defender's effective Defense for that one swing, and grants
      flat-foot immunity (overrides a concurrent FlatFootedUntil
      entry). A swing turned from hit→miss by the +4 publishes
      the new `CombatDodgeAvoided` event so cmd-layer subscribers
      can render the active "you twist aside" line instead of a
      passive miss; the stance consumes either way (one swing's
      worth of evasion). `pruneDead` clears `DodgeUntil` alongside
      `ParryingUntil` / `FlatFootedUntil`. Sidestep stamps
      `FlatFootedUntil` on the named attacker (existing read path
      picks it up) and publishes `CombatStance{Kind:"sidestep",
      Target:<attacker>}` — no new map needed. Throw is a one-shot
      ranged variant: gates on `WeaponStats.Range == "thrown"`,
      rolls a Normal-variant attack with the weapon stats, then
      clears `SlotPrimaryWield` via `RecordEquipment` and drops
      the item via `ItemRepo.SetParent` (into the freshly-spawned
      corpse via the new `latestCorpseInRoom` helper on kill) or
      `ItemRepo.SetRoom` (onto the floor on miss / hit-but-alive).
      Three new verbs in `internal/cmd/` follow the parry/attack
      templates: `dodge` (no args), `throw <weapon> <target>`,
      `sidestep <attacker>`. All three carry `Lag: 500 ms` per
      the slice-60 input-fairness floor. QA fixture: new room
      `test.qa.evasion` (linked off `test.qa.speed_range` to the
      north) with a rolling practice dummy + a brace of throwing
      knives (`range: thrown`); laminated card walks the full Aiel
      chain `attack quick → dodge → sidestep → throw`.

      **Slice 65 (feats that modify cadence) landed 2026-05-11.**
      `chargen.Feat` gains four optional cadence-modifier fields
      (`weapon_weight_penalty_mul`, `armor_weight_penalty_mul`,
      `stamina_cost_mul`, `stamina_regen_add`) validated in the
      catalog loader. A new `Catalog.FeatByHashedID` reverse map
      (built eagerly at `Load` end) gives the combat hot path
      cheap int32 → `*Feat` lookup over the FNV-32a hashes
      already stored on `creature.Core.Feats`.
      `combat.FeatModifiers` aggregates the per-feat fields in
      one resolver call; `ApplyFeatGearAttenuation` rewrites
      gear factors as `1 + (factor-1)*mul` so feats attenuate
      *penalties* (>1.0×) without rewarding already-light gear.
      The aggregate folds into `actorActionCost` (gear-step
      attenuation), `drainStamina` (cost-mul, rounded to
      nearest), and `StaminaTicker` (regen-add stacks before the
      heavy-armor halving in `EffectiveStaminaRegen`). Four new
      general feats seeded in `feats.yaml`: `feat_blademaster`
      (weapon mul 0.5), `feat_light_step` (armor mul 0.5),
      `feat_endurance` (cost mul 0.8), `feat_iron_constitution`
      (regen +1). `feat_two_weapon_grace` deferred until
      tickRoom grows an off-hand swing. Manager gains
      `cat *chargen.Catalog` + `SetCatalog`; cmd/server/main.go
      wires it after construction. `score` Combat line cites
      active contributors in brackets (`[Blademaster]`); both
      `score` and the hot path go through the same resolver so
      the rendered list matches what's firing.

      **Slice 66 (iterative attacks via cadence drain) landed
      2026-05-11.** Replaces the D&D 3.x "+6 BAB unlocks a second
      attack at -5" mechanic with cadence math: when a high-BAB
      attacker's `NextActAt` fires, `tickRoom` resolves the queued
      Attack `1 + IterativeCount` times back-to-back at successive
      -0/-5/-10/-15 attack-roll penalties, accumulating costs so
      `NextActAt` advances by `swings × actorActionCost`.
      `combat.IterativeBonusesFor(bab int16) []int16` is the pure
      tier table (BAB 1–5→1 swing, 6–10→2, 11–15→3, 16+→4); pinned
      onto `ActorEntry.PendingSwings` + `IterativeBonuses` at
      `Start` from `core.BAB`. `resolveAction` gains a trailing
      `extraAttackBonus int` parameter threaded to `RollAttack`
      atop the existing variant bonus — defensive / movement kinds
      (Parry/Dodge/Flee/Throw/Sidestep) ignore the param and stay
      single-resolution. `Manager.hasStaminaFor` mirrors
      `drainStamina`'s cost math non-mutating; the drain loop's
      pre-swing gate breaks the chain when stamina is dry or when
      the target died/fled on a prior swing (`iterativeTargetGone`
      reads `Fight.Dead`/`Fled`/`Order` under `m.mu`). First swing
      always fires — the EnqueueAction gate already approved it.
      `score` sheet's Combat block adds a `Swings: N (-5/-10)`
      line when the tier exceeds 1.

## 12. Skills, spells & progression

- [ ] Class / archetype model — table-driven `classes` (id, name,
      hit_die, bab_progression, save_progression `{fort,ref,will}`,
      class_skills, skill_points_per_level, weapon_armor_proficiency,
      class_features by level). The seven WoT classes (Algai'd'siswai,
      Armsman, Initiate, Noble, Wanderer, Wilder, Woodsman). Multi-
      class supported via `class_levels: map[Class]int` summed for
      `character_level`. Race (Human / Ogier) separately gates stat
      ranges, height/weight, and innate abilities. Background
      (eleven from §9) supplies starting gear, languages, height-mod.
      All selected during character-create after the ability-score
      roll/buy step.
- [~] Skill tree with ranks / training — `character_skills`
      (`character_id`, `skill_id`, `ranks`, `is_class_skill`). Class-
      skill cap = `character_level + 3`; cross-class cap = ½ that.
      Skill points per level from class table (Int mod adds, ×4 at
      1st level). Skill checks roll `d20 + ranks + ability_mod +
      misc`. Caps and skill list live in `skills` table seed.
      **Mid-game spend landed 2026-05-07** — Phase E #24. New
      `learn` verb spends `pending_skill_points` (deposited at
      level-up by `train` — see "Levels & XP curve" below).
      `chargen.HashID` hoisted from `internal/mode/chargen_features.go`
      so cmd-layer spend verbs share int32 keys with chargen-
      persisted ranks. `repo.RecordSkillRank(ctx, id, skillID,
      newRanks, isClassSkill, newPending)` — atomic UPSERT in a
      sqlite TX (read skills_json → merge → UPDATE skills_json +
      pending_skill_points). `learn <id|#> [n]` works anywhere
      (no trainer required); class-skill / background-skill picks
      cost 1 pending point per rank, cap = `level + 3`,
      `IsClassSkill=true`. `learn info <id>` is read-only. Refusals
      don't mutate or audit; success writes one `learn` audit row.
      **Cross-class slice landed 2026-05-14** — `learn` now lists
      every catalog skill. Cross-class picks (any catalog skill
      that's not in `ClassLevels` class-skills ∪ background skills)
      cost 2 pending points per rank, cap = `(level + 3) / 2`
      (floor), stored with `IsClassSkill=false`. Menu header shows
      dual cap (`"N (class) / M (cross)"`); per-row tag is
      `[class]` / `[bg]` / `[cross]` with bucket-specific cap.
      `learn info` shows Cost/rank + Rank-cap. Helper changes in
      `internal/cmd/learn.go`: `classSkillSet`,
      `crossClassSkillRankCap`, `skillCostAndCap`,
      `isClassOrBackgroundSkill`, `allLearnableSkillIDs`. No schema
      change; existing `IsClassSkill=true` rows untouched. Phase E
      #24 closed. Pending: prefix-disambiguation for skill tokens,
      skill-check rolls (`d20 + ranks + mod`) against skill DCs in
      combat / weave paths.
- [ ] Weave (One Power) list with slot levels — replaces the
      generic spell/mana model. `weaves` table (id, name, level
      0–9, school/affinity (Air/Earth/Fire/Water/Spirit — multiple),
      talent_required, rarity (`Common`/`Rare`/`Lost`), cast_time_
      ticks, target_type, effect script ref). `weave <name>
      [target]` validates: caster `embraced` (or full-round to
      embrace), gender vs source, ability threshold (Initiate
      Int/Wis or Wilder Cha/Wis ≥ 10+level), affinity covered (or
      angreal compensates), free slot at that level, talent gating
      for Talent-only weaves, line-of-sight. Locks caster for
      `cast_time` ticks (interrupt on damage forces a Concentration
      check), consumes a slot of that level (or higher), accrues
      `madness` for men, then resolves via the effect script (§15).
      No mana pool — slots refresh after 8h rest.
      **Mid-game weave learning landed 2026-05-08** — Phase E #28.
      Migration 0043 added `weave_teachers` (1:1 to mob_template
      with `max_level_taught` + `affinity_filter` PowerSet); the
      optional `weave_teacher:` YAML block on a mob seeds the row.
      Chargen `Weave` gained `practice_cost` (validated `>= 0`).
      `progression.ComputeLevelUp.PracticeDelta = 1` per level for
      every class; `LevelUpFields.PracticePointsDelta` deposits
      into `characters.practice_points` (column from 0009 was
      previously unwritten). `learn weave` now branches: with a
      teacher present in the room it drains practice_points via
      `RecordWeaveStudy(ctx, id, weaveID, newPP)` and applies the
      teacher's level cap + affinity filter intersected with the
      channeler's own affinities; without a teacher the existing
      `pending_weaves` chargen-pool drain runs verbatim. Cast
      time / Concentration / mana-pool / slot-refresh-on-rest
      mechanics still pending — slot refresh itself landed in
      Phase E #27 (off the Regen bucket, 8h wall-clock).
- [x] Levels & XP curve — d20 geometric XP table
      (`xp(n) = 1000 × n × (n-1) / 2`; cap level 20 v1). Level-up
      grants: roll new HD for HP (deterministic avg + Con mod, no
      full-heal), +1 BAB by class progression, save bumps, skill
      points, possibly a feat (every 3 levels) or ability increase
      (every 4), new class features and (for channelers) new weave
      slots. Stored on character; `train` command at a trainer NPC
      commits the level-up. **Closed 2026-05-07** — Phase E #23
      across 4 slices: (1) `internal/progression` package with
      `XPForLevel` / `LevelForXP` / `XPToNext` (MaxLevel=20) +
      read-only `xp` verb showing pending levels.
      (2) Migration 0038 `trainers` table (1:1 mob_template →
      class id) + `repo.TrainerRepo` + optional `trainer:` YAML
      block + `train` verb stub (resolver, no mutation).
      (3) Level commit. `progression.ComputeLevelUp(ch, cat,
      classKey) → LevelGains` (pure function — recomputes
      ClassLevels + HP/MaxHP/BAB/Saves; no full-heal; multiclass
      at-will via separate trainers; hard catalog-miss refusal).
      `repo.RecordLevelUp(ctx, id, LevelUpFields)` atomically
      writes the new totals; `train` audits on success.
      (4) Pending pools. Migration 0039 added
      `pending_feats` / `pending_skill_points` /
      `pending_ability_bumps` / `pending_weaves` (int32 NOT NULL
      DEFAULT 0). `LevelGains` extended with per-pool deltas at
      d20 cadence (feat per 3 levels, ability per 4, skill =
      max(1, class.SkillPoints + IntMod), weave for channelers).
      `RecordLevelUp` accumulates the four counters in the same
      UPDATE; `train` appends a single cyan "You gained N feat
      pick, M skill points." line on success, suppressed when no
      pool grew. Spend side: `learn` shipped as #24; `feat` /
      `bump <abil>` / `learn weave` shipped as Phase E #25
      (2026-05-07). Verb `feat` (not `pick feat` — `pick` collides
      with the lockpicking verb): `feat` / `feat <id>` /
      `feat info <id>`. `bump <ability>` accepts str/dex/con/
      int/wis/cha (or full names); hard cap at 20. `learn weave`
      is channeler-only and affinity-gated against
      `Channeling.Affinities`. New repo methods
      `RecordFeatPick` / `RecordAbilityBump` / `RecordWeavePick`
      mirror the `RecordSkillRank` shape (caller passes absolute
      new pending value; `RecordWeavePick` returns
      `ErrNotChanneler` on a nil Channeling row as defense in
      depth). All four pending pools now have a drain path; the
      level-up cycle is end-to-end functional. Refusals (empty
      pool, cap, duplicate, miss-affinity, unknown id) do not
      mutate or audit; success writes one `feat` / `bump` /
      `learn` audit row.
- [x] Affects / buffs / debuffs with durations — `creature_affects`
      list `(source_id, name, modifiers []StatMod, duration_ticks,
      tick_effect)`. **#25 closed 2026-05-10** across three slices.
      Remaining producer threads (combat-on-hit, weave-cast, healer
      NPC, light fuel, player dispel) live in
      `affects_followups.md` and are blocked on §D crit polish or
      Phase F/G surfaces; they don't gate #25 closure.
      Phase E #25 slice 1 landed 2026-05-09: player
      `affects` inspect verb + admin `affect` / `dispel` producer
      verbs. `affects.Apply` now has live callers (admin sentinel
      Source = -1); the existing `SessionTicker` decrements
      durations, `Effective` folds StatMods at attack time, and
      the `Expired` subscriber emits fade lines. **Slice 2 landed
      2026-05-10:** foundation gaps + first player-facing producer.
      `creature.Affect` extended with `ConditionMask` (Effective
      ORs into `Core.Conditions`) and `TickDamage` (ticker folds
      into HPCurrent when `TickEffect != ""`). Stacking cap of 4
      per affect Name from distinct Sources; eviction drops the
      shortest remaining duration. New `internal/effects/` YAML
      catalog (mirrors chargen embed-with-override pattern) seeded
      with `healing_draught` (HoT regen), `weak_poison` (DoT), and
      `bull_strength` (Str.Current +2). New `internal/affects/
      tick_effect.go` ApplyTickEffects pure function clamps HP at
      0 / HPMax. SessionTicker.tickOne calls `RecordCore` for the
      HP delta and publishes a new `affects.TickDamaged` event;
      cmd-layer subscriber emits per-tick lines via WriteAsync.
      DoT-death routes through new `combat.Manager.HandleAffectDeath
      (ctx, charID)` — wraps the existing `handleCharacterDeath`
      pipeline with empty Killer so XP debt / respawn / events
      reuse the §19 path. New `quaff <potion>` verb resolves a
      consumable from inventory, looks up the effect via
      `ConsumableStats.EffectID` (chargen.HashID round-trip),
      applies via `affects.Apply` with sentinel Source = -2
      (renders as "potion"), and `Delete`s the item. Charge field
      ignored in V1 — multi-charge consumables wait for an
      `ItemRepo.UpdateStats` method (slice 3+). Loader extension:
      consumable items accept `effect_id_string:` in the YAML
      `stats:` block; world.translateConsumableEffectID translates
      to int32 via chargen.HashID. Boot-time
      `validateConsumableEffectRefs` cross-checks every consumable
      EffectID against the loaded effects catalog so a typo in
      `effect_id_string:` fails the boot loudly. Three seed
      potions added to the Winespring Inn kitchen items file.
      Tick cadence constant `affects.TickSeconds` corrected from
      30 to 6 (the actual `Buckets.Affects` cadence). **Slice 3
      landed 2026-05-10:** foundation polish + multi-charge.
      New `ItemRepo.UpdateStats(ctx, itemID, stats)` write path
      (sqlite + memory) overwrites items.stats_json with a
      type-checked replacement; `quaff` now branches on
      ConsumableStats.Charges — 0 = unlimited (item never
      consumed, mirrors ToolStats convention), 1 = final dose
      (existing Delete + cleanInventoryRef), >1 = decrement via
      UpdateStats and item stays in inventory. Existing slice 2
      latent type-assertion bug fixed (loaded items carry
      `*ConsumableStats` pointer; quaff was asserting value
      type) — new `consumableStatsOf` helper handles both.
      `creature.Affect` extended with `ExpireMessage string`
      (no migration; affects_json carries it).
      `effects.Effect.ToAffect` now propagates `MessageOnExpire`.
      `affects.Tick` signature changed: returns
      `[]creature.Affect` (the dropped entries) instead of
      `[]string` so subscribers can read ExpireMessage.
      `affects.Expired` event reshaped: `Names []string` →
      `Entries []ExpiredEntry` (Name + Message pairs).
      cmd-layer Expired subscriber renders the authored message
      when present; falls back to "Your <name> fades." for
      admin-applied affects. Combat's end-of-round Tick
      publisher (manager.go::tickParticipantAffects) updated to
      build Entries identically. Seed `tr.potion_healing_draught`
      bumped to `charges: 3` so a builder can exercise the
      multi-dose path end-to-end. Out of slice 3:
      weave-cast and combat-on-hit producers (need cast verb +
      §D crit polish), healer NPC service, player-driven
      dispel weave, light/torch fuel burn-down (different
      bucket). Must support the
      WoT condition
      enum from §9:
      `AbilityDamaged, AbilityDrained, Blinded, Checked, Cowering,
      Dazed, Deafened, Disabled, Dying, Entangled, Exhausted,
      Fatigued, FlatFooted, Frightened, Grappled, Held, Helpless,
      Panicked, Paralyzed, Pinned, Prone, Shaken, Stable, Staggered,
      Stunned, Unconscious`. Plus environmental flags driven by
      surroundings rather than affects (`Flanked, Charging,
      TotalDefense, FightingDefensively, Concealed%, Cover`). The
      `combat`/`regen` buckets decrement durations; `tick_effect`
      fires per-tick (DoT, regen, fear check, madness gain while
      embraced). Stacking rules: same `(source, name)` refreshes;
      different sources stack up to a per-affect cap.
- [~] Cooldowns and global lag — per-skill `cooldown_until` on the
      character + the §4 `Command.Lag` global lag. Display in
      `cooldowns` command grouped by skill family. Phase E #26
      slice B landed 2026-05-09: migration 0047 added
      `skill_cooldowns_json` (placement: between `xp_debt` and
      `auth_level`); `Character.SkillCooldowns map[int32]time.Time`
      keyed by `chargen.HashID(skillID)`; new
      `CharacterRepo.RecordSkillCooldown` (read-modify-write, prunes
      past-now entries on every write). New verbs: `cooldowns`
      (Player; lists active entries sorted alphabetically — V1 has
      no `Family` field on chargen.Skill so the spec's "grouped by
      skill family" is deferred until that schema lands) and
      `cooldown <player> <skill> <seconds>` (Admin, audited;
      `<seconds>=0` clears, refusals do not audit). V1 producer is
      admin-only — real player skill-check verbs (track / hide /
      lockpick) will stamp at success when those gain skill-check
      gates.

## 13. Communication

- [x] `say` (room-scoped) — `internal/cmd/comm.go::NewSay`. Emits
      `You say, "<text>"` to the caller and `<Name> says, "<text>"`
      to every other session whose CurrentRoomID matches the
      speaker's room. `sanitizeChat` strips control bytes, caps at
      1024 runes, and defangs cfmt template syntax (`{{ }} ::`)
      so a player can't inject styling. NPC `on_say` trigger
      hookup lands with §15.
- [~] `tell` / `whisper` (private) — `tell` shipped
      (`internal/cmd/comm.go::NewTell`); resolves the recipient via
      `session.Registry.FindByCharacterName`, sets the recipient's
      `Session.LastTellFrom`, and renders both sides. `reply <text>`
      writes back to LastTellFrom (`NewReply`). Pending: `whisper`
      (room-local with bystanders), ignore-list filtering, and the
      `nochannels` flag — all blocked on §6 / §13 ignore plumbing.
- [~] `shout` / `yell` (zone-wide) — `internal/cmd/shout.go` ships
      both verbs sharing a `zoneBroadcast` helper that loads the
      speaker's room, walks `sessions.Snapshot`, and `WriteAsync`s
      to every peer whose room resolves to the same `zones.id`.
      `shout` colors yellow, `yell` colors red. Self echoes
      synchronously. Pre-login peers, unscoped rooms (zone 0),
      and `silent`-flagged rooms are filtered. Deferred: small
      move-drain to discourage spam.
- [~] Channels (`ooc`, `gossip`, `newbie`) with on/off toggles —
      catalog table + repo
      (`internal/repo/channel{,_sqlite,_memory,_test}.go`,
      migration `0011_create_channels.sql`) seeded with `ooc` /
      `gossip` / `newbie`. `internal/cmd/channel.go::NewChannel`
      registers one verb per row at startup; no-arg toggles the
      caller's mute bit and write-throughs via
      `CharacterRepo.RecordChannelSettings`, args broadcasts
      `[<NAME>] <speaker>: <text>` to every other authenticated
      session whose mute bit is off. `channels` overview command
      lists each channel with the caller's on/off state. Per-
      character mute lives in `characters.channel_settings_json`
      (sparse — keys present only when muted) and is mirrored onto
      `Session.channelMuted` (crossMu-guarded) at game promotion.
      Pending: `min_level` enforcement / `newbie` auto-leave at
      level 10 (blocked on §12 level computation), §8 event-bus
      dispatch for admin snoop, and `auction` / `clan` channels
      (clan needs clan_id from §17).
- [~] `who` — `internal/cmd/who.go::NewWho` iterates
      `session.Registry.Snapshot()`, renders each bound session
      with its character name, "(you)" marker, and idle time
      (≥30s) computed from `Session.LastInputAt` (stamped by the
      dispatcher on every command). Sessions still in login /
      character-select render as `(connecting)` so `who` can't be
      used to enumerate IPs. Pending: class / level / title columns
      (blocked on char-create populating those fields), AFK flag,
      and sort filters (`who level` / `who class <name>`).
- [ ] Ignore list / mute — per-character `ignored_names []string`
      capped at 50; tells/whispers/channel messages from ignored
      names are silently dropped on the receiver side. Admin
      override exists so staff can always reach a player.
- [ ] Mail between characters — `mail` table (`id`, `to_id`,
      `from_id`, `subject`, `body`, `sent_at`, `read_at`). Compose
      via the §5 multi-line editor. Commands: `mail`, `mail read
      <n>`, `mail send <name>`, `mail delete <n>`. Storage cap per
      character (e.g. 50 messages); login MOTD shows unread count.
- [ ] Bulletin boards / notes — `boards` table (id, name,
      min_read_level, min_post_level), `notes` table (board_id,
      author_id, subject, body, posted_at, expires_at). Boards
      live in specific rooms ("the town crier"); `note list/read/
      post/remove` while standing there. Auto-prune expired notes
      from a daily scheduler tick.

## 14. Inventory & economy

- [~] `inventory`, `get`, `drop`, `give`, `put`, `take` — `inventory`
      / `get` / `drop` / `give` shipped (`internal/cmd/inventory.go`)
      on top of migration `0017_item_owner.sql` (adds nullable
      `items.owner_character_id`, soft FK matching the `rooms.zone_id`
      pattern from §16). Items now carry an `OwnerCharacterID` field
      alongside `RoomID`; `ItemRepo.SetOwner` / `SetRoom` flip both
      atomically so the location invariant (exactly one non-null) holds.
      `inventory` lists held items in `Character.Inventory` JSON order
      (display ordering; SQL `owner_character_id` is the truth),
      renders carry weight + load band (Str-based d20 carrying-capacity
      table in `internal/cmd/encumbrance.go`), and the coin purse via
      `currency.Format`. `get` blocks `FlagNoTake`, blocks at the
      heavy-cap (Overloaded) load band, and broadcasts to room peers.
      `drop` blocks `FlagNoDrop`. `give <item> <name>` requires the
      target online + same room and gates on the recipient's encumbrance.
      `give <amount> <name>` parses via `currency.Parse` and transfers
      between purses with `ErrInsufficientFunds` surfaced cleanly.
      Keyword resolution accepts the ordinal `<n>.<keyword>` form
      (`get 2.sword`) via `internal/cmd/keyword.go::MatchItem` /
      `MatchMob`, used by both `examine` and the inventory verbs.
      `examine` now walks room mobs → room items → inventory items.
      `door` `lock` / `unlock` (§10) read keys from inventory rather
      than the room-floor placeholder. New CharacterRepo write-throughs
      `RecordInventory` / `RecordCoin` mirror the `RecordCore` pattern
      (single targeted UPDATE per call). Pending: `put` / `take from`
      container verbs (block on §9 container nesting), keyword
      disambiguation in non-inventory commands (move/teleport),
      coin weight in the carry calculation.
- [ ] `wear`, `wield`, `remove`, slot validation — `equipment`
      table keyed by `(character_id, slot)`. `wear <item>` picks
      first slot the item permits; `wear <item> <slot>` forces.
      Two-handed weapons consume `wield` + `hold`. Apply affects
      on equip, strip on remove, persist on logout.
- [x] Shops (buy, sell, list, value) — shopkeeper mob subtype with
      `shop` config (`buy_types []ItemType`, `sell_markup`,
      `buy_markdown`, `open_hour`/`close_hour`, `inventory []ItemID`
      with restock interval). `list` shows wares + price; `value
      <item>` previews sell price; refuses items the shop can't
      buy. Stock restocks via the `areaReset` bucket.
- [x] Banks / vaults (V1: balance / deposit / withdraw) — banker
      mob subtype with optional `banker:` YAML block carrying
      operating hours. Coin moves between `characters.coin` and
      `characters.bank_balance` via `RecordCoin`; deposit/withdraw
      audit on success. `transfer <name> <n>` (player-to-player
      wires) and item vaults are deferred to V2 — see
      `banker_followups.md`.
- [ ] Crafting recipes (later) — `recipes` table (id, name,
      skill_id, min_skill, ingredients []{ItemID, qty},
      result_item_id, result_qty, station_required). `craft
      <recipe>` consumes ingredients on success, partial on
      failure based on skill check.

## 15. Quests, scripts & NPC behavior

- [x] Quest engine (state machine per character per quest) — Phase
      F #31 landed 2026-05-09. No new migration: V1 reuses
      `characters.quest_log_json` (already shipped in 0009) and
      widened `creature.QuestProgress.QuestID` from int64 to
      string for catalog-id round-trip. V1 step types: `talk_to`,
      `kill_n`, `reach_room` (`fetch`/`deliver` deferred until item
      events land; `script` deferred to #32 Lua). Authoring lives
      in `internal/quest/default/<id>.yaml` (one file per quest)
      with `QUEST_DIR` env-override mirroring chargen / news.
      Cross-references against world content (mob_template +
      room ExternalIDs) validate at boot; a typo fails loudly.
      Two new dialogue Effects: `accept_quest` and `advance_quest`
      (closure-injected from `cmd/server/main.go` so
      `internal/cmd` and `internal/dialogue` stay free of the
      `internal/quest` import). Engine subscribes to
      `combat.CombatDeath` (kill_n decrement; new
      `MobTemplateID` + `MobTemplateExternalID` fields on the
      event so the template id survives the post-kill cleanup)
      and `world.PlayerEntered` (reach_room transition); talk_to
      advances via the dialogue advance_quest effect. Final-step
      transition grants XP via `RecordXP` and coin via
      `RecordCoin` with one optimistic-lock retry on
      `ErrCoinConflict` (mirrors the shop verbs). New `quest`
      verb (alias `quests`): bare lists active + completed, `info
      <id>` renders the step list with progress markers, `abandon
      <id>` drops an active entry (audited).
- [x] NPC dialogue trees — Phase F #30 landed 2026-05-08.
      Migration 0045 added `mob_templates.dialogue_json` (nullable
      TEXT). YAML mob entries gain an optional `dialogue:` block
      (sibling to `shop:`/`trainer:`/`weave_teacher:`/`triggers:`)
      with `root` + ordered `nodes`; the loader validates via
      `internal/dialogue.Validate` (rejects empty root, dangling
      `next`, unknown effect kinds, missing effect args) and
      persists the compact JSON encoding. `talk <mob>` resolves an
      NPC in the room, decodes its tree, and pushes the new
      `internal/mode.Dialogue` mode. The mode renders the current
      node prompt + numbered, flag-gated responses; accepts bare
      number, free-text keyword (case-insensitive substring match),
      or `bye`/`quit`/empty to exit. Effects: `set_flag`,
      `clear_flag`, `goto`, `push_mode` (closure-injected by the
      cmd-layer; no V1 targets), `end`. Per-character branch state
      (current node + flag bag) lives in-session and drops on
      logout — quest-bound persistent flags are deferred to #31.
- [x] Trigger / event system (`on_enter`, `on_say`, `on_attack`,
      `on_death`, `on_tick`) — Phase F #29 landed 2026-05-08.
      Migration 0044 added the `triggers` table; the YAML loader
      seeds it from optional `triggers:` blocks on mob_templates
      and rooms. `internal/trigger/` ships the in-memory
      `Registry`, an extensible `ActionRegistry` (V1 builtins:
      `noop`, `say`, `emote`), a fan-out `Runner`, and a
      `Dispatcher` wiring eventbus + `tick.Buckets.Phase` to the
      registered owners. New `world.PlayerSaid{Speaker, RoomID,
      Text}` event is published by the `say` verb after a
      successful room broadcast. Item-owned triggers and
      consecutive-fault auto-disable deferred until §32 ships
      the Lua action surface (deterministic V1 builtins don't
      need fault budgets).
- [~] Embedded scripting language — Phase F #32 slice 4 landed
      2026-05-10. Polish bundle: `room.players()` and `room.mobs()`
      return a 1-indexed Lua array of character / mob_instance IDs
      in the actor's current room (resolved at bind time from
      `b.Ctx.RoomID` so scripts can't snoop on other rooms).
      `clock.hour() → 0..23` and `clock.day() → int` expose the
      world clock (new `Clock.Day()` method on `internal/world/
      dayclock.go`). `target.classes(id)` returns the multiclass
      map keyed by chargen catalog class id (e.g.
      `{ armsman = 3, initiate = 2 }`); companion to
      `target.level(id)` which sums into a single int.
      `apply_affect` gained an optional 3rd arg (durationOverride
      int32; 0 means "use catalog default") so scripts can apply
      shorter/longer affects without authoring sibling catalog
      entries. APIBindings.ApplyAffect signature changed (3 args);
      LuaHooks struct extended; release wipe-list now covers
      `room` + `clock`. Two demo scripts added: `check_alone.lua`
      (room.players + apply_affect) and `night_warning.lua`
      (clock.hour gate). Slice 5+ defers combat mutations
      (deal_damage, blocked on §D crit polish), inventory
      take/transfer, async `wait()`, and `on_login`/`on_logout`
      events.

      Phase F #32 slice 5a landed 2026-05-11 (§D crit polish
      unblock). Four new APIBindings hooks shipped on the
      mutation surface: `deal_damage(target_id, amount [,
      source])`, `heal(target_id, amount)`, `transfer_item
      (item_id, to_owner_id)`, `drop_item(item_id)`. New combat
      entry points `combat.Manager.ApplyDamageExternal` /
      `combat.Manager.ApplyHealing` mirror the affect-death
      shape — raw amount (no DR / resists / crit roll), no
      threat-table mutation, lethal damage routes through the
      existing `handleCharacterDeath` / `handleMobDeath`
      pipelines. Two new events: `combat.ScriptDamageDealt`
      (with `Lethal` flag so the default-narration subscriber
      suppresses the "you suffer N damage" line on lethal hits)
      and `combat.ScriptHealingApplied` (Amount == 0 when
      target is already at full HP). Cmd-layer subscribers
      render default narration via `Session.WriteAsync` for
      unsourced damage / heal. Lua bindings take a single
      `target_id`; `resolveLuaTarget` tries `CharacterRepo.
      GetByID` first then `MobInstanceRepo.GetByID` so both
      kinds work behind one binding. No actor-kind guard at the
      trigger layer (mob-fired triggers legitimately damage /
      heal players); `drop_item` keeps a `ev.RoomID != 0`
      adapter guard so a context-less drop trips the fault
      budget instead of dumping into room 0. Killer attribution
      on lethal damage is anonymous in V1 (`ActorRef{}`); future
      slices may thread an authored hint through the source
      string. Two demo scripts shipped: `script_strike.lua`
      (emote + deal_damage) and `divine_heal.lua` (say + heal).
      Wipe-list extended with the four new globals.

      Phase F #32 slice 5b landed 2026-05-11. Three new APIs /
      events: `wait(seconds, "script_name")` defers a fresh
      `runner.Run` via `tick.AfterCtx` (range 1..300s; snapshots
      firing EventCtx so the deferred run inherits actor / room
      / event); `inventory(target_id)` returns a Lua table of
      `{id, name, external_id}` (wraps `ItemRepo.ListInInventory`,
      top-level only — container contents excluded); and the two
      new trigger event kinds `on_login` / `on_logout`
      (room-owned), backed by new `world.PlayerLoggedIn` /
      `world.PlayerLoggedOut` events. Migration 0051 widens the
      `triggers.event` CHECK via the SQLite table-rebuild dance
      (preserves the 0046 fault columns + the 0044 indexes).
      Login publish point: a package-level
      `mode.SetLoginPublisher` hook wired by `main.go` at boot
      and called from `promoteToGame` immediately after
      `SetInWorld` — chosen over threading a bus through
      promoteToGame's four call sites. Logout publish point:
      `handleConnection`'s defer block, guarded on
      `s.CharacterID != 0` so account-menu-only disconnects
      don't publish phantoms. Late-binding for `wait()`'s
      shutdown ctx: `main.go` declares a local var before the
      luaHooks block and back-fills after `signal.NotifyContext`;
      the wait factory captures a pointer and dereferences at
      fire time. Dialogue scripts get `inventory()` but NOT
      `wait()` (async inside interactive dialogue creates
      surprising UX). Two demos shipped: `wait_demo.lua` and
      `confiscate.lua`. Release wipe-list extended with `wait`,
      `inventory`. Deferred: `wait()` from dialogue, sub-second
      `wait()`, transitive `inventory()`, account-menu kick →
      `on_logout`, login ordering vs. `on_enter`.

      Phase F #32 slice 3 landed 2026-05-10. Slice 3 broadens the API surface for content
      authors with three composing closures: `apply_affect(target_id,
      effect_id)` (resolves through the effects catalog + the §E
      #25 producer pipeline; sentinel Source = -3 renders as
      "script" in the affects inspect verb), `give_item(target_id,
      external_id)` (clones a YAML-seeded item template into the
      target's inventory; mirrors the admin spawn path), and
      `target.hp(id)` / `target.level(id)` read APIs (multiclass
      level sums ClassLevels). Trigger handler guards mutations
      with the same actor-kind check as the V2 quest API
      (mob-fired triggers refused with classified fault); read
      APIs are unguarded. Dialogue's RunScript closure binds the
      same hooks, no extra guard (the dialogue actor is always
      the calling character). `LuaQuestHooks` renamed to
      `LuaHooks` (legacy alias kept). Two demo scripts added
      to the catalog: `bless_actor.lua` (apply_affect) and
      `gift_potion.lua` (give_item). Release wipe-list extended
      to cover `apply_affect`, `give_item`, `target` so per-call
      state can't leak across pool borrows. Slice 4+ defers
      combat mutations (deal_damage, blocked on §D crit polish),
      inventory mutations beyond give (take/transfer), room state
      iterators (room.players / room.mobs), and `wait()` async.

      Phase F #32 slice 2 landed 2026-05-09. `gopher-lua` is the
      chosen runtime. Slice 1
      shipped the foundation: an embedded
      `internal/scripts/default/<name>.lua` catalog (with
      `SCRIPT_DIR` env override), a sandboxed
      `internal/lua.Runner` (LState pool of 8, dangerous globals
      stripped, 50ms ctx timeout per call), and a new `lua`
      trigger action kind that resolves a script by name and
      runs it with a minimal V1 API: `say(text)`, `emote(text)`,
      `log(level, msg)`, plus a read-only `ctx` table exposing
      the EventCtx. Slice 2 wires the Lua runner into dialogue
      effects (`effects: kind: script`) and quest steps
      (`kind: script`), and adds the V2 API: `quest.accept(id)`
      / `quest.advance(id)` / `push_mode(name)` globals (nil-
      bound contexts register classified-error stubs so misuse
      trips the fault budget). Engine got a kind-agnostic
      `Advance(charID, questID)` covering both `talk_to` and
      `script` step kinds. Boot-time cross-refs reject typos
      against the script catalog for both surfaces. Slice 3
      defers `wait()` async scripts and richer mutation primitives
      (`player.give`, `mob.damage`, `on_login` / `on_logout`);
      `tedit` OLC defers to slice 4 / Phase G.
- [x] Sandboxing & resource caps for scripts — Phase F #32 slice 1
      landed 2026-05-09. The sandbox strips `os` / `io` / `debug`
      / `package` / `dofile` / `loadfile` / `loadstring` / `load`.
      Per-call wall-clock cap of 50ms via gopher-lua's SetContext
      (we don't use SetMx — it's a millisecond deadline that leaks
      a watchdog goroutine per LState). `safego.Go` recovers any
      runtime panic into `slog.Error` so a Lua-internal panic
      can't tear down the bus goroutine. Migration 0046 added
      `triggers.consecutive_faults` + `triggers.disabled`; the
      runner increments the counter when a Lua action returns an
      error wrapping `trigger.ErrActionFaulted`, auto-disables at
      `FaultThreshold = 5`, and resets to zero on any successful
      invocation. World re-deploys reset both columns.

## 16. Online creation (OLC)

- [~] `redit` / `oedit` / `medit` / `zedit` mode-based editors —
      one editor mode per aggregate (see §5). `redit` works on the
      current room by default or `redit <id>`; sub-commands
      `name/desc/exit/flag/sector/show/done`. Edits buffer until
      `done`, which writes to the SQLite world tables and re-syncs
      the in-memory cache.
      **Phase G #34 redit slice 1 landed 2026-05-12.** `redit` verb
      (AuthPlayer; gated by `cmd.CanEditZone`, so AuthAdmin or a
      `builder_zones` grant on the room's zone). `internal/mode/redit.go`
      buffers a draft of the room and exposes `show / name / short /
      desc / flag <name> [on|off] / sector / light / done / cancel /
      help`. `done` commits via the new `RoomRepo.Update` (memory +
      sqlite, with shared test) — preserves identity / location /
      coords / created_at, overwrites only the OLC-editable subset
      — and audits one `admin_audit` row with `verb=redit`,
      `target=<external_id>`, `args=<sorted comma list of changed
      fields>`. `cancel` discards the draft without auditing. No
      in-memory room cache exists today, so "re-sync" is implicit:
      the next `look` reads the updated row through the repo.
      Deferred to slice 2+: multi-line `desc` editor, `exit <dir>`
      subcommand (needs `ExitRepo.Update`), `extra <keyword>` for
      ExtraDescs, `oedit` / `medit` / `zedit` (each their own slice).
- [~] Permission gating (admin / builder roles) — `AuthLevel` enum
      `Player < Builder < Admin < Implementor`. OLC commands gated
      at `Builder`; admin-only verbs at `Admin`. Builders may be
      restricted to specific zones via a `builder_zones` table.
      **Phase G #33 landed 2026-05-12** as a per-zone-only design:
      `AuthLevel` stays {Guest, Player, Admin}; the new
      `builder_zones` table (migration 0055) keyed
      (character_id, zone_id) grants OLC rights on a per-zone basis.
      `BuilderZoneRepo` (memory + sqlite, shared test suite) +
      admin `grant <player> <zone>` / `revoke <player> <zone>` /
      `grants [<player>]` verbs (audited). On login,
      `postauth.promoteToGame` loads grants into a Session-cached
      map (`Session.IsBuilderFor`); admin `grant`/`revoke` refresh
      live targets in-place. `cmd.CanEditZone(s, zoneID)` is the
      single gate for #34's redit / oedit / medit / zedit. A global
      `AuthBuilder` enum tier was attempted and reverted — modernc-
      sqlite hung on the `ALTER TABLE DROP COLUMN` required to widen
      `auth_level`'s `BETWEEN 0 AND 2` CHECK. Per-zone-only is the
      cleaner semantic anyway.
- [ ] Versioned area saves with diff/preview before commit —
      `area_revisions` table keeps prior YAML serialization with
      `author_id`, `committed_at`, `message`. `diff` in editor shows
      changes vs HEAD; `commit "<msg>"` snapshots; `revert <rev>`
      restores. Optional git push of the YAML export to a builder
      repo as a follow-up.

## 17. Admin & moderation

- [~] `goto`, `transfer`, `summon`, `wizinvis`, `snoop` — `goto
      <player|room>`, `transfer <player> [<room>]`, `summon
      <player>`, and `wizinvis` (zero-arg toggle) all landed for
      AuthAdmin; player-name lookup wins on conflict in `goto`,
      NoTeleport rooms still resist, target gets the same async
      "world ripples" notice as `tp <user> <room>`. Wizinvis flag
      is session-scoped (no schema) and currently hides from `who`
      and `tell`-name completion / lookup; admins still see hidden
      peers with a `*` marker. Still pending: `wizinvis [level]`,
      "polite summon" prompt, and `snoop` with audited fan-out.
- [x] `shutdown` / `reboot` — both verbs accept
      `[<delay>] [<reason>]` (default 30s, clamped 0..1h) or
      `cancel`/`abort` to interrupt an in-flight countdown. Countdown
      broadcasts at T-{60,30,10,5..0}s via `Session.WriteAsync` to
      every live session, then closes the listener through the
      existing `signal.NotifyContext` cancel — same teardown path as
      SIGTERM (drain `wg`, run `persist.Manager.FlushAll`, stop
      buckets/scheduler/bus). `reboot` flips an `atomic.Bool` so
      `main()` `syscall.Exec`s the binary after `srv.shutdown()`
      returns; in-process FD/session restore (true copyover) is still
      a stretch goal. AuthAdmin only.
- [ ] `ban` / `siteban` / `kick` / `mute` — `bans` table (`pattern`,
      `kind` in `account|ip|cidr`, `reason`, `expires_at`,
      `created_by`). Login mode + accept loop both consult; CIDR
      via `net.ParseCIDR`. `kick <player> [reason]` closes the
      socket with a notice. `mute <player> <duration>` flips a
      flag blocking channel + say emits.
- [~] `spawn` admin command — `spawn mob <ext> [count]` and
      `spawn item <ext> [count]` ship in `internal/cmd/spawn.go`
      gated `AuthAdmin`. Mobs route through
      `MobTemplateRepo.GetByExternalID` + a fresh `MobInstance` per
      copy. Items use the seeded YAML row as a template — typed
      fields and Stats are deep-cloned (no aliasing across spawns)
      and a unique runtime external_id is minted (`<ext>#sp-<nanos>-
      <i>`) so the UNIQUE index holds. Default count = 1, capped at
      20. Tab completion offers `mob` / `item` then the matching
      template ids. Both `slog.Info("admin: spawn", ...)` and an
      `admin_audit` row land per successful spawn (Phase A 5).
      Pending: `spawn item <ext> in
      <container_keyword>`, `spawn mob <ext> at <room_id>`,
      reverse `despawn` / `purge`, dedicated item-template repo
      (split from item-instance repo when the world grows).
- [x] Audit log of admin actions — `admin_audit` table (migration
      0029) with `actor_character_id` / `actor_name` snapshot,
      `verb`, `target`, `args`, `ts`. `repo.AdminAuditRepo`
      (memory + sqlite, shared test suite) + the
      `internal/audit.Record(ctx, repo, session, verb, target,
      args)` helper. Wired into `spawn`, `teleport`, `goto`,
      `transfer`, `summon`, `wizinvis`, `shutdown`,
      `shutdown:cancel`, `reboot`. Synchronous write so the row
      commits before the verb's side effect (notably `shutdown`
      drain). The read-side `audit <name|verb> [since]` viewer
      verb is deferred (List API exists; no UX layer yet).
- [ ] Wizlist / staff hierarchy — `wizlist` command renders staff
      grouped by `AuthLevel` from accounts.auth_level. Title field
      ("Builder of the Plains") shown next to name.

## 18. Help & docs

- [x] In-game `help` command listing registered commands
- [ ] Topic-based help files (lore, mechanics, command detail) —
      `help/*.md` embedded via `//go:embed`; front-matter sets
      `keywords`, `min_level`, `category`. Indexed at startup.
      Commands auto-register a help stub from `Command.Help` so
      `help <verb>` always resolves.
- [ ] `help <topic>` resolution with prefix matching — exact match
      → keyword match → unique prefix match → ambiguity list, same
      ladder used by the command registry.
- [x] `news` / MOTD on login — `motd.md` embedded; `news` shows a
      list of dated entries from `news/*.md` with unread markers.
      Tracks per-character `last_news_seen`; login MOTD block notes
      unread count. Splash banner shown on TCP connect.
- [ ] In-repo developer docs beyond `CLAUDE.md` — `docs/`
      directory: `architecture.md` (subsystem map), `building.md`
      (running OLC + YAML loader), `protocol.md` (telnet/IAC
      gotchas + GMCP/MSDP packages), `contributing.md`. Linked
      from README.

## 19. Logging, telemetry, ops

- [x] `slog` structured logging at `LevelDebug`
- [~] Log levels controllable via env / config — `LOG_LEVEL` env
      (`debug|info|warn|error`) feeding `slog.HandlerOptions.Level`
      via `cmd/server/main.go::parseLogLevel`; unknown values
      default to `info` with a warning. Pending: a `loglevel
      <name>` admin command that swaps the level at runtime via
      an `slog.LevelVar`.
- [x] Request/command audit log per character — migration 0052
      adds `character_audit` (append-only, indexed on
      `(character_id, ts)` + `ts`). `internal/repo` ships
      sqlite + memory impls; raw column soft-capped at 4096
      bytes at insert. `internal/mode/game.go::CommandAuditFn`
      fires after every `Registry.Dispatch` (success + refusals)
      when the hook is installed. `cmd/server/main.go` gates on
      `audit.commands_enabled`; `audit.commands_exclude` filters
      high-frequency verbs (`look`, `prompt`). Phase J slice J3
      (#55), commit `8c921a3` (2026-05-12). Deferred: buffered
      async writes, retention/rotation.
- [x] Metrics endpoint (Prometheus) — `internal/metrics` registers
      a fresh `prometheus.Registry`, exposes `/metrics` (text
      format), `/healthz` (200 once `SetReady(true)` + DB ping
      succeeds; 503 otherwise), and `/debug/pprof/*` via stdlib
      `net/http/pprof`. Collectors: `wheelmud_commands_total
      {verb,result}`, `wheelmud_sessions_active`,
      `wheelmud_db_open_conns`, `wheelmud_build_info
      {version,commit,date,go_version}`, plus the Go + Process
      collectors. Bound to `cfg.Server.MetricsAddr` (loopback
      default). Lifecycle: `SetReady(true)` after telnet listener
      binds; `SetReady(false)` + `Shutdown` at drain start.
      `mode.Game.SetMetricHook` keeps `internal/mode` free of the
      metrics import. Phase J slice J5 (#54), commit `195d352`
      (2026-05-12). Deferred: `combat_swings_total`,
      `eventbus_published_total`, latency histograms.
- [~] Crash recovery — `internal/safego.Go(name, fn)` shipped:
      defers a `recover()` that logs panic + name + stack at
      `LevelError`. Wraps the shutdown-watcher, accept-loop's
      per-session goroutine, and the shutdown-drain in
      `cmd/server/main.go`. The telnet dispatcher
      (`telnet/server.go::runDispatcher`) inlines the same
      recover pattern to keep the telnet package boundary free
      of `internal/*` imports. The tick scheduler and eventbus
      already recover per-handler (existing inline recovers).
      Pending: metric counter for panics-by-name (waits on §19
      Prometheus endpoint) and any restart-on-panic policy
      (deliberate: a panic that survives recover means a logic
      bug to fix, not silently restart).
- [x] Profiling endpoints (`net/http/pprof`) — mounted on the
      metrics HTTP server's mux at `/debug/pprof/*` (Index +
      Cmdline + Profile + Symbol + Trace). Loopback default keeps
      pprof off the public net without a separate enable flag.
      Phase J slice J5 (#54), commit `195d352` (2026-05-12).

## 20. Configuration

- [x] `LISTEN_ADDR` env var
- [x] `DB_DSN` env var (default `wheelmud.db`; `:memory:` supported)
- [x] Config file (YAML) for ports, paths, feature flags —
      `internal/config` ships `Config` struct + `Load(path)` with
      precedence "defaults → YAML file → env". `cmd/server/main.go`
      takes a `-config <path>` flag (optional; env-only deployments
      are supported). Schema covers `server.listen_addr` /
      `server.metrics_addr` / `db.dsn` / `db.backup_dir` /
      `db.backup_interval_hours` / `db.backup_retention` /
      `world.dir` / `log.level` / `audit.commands_enabled` /
      `audit.commands_exclude`. Phase J slice J2 (#53), commit
      `b698b40` (2026-05-12). Deferred: per-environment file
      merging (`config.<env>.yaml`), feature-flag block,
      `auth.bcrypt_cost` (test-only knob today).
- [~] Per-environment overrides (dev/stage/prod) — env vars
      already override the YAML file for every supported field
      (J2). File-based per-env merging (`config.<env>.yaml`) and a
      production-mode safety guard are deferred.
- [x] Secrets via env, never committed — `.env.example` checked
      in at the repo root, mirrors every env var the server
      consumes; `.env` stays gitignored. Phase J slice J2 (#53),
      commit `b698b40` (2026-05-12). Deferred: required-secret
      validation at startup (no pepper / TLS key surface today).

## 21. Testing & CI

- [x] Unit + contract tests across `telnet`, `internal/auth`,
      `internal/db`, `internal/mode`, `internal/repo`, `internal/session`
- [x] Dependabot config (`.github/dependabot.yml`)
- [x] GitHub Actions — `.github/workflows/go.yml` matrix on
      ubuntu + macos (Go `1.25.x`) running `go vet`, `gofmt -l`,
      `go build`, and `go test -race -count=1 -covermode=atomic
      -coverprofile=coverage.out ./...`. Coverage summary printed
      and uploaded as an artifact (ubuntu leg). Separate
      `integration` job runs `go test -tags=integration` against
      `./test/integration/...`. Non-blocking `staticcheck` job
      surfaces findings without churning the queue. Phase J slice
      J1 (#52), commit `38544bd` (2026-05-12). Deferred: gosec,
      coverage gate threshold (baseline 72.5%; tighten later).
- [x] Integration test that drives the telnet protocol against a
      real listener — `test/integration/` (build-tag
      `integration`) spawns `cmd/server` as a subprocess with
      ephemeral ports + a tmp DB pointing at the real
      `./data/world` tree, waits for `/healthz=200`, then asserts
      `/metrics` emits the V1 collector set and a telnet
      connection sees the first-mode `Username` prompt after IAC
      negotiation. `TelnetClient.ReadUntil` strips IAC
      WILL/WONT/DO/DONT + SB…SE inline. Phase J slice J6 (#57),
      commit `cbd3a24` (2026-05-12). Deferred: chargen-flow
      scripting + minimal fixture world (today the test depends
      on the production world tree).
- [x] Fuzz tests on the IAC parser and command tokenizer —
      `telnet/iac_fuzz_test.go` ships `FuzzReadIAC`;
      `telnet/tokenize_fuzz_test.go` ships `FuzzTokenize` (with
      a per-token re-quote idempotence invariant) and
      `FuzzSplitOnSemicolon` (asserts no empty segments).
      `.github/workflows/fuzz.yml` runs each target nightly at
      06:00 UTC, 5 min default (override via `workflow_dispatch`).
      Phase J slice J1 (#58), commit `38544bd` (2026-05-12).
- [~] Coverage target tracked in CI — coverage summary printed
      per-leg (baseline 72.5% repo-wide). The hard gate
      threshold is deferred to a followup once the matrix is
      stable.

## 22. Packaging & deploy

- [x] `Dockerfile` + `docker-compose.yml`
- [x] `Makefile` targets (`build/server`, `run/server`, `run/live/server`)
- [x] Hot reload via `air` for dev
- [x] Versioned releases / `goreleaser` — `.goreleaser.yaml`
      (v2 schema) builds linux/darwin × amd64/arm64 with
      `CGO_ENABLED=0` + `-trimpath` and ldflags injecting
      `main.buildVersion` / `buildCommit` / `buildDate`. Archives
      ship tarball binaries plus `LICENSE`, `README`,
      `config.example.yaml`, `.env.example`, the `data/world`
      tree, and the systemd unit. SHA-256 checksums.
      `.github/workflows/release.yml` triggers on `v*` tags:
      `goreleaser release --clean` plus a buildx job pushing a
      multi-arch image to `ghcr.io/${owner}/wheelmud` tagged via
      `docker/metadata-action`. Phase J slice J7 (#59), commit
      `d86665c` (2026-05-12).
- [x] Systemd unit / deploy doc — `deploy/systemd/wheelmud.service`
      ships a hardened unit (`Type=simple`, `User=wheelmud`,
      `Restart=on-failure`, `NoNewPrivileges`,
      `ProtectSystem=strict`, `ReadWritePaths` whitelist for
      `/var/lib/wheelmud` + `/var/backups/wheelmud`, journald via
      `SyslogIdentifier=wheelmud`). `deploy/README.md` walks
      Docker + bare-metal installs, the env-override table, the
      observability surface (metrics / healthz / pprof / backups /
      audit), and the upgrade-with-backup ritual. Phase J slice
      J7 (#59), commit `d86665c` (2026-05-12).
- [x] Healthcheck endpoint — `/healthz` on the metrics listener
      (J5). 200 once `SetReady(true)` fires (after telnet listener
      binds) AND a 500ms DB ping succeeds; 503 otherwise.
      `SetReady(false)` flips at shutdown drain start so a
      probe-driven load balancer drains cleanly. Docker
      HEALTHCHECK + compose healthcheck both probe this endpoint
      via wget. Phase J slice J5 (#54), commit `195d352`
      (2026-05-12). Deferred: telnet-level liveness probe.

---

## Active follow-ups

Items already tracked elsewhere — included here so the roadmap points at them:

- **Command-input deferred work** (quoted args, argument completion) —
  `command_input_followups.md`.
- **Open code-review items** (ambiguous-error wording, empty-candidate
  guard, server ctx parent, timing side channel, AuthLevel encapsulation,
  cctx naming, cancellation test coverage) — `code_review_open_items.md`.
- **Terminal & rendering deferred work** (pager mode, prompt templating,
  wide-glyph wrap, long-token break, `SGR` perf, color-enum naming) —
  `terminal_rendering_followups.md`.
- **Persistence layer follow-ups** (down-migrations, brittle unique-violation
  match, DSN leak, CHECK constraints, memory repo O(n), clock seam, sqlite
  `:memory:` pool trap, NFKC normalization, `UsernameLower` /
  `NameLower` encapsulation, FK pragma reliance) —
  `persistence_followups.md`.
- **Auth (password hashing) follow-ups** (bcrypt cost tuning, lockout-
  before-Verify ordering — `SetCost` mutability now resolved) —
  `auth_followups.md`.
- **Login / account-create follow-ups** (control-char scrub on welcome
  echo, lockout enumeration leak, hash-in-memory hygiene, per-connection
  rate limits, account-create soft DoS, `CharacterSelect.chars` snapshot
  staleness, double Conn.Close) — `login_followups.md`.
- **§9 Zones slice follow-ups** (silent ambient marshal swallow,
  `Name`/`Builder` UTF-8 vs ASCII enforcement, `CountByZone` shared-
  suite gap, loader/schema default duplication) —
  `zones_followups.md`.
- **Off-world realms — design pending** (`docs/wot_geography_mud.md`):
  - **The Ways** (Waygate-linked off-world travel realm; reserved zone
    block 30000–30999). Mechanics: Avendesora-leaf seals, Machin Shin
    encounter, Island/Bridge/Guiding structure, time-slip on exit.
  - **Portal Stones / Mirror Worlds** (reserved zone block 31000–31999).
    Activation, instanced parallel-reality zones, alt-history one-shot
    spaces. **No schema or commands designed yet** — revisit once §10
    travel and §15 quest scripting are further along.
- **Sector enum extension — migration 0025 landed**: `blight`, `waste`,
  `stedding`, `swamp` added to `repo.Sector`, `validate.go::validSectors`,
  the rooms.sector CHECK (via writable_schema REPLACE on sqlite_master),
  `phase_ambient.go::sectorAmbients`, and `data/world/README.md`. Mechanical
  hooks still pending: channeling suppression in stedding (§12), ambient
  horror / DoT in blight (§11), movement-cost / encounter flavor in swamp
  and waste (§10–11).
- **Auto-derived room coordinates — landed.** Migration 0026 added
  `rooms.coords_auto` (1 = BFS-derived, 0 = anchor authored in YAML);
  loader stamps it in lock-step with `roomInsertValues`. BFS walks
  cardinal + diagonal + vertical exits from anchors with first-
  arrival wins; conflicts and orphans surface via `coords issues`
  rather than being auto-resolved. Admin verbs `coords rebuild` /
  `coords show <id|external_id>` / `coords issues` are wired.
  Remaining: incremental re-walk on OLC mutation (block on §16
  room/exit edit verbs).
- **Seed test fixtures for container + light flows — pending.** The
  starter zone (Two Rivers / Emond's Field) currently exercises
  `look` / `get` / `drop` / `give` reasonably well, but two newer
  systems have no in-world fixtures to drive an interactive test:
  - **Container `put` / `get-from`** (landed in 56dc597). Existing
    `type: container` rows in `data/world/.../emonds_field/**/items.yaml`
    (tankard, pitcher, dipper) carry no `stats:` block, so capacity
    is zero and nothing can actually be stowed. Need at least one
    seeded container with non-zero `capacity_lbs` / `capacity_cuft`,
    plus 2–3 small seed items (food / trash / trade good) authored
    in the same room so an operator can `put bread chest`,
    `look in chest`, `get bread from chest` end-to-end.
  - **Dark rooms + carried light source.** `ItemTypeLight` +
    `LightStats{RadiusFt, FuelTicks}` exist in code, and rooms have
    `LightLevel` (migration 0012), but no seeded item declares
    `type: light` and no starter room is dark enough to require
    one. Need a `type: light` torch (or lantern) in a starter room
    plus a low-light room (cellar / barn loft / mill basement) so
    the lighting pipeline has something to gate on once it lands.
  - **Where:** likely the Winespring Inn common room (cheap to add
    a chest with a few stowables) and a new dark sub-room under
    the inn or smithy. Touches `data/world/...` only — no schema
    or loader changes needed; container capacity is already a
    `stats:` field the loader accepts.
