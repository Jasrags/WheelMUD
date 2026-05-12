-- Phase F #32 slice 5c demo: walk the actor's full nested
-- inventory (including container contents) and report each item's
-- external_id. Demonstrates inventory_all — same shape as the
-- slice-5b confiscate.lua but visits items hidden inside satchels,
-- pouches, and other containers.
--
-- Useful for guard-style triggers that want to find contraband no
-- matter how it's stashed. Pair with transfer_item / drop_item from
-- the slice-5a surface to actually confiscate.
local found = 0
for _, item in ipairs(inventory_all(ctx.actor_id)) do
    log("info", "deep_search: " .. item.external_id .. " (id=" .. tostring(item.id) .. ")")
    found = found + 1
end
emote("eyes you up and down, taking inventory of every pocket and pouch.")
log("info", "deep_search: scanned " .. tostring(found) .. " items for actor " .. tostring(ctx.actor_id))
