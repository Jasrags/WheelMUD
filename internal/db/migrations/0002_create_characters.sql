-- +migrate up
CREATE TABLE characters (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name            TEXT    NOT NULL,
    name_lower      TEXT    NOT NULL UNIQUE,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_played_at  DATETIME
);

CREATE INDEX characters_account_id_idx ON characters(account_id);
