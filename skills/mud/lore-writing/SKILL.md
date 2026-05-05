---
name: lore-writing
description: Voice consistency across rooms, items, NPC dialogue, help topics, and MOTD entries — the Wheel of Time prose register. Defines Jordan's tells, nation flavor, faction speech, and oaths so every word the player reads sounds like it belongs in the same world.
triggers:
  - description voice
  - dialogue
  - in-game book
  - flavor
  - help topic prose
  - MOTD entry
  - faction speech
  - oath
  - idiom
  - Aes Sedai voice
  - Whitecloak
  - Asha'man
  - Forsaken
  - Aiel speech
  - Sea Folk
  - voice review
---

# lore-writing

## Role

Make every word the player reads sound like it belongs to the same
Wheel of Time. Owns voice; does not own structure (that's
`world-builder`) or mechanics (that's `wot-d20-rules`). Reviewing prose
for voice rot is as important as writing new prose.

## Core expertise

- **Jordan prose tells** — sentence rhythms, repeated cadences ("the
  Wheel weaves..."), idiomatic asides, unhurried scene-setting.
- **Nation flavor** — Andoran formality, Cairhienin Daes Dae'mar
  layering, Aiel directness, Saldaean fire, Tairen pride, Sea Folk
  bargaining cadence.
- **Faction speech** — Aes Sedai oath-bound formality, Whitecloak
  zealotry, Asha'man clipped command-voice, Forsaken velvet menace,
  Borderlander laconic warmth, Wise One bluntness.
- **Oaths and idioms** — "Light burn me," "Blood and ashes," "honor
  of …", "may the Light shine on you," "mother's milk in a cup,"
  "in the name of the Light." Used right these earn the setting;
  used wrong they sound like a tourist.

## Approach

When invoked:

1. Identify what's being written: room desc, item desc, NPC
   dialogue, help topic, MOTD, in-world book?
2. Identify the voice owner: nation? faction? individual NPC type?
3. Open the matching reference (nation/faction/voice).
4. Write or review prose. For reviews, flag the failing line + the
   voice rule it broke + a suggested rewrite.
5. Don't invent characters from canon; if a real Wheel of Time
   character would appear in the room, that's a separate canon
   decision (defer to the user).

## Clarifying questions

- Nation / faction / character archetype?
- Player-facing or NPC-internal? (room desc vs NPC monologue)
- Length budget? (one-liner? paragraph? in-world chapter?)
- Tone — neutral lore or charged with conflict?
- Is this canon-adjacent (named NPC) or original-character?

## Output formats

- **Prose draft** — ready to drop in.
- **Voice review** — line-by-line: ✅ keeps / ❌ breaks rule X /
  rewrite suggestion.
- **Idiom palette** — when seeding a new faction, a short list of
  oaths/idioms the writer can sample from.

## Dependencies

- `wot-d20-rules` — for any mechanic-aware flavor (e.g. a weave
  description must match the weave catalog).
- `world-builder` — receives prose, sets structure.

## Anti-triggers

- Does NOT write zone topology or YAML structure.
- Does NOT pick mechanics or stat lines.
- Does NOT invent canon characters or contradict canon.
- Does NOT use idioms outside their faction (a Whitecloak does not
  say "honor of"; an Aiel does not say "Light burn me").

## References

- `references/wot-voice-guide.md` — Jordan tells; common idioms;
  what to avoid.
- `references/nation-flavor.md` — Andoran, Cairhienin, Aiel,
  Saldaean, Tairen, Sea Folk, Domani, Tar Valon.
- `references/faction-speech.md` — Aes Sedai, Whitecloak, Asha'man,
  Forsaken, Wise Ones, Warders.
- `references/oaths-and-idioms.md` — sourced palette.

## Agents

- `agents/room-description-reviewer.md`
- `agents/dialogue-writer.md`
