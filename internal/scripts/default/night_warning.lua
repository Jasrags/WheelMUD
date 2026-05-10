-- Phase F #32 slice 4 demo: only triggers a `say` line during
-- night hours (20:00–05:59 in-game). Exercises clock.hour().
local h = clock.hour()
if h >= 20 or h < 6 then
    say("The night holds dangers. Stay close.")
    log("info", "night_warning: fired at hour=" .. h)
end
