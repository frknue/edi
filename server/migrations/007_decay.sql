-- 007_decay.sql — decay & stakes. Neglected attributes bleed XP as negative
-- xp_events (source='decay'), floored at the XP threshold of (peak level - 2).
-- peak_xp is maintained in the same tx as every award (stored-but-auditable,
-- like total_xp: it must equal the running max of cumulative event sums).
-- Wards are gold-bought decay shields; windows matter historically (days
-- covered by a lapsed ward are still excluded from billing), so rows are
-- never deleted.

ALTER TABLE attributes ADD COLUMN peak_xp INTEGER NOT NULL DEFAULT 0;

-- Decay never existed before this migration, so current totals are the peaks.
UPDATE attributes SET peak_xp = total_xp;

CREATE TABLE wards (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id),
    attribute_key TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    created_at    TEXT NOT NULL
);
CREATE INDEX idx_wards_user_attr ON wards(user_id, attribute_key, expires_at);
