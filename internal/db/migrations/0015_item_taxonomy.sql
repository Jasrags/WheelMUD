-- +migrate up
-- §9 Item taxonomy. Adds the polymorphic stat-block columns the
-- gameplay layers (combat, inventory, economy) will need on top of
-- the bare Item row that look/take/drop already use.
--
-- - `type` is a closed enum; CHECK rejects unknown values so a typo
--   in YAML fails at load time rather than producing a silently
--   useless item. New types require a migration plus a corresponding
--   stats struct in the repo.
-- - `weight_lbs` and `value_cp` mirror the WoT equipment tables.
--   value_cp uses copper pennies (the existing currency.Amount base
--   unit) so display formatting stays in one place.
-- - `quality` discriminates masterwork / masterpiece / power-wrought
--   tiers that grant attack/AC/check-penalty bonuses elsewhere.
-- - `flags` is a bitset (notake / nodrop / nosell / bind_on_pickup /
--   magic / glow / hum / trade_good); INTEGER keeps the column
--   cheap and lets us grow the bitset without altering the schema.
-- - `stats_json` is the per-type sub-record (weapon damage dice,
--   armor bonuses, container capacity, light fuel, etc.). Empty
--   `{}` is valid for `trash` items that need no extras.
ALTER TABLE items ADD COLUMN type TEXT NOT NULL DEFAULT 'trash'
    CHECK (type IN (
        'weapon', 'armor', 'shield', 'container', 'consumable',
        'light', 'key', 'tool', 'clothing', 'food', 'trade_good', 'trash'
    ));
ALTER TABLE items ADD COLUMN weight_lbs REAL NOT NULL DEFAULT 0
    CHECK (weight_lbs >= 0);
ALTER TABLE items ADD COLUMN value_cp INTEGER NOT NULL DEFAULT 0
    CHECK (value_cp >= 0);
ALTER TABLE items ADD COLUMN quality TEXT NOT NULL DEFAULT 'normal'
    CHECK (quality IN ('normal', 'masterwork', 'masterpiece', 'power_wrought'));
ALTER TABLE items ADD COLUMN flags INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN stats_json TEXT NOT NULL DEFAULT '{}';
