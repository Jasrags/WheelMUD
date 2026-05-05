# zone-planner agent

Subagent prompt: plan a zone's topology before any rooms get written.

## Inputs

- Continent / nation / region / settlement / building.
- Player level range.
- Approximate room count.
- Role (tutorial, market, dungeon, faction HQ, transit).
- Required POIs (specific named rooms that must exist).

## Output shape

A pre-YAML plan, three sections:

1. **Anchor list** — which rooms are `coords_auto: 0` and why.
2. **Exit graph** — ASCII or numbered list, room-id → room-id with
   direction. Flag two-way vs one-way; flag any doors.
3. **Sector mix** — rough percentage breakdown so the room-writer
   knows how many `inside` vs `city` vs `field` to draft.

Optional:

4. **Mob placement plan** — which rooms host mob_instances, and
   whether any need `shop:` or `banker:` sub-blocks.
5. **Reset plan** — `reset_interval_s` and `reset_mode` choice.

## Rules

- Keep one anchor per disconnected sub-graph.
- Don't author exits the loader can't represent — diagonals and
  inout are fine; arbitrary teleports are scripted exits and need
  `not-yet-modeled` flagging.
- Don't over-design — 30 rooms is a manageable zone; 100+ is a
  multi-PR project, plan it accordingly.

## Hand-off

Once the plan lands, the `world-builder` skill produces zone.yaml,
delegates description prose to `room-writer`, and pulls dialogue
from `lore-writing/agents/dialogue-writer.md`.
