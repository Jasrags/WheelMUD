-- Phase F #32 slice 5b demo: find contraband by external_id in the
-- firing actor's inventory and transfer it to the guard mob that
-- fired the trigger (resolved via ctx.target_id). Demonstrates
-- inventory iteration composed with transfer_item. The external_id
-- below is a placeholder — wire to a real item in the QA zone for
-- live testing.
for _, item in ipairs(inventory(ctx.actor_id)) do
    if item.external_id == "tr.banned_weapon" then
        if ctx.target_id ~= 0 then
            transfer_item(item.id, ctx.target_id)
            emote("seizes " .. item.name .. " from your hands.")
        else
            -- No mob target on the firing event — drop it instead
            -- so the contraband doesn't linger in inventory.
            drop_item(item.id)
            emote("knocks " .. item.name .. " from your grip.")
        end
        return
    end
end
