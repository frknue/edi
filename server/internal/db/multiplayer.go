package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"edi/internal/models"
)

// SharedQuestAccess describes how a board member may reach one logical shared
// quest. OwnQuestID is zero when the member can see the card but is not an
// assignee.
type SharedQuestAccess struct {
	SharedQuestID int64
	OwnQuestID    int64
	Assigned      bool
}

func (s *Store) QuestBoardForUser(userID int64) (models.QuestBoard, error) {
	var board models.QuestBoard
	err := s.db.QueryRow(
		`SELECT b.id, b.name, b.created_at
		 FROM quest_boards b JOIN quest_board_members m ON m.board_id = b.id
		 WHERE m.user_id = $1`, userID).Scan(&board.ID, &board.Name, &board.CreatedAt)
	if err == sql.ErrNoRows {
		return board, ErrNotFound
	}
	if err != nil {
		return board, err
	}
	return s.fillQuestBoardMembers(board)
}

func (s *Store) fillQuestBoardMembers(board models.QuestBoard) (models.QuestBoard, error) {
	rows, err := s.db.Query(
		`SELECT u.id, u.name FROM quest_board_members m
		 JOIN users u ON u.id = m.user_id WHERE m.board_id = $1
		 ORDER BY m.joined_at, u.id`, board.ID)
	if err != nil {
		return board, err
	}
	defer rows.Close()
	board.Members = []models.QuestBoardMember{}
	for rows.Next() {
		var member models.QuestBoardMember
		if err := rows.Scan(&member.UserID, &member.Name); err != nil {
			return board, err
		}
		board.Members = append(board.Members, member)
	}
	return board, rows.Err()
}

func (s *Store) CreateQuestBoard(userID int64, name string) (models.QuestBoard, error) {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.QuestBoard{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var existing int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM quest_board_members WHERE user_id = $1`, userID).Scan(&existing); err != nil {
		return models.QuestBoard{}, err
	}
	if existing > 0 {
		return models.QuestBoard{}, ErrAlreadyInBoard
	}
	now := time.Now().UTC()
	var id int64
	if err := tx.QueryRow(
		`INSERT INTO quest_boards(name, created_by, created_at) VALUES($1, $2, $3) RETURNING id`,
		name, userID, now).Scan(&id); err != nil {
		return models.QuestBoard{}, err
	}
	if _, err := tx.Exec(
		`INSERT INTO quest_board_members(board_id, user_id, joined_at) VALUES($1, $2, $3)`,
		id, userID, now); err != nil {
		return models.QuestBoard{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.QuestBoard{}, err
	}
	return s.QuestBoardForUser(userID)
}

func (s *Store) CreateQuestBoardInvite(userID int64, codeHash string, expiresAt time.Time) error {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var boardID int64
	if err := tx.QueryRow(
		`SELECT b.id FROM quest_boards b JOIN quest_board_members m ON m.board_id = b.id
		 WHERE m.user_id = $1 FOR UPDATE OF b`, userID).Scan(&boardID); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	var members int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM quest_board_members WHERE board_id = $1`, boardID).Scan(&members); err != nil {
		return err
	}
	if members >= 2 {
		return ErrBoardFull
	}
	if _, err := tx.Exec(
		`INSERT INTO quest_board_invites(board_id, code_hash, created_by, expires_at)
		 VALUES($1, $2, $3, $4)`, boardID, codeHash, userID, expiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) JoinQuestBoard(userID int64, codeHash string, now time.Time) (models.QuestBoard, error) {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.QuestBoard{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var existing int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM quest_board_members WHERE user_id = $1`, userID).Scan(&existing); err != nil {
		return models.QuestBoard{}, err
	}
	if existing > 0 {
		return models.QuestBoard{}, ErrAlreadyInBoard
	}

	var inviteID, boardID int64
	var expiresAt time.Time
	var consumedAt sql.NullTime
	if err := tx.QueryRow(
		`SELECT i.id, i.board_id, i.expires_at, i.consumed_at
		 FROM quest_board_invites i JOIN quest_boards b ON b.id = i.board_id
		 WHERE i.code_hash = $1 FOR UPDATE OF i, b`, codeHash).
		Scan(&inviteID, &boardID, &expiresAt, &consumedAt); err != nil {
		if err == sql.ErrNoRows {
			return models.QuestBoard{}, ErrInvalidInvite
		}
		return models.QuestBoard{}, err
	}
	if consumedAt.Valid || !expiresAt.After(now) {
		return models.QuestBoard{}, ErrInvalidInvite
	}
	var members int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM quest_board_members WHERE board_id = $1`, boardID).Scan(&members); err != nil {
		return models.QuestBoard{}, err
	}
	if members >= 2 {
		return models.QuestBoard{}, ErrBoardFull
	}
	if _, err := tx.Exec(
		`INSERT INTO quest_board_members(board_id, user_id, joined_at) VALUES($1, $2, $3)`,
		boardID, userID, now); err != nil {
		return models.QuestBoard{}, err
	}
	if _, err := tx.Exec(`UPDATE quest_board_invites SET consumed_at = $1 WHERE id = $2`, now, inviteID); err != nil {
		return models.QuestBoard{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.QuestBoard{}, err
	}
	return s.QuestBoardForUser(userID)
}

// InsertSharedQuest creates one normal quest row per assignee and links the
// copies. Existing per-user completion machinery then remains the only XP path.
func (s *Store) InsertSharedQuest(userID int64, in models.QuestInput, assigneeIDs []int64) (models.Quest, error) {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.Quest{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var boardID int64
	if err := tx.QueryRow(
		`SELECT b.id FROM quest_boards b JOIN quest_board_members m ON m.board_id = b.id
		 WHERE m.user_id = $1 FOR UPDATE OF b`, userID).Scan(&boardID); err != nil {
		if err == sql.ErrNoRows {
			return models.Quest{}, ErrNotFound
		}
		return models.Quest{}, err
	}
	for _, assigneeID := range assigneeIDs {
		var member bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM quest_board_members WHERE board_id = $1 AND user_id = $2)`,
			boardID, assigneeID).Scan(&member); err != nil {
			return models.Quest{}, err
		}
		if !member {
			return models.Quest{}, ErrNotFound
		}
	}

	now := time.Now().UTC()
	var sharedID int64
	if err := tx.QueryRow(
		`INSERT INTO shared_quests(board_id, created_by, created_at) VALUES($1, $2, $3) RETURNING id`,
		boardID, userID, now).Scan(&sharedID); err != nil {
		return models.Quest{}, err
	}
	var firstQuestID int64
	for _, assigneeID := range assigneeIDs {
		var questID int64
		if err := tx.QueryRow(
			`INSERT INTO quests(user_id, title, description, type, difficulty, status, attribute_rewards,
			 skip_count, created_at, due_date, shared_quest_id)
			 VALUES($1, $2, $3, $4, $5, 'active', $6, 0, $7, $8, $9) RETURNING id`,
			assigneeID, in.Title, in.Description, in.Type, in.Difficulty, marshalRewards(in.AttributeRewards),
			now, nullTime(in.DueDate), sharedID).Scan(&questID); err != nil {
			return models.Quest{}, err
		}
		if firstQuestID == 0 {
			firstQuestID = questID
		}
		if err := insertSubtasksTx(tx, assigneeID, questID, in.Subtasks, now); err != nil {
			return models.Quest{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.Quest{}, err
	}
	return s.GetVisibleQuest(userID, firstQuestID)
}

func insertSubtasksTx(tx *sql.Tx, userID, questID int64, subtasks []models.SubtaskInput, now time.Time) error {
	for _, st := range subtasks {
		if _, err := tx.Exec(
			`INSERT INTO quest_subtasks(user_id, quest_id, title, attribute_rewards, done, created_at)
			 VALUES($1, $2, $3, $4, 0, $5)`,
			userID, questID, st.Title, marshalRewards(st.AttributeRewards), now); err != nil {
			return err
		}
	}
	return nil
}

// ListVisibleQuests returns personal quests plus one collapsed card per linked
// shared quest on the user's board.
func (s *Store) ListVisibleQuests(userID int64, questType, status string) ([]models.Quest, error) {
	const qcols = `q.id, q.user_id, q.title, q.description, q.type, q.difficulty, q.status,
		q.attribute_rewards, q.skip_count, q.source_suggestion_id, q.created_at,
		q.completed_at, q.due_date, q.shared_quest_id, q.resume_note`
	query := `SELECT ` + qcols + ` FROM quests q WHERE
		((q.user_id = $1 AND q.shared_quest_id IS NULL) OR
		 (q.shared_quest_id IS NOT NULL AND EXISTS (
		   SELECT 1 FROM shared_quests sq JOIN quest_board_members m ON m.board_id = sq.board_id
		   WHERE sq.id = q.shared_quest_id AND m.user_id = $1
		 )))`
	args := []any{userID}
	if questType != "" {
		args = append(args, questType)
		query += fmt.Sprintf(` AND q.type = $%d`, len(args))
	}
	query += ` ORDER BY
		CASE q.type WHEN 'boss' THEN 0 WHEN 'main' THEN 1 WHEN 'daily' THEN 2 WHEN 'weekly' THEN 3 WHEN 'side' THEN 4 WHEN 'recovery' THEN 5 ELSE 6 END,
		q.created_at DESC, q.id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var raw []models.Quest
	for rows.Next() {
		q, err := scanQuest(rows)
		if err != nil {
			return nil, err
		}
		raw = append(raw, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachVisibleSubtasks(raw); err != nil {
		return nil, err
	}

	type grouped struct {
		copies []models.Quest
	}
	groups := map[int64]*grouped{}
	order := []int64{}
	personal := map[int64]models.Quest{}
	for _, q := range raw {
		if q.SharedQuestID == nil {
			key := -q.ID
			personal[key] = q
			order = append(order, key)
			continue
		}
		key := *q.SharedQuestID
		if groups[key] == nil {
			groups[key] = &grouped{}
			order = append(order, key)
		}
		groups[key].copies = append(groups[key].copies, q)
	}

	out := make([]models.Quest, 0, len(order))
	for _, key := range order {
		if key < 0 {
			q := personal[key]
			if status == "" || q.Status == status {
				out = append(out, q)
			}
			continue
		}
		copies := groups[key].copies
		q := copies[0]
		for _, copy := range copies {
			if copy.UserID == userID {
				q = copy
				break
			}
		}
		q.Assignees = make([]models.QuestAssignee, 0, len(copies))
		q.AssignedToMe = false
		q.MyStatus = ""
		allCompleted := true
		anyActive := false
		anySkipped := false
		allArchived := true
		var latestCompleted *time.Time
		for _, copy := range copies {
			u, err := s.GetUser(copy.UserID)
			if err != nil {
				return nil, err
			}
			q.Assignees = append(q.Assignees, models.QuestAssignee{
				UserID: copy.UserID, Name: u.Name, Status: copy.Status, CompletedAt: copy.CompletedAt,
			})
			if copy.UserID == userID {
				q.AssignedToMe = true
				q.MyStatus = copy.Status
			}
			allCompleted = allCompleted && copy.Status == models.StatusCompleted
			anyActive = anyActive || copy.Status == models.StatusActive
			anySkipped = anySkipped || copy.Status == models.StatusSkipped
			allArchived = allArchived && copy.Status == models.StatusArchived
			if copy.CompletedAt != nil && (latestCompleted == nil || copy.CompletedAt.After(*latestCompleted)) {
				t := *copy.CompletedAt
				latestCompleted = &t
			}
		}
		sort.Slice(q.Assignees, func(i, j int) bool { return q.Assignees[i].UserID < q.Assignees[j].UserID })
		q.AllCompleted = allCompleted
		switch {
		case allCompleted:
			q.Status = models.StatusCompleted
			q.CompletedAt = latestCompleted
		case anyActive:
			q.Status = models.StatusActive
			q.CompletedAt = nil
		case allArchived:
			q.Status = models.StatusArchived
			q.CompletedAt = nil
		case anySkipped:
			q.Status = models.StatusSkipped
			q.CompletedAt = nil
		default:
			q.Status = models.StatusActive
			q.CompletedAt = nil
		}
		if status == "" || q.Status == status {
			out = append(out, q)
		}
	}
	return out, nil
}

func (s *Store) attachVisibleSubtasks(quests []models.Quest) error {
	if len(quests) == 0 {
		return nil
	}
	args := make([]any, 0, len(quests))
	ph := make([]string, 0, len(quests))
	for _, q := range quests {
		args = append(args, q.ID)
		ph = append(ph, fmt.Sprintf("$%d", len(args)))
	}
	rows, err := s.db.Query(
		`SELECT id, quest_id, title, attribute_rewards, done FROM quest_subtasks
		 WHERE quest_id IN (`+strings.Join(ph, ",")+`) ORDER BY id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	byQuest := map[int64][]models.Subtask{}
	for rows.Next() {
		st, err := scanSubtask(rows)
		if err != nil {
			return err
		}
		byQuest[st.QuestID] = append(byQuest[st.QuestID], st)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range quests {
		quests[i].Subtasks = byQuest[quests[i].ID]
		if quests[i].Subtasks == nil {
			quests[i].Subtasks = []models.Subtask{}
		}
	}
	return nil
}

func (s *Store) GetVisibleQuest(userID, questID int64) (models.Quest, error) {
	var selectedShared sql.NullInt64
	err := s.db.QueryRow(
		`SELECT q.shared_quest_id FROM quests q WHERE q.id = $1 AND
		 ((q.user_id = $2 AND q.shared_quest_id IS NULL) OR
		  (q.shared_quest_id IS NOT NULL AND EXISTS (
		    SELECT 1 FROM shared_quests sq JOIN quest_board_members m ON m.board_id = sq.board_id
		    WHERE sq.id = q.shared_quest_id AND m.user_id = $2)))`, questID, userID).Scan(&selectedShared)
	if err == sql.ErrNoRows {
		return models.Quest{}, ErrNotFound
	}
	if err != nil {
		return models.Quest{}, err
	}
	quests, err := s.ListVisibleQuests(userID, "", "")
	if err != nil {
		return models.Quest{}, err
	}
	for _, q := range quests {
		if q.ID == questID || (selectedShared.Valid && q.SharedQuestID != nil && *q.SharedQuestID == selectedShared.Int64) {
			return q, nil
		}
	}
	return models.Quest{}, ErrNotFound
}

func (s *Store) SharedQuestAccessForViewer(userID, questID int64) (SharedQuestAccess, error) {
	var access SharedQuestAccess
	var ownQuestID sql.NullInt64
	err := s.db.QueryRow(
		`SELECT sq.id, mine.id
		 FROM quests selected
		 JOIN shared_quests sq ON sq.id = selected.shared_quest_id
		 JOIN quest_board_members member ON member.board_id = sq.board_id AND member.user_id = $1
		 LEFT JOIN quests mine ON mine.shared_quest_id = sq.id AND mine.user_id = $1
		 WHERE selected.id = $2`, userID, questID).
		Scan(&access.SharedQuestID, &ownQuestID)
	if err == sql.ErrNoRows {
		return access, ErrNotFound
	}
	if err != nil {
		return access, err
	}
	if ownQuestID.Valid {
		access.OwnQuestID = ownQuestID.Int64
		access.Assigned = true
	}
	return access, nil
}

func (s *Store) UpdateSharedQuest(userID, questID int64, p models.QuestPatch) error {
	access, err := s.SharedQuestAccessForViewer(userID, questID)
	if err != nil {
		return err
	}
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`SELECT id FROM shared_quests WHERE id = $1 FOR UPDATE`, access.SharedQuestID); err != nil {
		return err
	}
	var completed bool
	dayStart, _ := localDayBounds(time.Now())
	weekStart := localWeekStart(time.Now())
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM quests WHERE shared_quest_id = $1 AND status = 'completed'
		 AND (type NOT IN ('daily','weekly')
		   OR (type = 'daily' AND completed_at >= $2)
		   OR (type = 'weekly' AND completed_at >= $3)))`,
		access.SharedQuestID, dayStart, weekStart).Scan(&completed); err != nil {
		return err
	}
	if completed {
		return ErrQuestNotCompletable
	}
	var sets []string
	var args []any
	set := func(col string, value any) {
		args = append(args, value)
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
	if p.AttributeRewards != nil {
		set("attribute_rewards", marshalRewards(*p.AttributeRewards))
	}
	if p.DueDate != nil {
		set("due_date", nullTime(p.DueDate))
	}
	if len(sets) > 0 {
		args = append(args, access.SharedQuestID)
		if _, err := tx.Exec(fmt.Sprintf(
			`UPDATE quests SET %s WHERE shared_quest_id = $%d`, strings.Join(sets, ", "), len(args)), args...); err != nil {
			return err
		}
	}
	if p.Subtasks != nil {
		rows, err := tx.Query(`SELECT id, user_id FROM quests WHERE shared_quest_id = $1 ORDER BY id`, access.SharedQuestID)
		if err != nil {
			return err
		}
		type copy struct{ id, userID int64 }
		var copies []copy
		for rows.Next() {
			var c copy
			if err := rows.Scan(&c.id, &c.userID); err != nil {
				rows.Close()
				return err
			}
			copies = append(copies, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, c := range copies {
			if _, err := tx.Exec(`DELETE FROM quest_subtasks WHERE quest_id = $1 AND user_id = $2`, c.id, c.userID); err != nil {
				return err
			}
			if err := insertSubtasksTx(tx, c.userID, c.id, *p.Subtasks, now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) RestoreSharedQuestAssignment(userID, questID int64) error {
	access, err := s.SharedQuestAccessForViewer(userID, questID)
	if err != nil {
		return err
	}
	if !access.Assigned {
		return ErrNotFound
	}
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`SELECT id FROM shared_quests WHERE id = $1 FOR UPDATE`, access.SharedQuestID); err != nil {
		return err
	}
	res, err := tx.Exec(
		`UPDATE quests SET status = 'active', completed_at = NULL WHERE id = $1 AND user_id = $2`,
		access.OwnQuestID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) ArchiveSharedQuest(userID, questID int64) error {
	access, err := s.SharedQuestAccessForViewer(userID, questID)
	if err != nil {
		return err
	}
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`SELECT id FROM shared_quests WHERE id = $1 FOR UPDATE`, access.SharedQuestID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE quests SET status = 'archived' WHERE shared_quest_id = $1`, access.SharedQuestID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SkipSharedQuestAssignment(userID, questID int64) error {
	access, err := s.SharedQuestAccessForViewer(userID, questID)
	if err != nil {
		return err
	}
	if !access.Assigned {
		return ErrNotFound
	}
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`SELECT id FROM shared_quests WHERE id = $1 FOR UPDATE`, access.SharedQuestID); err != nil {
		return err
	}
	res, err := tx.Exec(
		`UPDATE quests SET status = 'skipped', skip_count = skip_count + 1
		 WHERE id = $1 AND user_id = $2 AND status = 'active'`, access.OwnQuestID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrQuestNotCompletable
	}
	return tx.Commit()
}
