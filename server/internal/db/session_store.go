package db

import (
	"database/sql"
	"time"

	"edi/internal/models"
)

// Quest sessions: presence during the action. A session is a row that Start
// opens and Stop / Complete / expiry closes. It never writes XP.

// StartQuestSession opens a session on questID, closing any other running
// session of the user first (reason 'switched') — choosing costs nothing.
// The quest must be active for the user (ErrNotFound / ErrQuestNotCompletable).
func (s *Store) StartQuestSession(userID, questID int64, now time.Time) (models.QuestSession, error) {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.QuestSession{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var status string
	if err := tx.QueryRow(`SELECT status FROM quests WHERE id = $1 AND user_id = $2`, questID, userID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return models.QuestSession{}, ErrNotFound
		}
		return models.QuestSession{}, err
	}
	if status != models.StatusActive {
		return models.QuestSession{}, ErrQuestNotCompletable
	}
	// Idempotent: starting the quest that is already running returns it.
	var existing int64
	err = tx.QueryRow(`SELECT id FROM quest_sessions WHERE user_id = $1 AND quest_id = $2 AND ended_at IS NULL`, userID, questID).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return models.QuestSession{}, err
	}
	if err == nil {
		if err := tx.Commit(); err != nil {
			return models.QuestSession{}, err
		}
		return s.getSession(userID, existing, now)
	}
	if _, err := tx.Exec(
		`UPDATE quest_sessions SET ended_at = $1, reason = 'switched' WHERE user_id = $2 AND ended_at IS NULL`,
		now, userID); err != nil {
		return models.QuestSession{}, err
	}
	var id int64
	if err := tx.QueryRow(
		`INSERT INTO quest_sessions(user_id, quest_id, started_at) VALUES($1, $2, $3) RETURNING id`,
		userID, questID, now).Scan(&id); err != nil {
		return models.QuestSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.QuestSession{}, err
	}
	return s.getSession(userID, id, now)
}

// StopQuestSession closes the user's running session (reason 'stopped') and
// stores note as the quest's resume note. ErrNotFound when nothing runs.
func (s *Store) StopQuestSession(userID int64, note string, now time.Time) (models.QuestSession, error) {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.QuestSession{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var id, questID int64
	if err := tx.QueryRow(`SELECT id, quest_id FROM quest_sessions WHERE user_id = $1 AND ended_at IS NULL`, userID).Scan(&id, &questID); err != nil {
		if err == sql.ErrNoRows {
			return models.QuestSession{}, ErrNotFound
		}
		return models.QuestSession{}, err
	}
	if _, err := tx.Exec(`UPDATE quest_sessions SET ended_at = $1, reason = 'stopped', note = $2 WHERE id = $3`, now, note, id); err != nil {
		return models.QuestSession{}, err
	}
	if note != "" {
		if _, err := tx.Exec(`UPDATE quests SET resume_note = $1 WHERE id = $2 AND user_id = $3`, note, questID, userID); err != nil {
			return models.QuestSession{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.QuestSession{}, err
	}
	return s.getSession(userID, id, now)
}

// ActiveQuestSession returns the running session, expiring one that started
// before today's local day (reason 'expired') so a stale timer never greets
// the user in the morning. nil when nothing runs.
func (s *Store) ActiveQuestSession(userID int64, now time.Time) (*models.QuestSession, error) {
	dayStart, _ := localDayBounds(now)
	if _, err := s.db.Exec(
		`UPDATE quest_sessions SET ended_at = $1, reason = 'expired' WHERE user_id = $2 AND ended_at IS NULL AND started_at < $3`,
		now, userID, dayStart); err != nil {
		return nil, err
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM quest_sessions WHERE user_id = $1 AND ended_at IS NULL`, userID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess, err := s.getSession(userID, id, now)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// closeQuestSessionTx ends a running session of questID as 'completed' and
// clears the quest's resume note — called inside the completion tx.
func closeQuestSessionTx(tx *sql.Tx, userID, questID int64, now time.Time) error {
	if _, err := tx.Exec(
		`UPDATE quest_sessions SET ended_at = $1, reason = 'completed' WHERE user_id = $2 AND quest_id = $3 AND ended_at IS NULL`,
		now, userID, questID); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE quests SET resume_note = '' WHERE id = $1 AND user_id = $2`, questID, userID)
	return err
}

func (s *Store) getSession(userID, id int64, now time.Time) (models.QuestSession, error) {
	var sess models.QuestSession
	var ended sql.NullTime
	var rewards string
	err := s.db.QueryRow(
		`SELECT s.id, s.quest_id, q.title, q.type, q.attribute_rewards, q.resume_note, s.started_at, s.ended_at, s.reason, s.note
		 FROM quest_sessions s JOIN quests q ON q.id = s.quest_id
		 WHERE s.id = $1 AND s.user_id = $2`, id, userID).
		Scan(&sess.ID, &sess.QuestID, &sess.Title, &sess.QuestType, &rewards, &sess.ResumeNote, &sess.StartedAt, &ended, &sess.Reason, &sess.Note)
	if err == sql.ErrNoRows {
		return sess, ErrNotFound
	}
	if err != nil {
		return sess, err
	}
	sess.AttributeRewards = unmarshalRewards(rewards)
	sess.EndedAt = timePtr(ended)
	end := now
	if sess.EndedAt != nil {
		end = *sess.EndedAt
	}
	sess.ElapsedSeconds = int64(end.Sub(sess.StartedAt).Seconds())
	if sess.ElapsedSeconds < 0 {
		sess.ElapsedSeconds = 0
	}
	sess.Running = sess.EndedAt == nil
	return sess, nil
}

// ListQuestSessions returns recent sessions, newest first.
func (s *Store) ListQuestSessions(userID int64, limit int, now time.Time) ([]models.QuestSession, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id FROM quest_sessions WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]models.QuestSession, 0, len(ids))
	for _, id := range ids {
		sess, err := s.getSession(userID, id, now)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, nil
}
