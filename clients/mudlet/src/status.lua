-- status.lua — renders the character header label from
-- gmcp.Char.Name and gmcp.Char.Status.
--
-- Server contracts:
--   gmcp.Char.Name (internal/gmcp/packages.go::CharName):
--     { name = string, fullname = string }
--   gmcp.Char.Status (internal/gmcp/packages.go::CharStatus):
--     { character_name, race, class, level, alignment }
--
-- Char.Name fires once on login; Char.Status fires on login + every
-- level-up + death/respawn. We cache both payloads and re-render the
-- label whenever either arrives so missing fields show as "—".

WheelMUD = WheelMUD or {}
WheelMUD.status = WheelMUD.status or {}

local function buildUI()
  if WheelMUD.ui.statusLabel then return end
  WheelMUD.ui.statusLabel = Geyser.Label:new({
    name = "WheelMUD.statusLabel",
    x = 0, y = "-4c", width = "100%", height = "1c",
  })
  if WheelMUD.registerUI then WheelMUD.registerUI(WheelMUD.ui.statusLabel) end
  WheelMUD.ui.statusLabel:setStyleSheet([[
    background-color: #1a1a1a;
    color: #e0e0e0;
    border-bottom: 1px solid #444;
    padding-left: 8px;
    font: 11pt "Consolas";
  ]])
  WheelMUD.ui.statusLabel:echo("WheelMUD")
end

-- render combines whatever the latest Char.Name and Char.Status
-- pushes have given us into a single line. Missing fields render as
-- "—" so a partial frame (e.g. Char.Status before Char.Name) still
-- produces useful output.
local function render()
  buildUI()
  local name = WheelMUD.status.name or "—"
  local s = WheelMUD.status.status or {}
  local race = s.race or "—"
  local class = s.class or "—"
  local level = s.level or 0
  WheelMUD.ui.statusLabel:echo(
    string.format("%s · %s %s, Level %d", name, race, class, level))
end

function WheelMUD.status.onName()
  local n = gmcp and gmcp.Char and gmcp.Char.Name
  if n then WheelMUD.status.name = n.name end
  render()
end

function WheelMUD.status.onStatus()
  local s = gmcp and gmcp.Char and gmcp.Char.Status
  if s then WheelMUD.status.status = s end
  -- Char.Status carries character_name redundantly with Char.Name;
  -- prefer Name's value but fall back if Name hasn't arrived yet.
  if not WheelMUD.status.name and s and s.character_name then
    WheelMUD.status.name = s.character_name
  end
  render()
end

if WheelMUD.registerHandler then
  WheelMUD.registerHandler("gmcp.Char.Name", "WheelMUD.status.onName")
  WheelMUD.registerHandler("gmcp.Char.Status", "WheelMUD.status.onStatus")
end
