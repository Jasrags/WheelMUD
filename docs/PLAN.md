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

7. ~~**Equipment slots + `wear` / `wield` / `remove`** (§9 / §14).
   `character_equipment` table keyed `(character_id, slot)`. Slot enum
   from the WoT body map. Validates against item type/flags.
   Encumbrance already reads transitive ownership — extend to include
   equipped.~~ Landed 2026-05-04. No migration needed —
   `equipment_json` and the `creature.Equipment` slot struct already
   round-trip via migration 0009. Verbs:
   `wear <item>` (Armor / Shield / Clothing→Outfit, slot derived from
   `ItemType`), `wield <item> [off]` (PrimaryWield default, OffHand
   on `off`/`offhand`/`second`), `remove <item>` (overlay clear; item
   stays owned), and `equipment` / `eq` listing. `inventory` annotates
   equipped items as `(worn)` / `(wielded)` / `(offhand)`. `drop` /
   `give` / `put` auto-clear the slot via `autoUnequipIfHeld` so
   leaving inventory can never strand a dangling slot pointer.
   Encumbrance unchanged (equipped items keep `owner_character_id` so
   `ListAllOwnedTransitive` already counts them). V1 follow-ups:
   two-handed weapons (no `FlagTwoHanded` yet), Cloak / Backpack /
   WornMisc / BeltPouches slot disambiguation, wear requirements
   (Str / class / level), auto-swap on full slot.
8. ~~**Shops** (§14). Shopkeeper mob subtype, `list` / `buy` / `sell` /
   `value`.~~ **LANDED 2026-05-05.** Migration 0030 added `shops` +
   `shop_stock`. `internal/repo/shop.go` + memory/sqlite backends.
   `internal/cmd/shop.go` ships `list` / `buy` / `sell` / `value`
   verbs (Auth=Player). World loader parses an optional `shop:` block
   on mob YAML entries — see `data/world/README.md`. Restocker
   subscribes to `tick.Buckets.AreaReset` (5min default) and refills
   any sub-max stock line older than `restock_interval_s`. Half-price
   rule honored via the existing `FlagTradeGood`. V1 follow-ups: shop
   ledger / stats, weight cap on shopkeeper, skill-based haggling,
   sold-back items go into shop stock instead of being deleted,
   shopkeeper movement (V1 keepers must be sentinels).
9. ~~**Banks / vaults** (§14). Banker NPC subtype, `balance` / `deposit`
   / `withdraw`. Reuses `currency.Amount` — no new money model.~~ **Done
   2026-05-05** — migration 0031, optional `banker:` YAML block on a
   mob, `internal/cmd/banker.go` verbs, audit on success. Inter-player
   `transfer` and item vaults deferred (see `banker_followups.md`).

After B: characters can outfit themselves and circulate currency.

---

## Phase C — Character creation flow

The current `internal/mode/character_create.go` is a single-screen
name prompt — every character lands as a class-less, abilityless,
backgroundless husk. Phase D (combat) reads `Core.Defense`, `BAB`,
`Saves`, and `Abilities`; without chargen, every roll defaults to
zeroes and combat is meaningless. The schema is already there:
`creature.Race` / `Background` / `Class` / `Abilities` / Heroic
Characteristics fields all exist on `Character` and round-trip
through migration 0009. What is missing is the **multi-step chargen
mode plus the WoT content catalogs** that drive each step. References
live in `docs/reference/{abilities,backgrounds,classes,heroic-
characteristics,feats,equipment,the-one-power}.md`.

10. **Chargen content loader** (§6 / §12) — **LANDED**. New
    `internal/chargen` package + embedded `internal/chargen/default/
    *.yaml` catalogs (backgrounds, classes, feats, skills, weaves)
    with `CHARGEN_DIR` env override mirroring `WORLD_DIR`. Loader
    validates cross-references at boot (background → feats/skills,
    class → skills, weave → power) and stamps `creature.Background`
    / `creature.Class` enum values onto each entry so the chargen
    mode can persist selections through the existing Character
    schema. No DB tables: catalogs are content, not state. Wiring:
    `cmd/server/main.go` constructs the catalog alongside news /
    help and threads it via the `server` struct. Ogier seed entry
    deferred — slots in cleanly when #14 lands.
11. **Multi-step `CharacterCreate` mode** (§5 / §6) — **SCAFFOLD
    LANDED**. `CharacterCreate.SetCatalog` now drives a substep
    state machine: name → race → background → class → review/
    confirm, with `back` to revisit and `cancel` to restart. Race
    selection filters the background and class menus
    (`Catalog.BackgroundsForRace` / `ClassesForRace`); the review
    step persists `Race`, `Background`, and `ClassLevels{chosen:1}`
    on the `Character` aggregate before promoting to game.
    Catalog is threaded `Login → Create → postAuth →
    CharacterCreate / CharacterSelect` via setters mirroring the
    existing `SetMOTD` pattern, and absent catalog (every existing
    test, dev fixtures with no `data/chargen` on disk) falls back
    to the legacy single-name flow. **Stubbed for follow-ups:**
    abilities (#12), heroic characteristics / identity (#14),
    feats / skills / starting equipment / channeler branch
    (#15) — each slots into the `chargenStep` enum + `chargenDraft`
    struct without restructuring this scaffold.
12. **Ability score generation** (§ref `abilities.md`). Point-buy
    V1 (cost table from book; default 25 points), then the player
    assigns the six rolled scores to Str/Dex/Con/Int/Wis/Cha.
    Reroll rule (sum of mods ≤ 0 OR highest score ≤ 13) lands
    later; standard array + 4d6-drop-lowest as opt-in alternates.
    Writes `Core.Abilities` directly.
13. **Background + class selection** (§ref `backgrounds.md` /
    `classes.md`). Eleven backgrounds (Aiel ... Taraboner) supply
    bonus feats, class-/cross-class skills, home + bonus languages,
    height/weight modifiers, and one of three equipment-option
    bundles. Seven classes (Algai'd'siswai, Armsman, Initiate,
    Noble, Wanderer, Wilder, Woodsman) drive BAB, save progression,
    HD, class-skill list, level-1 features. Race (Human / Ogier)
    gates which classes/backgrounds are available. Writes Race,
    Background, ClassLevels = {chosen: 1}.
14. **Heroic characteristics + identity** (§ref `heroic-
    characteristics.md`). Gender, age, height/weight (Table 6-1
    random rolls modified by background), handedness, alignment
    posture (Good default; Bad / Evil flagged so they can be hidden
    from the player-facing menu later), and the existing name
    prompt. Writes HeightCm, WeightKg, Age, Handedness, Alignment,
    Name.
15. **First-level feats, skills, weaves, starting equipment**
    (§ref `feats.md` / `classes.md` Table 3-1 / `equipment.md` /
    `the-one-power.md`). Drive Table 3-1 to allocate the 1st-level
    feat slot and the (4 + Int mod) × 4 class-skill ranks; merge in
    background bonus feats / skills. For Initiate / Wilder, branch
    into channeler chargen: Source by gender, affinities (one or
    two of the five Powers), and starting weaves from the level-0
    list, writing `Channeling` on the character. Spawn the chosen
    background's equipment-option bundle into the new character's
    inventory and auto-equip the free outfit (Outfit slot already
    handled by Phase B). Persists via the existing
    `CharacterRepo.Create` — chargen builds the full `Character`
    aggregate before the single insert.

After C: a freshly-created character is mechanically *complete* —
abilities, class, race, background, feats, skills, gear, and (for
channelers) an opening weave list. Phase D combat math now reads
real numbers instead of zeroes. Level-up / mid-game skill rank
investment / new-weave learning intentionally **stay in old Phase D**
(now E); this phase is day-zero only.

---

## Phase D — Combat MVP

16. **Initiative + round tick** (§11). Wire a `combat` tick bucket;
    per-room `Fight` state.
17. **Damage types & resistances** (§11). DR / resists already exist on
    `creature.Core`; just plumb the math.
18. **Hit/miss/dodge/parry rolls** (§11). `d20 + bab + ability` vs
    Defense. Reads `WeaponStats.ThreatLow` / `CritMult`.
19. **Death / corpses / looting / XP grant** (§11). At HP ≤ 0 drop a
    corpse item carrying the inventory list, schedule decay tick,
    award XP to attackers.
20. **Aggro / threat tables** (§11). Per-`Fight`
    `threat[CreatureID]int`.
21. **PvE / PvP zones + safe zones** (§11). Reuse existing
    `room.flags.peaceful`; add `pvp` flag on character.
22. **Group / party** (§11). `group` invites, `follow`, shared XP
    split, peaceful-on-group-leader.

After D: full "kill a thing, get XP, find loot, repeat" loop.

---

## Phase E — Progression & affects

Doable in parallel with late D; affects are shared by both. Phase C
shipped the *day-zero* class / feat / skill / weave selection; this
phase ships the *over-time* level-up paths that build on the same
catalogs.

23. **Levels & XP curve** (§12). Reads the §12 class / level table;
    awards feat slots, skill points, ability bumps, weave slots on
    train.
24. **Mid-game skill rank investment** (§12). Per-character
    `character_skills` writes; respects class-skill / cross-class
    caps from the chargen catalog.
25. **Affects / buffs / debuffs with durations** (§12).
    `creature_affects` table. Combat reads it for poison/bleed; weaves
    and consumables write it.
26. **Cooldowns + global lag** (§12 / §4). Per-skill `cooldown_until`;
    integrates with the §4 cooldown infrastructure.
27. **Channeling slot refresh + madness tick** (§9). Schema half-
    landed (creature/channeling tables); chargen seeded affinities +
    starting weaves; this finishes the per-tick mechanics (slot
    refresh on rest, madness accrual while embraced for men, stilled
    state).
28. **Mid-game weave learning** (§12). New weaves added to
    `WeavesKnown` via trainer NPC + practice-points spend. Catalog
    already loaded by Phase C #10.

After E: meaningful vertical progression on top of a chargen-
complete character.

---

## Phase F — NPC behavior & quests

Content multiplier. Without this the world is static.

29. **Trigger / event system** (§15) — `on_enter`, `on_say`,
    `on_attack`, `on_death`, `on_tick`. Pure dispatch layer; consumers
    in 30–31.
30. **NPC dialogue trees** (§15). JSON per mob; uses §13 `say` capture.
31. **Quest engine state machine** (§15). Per-character per-quest
    state + objective ticks.
32. **Embedded scripting (gopher-lua) + sandbox** (§15). Biggest lift
    in the whole roadmap. Defer until 29–31 prove the trigger surface
    is what you actually want — otherwise the Lua API gets redesigned
    twice.

---

## Phase G — OLC

Once content matters, builders need to author it without YAML edits +
restart.

33. **Permission/builder role formalization** (§16). `AuthLevel`
    already splits builder from admin; add per-zone builder grants.
34. **`redit` / `oedit` / `medit` / `zedit`** (§16). Mode-based
    editors using the existing mode stack.
35. **Versioned area saves + diff/preview** (§16). Snapshot before
    commit; admin `revert` rolls back.
36. **Hot-reload of areas without restart** (§7). The §7
    `reload world` admin command, gated on the new versioning.
    **Also unblocks the auto-coords incremental re-walk** parked in
    the roadmap on "blocked on §16."

---

## Phase H — Communication breadth & UX polish

Lower urgency; do whichever lands free time.

37. **Ignore / mute** (§13).
38. **Mail editor mode + `mail`** (§5 / §13). Mode stack supports
    multi-line input with `.` to end.
39. **Bulletin boards / notes** (§13).
40. **Width-aware wrap (CJK / combining marks)** (§2).
41. **Long-token break in `WrapText`** (§2).
42. **Lockout-on-failed-logins finish** (§6) — partial today.
43. **Email verification / password reset** (§6).

---

## Phase I — Network protocol breadth

À la carte; pick what your client population wants.

44. **CHARSET / UTF-8 negotiation** (§1). Cheapest; unblocks accented
    names.
45. **MSSP** (§1). Tiny, but gets WheelMUD on MUD-listing sites.
46. **GMCP** (§1). Highest-value modern protocol; opens
    MUSHclient/Mudlet integrations, in-client UI panels.
47. **MCCP2/3** (§1). Compression; nice-to-have unless bandwidth is a
    real concern.
48. **MXP** (§1). Clickable links; useful once help/news is rich.
49. **TLS listener** (§1).
50. **WebSocket gateway** (§1). Browser clients — high payoff if you
    want public reach.
51. **SSH listener for ops** (§1). Lowest priority unless doing a lot
    of remote admin.

---

## Phase J — Ops, CI, packaging (run in parallel from Phase B onward)

Never gate gameplay but the cost compounds if you wait.

52. **GitHub Actions CI matrix** (§21) + **coverage target** (§21).
    Do this **first** — every later phase produces more code to
    break.
53. **Config file (TOML/YAML) + per-env overrides + `.env.example`**
    (§20).
54. **Metrics + pprof on private `:9090`** (§19).
55. **Request/command audit log per character** (§19).
56. **Backup rotation (`VACUUM INTO`)** (§7).
57. **Telnet integration test driving the protocol** (§21).
58. **Fuzz tests on IAC parser + tokenizer** (§21).
59. **`goreleaser` + systemd unit + healthcheck** (§22).

---

## Sequencing rules

- **Run #52 (CI matrix) right now**, before anything else. It doesn't
  gate gameplay, but it stops regressions in everything below it.
- **Don't start Phase D (combat) without Phase C (chargen) finished.**
  Combat math reads `Core.Defense` / `BAB` / `Saves` / `Abilities`,
  all of which are class- and ability-driven; without chargen those
  default to zero and nothing combats meaningfully.
- **Don't start Phase D without Phase B finished either.** Combat
  against unequipped fists and naked mobs is throwaway content.
- **Phase C feeds Phase E.** The chargen catalog (#10) is the same
  data Phase E's level-up / weave-learning paths read; building both
  on the same loader keeps the level-1 and level-N rules from
  drifting.
- **Don't start Phase E's weaves before Phase D's hit/miss.** Weaves
  inherit the d20 pipeline.
- **Phase F #32 (Lua) is the single biggest lift.** Defer it until
  29–31 prove the trigger surface is what you actually want.
- **Phase G #36 is the natural unblock of the parked auto-coords
  incremental rewalk** — it should be the *last* item in G.
- **Phase I is à la carte.** GMCP first if you want third-party
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
