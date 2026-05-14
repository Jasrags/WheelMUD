-- init.lua — WheelMUD package bootstrap.
--
-- Mudlet sources package Lua files alphabetically after install,
-- which means init.lua runs after chat.lua / config.lua but before
-- mapper.lua / status.lua / vitals.lua. To avoid load-order fragility
-- every module sets up its own corner of the WheelMUD namespace
-- defensively (`WheelMUD = WheelMUD or {}`). This file's job is the
-- cross-cutting plumbing each module needs to register cleanup
-- callbacks at uninstall time.

WheelMUD = WheelMUD or {}

-- handlers holds every event-handler id returned by
-- registerAnonymousEventHandler. The uninstall hook iterates this
-- list and kills each handler so a reload doesn't leave orphaned
-- event subscriptions firing into deleted Geyser UI elements.
WheelMUD.handlers = WheelMUD.handlers or {}

-- ui collects every Geyser widget the package creates. The uninstall
-- hook hides + nils each so the visual layout returns to vanilla
-- Mudlet when the player removes the package.
WheelMUD.ui = WheelMUD.ui or {}

-- registerHandler is the canonical "subscribe + remember" wrapper.
-- Every module uses this rather than calling
-- registerAnonymousEventHandler directly so uninstall is uniform.
function WheelMUD.registerHandler(event, fn)
  local id = registerAnonymousEventHandler(event, fn)
  table.insert(WheelMUD.handlers, id)
end

-- registerUI is the equivalent for Geyser widgets — every module
-- registers its root container here so uninstall can hide() each.
function WheelMUD.registerUI(widget)
  table.insert(WheelMUD.ui, widget)
end

-- onUninstall fires when the player removes the package from
-- Mudlet's Package Manager. We tear down every event handler and
-- hide every UI widget so the screen returns to vanilla.
function WheelMUD.onUninstall(event, pkgName)
  if pkgName ~= "wheelmud" then return end
  for _, id in ipairs(WheelMUD.handlers or {}) do
    killAnonymousEventHandler(id)
  end
  for _, widget in ipairs(WheelMUD.ui or {}) do
    if widget and widget.hide then widget:hide() end
  end
  WheelMUD.handlers = {}
  WheelMUD.ui = {}
end

-- Wire the uninstall hook itself, plus stash the handler so a
-- reinstall doesn't accumulate duplicates of it.
if WheelMUD.uninstallHandler then
  killAnonymousEventHandler(WheelMUD.uninstallHandler)
end
WheelMUD.uninstallHandler = registerAnonymousEventHandler(
  "sysUninstallPackage", "WheelMUD.onUninstall")

cecho("\n<yellow>WheelMUD package loaded. Connect and watch the map fill in.\n")
