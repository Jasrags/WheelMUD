# Wondrous Items

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 14: Wondrous Items, pp. 290–299) for use in WheelMUD implementation.

## Overview

Two distinct item families predate the modern era and survive in small numbers:

| Family | Function | User |
|--------|----------|------|
| Angreal / Sa'angreal | Amplify a channeler's raw power | Same-gender channeler only |
| Ter'angreal | Use the One Power directly to deliver a specific effect | Varies — channelers, anyone, or both |

A third class of pre-Breaking Age-of-Legends technology (glowbulbs, shocklances, transcribers) exists alongside them as ordinary equipment.

## White Tower Claim

The Aes Sedai assert ownership over **all** angreal, sa'angreal, and ter'angreal anywhere their authority reaches. Demands are made without negotiation or compensation. Practical resistance: the Stone of Tear's Great Holding, defiant rulers, anything outside the Tower's reach.

## Angreal and Sa'angreal

### Universal Rules

- **Power Rating** 1–10. `1–3` = angreal; `4–10` = sa'angreal.
- The rating is the number of weave-slot levels the device adds **without** triggering an overchannel Fortitude save (see `the-one-power.md`). The user must still have a slot to fill (unless the user wants to overchannel without a slot, in which case standard overchannel rules apply on top).
- **Gender attunement** is permanent. Cross-gender users perceive the item as ordinary. Same-gender channelers can identify it by touch.
- **Stacking:** only the **most powerful** carried item applies; multiples do not stack.
- **Activation cost:** holding/touching is enough during a cast. Use does **not** consume an action unless the entry says otherwise.
- **Durability:** virtually unbreakable. Only an extraordinarily strong/enhanced channeler or huge physical force can damage one.
- **Overchannel ordering:** apply the device's free levels first, then evaluate further overchannel risk against the remaining gap.

### Sample Pieces

| Item | Power Rating | Attunement | Notes |
|------|-------------|-----------|-------|
| Statuettes (generic) | 1–2 (sometimes 3) | Either (figure's gender) | Most common form. ~8 in. ivory or stone. Examples: Moiraine's `2`, Aviendha's `1`, Verin's `1`, Rand's lost `3` (bald fat man with sword across knees). |
| Amber Turtle brooch | 2 | Female | Disguised as ordinary jewelry. From the Ebou Dar trove. |
| Golden Ring | 3 | Female | Held by Graendal as a surprise reserve. |
| White Wand | 7 | Female | The White Tower's strongest. Reserved for the Amyrlin Seat. |
| Callandor (Sword That Is Not A Sword) | 8 | Male | Crystal two-handed blade. **Has no buffer** — direct overchannel-style risk and sanity threats unless **linked with two women** (per Cadsuane's claim). Originally shielded inside the Stone of Tear. |
| Choedan Kal — Crystal Globe statues | 10 (paired) / 9 (solo) | Paired (one male statue near Cairhien, one female on Tremalking) | Used **together** by a man and a woman, can shatter the world. Direct touch is fatal — operated through paired miniature statuette **ter'angreal** that Rand al'Thor recovered in Rhuidean. Channeler < 10th level using one solo: Fort DC 30 or **5d20 dmg/round** while connected. |

## Pre-Breaking Technology

| Item | Function | Mechanics |
|------|----------|-----------|
| **Glowbulb** | Permanent light source; bright/clean/steady/cool | Vary intensity & color via touch + thought |
| **Shocklance** | Energy weapon | Range increment 150 ft; 4d10 dmg; threat 16–20; ×4 crit; Exotic Weapon Proficiency (shocklance) required (only acquired by trial-and-error). 8 charges; each charge regenerates after 4 hours |
| **Transcriber** | Tireless dictation/printer | Black-and-white or color variants; never runs out of power, paper, or ink |

## Ter'angreal

### Universal Rules

Each entry has six common fields:

- **Activation** — what action triggers the effect. Categories:
  - **Enter** — pass through (anyone may use unless restricted).
  - **Touch** — direct skin contact; effect lasts while contact persists.
  - **Wear / Carry** — must be worn appropriately (necklace round neck, etc.) or carried; effect lasts while worn/carried.
  - **Weave Sacrifice** — channeler spends a weave slot at the listed level; channeler-only.
  - **Wield** — used per the description; anyone unless restricted.
- **Affinities** — for weave-sacrifice ter'angreal: lacking all listed → bump slot up by 1; having all → drop slot by 1; partial → use listed slot.
- **Size** — Tiny / Small / Medium / Large / Huge (per `equipment.md`).
- **Weight**.
- **Occurrence** — `Common` (hundreds in existence with minor design variation), `Rare` (several variants), `Unique` (one only).
- **Default action cost** — using a ter'angreal is an attack action unless noted.
- **Persistence** — ter'angreal are nearly unbreakable; many are permanent fixtures rather than carried.

### Crafting

- The art was lost for centuries; only recently relearned by Elayne Trakand. Wider replication is mostly aspirational — keep ter'angreal scarce in play.

### Catalog

#### A'dam (Common, female-channeler activation)

- Wear (channeler only).
- Silver collar + bracelet linked by a metallic leash.
- Latched onto a target channeler: she cannot remove the collar (any attempt: 1d6 subdual + excruciating pain). Anyone except a male channeler can latch/unlatch.
- Sul'dam free-action: deal `1d6 subdual`; target Will DC 20 or **writhe helpless** that round.
- If the bracelet is set down somewhere, the leashed channeler cannot move it; trying triggers `1d6 subdual/round` and the same Will save.
- Sul'dam are simply latent female channelers; a leashed sul'dam suffers the same effects.
- **Cross-gender backlash:**
  - Male channeler **touches** an a'dam while a female channeler wears either part → both take **2d6 dmg**.
  - Male channeler **wears** one half while female channeler wears the other → **2d6 dmg/round to both**.
- A non-channeler wearing or touching it does nothing and grants no effects.

#### Amulet of Alertness (Rare, channeler-only)

- Wear.
- **+2 resistance** vs. anything that induces sleep, drowsiness, false security, or dulled perception.
- Warms when someone is spying or eavesdropping (no direction or identity, just presence).

#### Three Silver Arches (Rare, anyone)

- Enter; Huge; 800 lb; in the White Tower.
- Three silver arches, walked through in order.
- Confronts the candidate with worst fears (was / is / shall be).
- Will DC **10 / 12 / 14**. Failure → collapse, fail the test (typically barring Aes Sedai advancement).
- Mirror exists in Rhuidean for Wise Ones.
- **One-shot per lifetime** — second pass has no effect.

#### Tear Archway (Unique, anyone)

- Enter; Huge; 200 lb; Great Holding of Tear; carved with three sinuous lines.
- Leads to a yellow-clad realm believed to be the **Aelfinn**.
- Carry no lamps, torches, instruments, or iron — violation forcibly returns the seeker permanently.
- Three honest answers to three questions (any subject, any time).
- Frivolous questions → punishment (extended bad luck or similar non-lethal harm).
- Shadow questions risk madness or attracting the Dark One's notice; questions about other people may not be answered truthfully and may entangle the seeker in their fate.
- **One-shot per lifetime.**

#### Rhuidean Archway (Unique, anyone)

- Enter; Huge; 200 lb; carved with three lines of triangles. Removed from Rhuidean to the White Tower after the city was reopened.
- Leads to a realm of beings believed to be the **Eelfinn**.
- Three things asked-for and granted (not always as the seeker imagined).
- Always exacts a high price; wise seekers negotiate it. GM-discretion price tends to be intangible (creativity, capacity to love, future obligation).
- **One-shot per lifetime.**

#### Balefire Rod (Rare, channeler)

- Weave sacrifice **2nd level**; Affinities Air, Earth, Fire, Spirit, Water; Small; 4 lb.
- Produces **balefire** as a 9th-level cast.
- Each round of use: **Fort DC 18.** Fail → 1d6 dmg from the rod. Success → 1d6 subdual. Stolen by the Black Ajah.

#### Bell of Far Alarm (Unique, channeler)

- Weave sacrifice (variable); Spirit; Large; 15 lb. Bronze-like, lighter than its size.
- Concentration DC 16; failure wastes the slot and locks out for 24 hours.
- Rings silently in the **minds** of designated targets (any chosen scope: village, kingdom, faction).
- Range = `100 mi × slot level` (or 10 mi for 0-level).
- Wakes sleepers; conveys identity & location of the ringer; does not compel.

#### Bird Statuette (Rare, channeler)

- Weave sacrifice 0-level; Spirit; Tiny.
- See through one bird's eyes (must see the bird; no control of direction).
- Lose-of-sight breaks the link.
- Switching birds: free action + Concentration DC 15.

#### Bowl of the Winds (Unique, channeler)

- Weave sacrifice (variable); Air, Water; Medium; 2 lb. Crystal bowl ~2 ft across, cloud-pattern interior.
- Changes weather beyond any standard weave.
- **1st-level slot** = shift weather one season; lasts **1 day**, **5-mi radius** baseline.
- **3rd-level slot** = shift to opposite season; same baseline.

##### Bowl Add-Ons (each adds to the slot level)

| Duration | Slot levels |
|----------|-------------|
| 1 week | +1 |
| 1 month | +2 |
| 1 season | +3 |

| Area | Slot levels |
|------|-------------|
| 10-mi radius | +1 |
| 50-mi radius | +2 |
| 250-mi radius | +3 |
| 1,250-mi radius | +4 |
| Worldwide | +5 |

#### Cat Statuette (Rare, anyone)

- Carry; Tiny.
- **+1 enhancement Dex**, **+10 competence Move Silently**.

#### Circlet of Karim Tay (Unique, channeler)

- Weave sacrifice 1st level; Fire; Small. Twisted green stone armband / circlet.
- Bolts of green fire: **3d10 dmg**, range 100 ft, ranged touch attack.
- After each bolt: **Fort DC 16.** Fail → 1d3 dmg. Success → 1d3 subdual.

#### Crystal Spar of Temarhwin (Unique, anyone)

- Wield; Small; ivory rod with blue-green crystal head.
- 25-ft radius wholesome light.
- **4d6 dmg/round** to any Shadowspawn in the light (Fort DC 20 half).
- Last known carrier: Maraela Sedai (lost in the Blight 18 yr ago).

#### Dagger of Resistance (Unique, anyone)

- Carry; Tiny.
- **+4 resistance** on all saves vs. weaves.

#### Dream Ring (Rare, channeler)

- Weave sacrifice 0-level; Spirit; Tiny. Twisted-stone arm-ring.
- Worn while sleeping → enter `Tel'aran'rhiod` (`other-worlds.md`).
- Many variants in different shapes; some non-channeler-usable.

#### Ebon Scepter (Unique, channeler)

- Weave sacrifice 1st level; Fire; Small. Always feels hot.
- Channel Fire → create elaborate visual + auditory illusions up to 30 ft diameter.
- Touch reveals; Will DC 20 to disbelieve.
- **Concentration DC 18** to avoid backfire. Failure: scepter takes over the user's mind. She passes out; wakes `1d6+3 hours later` after broadcasting illusions of her dreams/fantasies for nearby observers.

#### Fancloth Loom (Unique, channeler)

- Weave sacrifice 1st level; Air, Spirit; Huge; 125 lb.
- Each use produces enough fancloth for **3 Warder cloaks**. **3 uses/day cap.**

#### Five Leaves Folded (Rare, anyone)

- Wield; Tiny.
- Touch to open the metal-cloth-flexible "leaves," revealing a flower of carved/painted ivory.
- Fills the room with sunlight, flower scent, and springtime feel for **3 hours** or until re-touched off.
- Once per day; daylight only (auto-deactivates at nightfall).

#### Foxhead Medallion (Unique, anyone)

- Wear; Tiny. Mat Cauthon's gift from the Eelfinn realm.
- Wearer **immune to all direct One Power effects** (helpful or harmful — including healing, Air-flow, illusion, balefire).
- Indirect effects pass (hurled rocks, set-fire-to-the-building).
- Goes ice-cold when anyone embraces the Source within 30 ft, alerting the wearer.

#### Hogarn Medallion (Rare, anyone)

- Weave sacrifice 1st level; Fire, Spirit; Tiny.
- Anti-Darkhound beam: **4d10 dmg** vs. Darkhounds, Fort DC 16 half. Range 50 ft, auto-hit. **One beam per round.**

#### Jenasa's Golden Key (Unique, channeler)

- Wield; Tiny.
- Equivalent of **sa'angreal power 6** for **Use Portal Stone** weave.
- Equivalent of **sa'angreal power 4** for any other Traveling weave.
- **+2 competence** on Concentration checks for Traveling weaves.

#### Lapis Sphere (Unique, channeler)

- Weave sacrifice 0-level; Spirit; Small; 2 lb.
- Concentration DC 18. Failure wastes the slot.
- Success: shows scenes of distant places or distant times in the real world (named target within `200 mi × slot level` and/or `100 yr × slot level`; or `20 mi`/`10 yr` for 0-level).
- **+1 slot level** to view `Tel'aran'rhiod`. Targeting a person in `Tel'aran'rhiod` requires them to actually be there.
- No sound. Read Lips skill applies.

#### Mask of Illusion (Unique, channeler)

- Weave sacrifice 0-level; Spirit; Small; 1 lb.
- Acts like the **Disguise** weave at casting level 2 (major change to self).
- **-2 to Will saves** to see through the illusion.
- Conceals the mask itself.

#### Medallion of Distraction (Rare, anyone)

- Wear; Tiny. Steel medallion, geometric etchings.
- Anyone looking directly at it (including frontal melee opponents): **Will DC 18** or **-4 to all checks/attacks/Defense** from the distraction.

#### Oath Rod (Unique, channeler)

- Wield (channeler only); Small; 2 lb.
- Oaths sworn on it bind a channeler "bone deep."
- Breaking a sworn oath: **Will DC 30.** Success → may break it but **2d6 subdual + extreme discomfort.**
- Channeler can use the rod to **release** another's oath.
- Stilling a sworn-on woman frees her from her oaths.
- Affects channelers only. Binding a non-channeler requires a **Binding Chair** (no known surviving examples).
- Possibly one of the lost **Nine Rods of Dominion**.

#### Penara's Buckle (Unique, anyone)

- Wear; Tiny; gold belt buckle that resizes to its wearer.
- Reduces nutritional needs to **one good meal + one cup of water per week.**
- Beyond that: standard starvation/thirst rules (`encounters.md`).

#### Talisman of Growing (Unique, channeler / Ogier-singer)

- Wield; Small; 1 lb. Crystal seed-of-bean shape; nested seeds visible inside.
- Activated by singing.
- Grows a new **Waygate** opening into the Ways (`other-worlds.md`).
- **Perform (singing) DC 5.** Treesinger feat: +5.
- Process takes **10 hours**, minus 15 min per point of margin over the DC.
- Each 15-min interval: **2 subdual dmg** (Fort DC 15 reduces to 1).
- If the user is knocked unconscious before completion, the Waygate fails and the entire process must restart.
- Ogier have not used it since the Ways were corrupted in the War of the Hundred Years; kept hidden in an undisclosed stedding.

#### Twelve Rings of Glass (Unique, anyone)

- Touch; Small; 5 lb. Heavy interlocking glass rings.
- Looking at it >1 round: **-1 to all rolls for 1d2 hours** (headache).
- For the bearer: **+2 competence** Concentration to **join a link** (and any subsequent channelers joining while the bearer is in the link).
- Treats the link as if **two additional women** had joined.

#### Zarinda's Rod of the Waves (Unique, channeler)

- Weave sacrifice 0-level; Water; Small; 2 lb. Turquoise rod ~2 ft.
- **+1 casting level** to any Water-Affinity weave.
- Summon fish: any chosen species (or all) within 100 ft of the rod respond instantly.

### Portal Stones (no formal classification)

- Pillar of gray stone, 3 ft thick × 20 ft tall, deeply carved with diagrams and runes. Some stand in ruins, some alone.
- Activated by the **Use Portal Stone** weave.
- Top-half markings: **Mirror Worlds**. Bottom-half markings: **other Stones in the same world**.
- **Knowledge (arcana)** to interpret: GM sets the DC by destination — **DC 15** for known Stones; **DC 25+** for exotic locales / Mirror Worlds.

## Implementation Notes (WheelMUD)

- **Item-class hierarchy:** model `Angreal`, `SaAngreal`, `TerAngreal` as subtypes of a `WondrousItem` interface. Shared fields: `id`, `name`, `description`, `attunement: Gender|None`, `occurrence: Common|Rare|Unique`, `breakable: false (default)`, `useAction: AttackAction|Free|Custom`.
- **Power-rating engine:** `WondrousAmplifier` interface (angreal/sa'angreal) carries `powerRating: int`. The casting pipeline (`the-one-power.md`) consults the **best** amplifier in inventory and bumps the effective slot level by `powerRating` for free; only excess overchannel triggers Fort saves.
- **Stacking rule:** when scanning amplifiers, return `max(powerRating)` rather than summing.
- **Gender lock:** before any amplifier effect applies, check `attunement === user.gender`. Failed match makes the item appear ordinary on touch (`identifyByTouch()` returns `null`).
- **Action-cost defaults:** holding an amplifier is free; using a ter'angreal is an attack action unless overridden by `useAction`.
- **Ter'angreal activation enum:** `Enter | Touch | Wear | Carry | WeaveSacrifice | Wield`. Each maps to a different listener:
  - `Enter` ↔ room-traversal hook.
  - `Touch` ↔ `onSkinContact` listener with persistent effect.
  - `Wear` / `Carry` ↔ equipment-slot or inventory predicate.
  - `WeaveSacrifice` ↔ consumes a slot from the channeler's per-day pool with affinity-adjustment per Chapter 9.
  - `Wield` ↔ in-hand equipped state.
- **Affinity adjustment for ter'angreal:** the same `±1 slot level` adjustment used by weaves applies to weave-sacrifice ter'angreal. Reuse the existing affinity helper.
- **One-shot lifetime arches:** the Three Silver Arches, Tear Archway, and Rhuidean Archway each track a per-character `usedThisLifetime` flag. Re-attempts silently no-op.
- **Aelfinn / Eelfinn scenes:** Tear and Rhuidean archways spawn scripted sub-encounters (see `other-worlds.md`). Tear archway enforces the iron/light/instrument prohibition by gear-scan on entry; violators get bounced and permanently barred.
- **Choedan Kal:** model the two giant statues as world-fixed objects with paired miniature statuette ter'angreal. Solo use of one giant by a sub-10th-level channeler triggers a Fort DC 30 save and 5d20/round damage tick.
- **Callandor:** instantiate as a `SaAngreal` with `powerRating=8` and a `requiresLink: { men: 1, women: 2 }` flag. Without the link, expose a `sanityRoll` hook that fires when high-power weaves are cast and trips Madness escalation (`gamemastering.md`).
- **A'dam:** unique state object with `wearer: NPC|null` and `latched: NPC|null`. The free-action sul'dam attack is a per-round move; subdual damage routes through the standard subdual counter (`combat.md`). Cross-gender backlash and the bracelet-set-down auto-pain are listener hooks. Apply `2d6` to both bearers when male channelers interact.
- **Foxhead Medallion / Dagger of Resistance:** these intercept the One Power resolver. Foxhead returns `immune: true` for direct effects (skipping save), but indirect physics-based attacks proceed normally. Dagger of Resistance simply adds `+4 resistance` to weave saves — same bonus stack as Resistance (`gamemastering.md`).
- **Bowl of the Winds add-on table:** treat as a single `cast(slotLevel, options{ duration, area, polarity })` function that requires `slotLevel ≥ baseline + Σ adjustments`.
- **Bell of Far Alarm:** dispatch a server-wide ping to a target predicate (background-, kingdom-, or person-list-derived). Sleepers wake; recipients learn ringer-id and ringer-coords; no auto-action.
- **Bird Statuette:** scope-driven camera. Subscribe to the bird NPC's perception stream; on `out-of-LOS` event, drop the link.
- **Talisman of Growing:** scripted Perform check + 10-hr-progress timer with subdual ticks; on KO, abort and restart. On success, instantiate a new Waygate node in the Ways subgraph (`other-worlds.md`).
- **Twelve Rings of Glass:** wires into the Linking aggregate (`the-one-power.md`) with a `+2` Concentration on join checks and a virtual-extra-women count of `+2`.
- **Lapis Sphere / Bird / Distant Eye / Eavesdrop overlap:** all are remote-perception primitives. Standardize on a `RemoteSenseRequest` record with `mode: Sight|Sound|Both`, `radius`, `target`, `realm: Real|Tel'aran'rhiod`. The Lapis Sphere additionally takes `pastTimestamp`.
- **Pre-Breaking tech (glowbulbs, shocklances, transcribers):** non-One-Power items. Glowbulb is a permanent room/item light source with adjustable color/intensity. Shocklance integrates into the weapon table (4d10, 16-20/×4, 150-ft increment, 8 charges, 4-hr per-charge regen). Transcribers are scripted "tireless scribe" NPCs in object form.
- **Portal Stones:** registry of `(stoneId, location, edges: { topHalf: [(symbol, mirrorWorldId)], bottomHalf: [(symbol, stoneId)] })`. Symbol interpretation reuses `Knowledge (arcana)` resolver with destination-tier DCs (15 known / 25+ exotic). The Use Portal Stone weave (`the-one-power.md`) consumes the chosen edge.
- **White Tower claim system:** flag wondrous items with `whiteTowerClaim: bool` and tag NPC Aes Sedai with a `demandWondrousItems` behavior; trigger reaction encounters when a PC carries one openly in Aes Sedai jurisdiction (`the-westlands.md`).
- **Crafting:** ter'angreal creation is an `Elayne-only` recipe — gate behind specific high-level character scripts; do not expose as a generic Craft check.
- **Persistence & rarity:** `Unique` items live in a singleton inventory ledger (only one in the world); `Rare` items track a soft cap; `Common` items can be spawned ad-hoc by GMs. Loss/destruction events update the ledger.
- **Activation cost auditing:** because most ter'angreal default to attack-action use, expose an admin lint that warns when a designed effect ignores action cost or applies passively without a wear/carry tag.
- **Backlash hooks:** Balefire Rod, Circlet of Karim Tay, Ebon Scepter — all share the `usePerRoundFortSave` pattern (Fort DC X → 1dN dmg on fail; 1dN subdual on success). Implement once, reuse across items via parameters `{ saveDC, dmgDie, subdualOnSuccess }`.
- **Stedding interaction:** any ter'angreal that requires the One Power (i.e. `Activation == WeaveSacrifice`, or any item whose effect channels) goes inert inside a stedding (`other-worlds.md` `noOnePower` flag). Foxhead Medallion's "ice-cold on embrace" alert is also inert there.
- **Saidin taint linkage:** ter'angreal made by male Aes Sedai during the Breaking (Ways and the Talisman of Growing) inherit corruption flags. New Waygates created via the Talisman now carry the same Machin Shin and Shadowspawn-encounter risk.
