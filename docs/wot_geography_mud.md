# Wheel of Time — Complete Geography Reference
## For MUD Zone & Room Building
*Compiled from map analysis + series knowledge*

> **Status: work in progress.** This doc is the canonical builder reference for
> Westlands geography, but conventions and tables below will continue to evolve
> as systems land. Don't treat any single row as locked unless `data/world/`
> already references it.

---

## BUILDER CONVENTIONS

These rules govern how the YAML world (`data/world/`) maps onto the schema in
`internal/repo/` and the loader in `internal/world/`. Builders read this
section first; the rest of the doc is the lore gazetteer.

### Zone-ID Allocation

- **Block size:** 1000 IDs per nation. Sub-zones (regions, settlements,
  buildings) draw from their nation's block. Cross-nation routes (roads,
  rivers) are owned by the nation containing the road's origin.
- **Order:** starter-first, then radiating outward toward the Borderlands and
  Shadow at the high end. Reserved blocks are not the same as "must be
  populated"; gaps are fine.

| Block | Nation / Region | Notes |
|-------|-----------------|-------|
| 1000–1999 | **Andor (incl. Two Rivers)** | Starter zone lives here; Two Rivers gets 1000–1199 reserved. |
| 2000–2999 | Cairhien | |
| 3000–3999 | Tear | |
| 4000–4999 | Mayene | City-state; small footprint. |
| 5000–5999 | Illian | |
| 6000–6999 | Murandy | |
| 7000–7999 | Far Madding | City-state. |
| 8000–8999 | Altara | |
| 9000–9999 | Ghealdan | |
| 10000–10999 | Amadicia | |
| 11000–11999 | Tarabon (incl. Toman Head / Falme) | |
| 12000–12999 | Arad Doman (incl. Almoth Plain) | |
| 13000–13999 | Tar Valon | City-state. |
| 14000–14999 | Saldaea | |
| 15000–15999 | Kandor (incl. Field of Merrilor) | |
| 16000–16999 | Arafel | |
| 17000–17999 | Shienar | |
| 18000–18999 | Aiel Waste | |
| 19000–19999 | Malkier (fallen) | Endgame; ruins + Blight encroachment. |
| 20000–20999 | The Blight / Mountains of Dhoom | Endgame. |
| 21000–21999 | Blasted Lands / Shayol Ghul | Endgame. |
| 22000–22999 | Shara | **Stub only**; closed border, deferred until canon supports it. |
| 23000–23999 | Seanchan-held islands (Tremalking, etc.) | Reserved. |
| 30000–30999 | **The Ways** | Off-world realm; Waygate-linked. |
| 31000–31999 | **Portal Stone mirror worlds** | Off-world realm; deferred design. |

### Level-Range / Climate / Sector Master Table

`level_range` drives mob_template tuning expectations (see ROADMAP §combat).
`climate` is descriptive (zone.yaml `climate:`); the **sector** column is the
*default* sector used when a room omits `sector:`. Per-room overrides always
win.

| Region | Band | level_range | Climate | Default sector |
|--------|------|------------:|---------|----------------|
| Two Rivers | Starter | 1–5 | temperate | field |
| Andor (settled) | Settled | 5–15 | temperate | city |
| Cairhien | Settled | 5–15 | temperate-cold | hills |
| Tear | Settled | 5–15 | warm-humid | city |
| Mayene | Settled | 5–15 | warm-coastal | city |
| Illian | Settled | 5–15 | warm-coastal | city |
| Murandy | Settled | 5–15 | temperate | hills |
| Far Madding | Settled | 5–15 | temperate-lake | city |
| Altara | Settled | 5–15 | warm-mediterranean | city |
| Ghealdan | Settled | 5–15 | temperate-mountain | hills |
| Amadicia | Settled | 5–15 | warm-mediterranean | city |
| Tarabon | Settled | 5–15 | warm-coastal | city |
| Toman Head / Falme | Contested | 10–20 | warm-coastal | city |
| Almoth Plain | Contested | 10–20 | temperate-arid | field |
| Arad Doman | Settled | 5–15 | temperate-coastal | city |
| Tar Valon | Settled | 5–15 | temperate | city |
| Saldaea | Borderland | 15–25 | cold-steppe | field |
| Kandor | Borderland | 15–25 | cold-mountain | hills |
| Arafel | Borderland | 15–25 | cold-mountain | hills |
| Shienar | Borderland | 15–25 | cold-mountain | mountain |
| Aiel Waste | Wilderness | 20–30 | arid-rocky | **waste** *(planned)* |
| Termool Desert | Wilderness | 20–30 | arid-hot | desert |
| Haddon Mirk | Wilderness | 15–25 | warm-humid | **swamp** *(planned)* |
| Drowned Lands | Wilderness | 15–25 | warm-humid | **swamp** *(planned)* |
| Paetrinh Swamp | Wilderness | 10–20 | temperate-humid | **swamp** *(planned)* |
| Steddings (any) | Sanctuary | n/a | varies | **stedding** *(planned)* |
| Manetheren ruins | Endgame-mid | 15–25 | mountain | mountain |
| Malkier (fallen) | Endgame | 30+ | corrupted-cold | **blight** *(planned)* |
| The Blight | Endgame | 30+ | corrupted | **blight** *(planned)* |
| Mountains of Dhoom | Endgame | 30+ | corrupted-mountain | mountain |
| Blasted Lands | Endgame | 35+ | corrupted-arid | **blight** *(planned)* |
| Shayol Ghul | Endgame | 40+ | corrupted-volcanic | mountain |
| The Ways | Special | 20–35 | timeless | underground |
| Portal Stone worlds | Special | varies | varies | varies |

### Planned Sector Enum Extensions

Current sectors (from `internal/repo/room.go`):
`city, forest, field, hills, mountain, desert, water, underwater, air, underground`.

Migration **0021 (planned)** will add:

| Sector | Purpose |
|--------|---------|
| `blight` | Corrupted lands north of Mountains of Dhoom; ambient horror; future DoT hook. |
| `waste` | Aiel Waste — arid rocky steppe, distinct from generic desert. |
| `stedding` | Ogier groves — One Power suppressed, Shadowspawn barred (mechanical effect TBD). |
| `swamp` | Haddon Mirk, Drowned Lands, Paetrinh — movement-cost / encounter flavor. |

Until 0021 lands, the doc tags above are advisory; loader-side `sector:` must
still use the existing enum values.

### Climate Conventions

`climate:` on `zone.yaml` is free-form text consumed by phase-ambient §9 and
future weather. Use lowercase hyphenated tokens (e.g., `temperate`,
`warm-coastal`, `cold-mountain`, `corrupted`). Steddings should set
`climate: stedding` regardless of surrounding terrain.

### Mirror-World / Off-World Realms

- **The Ways** — accessed via Waygates (sealed by the Aes Sedai but breakable
  in canon). Reserved zone block 30000–30999. See **The Ways** section below.
- **Portal Stones / Mirror Worlds** — alternate parallel worlds reachable via
  Portal Stone columns scattered across the Westlands, Aiel Waste, and
  off-continent. Reserved zone block 31000–31999. **Design deferred** — see
  ROADMAP.md.

---

## MAP-CONFIRMED ADDITIONS & CORRECTIONS TO PRIOR LIST

### NEW locations confirmed from map not previously listed:
- **Malkier** — shown as a distinct fallen nation with Seven Towers marked
- **Valley of Thakan'dar** — shown clearly between Shayol Ghul and the Blightborder
- **Field of Merrilor** — shown between Kandor/Arafel and Tar Valon area
- **Polov Heights** — shown near Field of Merrilor
- **Proska Flats** — shown in Saldaea/Kandor border area
- **Plain of Lances** — shown in Saldaea
- **Caralain Grass** — labeled between Andor and Cairhien
- **Drowned Lands** — labeled east of Godan in Tear
- **Termool Desert** — labeled east of Tear along the Spine
- **Stedding Shangtai** — confirmed labeled on map east of Tear
- **Alcair Dal** — Aiel Waste, shown on map
- **Cold Rocks Hold** — Aiel Waste, shown on map
- **Rhuidean** — shown in the Aiel Waste (southwest area of Waste)
- **Chaendaer** — mountain near Rhuidean
- **Imre Stand** — Aiel Waste
- **Silk Path** — named road/route through Aiel Waste
- **Kinslayer's Dagger** — mountain range shown between Cairhien and the Waste
- **Jolivaine Pass** — pass east of Andor/Cairhien border area
- **Kabal Deep** — coastal inlet, Altara/Illian border area
- **Fingers of the Dragon** — river delta south of Tear city
- **Bay of Remara** — east of Tear, near Mayene
- **Mayene** — city-state on the Sea of Storms, east of Tear
- **Cindaking** — island in the Sea of Storms
- **Tremalking** — large island in the Aryth Ocean (Seanchan-held, site of Choedan Kal)
- **Qaim** — small island in the Sea of Storms
- **Aile Dashar** — island group in Aryth Ocean
- **Aile Somera** — island in Aryth Ocean
- **Aile Jafar** — island in Aryth Ocean
- **Dantor** — island near Aile Jafar
- **World's End** — northwestern tip of the continent
- **Toman Head** — peninsula, west coast
- **Falme** — city on Toman Head
- **The Shadow Coast** — coastal region of Tarabon/Amadicia south
- **Windbiter's Finger** — coastal peninsula south of Shadow Coast
- **Mountains of Dhoom** — labeled clearly as the northern mountain range bordering the Blight
- **The Blasted Lands** — shown northeast, bordering Shara
- **The Fortress** (of the Shadow) — shown in Blasted Lands
- **Shayol Ghul** — labeled clearly with the mountain shown
- **Tar Valon** — shown prominently in central Andor/Cairhien area with island
- **Dragonmount** — shown just north of Tar Valon/Cairhien
- **Tower of Ghenjei** — labeled clearly in Andor near the Hills of Absher
- **Hawkwing's Statue** — labeled in Andor (near Caemlyn area)
- **Seven Towers** — labeled in Malkier
- **Silverwall Keeps** — labeled in Kandor
- **Fal Eisen** — confirmed labeled in Shienar/Arafel area
- **Ankor Dail** — labeled in Shienar
- **Niamh Passes** — eastern Shienar passes
- **Tarwin's Gap** — confirmed labeled between Shienar and Malkier/Blight
- **Camion Caan** — labeled in Arafel/Shienar area
- **Shirare** — town in Shienar
- **Meads** — settlement in Shienar area
- **Jakanda** — town in Arafel
- **Fomie** — settlement in Shienar
- **Millard's Hill** — in Shienar
- **Coron Ford** — in Arad Doman
- **Soanje** — town in Arad Doman
- **Darluna** — town in Arad Doman
- **Akuum** — settlement in Arad Doman
- **Kandelmar** — town in Arad Doman
- **Katar** — town/fort in Arad Doman (important in later books)
- **Atura's Orchard** — landmark in Arad Doman/Almoth Plain border
- **Tobin's Hollow** — settlement near Almoth Plain
- **Atuan's Mill** — settlement on Toman Head
- **Paetrinh Swamp** — swamp area near Almoth Plain/Arad Doman
- **Darkwood** — forest near Almoth Plain
- **Lake Somal** — lake in Andor/Two Rivers area
- **Comfrey** — confirmed on map in Two Rivers/Andor
- **Baerlon** — confirmed on map, northeastern Two Rivers area
- **Hills of Absher** — labeled between Shadar Logoth and Andor
- **Shadar Logoth** — confirmed labeled (ruins) with Tower of Ghenjei nearby
- **Goren Springs** — near Field of Merrilor
- **Dumai's Wells** — labeled in Cairhien area
- **Silanele Spring** — near Dragonmount/Cairhien
- **Lianrod** — town in Cairhien
- **Tremonsien** — confirmed in Cairhien
- **Maron** — town on Cairhien/Andor border area
- **Aringill** — confirmed on map on Andoran side of the Erinin
- **Morelle** — settlement near Tear/Cairhien border
- **Molvaine Gap** — pass/gap in mountains between Murandy and Altara area
- **Cumbar Hills** — hills in Murandy/Andor border area
- **Splintered Hills** — labeled in Andor, south of Caemlyn
- **Hinderstap** — confirmed on map in Andor/Murandy border
- **Trustair** — town in Murandy/Andor border
- **Damelien** — town near Caemlyn area
- **Four Kings** — confirmed on map
- **Arien** — confirmed on map on Caemlyn Road
- **New Braem** — town shown in Andor, north of Caemlyn (Braem Wood area)
- **Ibi** — settlement near Braem Wood
- **Naterin** — settlement in Andor
- **Harwin** — settlement near Andor/Cairhien
- **Black Towers** — labeled near Caemlyn (the M'Hael's Black Tower)
- **Doirlon Hills** — confirmed in Illian
- **Sabinel** — town in Altara/Illian area
- **Molvain** — settlement in Altara/Murandy area
- **Malden** — town in Altara (Shaido held)
- **Molzen** — settlement in Altara
- **Inishling** — settlement in Murandy/Altara
- **Remen** — town in Altara/Murandy border
- **Samaha** — settlement in Murandy/Altara
- **Tallan** — settlement in Altara
- **Tyall** — settlement in Altara/Murandy
- **Marelli** — settlement in Murandy
- **Hian Road** — labeled in Altara
- **Shar** — settlement in Altara
- **Corfar** — settlement in Altara/Amadicia border
- **Willar** — town in Altara/Ghealdan border area
- **Boannda** — settlement in Ghealdan
- **Dhaslin Pass** — pass in Ghealdan/Murandy area
- **Bethal** — town in Ghealdan
- **Jarra** — confirmed in Ghealdan
- **Sidon** — confirmed in Ghealdan
- **Tara** — settlement in Ghealdan
- **Jeramel** — town in Ghealdan/Amadicia area
- **Nassau** — settlement in Amadicia
- **Mardecin** — confirmed in Amadicia
- **Sienda** — town in Amadicia
- **Bellon** — confirmed in Amadicia
- **Almisar** — settlement in Amadicia
- **Shario** — settlement in Amadicia
- **Garsin** — settlement near Amadicia/Altara border
- **Gudan** — town in Amadicia south
- **Alkidaer** — southern settlement near Altara/Amadicia
- **Jurador** — town in southern Altara
- **Runnien Crossing** — crossing point in southern Altara
- **Mosra** — settlement near southern Altara
- **Maderin** — town in Altara
- **Saremaihe** — settlement in Altara
- **Elmora** — town in Tarabon
- **Serana** — town in Tarabon
- **Elmore** — town confirmed in Tarabon
- **Amador Road** — labeled road in Tarabon connecting toward Amadicia
- **Alcuna** — settlement near Tarabon/Ghealdan
- **Maracru** — settlement near Tarabon/Ghealdan border
- **Rhadavyn** — area/settlement near Ghealdan
- **Itura** — settlement near Ghealdan/Amadicia
- **Saldaea Road** — labeled road out of Bandar Eban
- **Ravinda** — town in Kandor
- **South Mettler** — settlement in Kandor/Saldaea border
- **South Hill** — settlement in Kandor
- **Manala** — confirmed labeled in Kandor
- **Canluum** — confirmed labeled in Kandor
- **Chachin** — confirmed labeled in Kandor
- **Irinjavar** — confirmed labeled in Saldaea
- **Sidona** — town in Saldaea
- **Maradon** — confirmed labeled in Saldaea
- **Murandy towns** confirmed: Lugard, Remen, Trustair, Hinderstap, Minde, Inishling, Sabinel, Molzen, Molvain, Malden, Samaha, Tallan, Tyall, Marelli

---

## COMPLETE NATION-BY-NATION GEOGRAPHY

---

## ANDOR

**Capital:** Caemlyn

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Caemlyn | Capital; Inner City, Outer City, Royal Palace, Black Tower nearby |
| Whitebridge | Major town; famous white bridge over the Arinelle |
| Baerlon | Large town, mining region, northeastern Two Rivers area |
| Four Kings | Town on Caemlyn Road |
| Arien | Town on Caemlyn Road |
| Market Sheran | Town on Caemlyn Road |
| Carysford | Town/river crossing on Caemlyn Road |
| Comfrey | Small settlement, Two Rivers border |
| Damelien | Town south of Caemlyn |
| New Braem | Town in Braem Wood area, north of Caemlyn |
| Ibi | Settlement near Braem Wood |
| Naterin | Settlement in Andor |
| Harwin | Settlement near Andor/Cairhien border |
| Hinderstap | Village near Andor/Murandy border — cursed, night resets |
| Trustair | Town near Andor/Murandy border |

**The Two Rivers (remote western district of Andor):**
| Location | Notes |
|----------|-------|
| Emond's Field | Principal village; Two Rivers |
| Taren Ferry | Village at northern edge of Two Rivers, river crossing |
| Watch Hill | Village in Two Rivers |
| Deven Ride | Village in Two Rivers |

### Rivers:
| River | Course |
|-------|--------|
| Arinelle | Flows from north through Saldaea/Andor, through Whitebridge, south to join Manetherendrelle |
| Manetherendrelle | Flows east-west along Andor's southern border; formed by Arinelle + Tarendrelle confluence |
| Tarendrelle | Flows from the Mountains of Mist east, forms northern Andor/Saldaea border area |
| "The Water" (local) | Stream near Emond's Field in Two Rivers |

### Named Roads:
| Road | Connects |
|------|----------|
| Caemlyn Road | Whitebridge → Four Kings → Market Sheran → Carysford → Arien → Caemlyn (east-west spine of Andor) |
| North Road | Caemlyn → northward through Braem Wood toward Kandor/Arafel |
| Road to Tar Valon | Caemlyn → northeast → Tar Valon (via Aringill crossing) |
| Road to Tear/Illian | Caemlyn → south through Murandy → Illian or Tear |

### Notable Locations:
- **Royal Palace of Andor** — Caemlyn, seat of the Lion Throne
- **The Black Tower** — near Caemlyn, headquarters of the Asha'man (male channelers)
- **Ogier Grove** — Caemlyn, contains the Waygate
- **Shadar Logoth** — ruins on Andor's northwestern border; utterly destroyed city, corrupted by Mashadar; location of the Tower of Ghenjei nearby
- **Tower of Ghenjei** — tall silver column in the Hills of Absher; entrance to the *Finn* (Aelfinn/Eelfinn) world
- **Hawkwing's Statue** — landmark in Andor near Caemlyn
- **Braem Wood** — large forest north of Caemlyn, dangerous territory, border with Arafel
- **Splintered Hills** — hills south of Caemlyn
- **Cumbar Hills** — hills near Andor/Murandy border
- **Hills of Absher** — between Shadar Logoth and main Andor
- **Lake Somal** — lake in the Two Rivers/Baerlon area
- **Westwood** — forest west of Emond's Field (Two Rivers)
- **Waterwood** — forested wetland area in the Two Rivers
- **Mountains of Mist** — western mountain range forming the Two Rivers' western wall; also called "the Mountains" locally
- **Tar Valon** — technically an independent city-state but located in Andor geographically on an island in the Erinin

---

## TAR VALON (Independent City-State)

### Description:
Built on a large island in the River Erinin. Home of the Aes Sedai and the White Tower. One of the largest and most powerful cities in the world.

### Key Locations:
- **The White Tower** — headquarters of the Aes Sedai; enormous white spire
- **The Black Tower** (Ajah headquarters, not to be confused with Asha'man's Black Tower)
- **Ogier-built bridges** — six great bridges connecting the island to both banks
- **The docks** — extensive river commerce
- **The Waygate** — in the Ogier Grove on the island; sealed by the Aes Sedai
- **Dragonmount** — the great volcanic mountain just north/northwest of Tar Valon; where Lews Therin died and was reborn as Rand al'Thor

---

## CAIRHIEN

**Capital:** Cairhien (the city)

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Cairhien | Capital; Sun Palace, Topless Towers, Great Library, Foregate |
| Tremonsien | Village; near large statue (Choedan Kal access key) |
| Lianrod | Town in Cairhien |
| Maron | Town on Cairhien/Andor border (Erinin crossing area) |
| Morelle | Settlement near Tear/Cairhien border |
| Aringill | River town on Andoran side, key Erinin crossing |
| Dumai's Wells | Location of major battle (Rand's rescue), interior Cairhien |
| Silanele Spring | Landmark near Dragonmount/Cairhien |

### Rivers:
| River | Course |
|-------|--------|
| Erinin | Flows north to south through eastern Andor/Cairhien border, past Tar Valon, into Tear delta |
| Alguenya | Flows through Cairhien city, joins the Erinin |

### Named Roads:
| Road | Connects |
|------|----------|
| Jangai Pass Road | Cairhien → east through Jangai Pass → Aiel Waste |
| Road north to Tar Valon | Cairhien → north along Alguenya/Erinin → Tar Valon |

### Notable Locations:
- **Sun Palace** — royal seat of Cairhien
- **The Topless Towers** — iconic towers of Cairhien city
- **The Great Library** — one of the great repositories of knowledge in the world
- **Ogier Grove** — outside Cairhien city; contains Waygate
- **The Foregate** — sprawling shantytown outside city walls (destroyed during Shaido siege)
- **Dragonmount** — volcanic mountain just north of Cairhien; visible from the city
- **Caralain Grass** — large open grassland between Andor and Cairhien, nominally part of the region
- **Kinslayer's Dagger** — jagged mountain range east of Cairhien, bordering the Spine of the World
- **The Spine of the World (Dragonwall)** — massive mountain range forming Cairhien's eastern border
- **Jangai Pass** — principal pass through the Spine of the World into the Aiel Waste
- **Jolivaine Pass** — pass further north, used as alternate route
- **Field of Talidar** — open area east of Cairhien/Shienar (historical battle site)

---

## TEAR

**Capital:** Tear (the city)

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Tear | Capital; dominated by the Stone of Tear |
| Godan | Coastal town east of Tear city |
| Morelle | Small settlement near northwestern Tear |

### Rivers:
| River | Course |
|-------|--------|
| Erinin | Flows into the delta south of Tear city; multiple channels |
| Fingers of the Dragon | The great river delta at the mouth of the Erinin, south of Tear city |

### Notable Locations:
- **The Stone of Tear** — enormous ancient fortress/palace; one of the most defensible structures in the world; Callandor held here
- **Callandor's chamber** — within the Stone, where the crystal sword sa'angreal was kept
- **Haddon Mirk** — dark, dangerous swamp/forest in western Tear
- **Drowned Lands** — coastal marshland east of Godan
- **Termool Desert** — dry region along the eastern Tear/Spine border
- **Bay of Remara** — east of Godan; bay leading to Mayene
- **Plains of Maredo** — open plains northwest of Tear, contested with Illian
- **Stedding Shangtai** — Ogier stedding east of Tear near the Termool; contains a Waygate

---

## MAYENE (Independent City-State)

- City-state on a peninsula east of Tear on the Bay of Remara
- Rules the Mayener oilfish fleet; economically powerful but politically pressured by Tear
- **Berelain** is First of Mayene during the series
- The **Winged Guards** are Mayene's elite cavalry

---

## ILLIAN

**Capital:** Illian (the city)

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Illian | Capital; built on islands/canals, Sea of Storms port |
| Inishling | Settlement northern Illian/Murandy border |
| Sabinel | Settlement in Illian/Altara border area |

### Rivers:
| River | Course |
|-------|--------|
| Manetherendrelle | Forms northern border with Murandy/Andor |
| Several unnamed rivers | Empty into the Sea of Storms through Illian's southern coast |

### Notable Locations:
- **The Square of Tammaz** — great central plaza of Illian city
- **Palace of the King** — royal seat in Illian
- **Ogier Grove / Waygate** — in Illian city
- **The Companions** — Illian's elite military force
- **Plains of Maredo** — northwest, contested with Tear
- **Doirlon Hills** — northwest of Illian city, between Illian and Murandy/Far Madding
- **Kabal Deep** — coastal inlet on Altara/Illian western border

---

## MURANDY

**Capital:** Lugard

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Lugard | Capital; city of intrigue |
| Trustair | Town, northern Murandy near Andor border |
| Hinderstap | Village on Andor/Murandy border (also listed under Andor) |
| Minde | Settlement near Hinderstap |
| Remen | Town, western Murandy/Altara border area |
| Malden | Town in Altara (Shaido-held during series, near Murandy) |
| Samaha | Settlement in Murandy/Altara |
| Tallan | Settlement |
| Tyall | Settlement |
| Marelli | Settlement |
| Molzen | Settlement |
| Molvain | Settlement |
| Inishling | Settlement on Murandy/Illian border |
| Sabinel | Settlement near Illian border |

### Rivers:
| River | Course |
|-------|--------|
| Manetherendrelle | Northern border with Andor |
| Stump | River flowing through Murandy |
| Several unnamed rivers | Feed into Altara/Illian river systems |

### Notable Locations:
- **Cumbar Hills** — northern Murandy near Andor border
- **Murandy** is notably fragmented — the king in Lugard has very little real authority over the many independent lords

---

## FAR MADDING (Independent City-State)

### Description:
Built on a large island in a lake in the Hills of Madding (between Murandy and Tear/Illian). Unique in all the world: a ter'angreal called **the Guardian** blocks all channeling of the One Power within the city and for some distance around it.

### Notable Locations:
- **The Guardian** — ancient ter'angreal that creates a dome blocking saidin and saidar; makes Far Madding neutral ground
- **The Hall of the Counsels** — governing body; twelve Counsels rule
- **Three causeways** — connect the island city to the surrounding shores (North, South, East)
- **The hills surrounding the lake** — known as the Hills of Madding

---

## ALTARA

**Capital:** Ebou Dar

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Ebou Dar | Capital; on the Sea of Storms; Tarasin Palace, the Rahad |
| Cormaed | Town in northern Altara |
| Salidar | Town in western Altara; temporary Aes Sedai seat |
| Remen | Town on the Altara/Murandy border (also listed Murandy) |
| Maderin | Town in central Altara |
| Jurador | Town in southern Altara |
| Saremaihe | Settlement in Altara |
| Runnien Crossing | River crossing, southern Altara |
| Mosra | Settlement near Runnien Crossing |
| Alkidaer | Settlement in southern Altara |
| Malden | Town (Shaido-held during series) |
| Shar | Settlement |
| Corfar | Settlement near Amadicia border |
| Willar | Town near Ghealdan border |

### Rivers:
| River | Course |
|-------|--------|
| Eldar | Flows through/past Ebou Dar into the Sea of Storms |
| Manetherendrelle | Northern border with Andor/Murandy |
| Several unnamed rivers | Including the river Salidar sits near |
| Hian Road river | Flows through eastern Altara (Hian Road follows it) |

### Named Roads:
| Road | Connects |
|------|----------|
| Lugard Road | Ebou Dar → north through Altara → Lugard (Murandy) |
| Hian Road | Eastern Altara, running north-south |

### Notable Locations:
- **Tarasin Palace** — royal seat of Altara in Ebou Dar
- **The Rahad** — extremely dangerous slum district of Ebou Dar; crossbow culture, daily dueling
- **The Wandering Woman** — famous inn in Ebou Dar
- **The Bowl of the Winds** — powerful ter'angreal found hidden in the Rahad
- **The Kin's farm** — outside Ebou Dar; used by the Kin (women who can channel but aren't Aes Sedai)
- **Hinderstap** — cursed village (events reset each night)
- **The Rhannon Hills** — hills in Altara
- **Kabal Deep** — coastal inlet on western Altara
- **Winged Guards** — Altara's elite military

---

## GHEALDAN

**Capital:** Jehannah

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Jehannah | Capital |
| Samara | Town on the Manetherendrelle; major river port; Prophet's movement activity |
| Sidon | Town in Ghealdan |
| Bethal | Town in Ghealdan |
| Tara | Settlement in Ghealdan |
| Jarra | Small village in Ghealdan |
| Abila | Town in Ghealdan; Prophet's base for a time |
| Boannda | Settlement in Ghealdan |
| Itura | Settlement near Ghealdan/Amadicia border |
| Jeramel | Town near Ghealdan/Amadicia border |

### Rivers:
| River | Course |
|-------|--------|
| Manetherendrelle | Flows along/through northern Ghealdan; Samara sits on this river |
| Dhalin Pass river | Flows through Ghealdan toward Murandy |

### Notable Locations:
- **Mountains of Mist** — run through/along Ghealdan; labeled on map as bordering Ghealdan
- **Dhaslin Pass** — pass through mountains between Ghealdan and Murandy
- **Shadow of the Mountains** — the area at the foot of the Mountains of Mist in Ghealdan; heavily forested

---

## AMADICIA

**Capital:** Amador

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Amador | Capital; seat of the Children of the Light |
| Mardecin | Town near Amadicia/Tarabon border |
| Sienda | Town in Amadicia |
| Bellon | Town in Amadicia |
| Nassau | Settlement in Amadicia |
| Almisar | Settlement in Amadicia |
| Shario | Settlement |
| Garsin | Settlement near Amadicia/Altara border |
| Gudan | Town in southern Amadicia |

### Rivers:
| River | Course |
|-------|--------|
| Gaean | Flows through Amadicia |
| Eldar (upper) | Upper reaches in eastern Amadicia |

### Named Roads:
| Road | Connects |
|------|----------|
| Amador Road | Confirmed labeled on map; connects Tanchico (Tarabon) → eastward toward Amador |
| Road to Ghealdan | Amador → northeast → Ghealdan |

### Notable Locations:
- **The Fortress of the Light** — enormous fortress in Amador; headquarters of the Children of the Light (Whitecloaks); contains the Court of the Sun
- Amadicia is effectively a Whitecloak theocracy; the king holds nominal power only

---

## TARABON

**Capital:** Tanchico

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Tanchico | Capital; peninsula city on three hills; Panarch's Palace, King's Palace |
| Elmora | Town in Tarabon |
| Elmore | Town in Tarabon |
| Serana | Town in Tarabon |
| Alcuna | Settlement near Tarabon/Ghealdan border |
| Maracru | Settlement near Tarabon/Ghealdan border |
| Rhadavyn | Area/settlement in northern Tarabon |
| Falme | City on Toman Head (Seanchan landing point in book 2) |

### Rivers:
| River | Course |
|-------|--------|
| Andahar | Flows near/through Tanchico |
| Several rivers | Flow from the Mountains of Mist west to the coast |

### Named Roads:
| Road | Connects |
|------|----------|
| Amador Road | Tanchico → east toward Amador (Amadicia) |

### Notable Locations:
- **The Panarch's Palace** — one of two seats of power in Tanchico; contains the Panarch's Museum with many ter'angreal
- **The King's Palace** — second seat of power in Tanchico
- **The Assembly of Lords** — Tanchico
- **Toman Head** — large peninsula on the northwest coast; scene of Seanchan invasion in *The Great Hunt*
- **Falme** — city on Toman Head; Seanchan-controlled; site of the Horn of Valere blowing
- **The Shadow Coast** — dark, rugged southern coastline of Tarabon/Amadicia
- **Windbiter's Finger** — southern coastal peninsula
- **Almoth Plain** — large contested open plain between Tarabon and Arad Doman

---

## ARAD DOMAN

**Capital:** Bandar Eban

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Bandar Eban | Capital; major coastal city on the Aryth Ocean |
| Soanje | Town in northern Arad Doman |
| Darluna | Town in Arad Doman |
| Akuum | Settlement in Arad Doman |
| Kandelmar | Town in Arad Doman |
| Katar | Fort/town in Arad Doman; significant in later books |
| Coron Ford | River ford/crossing in Arad Doman |
| Atura's Orchard | Landmark near Almoth Plain |
| Tobin's Hollow | Settlement near Almoth Plain |

### Rivers:
| River | Course |
|-------|--------|
| Dhagon | Flows through Arad Doman toward the Aryth Ocean |
| Several coastal rivers | Feed the Aryth Ocean coastline |

### Named Roads:
| Road | Connects |
|------|----------|
| Saldaea Road | Confirmed on map; Bandar Eban → north toward Saldaea |
| Road east to Almoth Plain | Bandar Eban → east toward the contested plain |

### Notable Locations:
- **The Council of Merchants** — the true power in Arad Doman alongside the King
- **Almoth Plain** — large contested plain to the south/southeast; historically disputed between Arad Doman and Tarabon
- **Paetrinh Swamp** — swampy area near the Almoth Plain/Arad Doman interior
- **Darkwood** — forested area near Almoth Plain
- **World's End** — northwestern tip of the continent, in Arad Doman's northwestern territory
- **Toman Head** — peninsula to the south of World's End, technically Tarabon

---

## SALDAEA

**Capital:** Maradon

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Maradon | Capital; heavily besieged in later books; Rand intervenes dramatically |
| Irinjavar | Town; site of battle against Seanchan |
| Sidona | Town in Saldaea |
| South Mettler | Settlement near Saldaea/Kandor border |

### Rivers:
| River | Course |
|-------|--------|
| Arinelle (upper) | Rises in Saldaea/Kandor area mountains, flows south |
| Several rivers | Flow across the open steppes of Saldaea |

### Notable Locations:
- **Plain of Lances** — large open plain in Saldaea; perfect cavalry country
- **Proska Flats** — flatland area in Saldaea/Kandor border region
- **Mountains of Dhoom** — northern border; the mountain range separating Saldaea from the Blight
- **The Blight** — begins just north of the Mountains of Dhoom

---

## KANDOR

**Capital:** Chachin

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Chachin | Capital; built on a mountain; enormous city |
| Canluum | Major city; known as the City of Lanterns |
| Manala | Town in Kandor |
| Ravinda | Town in Kandor |
| South Hill | Settlement in Kandor |
| Silverwall Keeps | Fortress complex in Kandor |

### Notable Locations:
- **The Aesdaishar Palace** — royal palace in Chachin
- **Silverwall Keeps** — border fortifications
- **Proska Flats** — open flat terrain in the south of Kandor/Saldaea border area
- **Field of Merrilor** — shown on the map south of the Blightborder in this general region; site of the great gathering before the Last Battle
- **Goren Springs** — near Field of Merrilor
- **Polov Heights** — near Field of Merrilor; tactically important during the Last Battle
- **Mountains of Dhoom** — northern border with the Blight

---

## ARAFEL

**Capital:** Shol Arbela

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Shol Arbela | Capital |
| Jakanda | Town in Arafel |
| Fal Eisen | Fortress town (on Arafel/Shienar border) |

### Notable Locations:
- **Twin-blade warriors** — Arafellin soldiers famous for fighting with two swords and wearing bells in their hair
- **Border fortifications** — extensive keeps along the Blight
- **Mountains of Dhoom** — northern border

---

## SHIENAR

**Capital:** Fal Moran

### Cities & Towns (map-confirmed):
| Location | Notes |
|----------|-------|
| Fal Moran | Capital |
| Fal Dara | Great fortress city near the Blight; major defensive stronghold |
| Fal Eisen | Fortress town (border with Arafel) |
| Ankor Dail | Town/fort in Shienar |
| Camion Caan | Town in Arafel/Shienar area |
| Shirare | Town in Shienar |
| Meads | Settlement in Shienar |
| Fomie | Settlement in Shienar |
| Millard's Hill | Settlement/landmark in Shienar |

### Notable Locations:
- **Fal Dara Keep** — massive fortress; scene of major early series events; Waygate within the city
- **Tarwin's Gap** — the great mountain pass where Shadowspawn armies historically pour south; defended by Shienar and other Borderlanders; site of major battles
- **The Blight** — begins immediately north of Shienar
- **Niamh Passes** — eastern passes in the mountains of Shienar toward Shara border
- **Field of Talidar** — open area, historically significant battle plain

---

## MALKIER (Fallen Nation)

*Overrun by the Blight a generation before the series begins; Lan is the uncrowned king*

### Map-Confirmed Locations:
| Location | Notes |
|----------|-------|
| Seven Towers | Former capital/seat of Malkier; shown on map in ruins/Blight area |
| Valley of Thakan'dar | Southwest of Shayol Ghul; where shadowspawn are forged; shown clearly on map |

### Notable Locations:
- **Seven Towers** — the capital of Malkier, now consumed by the Blight
- Malkier was a fertile land before the Blight consumed it; Lan was born here and sent south as an infant
- **Tarwin's Gap** — the invasion route used when Malkier fell

---

## THE BLIGHT & BEYOND

### Map-Confirmed Locations:
| Location | Notes |
|----------|-------|
| The Great Blight | Labeled clearly; corrupted lands north of the Mountains of Dhoom |
| Mountains of Dhoom | The mountain range forming the Blightborder along the north |
| Shayol Ghul | Dark mountain in the far north; Bore to the Dark One's prison; shown with volcanic imagery |
| Valley of Thakan'dar | Below Shayol Ghul; where Myrddraal swords are forged and Draghkar created |
| The Blasted Lands | Far northeast; between Blight and Shara; The Fortress shown here |
| The Fortress (of the Shadow) | Dark One's seat of power; shown in Blasted Lands |

---

## MAJOR RIVERS (Cross-Nation Summary for MUD Routing)

| River | Source → Mouth | Nations Touched |
|-------|---------------|-----------------|
| Erinin | North (near Tar Valon/Shienar) → south → Tear delta (Fingers of the Dragon) | Shienar border, Arafel border, Andor/Cairhien border, Tear |
| Arinelle | Saldaea/Kandor mountains → south → joins Manetherendrelle | Saldaea, Andor (Whitebridge) |
| Manetherendrelle | Formed by Tarendrelle + Arinelle confluence → west → Sea of Storms | Andor (south border), Ghealdan, Murandy (border), Illian |
| Tarendrelle | Mountains of Mist → east → joins Arinelle | Andor (north), Saldaea (border) |
| Alguenya | North → flows through Cairhien → joins Erinin | Cairhien |
| Eldar | Mountains → south → Ebou Dar/Sea of Storms | Ghealdan (upper), Amadicia, Altara |
| Gaean | Mountains → south through Amadicia | Amadicia |
| Andahar | Mountains → through Tarabon → Sea of Storms | Tarabon |
| Dhagon | Interior Arad Doman → Aryth Ocean | Arad Doman |

---

## MAJOR ROADS (Cross-Nation Summary for MUD Routing)

| Road Name | Route | Nations |
|-----------|-------|---------|
| Caemlyn Road | Whitebridge → Four Kings → Market Sheran → Carysford → Arien → Caemlyn | Andor |
| North Road | Caemlyn → Braem Wood → north toward Kandor | Andor, Kandor |
| Tar Valon Road | Caemlyn → Aringill → Tar Valon | Andor, Cairhien border |
| Amador Road | Tanchico → east through Tarabon → Amador | Tarabon, Amadicia |
| Saldaea Road | Bandar Eban → north toward Saldaea | Arad Doman, Saldaea |
| Lugard Road | Ebou Dar → north → Lugard | Altara, Murandy |
| Hian Road | Eastern Altara, north-south | Altara |
| Road to Tear | Caemlyn/Andor → south through Murandy → Tear | Andor, Murandy, Tear |
| Jangai Pass Road | Cairhien → east → Jangai Pass → Aiel Waste | Cairhien, Aiel Waste |

---

## GEOGRAPHIC FEATURES (for zone flavor/descriptions)

| Feature | Location | Notes |
|---------|----------|-------|
| Mountains of Mist | Western Andor/Two Rivers/Ghealdan | Form western wall of the Two Rivers |
| Spine of the World (Dragonwall) | Eastern Cairhien/Tear border | Massive range separating Westlands from Aiel Waste |
| Kinslayer's Dagger | East of Cairhien | Jagged spur of mountains |
| Mountains of Dhoom | Northern Borderlands | Northern wall before the Blight |
| The Black Hills | Between Saldaea and Andor | Dark, forbidding hill range |
| Splintered Hills | Southern Andor | Hills south of Caemlyn |
| Cumbar Hills | Andor/Murandy border | |
| Rhannon Hills | Altara | |
| Doirlon Hills | Northwest Illian | |
| Caralain Grass | Between Andor and Cairhien | Depopulated open grassland |
| Plain of Lances | Saldaea | Cavalry country |
| Plains of Maredo | Tear/Illian border | Contested plains |
| Proska Flats | Saldaea/Kandor border | Open flat terrain |
| Almoth Plain | Tarabon/Arad Doman border | Historically contested |
| Haddon Mirk | Western Tear | Dense, dark, dangerous swamp/forest |
| Braem Wood | North Andor | Dense forest between Andor and Arafel |
| Darkwood | Arad Doman interior | Forest near Almoth Plain |
| Drowned Lands | East of Godan, Tear | Coastal marshlands |
| Termool Desert | Eastern Tear | Arid desert along the Spine |
| Lake Somal | Andor interior | Lake near Baerlon/Two Rivers area |
| Paetrinh Swamp | Arad Doman/Almoth area | Swampland |
| Kabal Deep | Altara/Illian coast | Coastal inlet |
| Fingers of the Dragon | South of Tear | Great river delta of the Erinin |
| Bay of Remara | East of Tear/near Mayene | Coastal bay |
| Field of Merrilor | Kandor/Borderland area | Site of great gathering before Last Battle |
| Polov Heights | Near Field of Merrilor | Tactical high ground |
| Valley of Thakan'dar | Below Shayol Ghul | Where Shadowspawn are forged |

---

## ISLANDS & COASTAL FEATURES

| Location | Sea/Ocean | Notes |
|----------|-----------|-------|
| Tar Valon | Erinin (river island) | City built on large island |
| Tremalking | Aryth Ocean | Large Seanchan-held island; Choedan Kal (female) was here |
| Aile Dashar | Aryth Ocean | Island group west of Saldaea coast |
| Aile Somera | Aryth Ocean | Island south of Aile Dashar |
| Aile Jafar | Aryth Ocean | Island group, southern Aryth Ocean |
| Dantor | near Aile Jafar | Small island |
| Qaim | Sea of Storms | Small island, central southern coast |
| Cindaking | Sea of Storms | Island east of Tear |
| Toman Head | Aryth Ocean coast | Peninsula; Falme is here |
| World's End | Aryth Ocean coast | Northwestern tip of the continent |
| Windbiter's Finger | Sea of Storms coast | Southern coastal peninsula |

---

## AIEL WASTE (for reference)

| Location | Notes |
|----------|-------|
| Rhuidean | Hidden city of the Aiel; in the southwest Waste; where Wise Ones and clan chiefs are tested |
| Chaendaer | Mountain near Rhuidean |
| Alcair Dal | Meeting place of the Aiel clans; large canyon gathering site |
| Cold Rocks Hold | Clan hold in the Waste; Rhuarc's hold |
| Imre Stand | Location in the Waste |
| Jangai Pass | Western entry from Cairhien into the Waste |
| The Silk Path | Ancient trade route through the Waste connecting Cairhien to Shara |

---

## TWO RIVERS — EXPANDED STARTER DETAIL

*Zone block 1000–1199. Default level_range 1–5. Default sector `field`
(override per room). Climate `temperate`.*

### Emond's Field

The largest village of the Two Rivers; sits at the junction of the North Road
(to Watch Hill / Taren Ferry) and the Old Road (to Deven Ride). Set in
rolling fields between the Westwood and the Waterwood, with the Winespring
flowing south through the village.

| Location | Notes |
|----------|-------|
| The Green | Central commons; Bel Tine bonfire site; village gatherings. |
| Winespring Inn | Bran al'Vere's inn on the Green; common-room hearth, taproom, kitchens, upstairs guest rooms, stable yard. **Recommended starting room.** |
| The Winespring | Spring at the edge of the Green; source of the Winespring Water (locally "the Water"). |
| The Mayor's house | Bran al'Vere's home, attached to the inn. |
| The Wisdom's cottage | Nynaeve's cottage on the edge of the village; herb garden. |
| Smith's forge | Master Luhhan's smithy on the Quarry Road. |
| Cauthon farm | Outside the village proper, on the Old Road south. |
| al'Thor farm | West of village toward the Westwood; remote sheep farm. |
| The Quarry Road | Leads west out of Emond's Field toward the Mountains of Mist. |
| Old Road / North Road junction | Village center; signpost. |

### Watch Hill

Northern village; sits on a low rise overlooking the surrounding fields. Has
its own small green and inn. Defensive lookout point during Trolloc raids.

### Deven Ride

Southern village; quiet farming community. Smaller than Emond's Field.

### Taren Ferry

Northernmost settlement; sits on the south bank of the Taren river. Operated
by the Ferry family. The only practical crossing out of the Two Rivers to
the north — a rope-and-barge ferry. Reputation among other Two Rivers folk
is mildly disreputable ("Taren Ferry folk").

### The Westwood

Dense forest west of Emond's Field, climbing toward the Mountains of Mist.
Sector `forest`. Wolves, bandits in later content. Hides multiple
trail-heads into the mountains.

### The Waterwood

Wetland forest east of Emond's Field along the Winespring's course. Sector
`forest` with low-lying water rooms (`water`). Source of reeds, fish; site
of childhood adventures in canon.

### Mountains of Mist (Two Rivers approaches)

Western wall of the Two Rivers. Sector `mountain`. Trail-heads from the
Westwood lead to the **ruins of Manetheren** at higher altitude (see below).

---

## MANETHEREN (Fallen Nation — Ruins)

*Zone block draws from Andor's 1xxx range (suggested 1800–1899). Default
level_range 15–25. Sector `mountain`. Climate `mountain`.*

The lost kingdom of which the Two Rivers is the only surviving fragment.
Destroyed in the Trolloc Wars roughly 2000 years before the series. The
**city of Manetheren** itself stood high in the Mountains of Mist; the king
Aemon and queen Eldrene died defending it, and the One Power lash Eldrene
unleashed scoured the city to bare rock.

### Buildable Locations

| Location | Notes |
|----------|-------|
| Ruins of Manetheren | Shattered foundations on a high plateau; Sounding Stone fragments. |
| Aemon's Hall | Collapsed throne room; rumored cache of Age-of-Legends remnants. |
| Eldrene's Tower | Toppled spire; saidar-touched stone, lingering wards. |
| Mountain trails | Connect Westwood approach → ruins → high-altitude passes. |
| Hidden ways | Optional links to Manetheren-era stedding (see Stedding index). |

This is positioned as a **mid-game exploration zone** for Two Rivers-trained
characters who outgrow the starter content. Tone: melancholic, haunted.

---

## WAYGATES (Cross-Reference Index)

Waygates open onto **The Ways** — see dedicated section below. All
Waygates in canon were sealed by the Aes Sedai with an Avendesora leaf,
though many seals have since been broken or corrupted by Machin Shin.

| Waygate | Nation / Location | Status (canon) | Buildable in MUD |
|---------|-------------------|----------------|------------------|
| Caemlyn Ogier Grove | Andor (Caemlyn) | Sealed; broken in series | Yes |
| Tar Valon Ogier Grove | Tar Valon | Sealed | Yes |
| Cairhien Ogier Grove | Cairhien city | Sealed; broken in series | Yes |
| Illian Ogier Grove | Illian city | Sealed | Yes |
| Tear Ogier Grove | Tear city | Sealed | Yes |
| Fal Dara Keep | Shienar | Sealed; broken in series | Yes |
| Manetheren | Mountains of Mist (ruins) | Lost / collapsed | Optional (lore) |
| Stedding Shangtai | Tear/Termool border | Operational (Ogier-tended) | Yes |
| Stedding Tsofu | Cairhien-adjacent | Operational | Yes |

**Builder note:** Waygates are room-level features (not zones). The Way they
open onto is the off-world zone block 30000–30999. Use a one-way exit
flagged as a Waygate to enter; coming out is symmetric. See ROADMAP for
travel-system wiring.

---

## STEDDINGS (Cross-Reference Index)

Steddings are Ogier sanctuaries where the One Power cannot be touched and
Shadowspawn cannot enter. Mechanically distinct (planned sector `stedding`).

| Stedding | Region | Notes |
|----------|--------|-------|
| Stedding Shangtai | Tear / Termool border | Loial's home stedding. Contains Waygate. |
| Stedding Tsofu | Cairhien-adjacent | Closest to Tar Valon politically; visited in series. |
| Stedding Sherandu | Mountains of Mist (canon: lost) | Optional ruins. |
| Stedding Cantoine | Black Hills (Saldaea/Andor border) | Optional. |
| Stedding Taishin | Spine of the World | Optional. |
| Stedding Mardoon | Mountains of Mist (south) | Optional. |

All stedding rooms suppress channeling and Shadowspawn pathing (mechanical
hooks land with the planned `stedding` sector).

---

## THE WAYS (Off-World Realm)

*Zone block 30000–30999. Default level_range 20–35. Default sector
`underground`. Climate `timeless`.*

Built by male Ogier and male Aes Sedai during the Age of Legends, the Ways
are an enclosed reality of Bridges and Islands accessed via Waygates.
Distance compresses inside — a day's walk can equal hundreds of leagues
outside — but the taint on saidin corrupted the Ways during the Breaking,
and **Machin Shin** ("the Black Wind") now haunts them. Light fails in the
Ways; only Avendesora-leaf-bearers and certain Ogier songs hold it back.

### Structure

- **Islands** — flat platforms, one per Waygate. Named by Guidings (carved
  signposts) in the Ogier script.
- **Bridges** — ramps and arches connecting Islands. Some are broken; some
  are inverted (gravity flipped).
- **Guidings** — every Island has a Guiding stone; readable only by Ogier
  in canon (gameplay accommodation TBD).

### Hazards

- **Machin Shin** — random encounter, lethal. Drains sanity / soul.
- **Broken Bridges** — fall damage; some lead to dead-ends (deletes the
  party, or vents into a Portal Stone-style mirror — design TBD).
- **Time slip** — exiting a Waygate may land you hours, days, or weeks off
  from your entry.

The Ways are intentionally an **endgame travel system**, not a casual
shortcut. Most Waygates are sealed; breaking a seal is a major event.

---

## PORTAL STONES & MIRROR WORLDS (Deferred)

*Zone block 31000–31999. **Design deferred** — see ROADMAP.md.*

Portal Stones are Age-of-Legends artifacts predating even the Aes Sedai.
Tall cylindrical stones carved with symbol rows; activating one (requires
channeling, or a ta'veren of sufficient strength) transports the user to:

1. A different Portal Stone in the real world (instant travel), or
2. A **mirror world** — an alternate parallel reality where history
   diverged. Mirror worlds vary in stability; weaker ones decay around the
   traveler.

Locations of known Portal Stones (canon):
- **Aiel Waste** (multiple, including near Rhuidean)
- **Almoth Plain**
- **Tear / Plains of Maredo**
- **Toman Head**

**MUD design notes (open):** mirror worlds are well-suited as instanced
content, optional alt-history zones, or one-shot adventure spaces. No
schema or commands designed yet. Tracked in ROADMAP.

---

## FACTION ROSTER (Per Nation)

One-line entries to anchor mob_template builders. Expand inline as factions
are mechanically wired.

### Andor
- **Queen's Guard** — royal military, Caemlyn-based; red coats.
- **Children of the Light (Whitecloaks)** — present but unwelcome; patrols.
- **The Black Tower** — Asha'man order outside Caemlyn (male channelers).
- **Two Rivers Bowmen** — local militia, longbow-armed.

### Cairhien
- **Cairhienin Lancers** — royal cavalry, Sun Palace.
- **House factions (Daes Dae'mar)** — Damodred, Riatin, Saighan, others.
- **Aiel (Shaido + others, post-series)** — occupying force in some periods.

### Tear
- **Defenders of the Stone** — Tear's elite garrison.
- **High Lords / Lords of the Land** — ruling council.

### Mayene
- **Winged Guards** — Mayene's elite cavalry.
- **First's household** — Berelain sur Paendrag's court.

### Illian
- **The Companions** — Illian's elite military.
- **Council of Nine** — governing nobility.

### Murandy
- **King's Guard (nominal)** — Lugard.
- **Independent lords' levies** — Murandy is fragmented.

### Far Madding
- **City Guard** — enforces the Guardian's no-channeling zone.
- **Hall of the Counsels** — twelve ruling Counsels.

### Altara
- **Winged Guards (Altaran)** — palace guard, Ebou Dar.
- **The Kin** — secretive women who can channel but aren't Aes Sedai.
- **Seanchan occupation forces** — post-Knife of Dreams.

### Ghealdan
- **Crown Guard** — Jehannah.
- **Dragonsworn** — Prophet's followers (Masema's movement).

### Amadicia
- **Children of the Light** — true power; Fortress of the Light, Amador.
- **Questioners** — Whitecloak inquisitors.

### Tarabon
- **Panarch's Legion** — Tanchico.
- **King's Lifeguard** — Tanchico.
- **Dragonsworn / Seanchan** — both contest the country.

### Arad Doman
- **Council of Merchants** — economic power; rivals the king.
- **King's army** — Bandar Eban.
- **Dragonsworn** — significant presence.

### Tar Valon
- **Aes Sedai (Seven Ajahs + Black Ajah)** — the White Tower.
- **Warders** — bonded swordsmen.
- **Tower Guard** — secular military.

### Saldaea
- **Saldaean Light Cavalry** — famed for steppe warfare.
- **House Bashere** — premier noble house.

### Kandor
- **Kandori Lancers** — Borderlander cavalry.
- **Merchant guilds** — Kandor's economy is trade-driven.

### Arafel
- **Twin-blade swordsmen** — Arafellin warriors fight with two blades, bells in hair.
- **Border keeps** — extensive Blight fortifications.

### Shienar
- **Lances of Shienar** — heavy cavalry; topknot warriors.
- **Fal Dara garrison** — premier border keep.

### Aiel Waste
- **Twelve clans + societies** — Taardad, Shaarad, Goshien, Reyn, Tomanelle,
  Daryne, Miagoma, Codarra, Nakai, Shiande, Chareen, Shaido. Societies cross
  clan lines (Far Dareis Mai, Stone Dogs, Red Shields, etc.).
- **Wise Ones** — channeling and dream-walking authority.
- **Clan chiefs** — political authority.

### Malkier (fallen)
- **Lan Mandragoran** — uncrowned king; lone Malkieri.
- **Returned malkieri** — gathering at the Seven Towers in late series.
- **Blight-spawn occupiers** — Shadowspawn, Darkfriends.

### The Blight / Shayol Ghul / Blasted Lands
- **Forsaken** — thirteen named Chosen of the Dark One.
- **Myrddraal (Fades)** — eyeless half-men, lieutenants.
- **Trollocs** — beast-soldiers; tribes Dha'vol, Dhai'mon, Ahf'frait, etc.
- **Draghkar, Gholam, Worms, Jumara** — varied Shadowspawn.
- **Darkfriends (Friends of the Dark)** — human servants of the Shadow.
- **Black Ajah** — corrupted Aes Sedai.
- **Sharans (post-series)** — Shara emerges with the Shadow's host.

### Seanchan (off-continent / occupation)
- **Deathwatch Guard** — Empress's elite.
- **Sul'dam + damane** — channeler-handlers and a'dam-bound channelers.
- **Banner-Generals / Captains-General** — military hierarchy.

---

*Document compiled from map analysis of the official Last Battle Westlands map + series knowledge. Use for MUD zone/room grid building.*
