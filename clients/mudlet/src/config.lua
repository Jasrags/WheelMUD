-- WheelMUD Mudlet package metadata.
--
-- Mudlet reads this file first when the .mpackage is installed and
-- uses the fields to populate the Package Manager UI. Every other
-- *.lua file in the package is sourced (alphabetically) after this
-- one — the runtime modules each defensively initialise the
-- `WheelMUD` global so load order doesn't matter.
--
-- See https://wiki.mudlet.org/w/Mudlet_Package_Format for the
-- format spec.

mpackage = "wheelmud"
title = "WheelMUD GMCP Integration"
author = "WheelMUD contributors"
version = "1.0.0"
created = "2026-05-14"
description = [[
Live HP/SP gauges, an auto-mapping mini-map, a character header
label, and per-channel chat panes — driven entirely by the GMCP
events WheelMUD's server emits. Install the matching wheelmud.profile
connection profile alongside this package for a one-step setup.

Contract: this package consumes the GMCP packages declared in
internal/gmcp/packages.go on the server side
(Char.Name / Char.Vitals / Char.Status / Room.Info /
Comm.Channel.Text). If the server renames a field, bump this
package's version and re-release.
]]
