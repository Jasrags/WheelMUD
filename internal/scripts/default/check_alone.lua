-- Phase F #32 slice 4 demo: only blesses the actor when no other
-- players are in the room. Exercises room.players() (returns a
-- 1-indexed Lua table of character IDs in the actor's room) and
-- the slice-3 apply_affect hook.
local players = room.players()
if #players == 1 then
    apply_affect(ctx.actor_id, "bull_strength")
    log("info", "check_alone: blessed solo player " .. ctx.actor_id)
else
    log("info", "check_alone: skipped (room has " .. #players .. " players)")
end
