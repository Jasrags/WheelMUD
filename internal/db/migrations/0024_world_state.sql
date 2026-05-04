-- +migrate up
-- §9 Room day/night cycle: single-row world clock storage. The dayclock
-- (internal/world/dayclock.go) computes ticks lazily from wall-clock,
-- this row only persists the base across restarts. Seeded at 675 so a
-- fresh server boots near "noon" — the midpoint of the day quarter
-- (450 dawn ticks + 225 = 675), giving builders a fully-lit world to
-- walk before the cycle drops them into dusk.
CREATE TABLE world_state (
    id    INTEGER PRIMARY KEY CHECK (id = 1),
    ticks INTEGER NOT NULL DEFAULT 675
);
INSERT INTO world_state (id, ticks) VALUES (1, 675);
