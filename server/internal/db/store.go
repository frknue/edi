package db

import (
	"database/sql"
	"encoding/json"
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
	var created string
	err := s.db.QueryRow(`SELECT id, name, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Name, &created)
	if err != nil {
		return u, err
	}
	u.CreatedAt = mustParseTime(created)
	return u, nil
}

// --- attributes -------------------------------------------------------------

// ListAttributes returns raw attributes (TotalXP only); derived level/progress
// fields are filled by the service layer.
func (s *Store) ListAttributes(userID int64) ([]models.Attribute, error) {
	rows, err := s.db.Query(`SELECT id, user_id, key, name, total_xp FROM attributes WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Attribute
	for rows.Next() {
		var a models.Attribute
		if err := rows.Scan(&a.ID, &a.UserID, &a.Key, &a.Name, &a.TotalXP); err != nil {
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
		 WHERE user_id = ? AND created_at >= ? GROUP BY attribute_key`,
		userID, formatTime(since))
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
	var created string
	var completed, due sql.NullString
	var rewards string
	var srcSug sql.NullInt64
	err := scanner.Scan(&q.ID, &q.UserID, &q.Title, &q.Description, &q.Type, &q.Difficulty,
		&q.Status, &rewards, &q.SkipCount, &srcSug, &created, &completed, &due)
	if err != nil {
		return q, err
	}
	q.AttributeRewards = unmarshalRewards(rewards)
	q.CreatedAt = mustParseTime(created)
	q.CompletedAt = parseTimePtr(completed)
	q.DueDate = parseTimePtr(due)
	return q, nil
}

const questColumns = `id, user_id, title, description, type, difficulty, status, attribute_rewards, skip_count, source_suggestion_id, created_at, completed_at, due_date`

func (s *Store) GetQuest(userID, id int64) (models.Quest, error) {
	row := s.db.QueryRow(`SELECT `+questColumns+` FROM quests WHERE id = ? AND user_id = ?`, id, userID)
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
	q := `SELECT ` + questColumns + ` FROM quests WHERE user_id = ?`
	args := []any{userID}
	if questType != "" {
		q += ` AND type = ?`
		args = append(args, questType)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
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
	res, err := s.db.Exec(
		`INSERT INTO quests(user_id, title, description, type, difficulty, status, attribute_rewards, skip_count, source_suggestion_id, created_at, due_date)
		 VALUES(?, ?, ?, ?, ?, 'active', ?, 0, ?, ?, ?)`,
		userID, in.Title, in.Description, in.Type, in.Difficulty, marshalRewards(in.AttributeRewards),
		nullInt64(sourceSuggestionID), nowString(), formatTimePtr(in.DueDate))
	if err != nil {
		return models.Quest{}, err
	}
	id, _ := res.LastInsertId()
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
	if p.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *p.Title)
	}
	if p.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *p.Description)
	}
	if p.Type != nil {
		sets = append(sets, "type = ?")
		args = append(args, *p.Type)
	}
	if p.Difficulty != nil {
		sets = append(sets, "difficulty = ?")
		args = append(args, *p.Difficulty)
	}
	if p.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *p.Status)
	}
	if p.AttributeRewards != nil {
		sets = append(sets, "attribute_rewards = ?")
		args = append(args, marshalRewards(*p.AttributeRewards))
	}
	if p.DueDate != nil {
		sets = append(sets, "due_date = ?")
		args = append(args, formatTimePtr(p.DueDate))
	}
	if len(sets) > 0 {
		args = append(args, id, userID)
		if _, err := s.db.Exec(`UPDATE quests SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ?`, args...); err != nil {
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
	_, err := s.db.Exec(`UPDATE quests SET status = ? WHERE id = ? AND user_id = ?`, status, id, userID)
	return err
}

// SkipQuest marks a quest skipped and increments its skip counter.
func (s *Store) SkipQuest(userID, id int64) (models.Quest, error) {
	if _, err := s.db.Exec(
		`UPDATE quests SET status = 'skipped', skip_count = skip_count + 1 WHERE id = ? AND user_id = ?`,
		id, userID); err != nil {
		return models.Quest{}, err
	}
	return s.GetQuest(userID, id)
}

// QuestsSkippedRepeatedly returns active/skipped quests skipped >= threshold times.
func (s *Store) QuestsSkippedRepeatedly(userID int64, threshold int) ([]models.Quest, error) {
	rows, err := s.db.Query(`SELECT `+questColumns+` FROM quests
		WHERE user_id = ? AND skip_count >= ? AND status IN ('active','skipped') ORDER BY skip_count DESC`,
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

	tx, err := s.db.Begin()
	if err != nil {
		return models.Quest{}, nil, nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck — no-op after a successful Commit

	now := time.Now().UTC()
	nowStr := formatTime(now)

	// The completion gate is the conditional UPDATE itself, evaluated *inside* the
	// transaction. With a single writer (SetMaxOpenConns(1)) this serializes
	// concurrent completes: only the first request flips the status and gets
	// RowsAffected()==1; any racing duplicate (double-tapped button, second client)
	// matches zero rows and is rejected — no double XP, no duplicate completion row.
	res, err := tx.Exec(
		`UPDATE quests SET status = 'completed', completed_at = ?
		 WHERE id = ? AND user_id = ? AND status NOT IN ('completed','archived')`,
		nowStr, questID, userID)
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
		switch e := tx.QueryRow(`SELECT status FROM quests WHERE id = ? AND user_id = ?`, questID, userID).Scan(&status); e {
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
	if err := tx.QueryRow(`SELECT title, attribute_rewards FROM quests WHERE id = ? AND user_id = ?`, questID, userID).
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
	if _, err := tx.Exec(`INSERT INTO quest_completions(user_id, quest_id, xp_awarded, completed_at) VALUES(?, ?, ?, ?)`,
		userID, questID, total, nowStr); err != nil {
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
			if err := tx.QueryRow(`SELECT total_xp FROM attributes WHERE user_id = ? AND key = ?`, userID, a.key).Scan(&old); err != nil {
				if err == sql.ErrNoRows {
					continue // unknown attribute key — skip silently
				}
				return models.Quest{}, nil, nil, 0, err
			}
			baseXP[a.key] = old
		}
		res, err := tx.Exec(
			`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES(?, ?, ?, 'quest', ?, ?, ?)`,
			userID, a.key, a.amount, questID, a.note, nowStr)
		if err != nil {
			return models.Quest{}, nil, nil, 0, err
		}
		if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + ? WHERE user_id = ? AND key = ?`, a.amount, userID, a.key); err != nil {
			return models.Quest{}, nil, nil, 0, err
		}
		if g := goldForXP(a.amount); g > 0 {
			if _, err := insertGoldEventTx(tx, userID, g, a.src, a.note, nil, nowStr); err != nil {
				return models.Quest{}, nil, nil, 0, err
			}
			goldTotal += g
		}
		runningXP[a.key] = old + a.amount
		evID, _ := res.LastInsertId()
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
	err := tx.QueryRow(`SELECT current_count, longest_count, last_active_date FROM streaks WHERE user_id = ?`, userID).
		Scan(&current, &longest, &last)
	if err == sql.ErrNoRows {
		_, e := tx.Exec(`INSERT INTO streaks(user_id, current_count, longest_count, last_active_date) VALUES(?, 1, 1, ?)`, userID, today)
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
	_, e := tx.Exec(`UPDATE streaks SET current_count = ?, longest_count = ?, last_active_date = ? WHERE user_id = ?`,
		current, longest, today, userID)
	return e
}

func (s *Store) GetStreak(userID int64) (models.Streak, error) {
	var st models.Streak
	var last sql.NullString
	err := s.db.QueryRow(`SELECT current_count, longest_count, last_active_date FROM streaks WHERE user_id = ?`, userID).
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
		 WHERE e.user_id = ? ORDER BY e.id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.XPEvent
	for rows.Next() {
		var e models.XPEvent
		var created string
		var srcID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.AttributeKey, &e.AttributeName, &e.Amount, &e.Source, &srcID, &e.Note, &created); err != nil {
			return nil, err
		}
		if srcID.Valid {
			v := srcID.Int64
			e.SourceID = &v
		}
		e.CreatedAt = mustParseTime(created)
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
		 WHERE user_id = ? AND attribute_key = ? AND source = 'quest' AND created_at >= ?`,
		userID, attr, formatTime(since)).Scan(&n)
	return n, err
}

// --- completions / activity -------------------------------------------------

// CompletedTodayCount counts quest completions on the local "today".
func (s *Store) CompletedTodayCount(userID int64) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM quest_completions
		 WHERE user_id = ? AND date(completed_at,'localtime') = date('now','localtime')`, userID).Scan(&n)
	return n, err
}

// ActiveDaysSince counts distinct local days with at least one completion since cutoff.
func (s *Store) ActiveDaysSince(userID int64, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(DISTINCT date(completed_at,'localtime')) FROM quest_completions
		 WHERE user_id = ? AND completed_at >= ?`, userID, formatTime(since)).Scan(&n)
	return n, err
}

// CompletionsSince counts total completions since cutoff.
func (s *Store) CompletionsSince(userID int64, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM quest_completions WHERE user_id = ? AND completed_at >= ?`,
		userID, formatTime(since)).Scan(&n)
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

	tx, err := s.db.Begin()
	if err != nil {
		return models.JournalEntry{}, nil, nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	nowStr := formatTime(now)

	// First entry of the local day? (checked inside the tx, before our insert)
	var existingToday int
	if err := tx.QueryRow(
		`SELECT COUNT(1) FROM journal_entries WHERE user_id = ? AND date(created_at,'localtime') = date('now','localtime')`,
		userID).Scan(&existingToday); err != nil {
		return models.JournalEntry{}, nil, nil, 0, err
	}

	res, err := tx.Exec(`INSERT INTO journal_entries(user_id, mood, energy, notes, created_at) VALUES(?, ?, ?, ?, ?)`,
		userID, in.Mood, in.Energy, in.Notes, nowStr)
	if err != nil {
		return models.JournalEntry{}, nil, nil, 0, err
	}
	entryID, _ := res.LastInsertId()

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
			if err := tx.QueryRow(`SELECT total_xp FROM attributes WHERE user_id = ? AND key = ?`, userID, key).Scan(&oldXP); err != nil {
				continue // unknown attribute — skip
			}
			ev, err := tx.Exec(
				`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES(?, ?, ?, 'journal', ?, ?, ?)`,
				userID, key, amount, entryID, "Daily reflection", nowStr)
			if err != nil {
				return models.JournalEntry{}, nil, nil, 0, err
			}
			if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + ? WHERE user_id = ? AND key = ?`, amount, userID, key); err != nil {
				return models.JournalEntry{}, nil, nil, 0, err
			}
			if g := goldForXP(amount); g > 0 {
				if _, err := insertGoldEventTx(tx, userID, g, "journal", "Daily reflection", nil, nowStr); err != nil {
					return models.JournalEntry{}, nil, nil, 0, err
				}
				goldTotal += g
			}
			evID, _ := ev.LastInsertId()
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
	if p.Mood != nil {
		sets = append(sets, "mood = ?")
		args = append(args, *p.Mood)
	}
	if p.Energy != nil {
		sets = append(sets, "energy = ?")
		args = append(args, *p.Energy)
	}
	if p.Notes != nil {
		sets = append(sets, "notes = ?")
		args = append(args, *p.Notes)
	}
	if len(sets) > 0 {
		args = append(args, id, userID)
		res, err := s.db.Exec(`UPDATE journal_entries SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ?`, args...)
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
	res, err := s.db.Exec(`DELETE FROM journal_entries WHERE id = ? AND user_id = ?`, id, userID)
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
	var created string
	err := s.db.QueryRow(`SELECT id, mood, energy, notes, created_at FROM journal_entries WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&e.ID, &e.Mood, &e.Energy, &e.Notes, &created)
	if err != nil {
		return e, err
	}
	e.CreatedAt = mustParseTime(created)
	return e, nil
}

// ListJournal returns recent entries, optionally full-text filtered on notes.
func (s *Store) ListJournal(userID int64, limit int, search string) ([]models.JournalEntry, error) {
	if limit <= 0 {
		limit = 30
	}
	q := `SELECT id, mood, energy, notes, created_at FROM journal_entries WHERE user_id = ?`
	args := []any{userID}
	if search != "" {
		q += ` AND notes LIKE ? ESCAPE '\'`
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
		args = append(args, "%"+escaped+"%")
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.JournalEntry
	for rows.Next() {
		var e models.JournalEntry
		var created string
		if err := rows.Scan(&e.ID, &e.Mood, &e.Energy, &e.Notes, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = mustParseTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- agent suggestions ------------------------------------------------------

func (s *Store) InsertSuggestion(userID int64, sug models.AgentSuggestion) (models.AgentSuggestion, error) {
	tmpl, _ := json.Marshal(sug.SuggestedQuest)
	res, err := s.db.Exec(
		`INSERT INTO agent_suggestions(user_id, type, title, reason, suggested_quest, status, source_quest_id, created_at)
		 VALUES(?, ?, ?, ?, ?, 'pending', ?, ?)`,
		userID, sug.Type, sug.Title, sug.Reason, string(tmpl), nullInt64(sug.SourceQuestID), nowString())
	if err != nil {
		return models.AgentSuggestion{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetSuggestion(userID, id)
}

func (s *Store) GetSuggestion(userID, id int64) (models.AgentSuggestion, error) {
	row := s.db.QueryRow(
		`SELECT id, type, title, reason, suggested_quest, status, created_quest_id, source_quest_id, created_at, resolved_at
		 FROM agent_suggestions WHERE id = ? AND user_id = ?`, id, userID)
	return scanSuggestion(row)
}

func (s *Store) ListSuggestions(userID int64, status string) ([]models.AgentSuggestion, error) {
	q := `SELECT id, type, title, reason, suggested_quest, status, created_quest_id, source_quest_id, created_at, resolved_at
		FROM agent_suggestions WHERE user_id = ?`
	args := []any{userID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
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
	_, err := s.db.Exec(`DELETE FROM agent_suggestions WHERE user_id = ? AND status = 'pending'`, userID)
	return err
}

// HasPendingSuggestionOfType reports whether a pending suggestion of the given
// type already exists (used to avoid duplicate suggestions).
func (s *Store) HasPendingSuggestionOfType(userID int64, sugType string, sourceQuestID *int64) (bool, error) {
	q := `SELECT COUNT(1) FROM agent_suggestions WHERE user_id = ? AND type = ? AND status = 'pending'`
	args := []any{userID, sugType}
	if sourceQuestID != nil {
		q += ` AND source_quest_id = ?`
		args = append(args, *sourceQuestID)
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
		`UPDATE agent_suggestions SET status = ?, created_quest_id = ?, resolved_at = ? WHERE id = ? AND user_id = ?`,
		status, nullInt64(createdQuestID), nowString(), id, userID)
	return err
}

// AcceptSuggestion atomically creates a quest from a pending suggestion and marks
// the suggestion accepted (linking the new quest). Either both writes land or
// neither does — no orphan quests, no double-accept. Returns ErrNotFound if the
// suggestion is missing or ErrSuggestionNotPending if already resolved.
func (s *Store) AcceptSuggestion(userID, suggestionID int64, in models.QuestInput) (models.Quest, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.Quest{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var status string
	switch e := tx.QueryRow(`SELECT status FROM agent_suggestions WHERE id = ? AND user_id = ?`, suggestionID, userID).Scan(&status); e {
	case sql.ErrNoRows:
		return models.Quest{}, ErrNotFound
	case nil:
		if status != "pending" {
			return models.Quest{}, ErrSuggestionNotPending
		}
	default:
		return models.Quest{}, e
	}

	res, err := tx.Exec(
		`INSERT INTO quests(user_id, title, description, type, difficulty, status, attribute_rewards, skip_count, source_suggestion_id, created_at, due_date)
		 VALUES(?, ?, ?, ?, ?, 'active', ?, 0, ?, ?, ?)`,
		userID, in.Title, in.Description, in.Type, in.Difficulty, marshalRewards(in.AttributeRewards),
		suggestionID, nowString(), formatTimePtr(in.DueDate))
	if err != nil {
		return models.Quest{}, err
	}
	questID, _ := res.LastInsertId()

	if _, err := tx.Exec(
		`UPDATE agent_suggestions SET status = 'accepted', created_quest_id = ?, resolved_at = ? WHERE id = ? AND user_id = ?`,
		questID, nowString(), suggestionID, userID); err != nil {
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
	var created string
	var resolved sql.NullString
	err := scanner.Scan(&sug.ID, &sug.Type, &sug.Title, &sug.Reason, &tmpl, &sug.Status,
		&createdQuestID, &sourceQuestID, &created, &resolved)
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
	sug.CreatedAt = mustParseTime(created)
	sug.ResolvedAt = parseTimePtr(resolved)
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
