-- 001_init.sql — initial schema for the Life RPG MVP.
-- All timestamps are stored as RFC3339 TEXT (UTC). attribute_rewards / suggested_quest
-- are stored as JSON TEXT. XP is always auditable: totals live on `attributes`, but every
-- change is also recorded as a row in `xp_events`.

CREATE TABLE users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE attributes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    key        TEXT NOT NULL,
    name       TEXT NOT NULL,
    total_xp   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE(user_id, key)
);
CREATE INDEX idx_attributes_user ON attributes(user_id);

CREATE TABLE quests (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id              INTEGER NOT NULL REFERENCES users(id),
    title                TEXT NOT NULL,
    description          TEXT NOT NULL DEFAULT '',
    type                 TEXT NOT NULL DEFAULT 'daily',      -- daily, weekly, main, side, boss, recovery
    difficulty           TEXT NOT NULL DEFAULT 'easy',       -- trivial, easy, medium, hard, boss
    status               TEXT NOT NULL DEFAULT 'active',     -- active, completed, skipped, archived
    attribute_rewards    TEXT NOT NULL DEFAULT '{}',         -- JSON: {"strength":40,"discipline":10}
    skip_count           INTEGER NOT NULL DEFAULT 0,
    source_suggestion_id INTEGER,
    created_at           TEXT NOT NULL,
    completed_at         TEXT,
    due_date             TEXT
);
CREATE INDEX idx_quests_user_status ON quests(user_id, status);
CREATE INDEX idx_quests_type ON quests(user_id, type);

CREATE TABLE quest_completions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id),
    quest_id     INTEGER NOT NULL REFERENCES quests(id),
    xp_awarded   INTEGER NOT NULL DEFAULT 0,
    completed_at TEXT NOT NULL
);
CREATE INDEX idx_completions_user ON quest_completions(user_id, completed_at);

CREATE TABLE xp_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id),
    attribute_key TEXT NOT NULL,
    amount        INTEGER NOT NULL,
    source        TEXT NOT NULL,           -- quest, manual, ...
    source_id     INTEGER,                 -- e.g. quest id
    note          TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL
);
CREATE INDEX idx_xp_events_user ON xp_events(user_id, created_at);
CREATE INDEX idx_xp_events_attr ON xp_events(user_id, attribute_key, created_at);

CREATE TABLE streaks (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL UNIQUE REFERENCES users(id),
    current_count    INTEGER NOT NULL DEFAULT 0,
    longest_count    INTEGER NOT NULL DEFAULT 0,
    last_active_date TEXT
);

CREATE TABLE journal_entries (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    mood       INTEGER NOT NULL,    -- 1..10
    energy     INTEGER NOT NULL,    -- 1..10
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX idx_journal_user ON journal_entries(user_id, created_at);

CREATE TABLE agent_suggestions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL REFERENCES users(id),
    type             TEXT NOT NULL,                  -- low_attribute, level_up_focus, make_easier, recovery
    title            TEXT NOT NULL,
    reason           TEXT NOT NULL DEFAULT '',
    suggested_quest  TEXT NOT NULL DEFAULT '{}',     -- JSON quest template
    status           TEXT NOT NULL DEFAULT 'pending',-- pending, accepted, dismissed
    created_quest_id INTEGER,
    source_quest_id  INTEGER,
    created_at       TEXT NOT NULL,
    resolved_at      TEXT
);
CREATE INDEX idx_suggestions_user_status ON agent_suggestions(user_id, status);
