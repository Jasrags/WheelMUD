# Coords anchors and BFS

Migration 0026 added `rooms.coords_auto` (1 = derived by BFS, 0 =
anchor authored in YAML). The auto-coords pass walks exits from
anchors and stamps `(x, y, z)` on auto rooms.

## When to anchor (`coords_auto: 0`)

- One **per disconnected sub-graph**. If your zone has two
  unconnected exit clusters, both need an anchor.
- Cross-zone bridge rooms — the room a stair/portal lands in. Pin
  it so the BFS walk in the new zone has a defined origin.
- Landmark rooms whose grid position matters for `zonemap` — town
  square, throne room, harbor mouth.

## When NOT to anchor

Most rooms. The BFS is faster and less brittle than hand-authoring
coordinates. Authoring 200 rooms with hand coords is how you ship a
broken minimap.

## Direction → delta

Standard MUD assumption (already encoded in the BFS):

| Dir | dx | dy | dz |
|---|---|---|---|
| n | 0 | -1 | 0 |
| s | 0 | +1 | 0 |
| e | +1 | 0 | 0 |
| w | -1 | 0 | 0 |
| u | 0 | 0 | +1 |
| d | 0 | 0 | -1 |
| ne | +1 | -1 | 0 |
| nw | -1 | -1 | 0 |
| se | +1 | +1 | 0 |
| sw | -1 | +1 | 0 |

Inout exits are zero-delta and are skipped by the BFS.

## Conflicts

If two exit paths assign different coords to the same room, the BFS
flags an `issue` reachable via the `coords` admin verbs (`coords
issues`). Fix by re-anchoring or by deleting the inconsistent exit
(usually a typo: a one-way that should have been two-way).

## Hidden / nomap rooms

Migration 0020 added `rooms.nomap` so the player-facing `map` BFS
hides secret hideouts and admin zones. Anchors still work; the room
is just suppressed from the player render.

## Admin verbs

`coords rebuild` / `coords show <room-id>` / `coords issues` (see
`internal/cmd/coords.go`). Privileged; refusal paths do not audit.
