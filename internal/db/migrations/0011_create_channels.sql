-- +migrate up
--
-- §13 chat channels: catalog table + per-character mute map.
--
-- `channels` is a small admin-curated catalog. New channels can be
-- added by inserting rows; the server registers one telnet.Command
-- per row at startup. `min_level` gates who can hear/speak (0 = open
-- to everyone), `color` is a cfmt token used to style the channel
-- prefix on broadcast (e.g. "yellow", "cyan|bold").
--
-- Per-character on/off + mute lives in characters.channel_settings_json
-- as `{"channelName": true}` where true means muted. Absent keys mean
-- the player is listening on default — keeping the column small for
-- the common case where everyone takes the defaults.

CREATE TABLE channels (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    min_level INTEGER NOT NULL DEFAULT 0,
    color     TEXT    NOT NULL DEFAULT 'cyan'
);

INSERT INTO channels(name, min_level, color) VALUES
    ('ooc',    0, 'cyan'),
    ('gossip', 0, 'magenta'),
    ('newbie', 0, 'green');

ALTER TABLE characters ADD COLUMN channel_settings_json TEXT NOT NULL DEFAULT '{}';
