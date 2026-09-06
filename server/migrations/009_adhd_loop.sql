-- ADHD loop, phase 0 (additive only).
-- Loot buffs become "24 hours or 3 uses": NULL keeps every pre-existing row
-- unlimited, so nothing already dropped changes behavior.
ALTER TABLE user_buffs ADD COLUMN IF NOT EXISTS uses_left INTEGER;

-- Streak auto-mend: a one-day gap is bridged for free at most once per
-- 7 days; the date of the last mend gates the cooldown.
ALTER TABLE streaks ADD COLUMN IF NOT EXISTS last_mend_date TEXT;
