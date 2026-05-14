-- mapper.lua — feeds Mudlet's built-in mapper from gmcp.Room.Info.
--
-- Server contract (internal/gmcp/packages.go::RoomInfo):
--   { num = int64, name = string, zone = string,
--     exits = { [shortdir] = int64, ... }, desc = string, extra = {} }
--
-- The strategy is "walk-and-build": every time the player enters a
-- room we ensure that room exists in Mudlet's map, stitch the exit
-- back from the previous room, place the new room at the canonical
-- compass offset from the previous one, and centerview() so the map
-- window scrolls with the player. If a room already exists, we just
-- re-center.
--
-- Mudlet's mapper window opens on demand via openMapWidget() — we
-- don't force it open here; the player can keep it closed.

WheelMUD = WheelMUD or {}
WheelMUD.mapper = WheelMUD.mapper or {}

-- exitOffsets is the canonical cardinal-direction → (dx, dy, dz)
-- vector. Mudlet places the new room at (prev.x + dx, prev.y + dy,
-- prev.z + dz). Diagonal-only zones may produce overlapping rooms
-- that the player needs to clean up manually (documented in README).
local exitOffsets = {
  n  = { 0,  1,  0}, s  = { 0, -1,  0},
  e  = { 1,  0,  0}, w  = {-1,  0,  0},
  ne = { 1,  1,  0}, nw = {-1,  1,  0},
  se = { 1, -1,  0}, sw = {-1, -1,  0},
  u  = { 0,  0,  1}, d  = { 0,  0, -1},
}

-- reverseDir maps a short direction to its opposite so the exit
-- stitched back from prev → new also gets stitched new → prev when
-- the player walks back through it.
local reverseDir = {
  n = "s", s = "n", e = "w", w = "e",
  ne = "sw", sw = "ne", nw = "se", se = "nw",
  u = "d", d = "u",
}

-- ensureArea creates a Mudlet area (mapper region) with the given
-- name on first use and returns its id. Mudlet area ids are stable
-- per profile so a returning player keeps their auto-built map.
local function ensureArea(name)
  if not name or name == "" then return 1 end
  local id = getAreaTable()[name]
  if id then return id end
  return addAreaName(name)
end

-- ensureRoom creates the room if Mudlet doesn't know it yet and
-- stamps name + area. coords (if supplied) are applied for new
-- rooms; existing rooms keep their current coords so a player's
-- hand-tweaked map isn't clobbered on re-entry.
local function ensureRoom(num, name, areaName, coords)
  local existed = (getRoomName(num) ~= nil)
  if not existed then
    addRoom(num)
    if coords then setRoomCoordinates(num, coords[1], coords[2], coords[3]) end
  end
  setRoomName(num, name or "Unnamed")
  local areaID = ensureArea(areaName)
  setRoomArea(num, areaID)
  return existed
end

-- stitchExit creates the from → to exit on Mudlet's map. Mudlet's
-- setExit/setExitStub API distinguishes cardinal directions (via
-- numeric ids) from custom names. We use setExit() for the cardinal
-- short-dirs and let Mudlet handle the rendering.
local cardinalExitMap = {
  n  = 1,  ne = 2,  nw = 3,  e  = 4,
  w  = 5,  s  = 6,  se = 7,  sw = 8,
  u  = 9,  d  = 10,
}

local function stitchExit(fromNum, dir, toNum)
  local code = cardinalExitMap[dir]
  if code then
    setExit(fromNum, toNum, code)
  end
end

function WheelMUD.mapper.onRoomInfo()
  local r = gmcp and gmcp.Room and gmcp.Room.Info
  if not r or not r.num or r.num == 0 then return end

  local newNum = r.num
  local prevNum = WheelMUD.mapper.lastRoom
  local areaName = (r.zone ~= "" and r.zone) or "wheelmud"

  -- If we have a previous room and we walked through a known exit,
  -- place the new room at the cardinal offset from prev. Otherwise
  -- (first room of the session, or a teleport) let Mudlet default
  -- to (0,0,0).
  local coords = nil
  local walkedDir = WheelMUD.mapper.lastWalkDir
  if prevNum and walkedDir and exitOffsets[walkedDir] then
    local px, py, pz = getRoomCoordinates(prevNum)
    if px ~= nil then
      local off = exitOffsets[walkedDir]
      coords = { px + off[1], py + off[2], pz + off[3] }
    end
  end

  ensureRoom(newNum, r.name, areaName, coords)

  -- Stitch the exit we walked through, both forward and reverse
  -- (so the back-walk shows the same edge instead of a new one).
  if prevNum and walkedDir then
    stitchExit(prevNum, walkedDir, newNum)
    local rev = reverseDir[walkedDir]
    if rev then stitchExit(newNum, rev, prevNum) end
  end

  -- Stitch all known outbound exits from the new room so the map
  -- shows them as stubs even before the player walks them.
  if r.exits then
    for dir, dest in pairs(r.exits) do
      if cardinalExitMap[dir] and dest and dest ~= 0 then
        -- setExit on an unmapped destination silently no-ops in some
        -- Mudlet versions; addRoom() pre-creates a stub the player
        -- will fill in by walking there.
        if getRoomName(dest) == nil then
          addRoom(dest)
          setRoomArea(dest, ensureArea(areaName))
        end
        stitchExit(newNum, dir, dest)
      end
    end
  end

  WheelMUD.mapper.lastRoom = newNum
  WheelMUD.mapper.lastWalkDir = nil
  centerview(newNum)
end

-- onCommand records the last cardinal-direction command the player
-- typed so we can place the new room at the right offset when the
-- next Room.Info arrives. Mudlet's `sysDataSendRequest` fires for
-- every line sent to the server.
function WheelMUD.mapper.onCommand(event, line)
  if not line then return end
  -- Normalize common direction aliases ("north" → "n", etc.).
  local short = ({
    n = "n", north = "n",
    s = "s", south = "s",
    e = "e", east = "e",
    w = "w", west = "w",
    ne = "ne", northeast = "ne",
    nw = "nw", northwest = "nw",
    se = "se", southeast = "se",
    sw = "sw", southwest = "sw",
    u = "u", up = "u",
    d = "d", down = "d",
  })[line:lower():gsub("^%s+", ""):gsub("%s+$", "")]
  if short then
    WheelMUD.mapper.lastWalkDir = short
  end
end

if WheelMUD.registerHandler then
  WheelMUD.registerHandler("gmcp.Room.Info", "WheelMUD.mapper.onRoomInfo")
  WheelMUD.registerHandler("sysDataSendRequest", "WheelMUD.mapper.onCommand")
end
