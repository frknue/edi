package db

import (
	"database/sql"
	"errors"
	"time"

	"edi/internal/models"
)

// Cosmetics: hero gear bought with gold. The catalog lives in the service
// layer (services/cosmetics.go); the store owns ownership rows, the loadout,
// and the gold spend — all inside the per-user advisory lock so a double-tap
// can never buy a piece twice or overspend (sibling of PurchaseShopItem).

var (
	// ErrCosmeticOwned — the user already owns this piece (bought once, forever).
	ErrCosmeticOwned = errors.New("cosmetic already owned")
	// ErrCosmeticNotOwned — equip of a piece the user has not bought.
	ErrCosmeticNotOwned = errors.New("cosmetic not owned")
)

// OwnedCosmetics returns the catalog keys the user owns.
func (s *Store) OwnedCosmetics(userID int64) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT item_key FROM cosmetic_items WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

// CosmeticLoadout returns slot -> equipped catalog key.
func (s *Store) CosmeticLoadout(userID int64) (map[string]string, error) {
	return cosmeticLoadoutQ(s.db, userID)
}

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func cosmeticLoadoutQ(q querier, userID int64) (map[string]string, error) {
	rows, err := q.Query(`SELECT slot, item_key FROM cosmetic_loadout WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var slot, key string
		if err := rows.Scan(&slot, &key); err != nil {
			return nil, err
		}
		out[slot] = key
	}
	return out, rows.Err()
}

// PurchaseCosmetic buys one catalog piece: owned check → balance check →
// negative gold_events row (source 'cosmetic') → ownership row → equip into
// its slot, all in ONE tx under the per-user lock. Returns the gold event
// and the balance after.
func (s *Store) PurchaseCosmetic(userID int64, key, slot, name string, price int64) (models.GoldEvent, int64, error) {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.GoldEvent{}, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()

	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM cosmetic_items WHERE user_id = $1 AND item_key = $2)`, userID, key).Scan(&exists); err != nil {
		return models.GoldEvent{}, 0, err
	}
	if exists {
		return models.GoldEvent{}, 0, ErrCosmeticOwned
	}

	var balance int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = $1`, userID).Scan(&balance); err != nil {
		return models.GoldEvent{}, 0, err
	}
	if balance < price {
		return models.GoldEvent{}, 0, ErrInsufficientGold
	}

	evID, err := insertGoldEventTx(tx, userID, -price, "cosmetic", name, nil, now)
	if err != nil {
		return models.GoldEvent{}, 0, err
	}
	if _, err := tx.Exec(`INSERT INTO cosmetic_items(user_id, item_key, created_at) VALUES($1, $2, $3)`, userID, key, now); err != nil {
		return models.GoldEvent{}, 0, err
	}
	if err := equipCosmeticTx(tx, userID, slot, key, now); err != nil {
		return models.GoldEvent{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return models.GoldEvent{}, 0, err
	}
	return models.GoldEvent{ID: evID, Amount: -price, Source: "cosmetic", Label: name, CreatedAt: now}, balance - price, nil
}

func equipCosmeticTx(tx *sql.Tx, userID int64, slot, key string, now time.Time) error {
	_, err := tx.Exec(`INSERT INTO cosmetic_loadout(user_id, slot, item_key, updated_at) VALUES($1, $2, $3, $4)
		ON CONFLICT(user_id, slot) DO UPDATE SET item_key = EXCLUDED.item_key, updated_at = EXCLUDED.updated_at`,
		userID, slot, key, now)
	return err
}

// EquipCosmetic wears an OWNED piece in its slot (ownership is re-checked
// inside the tx). Idempotent.
func (s *Store) EquipCosmetic(userID int64, key, slot string) error {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var owned bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM cosmetic_items WHERE user_id = $1 AND item_key = $2)`, userID, key).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return ErrCosmeticNotOwned
	}
	if err := equipCosmeticTx(tx, userID, slot, key, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

// UnequipCosmetic empties a slot (the hero falls back to the level look).
// Unequipping an empty slot is a no-op.
func (s *Store) UnequipCosmetic(userID int64, slot string) error {
	_, err := s.db.Exec(`DELETE FROM cosmetic_loadout WHERE user_id = $1 AND slot = $2`, userID, slot)
	return err
}
