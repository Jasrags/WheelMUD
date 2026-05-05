# WheelMUD — Sequenced Plan of Attack

> Companion to [`ROADMAP.md`](../ROADMAP.md). Roadmap is the *what* and
> the *status*; this file is the *order* and the *why*. When the two
> disagree about whether something is done, ROADMAP wins; when they
> disagree about what to do **next**, this file wins. Re-derive this
> file from the roadmap whenever a phase finishes or scope shifts.

The throughline: **first make the existing surface comfortable to test,
then close the equipment/economy gap that combat needs, then ship
combat, then progression, then content tooling.** Each phase is sized
so something *playable* lands at the end of it, not just plumbing.

---

## Phase A — Quick wins on what already exists

Cheap, low-risk, makes every later phase faster to test.

1. **Help topics + prefix matching** (§18). `help <topic>` exact →
   keyword → unique-prefix → ambiguity list. Reuses the same matcher
   pattern as the registry.
2. **Pager mode** (§2 / §5). Push a pager mode when output exceeds
   `Session.Height`. Immediately benefits `who`, `news`, `help`,
   future `score` / `equipment`.
3. ~~**`goto` / `transfer` / `summon` / `wizinvis`** (§17). Thin
   wrappers over `teleport` + the session registry. No schema
   changes.~~ Landed 2026-05-04. `goto <player|room>`, `transfer
   <player> [<room>]`, `summon <player>`, and `wizinvis` toggle —
   all gated AuthAdmin. Wizinvis is session-scoped (no schema) and
   filters from `who` + tell-name completion / lookup.
4. ~~**`shutdown` / `reboot`** (§17). Drain inputs, flush
   `persist.Manager`, close the listener. Removes the
   "kill -TERM and pray" workflow.~~ Landed 2026-05-04. Both verbs
   accept `[<delay>] [<reason>]` (default 30s, clamped to ≤1h) or
   `cancel`/`abort` to interrupt an in-flight countdown. Countdown
   broadcasts at T-{60,30,10,5..0}s. `reboot` re-execs via
   `syscall.Exec` after the existing drain + persist.FlushAll.
   AuthAdmin only.
5. ~~**Admin audit log** (§17). `admin_audit` table + a single
   `Audit(ctx, actor, verb, target)` helper. Wires retroactively into
   `spawn`, `teleport`, future `goto` / `shutdown`. Closes the spawn-
   audit gap noted in `spawn_followups.md`.~~ Landed 2026-05-04.
   Migration 0029 + `repo.AdminAuditRepo` (memory + sqlite) +
   `internal/audit.Record` helper. Wired into `spawn`, `teleport`,
   `goto`, `transfer`, `summon`, `wizinvis`, `shutdown`, `reboot`
   (and `shutdown:cancel`). Synchronous write so `shutdown` flushes
   the row before drain begins. Read-side viewer verb deferred.
6. ~~**Macro / multi-command lines (`;` separator)** (§4). Tokenizer-side
   split with bounded recursion depth so `;` inside `say` text doesn't
   recurse.~~ Landed 2026-05-04. `telnet.SplitOnSemicolon` mirrors
   `Tokenize`'s quote/escape rules; `Registry.Dispatch` splits the
   input line into segments and runs each through `dispatchOne`
   (alias-expand → lookup → auth → tokenize → run). Unbalanced quotes
   surface the existing "Unbalanced quote" response. Cap of 16
   segments per line with a "(too many commands; truncated)" notice;
   alias-expansion-introduces-`;` is bounded at depth 3. First Run
   error is returned but later segments still run.

After A: meaningfully nicer to live in for ops and builders.

---

## Phase B — Equipment & economy foundation

Combat needs armor on bodies and weapons in hands. Shops/banks need
item flow. Both build on the inventory/container work just landed.

7. **Equipment slots + `wear` / `wield` / `remove`** (§9 / §14).
   `character_equipment` table keyed `(character_id, slot)`. Slot enum
   from the WoT body map. Validates against item type/flags.
   Encumbrance already reads transitive ownership — extend to include
   equipped.
8. **Shops** (§14). Shopkeeper mob subtype, `list` / `buy` / `sell` /
   `value`. Half-price sell rule (the existing `trade_good` flag is
   already exempt in code).
9. **Banks / vaults** (§14). Banker NPC subtype, `balance` / `deposit`
   / `withdraw`. Reuses `currency.Amount` — no new money model.

After B: characters can outfit themselves and circulate currency.

---

## Phase C — Combat MVP

10. **Initiative + round tick** (§11). Wire a `combat` tick bucket;
    per-room `Fight` state.
11. **Damage types & resistances** (§11). DR / resists already exist on
    `creature.Core`; just plumb the math.
12. **Hit/miss/dodge/parry rolls** (§11). `d20 + bab + ability` vs
    Defense. Reads `WeaponStats.ThreatLow` / `CritMult`.
13. **Death / corpses / looting / XP grant** (§11). At HP ≤ 0 drop a
    corpse item carrying the inventory list, schedule decay tick,
    award XP to attackers.
14. **Aggro / threat tables** (§11). Per-`Fight`
    `threat[CreatureID]int`.
15. **PvE / PvP zones + safe zones** (§11). Reuse existing
    `room.flags.peaceful`; add `pvp` flag on character.
16. **Group / party** (§11). `group` invites, `follow`, shared XP
    split, peaceful-on-group-leader.

After C: full "kill a thing, get XP, find loot, repeat" loop.

---

## Phase D — Progression & affects

Doable in parallel with late C; affects are shared by both.

17. **Class / archetype model** (§12). Drives chargen pick + level-
    table key.
18. **Levels & XP curve** (§12).
19. **Skill tree + ranks** (§12). `character_skills` table; ability
    checks read it.
20. **Affects / buffs / debuffs with durations** (§12).
    `creature_affects` table. Combat reads it for poison/bleed; weaves
    and consumables write it.
21. **Cooldowns + global lag** (§12 / §4). Per-skill `cooldown_until`;
    integrates with the §4 cooldown infrastructure.
22. **Channeling sub-record** (§9). Schema half-landed
    (creature/channeling tables); finish wiring.
23. **Weave list with slot levels** (§12). Replaces placeholder weaves
    with the real WoT spell list.

After D: meaningful vertical progression.

---

## Phase E — NPC behavior & quests

Content multiplier. Without this the world is static.

24. **Trigger / event system** (§15) — `on_enter`, `on_say`,
    `on_attack`, `on_death`, `on_tick`. Pure dispatch layer; consumers
    in 25–26.
25. **NPC dialogue trees** (§15). JSON per mob; uses §13 `say` capture.
26. **Quest engine state machine** (§15). Per-character per-quest
    state + objective ticks.
27. **Embedded scripting (gopher-lua) + sandbox** (§15). Biggest lift
    in the whole roadmap. Defer until 24–26 prove the trigger surface
    is what you actually want — otherwise the Lua API gets redesigned
    twice.

---

## Phase F — OLC

Once content matters, builders need to author it without YAML edits +
restart.

28. **Permission/builder role formalization** (§16). `AuthLevel`
    already splits builder from admin; add per-zone builder grants.
29. **`redit` / `oedit` / `medit` / `zedit`** (§16). Mode-based
    editors using the existing mode stack.
30. **Versioned area saves + diff/preview** (§16). Snapshot before
    commit; admin `revert` rolls back.
31. **Hot-reload of areas without restart** (§7). The §7
    `reload world` admin command, gated on the new versioning.
    **Also unblocks the auto-coords incremental re-walk** parked in
    the roadmap on "blocked on §16."

---

## Phase G — Communication breadth & UX polish

Lower urgency; do whichever lands free time.

32. **Ignore / mute** (§13).
33. **Mail editor mode + `mail`** (§5 / §13). Mode stack supports
    multi-line input with `.` to end.
34. **Bulletin boards / notes** (§13).
35. **Width-aware wrap (CJK / combining marks)** (§2).
36. **Long-token break in `WrapText`** (§2).
37. **Lockout-on-failed-logins finish** (§6) — partial today.
38. **Email verification / password reset** (§6).

---

## Phase H — Network protocol breadth

À la carte; pick what your client population wants.

39. **CHARSET / UTF-8 negotiation** (§1). Cheapest; unblocks accented
    names.
40. **MSSP** (§1). Tiny, but gets WheelMUD on MUD-listing sites.
41. **GMCP** (§1). Highest-value modern protocol; opens
    MUSHclient/Mudlet integrations, in-client UI panels.
42. **MCCP2/3** (§1). Compression; nice-to-have unless bandwidth is a
    real concern.
43. **MXP** (§1). Clickable links; useful once help/news is rich.
44. **TLS listener** (§1).
45. **WebSocket gateway** (§1). Browser clients — high payoff if you
    want public reach.
46. **SSH listener for ops** (§1). Lowest priority unless doing a lot
    of remote admin.

---

## Phase I — Ops, CI, packaging (run in parallel from Phase B onward)

Never gate gameplay but the cost compounds if you wait.

47. **GitHub Actions CI matrix** (§21) + **coverage target** (§21).
    Do this **first** — every later phase produces more code to
    break.
48. **Config file (TOML/YAML) + per-env overrides + `.env.example`**
    (§20).
49. **Metrics + pprof on private `:9090`** (§19).
50. **Request/command audit log per character** (§19).
51. **Backup rotation (`VACUUM INTO`)** (§7).
52. **Telnet integration test driving the protocol** (§21).
53. **Fuzz tests on IAC parser + tokenizer** (§21).
54. **`goreleaser` + systemd unit + healthcheck** (§22).

---

## Sequencing rules

- **Run #47 (CI matrix) right now**, before anything else. It doesn't
  gate gameplay, but it stops regressions in everything below it.
- **Don't start Phase C without Phase B finished.** Combat against
  unequipped fists and naked mobs is throwaway content.
- **Don't start Phase D's weaves before Phase C's hit/miss.** Weaves
  inherit the d20 pipeline.
- **Phase E #27 (Lua) is the single biggest lift.** Defer it until
  24–26 prove the trigger surface is what you actually want.
- **Phase F #31 is the natural unblock of the parked auto-coords
  incremental rewalk** — it should be the *last* item in F.
- **Phase H is à la carte.** GMCP first if you want third-party
  clients; WebSocket first if you want browser reach.

---

## Maintenance

- When a phase item ships: mark it on `ROADMAP.md`, then strike it
  through (`~~item~~`) here. Don't delete — the rationale stays
  useful for retros.
- When a phase finishes: append a one-line "Closed YYYY-MM-DD,
  shipped in commits abc..def" footer.
- When scope shifts (new system added, an item gets unblocked
  earlier): re-derive the affected phase from `ROADMAP.md` rather
  than patching this file in place. The whole point is that the
  ordering is *coherent* across phases, not that any one item is
  immutable.
