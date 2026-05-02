# What Follows in Shadow (Starter Adventure)

Reference extracted from *The Wheel of Time Roleplaying Game* (Chapter 15: What Follows in Shadow, pp. 300–315) for use in WheelMUD implementation. The chapter is a printed adventure module for 4–6 first-level characters set in Caemlyn during Logain's parade. This document captures it as a structured scenario that the MUD can host as canonical starter content, with implementation hooks.

## Setting

- **Location:** Caemlyn (Andor) and the surrounding outskirts.
- **Era:** During Rand al'Thor's first visit to Caemlyn — Logain has been captured and is being paraded through the city on his way to Tar Valon.
- **Background tensions in the city:**
  - The "winter that will not end" has driven thousands of refugees and travelers into the city.
  - Red cockades / armbands / sword-wrappings = pro-Morgase. White = anti-Morgase.
  - Children of the Light patrol, looking for "Darkfriends."
  - Trollocs and a Myrddraal have infiltrated the outskirts hunting Rand al'Thor.
  - **Padan Fain** is also stalking the area, looking for an item he believes can harm the Dragon Reborn.

## The Cloak (MacGuffin)

- A personal item carried by a single PC — a heirloom cloak by default, but any moderately-valued sentimental item works.
- Cosmetically: well-made, fur-lined, mildly unusual trim.
- Mechanically: imbued with One Power resonance during making (but not a true ter'angreal). Effect = exceptional comfort in cold weather; **not** protective against extreme environments.
- Padan Fain has misidentified the cloak's resonance and believes it can render the wearer invisible to the Dragon Reborn. He will pursue it relentlessly. The belief is wrong; the resonance is real but harmless.
- GM should plant the cloak's significance during character generation without telegraphing its plot weight.

## Scenes

### Scene 1: Welcome to the Golden Stag

- **Location:** Golden Stag Inn, in the warehouse district outside the city wall.
- **Setup:** Open-air stableyard with overflow tables, two bonfires, mixed crowd of red and white partisans kept in line by Master Ferrin's bouncers.
- **Trigger:** A red-vs-white shouting match is broken up by bouncer **Rol** throwing the white partisan through a shed door. Inside the shed: 3 Trollocs preparing to abduct **Sirene**, the stablemaster's daughter.

#### Trolloc statline
- Defending Trolloc: hp 18.
- Fleeing Trollocs (×2): hp 14, 16.
- Bagging Sirene takes the fleeing Trollocs a full round; they exit the rear of the shed at the start of round 2.

#### Mechanics
- **Spot DC 12** to notice the back-shed kidnapping.
- **Knowledge (Shadowspawn) DC 12** to recall Trollocs' light sensitivity (-2 attack in bright light) — a quick-thinking PC may light lanterns or channel light to apply the penalty. Bonfires alone are insufficient.
- Trolloc tactics: defending Trolloc holds the doorway, lets the kidnap pair escape, then fights to the death (no surrender, no flee).
- The Queen's Guard arrives ~10+ minutes after alarm — too late.

#### Aftermath
- Cleric NPC **Mere** has Heal +5 and one dose of healer's balm.
- If the players prevented the abduction, swap a happy reunion in for Scene 2 and provide an alternate hook (Master Ferrin posts a bounty to investigate the source).

### Scene 2: The Hunt

- **Hook:** Stablemaster Bennet pleads for rescue. Reward purse: Bennet's life savings (72 sm) + Ferrin (30 sm) = **102 sm**.
- **Padan Fain (in beggar disguise)** offers to lead the party to the Trolloc trail, hoping to get close to the cloak.

#### Tracking without the beggar
- Search DC 17 for tracks in the alleyway.
- With Track feat: Wilderness Lore DC 15 to follow.
- Without: Search DC 19.

#### Approach to the copse (¼ mile across a tilled field)
- Hedgerow approach: Hide vs. Trolloc Spot. Darkness penalties offset by Trolloc low-light vision.
- Ravine / pasture-bushes approach: each PC makes one Hide check **+4 circumstance** (inattentive Trollocs).

#### The Hideout
- 3 Trollocs (the same fleeing pair plus a third); hp 14, 16, 19.
- Trollocs ambush from cover (Hide vs. PC Spot) on PC entry to the copse.
- **Tactics:** fight fiercely; attempt to flee only when **all three are below half HP**. They abandon Sirene's sack on retreat.
- If players prevented the kidnapping in Scene 1, only the third Trolloc (hp 19) is here.

#### Aftermath
- Returning to the inn (success or failure):
  - Mere tends wounds again.
  - Lieutenant Jerman (Queen's Guard) interviews witnesses, dispatches more Guards, hauls Trolloc bodies away for burning.
  - Master Ferrin upgrades the heroes to second-floor private rooms at the front of the inn for the rest of their stay.
- **Padan Fain's first cloak grab** — the beggar approaches the cloak-bearing hero asking for the cloak as a reward "to warm myself." Ferrin offers blankets and a stable loft instead. Fain reluctantly releases the cloak and slips off into the night. If the hero hands over the cloak voluntarily, the adventure essentially ends — go to Epilogue.

### Scene 3: The False Dragon

- **Plot day:** Logain's parade through Caemlyn.
- **Whitecloak encounter:** Underlieutenant Arlvin (Spot +4) plus 3 Whitecloaks accosts the heroes looking for "backcountry bumpkins" matching Two Rivers descriptions.
  - Hide opposed by Arlvin's Spot +4 to slip away.
  - Or initiative-then-run (the crowd opens to slow the Whitecloaks; only those who act before the Whitecloaks can slip away cleanly; failed Hide → Whitecloak grapples).
  - PCs claiming Two Rivers origin are detained until the Queen's Guard arrives and Arlvin backs off.
- **Whitecloaks (4):** hp 9, 7, 7, 6.

#### The parade
- Lines of pikemen + thousands of soldiers + a master ward weave dome over Logain's iron cage. Disrupting it is functionally impossible: ranged attacks bounce off the dome.
- Aes Sedai (8, two per corner) escort. Twelve Warders flank the wagon in fancloth cloaks.
- Logain himself is staged as a defiant figure who briefly meets the cloak-bearer's eyes.

#### A Watcher
- **Spot DC 14** to notice **Grom Fetchit** (commoner, hp 3) tailing them. He flees on eye contact.
- Pursuit: **Gather Information DC 14** to learn his name & nearby home.
- Fetchit watches the road from an upper window; flees out the back if approached openly. Stealthy approach: Hide / Disguise vs. his Spot.
- **Intimidate DC 11** to make him talk. Reveals: paid by Whitecloaks to track the party; targets are Two Rivers folk.

### Scene 4: The Beggar Returns

- The beggar tries to deliver a fake message from Lt. Jerman ("captain wants to talk to ya"), leading the heroes to an empty New City storefront flying an Andoran banner.
- **Sense Motive vs. Fain's Bluff** to feel something off.
- **Spot DC 16** to notice the "soldier" inside has a frayed tabard.

#### The ambush
- 4 Thugs total: 3 inside the empty storeroom + 1 in the antechamber stairwell to cut off retreat. hp 7, 6, 6, 4.
- Fain does not fight; he slinks off to disappear in the crowd.
- **Intimidate DC 13** vs. captured thugs reveals: paid by the beggar — 5 gc each + 5 gc more on completion; subdue heroes, strip non-cash possessions, deliver everything to the beggar; rendezvous unspecified.

#### Variant: confronting the beggar in the alley
- A PC who tails Fain instead of entering the building can corner him.
- Hide vs. Fain's Spot to follow without being noticed; if noticed, Fain works winding evasion (Hide vs. Spot).
- In the alley, Fain plays dumb, then springs a sap attack — Sense Motive vs. Bluff to avoid being flat-footed; a flat-footed PC eats Fain's sneak attack +2d6.
- Passersby trigger the alarm; Guards respond after **2 rounds** — Fain flees if not dropped.

#### Variant: ignoring the beggar
- Fain gathers the thugs and tails the party. Spot vs. Hide to notice. Bluff vs. Sense Motive then Hide vs. Spot to lose them; otherwise the thugs ambush as soon as the party enters a secluded area.

#### Variant: no opportunity at all
- Fain skips this step and proceeds directly to Scene 5.

### Scene 5: Shadows in the Darkness

- **Time:** Just before dawn, the morning after Scene 4.
- **Fain's break-in:**
  - Climb DC 18 to scale to the second-floor window.
  - Open Lock DC 14 to lift the shutter bar with a thin dagger.
  - If shutters are reinforced, Fain enters elsewhere.

#### Watch & detection
- Last-watch PC: Concentration DC 15 to stay awake (+4 with Endurance feat). Failure → dozing → Listen at -5 to skill modifier (no d20 roll for sleepers).
- Sleeping PC: Listen check uses skill modifier only (no d20).
- Awake PC in cloak room: Spot +4 vs. Fain's first Move Silently as he opens the window/door.

#### Action sequence
- Fain enters at <½ speed (no movement penalty).
- If spotted, he drops pretense and lunges for the cloak; running past a defender provokes one or two AoOs (one to grab, one to flee).
- Movement note (rules anchor for the scene): drawing a weapon or rising from a bed is a move action; can be combined with one (not two) attack provided the PC doesn't move >5 ft.

#### Pursuit
- Fain has the **Run** feat → 150 ft/round.
- Run-feat PCs match pace and stay 120 ft behind.
- Non-Run PCs lose 30 ft/round; line-of-sight breaks at ~3 rounds.
- Fain reaches his target in **10 rounds** (~420 ft lead at the end).
- **Bennet** (stablemaster) intercepts Fain at the inn gate, nicks him with a knife and leaves a blood trail.
- Two **Search DC 12** checks to follow blood trail in sequence; failures cost a few moments but allow retry.

#### The cellar
- Hidden door in an alley off a New City merchant block.
- Open lock + sturdy interior bar (or a fresh-rusty padlock from the alley) — bar from inside to delay the 12 pursuing Whitecloaks "several minutes."
- Inside: brick pillars, baggage at the foot of the stairs (incl. torch bundle), broken barrels.
- **Search DC 13** to recognize fresh tracks of multiple humans, pack animals, and Trollocs across the dust → all vector toward a stone double door.
- The stone wall holds a **Waygate** — Knowledge (arcana) **DC 22** to identify (+4 racial bonus for Ogier).
- **Whitecloaks (12):** hp 8, 7, 7, 6, 6, 6, 6, 6, 5, 5, 4, 4. Direct combat with all 12 is a likely TPK; ducking inside and barring the door is the intended path.

### Scene 6: The Dark Along the Ways

- **Crossing the Waygate:** the inner film feels like a ribbon of ice but offers no resistance. Inside: pitted, ancient stone, an arching white-stripe path, and audible echoing footsteps from Fain's party.
- **Light required.** Cellar torches or channeled light. Light radius is halved (Ways rule from `other-worlds.md`).

#### Following Fain
At each pedestal junction, the party picks a route:

**Listen track:**
- First check **DC 10**. If all PCs fail, retry once at **DC 12**.
- Each subsequent island reduces DC by **-2** (chasing closer).
- When DC drops to **4**, the party catches Fain → go to Shadows and Shadowspawn.

**Wilderness Lore + Track:**
- Constant **DC 19**. Does not scale.
- Failure can be retried by backtracking.
- Bridges may be crossed at speed; tracking checks happen only on islands.
- Successful chase catches Fain on the **fourth island**; failures stretch this out.

Choosing a wrong bridge with no other tracking method = lost in the Ways → epilogue.

#### Shadows and Shadowspawn (final encounter)
- **Padan Fain (transformed):** disheveled traveling clothes; cloak bundled atop his baggage. hp 43.
- 2 Trollocs: hp 18, 15.
- **Surprise:** PC Spot DC 8 vs. Trolloc/Fain Listen DC 10 to determine surprise on entry to the platform.

##### Tactics

- Trollocs hold the foot of the heroes' bridge, no more than 10 ft apart.
- Fain stays at the back, urging Trollocs forward in round 1.
- If a hero rushes past the Trollocs, Fain drops his torch, draws a short sword, and engages.
- **Withdraw triggers** (one per round, Fain checks them in order):
  - One Trolloc drops, **or**
  - Fain has taken **>15 dmg** → he begins withdrawing.
  - At **≥20 dmg** → full-round disengage and flee up the rear bridge.
  - At **<20 dmg** → one attempt to grab the cloak (provokes AoOs from threatening PCs). On a hit landing, he abandons the grab. Either way, 5-ft step + total defense for the rest of his turn, then disengage next round.

##### The leap
- If pursued to the apex of the next bridge, Fain shouts "I don't need the cloak to kill al'Thor!" and jumps to a barely-visible lower bridge. **Jump DC 15** for him.
- A pursuing PC may try the same. Success: lands and may continue pursuit. **Failure: endless plummet through the Ways → eventual death.** The book strongly recommends letting Fain go.

## Epilogue

- **Recovery success path:** PCs left in the Ways. Three **Intelligence DC 15** checks to find their way back to Caemlyn. Failure routes them to a Waygate of the GM's choice (anywhere in the world) — explicit hook into *Prophecies of the Dragon*, which begins at Toman Head.
- **No Waygate entry:** Fain escapes for good and discards the cloak in the Ways once he realizes it isn't a true ter'angreal. Heroes still have to deal with the 12 Whitecloaks outside the cellar.
- **Cloak voluntarily handed over:** adventure short-circuits at Scene 2 aftermath.

## Rewards

- **Total party XP: 1,000** for completing the adventure (split among active participants), regardless of whether the cloak was recovered or Fain was defeated.

## Antagonist Statblocks

### Padan Fain (Caemlyn-tier; weaker than the Chapter 11 ver.)

```
Midlander Wanderer 10
HD 10d6  HP 43
Init +3  Defense 20 (+7 class, +3 Dex)  Spd 30
Atk +9/+4 melee (1d4 dagger / 1d6 short sword / 1d6 subdual sap), +10/+5 ranged
Special Attacks: illicit barter, sneak attack +2d6
Saves: Fort +4 / Ref +11 / Will +8   Rep 4
Str 14  Dex 17  Con 10  Int 16  Wis 12  Cha 13   CC: E
Skills: Balance +7, Bluff +11, Climb +13, Diplomacy +11, Disguise +7, Escape Artist +7,
  Gather Information +9, Hide +9, Intimidate +13, Intuit Direction +5, Jump +11,
  Knowledge (Shadowspawn) +7, Listen +8, Move Silently +9, Open Lock +12,
  Pick Pocket +5, Profession (merchant) +7, Ride +6, Search +7, Sense Motive +7,
  Spot +12, Swim +4, Tumble +6, Use Rope +5
Feats: Alertness, Bullheaded, The Dark One's Own Luck, Luck of Heroes, Persuasive,
  Run, Skill Emphasis (Intimidate), Skill Emphasis (Knowledge [Shadowspawn]), Stealthy
Possessions: Dagger, sap, short sword, backpack, 12 torches, 34 sm, 41 gc
```

### Whitecloak (Midlander Armsman 1)

```
HD 1d10  HP 6 avg
Init +1  Defense 16 (+5 full chain, +1 Dex)  Spd 30
Atk +3 melee (1d8+2 longsword)
Saves: Fort +3 / Ref +2 / Will +0   Rep 0
Str 15  Dex 12  Con 13  Int 10  Wis 10  Cha 11   CC: B
Skills: Climb +6, Intimidate +4, Jump +6, Ride +5
Feats: Endurance, Mounted Combat
```

### Grom Fetchit (Midlander Commoner 1)

```
HD 1d4  HP 3
Init +0  Defense 10  Spd 30
Atk +0 melee (1d4 dagger)
Saves: Fort -1 / Ref +0 / Will +0
Str 10  Dex 11  Con 9  Int 13  Wis 11  Cha 9   CC: A
Skills: Craft (brewing) +4, Listen +4, Spot +4
Feats: Alertness, Skill Focus (Craft [brewing])
```

### Thug (Midlander Warrior 1)

```
HD 1d8  HP 7 avg
Init +0  Defense 14 (+4 chain shirt)  Spd 30
Atk +3 melee (1d8+2 longsword)
Saves: Fort +6 / Ref +0 / Will +0   Rep 0
Str 14  Dex 10  Con 15  Int 10  Wis 10  Cha 12   CC: A
Skills: Intimidate +4, Jump +4
Feats: Great Fortitude, Run
```

### Trolloc bestiary reference
See `encounters.md` for canonical Trolloc statlines; this adventure assigns specific HP rolls per encounter.

## Implementation Notes (WheelMUD)

- **Treat as a packaged starter quest:** model the chapter as a single `Adventure` aggregate with six `Scene` nodes plus an Epilogue branch. Each scene exposes triggers, NPC roster, success/failure transitions, and per-PC item dependencies (the Cloak).
- **The Cloak as personal-item plot hook:** add a "personal-significance" tag to character-creation items (`heroic-characteristics.md` already provisions for sentimental gear). The adventure spawner picks any tagged item from any party member; otherwise prompts the storyteller to designate one. The cloak's mechanical properties: cosmetic + slight cold-weather comfort + flagged `onePowerResonance: harmless`. Fain's AI keys off the resonance flag.
- **Scene 1 trap:** combat encounter with 3 Trollocs and a noncombatant victim (Sirene). Outcome of Spot/Knowledge/light-source play feeds modifiers into the Trolloc statline. Add a `lightSensitivityActive` flag on Trolloc NPCs that bumps to true when ambient light is forced bright. Track Sirene as a ChildNPC with abduction state: `safe / sacked / rescued / fed`.
- **Scene 2 chase mechanics:** generic search-and-pursuit subsystem reused throughout. Encode the "Hedgerow / Ravine / Pasture-bushes" choices as a route-graph with per-route Spot/Hide modifiers.
- **Scene 3 parade:** model as a scripted citywide event with a `parade.route` POI list. The Logain wagon is a moving room with a `MasterWardDome` indestructibility flag (ranged attacks return harmlessly). Disruption attempts emit narrative reactions but no real progress.
- **Whitecloak detain logic:** Arlvin's encounter has a `Two Rivers origin` flag check — branching dialog that escalates to detention if PCs claim TR origin; mob-rescue and Queen's Guard intervention scripted at fixed beats. Reuse for similar Whitecloak harassment encounters elsewhere.
- **Fetchit shadow:** a Spot-gated hidden NPC tail. Add a `WatcherNPC` template that fires on PC entry to a region and dies / flees on detection.
- **Fain ambush:** chained encounter with three branches (follow → ambush, intercept → alley fight, ignore → tail). All three terminate at "Fain reaches Scene 5." Use a shared `AmbushOutcome` resolver so each branch updates the world identically.
- **Scene 5 break-in:** support per-room "watch" assignments at sleep time; expose Concentration DC 15 (with Endurance +4 modifier) as a status check the watching PC rolls automatically each watch. Sleepers' Listen drops d20 and uses skill modifier only — codify this rule in the perception subsystem.
- **Run-feat pursuit:** standardize a foot-chase resolver with `runner.speed`, `pursuer.speed`, `lead`, and a per-round delta. Bennet's interception and the bloodstain trail are scripted cutscenes: Bennet provides the visible blood-trail arrow, and Search DC 12 chains feed back into the pursuit resolver.
- **Cellar Waygate identification:** the "Knowledge (arcana) DC 22" check is the canonical Waygate-recognition DC from `other-worlds.md`. Reuse the helper.
- **Whitecloak siege of the cellar:** rather than running 12 NPCs in a TPK encounter, the adventure routes through the bar-the-door beat. Implement a "scene gate" that requires the bar to be set; otherwise the Whitecloaks engage and combat resolution favors them mathematically (and the adventure flags as failed).
- **Scene 6 Ways crawl:** the Ways subsystem already exists from `other-worlds.md`. The adventure encodes:
  - A custom override of the Listen DC schedule (`10 → 12 retry → step down by 2 each island`) reaching the 4-DC threshold to catch Fain.
  - A constant Wilderness Lore + Track DC 19 alternative.
  - The fourth-island catch under tracking.
- **Final platform fight:** scripted multi-stage encounter with thresholded behavior. Fain's withdraw logic should be a state machine keyed on `(troll1Down OR troll2Down)`, `damageTaken`, and `cloakReachable`.
- **Endless-plummet jump:** explicitly model the Jump DC 15 + endless-fall failure mode. UI needs to warn the player ("This is the Ways — failed jumps are likely fatal") rather than silently killing.
- **Epilogue routing:** the "lost in the Ways → GM-chosen Waygate" hook makes a clean handoff to *Prophecies of the Dragon* (Toman Head). For WheelMUD: provide an admin-configurable `nextAdventure` Waygate so the lost-in-Ways state can teleport the party into the next staged scenario.
- **NPC seed data:** include the four supporting statblocks (Padan Fain Caemlyn-tier, Whitecloak Armsman 1, Grom Fetchit, Thug Warrior 1) in the standard NPC seed catalog. Tag Fain here as `padanFain.caemlyn` to avoid colliding with the higher-level `padanFain.modern` from `encounters.md`.
- **Reward grant:** flat 1,000 XP split among active participants on adventure completion (any branch). Use the same `awardXP` helper as `gamemastering.md` short-adventure tier.
- **Replay safety:** mark the adventure as a one-shot hook that is consumed when run for a given party — once the cloak has been resolved (recovered, lost, or surrendered) the spawn flags `completed`.
- **Telemetry:** log per-scene branch outcomes (kidnapping prevented? Fain ambush triggered? Waygate breached?) for storyteller review; useful when tuning the difficulty curve for new players.
- **Voluntary cloak surrender:** short-circuit handling — if the cloak is given over in Scene 2 aftermath, jump to Epilogue without spawning Scenes 3–6.
- **Crowd safety net:** the crowd-and-Queen's-Guard mob mechanic recurs (Scene 3 Whitecloaks, Scene 4 alley fight, Scene 5 cellar barricade). Implement a generic "mob-witnesses-call-the-Guards" timer (default 2 rounds) so other adventures can reuse it.
