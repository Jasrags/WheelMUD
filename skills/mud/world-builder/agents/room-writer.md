# room-writer agent

Subagent prompt: produce a single room description (or a small batch)
that drops cleanly into a zone.yaml.

## Inputs the agent expects

- Zone id and room suffix.
- Sector + climate.
- Geographic context (which nation/region/settlement, what's
  next door).
- Time-of-day intent (if not auto).
- One sentence describing the room's role (transit / destination /
  landmark).
- Any required `extra_descs` keywords (objects you want lookable).

## Output shape

```yaml
- id: caemlyn:new-queens-blessing-common-room
  name: The Queen's Blessing — Common Room
  sector: inside
  description: >
    You stand in a low-ceilinged common room … (3–5 sentences).
  extra_descs:
    fireplace: >
      A wide stone hearth …
    landlord: >
      A round-faced Andoran with …
```

## Rules

- Second-person present (see `room-description-style.md`).
- 3–5 sentences for medium rooms; 2–3 for transit; 7+ only for
  landmarks (justify in PR body).
- Every named object that isn't a real item gets an `extra_descs`
  entry.
- Don't list exits in prose — the renderer does that.
- Pull voice from `lore-writing/references/wot-voice-guide.md`.

## Anti-triggers

- Don't write combat scripting.
- Don't invent NPC dialogue here — that's `lore-writing/agents/
  dialogue-writer.md`.
- Don't pick zone-level metadata (level_range, climate); those are
  the world-builder's call before invoking this agent.
