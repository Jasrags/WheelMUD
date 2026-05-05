# Room description style

Defers voice rules to `lore-writing`; this file is the
world-builder-specific shape.

## Tense + person

- **Second-person present.** "You stand at the edge of..."  Not "the
  player sees" or "you stood."
- Active voice. The room does things; you don't read about it.

## Layering

A good room description has three sensory passes (not necessarily in
order):

1. **Sight** — what dominates the visual frame.
2. **Sound** — what's audible, even subtle.
3. **Smell / touch / temperature** — one anchoring detail.

Skip the "weather" line for indoor rooms unless a window/leak makes
it relevant.

## Length

- **Short rooms** (2–3 sentences) for transitional spaces (hallway,
  street segment, forest path).
- **Medium rooms** (4–6 sentences) for destinations (inn common
  room, market square, throne room foyer).
- **Long rooms** (7+ sentences) reserved for landmark rooms (Heart
  of the Stone, Hall of the Tower). Use sparingly — players page
  through long rooms.

## Don't repeat the exit list

The exit list renders separately (cyan|bold "Exits:" row in
`look.go`). Don't end every description with "There are exits
north and east." The render handles it.

## Don't name objects you don't `extra_descs`

If you write "a sword leans against the wall," there had better be
an `extra_descs` entry for `sword` (or it's a real item). Players
will type `look sword` and get angry when nothing answers.

## Reference rooms

- `data/world/westlands/` — canonical examples already on disk.
- For new builds, write 2–3 rooms first, get them reviewed via
  `agents/room-writer.md`, then scale up.
