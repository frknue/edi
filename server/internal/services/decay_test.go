package services

import (
	"errors"
	"testing"
	"time"
)

func TestWardAttribute(t *testing.T) {
	svc := newTestService(t) // seed balance: 252 gold
	res, err := svc.WardAttribute("strength")
	if err != nil {
		t.Fatalf("ward: %v", err)
	}
	if res.Balance != 252-WardCostGold {
		t.Errorf("balance = %d, want %d", res.Balance, 252-WardCostGold)
	}
	wantExpiry := time.Now().Add(time.Duration(WardDays) * 24 * time.Hour)
	if diff := res.Ward.ExpiresAt.Sub(wantExpiry); diff < -time.Minute || diff > time.Minute {
		t.Errorf("expiry %v not ~7 days out", res.Ward.ExpiresAt)
	}

	// Re-warding extends from the CURRENT expiry (stacking).
	res2, err := svc.WardAttribute("strength")
	if err != nil {
		t.Fatalf("re-ward: %v", err)
	}
	if got := res2.Ward.ExpiresAt.Sub(res.Ward.ExpiresAt); got < 7*24*time.Hour-time.Minute || got > 7*24*time.Hour+time.Minute {
		t.Errorf("stacked expiry extends %v, want ~7 days beyond the first", got)
	}

	// Gold ledger has two 'ward' spends with the attribute name in the label.
	events, _ := svc.ListGoldEvents(10, "ward")
	if len(events) != 2 || events[0].Amount != -WardCostGold || events[0].Label != "Maintenance Ward · Strength" {
		t.Errorf("ward gold events = %+v", events)
	}
}

func TestWardErrors(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.WardAttribute("nonsense"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown attribute: got %v, want ErrNotFound", err)
	}
	// Drain the balance: 252 gold buys 8 wards (240), 9th must fail.
	for i := 0; i < 8; i++ {
		if _, err := svc.WardAttribute("focus"); err != nil {
			t.Fatalf("ward %d: %v", i, err)
		}
	}
	if _, err := svc.WardAttribute("focus"); !errors.Is(err, ErrValidation) {
		t.Errorf("insufficient gold: got %v, want ErrValidation", err)
	}
	bal, _ := svc.GoldBalance()
	if bal != 252-8*WardCostGold {
		t.Errorf("balance = %d, want %d (failed ward must not charge)", bal, 252-8*WardCostGold)
	}
}
