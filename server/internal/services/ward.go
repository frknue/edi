package services

import (
	"errors"

	"edi/internal/db"
	"edi/internal/models"
)

// WardAttribute buys a Maintenance Ward: WardCostGold gold shields one
// attribute from decay for WardDays days (extends a still-active ward).
func (s *Service) WardAttribute(key string) (models.WardResult, error) {
	names, err := s.store.AttributeNames(s.userID)
	if err != nil {
		return models.WardResult{}, err
	}
	name, ok := names[key]
	if !ok {
		return models.WardResult{}, ErrNotFound
	}
	ward, balance, err := s.store.CreateWard(s.userID, key, name, WardCostGold, WardDays)
	switch {
	case errors.Is(err, db.ErrInsufficientGold):
		return models.WardResult{}, validationErr("not enough gold for a ward (%dg)", WardCostGold)
	case err != nil:
		return models.WardResult{}, err
	}
	return models.WardResult{Ward: ward, Balance: balance}, nil
}
