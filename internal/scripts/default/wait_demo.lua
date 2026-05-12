-- Phase F #32 slice 5b demo: delayed retaliation. The trigger
-- fires this script with a 3s pause before the actual punishment
-- lands, giving the player a moment to react. The deferred
-- "script_strike" sees the same ctx (actor / room / event) the
-- original firing observed.
emote("frowns at the slur and gathers warding fire.")
wait(3, "script_strike")
log("info", "wait_demo: scheduled script_strike for actor " .. ctx.actor_id)
