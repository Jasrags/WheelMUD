-- §18 news/MOTD: per-character "newest entry I've seen" watermark.
-- Stored as unix seconds; 0 means "never seen" so every existing
-- character treats every seeded news entry as unread on first login
-- after this migration. Reading an entry with `news <id>` updates
-- the watermark via CharacterRepo.MarkNewsSeen.
ALTER TABLE characters ADD COLUMN last_news_seen INTEGER NOT NULL DEFAULT 0;
