-- +migrate up
-- Tiny three-room starter zone. Explicit ids so application code can refer
-- to room 1 as the starter (see repo.StarterRoomID). Layout: Plaza in the
-- middle, North Road to the north, South Road to the south.
INSERT INTO rooms (id, name, short_desc, long_desc) VALUES
    (1, 'Town Plaza',
        'A bustling town plaza.',
        'Cobblestones radiate from a worn fountain in the centre of the plaza. Roads run away to the north and south, and townsfolk come and go without sparing you a second glance.'),
    (2, 'North Road',
        'A quiet stretch of road north of the plaza.',
        'The road narrows as it leaves the plaza, hemmed in by leaning timber houses. A faint smell of bread drifts from somewhere up ahead.'),
    (3, 'South Road',
        'A dusty road heading south from the plaza.',
        'Wagon ruts cut deep into the dry earth here. The plaza''s noise dwindles to a hum behind you.');

INSERT INTO exits (from_room_id, to_room_id, direction) VALUES
    (1, 2, 'n'),
    (2, 1, 's'),
    (1, 3, 's'),
    (3, 1, 'n');

INSERT INTO items (name, name_lower, short_desc, room_id) VALUES
    ('a small pebble',  'a small pebble',  'a smooth grey pebble lies on the cobbles', 1),
    ('a wooden sign',   'a wooden sign',   'a weathered wooden sign is staked beside the road', 2),
    ('a fallen leaf',   'a fallen leaf',   'a curling autumn leaf rests in the dust',  3);

INSERT INTO mobs (name, name_lower, short_desc, room_id) VALUES
    ('a town crier', 'a town crier', 'a town crier in a faded coat shouts the day''s news', 1);
