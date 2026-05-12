-- Phase F #32 slice 5a demo: deal raw damage to the firing actor.
-- Hooked from a room on_say trigger keyed on a forbidden word, or
-- from a dialogue effect after a hostile response. The 6 hp delta is
-- a flat value (no DR / resists / crit) — script authors who want
-- elemental should layer apply_affect with a DoT effect instead.
emote("crackles with sudden warding fire")
deal_damage(ctx.actor_id, 6, "warding_ward")
log("info", "script_strike: dealt 6 damage to actor " .. ctx.actor_id)
