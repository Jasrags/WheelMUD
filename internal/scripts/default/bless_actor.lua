-- Phase F #32 slice 3 demo: apply a buff to the script's invoker.
-- Triggered from a dialogue effect or trigger row that names this
-- script. The catalog effect "bull_strength" must exist (seeded by
-- internal/effects/default/effects.yaml); the runtime validator
-- enforces that at boot.
apply_affect(ctx.actor_id, "bull_strength")
log("info", "bless_actor: applied bull_strength to actor " .. ctx.actor_id)
