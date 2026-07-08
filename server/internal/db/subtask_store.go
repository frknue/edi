package db

import (
	"database/sql"
	"strings"

	"edi/internal/models"
)

// scanSubtask reads one quest_subtasks row.
func scanSubtask(scanner interface{ Scan(...any) error }) (models.Subtask, error) {
	var st models.Subtask
	var rewards string
	var done int
	err := scanner.Scan(&st.ID, &st.QuestID, &st.Title, &rewards, &done)
	if err != nil {
		return st, err
	}
	st.AttributeRewards = unmarshalRewards(rewards)
	st.Done = done != 0
	return st, nil
}

// subtasksForQuests loads subtasks for a set of quest ids, keyed by quest id.
func (s *Store) subtasksForQuests(userID int64, questIDs []int64) (map[int64][]models.Subtask, error) {
	out := map[int64][]models.Subtask{}
	if len(questIDs) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(questIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(questIDs)+1)
	args = append(args, userID)
	for _, id := range questIDs {
		args = append(args, id)
	}
	rows, err := s.db.Query(
		`SELECT id, quest_id, title, attribute_rewards, done FROM quest_subtasks
		 WHERE user_id = ? AND quest_id IN (`+placeholders+`) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		st, err := scanSubtask(rows)
		if err != nil {
			return nil, err
		}
		out[st.QuestID] = append(out[st.QuestID], st)
	}
	return out, rows.Err()
}

// attachSubtasks fills Quest.Subtasks (always non-nil) for the given quests.
func (s *Store) attachSubtasks(userID int64, quests []models.Quest) error {
	ids := make([]int64, len(quests))
	for i, q := range quests {
		ids[i] = q.ID
	}
	byQuest, err := s.subtasksForQuests(userID, ids)
	if err != nil {
		return err
	}
	for i := range quests {
		sts := byQuest[quests[i].ID]
		if sts == nil {
			sts = []models.Subtask{}
		}
		quests[i].Subtasks = sts
	}
	return nil
}

// replaceSubtasks deletes and reinserts a quest's subtasks (done flags reset).
func (s *Store) replaceSubtasks(userID, questID int64, subtasks []models.SubtaskInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM quest_subtasks WHERE user_id = ? AND quest_id = ?`, userID, questID); err != nil {
		return err
	}
	for _, st := range subtasks {
		if _, err := tx.Exec(
			`INSERT INTO quest_subtasks(user_id, quest_id, title, attribute_rewards, done, created_at) VALUES(?, ?, ?, ?, 0, ?)`,
			userID, questID, st.Title, marshalRewards(st.AttributeRewards), nowString()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ToggleSubtask flips a subtask's done flag. It refuses when the parent quest is
// already completed or archived (bonuses are frozen at completion).
func (s *Store) ToggleSubtask(userID, questID, subtaskID int64) (models.Subtask, error) {
	var status string
	switch err := s.db.QueryRow(`SELECT status FROM quests WHERE id = ? AND user_id = ?`, questID, userID).Scan(&status); err {
	case sql.ErrNoRows:
		return models.Subtask{}, ErrNotFound
	case nil:
	default:
		return models.Subtask{}, err
	}
	if status == models.StatusCompleted || status == models.StatusArchived {
		return models.Subtask{}, ErrQuestNotCompletable
	}

	res, err := s.db.Exec(
		`UPDATE quest_subtasks SET done = 1 - done WHERE id = ? AND quest_id = ? AND user_id = ?`,
		subtaskID, questID, userID)
	if err != nil {
		return models.Subtask{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Subtask{}, ErrNotFound
	}
	row := s.db.QueryRow(
		`SELECT id, quest_id, title, attribute_rewards, done FROM quest_subtasks WHERE id = ? AND user_id = ?`,
		subtaskID, userID)
	return scanSubtask(row)
}

// doneSubtasksTx returns the checked subtasks of a quest inside a transaction.
func doneSubtasksTx(tx *sql.Tx, userID, questID int64) ([]models.Subtask, error) {
	rows, err := tx.Query(
		`SELECT id, quest_id, title, attribute_rewards, done FROM quest_subtasks
		 WHERE user_id = ? AND quest_id = ? AND done = 1 ORDER BY id`, userID, questID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Subtask
	for rows.Next() {
		st, err := scanSubtask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
