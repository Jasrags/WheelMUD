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
2. ~~**Pager mode** (§2 / §5). Push a pager mode when output exceeds
   `Session.Height`.~~ Landed 2026-05-11. `telnet/pager.go` +
   `Session.WritePaged` / `WritePagedWrapped`; wired into `help`,
   `news`, `who`, `examine`, `inventory`, `quest`, `zonemap`.
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

7. ~~**QA test zone** (tooling). A dedicated `data/world/test/`
   zone wired up only for exercising shipped systems: a few
   rooms, a shopkeeper + banker NPC, a trainer, a stationary
   combat dummy, a wandering combat dummy, sample items spanning
   every taxonomy, a quest-giver mob with a
   `talk_to`+`kill_n`+`reach_room` quest, a Lua-trigger room, a
   dark cellar paired with a torch.~~ **Landed 2026-05-10.**
   Zone `test.qa` lives at `data/world/test/qa_zone/` (4 YAML
   files: zone / rooms / items / mobs) plus the smoke quest at
   `internal/quest/default/test_qa.yaml`. 10 rooms, all
   `nomap: true`, `level_range: 95-99` so players can't
   stumble in. Hub is `test.qa.hub`; admins reach it via
   `teleport test.qa.hub`. Reset cadence
   `reset_interval_s: 60` + `reset_mode: always` so destructive
   tests recover quickly. Mobs default to HP=1 / Defense=10 /
   unarmed via `insertMobs`, so combat dummies die in one hit
   and respawn within 60s — no new schema needed. Lua trigger
   reuses the existing `bless_actor.lua` script. Zero Go code
   changed; pure content. **Convention going forward:** every
   feature added from Phase B onward should land a one-room
   repro fixture in this zone in the same PR.

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
12. **Ability score generation** (§ref `abilities.md`) — **LANDED
    2026-05-05**. Point-buy V1 in chargen as a new
    `chargenStepAbilities` substep between class and review.
    Standard d20 cost table (8=0, 9=1, 10=2, 11=3, 12=4, 13=5,
    14=6, 15=8, 16=10, 17=13, 18=16) with a 25-point budget;
    score range bounded to [8..18]. Verbs at the substep:
    `set <abil> <n>` (also accepts shorthand `<abil> <n>`),
    `reset` (all back to 8), `done` to advance, `show`/blank to
    re-render. Over-budget assignments are refused without
    overwriting prior state. Stamps `Core.Abilities` (Current=Max=
    score, Inherent=0) on the `CharacterRepo.Create` call. `back`
    from review preserves the assigned scores. **Stubbed for
    follow-ups:** standard array + 4d6-drop-lowest opt-in
    alternates, reroll rule (sum of mods ≤ 0 OR highest ≤ 13),
    racial / heroic-characteristic modifiers (#14 layers them on
    after this step writes the base scores).
13. ~~**Background + class selection** (§ref `backgrounds.md` /
    `classes.md`).~~ **LANDED 2026-05-05.** Substep persistence
    landed in #11; this item ships the player-facing detail surface
    so picks are informed rather than blind. Background and class
    menus render compact one-line summaries (background:
    home_language + feat/skill/outfit counts; class: HD + BAB +
    channeler tag) plus a footer "Type 'info <id|#>' for full
    details." `info <id|#>` at either step opens a descriptor block
    showing languages, bonus feats, background skills, height mod,
    restrictions, and equipment-option bundles for backgrounds; HD,
    BAB, saves, skill points/level, key abilities, channeler source,
    and class-skill list for classes. `info` is a read-only verb —
    it never commits a selection. Race (Human / Ogier) gating goes
    through `Catalog.BackgroundsForRace` / `ClassesForRace` (the
    latter filters out channelers for ogier picks per book lore).
    Eleven human backgrounds and seven classes ship; the Ogier
    background-equivalent slots in cleanly when #14 lands. Writes
    Race, Background, ClassLevels = {chosen: 1} via the existing
    review-step path. **Stubbed for follow-ups:** application of
    bonus_feats / background_skills / equipment_options to the
    draft (those land with #15 once feats/skills/equipment have
    runtime effect); height/weight modifier surfaces visually here
    but is consumed by #14 when identity lands.
14. ~~**Heroic characteristics + identity** (§ref `heroic-
    characteristics.md`).~~ **LANDED 2026-05-05.** New
    `chargenStepIdentity` substep between abilities and review,
    with `gender <m|f>`, `age <n>`, `handed <r|l|a>`,
    `align <good|bad|evil>`, `roll`, and `done` verbs. Defaults
    are stamped on first entry — gender male, alignment good,
    handedness right; age 20 (human) / 95 (ogier); height &
    weight rolled via Table 6-1 against the chosen race +
    `Background.HeightModIn` (female base sizes are slightly
    smaller per the rulebook table). `gender <m|f>` re-rolls
    height/weight so the line stays consistent with the gender
    base; `roll` re-rolls without changing other fields. Random
    source is `*rand.Rand` on the mode (`SetRNG` for tests; time-
    seeded by default). Stamps `Core.Gender`, `Core.Alignment`,
    and the top-level `HeightCm` / `WeightKg` / `Age` /
    `Handedness` columns through the existing
    `CharacterRepo.Create` path. Bad / Evil postures are flagged
    via the existing `creature.Posture` enum but not yet hidden
    from the menu (deferred until the alignment-driven content
    lands). Name continues to come from `chargenStepName` up
    front. **Stubbed for follow-ups:** ogier-race backgrounds
    (catalog-side, blocked on #14's content surface),
    background-driven default-age hint, height/weight readouts
    using the in-world span/league/stone display layer.
15. **First-level feats, skills, weaves, starting equipment**
    (§ref `feats.md` / `classes.md` Table 3-1 / `equipment.md` /
    `the-one-power.md`). **SLICE 1 LANDED 2026-05-05.** New
    `chargenStepFeat` + `chargenStepSkills` substeps slot in
    between identity and review:
    - **Feat:** menu of `catalog.FeatsForBackground(bg)` plus the
      auto-merged `bg.BonusFeats`. Verbs: `pick <id|#>`, bare
      `<id|#>`, `info <id|#>`, `done`. `done` requires a pick
      when the background offers options; bg with no restricted
      feats accepts `done` immediately so the player still gets
      the auto-merged bonus set.
    - **Skills:** budget = `max(1, class.SkillPoints + IntMod) × 4`
      with the d20 1-point/level floor. Allowed skills are
      class skills ∪ background skills (deduped). Per-skill cap
      4 ranks (level+3 at level 1). Verbs: `rank <id|#> <n>`,
      `reset`, `done`. Over-budget assignments are refused
      without overwriting prior state. Unspent points are
      forfeit (V1 — Phase E level-up will let players bank).
    - String catalog ids are stably hashed to int32 via FNV-32a
      (`catalogIDInt32`) so the existing
      `Character.Feats []int32` / `Character.Skills
      map[int32]SkillRanks` columns round-trip without a new
      enum table. Every persisted skill is flagged
      `IsClassSkill=true` since V1 only allows class+bg picks.
    - Persists via `CharacterRepo.Create` — the existing
      `feats_json` / `skills_json` columns already handle the
      round trip, no migration needed.
    **SLICE 2 LANDED 2026-05-05.** New `chargenStepChanneling`
    substep slots in between skills and review for classes whose
    YAML declares `channeler: true` (Initiate, Wilder); non-
    channeler classes skip silently.
    - **Source:** auto-derived from gender — Saidin if Male,
      Saidar if Female. No prompt today (both eligible classes
      use `channel_source: either`); the substep displays the
      inferred source as a confirmation row.
    - **Affinities:** exactly 2 of 5 (Air/Earth/Fire/Water/
      Spirit). Stored as a `creature.PowerSet` bitmask.
    - **Starting weaves:** exactly 3 from the level-0 catalog
      filtered to weaves whose `Power` matches at least one
      selected affinity. Switching affinities drops the prior
      weave selection because the eligibility filter shifted.
    - Persists via the new `channeling_json` column on
      `characters` (migration 0033). Non-channelers persist
      `'null'`; channelers persist a JSON-encoded
      `*creature.Channeling` carrying GenderSource,
      ChannelerType (Initiate/Wilder), Affinities bitmask, and
      `WeavesKnownIDs []string` (transitional sibling to the
      int32-keyed `WeavesKnown` until §12 authors a numeric
      weave table).
    **SLICE 3 LANDED 2026-05-06.** Starting-equipment bundle
    spawning + auto-equip ships in a new `chargenStepEquipment`
    substep that slots in between channeling and review (or
    skills and review for non-channeler classes). The chargen
    catalog gains a sibling `internal/chargen/default/items.yaml`
    with 50 starting-equipment templates (mirroring the
    `data/world/` item schema: type / weight / value / quality /
    flags / typed Stats); cross-validated at boot so a typo in
    any background's `equipment_options[].items` reference fails
    chargen.Load. At finalize, `applyStartingEquipment` clones
    each picked-bundle item via `ItemRepo.Create` with a unique
    runtime external_id (`<id>#cgen-<charID>-<i>`),
    `RecordInventory`s the id list, and auto-equips the first
    armor / shield / outfit / weapon via `Equipment.Set` +
    `RecordEquipment`. Items thread `Login → Create → postAuth →
    AccountMenu → CharacterCreate` via a new `SetItems` setter.
    No migration — items + equipment_json + inventory_json
    already round-trip.
    **Slice 3 polish — landed 2026-05-10:** per-class starting
    coin. New `Class.StartingCoin` field (validated at boot via
    `currency.Parse`) seeded across all 7 classes
    (`internal/chargen/default/classes.yaml`); finalize calls
    `RecordCoin(ctx, c.ID, parsed, 0, 0)` after
    `applyStartingEquipment`. Closes the day-zero economy gap
    where fresh characters had `Coin = 0` and couldn't reach a
    shop without farming a mob first. Background `coin_pouch`
    items still land as trade-good items on the 2 bundles that
    grant them (deferred follow-up to convert pouch→purse so
    players don't eat the 50% sell hit).
    **Stubbed for follow-ups** (see
    `chargen_features_followups.md`):
    - Two-handed / off-hand / light / quiver auto-equip — slice
      1/2 follow-ups already track these.
    - Cross-class skill picks (half-rate, double cost) — defer
      until level-up needs the same plumbing in §12.
    - Dice-expression starting wealth (e.g. `5d4*10sp`) — V1
      uses fixed averages; defer until a public dice utility
      exists.
    - Background coin pouch → purse conversion — gambler /
      innkeeper bundles still drop the pouch as an item.

After C: a freshly-created character is mechanically *complete* —
abilities, class, race, background, feats, skills, gear, and (for
channelers) an opening weave list. Phase D combat math now reads
real numbers instead of zeroes. Level-up / mid-game skill rank
investment / new-weave learning intentionally **stay in old Phase D**
(now E); this phase is day-zero only.

---

## Phase D — Combat MVP

16. ~~**Initiative + round tick** (§11). Wire a `combat` tick bucket;
    per-room `Fight` state.~~ **LANDED 2026-05-06.** New
    `internal/combat` package: per-room `Fight` aggregate with
    initiative-ordered actor list (d20 + DexMod + InitMod, ties
    broken by raw-d20 then ActorRef.ID), `Manager` keyed by RoomID
    with `Start` / `End` / `Get` / `Tick` / `Stop`, and typed
    `CombatStarted` / `RoundStarted` / `CombatEnded` events on the
    eventbus. Subscribed to `tick.Buckets.Combat` (4s default
    cadence). Auto-ends a fight when every mob participant has
    left the room. No `attack` verb yet — that's #18 — and no
    persistence (server restart drops in-flight fights, acceptable
    for V1).
17. ~~**Damage types & resistances** (§11). DR / resists already exist on
    `creature.Core`; just plumb the math.~~ **LANDED 2026-05-07** as the
    minimal slice bundled with #18: `internal/combat/resolution.go`'s
    `applyDamage` walks `Core.DR` (flat clamp) then `Core.Resists`
    (percent modifier, negative = vuln) and routes subdual damage to
    `Core.Subdual`. The B/P/S → `creature.DamageType` mapper lives
    next to it. Bypass keywords + magic/cold-iron tags + per-resist
    type-tag parsing are deferred follow-ups.
18. ~~**Hit/miss/dodge/parry rolls** (§11). `d20 + bab + ability` vs
    Defense. Reads `WeaponStats.ThreatLow` / `CritMult`.~~ **LANDED
    2026-05-07.** New `internal/combat/resolution.go`
    (`RollAttack` / `RollDamage` / `applyDamage` pure functions),
    per-`Fight` `Actions map[ActorRef]Action` queue with
    `Manager.EnqueueAction` / `PendingAction`, `Tick` resolver that
    pops the active actor's queued action, rolls hit/miss/crit,
    applies damage, and writes HP back via `MobInstanceRepo.UpdateLive`
    / `CharacterRepo.RecordCore`. `CombatHit` / `CombatMiss` /
    `ActionResolved` events. New `attack <target>` verb (alias `kill`)
    that resolves the room mob via `MatchMob`, starts (or reuses) the
    fight, queues `ActionAttack` with the wielded weapon. Refused in
    Peaceful rooms. Re-issuing while a fight is in progress switches
    the queued target without restarting initiative.
    `CharacterRepo.GetByID` added (sqlite + memory + shared test) so
    `Manager.resolveCore` can load player participants.
    Deferred follow-ups: parry check (needs FlatFooted state machine),
    two-weapon / off-hand attacks, iterative attacks at +6/+11/+16,
    ranged / thrown weapons, `flee`, combat prompt repaint.
19. **Death / corpses / looting / XP grant** (§11). Slice 1 LANDED
    2026-05-07: mob HP ≤ 0 spawns a corpse container in the room
    via `ItemRepo.Create`, despawns the mob (`UpdateRoom(0)` +
    `Delete`), and awards XP weighted by damage to character
    attackers from a per-`Fight` `DamageTally map[ActorRef]int32`.
    Per-template XP value comes from a hard-coded
    `xpValueForChallenge(ChallengeCode)` table (A=100 → I=38400).
    `CombatDeath` / `CombatXPAwarded` events. `Fight.Dead` set is
    pruned from `Order` at the top of the next `tickRoom` so
    `ActiveIdx` math observes a stable order during resolution; the
    fight auto-ends when `Order` empties. New
    `CharacterRepo.RecordXP` (sqlite + memory + shared test).
    Looting bundle, durable corpse decay, player-death respawn +
    XP-debt landed earlier (see ROADMAP for dates). **Bind verb
    LANDED 2026-05-14**: migration 0056 adds `rooms.bindable`;
    new player verb `bind` (no args, Auth=Player) records the
    current room as `Character.BoundRoomID` via new
    `CharacterRepo.RecordBoundRoom` (mirrors RecordRoom — no
    optimistic lock). Refuses on non-bindable rooms; short-circuits
    when already bound. `RoomFlags.Bindable` flows through repo /
    YAML / loader / `redit` OLC / `zonemap` reporter
    (loader-lockstep). Existing `combat.handleCharacterDeath`
    respawn path picks up the new BoundRoomID without changes.
    **PvP XP awards LANDED 2026-05-14**: shared `creditXPShares`
    helper extracted from mob's `awardKillXP` (mob path
    byte-equivalent via thin wrapper); `handleCharacterDeath`
    snapshots `Fight.DamageTally` under `m.mu` and, after the
    death/respawn events, strips the victim from the tally and
    runs `creditXPShares` with `pvpXPForKill(attackerLevel,
    victimLevel)`. New `pvp_xp.go`:
    `PvPXPPerVictimLevel = 50`, `PvPLevelDiffCap = 5` — kills
    where attacker outlevels victim by more than the cap return
    0 (anti-farm). Non-combat deaths (empty killer from
    `HandleAffectDeath`) and empty-tally edges short-circuit.
    **Drop-on-death toggle LANDED 2026-05-15** — Phase D §19 closer.
    `CombatConfig{DropOnDeath bool}` (env `DROP_ON_DEATH`, YAML
    `combat.drop_on_death`) threaded via `Manager.SetDropOnDeath`.
    When enabled, `handleCharacterDeath` runs `dropCharacterLoot`
    before respawn: durable player-corpse in the death room
    (`pcorpse-<id>-<nano>`, `corpseDecayDuration`), top-level
    inventory + equipped items moved via `TransferOwnerToContainer`
    (nested items follow their container), carried coin spawns a
    `TradeGood` pile inside the corpse, `Character.Coin` zeroed
    (bank preserved, optimistic-lock retry on `ErrCoinConflict`),
    Equipment cleared. The 10% XP-debt delta is waived when the
    drop fires — gear/coin loss replaces XP debt as the cost.
    `CharacterDied.CorpseID` plumbs the corpse id through the bus
    for future broadcast variants. Affect-death path shares the
    gate. Still deferred: two-UPDATE TOCTOU on RecordXP +
    RecordXPDebt (safe under single-session-per-account); per-zone
    room-flag override is a future hardcore-zone follow-up.
    **§19 is now closed** in ROADMAP.
20. **Aggro / threat tables** (§11). Slice 1 LANDED 2026-05-07:
    `Fight.Threat map[ActorRef]map[ActorRef]int32` (defender →
    attacker → cumulative). Damage adds 1:1 from the same site that
    bumps `DamageTally`. `pruneDead` clears both the dead actor's
    defender row and their attacker column in every other row.
    `Fight.HighestThreat(defender)` returns the largest contributor
    with deterministic tie break. Deferred: NPC retarget (needs mob
    AI), healing-adds-threat, taunt verb, `feign death`.
21. ~~**PvE / PvP zones + safe zones** (§11). Reuse existing
    `room.flags.peaceful`; add `pvp` flag on character.~~ **LANDED
    2026-05-07.** Slice 1 — migration 0037 + `characters.pvp` +
    `pvp` verb + `attack <player>` guard (nopvp room, newbie cap
    10, both-side opt-in). Slice 2 — `who` PvP tag. Slice 3 —
    defender-side reverse broadcast. Slice 4 — player keyword
    ordinals: `MatchPlayer(target, sessions, self)` in
    `internal/cmd/keyword.go` mirrors `MatchItem`/`MatchMob`,
    sorts in-room peers by CharacterID ascending for stable
    ordinals under map-iteration randomness, and reuses
    `parseOrdinal`/`nameMatches`. `attack 2.jas` now picks the
    second matching peer. Deferred follow-ups: tab-completion for
    `attack` player targets, `consider <player>`.
22. ~~**Group / party** (§11). `group` invites, `follow`, shared XP
    split, peaceful-on-group-leader.~~ **CLOSED 2026-05-07.**
    **SLICE 1 LANDED 2026-05-07.** New `internal/group` package:
    `Group` aggregate (Leader + Members map), `Manager` keyed by
    leader CharacterID with reverse `byCharacter` index, in-memory
    state, `MaxGroupSize = 6`, leader-leaves-disbands. Manager
    methods: `Invite`/`Accept`/`Decline`/`Leave`/`Kick`/`Disband`/
    `Of`/`SameGroup`/`MembersInRoom`/`PendingInvite`/
    `ClearForCharacter`. New verb `group <invite|accept|decline|
    leave|kick|disband>` plus bare `group` roster. Logout cleanup
    threads through `handleConnection`'s teardown defer in
    `cmd/server/main.go`.
    **SLICE 2 LANDED 2026-05-07.** Same-group PvP refusal:
    `pvpRefusalReason` gains a `sameGroup bool` parameter and a
    new gate (priority 2, between NoPVP and the newbie cap) that
    refuses with `"X is a comrade — you won't strike them."`.
    `NewAttack` factory threads `*group.Manager`; `cmd/server/
    main.go` passes the live manager. Tests cover the comrade
    line, ungrouped pairs (positive control), and nil-manager
    backwards-compat.
    **SLICE 3 LANDED 2026-05-07.** `follow <player>` + `unfollow`
    verbs plus chain-on-move. New `Session.followingID` (crossMu-
    guarded, `Following()` / `SetFollowing()` helpers). Verb
    refuses unless target is same-party + same-room + non-cycling
    (walks the leader's chain up to maxFollowDepth=16). Move verb
    re-runs `moveDir` for every co-located peer following the
    leader; recursion bounded by `followDepth`; on a per-follower
    failure (locked door, sector gate, missing exit) the
    relationship clears with a "couldn't keep up" notice. Logout
    cleanup of `followingID` is implicit (session goes away).
    **SLICE 4 LANDED 2026-05-07.** Shared XP split. New
    `combat.GroupResolver = func(charID, roomID int64) []int64`
    type and `Manager.SetGroupResolver` setter (mirrors
    `SetDecayer`/`SetFleeMover`). `cmd/server/main.go` wires the
    live `groups.MembersInRoom` resolver into the combat manager.
    `handleMobDeath` now calls `expandTallyByGroup(tallySnap,
    roomID, resolver)` before `allocateXP`, splitting each
    character contributor's damage equally across in-room party
    members (remainder credits to the dealer so totals don't
    drift). Non-character actors (mobs, future NPC allies) pass
    through unchanged. Nil resolver = pre-slice-4 solo behaviour.
    Tests cover solo, 2-member split, remainder, partial in-room,
    non-character passthrough, nil-resolver no-op, and two-dealer
    accumulation.

After D: full "kill a thing, get XP, find loot, repeat" loop.

---

## Phase E — Progression & affects

Doable in parallel with late D; affects are shared by both. Phase C
shipped the *day-zero* class / feat / skill / weave selection; this
phase ships the *over-time* level-up paths that build on the same
catalogs.

23. ~~**Levels & XP curve** (§12). Reads the §12 class / level table;
    awards feat slots, skill points, ability bumps, weave slots on
    train.~~ Closed 2026-05-07 (slices 1–4 landed; see ROADMAP §12).
24. ~~**Mid-game skill rank investment** (§12). Per-character
    `character_skills` writes; respects class-skill / cross-class
    caps from the chargen catalog.~~ **Closed 2026-05-14.** Slice 1
    (class + background, 1pt/rank, cap level+3) landed 2026-05-07
    (see ROADMAP §12). **Cross-class slice LANDED 2026-05-14**:
    `learn` now lists the full catalog. Class/background entries
    keep 1pt/rank + cap level+3 (`IsClassSkill=true`); cross-class
    entries cost 2pt/rank + cap `(level+3)/2` (`IsClassSkill=false`).
    `learn.go` adds `classSkillSet` / `crossClassSkillRankCap` /
    `skillCostAndCap` helpers; `isClassOrBackgroundSkill` picks the
    bucket at commit time and feeds the existing `RecordSkillRank`
    write path unchanged. Menu header shows dual cap
    (`"6 (class) / 3 (cross)"`); each row carries `[class]`/`[bg]`/
    `[cross]` tag with bucket-specific cap. `learn info` shows
    Cost/rank and Rank-cap. Unknown-token refusal text changed
    from "not available to you" → "No such skill." (cross-class is
    now available; only off-catalog tokens refuse). Refusal
    ordering preserved (cap → budget → repo → audit on success).
    No schema, no repo signature change, no new pending pool.
    Deferred: prefix-match disambiguation.
25. ~~**Affects / buffs / debuffs with durations** (§12).~~
    **#25 closed 2026-05-10.** Remaining producer threads
    (combat-on-hit, weave-cast, healer NPC, light fuel, player
    dispel) live in `affects_followups.md` and are blocked on §D
    crit polish or Phase F/G surfaces; they don't gate #25
    closure. **Slice 1 landed 2026-05-09:** the
    `affects` inspect verb (Player) + `affect` / `dispel` admin verbs
    fill the no-callers gap on `affects.Apply` and give builders an
    end-to-end producer to test the existing tick / Effective /
    Expired pipeline. Sentinel Source `-1` flags admin-applied
    affects in the inspect-verb display. **Slice 2 landed 2026-05-10:**
    foundation gaps closed + first player producer. `creature.Affect`
    gained `ConditionMask` (Effective ORs into `Core.Conditions`) and
    `TickDamage` (ticker folds into HPCurrent when `TickEffect != ""`).
    Apply now caps at 4 entries per Name from distinct Sources
    (shortest remaining duration evicted). New `internal/effects/`
    YAML catalog (chargen-style embed+override) seeded with
    `healing_draught`, `weak_poison`, `bull_strength`. New
    `affects.ApplyTickEffects` pure fn drives DoT/HoT in
    `SessionTicker.tickOne`, persisted via `RecordCore`, broadcast
    via the new `affects.TickDamaged` event. DoT-death wires through
    new `combat.Manager.HandleAffectDeath` reusing the §19 pipeline.
    New `quaff <potion>` verb (sentinel Source = -2 → "potion"
    label) consumes the item on apply (V1: ignores Charges).
    Loader: consumable items accept `effect_id_string:` translated
    via chargen.HashID; boot-time `validateConsumableEffectRefs`
    cross-checks against the catalog. Three seed potions in the
    Winespring Inn kitchen. **Slice 3 landed 2026-05-10:**
    foundation polish + multi-charge consumables + effect-message
    plumbing. New `ItemRepo.UpdateStats` write path; `quaff`
    branches on Charges (0=unlimited, 1=delete, >1=decrement); a
    latent slice-2 type-assertion bug (loaded items carry pointer
    ConsumableStats, quaff was asserting value type) was fixed via
    `consumableStatsOf` helper. `creature.Affect.ExpireMessage`
    + `affects.Tick` returns `[]Affect` + `Expired` event reshaped
    to `Entries []ExpiredEntry` so the cmd-layer subscriber
    renders the authored `MessageOnExpire` line. Combat's
    end-of-round Tick publisher updated identically. Seed
    `potion_healing_draught` bumped to `charges: 3`. Slice 4+:
    weave/combat-hit producers, healer NPC service, player
    dispel, light/torch fuel burn-down.

    **Tracked deferred producers** (do not lose):
    - **Healer NPC service** — banker-style mob entry (1:1 to
      `mob_template`) that applies a healing affect or instant
      heal for coin. Pairs naturally with the test-zone setup.
    - **Light / torch fuel burn-down** — needs a per-tick
      `Buckets.Light` ticker decrementing `LightStats.FuelTicks`
      on every lit torch in active rooms. `ItemRepo.UpdateStats`
      (slice 3) is the existing write path.
    - **Player-driven `dispel` weave** — admin verb already
      ships; player-facing dispel waits for the weave-cast
      surface (Phase F mechanics, after #32 producer slices).

    See `affects_followups.md` for full context.
26. **Cooldowns + global lag** (§12 / §4). Per-skill `cooldown_until`;
    integrates with the §4 cooldown infrastructure. **Landed 2026-05-09**
    — both slices in one PR. Slice A (§4): `Command.Lag time.Duration`
    + `Session.NextReady` (crossMu-guarded) + per-segment gate in
    `Registry.dispatchOne`; refuse-with-message V1, stamp-on-success-
    only; wired to `attack`/`kill`=3s, `flee`=2s, `parry`=1s,
    `shout`/`yell`=2s. Slice B (§12): migration 0047 added
    `skill_cooldowns_json`, new `Character.SkillCooldowns`,
    `CharacterRepo.RecordSkillCooldown`, and the `cooldown` (Admin,
    audited) + `cooldowns` (Player) verbs. Cooldowns are stored as
    absolute deadlines, lazy-expired on read, and pruned on every
    write. V1 producer is admin-only — real skill-check verbs stamp
    when their gates land. Deferred: bounded queue (single dispatcher
    swap on the same wire shape), movement lag (waits on §9 sector
    cost table), say/tell lag (anti-RP), Skill.Family field for the
    `cooldowns` spec's grouped display.
27. ~~**Channeling slot refresh + madness tick** (§9). Schema half-
    landed (creature/channeling tables); chargen seeded affinities +
    starting weaves; this finishes the per-tick mechanics (slot
    refresh on rest, madness accrual while embraced for men, stilled
    state).~~ **LANDED 2026-05-08.** New `internal/channeling`
    package: `RefreshIfDue` (8h wall-clock cooldown, no-op when
    `Stilled`) + `AccrueMadness` (Saidin + Embraced + non-Stilled,
    int16-clamped) as pure functions; `SessionTicker` walks the
    same Candidate snapshot the affects ticker uses and persists
    via the new `CharacterRepo.RecordChanneling` (mirrors
    `RecordAffects` shape; no migration — `LastSlotRefreshAt`
    piggybacks on the existing `channeling_json` blob).
    Subscribed to `Buckets.Regen` (30s). New verbs: `embrace` /
    `release` (Player; toggle `Embraced`, stamp
    `EmbracedSince`, refused on `Stilled`) and `still` / `unstill`
    (AuthAdmin, audited; online-targets-only V1; `still` zeroes
    `Slots[*].Cur` so the gate is observable immediately,
    `unstill` does NOT auto-refill — slots refill on the next 8h
    pulse). `score` gained a Channeling subsection rendering
    Source / per-level slot pools / Madness / state flags.
    Deferred: `rest` verb (would unlock embracing's rest/heal
    blockers), Madness thresholds + Mental Stability save +
    `Heal the Mind` reduction, embrace passives (same-gender
    perception, saidar aura, gender detection within 15 ft),
    angreal/sa'angreal slot bonuses, bond/circle/a'dam
    interactions.
28. ~~**Mid-game weave learning** (§12). New weaves added to
    `WeavesKnown` via trainer NPC + practice-points spend. Catalog
    already loaded by Phase C #10.~~ **LANDED 2026-05-08.**
    Migration 0043 added the `weave_teachers` table (1:1 to
    `mob_template`, carrying `max_level_taught` + `affinity_filter`
    PowerSet). Optional `weave_teacher:` YAML block on a mob seeds
    the row (data/world/README.md updated). Chargen `Weave` gained
    `practice_cost int` (validated `>= 0`); all eight catalog level-0
    weaves seeded with `practice_cost: 1`. Practice-points earning:
    `ComputeLevelUp.PracticeDelta = 1` per level for every class,
    threaded through `LevelUpFields.PracticePointsDelta` and
    deposited by `RecordLevelUp` (the existing `practice_points`
    column from migration 0009 was previously unwritten). Verb:
    `learn weave` now detects a weave-teacher in the room — present
    drains `practice_points` via the new
    `RecordWeaveStudy(ctx, id, weaveID, newPP)` (mirrors
    `RecordWeavePick`'s tx shape); absent keeps the existing
    `pending_weaves` chargen-pool drain. Menu shows the teacher
    byline + per-weave PP cost + filtered offerings (level cap +
    affinity filter intersected with the channeler's own
    affinities). Audit on the mid-game path: `kind=weave_study
    power=<p> cost=<n>`. `score`'s Channeling block gained a
    Practice line. Deferred: outside-affinity learning at premium
    cost (Wilders 2 / Initiates 0 from §12 lines 43–50; both paths
    still refuse outside-affinity entirely), class-based weave-level
    caps (moot — catalog is level-0 only), PP earning from sources
    other than level-up, time-cost / lesson-fee, the §12 numeric
    weave-table reconciliation between `WeavesKnown []WeaveRef` and
    `WeavesKnownIDs []string`. RMW TOCTOU on `RecordWeaveStudy`
    inherits the same followup as `RecordWeavePick` — see
    `optimistic_lock_followups.md`.

After E: meaningful vertical progression on top of a chargen-
complete character.

---

## Phase F — NPC behavior & quests

Content multiplier. Without this the world is static.

29. ~~**Trigger / event system** (§15) — `on_enter`, `on_say`,
    `on_attack`, `on_death`, `on_tick`. Pure dispatch layer; consumers
    in 30–31.~~ **Landed 2026-05-08.** Migration 0044 added the
    `triggers` table (CHECK on `owner_kind in
    ('mob_template','room')` and on `event in
    ('on_enter','on_say','on_attack','on_death','on_tick')`) plus
    `(owner_kind, owner_id, event)` and `event` indexes. YAML
    schema gained an optional `triggers:` block on `Mob` and
    `Room`; loader inserts rows in the same transaction as the
    owner via `insertRoomTriggers` / `insertMobTriggers`,
    validation rejects unknown event/empty action/malformed
    payload. `internal/trigger/` ships `Registry` (in-memory
    index keyed by owner+event with priority-DESC ordering),
    `ActionRegistry` (V1 builtins `noop` / `say` / `emote`;
    consumers register more), `Runner` (priority fan-out, swallows
    handler errors so one bad trigger can't take down the bus),
    and `Dispatcher` wiring the existing eventbus +
    `tick.Buckets.Phase`. NEW event `world.PlayerSaid{Speaker,
    RoomID, Text}` published by `internal/cmd/comm.go::NewSay`
    after the room broadcast (silent rooms / empty payload
    short-circuit before publish). Action handlers MUST use
    `Session.WriteAsync` (cross-session output rule — they run on
    the eventbus goroutine). Deferred: item-owned triggers (no
    new schema needed beyond CHECK widening), per-trigger
    consecutive-fault auto-disable (only matters once user-
    authored Lua lands in #32), `on_login` / `on_logout` PC
    events, sharded `on_tick` walks once content uses them, and
    the `tedit` authoring verb (#34).
30. ~~**NPC dialogue trees** (§15). JSON per mob; uses §13 `say`
    capture.~~ **Landed 2026-05-08.** Migration 0045 added
    `mob_templates.dialogue_json` (nullable TEXT). YAML schema gained
    an optional `dialogue:` block on `Mob` (root + ordered nodes,
    each with prompt + responses; responses carry match keywords,
    reply text, next-node, effect list, and flag-gated `show`
    visibility). `internal/dialogue/` is the pure-data package
    (Tree/Node/Response/Effect/Show + Validate); `internal/mode/
    dialogue.go` is the runtime — pushed by `talk <mob>` —
    rendering numbered choices and dispatching numeric / keyword /
    `bye`-style input. Effects implemented: `set_flag`,
    `clear_flag`, `goto`, `push_mode` (closure-injected by the
    cmd-layer for future shop/banker hand-off; nil in V1), `end`.
    Per-character branch state (current node + flag bag) is
    in-session only — drops on logout; persisted per-NPC state is
    deferred to #31. The legacy reserved `mob_templates.
    dialogue_tree_id` INT column from 0008 stays unused (V1 trees
    are one-off per template; collapsing it would force a
    migration with no win). Followups: persisted flag bag,
    catalog-style reusable trees, `tedit` OLC, on_say-driven
    ambient dialogue layered on top of trees.
31. ~~**Quest engine state machine** (§15). Per-character per-quest
    state + objective ticks.~~ **Landed 2026-05-09.** No migration
    — reused `characters.quest_log_json` from migration 0009 and
    widened `creature.QuestProgress.QuestID` to string for
    catalog-id round-trip. V1 step types: `talk_to`, `kill_n`,
    `reach_room`. `fetch` / `deliver` deferred (need
    `world.ItemPickedUp` / `world.ItemGivenToMob` events from
    get/give verbs); `script` deferred to #32 Lua.
    `internal/quest/` is the new package — Catalog (one YAML file
    per quest under `internal/quest/default/`, `QUEST_DIR`
    override mirrors chargen/news), Validator (cross-refs
    against world content at boot), Engine (subscribes to
    `combat.CombatDeath` + `world.PlayerEntered`; talk_to
    drives via dialogue effect, not an event). New dialogue
    effects `accept_quest` / `advance_quest` use the same
    closure-injection pattern as `push_mode` so the cmd-layer
    wires them without internal/cmd importing internal/quest.
    `combat.CombatDeath` gained `MobTemplateID` and
    `MobTemplateExternalID` fields so kill_n can match by
    template ExternalID without re-fetching the dead instance.
    Final-step transition grants XP via `RecordXP` and coin via
    `RecordCoin` with one optimistic-lock retry on
    `ErrCoinConflict`; one `audit.Record(verb=quest_complete)`
    per completion. New `quest` verb (alias `quests`): bare
    lists active + completed, `info <id>` renders steps with
    progress markers, `abandon <id>` drops + audits. Followups:
    item events for fetch/deliver, item rewards on completion,
    qedit OLC, group quest sharing, XP-debt drain on quest XP.
32. **Embedded scripting (gopher-lua) + sandbox** (§15). Biggest lift
    in the whole roadmap. Sliced.
    - **Slice 1 — landed 2026-05-09**: foundation. Migration 0046
      added `triggers.consecutive_faults` + `triggers.disabled`.
      `internal/scripts/` is the catalog (one `*.lua` per file
      under `internal/scripts/default/`, `SCRIPT_DIR` env
      override, syntax-validate at boot). `internal/lua/` is the
      sandbox + runner: LState pool of 8 (pre-allocated; bounded
      via buffered channel — no overflow allocation), all
      dangerous globals stripped (`os`/`io`/`debug`/`package`/
      `dofile`/`loadfile`/`loadstring`/`load`), 50ms ctx timeout
      per call via gopher-lua SetContext. New trigger action
      kind `lua` resolves a script name from the payload
      (`{"script":"warden_alert"}`) and runs it with the V1 API:
      `say(text)` / `emote(text)` / `log(level, msg)` / read-only
      `ctx` table. Faults wrap `trigger.ErrActionFaulted`; the
      Runner auto-disables at `FaultThreshold = 5` and resets on
      success. World re-deploys reset both columns via
      `TriggerRepo.ResetAllFaults`.
    - **Slice 2 — landed 2026-05-09**: state-machine APIs.
      Dialogue gains `effects: kind: script` (Args["script"]
      names a catalog entry); quest gains `kind: script` step
      kind (catalog name in `Step.Script`). Lua API V2 adds the
      `quest` table (`quest.accept(id)` / `quest.advance(id)`)
      and a top-level `push_mode(name)` global; nil-bound hooks
      register classified-error stubs so misuse trips the fault
      budget instead of "attempt to call nil". Engine gets a
      kind-agnostic `Advance(charID, questID)` covering both
      `talk_to` and `script` step kinds; counter-driven kinds
      (`kill_n` / `reach_room`) log + no-op so a stale Lua call
      can't skip a kill quota. Trigger Lua actions thread the
      same V2 hooks via `LuaQuestHooks`; events without a
      character actor (`on_tick`) refuse the quest API at
      EventCtx-resolve time and surface as `ErrActionFaulted`.
      Boot-time cross-ref: dialogue `script` effects validate
      against the script catalog (in main.go's
      `validateDialogueScriptRefs`); quest `StepScript` cross-
      refs via the new `quest.RefSets.Scripts` set.
    - **Slice 3 — landed 2026-05-10**: API surface broadened for
      content authors. Three composing closures wired through both
      trigger and dialogue paths: `apply_affect(target_id,
      effect_id)` (resolves the effects catalog and feeds the §E
      #25 producer pipeline; sentinel Source = -3 → "script"
      label in the affects inspect verb), `give_item(target_id,
      external_id)` (clones a YAML-seeded template into the
      target's inventory; mirrors the admin spawn path), and a
      read-only `target` table with `target.hp(id) → cur,max` and
      `target.level(id) → int` (multiclass sums ClassLevels).
      Trigger handler guards mutations with the same actor-kind
      check the V2 quest API uses (mob-fired triggers refused
      with classified faults); read APIs are unguarded by design.
      `LuaQuestHooks` renamed to `LuaHooks` (legacy alias kept).
      Two demo scripts (`bless_actor.lua`, `gift_potion.lua`)
      shipped in the catalog so the API has live exercise +
      authoring examples. Release wipe-list extended to cover
      `apply_affect`, `give_item`, `target` so per-call state
      can't leak across pool borrows.
    - **Slice 4 — landed 2026-05-10**: polish bundle. Read APIs:
      `room.players()` / `room.mobs()` (resolved from EventCtx.RoomID
      at bind time so scripts can't snoop on other rooms),
      `clock.hour()` / `clock.day()` (new `Clock.Day()` method on
      `internal/world/dayclock.go`), `target.classes(id)` returning
      multiclass map keyed by chargen catalog class id (e.g.
      `{ armsman = 3, initiate = 2 }`). Mutation surface:
      `apply_affect` gained an optional 3rd arg (durationOverride
      int32; 0 means "use catalog default") so scripts can override
      the catalog's authored DurationTicks per call. APIBindings
      .ApplyAffect signature changed (3 args); LuaHooks struct
      extended with 5 new fields; release wipe-list now covers
      `room` + `clock`. Two demo scripts added: `check_alone.lua`
      (room.players + apply_affect) and `night_warning.lua`
      (clock.hour gate).
    - **Slice 5a — landed 2026-05-11**: combat + inventory
      mutations (§D crit polish unblock). Four new APIBindings
      hooks: `deal_damage(target_id, amount [, source])`,
      `heal(target_id, amount)`, `transfer_item(item_id,
      to_owner_id)`, `drop_item(item_id)`. New
      `combat.Manager.ApplyDamageExternal` /
      `combat.Manager.ApplyHealing` entry points mirror the
      `HandleAffectDeath` shape — raw amount (no DR/resists/crit),
      no threat-table mutation, lethal damage routes to the
      existing `handleCharacterDeath` / `handleMobDeath` pipelines.
      Two new events: `combat.ScriptDamageDealt` (with `Lethal`
      flag so the default-narration subscriber suppresses the
      "you suffer N damage" line on lethal hits — death lines
      already flow from `CharacterDied` / `CombatDeath`) and
      `combat.ScriptHealingApplied` (Amount == 0 when already at
      full HP so the warm-light flavor line still renders).
      Cmd-layer subscribers in `cmd/server/main.go` render
      default narration via `Session.WriteAsync` for unsourced
      damage / heal. Lua bindings take a single `target_id`;
      `resolveLuaTarget` tries `CharacterRepo.GetByID` first
      then `MobInstanceRepo.GetByID` so both kinds work behind
      one binding. No actor-kind guard at the trigger layer
      (mob-fired triggers legitimately damage / heal players);
      `drop_item` keeps a `ev.RoomID != 0` adapter guard
      (mirrors `room.players` / `room.mobs`) so a context-less
      drop trips the fault budget instead of dumping into room
      0. Killer attribution on lethal damage is anonymous in
      V1 (`ActorRef{}`); future slices may thread an authored
      hint through the source string. Two demo scripts:
      `script_strike.lua` (emote + deal_damage) and
      `divine_heal.lua` (say + heal). Wipe-list extended with
      the four new globals so per-call state can't leak across
      pool borrows.
    - **Slice 5b — landed 2026-05-11**: async + login lifecycle +
      inventory iter. Three new APIs / events:
      `wait(seconds, "script_name")` defers a fresh `runner.Run`
      via `tick.AfterCtx`; range 1..300s; snapshots the firing
      `EventCtx` so the deferred run sees the same actor/room
      context. `inventory(target_id)` returns a Lua table of
      `{id, name, external_id}` for the target's top-level
      inventory (wraps `ItemRepo.ListInInventory`). `on_login` /
      `on_logout` are two new trigger event kinds (room-owned)
      backed by new `world.PlayerLoggedIn{CharacterID, RoomID}`
      and `world.PlayerLoggedOut{CharacterID, RoomID}` events.
      Migration 0051 widens the `triggers.event` CHECK via the
      SQLite table-rebuild dance (preserves the
      consecutive_faults / disabled columns from 0046 and the
      `idx_triggers_owner_event` / `idx_triggers_event` indexes
      from 0044). Login publish point: a new package-level hook
      `mode.SetLoginPublisher` is wired by `main.go` at boot and
      called from `promoteToGame` immediately after `SetInWorld`
      — chosen over threading a bus through promoteToGame's four
      call sites (character_select, character_create x2,
      account_menu_play). Logout publish point: `handleConnection`'s
      defer block, guarded on `s.CharacterID != 0` so account-
      menu-only disconnects don't publish a phantom logout.
      Late-binding for `wait()`'s shutdown ctx: `main.go` declares
      `var srvShutdownCtx context.Context` before the luaHooks
      block and back-fills it after `signal.NotifyContext`; the
      wait factory captures a pointer and dereferences at fire
      time (safe because scheduler.Start runs after the back-fill).
      Dialogue scripts get `inventory()` but NOT `wait()` —
      async deferral inside an interactive dialogue creates
      surprising UX; authors who need it route through a trigger.
      Two demo scripts shipped: `wait_demo.lua`
      (emote + scheduled script_strike) and `confiscate.lua`
      (inventory iter + transfer_item composition). Release
      wipe-list extended with `wait`, `inventory`.
    - Slice 6: OLC `tedit` (depends on Phase G).

32a. ~~**Authored mob paths + pathfinding** (§15 / §10).~~ **LANDED
    2026-05-12** across two slices.
    - **Slice 1 — strict path** (commit `5a5eb01`). Migration
      `0053_mob_templates_path.sql` adds `mob_templates.path TEXT`
      holding a JSON array of room external_ids. YAML schema gains
      an optional `path: [room_ext, ...]` block on `Mob`
      (`internal/world/yaml.go`); loader validates length ≥ 2,
      no dupes, all rooms exist, every adjacent edge walkable via
      `validateMobPath`, then resolves external_ids → internal
      room IDs into `MobTemplate.PathRoomIDs` in
      `internal/world/loader.go::insertMobs`. Runtime branch
      `WanderHandler.considerStrictPath`
      (`internal/mob/wander.go:316`) advances one step per wander
      tick along a closed loop (modulo wraparound), ignoring
      `wander_chance`. Per-template `wanderProfile` cache
      (`internal/mob/wander.go:202`) avoids re-fetching templates
      per pulse. Tests in `internal/mob/wander_test.go`
      (`TestWander_StrictPath_*`) cover closed-loop traversal,
      chance-zero bypass, off-path no-op, and blocked-door no-step.
      QA fixture: `test.qa.patrol_guard` in
      `data/world/test/qa_zone/mobs.yaml` walks hub ↔ combat_wander.
    - **Slice 2 — BFS pathfinding** (commit `ce1168a`). Migration
      `0054_mob_templates_wander_radius.sql` adds
      `mob_templates.wander_radius INTEGER NOT NULL DEFAULT 0`.
      Branch `considerBFSWander` (`internal/mob/wander.go:403`)
      picks a goal room within `wander_radius` hops via BFS over
      the existing room graph and steps one room per tick toward
      it; builds on `mob_trails` so backtracking is observable.
      QA fixture: `test.qa.scout` (`wander_radius: 2`). Review
      fixes in commit `dddc270`.
    Deferred (recorded in `mob_paths_followups.md`, unblock when
    content demand surfaces): **ping-pong path mode** (only
    closed-loop wraparound shipped) and **persistent `path_index`
    on `mob_instances`** (current handler recomputes position by
    linear scan per pulse — fine for small N, acceptable trade
    versus a new column).

---

## Phase G — OLC

Once content matters, builders need to author it without YAML edits +
restart.

33. ~~**Permission/builder role formalization** (§16).~~ Landed
    2026-05-12 across two slices.
    - Slice 1: migration 0055 (`builder_zones` table), `BuilderZoneRepo`
      (memory + sqlite + shared test suite), and admin `grant` /
      `revoke` / `grants` verbs (AuthAdmin only, audited).
    - Slice 2: `Session.BuilderZones` cache populated by
      `postauth.promoteToGame` (via `BuilderZoneRepo.ListForCharacter`),
      `cmd.CanEditZone(s, zoneID)` helper, and online-target refresh in
      `grant` / `revoke` (re-fetch + `Session.SetBuilderZones` +
      `WriteAsync` notice).
    - Note: the original "introduce AuthBuilder enum tier" approach was
      attempted and reverted — modernc-sqlite hung on the `ALTER TABLE
      DROP COLUMN` required to widen `characters.auth_level`'s
      `BETWEEN 0 AND 2` CHECK. The per-zone-only design (AuthLevel
      stays {Guest, Player, Admin}; `builder_zones` is the scope
      table) matches the spec's "per-zone builder grants" intent and
      is what #34 consumes.
34. **`redit` / `oedit` / `medit` / `zedit`** (§16). Mode-based
    editors using the existing mode stack.
    - Slice 1 (redit V1): landed 2026-05-12. New `RoomRepo.Update`
      (memory + sqlite, shared test) for the OLC-editable subset
      (name / short / long / flags / sector / light / extras —
      identity / coords / zone preserved). `cmd.NewREdit` verb gates
      via `CanEditZone`; pushes `mode.REdit` which buffers a draft
      and exposes `show / name / short / desc / flag <n> [on|off] /
      sector / light / done / cancel / help`. `done` commits and
      audits the field-name list; `cancel` discards.
    - Slice 2: LANDED 2026-05-17. Three pieces in one commit.
      **Multi-line `desc`**: `desc` with no argument now enters a
      mode-internal buffering state (`bufActive` + `bufLines`);
      every subsequent input line accumulates until `.` on its own
      line flushes via `strings.Join(lines, "\n")` into
      `Room.LongDesc` and marks the draft dirty. `@abort` discards.
      Single-line `desc <text>` still works. **`extra <keyword>
      ...`**: list (`extra`) / show (`extra <kw>`) / set
      (`extra <kw> <text>`) / multi-line (`extra <kw> .`) / delete
      (`extra <kw> delete`). Keywords lowercased on input to match
      the convention documented on `Room.ExtraDescs` (and what
      `marshalExtraDescs` already enforces on write). Extras commit
      through the existing `done` → `RoomRepo.Update` path with no
      schema changes — the `extra_descs_json` column has shipped
      since migration 0013. **`exit <dir> ...`**: foundation work
      added `ExitRepo.Update` (memory + sqlite + shared
      `runExitRepoTests`) that overwrites the authoring subset
      (to_room_id, description, key_external_id, lock_difficulty,
      pickable, hidden, nopass) while preserving identity, runtime
      door state (closed, locked), and the `authored_*` snapshots.
      Subverbs: `show` / `desc` / `key <id|none>` / `difficulty
      <0-100>` / `flag <pickable|hidden|nopass> [on|off]` / `to
      <room_external_id>`. Exit edits write through `ExitRepo.Update`
      immediately (not buffered into the draft like room fields)
      and each successful edit emits one `admin_audit` row with verb
      `redit_exit` and target `<room_ext>:<dir>:<field>`. `NewREdit`
      signature grew to `(rooms, exits, audits, RoomLookupFn, room)`;
      the lookup closure is wired in `cmd/server/main.go` from
      `RoomRepo.FindByExternalID`. Tests: 14 new cases in
      `internal/mode/redit_test.go` cover single+multi-line desc,
      extras set/show/delete/lowercased/multi-line, and every exit
      subverb including bounds and refusal paths.
    - Slice 3 (deferred-pending-Flow, §M.7 decision 2026-05-18):
      `oedit` (item templates), `medit` (mob templates), `zedit`
      (zones). Originally scoped as three more mode-pair editors
      fanning out from the `redit` pattern. **Now waiting on
      Phase N #N19 (Flow engine)** — once Flow lands, these
      three editors become YAML/Lua flow definitions and the
      ~600 LOC of hand-rolled mode code is avoided. Until then
      Phase G is "redit shipped; oedit/medit/zedit pending Flow."
      Do NOT build slice 3 against the redit pattern — that work
      would be throwaway when Flow arrives.
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

44. ~~**CHARSET / UTF-8 negotiation** (§1).~~ Landed 2026-05-14.
    `WILL CHARSET` on connect; on `DO CHARSET` the server offers
    `;UTF-8`. `Session.Charset` drives `WrapText`'s display-cell
    counting via `go-runewidth` (the §2 wide-glyph item closed in
    the same PR). `telnet/charset_test.go` covers the handshake.
45. ~~**MSSP** (§1).~~ Landed 2026-05-14. Full crawler variable set
    (NAME / PLAYERS / UPTIME / world stats / capability flags) in
    `cmd/server/main.go::msspVars`; encoder in `telnet/mssp.go`;
    optional `mssp:` config block adds CONTACT / HOSTNAME /
    LOCATION / WEBSITE / STATUS. Provider closure
    (`Session.MSSPProvider`) wired at session construction.
46. ~~**GMCP** (§1).~~ Landed 2026-05-14. `internal/gmcp.Manager`
    owns inbound Core.* dispatch + per-session subscription
    lifecycle; V1 outbound packages are Char.Name, Char.Vitals,
    Char.Status, Room.Info, Comm.Channel.Text. New world events
    `ChannelBroadcast` + `PlayerTold` cover every chat surface.
    Plus a `clients/mudlet/` package (v1) that consumes those
    frames — drag-and-drop installs HP/SP gauges, auto-mapper,
    per-channel chat panes, and character header. Build with
    `make mudlet-package`. Stretch (triggers / aliases / affects)
    deferred to v2 alongside `Char.Affects` and `Char.Quests`.
47. **MCCP2/3** (§1). Compression; nice-to-have unless bandwidth is a
    real concern.
48. **MXP** (§1). Clickable links; useful once help/news is rich.
49. **TLS listener** (§1).
50. **WebSocket gateway** (§1). Browser clients — high payoff if you
    want public reach.
51. **SSH listener for ops** (§1). Lowest priority unless doing a lot
    of remote admin.

### Out-of-phase fixes landed alongside Phase I (2026-05-14)

Tracked here so the timeline reads cleanly; these aren't numbered
backlog items, just polish that surfaced during the Phase I work.

- **Long-token break in WrapText** (§2). Tokens whose cell width
  exceeds the wrap width are now chunked into successive lines
  instead of overflowing past the right margin. URLs in MOTD /
  help / chargen render correctly on narrow Mudlet profiles.
  Closes `terminal_rendering_followups.md` item #3.
- **WrapText CR-drop regression coverage** (§2). Locks in the
  CR-drop contract `WriteWrapped` / `WritePagedWrapped` depend on
  via three new tests; documents the dependency in both helpers'
  godocs. Closes `terminal_rendering_followups.md` item #6.
- **World loader additive resync** (§9). Loader now picks up new
  YAML rooms / items / mobs on every boot instead of silently
  short-circuiting when the DB already has rows. Strictly additive
  — operator-edited rows keep their drift. Boot summary log shows
  per-table new-row counts.
- **`auth_level` startup audit** (§6). Boot-time clamp brings any
  row with `auth_level > AuthLevelMax` back into [0,2] with a warn
  log. Fixes "invalid auth_level <N>" errors locking accounts out
  of character select on DBs that predated migration 0019's CHECK
  constraint.
- **Width-aware cursor accounting** (§2). The byte dispatcher now
  accumulates UTF-8 continuation bytes and hands complete runes to
  a new `LineEdit.InsertRune`; backspace / delete / motion / kill /
  replace primitives walk by rune and emit cell-count BS. Password
  mode echoes one `*` per glyph; `extendBuffer` and `WriteAsync`'s
  masked redraw follow the same cell-count contract. Pairs with
  the §44 wrap-side cell-width work to make the full input + output
  pipeline UTF-8 / CJK correct. Closes
  `terminal_rendering_followups.md` item #2.

---

## Phase J — Ops, CI, packaging — LANDED 2026-05-12

Shipped across slices J1–J7. See `ROADMAP.md` §7 / §19 / §20 / §21 / §22
for the per-item completion notes.

52. ~~**GitHub Actions CI matrix** (§21) + **coverage target** (§21).~~
    Landed J1 (commit `38544bd`): ubuntu+macos matrix, race + atomic
    coverage profile, staticcheck job (non-blocking). Baseline 72.5%.
53. ~~**Config file (TOML/YAML) + per-env overrides + `.env.example`**
    (§20).~~ Landed J2 (commit `b698b40`): YAML over defaults with env
    overrides, `-config` flag, `config.example.yaml` + `.env.example`.
54. ~~**Metrics + pprof on private `:9090`** (§19).~~ Landed J5 (commit
    `195d352`): Prometheus registry, `/metrics` `/healthz`
    `/debug/pprof/*`, loopback default.
55. ~~**Request/command audit log per character** (§19).~~ Landed J3
    (commit `8c921a3`): migration 0052 + repo + Game audit hook,
    opt-in via `audit.commands_enabled`.
56. ~~**Backup rotation (`VACUUM INTO`)** (§7).~~ Landed J4 (commit
    `938579f`): wall-clock ticker, mtime-based retention, gated on
    `db.backup_dir` + interval.
57. ~~**Telnet integration test driving the protocol** (§21).~~ Landed
    J6 (commit `cbd3a24`): subprocess-based end-to-end smoke under
    `-tags=integration`.
58. ~~**Fuzz tests on IAC parser + tokenizer** (§21).~~ Landed J1
    (commit `38544bd`): `FuzzReadIAC` + `FuzzTokenize` +
    `FuzzSplitOnSemicolon`, nightly workflow with per-target matrix.
59. ~~**`goreleaser` + systemd unit + healthcheck** (§22).~~ Landed
    J7 (commit `d86665c`): `.goreleaser.yaml`, hardened Dockerfile,
    `deploy/systemd/wheelmud.service`, `release.yml` workflow with
    multi-arch GHCR push.

---

## Phase K — Crafting (future, high-level)

Not on the near-term path; documented here so design-affecting
decisions in earlier phases (item taxonomy, shop economy, skill
ranks) keep this surface in mind.

Common needs once we sit down to design it:

- **Recipe catalog** — YAML-authored, mirrors the chargen / news
  embed-with-override pattern. Each recipe: id, skill + min rank,
  required tool item ids, station / room flag (forge / workbench /
  alchemy table), input items + counts, output item id + count,
  practice/skill check, time cost.
- **Material taxonomy** — extends §9 item taxonomy. New
  `ItemTypeMaterial` (or sub-flag on TradeGood) with stackable
  semantics. Drops from mobs and gathered from sector-typed
  rooms (e.g. mining, herbalism).
- **Stations** — room flag (`flag_forge` etc.) gates which
  recipes run there. Slots into existing room flag plumbing.
- **Skill integration** — reuses §12 skill ranks + §26 cooldown
  infra. Failure consumes inputs (or partial inputs) and grants
  practice. Success grants XP via existing `RecordXP`.
- **Quality tiers** — masterwork / common / inferior on the
  output item. Composition with §9 masterwork follow-up: same
  tier enum, same stat modifiers.
- **Verbs** — `craft <recipe>`, `recipes` (list known),
  `learn recipe <id>` (from a teacher NPC, mirrors weave-teacher
  pattern), `gather` for sector-driven harvest.
- **Persistence** — new `character_recipes` table (id +
  characters_id, mirrors `character_skills`). One row per
  recipe-known.
- **Economy hook** — shopkeepers should buy crafted goods at
  reduced markdown vs. trade goods to avoid trivial money loops.

Blocked on: nothing structural. Best paired with a `gather`
sector pass and a §9 masterwork follow-up so the output stat
tier has somewhere to land.

---

## Phase L — Combat depth & feel

Phase D shipped a working combat MVP: round-robin, one action per
4 s bucket, 3 s verb-lag gate. Functionally complete but
**tempo-wise it reads as turn-based** — a 2-combatant fight swings
every 4 s regardless of who's fighting. Long-term we want combat
that distinguishes an Aiel spearman from a Borderland heavy
without anyone re-typing their verb faster. See ROADMAP §11
"Action cost & per-actor cadence" for the full design; this
phase is the implementation order.

Direction lock-in: **per-actor cadence with action costs**, not
"speed up the bucket." Cheap to start, scales naturally into
racial flavor, stamina, iterative attacks, and richer action
variety. Each slice is shippable on its own; we can stop after
any slice and the game still works.

60. ~~**Per-actor cadence — kill the round-robin.** Foundation
    slice. No new content, no balance changes; just replace
    "one actor per pulse" with "every actor whose timer fired."~~
    Landed 2026-05-10.
    - `ActorEntry` gains `NextActAt time.Time` and
      `LastActedAt time.Time` (debug-only; helps tests assert
      "Aiel acted 3× while Trolloc acted 1×").
    - `tickRoom` stops advancing `ActiveIdx`. Instead it walks
      `Order` in initiative position, and for each entry whose
      `NextActAt <= m.now()`, pops the queued action and runs
      `resolveAction`, then sets `NextActAt = m.now() +
      defaultActionCost(action)`. `ActiveIdx` field deleted
      (or repurposed as "last actor to swing" for prompt).
    - New `combat.defaultActionCost(action Action) time.Duration`
      pure fn — flat table for now: Attack=3 s, Parry=2 s,
      Flee=2 s. Tuneable constants, not a function of actor yet.
    - Bucket cadence drops from 4 s → 1 s
      (`DefaultCombatInterval`). Verb `Lag` drops accordingly:
      attack 3 s → 0.5 s, parry 1 s → 0.5 s, flee 2 s → 0.5 s.
      Lag is now input fairness, not combat pacing.
    - `Manager.SetClock` already exists; tests get rewritten
      around clock advances rather than tick counts.
    - **Slice exit criterion:** existing combat tests pass with
      the new tick loop; two-combatant fight feels like real-time
      melee, no flavor differentiation yet.

61. ~~**Attack variants — `power` and `quick`.** First flavor
    payoff with no new schema.~~ Landed 2026-05-10.
    - `Action.Kind` keeps `Attack`; `Action` gains
      `Variant AttackVariant` (`Normal | Power | Quick`).
    - `attack <target>` keeps current behavior (`Normal`).
      `attack <target> power` (alias: `power <target>`) and
      `attack <target> quick` (alias: `jab <target>`) queue the
      variants. Re-issuing during a fight switches the variant
      for the next swing.
    - `defaultActionCost` becomes a 1-arg lookup keyed by
      `(Kind, Variant)`: Normal 3 s / Power 4.5 s / Quick 1.8 s.
      `RollDamage` reads a variant multiplier: Normal 1.0 /
      Power 1.5 / Quick 0.6. Attack roll bonus: Normal 0 /
      Power -2 / Quick +1.
    - Echo lines distinguish variants ("you lunge with a power
      strike", "you flick a quick jab").
    - **Slice exit:** a high-Dex character realistically chains
      quick jabs while a heavy hitter loads up power swings;
      damage-per-second roughly equivalent across variants on a
      stationary target.

62. ~~**Gear-driven speed — weapon weight + armor encumbrance.**~~
    Landed 2026-05-10.
    - `combat.ActionCost(base, weaponWeight, armorWeightClass)` is
      the new pure fn layered on top of `DefaultActionCost`.
      Manager-side: `actorActionCost(ctx, ref, action)` +
      `resolveEquipment(ctx, ref)` mirror `resolveCore`.
      `tickRoom` callsite at L366 swapped from `DefaultActionCost`.
    - Weapon weight comes from `repo.Item.Weight` (lb) — not
      `WeaponStats.Weight` (no such field). Armor weight from
      `repo.ArmorStats.WeightClass` (already a string). No loader
      changes needed.
    - `score` sheet renders `Combat: 1.95x (1.50 weapon x 1.30
      armor)` directly under movement Speed in Vitals;
      `NewScore` took an `items repo.ItemRepo` parameter (nil
      yields the unarmed/naked baseline so tests keep passing).
    - QA fixture: room `test.qa.speed_range` off the hub via `u`,
      dummy `test.qa.dummy_speed`, kit greatsword (16 lb) /
      dagger (1 lb) / plate (heavy). Mob-side equipment authoring
      deferred — would need a new migration on `mob_templates`.

63. ~~**Racial speed + stamina pool.** First "your race matters"
    pass; biggest schema lift in the phase.~~ Landed 2026-05-11.
    - Migration: `characters.stamina_cur` + `.stamina_max` +
      `.stamina_regen` (int16; mirror HP shape). `creature.Core`
      gains the three fields. Mob templates get the same trio
      via existing `Core` round-trip — no separate column on
      `mob_templates`.
    - Race table grows `SpeedFactor float32` + `StaminaMax int16`
      + `StaminaRegen int16` (per-pulse on `Buckets.Regen`).
      V1 seed: Human 1.0×/100/2, Aiel 0.7×/130/3, Ogier 1.2×/150/1,
      Trolloc 1.0×/110/2, Myrddraal 0.8×/120/2.
    - `actorActionCost` folds in `core.SpeedFactor` as another
      multiplicative term.
    - Actions gain `Stamina int16` cost (Normal=5, Power=12,
      Quick=3, Parry=4, Flee=8). `Manager.EnqueueAction` refuses
      with `ErrInsufficientStamina` when the pool is dry; the
      `attack` verb surfaces it as "you're too winded".
    - `tick.Buckets.Regen` ticker (already exists for affects)
      gets a new subscriber that refills `Stamina` toward
      `StaminaMax` at `StaminaRegen`/pulse. Regen halts while
      `Core.HPCurrent <= 0`; modifier on the regen rate from
      armor weight (heavy = -50% regen).
    - `score` sheet gains a stamina line; `prompt` template
      reserves `%p`/`%P` (cur/max) — wires into the §2 prompt
      templating slot.
    - **Slice exit:** Aiel can chain 4–5 quick jabs in the
      window a plate Borderlander gets one swing, then visibly
      tires; recovery feel matches the flavor.

64. ~~**New action verbs — `dodge` / `throw` / `sidestep`.** The
    action menu Aiel actually wants.~~ Landed 2026-05-11.
    - `dodge`: short defensive stance (1 round). Grants
      `FlatFootedUntil` immunity + +4 Defense from the next
      attack against you. Cost 1.0 s + 3 SP. Mirror of `parry`
      but Dex-favored vs. weapon-favored.
    - `throw <weapon> <target>`: ranged-attack variant.
      Resolves like `Attack` but consumes the
      `creature.SlotPrimaryWield` item from the wield slot via
      `Equipment.Set(Slot, 0)` + `RecordEquipment`, and on
      resolution drops the thrown item to the target's room
      (or into the corpse on a kill). Throwable items need a
      new `FlagThrowable` on `WeaponStats`. Cost 2.0 s + 6 SP.
    - `sidestep <attacker>`: applies a one-round
      `FlatFootedUntil` to the named attacker. Cheap (0.5 s +
      2 SP) — pure positional play, no damage. Mirrors how the
      Manager already tracks `FlatFootedUntil` for parry
      reflections.
    - Each verb is a separate `*telnet.Command` in
      `internal/cmd/`, following the `attack.go` pattern.
      `Action.Kind` grows three values; `resolveAction`'s
      switch grows three cases.
    - **Slice exit:** the qa_zone gains a "skirmisher dummy"
      and a "throwing range" fixture so a tester can verify
      the full Aiel chain `quick → dodge → throw → sidestep`.

65. ~~**Feats that modify cadence.** Make character build choices
    actually move the speed dial.~~ Landed 2026-05-11.
    - Chargen `Feat` entries gain optional speed-modifier
      fields: `weapon_weight_penalty_mul float32` (default 1.0;
      Blademaster = 0.5), `armor_weight_penalty_mul float32`
      (Light Step = 0.5), `stamina_cost_mul float32` (Endurance =
      0.8), `stamina_regen_add int16` (Iron Constitution = +1).
      Catalog validator rejects non-zero `*Mul` outside (0,2] and
      `stamina_regen_add` outside [-10,10].
    - `actorActionCost` walks `core.Feats` (already `[]int32` —
      FNV-32a hashes via `chargen.HashID`) through a new
      `Catalog.FeatByHashedID` reverse map built eagerly at the
      end of `chargen.Load`. `FeatModifiers` aggregate stacks
      the per-feat `*Mul` fields multiplicatively and
      `StaminaRegenAdd` additively in a single resolver call.
      `ApplyFeatGearAttenuation` rewrites each gear factor as
      `1 + (factor-1) * mul` when `factor > 1.0` so the feat
      attenuates the *penalty portion* without rewarding
      already-light gear. Pure-fn tests in
      `internal/combat/featmod_test.go` pin the math.
    - Stamina sides fold the aggregate too: `drainStamina`
      multiplies its computed cost by `StaminaCostMul` (rounded
      to nearest, floored at 0); `StaminaTicker` adds
      `StaminaRegenAdd` to the base before
      `EffectiveStaminaRegen` so Iron Constitution stacks
      correctly with the heavy-armor halving (mirrors the
      score-sheet's order so the live value and the rendered
      hint always agree).
    - New seed feats in
      `internal/chargen/default/feats.yaml`:
      `feat_blademaster`, `feat_light_step`, `feat_endurance`,
      `feat_iron_constitution`. Prerequisites V1: none
      (mirrors current chargen feat stance). All
      `background: false` (general 1st-level picks).
      `feat_two_weapon_grace` deferred — no off-hand swing in
      tickRoom yet (no consumer for `off_hand_cost_mul`).
    - `score` sheet's Combat line cites active feat
      contributors: `Combat: 0.97x (0.75 weapon × 1.30 armor)
      [Blademaster]`. Both `score` and the combat hot path go
      through `combat.ResolveFeatModifiers` so the rendered
      bracketed list reflects exactly what's firing in resolve.
    - Combat Manager gains `cat *chargen.Catalog` field +
      `SetCatalog` setter; `cmd/server/main.go` wires it after
      construction. `StaminaTicker` constructor takes the catalog
      as a new parameter (nil-safe for combat-only tests).
    - **Slice exit (verified):** Blademaster on a greatsword
      brings a normal swing from 4.5s to 3.75s while still
      paying the gear baseline; Iron Constitution renders
      `(+3/pulse)` over a base of 2; full suite race-clean.

66. ~~**Iterative attacks via cadence drain.** Replaces the D&D
    3.x "+6 BAB gives a second attack at -5" mechanic with
    cadence math.~~ Landed 2026-05-11.
    - `ActorEntry` gains `PendingSwings int` and
      `IterativeBonuses []int16` (e.g. `[0, -5, -10]` for a
      BAB+11 fighter). Computed at `Start` from `core.BAB`.
    - `tickRoom`'s drain loop, instead of running once per
      ready actor, runs `1 + min(IterativeCount, queued
      actions)` times back-to-back for the same actor, applying
      the iterative bonus to each successive `RollAttack`.
      Costs accumulate so `NextActAt` is set after the *last*
      swing, not the first.
    - Stamina drains per swing; the iterative chain truncates
      when stamina runs out (matches the "you're winded after
      the fourth swing" feel without a separate gate).
    - **Slice exit:** high-BAB fighters get the expected
      multi-attack bursts on their turn rather than waiting
      out 3 buckets in a row.

67. **Write-coalescing + compact echo mode.** Production-load
    polish. Optional — only needed once content fills the
    server with simultaneous fights.
    - `persist.Manager` dirty-bit aggregate for HP / Stamina /
      Conditions on `Character` and `MobInstance`. Combat
      `resolveAction` marks the row dirty in-memory and the
      Manager flushes at its own cadence (default 2 s) instead
      of one UPDATE per swing.
    - New `combat brief` player setting (persisted via the
      account-settings JSON from migration 0035). When set,
      `internal/cmd/server/main.go`'s combat-echo subscribers
      collapse runs of consecutive misses into a single
      "you swing wildly (×3)" rollup keyed off
      `Session.lastCombatEcho`. Hits, crits, and damage taken
      always render.
    - Prompt template `%t` (in-combat) reserved in §2 wires
      to a compact `HP|SP|Cooldown` gauge.
    - **Slice exit:** a 20-fight stress test holds
      sub-millisecond per-swing CPU and the combat log of a
      single character stays under one screen of text per
      round.

After L: combat feels like real-time melee with meaningful
distinctions between fighter archetypes. Aiel skirmishers,
plate-armored heavies, and named-feat exceptions like Lan all
have mechanically supported playstyles.

---

## Phase M — Anti-abuse, UX polish & content surfaces

Phase M is the cross-cutting "make the live game feel intentional"
track. None of the slices are mechanically required to play, but
each one closes a class of operator pain (long boot scripts, hand-
maintained help, abusive input, missing roleplay verbs). Slices are
shippable independently and have no internal dependencies beyond
M.0 prepping the file layout.

Direction lock-in: stay additive. Phase M doesn't refactor combat
or chargen; it adds catalogs (help, emote), filters (visibility),
and rate-limits (BadInputTracker, FloodContext). The work it would
*want* to refactor (character_create.go, combat/manager.go) is
deferred to whatever phase finally adopts a Flow engine /
EffectsManager generalization.

M.0. ~~**Split `cmd/server/main.go` into per-concern siblings.**
    Pure refactor, no behavior change.~~ Landed 2026-05-17.
    - main.go drops below the 800-line guideline; new siblings:
      `registry.go`, `mssp.go`, `adapters.go`, `audit_metrics.go`,
      `catalog_validate.go`, `lua_bindings.go`,
      `subscribers_combat.go`, `tickers.go`,
      `bootstrap_observability.go`, `shutdown_admin.go`.
    - `cmd/server/main.go` becomes a thin orchestrator wiring
      config → DB → repos → catalogs → registry → scheduler.
    - Followups (none).

M.1. ~~**HelpService with HELP_DIR override + auto-generated command
    topics.**~~ Landed 2026-05-18.
    - `internal/help` exposes `Catalog`, `Load`, `LoadFS`,
      `MergeGenerated`. `SourceFS()` honours `HELP_DIR` so
      operators can edit Markdown topics without rebuilding.
    - `cmd.GenerateCommandTopics(registry)` walks every
      registered `*telnet.Command` and emits a topic per
      `Command.Help`/`Long`. Aliases become topic keywords;
      authored topics win on collision (`MergeGenerated` skips
      generated stubs when an authored topic claims the id).
    - Followups (none).

M.2. ~~**BadInputTracker + FloodContext + VisibilityFilter.**~~
    Landed 2026-05-18.
    - `telnet.BadInputTracker` counts unknown/refused verbs per
      session and silently throttles the response after a burst
      budget. Removes the timing side channel on privilege-denied
      verbs (`code_review_open_items.md`).
    - `telnet.FloodContext` is consulted by `Session.WriteRaw`
      so a runaway broadcast loop can't drown a peer's pipe.
    - `internal/visibility.CanSee` centralizes the wizinvis rule
      that had drifted across half a dozen callsites. Closes the
      "wizinvis-silent for say/shout/channels" followup
      (`admin_movement_followups.md`).

M.3. ~~**EmoteRegistry — YAML social-verb catalog + freeform
    `emote`.**~~ Landed 2026-05-18.
    - `internal/emote` catalog mirrors the effects/chargen/help
      pattern (`default/socials.yaml` embedded, `EMOTE_DIR`
      override, fail-loud Load).
    - `cmd.NewSocials` emits one `*telnet.Command` per entry;
      targeted forms render the three-way actor/target/others
      split. Mob targets resolve via `MatchMob` (same precedence
      as `attack`) so `smile scout` works.
    - `cmd.NewEmote` (alias `:`) is the freeform escape hatch.
    - Followups: `socials` listing verb, hot-reload, cooldowns
      (none of which are blockers).

M.4. ~~**MSSP (Mud Server Status Protocol).**~~ Shipped earlier as
    Phase I #45 (2026-05-14); the §M.0 main.go split (2026-05-17)
    moved the existing code into `cmd/server/mssp.go` as a sibling
    file, which created the appearance that MSSP was Phase M scope
    when it was just a refactor of code that already negotiated,
    encoded, and served the full 27-variable block.
    Closed-on-arrival in §M.4 (2026-05-18) with a doc + audit pass:
    new MSSP sections in `docs/CODEMAPS/telnet.md` and
    `docs/CONVENTIONS.md`; ROADMAP §1 already records the landing.
    - Audit findings (deferred polish, not bugs):
      - Wizinvis admins are counted in `PLAYERS`. Crawlers see
        N + admins instead of the public-facing N.
      - `FAMILY` / `GENRE` / `GAMEPLAY` / `LANGUAGE` / `CODEBASE`
        are hardcoded literals in `msspVars`. A fork rebranding the
        codebase would have to recompile. Move to `MSSPConfig`.
      - `CRAWL DELAY: -1` is hardcoded; spec interpretation varies
        across crawlers. Make config-driven with a sane default.
      - No admin `mssp` verb to dump the variable block for
        operator inspection without an actual crawler.
      - `collectMSSPWorldStats` is a boot snapshot — a future
        OLC slice that adds rooms/zones at runtime will silently
        drift the counts. Invalidate-on-save or move to live
        counts when that lands.

M.5. **`socials` listing verb.**
    - Player-facing dump of the emote catalog grouped by
      targetable / untargeted. Reads `emote.Catalog.All()`; no
      new wiring beyond a registration line.
    - Blocked on: nothing.

M.6. **Hot-reload for socials + help.** Admin-only.
    - `reload socials` re-runs `emote.Load(SourceFS())` and
      replaces the catalog atomically. `reload help` does the
      same for `help.Catalog`. Re-registers any new social
      commands and de-registers removed ones.
    - Blocked on: a clear story for safely mutating
      `telnet.Registry` after boot. Needs a small
      `Registry.Replace` or equivalent — the current API only
      supports `Register`.

M.7. ~~**Decide §G #34 slice 3 (oedit / medit / zedit) —
    close-or-defer.**~~ Decided 2026-05-18: **Option A**.
    §G #34 slice 3 stays deferred pending Phase N #N19 (Flow
    engine). Rationale: a generic multi-step Flow framework
    absorbs oedit/medit/zedit as YAML/Lua flow definitions and
    also unlocks chargen / quest dialogue / sustenance UX from
    the same engine — building slice 3 by hand against the
    `redit` pattern would be ~600 LOC of throwaway work if Flow
    lands. ROADMAP §16 and §17 OLC entries updated to reflect
    the deferred-pending-Flow status. Phase G is now in a
    coherent "redit shipped, oedit/medit/zedit waiting on
    Phase O" state instead of half-open.

After M: operators can author content (socials, help) and
moderate behaviour (anti-abuse, visibility) without code
changes. Mud-listing sites discover the server. The §G OLC track
is in a known state instead of half-open.

Closed 2026-05-18: M.0–M.7 all landed or formally deferred.
Phase M is now closed; Phase O (Flow engine, promoted from N.19)
is the upstream unblock for the §G slice 3 backlog.

---

## Phase O — Flow engine (player-creator framework)

Promoted from Phase N #N19 on 2026-05-18 per the §M.7 decision.
Phase O builds a generic multi-step interactive Flow engine
that subsumes:

- §G #34 slice 3 (`oedit` / `medit` / `zedit`) — currently
  deferred-pending-Flow.
- Chargen — today 1,415 LOC of hand-rolled mode code in
  `internal/cmd/character_create.go`.
- Account creation — similar shape, hand-rolled.
- Future content surfaces: sustenance UX, quest dialogue trees,
  weather wizards, anything that wants a prompt-validate-branch
  loop without a recompile-per-step.

Direction lock-in (chosen 2026-05-18; reverse with a follow-up
decision, not silently):

- **Go-only action / validator registry.** Steps reference Go
  callbacks by string key. YAML is data-only; new validators
  need a recompile. Type-safe, fast, no Lua sandbox surface to
  widen for this phase. If Lua-backed validators ever land,
  they're an additive Phase O.x slice — the registry contract
  doesn't have to change.
- **Persisted from O.0.** Flow state lives in SQLite
  `flow_state` keyed by `(account_id, flow_id)`. Resume on
  reconnect from day one. Matches the Tapestry UX where a
  player mid-chargen can log back in and continue. Per-flow
  `Resumable bool` lets ephemeral wizards (e.g. an admin's
  `oedit` session) opt out.
- **Horizontal slicing first.** O.0 ships the engine with a
  test-only `wizdemo` consumer so the API can stabilise before
  any production consumer locks it in. Real consumers
  (`oedit` / chargen / account-create) come strictly after
  O.3 (extended step kinds) and benefit from O.2's persistence.

The slices below are independently shippable but have linear
dependencies: O.0 → O.1 → O.2 are foundational; O.3 adds the
step taxonomy needed for any real consumer; O.4–O.6 close §G;
O.7–O.8 retire the hand-rolled flows; O.9 finishes the §M.6
hot-reload integration.

O.0. **Engine core.** `internal/flow/` package with `Flow`,
    `Step`, `State`, `Runner`. Step kinds: `text`, `choice`,
    `confirm`. Go action+validator registry (`flow.RegisterAction`
    / `flow.RegisterValidator`). In-memory state. Test-only
    harness (no live dispatcher integration yet).
    - ~600–800 LOC.
    - Blocked on: nothing.

O.1. **Mode integration + YAML loader.** `mode.Flow` pushes
    onto the existing mode stack; the dispatcher routes input
    through the active step. Meta-commands `back` / `cancel` /
    `help` apply uniformly. YAML loader under `internal/flow/`
    with `FLOW_DIR` override (mirrors HELP_DIR / EMOTE_DIR /
    CHARGEN_DIR). Live `wizdemo` test verb so the dispatcher
    integration is exercised on a real session.
    - ~500 LOC.
    - Blocked on: O.0.

O.2. **Persistence.** New `flow_state` table + migration; save
    on every step transition; resume on reconnect lookup by
    `(account_id, flow_id)`. Per-flow `Resumable` flag; abort-
    on-disconnect when false. Per-session capacity cap so a
    stuck wizard can't pile up rows.
    - ~400 LOC + migration.
    - Blocked on: O.0, O.1.
    - **Landed 2026-05-18.** Migration 0057 creates the
      `flow_state` table (composite PK `(account_id, flow_id)`,
      `(account_id, updated_at DESC)` index for LRU eviction);
      `repo.FlowStateRepo` (memory + sqlite, shared test suite)
      enforces `MaxFlowStatesPerAccount = 4` with oldest-wins
      eviction at insert time. Engine gained an optional
      `flow.Persister` interface; the Runner Saves after every
      transition and Deletes on Completed/Cancelled. `mode.NewFlow`
      takes a Persister + FlowLoader pair and, when the Flow is
      `Resumable`, hydrates state from the loader and calls
      `Runner.Resume()` instead of `Start()`. `postAuth` fires a
      package-level `flowResumer` hook (paired with
      `loginPublisher`) after `ReplaceMode` to push any
      resumable row on top of the AccountMenu / CharacterCreate.
      `wizdemo.yaml` flipped `resumable: true` to exercise the
      whole path. Engine package stays free of `repo` imports —
      the bridge lives in `cmd/server/adapters.go`
      (`flowRepoPersister`).

O.3. **Extended step kinds.** `number`, `multi-select`,
    `point_buy`, `conditional` (branching on prior step output),
    no-prompt `action` step (run a side effect, advance). Each
    new kind is a separate Step implementation pluggable into
    the runner.
    - ~300 LOC.
    - Blocked on: O.0.
    - **Landed 2026-05-18.** Five new step kinds (`number`,
      `multi_select`, `point_buy`, `conditional`, `action`) land
      one-file-per-kind in `internal/flow/step_*.go`. Compound
      outputs (multi_select, point_buy) JSON-encode under
      `store_as` so `State.Values` keeps its `map[string]string`
      shape. Engine extension: optional `AutoAdvancer` interface +
      `AutoRuntime` accessor; `Runner.advanceTo` walks chains of
      no-prompt steps (conditional, action) up to `MaxAutoChain = 32`
      and aborts with a clear error on cycles. ActionStep resolves
      its `Action` ref against the runner's ActionRegistry via
      AutoRuntime so the step type stays free of runner-internal
      references. YAML loader gained five new switch cases; the
      "unknown kind" error message now lists all eight kinds.
      Per-step table-driven tests (number, multi_select, point_buy,
      conditional, action) plus runner-level chain + cap tests in
      `auto_advance_test.go`. `wizdemo2.yaml` exercises one of each
      new kind and is reachable via the existing `flow wizdemo2`
      admin verb (resumable: true so it doubles as an O.2 × O.3
      integration smoke).

O.4. **First production consumer: `oedit`.** Closes §G #34 slice
    3 part 1. Item-template editor as a flow YAML.
    `ItemRepo.Update` (memory + sqlite + shared test) added if
    not already present. Audited via `oedit` verb + per-field
    `admin_audit` rows.
    - ~200 LOC Go + YAML.
    - Blocked on: O.3.

O.5. **`zedit`.** Zone editor (smaller surface than `medit`).
    Closes §G #34 slice 3 part 2.
    - ~150 LOC + YAML.
    - Blocked on: O.4 (pattern proven).

O.6. **`medit`.** Mob-template editor. §G #34 slice 3 fully
    closed.
    - ~200 LOC + YAML.
    - Blocked on: O.5.

O.7. **Chargen migration.** Rewrite `character_create.go` as
    a flow YAML + the necessary Go actions/validators. Heavy
    regression-test coverage because chargen is on the boot
    path for every new account. Old hand-rolled mode stays in
    a feature flag for one release to allow rollback.
    - ~600 LOC + heavy test.
    - Blocked on: O.3 (point_buy step), O.2 (persistence —
      chargen always Resumable).

O.8. **Account-create migration.** Smaller shape than chargen.
    YAML + Go actions.
    - ~150 LOC + YAML.
    - Blocked on: O.7.

O.9. **Hot-reload: `reload flows` admin verb.** §M.6
    integration. Flow YAML can be hot-reloaded the same way
    socials and help can. New entries register; removed entries
    abort any in-flight resumable sessions cleanly.
    - ~100 LOC.
    - Blocked on: O.2.

After O: oedit/medit/zedit ship without §G #34 slice 3 ever
being hand-rolled. Chargen and account-create are content, not
code. Future content systems (sustenance, weather, dialogue)
have a flow engine to compose against instead of inventing
their own state machine each time.

Phase total: ~3,000–3,500 LOC over 9–12 weeks depending on the
chargen migration test cycle.

---

## Phase N — Strategic / future-scope (Tapestry parity)

Phase N is **not committed work** — it's an option set generated
by reading the Tapestry codebase end-to-end and listing every
system they have that we don't (or do differently). Nothing here
is sequenced; nothing here blocks Phase M. Promote an item out of
Phase N into a real phase when you decide to actually build it.

**Why this list exists:** WheelMUD and Tapestry started from
different design centres — we're SQLite + repo pattern + race-
tested concurrency + strict invariants; they're property-bag +
YAML-first + 45-module JS scripting + OTEL tracing. Both stacks
are coherent. The point of Phase N is *visibility*, not envy: when
a Phase M.x slice could be designed to also close a strategic gap,
knowing about the gap up front avoids painting ourselves into a
corner.

### High-leverage parity items

N.1. **Pack/plugin system.** Drop `data/packs/<name>/` with a
    manifest, load YAML catalogs from there at boot. Lets
    community content land without recompile. Builds naturally
    on the EmoteRegistry / help-catalog / chargen-catalog
    loaders we already have — they're all already `fs.FS`-based.
    Manifest needs: name, version, deps, catalogs (`socials`,
    `help`, `chargen`, `world`, …). No script execution from
    packs in V1 (avoid the Lua-from-packs security boundary).
    Strategic: biggest single content-extensibility win.

N.2. **WebSocket transport + minimal web client.** Biggest UX
    gap vs. Tapestry. Telnet-only is increasingly niche.
    Pattern: parallel listener on `WS_ADDR` that adapts to the
    same `telnet.Session` plumbing (or a sibling Session type)
    so the dispatcher / mode-stack / world-loop are transport-
    agnostic. Browser client renders the same lines telnet would.
    Strategic: opens a much larger player surface.

N.3. **MSSP** — *promoted to §M.4 above.* Listed here for the
    Tapestry-parity diff; track it in Phase M.

N.4. **OpenTelemetry tracing.** GameLoop tick breakdown +
    per-command spans. We already export Prometheus; OTEL is
    the next axis. Useful for hunting long-tail tick stalls
    (combat resolve + Lua callbacks). Jaeger / Loki / Grafana
    stack is well-understood; the WheelMUD-side work is span
    instrumentation in `internal/tick`, `internal/combat`,
    `internal/lua`, plus the dispatcher.

### Content-system items (each could be its own phase)

N.5. **Sustenance / Rest / Consumables.** Hunger, thirst,
    resting recovery, food/drink items. Feeds Phase K crafting
    (cooking, brewing). Reuses `internal/effects` for status
    application. No Tapestry-specific design lock-in needed.

N.6. **Weather + weather zones service.** Today we have phase
    ambients only. A real weather system means typed
    `WeatherState` per zone with transitions, modifiers exposed
    to combat/visibility, and YAML-authored climate profiles.

N.7. **Alignment system.** Ranges + config-driven thresholds;
    knock-on hooks in dialogue, shop pricing, faction reactions.
    Cheap to add once you commit to it; *value* depends on
    whether the game world cares about alignment narratively.

N.8. **Essence / Rarity / Stacking for items.** Item taxonomy
    today has slot/weight/damage but no rarity tier or stacking
    semantics. Tapestry has all three. A rarity field is one
    schema migration; stacking is harder because it interacts
    with the items 3-location invariant.

N.9. **Effects manager generalization.** §E #25 affects is
    task-scoped; Tapestry's effect manager is a system. Generic
    `Effect` with `Apply` / `Tick` / `Remove` hooks; status
    affects, environmental effects, blessing/curse all become
    instances. Touches `combat/manager.go` (881 LOC) —
    co-schedule with that file's split.

N.10. **Class paths / class-path processor for branching
    subclasses.** Today a class is a leaf; Tapestry models
    branches (e.g. Warder → Borderland Warder vs. Two Rivers
    Warder). Schema delta + chargen flow rework.

N.11. **Training / TrainerConfig generalization.** We have a
    `train` cmd; Tapestry has a configured training service
    with cap tiers and prereq trees. Pairs with N.10.

N.12. **Door service + temporary exits.** We have doors; this
    is adding portals / summoned exits / spell-conjured doors
    that live for N ticks. Builds on the existing exit repo
    plus a TTL on a new flag.

N.13. **Portals as a scripting module.** Once N.12 lands,
    Portals = Lua-exposed `summon_portal(room_a, room_b, ttl)`.

N.14. **Mob AI manager + command queue + disposition
    evaluator.** Generalized AI scheduler. Today our mobs do
    paths + combat but not generic command queues. Pairs with
    Lua surface expansion (§F #32 slice 6+).

N.15. **Heartbeat phases (CombatPulse / AbilityResolution /
    Pulse handlers).** Phase-based tick architecture. Ours is
    bucket-based — refactor candidate, not a green-field add.

N.16. **PropertyRegistry — typed dynamic property system.**
    Strategic alternative to "one column per attribute"
    schema. Pros: no migration for every new attribute; YAML
    can author arbitrary props. Cons: loses SQLite's type
    safety + grep-ability. **Evaluate before the next big
    schema add**; do not refactor existing tables onto it
    without a concrete win.

N.17. **Tags as a first-class system with observers.**
    Entities accumulate tags (`flammable`, `magical`,
    `quest_item`); systems subscribe to tag changes. Common
    MUD-engine idiom. Cheap to add, hard to retrofit later.

N.18. **Theme registry — per-player colour themes.** Beyond
    `colors` (which toggles 16/256/truecolor): named themes a
    player picks (`theme dark-luxury`, `theme high-contrast`).

N.19. ~~**Flows / wizard / player-creator engine.**~~ **Promoted
    to Phase O (Flow engine) on 2026-05-18.** See the Phase O
    block below for the full slice list and architecture
    decisions. Kept here for the Tapestry-parity diff only.

### Platform items (likely separate effort)

N.20. **CLI tooling.** `wheelmud init` / `wheelmud install
    <pack>` / `wheelmud start`. Operator UX vs. raw `make
    run/server`. Builds on N.1 once packs exist.

### What we have that Tapestry doesn't (for symmetry)

- SQLite + forward-only migrations + repo pattern with
  sqlite+memory parity. Easier to reason about; easier for
  them to swap backends.
- Race-tested concurrency posture (`safego.Go`, `go test
  -race`, fuzz on IAC/tokenize, integration tag). No equivalent
  race/fuzz harness on the Tapestry side.
- Strict path/BFS mob movement (§32a).
- Group system, channeling driver, quest engine wired to event
  bus — comparable to theirs, leaner here.
- Lockstep schema discipline (CONVENTIONS.md write-paths,
  items 3-location invariant). Tapestry hides this behind
  PropertyRegistry.

**Promotion rule:** when promoting a Phase N item to a real
phase, delete its block here and add it to PLAN under the
phase it landed in. Phase N should only ever hold work we've
*not yet committed to do.*

---

## Sequencing rules

- ~~**Run #52 (CI matrix) right now**, before anything else.~~ Phase J
  (#52–#59) landed 2026-05-12; CI matrix + coverage are live.
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
- **Phase L slices are independently shippable** but order
  matters semantically: #60 (cadence) is the foundation every
  later slice builds on, so it must land first. #61 (variants)
  and #62 (gear-driven speed) are independent of each other and
  can ship in either order, but both should land before #63
  (stamina) because stamina cost values are easier to tune once
  variant + gear math is settled. #64 (new verbs) can land any
  time after #63. #65 (feats) and #66 (iteratives) are
  independent of each other; #65 needs the chargen Feat schema
  fields, #66 only touches combat. #67 (write-coalescing) is
  pure polish — land when load measurements call for it, not
  before.
- **Phase L can run concurrently with Phase F** — they touch
  different packages (combat vs. quest/trigger/Lua) and Lua
  scripts don't currently mutate combat state in ways the
  cadence change would break. The slice-3 Lua surface
  (apply_affect, give_item) stays compatible.

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
