package db

import (
	"database/sql"
	"time"

	"edi/internal/models"
)

func scanWard(scanner interface{ Scan(...any) error }) (models.Ward, error) {
	var w models.Ward
	var expires, created string
	if err := scanner.Scan(&w.ID, &w.AttributeKey, &expires, &created); err != nil {
		return w, err
	}
	w.ExpiresAt = mustParseTime(expires)
	w.CreatedAt = mustParseTime(created)
	return w, nil
}

// ListWards returns every ward window for an attribute, oldest first. Lapsed
// windows still matter: the decay engine excludes the days they covered.
func (s *Store) ListWards(userID int64, attrKey string) ([]models.Ward, error) {
	rows, err := s.db.Query(
		`SELECT id, attribute_key, expires_at, created_at FROM wards
		 WHERE user_id = ? AND attribute_key = ? ORDER BY id`, userID, attrKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Ward
	for rows.Next() {
		w, err := scanWard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ActiveWardExpiry returns the latest future expiry for an attribute, or nil.
func (s *Store) ActiveWardExpiry(userID int64, attrKey string, now time.Time) (*time.Time, error) {
	var expires sql.NullString
	err := s.db.QueryRow(
		`SELECT MAX(expires_at) FROM wards WHERE user_id = ? AND attribute_key = ? AND expires_at > ?`,
		userID, attrKey, formatTime(now)).Scan(&expires)
	if err != nil {
		return nil, err
	}
	return parseTimePtr(expires), nil
}

// CreateWard buys decay protection for one attribute: balance check, gold
// spend (source 'ward'), and the ward insert happen in ONE tx on the single
// writer connection — the same never-overspend discipline as shop purchases.
// A still-active ward extends from its current expiry (stacking).
func (s *Store) CreateWard(userID int64, attrKey, attrName string, cost int64, days int) (models.Ward, int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.Ward{}, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	nowStr := formatTime(now)

	var balance int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = ?`, userID).Scan(&balance); err != nil {
		return models.Ward{}, 0, err
	}
	if balance < cost {
		return models.Ward{}, 0, ErrInsufficientGold
	}

	// Extend from the current active expiry when one exists.
	base := now
	var current sql.NullString
	if err := tx.QueryRow(
		`SELECT MAX(expires_at) FROM wards WHERE user_id = ? AND attribute_key = ? AND expires_at > ?`,
		userID, attrKey, nowStr).Scan(&current); err != nil {
		return models.Ward{}, 0, err
	}
	if cur := parseTimePtr(current); cur != nil {
		base = *cur
	}
	expires := base.Add(time.Duration(days) * 24 * time.Hour)

	res, err := tx.Exec(`INSERT INTO wards(user_id, attribute_key, expires_at, created_at) VALUES(?, ?, ?, ?)`,
		userID, attrKey, formatTime(expires), nowStr)
	if err != nil {
		return models.Ward{}, 0, err
	}
	id, _ := res.LastInsertId()

	if _, err := insertGoldEventTx(tx, userID, -cost, "ward", "Maintenance Ward · "+attrName, nil, nowStr); err != nil {
		return models.Ward{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return models.Ward{}, 0, err
	}
	return models.Ward{ID: id, AttributeKey: attrKey, ExpiresAt: expires, CreatedAt: now}, balance - cost, nil
}
