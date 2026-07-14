-- 006_gold.sql — gold economy. Gold is the spendable twin of XP: every XP
-- award mints gold (1g per 10 XP, min 1), purchases spend it. gold_events is
-- the immutable ledger; the balance is ALWAYS computed as SUM(amount) on read
-- (same audit pattern as xp_events — never store a balance).

CREATE TABLE gold_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id),
    amount       INTEGER NOT NULL,               -- + mint, - purchase
    source       TEXT NOT NULL,                  -- quest, subtask, tool, journal, purchase, grant
    label        TEXT NOT NULL DEFAULT '',       -- human-readable ("quest · subtask" / item name)
    shop_item_id INTEGER,                        -- set for purchases
    created_at   TEXT NOT NULL
);
CREATE INDEX idx_gold_events_user ON gold_events(user_id, created_at);
CREATE INDEX idx_gold_events_source ON gold_events(user_id, source);

CREATE TABLE shop_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    name        TEXT NOT NULL,
    price       INTEGER NOT NULL,
    created_at  TEXT NOT NULL,
    archived_at TEXT                             -- archive, never delete (purchase history keeps labels)
);
CREATE INDEX idx_shop_items_user ON shop_items(user_id, archived_at);

-- One-time retroactive grant so the shop is usable on day one: users with
-- existing XP history get gold at the same 10:1 ratio. Fresh databases have no
-- xp_events yet at migration time (seed runs after), so this is a no-op there —
-- Seed() writes its own grant.
INSERT INTO gold_events(user_id, amount, source, label, created_at)
SELECT user_id, MAX(1, SUM(amount) / 10), 'grant', 'Retroactive grant for past XP',
       strftime('%Y-%m-%dT%H:%M:%f', 'now') || '000000Z'
FROM xp_events
GROUP BY user_id
HAVING SUM(amount) > 0;
