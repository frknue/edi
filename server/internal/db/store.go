package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"edi/internal/models"
)

// dayFormat is the YYYY-MM-DD layout used for streak / daily comparisons.
const dayFormat = "2006-01-02"

func marshalRewards(r map[string]int64) string {
	if r == nil {
		r = map[string]int64{}
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func unmarshalRewards(s string) map[string]int64 {
	m := map[string]int64{}
	if s == "" {
		return m
	}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

// --- users ------------------------------------------------------------------

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) GetUser(id int64) (models.User, error) {
	var u models.User
	err := s.db.QueryRow(`SELECT id, name, is_admin, created_at FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Name, &u.IsAdmin, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) ListUsers() ([]models.User, error) {
	rows, err := s.db.Query(`SELECT id, name, is_admin, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Name, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetUserIDByTokenHash resolves a bearer token (pre-hashed by the service
// layer) to a user id. ErrNotFound for unknown tokens.
func (s *Store) GetUserIDByTokenHash(hash string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE token_hash = $1`, hash).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return id, err
}

// SetUserTokenHash sets (or clears, with "") a user's login token hash.
func (s *Store) SetUserTokenHash(userID int64, hash string) error {
	var v interface{}
	if hash != "" {
		v = hash
	}
	res, err := s.db.Exec(`UPDATE users SET token_hash = $1 WHERE id = $2`, v, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateUserWithDefaults creates a user plus everything a playable character
// needs — the nine attributes at 0 XP (no xp_events: SUM(0 rows)==0 keeps the
// audit invariant) and a zeroed streak row — in one transaction.
func (s *Store) CreateUserWithDefaults(name string, isAdmin bool) (models.User, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.User{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	var id int64
	if err := tx.QueryRow(`INSERT INTO users(name, is_admin, created_at) VALUES($1, $2, $3) RETURNING id`,
		name, isAdmin, now).Scan(&id); err != nil {
		return models.User{}, err
	}
	for _, a := range DefaultAttributes {
		if _, err := tx.Exec(
			`INSERT INTO attributes(user_id, key, name, total_xp, peak_xp, created_at) VALUES($1, $2, $3, 0, 0, $4)`,
			id, a.Key, a.Name, now); err != nil {
			return models.User{}, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO streaks(user_id, current_count, longest_count) VALUES($1, 0, 0)`, id); err != nil {
		return models.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.User{}, err
	}
	return models.User{ID: id, Name: name, IsAdmin: isAdmin, CreatedAt: now}, nil
}

// --- attributes -------------------------------------------------------------

// ListAttributes returns raw attributes (TotalXP and PeakXP); derived level/progress
// fields are filled by the service layer.
func (s *Store) ListAttributes(userID int64) ([]models.Attribute, error) {
	rows, err := s.db.Query(`SELECT id, user_id, key, name, total_xp, peak_xp FROM attributes WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Attribute
	for rows.Next() {
		var a models.Attribute
		if err := rows.Scan(&a.ID, &a.UserID, &a.Key, &a.Name, &a.TotalXP, &a.PeakXP); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AttributeNames returns a key->display-name map.
func (s *Store) AttributeNames(userID int64) (map[string]string, error) {
	attrs, err := s.ListAttributes(userID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Key] = a.Name
	}
	return m, nil
}

// WeeklyXPByAttribute sums xp_events per attribute since the given time.
func (s *Store) WeeklyXPByAttribute(userID int64, since time.Time) (map[string]int64, error) {
	rows, err := s.db.Query(
		`SELECT attribute_key, COALESCE(SUM(amount),0) FROM xp_events
		 WHERE user_id = $1 AND created_at >= $2 GROUP BY attribute_key`,
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// --- quests -----------------------------------------------------------------

func scanQuest(scanner interface{ Scan(...any) error }) (models.Quest, error) {
	var q models.Quest
	var completed, due sql.NullTime
	var rewards string
	var srcSug sql.NullInt64
	err := scanner.Scan(&q.ID, &q.UserID, &q.Title, &q.Description, &q.Type, &q.Difficulty,
		&q.Status, &rewards, &q.SkipCount, &srcSug, &q.CreatedAt, &completed, &due)
	if err != nil {
		return q, err
	}
	q.AttributeRewards = unmarshalRewards(rewards)
	q.CompletedAt = timePtr(completed)
	q.DueDate = timePtr(due)
	return q, nil
}

const questColumns = `id, user_id, title, description, type, difficulty, status, attribute_rewards, skip_count, source_suggestion_id, created_at, completed_at, due_date`

func (s *Store) GetQuest(userID, id int64) (models.Quest, error) {
	row := s.db.QueryRow(`SELECT `+questColumns+` FROM quests WHERE id = $1 AND user_id = $2`, id, userID)
	q, err := scanQuest(row)
	if err != nil {
		return q, err
	}
	quests := []models.Quest{q}
	if err := s.attachSubtasks(userID, quests); err != nil {
		return q, err
	}
	return quests[0], nil
}

// ListQuests returns quests filtered by optional type and status (empty = all).
func (s *Store) ListQuests(userID int64, questType, status string) ([]models.Quest, error) {
	q := `SELECT ` + questColumns + ` FROM quests WHERE user_id = $1`
	args := []any{userID}
	if questType != "" {
		args = append(args, questType)
		q += fmt.Sprintf(` AND type = $%d`, len(args))
	}
	if status != "" {
		args = append(args, status)
		q += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	q += ` ORDER BY
		CASE type WHEN 'boss' THEN 0 WHEN 'main' THEN 1 WHEN 'daily' THEN 2 WHEN 'weekly' THEN 3 WHEN 'side' THEN 4 WHEN 'recovery' THEN 5 ELSE 6 END,
		created_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Quest
	for rows.Next() {
		qst, err := scanQuest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, qst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachSubtasks(userID, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) InsertQuest(userID int64, in models.QuestInput, sourceSuggestionID *int64) (models.Quest, error) {
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO quests(user_id, title, description, type, difficulty, status, attribute_rewards, skip_count, source_suggestion_id, created_at, due_date)
		 VALUES($1, $2, $3, $4, $5, 'active', $6, 0, $7, $8, $9) RETURNING id`,
		userID, in.Title, in.Description, in.Type, in.Difficulty, marshalRewards(in.AttributeRewards),
		nullInt64(sourceSuggestionID), time.Now().UTC(), nullTime(in.DueDate)).Scan(&id)
	if err != nil {
		return models.Quest{}, err
	}
	if len(in.Subtasks) > 0 {
		if err := s.replaceSubtasks(userID, id, in.Subtasks); err != nil {
			return models.Quest{}, err
		}
	}
	return s.GetQuest(userID, id)
}

// UpdateQuest applies a partial patch and returns the updated quest.
func (s *Store) UpdateQuest(userID, id int64, p models.QuestPatch) (models.Quest, error) {
	var sets []string
	var args []any
	set := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if p.Title != nil {
		set("title", *p.Title)
	}
	if p.Description != nil {
		set("description", *p.Description)
	}
	if p.Type != nil {
		set("type", *p.Type)
	}
	if p.Difficulty != nil {
		set("difficulty", *p.Difficulty)
	}
	if p.Status != nil {
		set("status", *p.Status)
	}
	if p.AttributeRewards != nil {
		set("attribute_rewards", marshalRewards(*p.AttributeRewards))
	}
	if p.DueDate != nil {
		set("due_date", nullTime(p.DueDate))
	}
	if len(sets) > 0 {
		args = append(args, id, userID)
		q := fmt.Sprintf(`UPDATE quests SET %s WHERE id = $%d AND user_id = $%d`,
			strings.Join(sets, ", "), len(args)-1, len(args))
		if _, err := s.db.Exec(q, args...); err != nil {
			return models.Quest{}, err
		}
	}
	if p.Subtasks != nil {
		if err := s.replaceSubtasks(userID, id, *p.Subtasks); err != nil {
			return models.Quest{}, err
		}
	}
	return s.GetQuest(userID, id)
}

// SetQuestStatus updates only the status column.
func (s *Store) SetQuestStatus(userID, id int64, status string) error {
	_, err := s.db.Exec(`UPDATE quests SET status = $1 WHERE id = $2 AND user_id = $3`, status, id, userID)
	return err
}

// SkipQuest marks a quest skipped and increments its skip counter.
func (s *Store) SkipQuest(userID, id int64) (models.Quest, error) {
	if _, err := s.db.Exec(
		`UPDATE quests SET status = 'skipped', skip_count = skip_count + 1 WHERE id = $1 AND user_id = $2`,
		id, userID); err != nil {
		return models.Quest{}, err
	}
	return s.GetQuest(userID, id)
}

// QuestsSkippedRepeatedly returns active/skipped quests skipped >= threshold times.
func (s *Store) QuestsSkippedRepeatedly(userID int64, threshold int) ([]models.Quest, error) {
	rows, err := s.db.Query(`SELECT `+questColumns+` FROM quests
		WHERE user_id = $1 AND skip_count >= $2 AND status IN ('active','skipped') ORDER BY skip_count DESC`,
		userID, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Quest
	for rows.Next() {
		q, err := scanQuest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// --- completion (transactional) --------------------------------------------

// CompleteQuest marks a quest completed, records a completion row, writes one
// xp_event per rewarded attribute, bumps attribute totals, and updates the
// streak — all atomically. It returns the completed quest, the created events,
// and any attribute level-ups.
func (s *Store) CompleteQuest(userID, questID int64) (models.Quest, []models.XPEvent, []models.LevelUp, int64, error) {
	names, err := s.AttributeNames(userID)
	if err != nil {
		return models.Quest{}, nil, nil, 0, err
	}

	// The per-user advisory lock (beginUserTx) serializes concurrent completes
	// the way the SQLite single-writer used to; the conditional UPDATE below is
	// the completion gate itself: only the first request flips the status and
	// gets RowsAffected()==1 — no double XP, no duplicate completion row.
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.Quest{}, nil, nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck — no-op after a successful Commit

	now := time.Now().UTC()

	res, err := tx.Exec(
		`UPDATE quests SET status = 'completed', completed_at = $1
		 WHERE id = $2 AND user_id = $3 AND status NOT IN ('completed','archived')`,
		now, questID, userID)
	if err != nil {
		return models.Quest{}, nil, nil, 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return models.Quest{}, nil, nil, 0, err
	}
	if affected == 0 {
		// Distinguish "doesn't exist" from "not completable right now".
		var status string
		switch e := tx.QueryRow(`SELECT status FROM quests WHERE id = $1 AND user_id = $2`, questID, userID).Scan(&status); e {
		case sql.ErrNoRows:
			return models.Quest{}, nil, nil, 0, ErrNotFound
		case nil:
			return models.Quest{}, nil, nil, 0, ErrQuestNotCompletable
		default:
			return models.Quest{}, nil, nil, 0, e
		}
	}

	// Read the rewards/title inside the tx, now that the row is locked as completed.
	var rewardsJSON, title string
	if err := tx.QueryRow(`SELECT title, attribute_rewards FROM quests WHERE id = $1 AND user_id = $2`, questID, userID).
		Scan(&title, &rewardsJSON); err != nil {
		return models.Quest{}, nil, nil, 0, err
	}
	rewards := unmarshalRewards(rewardsJSON)

	// Checked subtasks add their own rewards as separately-labeled bonus awards.
	doneSubs, err := doneSubtasksTx(tx, userID, questID)
	if err != nil {
		return models.Quest{}, nil, nil, 0, err
	}

	// Build the award list: base quest rewards first, then one entry per checked
	// subtask. Each becomes its own xp_event (clean audit trail); level-ups are
	// computed cumulatively per attribute so base+bonus on the same attribute
	// count once from the original XP to the final total.
	type award struct {
		key    string
		amount int64
		note   string
		src    string // gold_events source: "quest" or "subtask"
	}
	var awards []award
	for _, key := range orderedKeys(rewards) {
		if rewards[key] != 0 {
			awards = append(awards, award{key, rewards[key], title, "quest"})
		}
	}
	for _, st := range doneSubs {
		for _, key := range orderedKeys(st.AttributeRewards) {
			if st.AttributeRewards[key] != 0 {
				awards = append(awards, award{key, st.AttributeRewards[key], title + " · " + st.Title, "subtask"})
			}
		}
	}

	total := int64(0)
	for _, a := range awards {
		total += a.amount
	}
	if _, err := tx.Exec(`INSERT INTO quest_completions(user_id, quest_id, xp_awarded, completed_at) VALUES($1, $2, $3, $4)`,
		userID, questID, total, now); err != nil {
		return models.Quest{}, nil, nil, 0, err
	}

	var events []models.XPEvent
	var levelUps []models.LevelUp
	var goldTotal int64
	baseXP := map[string]int64{}    // XP before this completion, per touched attribute
	runningXP := map[string]int64{} // XP after awards applied so far
	for _, a := range awards {
		old, seen := runningXP[a.key]
		if !seen {
			if err := tx.QueryRow(`SELECT total_xp FROM attributes WHERE user_id = $1 AND key = $2`, userID, a.key).Scan(&old); err != nil {
				if err == sql.ErrNoRows {
					continue // unknown attribute key — skip silently
				}
				return models.Quest{}, nil, nil, 0, err
			}
			baseXP[a.key] = old
		}
		var evID int64
		if err := tx.QueryRow(
			`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES($1, $2, $3, 'quest', $4, $5, $6) RETURNING id`,
			userID, a.key, a.amount, questID, a.note, now).Scan(&evID); err != nil {
			return models.Quest{}, nil, nil, 0, err
		}
		if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + $1, peak_xp = GREATEST(peak_xp, total_xp + $1) WHERE user_id = $2 AND key = $3`, a.amount, userID, a.key); err != nil {
			return models.Quest{}, nil, nil, 0, err
		}
		if g := goldForXP(a.amount); g > 0 {
			if _, err := insertGoldEventTx(tx, userID, g, a.src, a.note, nil, now); err != nil {
				return models.Quest{}, nil, nil, 0, err
			}
			goldTotal += g
		}
		runningXP[a.key] = old + a.amount
		sid := questID
		events = append(events, models.XPEvent{
			ID: evID, AttributeKey: a.key, AttributeName: names[a.key], Amount: a.amount,
			Source: "quest", SourceID: &sid, Note: a.note, CreatedAt: now,
		})
	}
	// Level-ups: from the pre-completion XP to the final total, once per attribute.
	for _, key := range orderedKeys(runningXP) {
		if from, to := levelFromTo(baseXP[key], runningXP[key]); to > from {
			levelUps = append(levelUps, models.LevelUp{
				AttributeKey: key, AttributeName: names[key], FromLevel: from, ToLevel: to,
			})
		}
	}

	if err := updateStreakTx(tx, userID, now); err != nil {
		return models.Quest{}, nil, nil, 0, err
	}

	if err := tx.Commit(); err != nil {
		return models.Quest{}, nil, nil, 0, err
	}

	updated, err := s.GetQuest(userID, questID)
	if err != nil {
		return models.Quest{}, nil, nil, 0, err
	}
	return updated, events, levelUps, goldTotal, nil
}

// updateStreakTx advances the streak for "today" (local day).
func updateStreakTx(tx *sql.Tx, userID int64, now time.Time) error {
	today := now.Local().Format(dayFormat)
	var current, longest int
	var last sql.NullString
	err := tx.QueryRow(`SELECT current_count, longest_count, last_active_date FROM streaks WHERE user_id = $1`, userID).
		Scan(&current, &longest, &last)
	if err == sql.ErrNoRows {
		_, e := tx.Exec(`INSERT INTO streaks(user_id, current_count, longest_count, last_active_date) VALUES($1, 1, 1, $2)`, userID, today)
		return e
	}
	if err != nil {
		return err
	}
	switch {
	case last.Valid && last.String == today:
		// already counted today
	case last.Valid && last.String == now.Local().AddDate(0, 0, -1).Format(dayFormat):
		current++
	default:
		current = 1
	}
	if current > longest {
		longest = current
	}
	_, e := tx.Exec(`UPDATE streaks SET current_count = $1, longest_count = $2, last_active_date = $3 WHERE user_id = $4`,
		current, longest, today, userID)
	return e
}

func (s *Store) GetStreak(userID int64) (models.Streak, error) {
	var st models.Streak
	var last sql.NullString
	err := s.db.QueryRow(`SELECT current_count, longest_count, last_active_date FROM streaks WHERE user_id = $1`, userID).
		Scan(&st.Current, &st.Longest, &last)
	if err == sql.ErrNoRows {
		return models.Streak{}, nil
	}
	if err != nil {
		return st, err
	}
	if last.Valid {
		v := last.String
		st.LastActiveDate = &v
	}
	return st, nil
}

// --- xp events --------------------------------------------------------------

func (s *Store) ListXPEvents(userID int64, limit int) ([]models.XPEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT e.id, e.attribute_key, COALESCE(a.name, e.attribute_key), e.amount, e.source, e.source_id, e.note, e.created_at
		 FROM xp_events e LEFT JOIN attributes a ON a.user_id = e.user_id AND a.key = e.attribute_key
		 WHERE e.user_id = $1 ORDER BY e.id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.XPEvent
	for rows.Next() {
		var e models.XPEvent
		var srcID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.AttributeKey, &e.AttributeName, &e.Amount, &e.Source, &srcID, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		if srcID.Valid {
			v := srcID.Int64
			e.SourceID = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DistinctQuestsRewardingAttributeSince counts distinct completed quests that
// awarded XP to a given attribute since the cutoff.
func (s *Store) DistinctQuestsRewardingAttributeSince(userID int64, attr string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(DISTINCT source_id) FROM xp_events
		 WHERE user_id = $1 AND attribute_key = $2 AND source = 'quest' AND created_at >= $3`,
		userID, attr, since).Scan(&n)
	return n, err
}

// --- completions / activity -------------------------------------------------

// CompletedTodayCount counts quest completions on the local "today"
// (day bounds computed in Go, honoring TZ).
func (s *Store) CompletedTodayCount(userID int64) (int, error) {
	start, end := localDayBounds(time.Now())
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM quest_completions
		 WHERE user_id = $1 AND completed_at >= $2 AND completed_at < $3`, userID, start, end).Scan(&n)
	return n, err
}

// ActiveDaysSince counts distinct local days with at least one completion since
// cutoff. The distinct-day bucketing happens in Go so SQL never needs the zone.
func (s *Store) ActiveDaysSince(userID int64, since time.Time) (int, error) {
	rows, err := s.db.Query(
		`SELECT completed_at FROM quest_completions WHERE user_id = $1 AND completed_at >= $2`,
		userID, since)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	days := map[string]bool{}
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return 0, err
		}
		days[t.Local().Format(dayFormat)] = true
	}
	return len(days), rows.Err()
}

// CompletionsSince counts total completions since cutoff.
func (s *Store) CompletionsSince(userID int64, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM quest_completions WHERE user_id = $1 AND completed_at >= $2`,
		userID, since).Scan(&n)
	return n, err
}

// --- journal ----------------------------------------------------------------

// InsertJournal stores an entry and, when it is the FIRST entry of the local
// day, awards dailyRewards atomically the same auditable way as quests/tools:
// xp_events (source='journal') + attribute bumps + streak, all in one tx.
// Later entries the same day store fine but award nothing.
func (s *Store) InsertJournal(userID int64, in models.JournalInput, dailyRewards map[string]int64) (models.JournalEntry, []models.XPEvent, []models.LevelUp, int64, error) {
	names, err := s.AttributeNames(userID)
	if err != nil {
		return models.JournalEntry{}, nil, nil, 0, err
	}

	// Per-user lock: the first-entry-of-the-day check below is a read the
	// following writes depend on.
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.JournalEntry{}, nil, nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	dayStart, dayEnd := localDayBounds(now)

	// First entry of the local day? (checked inside the tx, before our insert)
	var existingToday int
	if err := tx.QueryRow(
		`SELECT COUNT(1) FROM journal_entries WHERE user_id = $1 AND created_at >= $2 AND created_at < $3`,
		userID, dayStart, dayEnd).Scan(&existingToday); err != nil {
		return models.JournalEntry{}, nil, nil, 0, err
	}

	var entryID int64
	if err := tx.QueryRow(`INSERT INTO journal_entries(user_id, mood, energy, notes, created_at) VALUES($1, $2, $3, $4, $5) RETURNING id`,
		userID, in.Mood, in.Energy, in.Notes, now).Scan(&entryID); err != nil {
		return models.JournalEntry{}, nil, nil, 0, err
	}

	var events []models.XPEvent
	var levelUps []models.LevelUp
	var goldTotal int64
	if existingToday == 0 && len(dailyRewards) > 0 {
		for _, key := range orderedKeys(dailyRewards) {
			amount := dailyRewards[key]
			if amount == 0 {
				continue
			}
			var oldXP int64
			if err := tx.QueryRow(`SELECT total_xp FROM attributes WHERE user_id = $1 AND key = $2`, userID, key).Scan(&oldXP); err != nil {
				continue // unknown attribute — skip
			}
			var evID int64
			if err := tx.QueryRow(
				`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES($1, $2, $3, 'journal', $4, $5, $6) RETURNING id`,
				userID, key, amount, entryID, "Daily reflection", now).Scan(&evID); err != nil {
				return models.JournalEntry{}, nil, nil, 0, err
			}
			if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + $1, peak_xp = GREATEST(peak_xp, total_xp + $1) WHERE user_id = $2 AND key = $3`, amount, userID, key); err != nil {
				return models.JournalEntry{}, nil, nil, 0, err
			}
			if g := goldForXP(amount); g > 0 {
				if _, err := insertGoldEventTx(tx, userID, g, "journal", "Daily reflection", nil, now); err != nil {
					return models.JournalEntry{}, nil, nil, 0, err
				}
				goldTotal += g
			}
			sid := entryID
			events = append(events, models.XPEvent{
				ID: evID, AttributeKey: key, AttributeName: names[key], Amount: amount,
				Source: "journal", SourceID: &sid, Note: "Daily reflection", CreatedAt: now,
			})
			if from, to := levelFromTo(oldXP, oldXP+amount); to > from {
				levelUps = append(levelUps, models.LevelUp{
					AttributeKey: key, AttributeName: names[key], FromLevel: from, ToLevel: to,
				})
			}
		}
		if err := updateStreakTx(tx, userID, now); err != nil {
			return models.JournalEntry{}, nil, nil, 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.JournalEntry{}, nil, nil, 0, err
	}
	entry, err := s.GetJournal(userID, entryID)
	return entry, events, levelUps, goldTotal, err
}

// UpdateJournal applies a partial patch to an entry.
func (s *Store) UpdateJournal(userID, id int64, p models.JournalPatch) (models.JournalEntry, error) {
	var sets []string
	var args []any
	set := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if p.Mood != nil {
		set("mood", *p.Mood)
	}
	if p.Energy != nil {
		set("energy", *p.Energy)
	}
	if p.Notes != nil {
		set("notes", *p.Notes)
	}
	if len(sets) > 0 {
		args = append(args, id, userID)
		q := fmt.Sprintf(`UPDATE journal_entries SET %s WHERE id = $%d AND user_id = $%d`,
			strings.Join(sets, ", "), len(args)-1, len(args))
		res, err := s.db.Exec(q, args...)
		if err != nil {
			return models.JournalEntry{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return models.JournalEntry{}, ErrNotFound
		}
	}
	return s.GetJournal(userID, id)
}

// DeleteJournal removes an entry. Awarded XP is NOT clawed back — xp_events are
// an immutable audit log, and the reflection still happened.
func (s *Store) DeleteJournal(userID, id int64) error {
	res, err := s.db.Exec(`DELETE FROM journal_entries WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetJournal(userID, id int64) (models.JournalEntry, error) {
	var e models.JournalEntry
	err := s.db.QueryRow(`SELECT id, mood, energy, notes, created_at FROM journal_entries WHERE id = $1 AND user_id = $2`, id, userID).
		Scan(&e.ID, &e.Mood, &e.Energy, &e.Notes, &e.CreatedAt)
	return e, err
}

// ListJournal returns recent entries, optionally full-text filtered on notes.
func (s *Store) ListJournal(userID int64, limit int, search string) ([]models.JournalEntry, error) {
	if limit <= 0 {
		limit = 30
	}
	q := `SELECT id, mood, energy, notes, created_at FROM journal_entries WHERE user_id = $1`
	args := []any{userID}
	if search != "" {
		// ILIKE keeps the SQLite behavior (LIKE there is ASCII case-insensitive).
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
		args = append(args, "%"+escaped+"%")
		q += fmt.Sprintf(` AND notes ILIKE $%d`, len(args))
	}
	args = append(args, limit)
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT $%d`, len(args))
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.JournalEntry
	for rows.Next() {
		var e models.JournalEntry
		if err := rows.Scan(&e.ID, &e.Mood, &e.Energy, &e.Notes, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- agent suggestions ------------------------------------------------------

func (s *Store) InsertSuggestion(userID int64, sug models.AgentSuggestion) (models.AgentSuggestion, error) {
	tmpl, _ := json.Marshal(sug.SuggestedQuest)
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO agent_suggestions(user_id, type, title, reason, suggested_quest, status, source_quest_id, created_at)
		 VALUES($1, $2, $3, $4, $5, 'pending', $6, $7) RETURNING id`,
		userID, sug.Type, sug.Title, sug.Reason, string(tmpl), nullInt64(sug.SourceQuestID), time.Now().UTC()).Scan(&id)
	if err != nil {
		return models.AgentSuggestion{}, err
	}
	return s.GetSuggestion(userID, id)
}

func (s *Store) GetSuggestion(userID, id int64) (models.AgentSuggestion, error) {
	row := s.db.QueryRow(
		`SELECT id, type, title, reason, suggested_quest, status, created_quest_id, source_quest_id, created_at, resolved_at
		 FROM agent_suggestions WHERE id = $1 AND user_id = $2`, id, userID)
	return scanSuggestion(row)
}

func (s *Store) ListSuggestions(userID int64, status string) ([]models.AgentSuggestion, error) {
	q := `SELECT id, type, title, reason, suggested_quest, status, created_quest_id, source_quest_id, created_at, resolved_at
		FROM agent_suggestions WHERE user_id = $1`
	args := []any{userID}
	if status != "" {
		args = append(args, status)
		q += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AgentSuggestion
	for rows.Next() {
		sug, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sug)
	}
	return out, rows.Err()
}

// DeletePendingSuggestions removes all still-pending suggestions for a user
// (used to refresh the set when regenerating).
func (s *Store) DeletePendingSuggestions(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM agent_suggestions WHERE user_id = $1 AND status = 'pending'`, userID)
	return err
}

// HasPendingSuggestionOfType reports whether a pending suggestion of the given
// type already exists (used to avoid duplicate suggestions).
func (s *Store) HasPendingSuggestionOfType(userID int64, sugType string, sourceQuestID *int64) (bool, error) {
	q := `SELECT COUNT(1) FROM agent_suggestions WHERE user_id = $1 AND type = $2 AND status = 'pending'`
	args := []any{userID, sugType}
	if sourceQuestID != nil {
		args = append(args, *sourceQuestID)
		q += fmt.Sprintf(` AND source_quest_id = $%d`, len(args))
	}
	var n int
	if err := s.db.QueryRow(q, args...).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// ResolveSuggestion sets the suggestion's status (accepted/dismissed) and,
// for an accepted one, links the created quest.
func (s *Store) ResolveSuggestion(userID, id int64, status string, createdQuestID *int64) error {
	_, err := s.db.Exec(
		`UPDATE agent_suggestions SET status = $1, created_quest_id = $2, resolved_at = $3 WHERE id = $4 AND user_id = $5`,
		status, nullInt64(createdQuestID), time.Now().UTC(), id, userID)
	return err
}

// AcceptSuggestion atomically creates a quest from a pending suggestion and marks
// the suggestion accepted (linking the new quest). Either both writes land or
// neither does — no orphan quests, no double-accept. Returns ErrNotFound if the
// suggestion is missing or ErrSuggestionNotPending if already resolved.
func (s *Store) AcceptSuggestion(userID, suggestionID int64, in models.QuestInput) (models.Quest, error) {
	// Per-user lock: the pending-status read gates the writes.
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.Quest{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var status string
	switch e := tx.QueryRow(`SELECT status FROM agent_suggestions WHERE id = $1 AND user_id = $2`, suggestionID, userID).Scan(&status); e {
	case sql.ErrNoRows:
		return models.Quest{}, ErrNotFound
	case nil:
		if status != "pending" {
			return models.Quest{}, ErrSuggestionNotPending
		}
	default:
		return models.Quest{}, e
	}

	now := time.Now().UTC()
	var questID int64
	if err := tx.QueryRow(
		`INSERT INTO quests(user_id, title, description, type, difficulty, status, attribute_rewards, skip_count, source_suggestion_id, created_at, due_date)
		 VALUES($1, $2, $3, $4, $5, 'active', $6, 0, $7, $8, $9) RETURNING id`,
		userID, in.Title, in.Description, in.Type, in.Difficulty, marshalRewards(in.AttributeRewards),
		suggestionID, now, nullTime(in.DueDate)).Scan(&questID); err != nil {
		return models.Quest{}, err
	}

	if _, err := tx.Exec(
		`UPDATE agent_suggestions SET status = 'accepted', created_quest_id = $1, resolved_at = $2 WHERE id = $3 AND user_id = $4`,
		questID, now, suggestionID, userID); err != nil {
		return models.Quest{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.Quest{}, err
	}
	return s.GetQuest(userID, questID)
}

func scanSuggestion(scanner interface{ Scan(...any) error }) (models.AgentSuggestion, error) {
	var sug models.AgentSuggestion
	var tmpl string
	var createdQuestID, sourceQuestID sql.NullInt64
	var resolved sql.NullTime
	err := scanner.Scan(&sug.ID, &sug.Type, &sug.Title, &sug.Reason, &tmpl, &sug.Status,
		&createdQuestID, &sourceQuestID, &sug.CreatedAt, &resolved)
	if err != nil {
		return sug, err
	}
	_ = json.Unmarshal([]byte(tmpl), &sug.SuggestedQuest)
	if sug.SuggestedQuest.AttributeRewards == nil {
		// Keep the JSON contract honest: never emit attribute_rewards:null.
		sug.SuggestedQuest.AttributeRewards = map[string]int64{}
	}
	if createdQuestID.Valid {
		v := createdQuestID.Int64
		sug.CreatedQuestID = &v
	}
	if sourceQuestID.Valid {
		v := sourceQuestID.Int64
		sug.SourceQuestID = &v
	}
	sug.ResolvedAt = timePtr(resolved)
	return sug, nil
}

// --- helpers ----------------------------------------------------------------

func nullInt64(p *int64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

// orderedKeys returns map keys sorted for deterministic iteration.
func orderedKeys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// simple insertion sort (small maps)
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

// levelFromTo mirrors services.LevelForXP without importing services (avoids a
// cycle); kept in sync with the single MVP formula.
func levelFromTo(oldXP, newXP int64) (int, int) {
	return levelForXP(oldXP), levelForXP(newXP)
}

func levelForXP(xp int64) int {
	if xp < 0 {
		xp = 0
	}
	// floor(sqrt(xp/100)) + 1 via integer search to avoid importing math here.
	lvl := 0
	for int64(lvl*lvl)*100 <= xp {
		lvl++
	}
	return lvl // because (lvl-1) is the floor; loop overshoots by one => this equals floor()+1
}

// xpForLevel mirrors services.XPForLevel (kept in sync, avoids the cycle).
func xpForLevel(level int) int64 {
	if level < 1 {
		level = 1
	}
	l := int64(level - 1)
	return l * l * 100
}
