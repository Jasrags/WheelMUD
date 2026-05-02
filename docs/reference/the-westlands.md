# The Westlands

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 12: The Westlands, pp. 260–281) for use in WheelMUD implementation. The chapter is mostly setting prose; this document captures the rules-relevant scaffolding (calendars, kingdom symbols and rulers, governance structures, distinctive laws and customs, geography, threats) that drive world-data in the MUD.

## Calendars

Three calendars in succession have been used in the westlands:

| Calendar | Era marker | Origin |
|----------|-----------|--------|
| Toman | `AB` (After the Breaking) | Established ~2 centuries after the death of the last male Aes Sedai. |
| Gazaran | `FY` (Free Year) | Established after the Trolloc Wars destroyed many records. Year 1 is disputed (≈ 1000+ AB). |
| Farede | `NE` (New Era) | Devised by the Atha'an Miere. Most scholars equate `FY 1135 = 1 NE`. |

- The setting "present day" used by the book is `998 NE` — when Rand al'Thor leaves Emond's Field.
- Older dates become progressively less reliable. Treat AB ↔ FY ↔ NE as a one-way conversion table; world dates should be stored in NE internally.

## Historical Eras (anchor points)

| Era | Highlights |
|-----|-----------|
| Age of Legends | Saidin untainted; co-ed Aes Sedai; cities with antigravity tech (`sho-wings`, `jo-cars`); Hall of the Servants; First Among Servants. |
| Drilling of the Bore | Researchers from the Collam Daan punched into the Dark One's prison; Sharom destroyed; "Collapse" (~100 yr decay) follows. |
| War of the Shadow / War of Power | ~10 yr; Lews Therin Telamon "the Dragon" leads forces of Light; balefire discovered, then voluntarily abandoned. |
| Sealing of the Bore | Lews Therin + 113 Hundred Companions seal the Bore, trap 13 Forsaken with the Dark One; backblast taints saidin. |
| Breaking of the World | 239–344 yr (records differ); male Aes Sedai go mad and reshape continents until the last male channeler dies. |
| Compact of the Ten Nations | 209 AB onward, ~800 yr alliance: Aelgar, Almoren, Aramaelle, Aridhol, Coremanda, Eharon, Essenia, Jaramide, Manetheren, Safer. |
| Trolloc Wars | ~1000 AB, ~350 yr; Manetheren falls; Maighande victory ends them. |
| Time of the High King | Artur Paendrag Tanreall ("Hawkwing") consolidates the westlands by FY 963. Bounty on Aes Sedai placed FY 975; siege of Tar Valon. Dies FY 994 of fever. |
| War of the Hundred Years | FY 994 → FY 1117. Empire fragments into the modern fourteen nations. |
| Aiel War | 976 NE: Laman Damodred fells Avendoraldera. 978 NE: Battle of Tar Valon (Bloody Snows). Aiel kill Laman, retreat. |
| Modern day | ~998 NE; Dragon Reborn revealed. |

### Other organizational origins

- **Children of the Light** founded `FY 1021` as itinerant Darkfriend-hunting preachers; militarized over centuries.
- **White Tower / Tar Valon** construction began 98 AB by Aes Sedai with Ogier help, finished 202 AB.
- **Ajah system** present by ~98 AB (informal); modern seven Ajahs by ~300 AB.

## Westland Kingdoms (the fourteen)

| Kingdom | Symbol | Capital | Ruler @ 998 NE | Notes |
|---------|--------|---------|----------------|-------|
| Altara | Golden Leopards | Ebou Dar | Queen Tylin Quintara, House Mitsobar | Loose confederation; dueling culture; marriage knives. |
| Amadicia | Thistle and Star | Amador | King Ailron (Children of the Light dominate via Fortress of the Light) | Channeling outlawed (death penalty possible); Wisdoms rare → Healers are male. |
| Andor | White Lion / Rose Crown | Caemlyn | Queen Morgase Trakand | Daughter-only succession; Daughter-Heir trains in White Tower; First Prince of the Sword commands Queen's Guard. |
| Arad Doman | Sword and Hand | Bandar Eban | King Alsalam Saeed Almadar (elected by Council of Merchants) | King-only; 75% Council vote can depose; trade-driven. |
| Arafel | Three white + three red Roses | Shol Arbela | King Paitar Neramovni Nachiman | Borderland; two-sword warriors; Aes Sedai well-regarded. |
| Cairhien | Rising Sun | Cairhien | (Sun Throne in flux post-Galldrian) | Daes Dae'mar = Game of Houses; precise grid city; Topless Towers. |
| Illian | Golden Bees / Laurel Crown | Illian | King Mattin Stepaneos den Balak (missing → Dragon Reborn now wears Laurel Crown) | Three-way power: King + Council of Nine + Assemblage. Companions = elite guard. |
| Kandor | Red Horse | Chachin | Queen Ethenielle Kirukon Materasu | Borderland; either gender may rule; Council half-commoner. |
| Saldaea | Silver Fish | Maradon | Queen Tenobia si Bashere Kazadi | Borderland; light cavalry; women dueling, dance `sa'sara`, fan hand-speech, knife-throwing. |
| Shienar | Black Hawk | Fal Moran | King Easar Togita | Borderland; "lancers" with shaved heads + topknot; honor/shame culture; Lord Agelmar Jagad at Fal Dara. |
| Tarabon | Golden Tree | Tanchico | King + Panarch (always opposite gender), Assembly | King Andric dead, Panarch Amathera enslaved by Seanchan; foundered FY 1006. |
| Tear | Crescent Moons | Tear | Council of High Lords (and Ladies) | Founded FY 994; channeling outlawed (Dragon Reborn revoked); Stone of Tear; sword Callandor. |

City-states distinct from the fourteen kingdoms:

- **Far Madding**
- **Mayene**
- **Tar Valon** (White Flame of the Aes Sedai; island in the Erinin; six Ogier-built bridges; 500 ft × 300 ft White Tower)

### Borderland Kingdoms (north hedge against the Blight)

The five — Saldaea, Kandor, Arafel, Shienar, Malkier — were founded together late in the War of the Hundred Years from the northern provinces of Hawkwing's empire. Their founding governors:

| Province | Founder |
|----------|---------|
| Arafel | Lady Mahira Svetanya |
| Saldaea | Lord Rylen t'Boriden Rashad |
| Kandor | Lord Jarel Soukovni |
| Shienar | Lady Merean Tihomar |
| Malkier | Lord Shevar Jamelle (fell to Trollocs in 955 NE) |

All five abstained from the War of the Hundred Years' general fighting and focused on Blight defense.

### Selected Capital City Notes

- **Caemlyn** — Inner City (Ogier-built) inside a 50 ft white-stone wall + New City (human). Royal Palace; library second only to Tar Valon and Cairhien Royal Library.
- **Ebou Dar** — Two halves split by the Eldar; west bank has nobility; the **Rahad** (east) is a low-class district with vicious thugs. Heavy canal/bridge transit; Tarasin Palace on Mol Hara square.
- **Cairhien** — Strict grid; **Foregate** is the chaotic outer-wall district (destroyed in the Shaido War). Royal Library survived even the Aiel sack.
- **Illian (city)** — Built on swamp; no walls; defended by causeways and the army. Square of Tammaz; King's Palace and Great Hall (a 2 ft smaller copy by royal dictum). The **Perfumed Quarter** is the lawless port district.
- **Tear (city)** — Walled inner district; outer district unpaved (citizens wear wooden shoes). Districts: **Maule** (port), **Chaim** (warehouses/docks), **Tavar** (farmer's market). Defenders of the Stone are an elite guard.
- **Tanchico** — Three peninsulas (Verana, Maseta, Calpene); three "Circles" (Great, King's, Panarch's); Panarch's Palace holds a museum of ancient bones.
- **Tar Valon** — Shining Walls; six bronze-clad gates; Great Library; barracks for Warders; hostel for petitioners; one of the few surviving Ogier groves with a fenced Waygate.

## Cross-Cutting Cultural Mechanics

### Game of Houses (Daes Dae'mar)

- Cairhien-centered; replicated at every social tier.
- Mechanically a chain of social interaction encounters (`encounters.md`); favors Bluff, Diplomacy, Innuendo, Sense Motive, Gather Information.
- Strangers entering Cairhienin society are presumed players whether they know it or not.

### Dueling (Altara)

- Constant low-level duels; up to hourly in Ebou Dar.
- Marriage knives worn by women; decoration encodes status (single/married/widowed), number of children, and other tags.
- Speech bound by the courtesy code; "lean back on your knife" gives a one-time pass to speak freely.

### Andoran Succession Law

- Lion Throne passes only to women.
- The eldest daughter must train at the White Tower.
- Sons become soldiers/generals (eldest with Warders).
- No female direct heir → nearest matrilineal female blood relation of Queen Ishara. "Disturbances" or wars of succession may follow.

### Arafel Honor

- Speak mind freely when asked.
- Always keep your word.
- Repay debts/obligations as fast as possible.

### Shienaran Honor & Shame

- Shame is treated as westlanders treat criminal arrest. Lords leverage duty against shame to compel obedience.

### Tairen Channeling Restriction

- Channeling formally outlawed by the High Lords; Aes Sedai tolerated only if they don't channel. Dragon Reborn has revoked the law in current era, but distrust persists.

### Amadician Channeling Outlaw

- Penalty up to death merely for *having* the One Power. Drives healers to be male; Wisdoms vanish.

### Saldaean Special Skills

- Knife-throwing (women) — uncanny accuracy.
- `Sa'sara` — seductive dance with battlefield-strategic potency. Often outlawed.
- Fan hand-speech — distinct language used by women.

## Geography of the Wider World

### Unclaimed Lands

Caralain Grass, Haddon Mirk, forests near the River Ivo, etc. Wilderness with bandits, beasts, and no government.

### Shadar Logoth ("Place Where the Shadow Waits")

- Ruins of the Aridhol capital; cursed by the disembodied evil **Mashadar**.
- Mordeth was its corruptor; consumes any visitor (human or Trolloc).
- Shadowspawn refuse to enter without being driven by Dreadlords/Myrddraal.

### Mountains of Dhoom and the Blight

- Northern boundary; Dark-One-warped wilderness.
- The Blight has crept south of the Mountains of Dhoom since the Breaking.
- North of the Blight: the **Blasted Lands** — even Shadowspawn refuse to enter.
- Blight flora are toxic to touch; some trees are mobile predators.
- **Shayol Ghul** — Dark One's prison; **Thakan'dar** valley below it forges Myrddraal shadow-blades.

### Aiel Waste (Three-fold Land / Djevik K'Shar)

- Two regions: **Termool** (Waterless Sands; southern fifth) and the rocky northern four-fifths.
- One river (recently created by the Dragon Reborn); every other water source owned by an Aiel clan under ancient treaty.
- Wagons must keep to designated roads.
- One city: **Rhuidean** — Jenn Aiel construction; sealed by Aes Sedai as a chief-test ter'angreal site; recently reopened with new lake & river.
- **Notable wildlife:** bloodsnake (3-pace, lethal venom — blood gels in minutes; no cure), red adder, rock snake, dust viper, scorpions, caisid (cat), large lizards, wild pigs.

### Isles of the Sea Folk (Atha'an Miere)

- **Tremalking** (largest, crescent SW of Tarabon).
- **Aile Jafar / Aile Somera / Aile Dashar** west of the westlands.
- Southern home islands have warm, exotic flora and fauna; Sea Folk live in coastal towns and tree-villages, never large cities.
- Sea Folk refuse to cross the Aryth Ocean — claim "Islands of the Dead" lie beyond.

#### Atha'an Miere Hierarchy

- **Sailmistress** captains a ship.
- **Windfinder** = first mate, chief navigator (often a channeler; see `gamemastering.md` Windfinder prestige class).
- **Cargomaster** = chief male officer; trade & defense.
- **Wavemistress** leads a clan.
- **First Twelve** of a clan elect a new Wavemistress.
- **Mistress of the Ships** ("queen") — elected by the First Twelve of the Wavemistresses; wears a multi-medallion nose chain; attended by a servant carrying a three-tiered blue parasol with gold fringe.
- **Master of the Blades** serves the Mistress of the Ships; **Swordmaster** equivalent serves each Wavemistress.

### Seanchan

- Trans-Aryth continent; conquered by Luthair Paendrag Mondwin (Hawkwing's last surviving son) starting FY 992; consolidated within ~300 yr.
- Ruled by the **Empress** (or Emperor) from the **Crystal Throne** in the **Court of the Nine Moons**, capital **Seandar**.
- Calling the empire's expansion into the westlands the **Return** (`Corenne`), preceded by the **Hailene** ("Forerunners") fleet of 500 ships.

#### Seanchan Social Structure

- **The Blood** — nobility descended from Paendrag. Identified by shaved heads and lacquered fingernails. Empress = 4 nails, lower nobility = 1.
- **Ha'shain** / craftspeople / merchants / commoners.
- **Da'covale (covale)** — slaves. Slavery is normalized; some slaves (e.g. **so'jhin** — hereditary Blood-servants, Deathwatch Guard, Seekers for Truth) outrank free commoners.
- **Sei'taer** ("level eyes") = honor; sanctity of one's word.
- **Sei'mosiev** ("downcast eyes") = lost honor; transferable from family/slaves to head of household.

#### Seanchan Channeling Apparatus

- All female children tested for channeling and for `sul'dam` (leash-holder) potential.
- Female channelers leashed via the **a'dam** ter'angreal as **damane** ("Leashed Ones"); treated as property.
- Male channelers hunted and executed; putting an a'dam on a man kills both man and sul'dam.
- `Sul'dam` wear dark blue dresses with red panels and silver forked-lightning bolts.

### Shara

- East of the Aiel Waste; cliffs hide all interior view.
- Trade only via Aiel overland routes, Sea Folk shipping, or Mayene captains, into 11 walled ports (6 Cliffs of Dawn / 5 southern coast).
- One peaceful empire since the Breaking.
- Ruler **Sh'boan** (female) or **Sh'botay** (male) takes a spouse on accession; dies exactly seven years later. Spouse rules and dies seven years on. Sharans call it "the Will of the Pattern."
- True power likely lies with the **Ayyad** (channelers), kept in walled villages. Male Ayyad are bred then executed; breed-trips outside villages are made hooded.

## Implementation Notes (WheelMUD)

- **World date model:** persist canonical time in `NE` years; expose conversion helpers `toAB` / `toFY` for in-character display. Treat AB↔FY↔NE as approximate — store conversions as named constants (`FY1_TO_AB ≈ AB_unknown_range`, `FY1135 ≈ 1 NE`) rather than fudging exact arithmetic.
- **Kingdom registry:** seed `world/kingdoms/*.yaml` from the table above with fields `id`, `symbol`, `capital`, `ruler`, `government`, `customs[]`, `lawsOfNote[]` (e.g. "channeling outlawed"), `borderlandFlag`, `dependentSymbols`. Drive on-room data (room.kingdom, room.cityDistrict) from this registry rather than hardcoding.
- **City districts:** model major cities as a graph of named districts/quarters (`Caemlyn.InnerCity`, `Caemlyn.NewCity`; `Ebou Dar.West`, `Ebou Dar.Rahad`; `Tear.Inner`, `Tear.Outer`, `Tear.Maule`, `Tear.Chaim`, `Tear.Tavar`; `Cairhien.Grid`, `Cairhien.Foregate`; `Illian.Tammaz`, `Illian.PerfumedQuarter`; `Tar Valon.WhiteTowerCompound`, `Tar Valon.OgierGrove`). Each district can carry its own crime tier, light-level, and patrol presence.
- **Law enforcement:** distinct law-effect flags per kingdom: `channelingForbidden` (Amadicia, Tear pre-revocation), `marriageKnifeRequired` (Altara), `duelingCustom` (Altara), `borderlandsConscription` (Borderlands). Wire into NPC reactions and status checks.
- **Succession & rulers:** the ruler field is a `Ref<NPC>` that ages with the world. Andor's matrilineal rule, Arad Doman's elective monarchy, and Tear's High Lords council all need separate "vacancy → succession" hooks for in-game political events.
- **Game of Houses:** model as a structured multi-NPC encounter using the `encounters.md` skill-challenge framework. Daes Dae'mar opposed checks default to Bluff/Diplomacy/Sense Motive; tag NPCs as `daesDaeMar.player` in Cairhien.
- **Marriage knives (Altara):** treat as a wearable item with metadata `{ status: single/married/widowed, children: int, otherTags }`. NPC reactions key off this metadata.
- **Borderlands defense:** add an event subsystem for periodic Trolloc raids in Saldaea/Kandor/Arafel/Shienar. Lancers and special unit types are NPC class templates (warrior + Mounted Combat + region-specific feats).
- **Atha'an Miere ranks:** ship objects carry crew slots `{ Sailmistress, Windfinder, Cargomaster, ... }`; Wavemistress and Mistress-of-the-Ships roles attach at clan / fleet scope. Each slot has a default skill template.
- **Seanchan rank/honor:** add `seanchanRank` and `seiTaer` (integer) properties on NPCs and PCs. Penalize on broken word; family head and slave-owner hooks propagate `sei'mosiev` upward.
- **A'dam interaction:** the existing weave/ter'angreal layer (`the-one-power.md`) needs an `a'dam` ter'angreal entry whose `apply(target)` checks gender and channeling — applying to a male triggers a lethal feedback mechanic affecting both bearers.
- **Threats and danger zones:** mark world regions with `regionDanger` tags — `Blight`, `BlastedLands`, `ShadowCoast`, `ShadarLogoth`, `AielWaste`, `SeaOfStorms`. Each tag carries spawn tables and environmental hazards from `encounters.md` (heat, cold, poison flora, wandering Mashadar trigger).
- **Mashadar:** Shadar Logoth needs a custom area effect that damages all visitors over time and a flag that blocks Shadowspawn entry by default unless overridden by a Dreadlord/Myrddraal in the encounter context.
- **Aiel Waste water-rights:** assign every named oasis/spring to a clan; movement off marked roads triggers a Wilderness Lore check or scattered hazards (broken axle, gully, bandit ambush).
- **Rhuidean ter'angreal:** two distinct tests — chief-candidate and Wise One — gated by background (Aiel) and by gender. The arch ter'angreal mechanism is a scripted scene rather than a generic weave.
- **Shara access:** restrict overland transit to the Aiel-mediated route and limit ship docking to the 11 walled cities. Trade-only with mandatory Appraise checks (Sharans always cheat — apply a default `-bonus` to every transaction).
- **Seanchan invasion timeline:** add a campaign clock that advances the Return through Tarabon (`King Andric dead, Panarch Amathera enslaved`) and into Altara/Amadicia for higher-level adventures.
- **Calendar conversion display:** in-character text and journals should always display the active local calendar (Borderlands uses NE; ancient ruins display Toman dates carved into stone). Provide a `formatDate(date, perspective)` helper.
- **Reputation overlap:** kingdom-specific reputation modifiers stack with the global Reputation system (`heroic-characteristics.md`). E.g. carrying a marked Trolloc scythesword in Shienar should be much worse than the default -2.
- **Cross-kingdom currency:** the Andoran gold mark having more gold than other coins implies a per-kingdom mint; accept the standard mk/sp/cp/gc abstraction at face value but allow optional per-currency exchange penalties when desired.
- **Companions / Queen's Guard / Defenders / Aiel society templates:** define elite-unit templates as `npcTemplate` records that other NPCs can inherit (e.g. `companion.illian`, `queensGuard.andor`, `defenderOfTheStone.tear`, `farDareisMai.aiel`, `algai'd'siswai.aiel`, `lancer.shienar`, `lightCavalry.saldaea`).
- **Ogier groves and Waygates:** Tar Valon's grove holds a fenced Waygate — model Waygates as world-graph adapters bridging into the Ways (Chapter 13 territory). Each grove has `accessible: bool` and a guard NPC.
- **Whitebridge & Tower of Ghenjei:** model as ancient `unbreakable: true` features in Andor with custom interactions; carved Arinelle bluff statues are flavor scenery objects.
- **Stone of Tear:** mark `weldedStone: true` (cannot be damaged by mundane means); houses the **Great Holding** with the largest non-Tar-Valon collection of angreal/ter'angreal — gate its inventory behind `dragonRebornAccess` and Defender-of-the-Stone permissions.
- **Shienaran fortifications:** roster the named fortresses (`Ankor Dail`, `Camron Caan`, `Fal Sion`, `Mos Shirare`, `Fal Dara`, `Fal Moran`) as defensive POIs with garrison templates.
- **Aes Sedai / Warder pipeline:** Andoran heirs and many Borderland nobles funnel into Tar Valon training — wire a "background → training path" mapping so character creation defaults match canon when the player picks the relevant background.
- **Free Year ↔ NE constants:** keep these in a single `world/calendar.ts` (or equivalent) so sweeping era changes only edit one file.
- **Lost / Sealed sites:** Rhuidean (sealed → reopened), Stone of Tear (impregnable until Dragon Reborn), Tar Valon (never fallen) — track their `sealedUntilEvent` so plot triggers can flip them open at scripted moments.
