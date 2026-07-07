-- 004_tool_entries.sql — "Tools" are guided instruments (e.g. the Daily Mood
-- Log) that award XP when completed. Each completion is stored with its
-- structured data as JSON; XP is auditable via xp_events (source='tool').

CREATE TABLE tool_entries (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    tool_key    TEXT NOT NULL,
    data        TEXT NOT NULL DEFAULT '{}',   -- JSON, tool-specific
    summary     TEXT NOT NULL DEFAULT '',     -- one-line history label
    xp_awarded  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_tool_entries_user ON tool_entries(user_id, tool_key, created_at);
