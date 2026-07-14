package db

import (
	"database/sql"

	"edi/internal/models"
)

// goldForXP mirrors services.GoldForXP without importing services (avoids an
// import cycle, same trick as levelForXP in store.go). Keep both in sync:
// 1 gold per 10 XP, minimum 1 for any positive award.
func goldForXP(xp int64) int64 {
	if xp <= 0 {
		return 0
	}
	if g := xp / 10; g > 1 {
		return g
	}
	return 1
}

// insertGoldEventTx writes one gold ledger row inside an existing transaction.
// Positive amounts mint, negative amounts spend.
func insertGoldEventTx(tx *sql.Tx, userID, amount int64, source, label string, shopItemID *int64, nowStr string) (int64, error) {
	res, err := tx.Exec(
		`INSERT INTO gold_events(user_id, amount, source, label, shop_item_id, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		userID, amount, source, label, nullInt64(shopItemID), nowStr)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// GoldBalance computes the spendable balance as SUM(gold_events.amount) — the
// same auditable compute-on-read pattern as attribute XP. Never stored.
func (s *Store) GoldBalance(userID int64) (int64, error) {
	var bal int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = ?`, userID).Scan(&bal)
	return bal, err
}

// ListGoldEvents returns the most recent gold ledger rows (mints and purchases).
func (s *Store) ListGoldEvents(userID int64, limit int) ([]models.GoldEvent, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(
		`SELECT id, amount, source, label, shop_item_id, created_at
		 FROM gold_events WHERE user_id = ? ORDER BY id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.GoldEvent
	for rows.Next() {
		var e models.GoldEvent
		var itemID sql.NullInt64
		var created string
		if err := rows.Scan(&e.ID, &e.Amount, &e.Source, &e.Label, &itemID, &created); err != nil {
			return nil, err
		}
		if itemID.Valid {
			v := itemID.Int64
			e.ShopItemID = &v
		}
		e.CreatedAt = mustParseTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}
