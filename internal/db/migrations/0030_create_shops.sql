-- 0030_create_shops.sql
--
-- Shopkeeper subsystem (§14). Two tables:
--
--   shops        — config keyed 1:1 to a mob_template (the shopkeeper).
--                  Carries pricing knobs (sell_markup / buy_markdown),
--                  the buy_types whitelist, and operating hours.
--   shop_stock   — per-shop inventory. Each row points at an item
--                  template by external_id (TEXT) — the buy verb
--                  materializes a fresh items row from that template.
--                  qty == -1 is the "infinite stock" sentinel for
--                  staple goods (torches, bread).
--
-- Schema choices:
--   * mob_template_id is a soft FK; UNIQUE so one template = one shop.
--   * buy_types_json is a JSON array of ItemType strings. Empty array
--     means "shop refuses every sell". Validated by the loader and
--     by sell-time code, not the DB.
--   * sell_markup defaults to 1.0 (price = item.value). buy_markdown
--     defaults to 0.5 (the half-price rule for non-trade-good items).
--   * open_hour == close_hour means always open (24h shop).
--   * restock_interval_s on shops is the default; per-stock rows can
--     ride it. last_restock_ts stays per-row so each line refills on
--     its own schedule.
--   * No FKs on shop_stock.shop_id — shops can be re-loaded by the
--     world loader (drop/recreate); the loader does its own cleanup.
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE shops (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    mob_template_id     INTEGER NOT NULL UNIQUE,
    buy_types_json      TEXT    NOT NULL DEFAULT '[]',
    sell_markup         REAL    NOT NULL DEFAULT 1.0,
    buy_markdown        REAL    NOT NULL DEFAULT 0.5,
    open_hour           INTEGER NOT NULL DEFAULT 0,
    close_hour          INTEGER NOT NULL DEFAULT 0,
    restock_interval_s  INTEGER NOT NULL DEFAULT 3600
);

CREATE TABLE shop_stock (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    shop_id             INTEGER NOT NULL,
    item_external_id    TEXT    NOT NULL,
    qty                 INTEGER NOT NULL,
    qty_max             INTEGER NOT NULL,
    last_restock_ts     INTEGER NOT NULL DEFAULT 0,
    UNIQUE(shop_id, item_external_id)
);

CREATE INDEX shop_stock_shop_idx ON shop_stock(shop_id);
