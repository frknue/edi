package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"edi/internal/models"
)

func scanShopItem(scanner interface{ Scan(...any) error }) (models.ShopItem, error) {
	var it models.ShopItem
	var archived sql.NullTime
	if err := scanner.Scan(&it.ID, &it.UserID, &it.Name, &it.Price, &it.CreatedAt, &archived); err != nil {
		return it, err
	}
	it.ArchivedAt = timePtr(archived)
	return it, nil
}

const shopColumns = `id, user_id, name, price, created_at, archived_at`

// ListShopItems returns active (non-archived) items, oldest first.
func (s *Store) ListShopItems(userID int64) ([]models.ShopItem, error) {
	rows, err := s.db.Query(`SELECT `+shopColumns+` FROM shop_items WHERE user_id = $1 AND archived_at IS NULL ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ShopItem
	for rows.Next() {
		it, err := scanShopItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) GetShopItem(userID, id int64) (models.ShopItem, error) {
	row := s.db.QueryRow(`SELECT `+shopColumns+` FROM shop_items WHERE id = $1 AND user_id = $2`, id, userID)
	it, err := scanShopItem(row)
	if err == sql.ErrNoRows {
		return it, ErrNotFound
	}
	return it, err
}

func (s *Store) InsertShopItem(userID int64, in models.ShopItemInput) (models.ShopItem, error) {
	var id int64
	err := s.db.QueryRow(`INSERT INTO shop_items(user_id, name, price, created_at) VALUES($1, $2, $3, $4) RETURNING id`,
		userID, in.Name, in.Price, time.Now().UTC()).Scan(&id)
	if err != nil {
		return models.ShopItem{}, err
	}
	return s.GetShopItem(userID, id)
}

// UpdateShopItem patches an ACTIVE item; archived/missing -> ErrNotFound.
func (s *Store) UpdateShopItem(userID, id int64, p models.ShopItemPatch) (models.ShopItem, error) {
	var sets []string
	var args []any
	set := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if p.Name != nil {
		set("name", *p.Name)
	}
	if p.Price != nil {
		set("price", *p.Price)
	}
	if len(sets) == 0 {
		// Nothing to change — still 404 if the item isn't active.
		it, err := s.GetShopItem(userID, id)
		if err == nil && it.ArchivedAt != nil {
			return models.ShopItem{}, ErrNotFound
		}
		return it, err
	}
	args = append(args, id, userID)
	q := fmt.Sprintf(`UPDATE shop_items SET %s WHERE id = $%d AND user_id = $%d AND archived_at IS NULL`,
		strings.Join(sets, ", "), len(args)-1, len(args))
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return models.ShopItem{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ShopItem{}, ErrNotFound
	}
	return s.GetShopItem(userID, id)
}

// ArchiveShopItem hides an item from the shop; the row (and purchase-history
// labels) stay. Idempotent archiving of an archived item -> ErrNotFound.
func (s *Store) ArchiveShopItem(userID, id int64) error {
	res, err := s.db.Exec(`UPDATE shop_items SET archived_at = $1 WHERE id = $2 AND user_id = $3 AND archived_at IS NULL`,
		time.Now().UTC(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurchaseShopItem spends gold on an active item, atomically: the balance
// check and the negative ledger write happen inside one transaction that holds
// the per-user advisory lock, so racing purchases serialize and the balance
// can never go negative (gold sibling of the CompleteQuest gate).
func (s *Store) PurchaseShopItem(userID, itemID int64) (models.PurchaseResult, error) {
	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.PurchaseResult{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()

	var it models.ShopItem
	err = tx.QueryRow(`SELECT id, user_id, name, price, created_at FROM shop_items WHERE id = $1 AND user_id = $2 AND archived_at IS NULL`,
		itemID, userID).Scan(&it.ID, &it.UserID, &it.Name, &it.Price, &it.CreatedAt)
	if err == sql.ErrNoRows {
		return models.PurchaseResult{}, ErrNotFound
	}
	if err != nil {
		return models.PurchaseResult{}, err
	}

	var balance int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = $1`, userID).Scan(&balance); err != nil {
		return models.PurchaseResult{}, err
	}
	if balance < it.Price {
		return models.PurchaseResult{}, ErrInsufficientGold
	}

	evID, err := insertGoldEventTx(tx, userID, -it.Price, "purchase", it.Name, &it.ID, now)
	if err != nil {
		return models.PurchaseResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.PurchaseResult{}, err
	}

	itemID2 := it.ID
	return models.PurchaseResult{
		Item: it,
		Event: models.GoldEvent{
			ID: evID, Amount: -it.Price, Source: "purchase", Label: it.Name,
			ShopItemID: &itemID2, CreatedAt: now,
		},
		Balance: balance - it.Price,
	}, nil
}
