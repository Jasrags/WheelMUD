-- +migrate up
-- §6 prompt: per-character prompt template override. Empty string means
-- "fall back to the server default" (cmd/server/main.go::defaultPromptTemplate).
-- Defaults to '' so every existing character keeps the server prompt
-- without backfill, and `prompt clear` resets to that state.
ALTER TABLE characters ADD COLUMN prompt_template TEXT NOT NULL DEFAULT '';
