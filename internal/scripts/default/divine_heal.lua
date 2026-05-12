-- Phase F #32 slice 5a demo: restore HP to the firing actor. The
-- default narration line ("a warm light suffuses you...") comes
-- from the cmd-layer ScriptHealingApplied subscriber when the
-- script doesn't say/emote anything itself; we layer a flavor line
-- on top so the dialogue / trigger feels intentional.
say("Light's blessing upon you.")
heal(ctx.actor_id, 10)
log("info", "divine_heal: healed 10 hp to actor " .. ctx.actor_id)
