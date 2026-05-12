-- Phase F #32 slice 5c demo: a fast feint that lands a follow-up
-- swing well under a second after the opening emote. wait_ms gives
-- authors sub-second pacing for combat flourishes that would feel
-- sluggish with the slice-5b 1-second wait floor.
--
-- Real firing precision is bounded by the scheduler tick rate
-- (1 Hz today), so the 400ms here rounds up to the next pulse;
-- finer-grained scheduler buckets are a separate followup. The
-- behavior still ships correctly — authors get the right ordering
-- and the right "delayed" feel; only the precision tightens later.
emote("twists into a feint, blade flashing low—")
wait_ms(400, "script_strike")
log("info", "quick_feint: scheduled script_strike for actor " .. tostring(ctx.actor_id))
