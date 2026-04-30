-- +migrate up
CREATE TABLE accounts (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    username            TEXT    NOT NULL,
    username_lower      TEXT    NOT NULL UNIQUE,
    password_hash       TEXT    NOT NULL,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at       DATETIME,
    failed_login_count  INTEGER NOT NULL DEFAULT 0,
    locked_until        DATETIME
);

CREATE INDEX accounts_locked_until_idx
    ON accounts(locked_until)
    WHERE locked_until IS NOT NULL;
