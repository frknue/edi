package services

import (
	"errors"
	"strings"

	"edi/internal/db"
	"edi/internal/models"
)

// The reward shop: self-defined real-life rewards purchasable with gold.
// Items are repeatable; big one-time rewards are simply archived after buying.

func validateShopFields(name string, price int64) error {
	if strings.TrimSpace(name) == "" {
		return validationErr("name is required")
	}
	if price <= 0 {
		return validationErr("price must be greater than 0")
	}
	return nil
}

// ListShopItems returns the active reward-shop items.
func (s *Service) ListShopItems() ([]models.ShopItem, error) {
	items, err := s.store.ListShopItems(s.userID)
	return orEmpty(items), err
}

// CreateShopItem validates and adds a reward to the shop.
func (s *Service) CreateShopItem(in models.ShopItemInput) (models.ShopItem, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := validateShopFields(in.Name, in.Price); err != nil {
		return models.ShopItem{}, err
	}
	return s.store.InsertShopItem(s.userID, in)
}

// UpdateShopItem applies a partial patch to an active item.
func (s *Service) UpdateShopItem(id int64, p models.ShopItemPatch) (models.ShopItem, error) {
	if p.Name != nil {
		trimmed := strings.TrimSpace(*p.Name)
		if trimmed == "" {
			return models.ShopItem{}, validationErr("name is required")
		}
		p.Name = &trimmed
	}
	if p.Price != nil && *p.Price <= 0 {
		return models.ShopItem{}, validationErr("price must be greater than 0")
	}
	item, err := s.store.UpdateShopItem(s.userID, id, p)
	if errors.Is(err, db.ErrNotFound) {
		return models.ShopItem{}, ErrNotFound
	}
	return item, err
}

// ArchiveShopItem removes an item from the shop (history keeps its label).
func (s *Service) ArchiveShopItem(id int64) error {
	err := s.store.ArchiveShopItem(s.userID, id)
	if errors.Is(err, db.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// PurchaseShopItem spends gold on an item. Insufficient balance is a client
// condition (400), not a server error.
func (s *Service) PurchaseShopItem(id int64) (models.PurchaseResult, error) {
	res, err := s.store.PurchaseShopItem(s.userID, id)
	switch {
	case errors.Is(err, db.ErrNotFound):
		return models.PurchaseResult{}, ErrNotFound
	case errors.Is(err, db.ErrInsufficientGold):
		return models.PurchaseResult{}, validationErr("not enough gold")
	}
	return res, err
}
