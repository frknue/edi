-- 005_quest_subtasks.sql — optional "bonus objectives" on a quest. Checking a
-- subtask before completing the quest awards its own attribute_rewards as extra
-- xp_events (in the same completion transaction). Subtasks never block completion.

CREATE TABLE quest_subtasks (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER NOT NULL REFERENCES users(id),
    quest_id          INTEGER NOT NULL REFERENCES quests(id),
    title             TEXT NOT NULL,
    attribute_rewards TEXT NOT NULL DEFAULT '{}',  -- JSON bonus, e.g. {"health":15}
    done              INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL
);
CREATE INDEX idx_subtasks_quest ON quest_subtasks(quest_id);
CREATE INDEX idx_subtasks_user ON quest_subtasks(user_id);
