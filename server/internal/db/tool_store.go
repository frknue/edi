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

	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.ToolEntry{}, nil, nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()

	var total int64
	for _, v := range rewards {
		total += v
	}

	var entryID int64
	if err := tx.QueryRow(
		`INSERT INTO tool_entries(user_id, tool_key, data, summary, xp_awarded, created_at) VALUES($1, $2, $3, $4, $5, $6) RETURNING id`,
		userID, toolKey, string(data), summary, total, now).Scan(&entryID); err != nil {
		return models.ToolEntry{}, nil, nil, 0, err
	}

	var events []models.XPEvent
	var levelUps []models.LevelUp
	var goldTotal int64
	for _, key := range orderedKeys(rewards) {
		amount := rewards[key]
		if amount == 0 {
			continue
		}
		var oldXP int64
		if err := tx.QueryRow(`SELECT total_xp FROM attributes WHERE user_id = $1 AND key = $2`, userID, key).Scan(&oldXP); err != nil {
			continue // unknown attribute key — skip
		}
		var evID int64
		if err := tx.QueryRow(
			`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES($1, $2, $3, 'tool', $4, $5, $6) RETURNING id`,
			userID, key, amount, entryID, toolName, now).Scan(&evID); err != nil {
			return models.ToolEntry{}, nil, nil, 0, err
		}
		if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + $1, peak_xp = GREATEST(peak_xp, total_xp + $1) WHERE user_id = $2 AND key = $3`, amount, userID, key); err != nil {
			return models.ToolEntry{}, nil, nil, 0, err
		}
		if g := goldForXP(amount); g > 0 {
			if _, err := insertGoldEventTx(tx, userID, g, "tool", toolName, nil, now); err != nil {
				return models.ToolEntry{}, nil, nil, 0, err
			}
			goldTotal += g
		}
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
		 FROM tool_entries WHERE user_id = $1 AND tool_key = $2 ORDER BY id DESC LIMIT $3`,
		userID, toolKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ToolEntry
	for rows.Next() {
		var e models.ToolEntry
		var data string
		if err := rows.Scan(&e.ID, &e.ToolKey, &data, &e.Summary, &e.XPAwarded, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}
