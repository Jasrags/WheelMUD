-- chat.lua — routes gmcp.Comm.Channel.Text frames into per-channel
-- mini-consoles on the right edge of the main window.
--
-- Server contract (internal/gmcp/packages.go::CommChannelText):
--   { channel = string, talker = string, text = string }
--
-- Channels are created lazily on first frame so unused channels
-- stay invisible. tell.in and tell.out collapse into one "tells"
-- pane with directional arrows on the talker prefix.

WheelMUD = WheelMUD or {}
WheelMUD.chat = WheelMUD.chat or {}

-- channelColor maps a canonical channel name to its display color
-- (used both for the channel header line and for inbound talker
-- highlight). Unknown channels fall through to a neutral cyan.
local channelColor = {
  say    = "white",
  ooc    = "cyan",
  gossip = "yellow",
  newbie = "green",
  tells  = "magenta",
}

local function colorFor(channel) return channelColor[channel] or "cyan" end

-- channelDisplayName normalises tell.in / tell.out → "tells" so
-- both halves of a private message land in the same pane.
local function channelDisplayName(raw)
  if raw == "tell.in" or raw == "tell.out" then return "tells" end
  return raw
end

-- buildContainer lazily constructs the right-side VBox that houses
-- the per-channel mini-consoles. Width is a fixed 28c column so the
-- main output window isn't cramped on a narrow display.
local function buildContainer()
  if WheelMUD.ui.chatBox then return end
  WheelMUD.ui.chatBox = Geyser.VBox:new({
    name = "WheelMUD.chatBox",
    x = "-28c", y = 0, width = "28c", height = "-4c",
  })
  if WheelMUD.registerUI then WheelMUD.registerUI(WheelMUD.ui.chatBox) end
end

-- ensureChannel returns the mini-console for the given canonical
-- channel name, creating it on first call. New consoles inherit the
-- main window font + a dark background for contrast.
local function ensureChannel(name)
  WheelMUD.chat.channels = WheelMUD.chat.channels or {}
  if WheelMUD.chat.channels[name] then
    return WheelMUD.chat.channels[name]
  end
  buildContainer()
  local mc = Geyser.MiniConsole:new({
    name = "WheelMUD.chat." .. name,
    width = "100%", height = "20%",
    color = "black",
    fontSize = 10,
  }, WheelMUD.ui.chatBox)
  mc:setBackgroundColor(0, 0, 0, 220)
  mc:echo("[" .. name:upper() .. "]\n")
  WheelMUD.chat.channels[name] = mc
  return mc
end

-- formatTalker builds the per-line "speaker:" prefix. tell.in gets
-- a leftward arrow (← Bob: …) so the player can tell at a glance
-- who initiated; tell.out gets rightward (→ to Bob: …).
local function formatTalker(rawChannel, talker)
  if rawChannel == "tell.in" then return "← " .. talker end
  if rawChannel == "tell.out" then return "→ " .. talker end
  return talker
end

function WheelMUD.chat.onText()
  local f = gmcp and gmcp.Comm and gmcp.Comm.Channel and gmcp.Comm.Channel.Text
  if not f or not f.channel then return end

  local pane = channelDisplayName(f.channel)
  local mc = ensureChannel(pane)
  local color = colorFor(pane)
  local talker = formatTalker(f.channel, f.talker or "")
  local text = f.text or ""

  -- Render: "[hh:mm] <color>talker:</color> text\n"
  local ts = os.date("%H:%M")
  mc:cecho(string.format("<grey>[%s]</grey> <%s>%s:</%s> %s\n",
    ts, color, talker, color, text))
end

if WheelMUD.registerHandler then
  WheelMUD.registerHandler("gmcp.Comm.Channel.Text", "WheelMUD.chat.onText")
end
