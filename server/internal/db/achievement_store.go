package db

import (
	"time"
)

// AwardAchievement records an achievement once; re-awarding is a no-op
// (UNIQUE + ON CONFLICT). Returns true when this call was the first award.
func (s *Store) AwardAchievement(userID int64, key string) (bool, error) {
	res, err := s.db.Exec(
		`INSERT INTO achievements(user_id, key, awarded_at) VALUES($1, $2, $3)
		 ON CONFLICT(user_id, key) DO NOTHING`,
		userID, key, time.Now().UTC())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// EarnedAchievements returns key -> awarded time for a user.
func (s *Store) EarnedAchievements(userID int64) (map[string]time.Time, error) {
	rows, err := s.db.Query(`SELECT key, awarded_at FROM achievements WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var k string
		var at time.Time
		if err := rows.Scan(&k, &at); err != nil {
			return nil, err
		}
		out[k] = at
	}
	return out, rows.Err()
}

// --- condition inputs the achievement catalog needs -------------------------

// CountCompletions counts every quest completion.
func (s *Store) CountCompletions(userID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM quest_completions WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

// CompletionHours returns the LOCAL hour of every completion (bucketed in Go —
// same no-zone-names-in-SQL discipline as the rest of the day math).
func (s *Store) CompletionHours(userID int64) ([]int, error) {
	rows, err := s.db.Query(`SELECT completed_at FROM quest_completions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hours []int
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		hours = append(hours, t.Local().Hour())
	}
	return hours, rows.Err()
}

// CountBossCompletions counts completed quests of type boss.
func (s *Store) CountBossCompletions(userID int64) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM quests WHERE user_id = $1 AND type = 'boss' AND status = 'completed'`, userID).Scan(&n)
	return n, err
}

// CountItemsByRarity returns rarity -> item count for a user.
func (s *Store) CountItemsByRarity(userID int64) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT rarity, COUNT(1) FROM user_items WHERE user_id = $1 GROUP BY rarity`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var r string
		var n int
		if err := rows.Scan(&r, &n); err != nil {
			return nil, err
		}
		out[r] = n
	}
	return out, rows.Err()
}
