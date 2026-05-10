-- Phase F #32 slice 3 demo: hand a potion to the script's invoker.
-- The external_id "tr.potion_healing_draught" must exist in the world
-- (seeded by data/world/.../winespring_inn/items.yaml). give_item
-- clones the YAML template into the actor's inventory.
give_item(ctx.actor_id, "tr.potion_healing_draught")
log("info", "gift_potion: gave a healing draught to actor " .. ctx.actor_id)
