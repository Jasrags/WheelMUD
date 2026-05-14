# WheelMUD — Mudlet client integration

Drop-in Mudlet package that turns WheelMUD's GMCP wire data into a
real Mudlet experience: live HP/SP gauges, a character header label,
an auto-mapping mini-map, and per-channel chat panes.

## What you get

| Surface | Source GMCP package | Where it shows up |
|---|---|---|
| HP/SP gauges | `Char.Vitals` | Bottom of the Mudlet window |
| Character header | `Char.Name` + `Char.Status` | Above the gauges |
| Auto-mapping | `Room.Info` | Mudlet's built-in map window |
| Per-channel chat | `Comm.Channel.Text` | Right-side dockable pane |

No combat triggers, alias hotkeys, or affect tracking in v1 — those
ship later. See `docs/PLAN.md` for the roadmap.

## Install

You need two files:

- `wheelmud.profile` — a Mudlet connection profile.
- `wheelmud.mpackage` — the script package.

Build both from the repo root:

```
make mudlet-package
```

This drops the artifacts into `dist/mudlet/`. Override the host /
port at build time if you're shipping for a public server:

```
make mudlet-package WHEELMUD_HOST=mymud.example.com WHEELMUD_PORT=2323
```

Then in Mudlet 4.x or later:

1. **File → Import Profile** → pick `dist/mudlet/wheelmud.profile`.
   A "WheelMUD" entry appears in the connect list.
2. Click the WheelMUD profile and **Connect**. Log in normally.
3. **Toolbox → Package Manager → Install** → pick
   `dist/mudlet/wheelmud.mpackage`. The gauges appear at the bottom,
   the chat pane on the right, and the map window starts drawing
   rooms as you walk.

## Verifying GMCP frames are arriving

In Mudlet's input bar, type:

```
lua display(gmcp)
```

You should see a table containing `Char.Vitals`, `Char.Status`,
`Room.Info`, and (after someone speaks) `Comm.Channel.Text`. If
that table is empty, the server isn't sending GMCP and the package
won't do anything — verify the server side first (see
`internal/gmcp/` in the repo).

## Server-side contract

The Lua scripts depend on the exact field names declared in
`internal/gmcp/packages.go`. The contract today:

```go
CharName       { name, fullname }
CharVitals     { hp, maxhp, sp, maxsp }
CharStatus     { character_name, race, class, level, alignment }
RoomInfo       { num, name, zone, exits, desc, extra }
CommChannelText { channel, talker, text }
```

If a server-side rename ever changes one of these, bump
`config.lua`'s `version` and re-release. The package is small
enough (~6 files, ~250 lines) that field-rename churn is cheap.

## Known limitations (v1)

- **Diagonal-only zones** mis-place rooms. The mapper uses fixed
  compass offsets per direction; pure NE/NW/SE/SW chains stack
  diagonally and may overlap on screen. Use Mudlet's map editor
  to clean these up manually.
- **No room re-attribution.** If a builder reassigns a room's zone
  in YAML, the script doesn't move the existing map entry — it
  creates a new room at the new coordinate. Delete the stale one
  in Mudlet's map editor.
- **Tells fold into one pane.** Both `tell.in` and `tell.out`
  frames land in the same "tells" pane with an arrow prefix
  (`← Bob:` for inbound, `→ Bob:` for outbound). Some players
  prefer split panes; v2 will add a config flag.
- **Map persistence is Mudlet's.** The auto-built map saves with
  the Mudlet profile. Deleting the profile deletes the map. Export
  via Mudlet's map menu if you want to keep it.
- **Container width is fixed.** The chat pane is 28 character
  columns wide. On very narrow terminals this crowds the main
  output; resize Mudlet wider or hide the chat pane.

## Repo layout

```
clients/mudlet/
├── README.md               # this file
└── src/
    ├── config.lua          # package metadata (mpackage spec)
    ├── init.lua            # namespace bootstrap + uninstall hook
    ├── mapper.lua          # gmcp.Room.Info → Mudlet mapper
    ├── vitals.lua          # gmcp.Char.Vitals → HP/SP gauges
    ├── status.lua          # gmcp.Char.Name + gmcp.Char.Status → header
    ├── chat.lua            # gmcp.Comm.Channel.Text → per-channel pane
    └── profile.xml.template # connection profile with HOST/PORT placeholders
```

Build output lands in `dist/mudlet/` (git-ignored).

## Development notes

- Run `luac -p clients/mudlet/src/*.lua` for syntax-check without
  installing Mudlet. CI can use this without the GUI.
- Mudlet sources `.lua` files alphabetically inside a package, so
  every module defensively initialises its corner of the `WheelMUD`
  namespace (`WheelMUD = WheelMUD or {}`). Load order doesn't
  matter.
- `init.lua` owns the `sysUninstallPackage` hook that kills every
  event handler and hides every Geyser widget on package removal.
  New modules should register handlers via
  `WheelMUD.registerHandler(event, fn)` and widgets via
  `WheelMUD.registerUI(widget)` for uniform cleanup.
