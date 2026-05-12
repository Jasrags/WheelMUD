-- Phase F #32 slice 5b demo: delayed retaliation. The trigger
-- fires this script with a 3s pause before the actual punishment
-- lands, giving the player a moment to react. The deferred
-- "script_strike" sees the same ctx (actor / room / event) the
-- original firing observed.
--
-- Note: ctx.actor_id is 0 for events that have no character actor
-- (e.g. an on_tick trigger). tostring() defends against a future
-- author wiring this to such an event so the log line stays
-- well-formed rather than coercing 0 to "0" silently.
emote("frowns at the slur and gathers warding fire.")
wait(3, "script_strike")
log("info", "wait_demo: scheduled script_strike for actor " .. tostring(ctx.actor_id))
