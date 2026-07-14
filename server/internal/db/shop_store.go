package db

import (
	"database/sql"
	"strings"

	"edi/internal/models"
)

func scanShopItem(scanner interface{ Scan(...any) error }) (models.ShopItem, error) {
	var it models.ShopItem
	var created string
	var archived sql.NullString
	if err := scanner.Scan(&it.ID, &it.UserID, &it.Name, &it.Price, &created, &archived); err != nil {
		return it, err
	}
	it.CreatedAt = mustParseTime(created)
	it.ArchivedAt = parseTimePtr(archived)
	return it, nil
}

const shopColumns = `id, user_id, name, price, created_at, archived_at`

// ListShopItems returns active (non-archived) items, oldest first.
func (s *Store) ListShopItems(userID int64) ([]models.ShopItem, error) {
	rows, err := s.db.Query(`SELECT `+shopColumns+` FROM shop_items WHERE user_id = ? AND archived_at IS NULL ORDER BY id`, userID)
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
	row := s.db.QueryRow(`SELECT `+shopColumns+` FROM shop_items WHERE id = ? AND user_id = ?`, id, userID)
	it, err := scanShopItem(row)
	if err == sql.ErrNoRows {
		return it, ErrNotFound
	}
	return it, err
}

func (s *Store) InsertShopItem(userID int64, in models.ShopItemInput) (models.ShopItem, error) {
	res, err := s.db.Exec(`INSERT INTO shop_items(user_id, name, price, created_at) VALUES(?, ?, ?, ?)`,
		userID, in.Name, in.Price, nowString())
	if err != nil {
		return models.ShopItem{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetShopItem(userID, id)
}

// UpdateShopItem patches an ACTIVE item; archived/missing -> ErrNotFound.
func (s *Store) UpdateShopItem(userID, id int64, p models.ShopItemPatch) (models.ShopItem, error) {
	var sets []string
	var args []any
	if p.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *p.Name)
	}
	if p.Price != nil {
		sets = append(sets, "price = ?")
		args = append(args, *p.Price)
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
	res, err := s.db.Exec(`UPDATE shop_items SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ? AND archived_at IS NULL`, args...)
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
	res, err := s.db.Exec(`UPDATE shop_items SET archived_at = ? WHERE id = ? AND user_id = ? AND archived_at IS NULL`,
		nowString(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurchaseShopItem spends gold on an active item, atomically: the balance
// check and the negative ledger write happen inside one transaction on the
// single writer connection, so racing purchases serialize and the balance can
// never go negative (gold sibling of the CompleteQuest gate).
func (s *Store) PurchaseShopItem(userID, itemID int64) (models.PurchaseResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.PurchaseResult{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	nowStr := nowString()

	var it models.ShopItem
	var created string
	err = tx.QueryRow(`SELECT id, user_id, name, price, created_at FROM shop_items WHERE id = ? AND user_id = ? AND archived_at IS NULL`,
		itemID, userID).Scan(&it.ID, &it.UserID, &it.Name, &it.Price, &created)
	if err == sql.ErrNoRows {
		return models.PurchaseResult{}, ErrNotFound
	}
	if err != nil {
		return models.PurchaseResult{}, err
	}
	it.CreatedAt = mustParseTime(created)

	var balance int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = ?`, userID).Scan(&balance); err != nil {
		return models.PurchaseResult{}, err
	}
	if balance < it.Price {
		return models.PurchaseResult{}, ErrInsufficientGold
	}

	evID, err := insertGoldEventTx(tx, userID, -it.Price, "purchase", it.Name, &it.ID, nowStr)
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
			ShopItemID: &itemID2, CreatedAt: mustParseTime(nowStr),
		},
		Balance: balance - it.Price,
	}, nil
}
