-- vitals.lua — drives the HP/SP gauges from gmcp.Char.Vitals.
--
-- Server contract (internal/gmcp/packages.go::CharVitals):
--   { hp = int32, maxhp = int32, sp = int32, maxsp = int32 }
--
-- The gauges live in a bottom-anchored Geyser.HBox. Idempotent on
-- container creation so reloading the package doesn't duplicate
-- widgets — we check `WheelMUD.ui.vitalsBox` before instantiating.

WheelMUD = WheelMUD or {}
WheelMUD.vitals = WheelMUD.vitals or {}

-- buildUI lazily constructs the bar container + the two gauges.
-- The status label (see status.lua) anchors to the same vitalsBox
-- so the header and the gauges share a single bottom panel.
function WheelMUD.vitals.buildUI()
  if WheelMUD.ui.vitalsBox then return end

  WheelMUD.ui.vitalsBox = Geyser.HBox:new({
    name = "WheelMUD.vitalsBox",
    x = 0, y = "-3c", width = "100%", height = "3c",
  })
  if WheelMUD.registerUI then WheelMUD.registerUI(WheelMUD.ui.vitalsBox) end

  WheelMUD.ui.hpGauge = Geyser.Gauge:new({
    name = "WheelMUD.hpGauge",
    width = "50%", height = "100%",
  }, WheelMUD.ui.vitalsBox)
  WheelMUD.ui.hpGauge.front:setStyleSheet([[
    background-color: #b22222;
    border: 1px solid #2a0808;
  ]])
  WheelMUD.ui.hpGauge.back:setStyleSheet([[
    background-color: #2a0808;
    border: 1px solid #2a0808;
  ]])
  WheelMUD.ui.hpGauge:setValue(100, 100, "HP")

  WheelMUD.ui.spGauge = Geyser.Gauge:new({
    name = "WheelMUD.spGauge",
    width = "50%", height = "100%",
  }, WheelMUD.ui.vitalsBox)
  WheelMUD.ui.spGauge.front:setStyleSheet([[
    background-color: #1e3a8a;
    border: 1px solid #0b1840;
  ]])
  WheelMUD.ui.spGauge.back:setStyleSheet([[
    background-color: #0b1840;
    border: 1px solid #0b1840;
  ]])
  WheelMUD.ui.spGauge:setValue(100, 100, "SP")
end

-- hpFrontColor picks an HP-bar fill color by percentage. Red below
-- 25% gives the player an unmistakable danger cue; green at full;
-- yellow in the wounded band.
local function hpFrontColor(pct)
  if pct < 25 then return "#b22222" end
  if pct < 60 then return "#d4a017" end
  return "#2e7d32"
end

-- onUpdate fires on every gmcp.Char.Vitals event. It builds the UI
-- on first call (idempotent), recolors the HP bar by current
-- percentage, and stamps the numeric label in the bar's overlay.
function WheelMUD.vitals.onUpdate()
  WheelMUD.vitals.buildUI()
  local v = gmcp and gmcp.Char and gmcp.Char.Vitals
  if not v then return end

  local hpMax = (v.maxhp and v.maxhp > 0) and v.maxhp or 1
  local spMax = (v.maxsp and v.maxsp > 0) and v.maxsp or 1
  local hp = v.hp or 0
  local sp = v.sp or 0

  local hpPct = math.floor(100 * hp / hpMax)
  WheelMUD.ui.hpGauge.front:setStyleSheet(string.format([[
    background-color: %s;
    border: 1px solid #2a0808;
  ]], hpFrontColor(hpPct)))
  WheelMUD.ui.hpGauge:setValue(hp, hpMax, string.format("HP %d/%d", hp, hpMax))
  WheelMUD.ui.spGauge:setValue(sp, spMax, string.format("SP %d/%d", sp, spMax))
end

-- Register the event subscription. WheelMUD.registerHandler stashes
-- the id so uninstall cancels it.
if WheelMUD.registerHandler then
  WheelMUD.registerHandler("gmcp.Char.Vitals", "WheelMUD.vitals.onUpdate")
end
