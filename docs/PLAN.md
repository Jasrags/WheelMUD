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
    **Stubbed for follow-ups** (see
    `chargen_features_followups.md`):
    - Two-handed / off-hand / light / quiver auto-equip — slice
      1/2 follow-ups already track these.
    - Cross-class skill picks (half-rate, double cost) — defer
      until level-up needs the same plumbing in §12.

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
    Deferred to slice 2: corpse decay tick + `items.decay_expires_at`
    column, mob inventory transfer into corpse, player death &
    bound-room respawn + XP-debt penalty, mob respawn via §9 zone
    reset, looting verbs (`get from corpse` already works through
    existing container plumbing — no new verb needed).
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

23. **Levels & XP curve** (§12). Reads the §12 class / level table;
    awards feat slots, skill points, ability bumps, weave slots on
    train.
24. **Mid-game skill rank investment** (§12). Per-character
    `character_skills` writes; respects class-skill / cross-class
    caps from the chargen catalog.
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

29. **Trigger / event system** (§15) — `on_enter`, `on_say`,
    `on_attack`, `on_death`, `on_tick`. Pure dispatch layer; consumers
    in 30–31. **Landed 2026-05-08.** Migration 0044 added the
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
    - Slice 4 (sketch): combat mutations (deal_damage / heal —
      blocked on §D crit polish), inventory take/transfer, room
      state iterators (room.players / room.mobs), `wait(seconds,
      fn)` async scripts, `on_login` / `on_logout` events.
    - Slice 5: OLC `tedit` (depends on Phase G).

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
