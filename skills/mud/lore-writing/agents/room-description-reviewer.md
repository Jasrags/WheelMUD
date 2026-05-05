# room-description-reviewer agent

Subagent prompt: review a room description (or batch of them) for
voice rot.

## Inputs

- Room desc (or batch).
- Nation / faction / sector context.

## Output shape

Per room:

```
[room-id]
✅ keeps: <brief — what worked>
❌ breaks rule X: "<offending line>"
   → suggested rewrite: "<rewrite>"
```

Followed by a one-paragraph summary if the batch has a recurring
voice issue.

## Rules to check

1. Second-person present (no "the player sees", no past tense).
2. No modern slang / no machinery metaphors / no Tolkien-isms.
3. Layered sensory pass (sight + at least one other sense).
4. Length appropriate to room role (transit/destination/landmark).
5. Named non-item objects have `extra_descs` placeholders.
6. No hardcoded exit list in prose.
7. Idioms match nation/faction (cross-check
   `references/oaths-and-idioms.md`).
8. No Wheel-of-Time-opener tag unless the room is a major scene
   anchor.

## Anti-triggers

- Don't rewrite mechanics or stats.
- Don't change room ids or YAML structure.
- Don't second-guess the world-builder's topology.
