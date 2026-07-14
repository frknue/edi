package db

import (
	"encoding/json"
	"time"

	"edi/internal/models"
)

// CompleteTool records a tool completion and awards XP atomically: it inserts a
// tool_entries row, writes one xp_event per rewarded attribute (source='tool'),
// bumps attribute totals, and advances the streak — mirroring quest completion
// so the XP audit invariant (total_xp == SUM(xp_events)) always holds.
func (s *Store) CompleteTool(userID int64, toolKey, toolName string, data []byte, summary string, rewards map[string]int64) (models.ToolEntry, []models.XPEvent, []models.LevelUp, int64, error) {
	names, err := s.AttributeNames(userID)
	if err != nil {
		return models.ToolEntry{}, nil, nil, 0, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return models.ToolEntry{}, nil, nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	nowStr := formatTime(now)

	var total int64
	for _, v := range rewards {
		total += v
	}

	res, err := tx.Exec(
		`INSERT INTO tool_entries(user_id, tool_key, data, summary, xp_awarded, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		userID, toolKey, string(data), summary, total, nowStr)
	if err != nil {
		return models.ToolEntry{}, nil, nil, 0, err
	}
	entryID, _ := res.LastInsertId()

	var events []models.XPEvent
	var levelUps []models.LevelUp
	var goldTotal int64
	for _, key := range orderedKeys(rewards) {
		amount := rewards[key]
		if amount == 0 {
			continue
		}
		var oldXP int64
		if err := tx.QueryRow(`SELECT total_xp FROM attributes WHERE user_id = ? AND key = ?`, userID, key).Scan(&oldXP); err != nil {
			continue // unknown attribute key — skip
		}
		ev, err := tx.Exec(
			`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES(?, ?, ?, 'tool', ?, ?, ?)`,
			userID, key, amount, entryID, toolName, nowStr)
		if err != nil {
			return models.ToolEntry{}, nil, nil, 0, err
		}
		if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + ? WHERE user_id = ? AND key = ?`, amount, userID, key); err != nil {
			return models.ToolEntry{}, nil, nil, 0, err
		}
		if g := goldForXP(amount); g > 0 {
			if _, err := insertGoldEventTx(tx, userID, g, "tool", toolName, nil, nowStr); err != nil {
				return models.ToolEntry{}, nil, nil, 0, err
			}
			goldTotal += g
		}
		evID, _ := ev.LastInsertId()
		sid := entryID
		events = append(events, models.XPEvent{
			ID: evID, AttributeKey: key, AttributeName: names[key], Amount: amount,
			Source: "tool", SourceID: &sid, Note: toolName, CreatedAt: now,
		})
		if from, to := levelFromTo(oldXP, oldXP+amount); to > from {
			levelUps = append(levelUps, models.LevelUp{
				AttributeKey: key, AttributeName: names[key], FromLevel: from, ToLevel: to,
			})
		}
	}

	if err := updateStreakTx(tx, userID, now); err != nil {
		return models.ToolEntry{}, nil, nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return models.ToolEntry{}, nil, nil, 0, err
	}

	return models.ToolEntry{
		ID: entryID, ToolKey: toolKey, Data: json.RawMessage(data),
		XPAwarded: total, Summary: summary, CreatedAt: now,
	}, events, levelUps, goldTotal, nil
}

// ListToolEntries returns recent entries for a tool (most recent first).
func (s *Store) ListToolEntries(userID int64, toolKey string, limit int) ([]models.ToolEntry, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(
		`SELECT id, tool_key, data, summary, xp_awarded, created_at
		 FROM tool_entries WHERE user_id = ? AND tool_key = ? ORDER BY id DESC LIMIT ?`,
		userID, toolKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ToolEntry
	for rows.Next() {
		var e models.ToolEntry
		var data, created string
		if err := rows.Scan(&e.ID, &e.ToolKey, &data, &e.Summary, &e.XPAwarded, &created); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		e.CreatedAt = mustParseTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}
