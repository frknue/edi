package services

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"edi/internal/db"
	"edi/internal/models"
)

func TestShopCreateValidation(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateShopItem(models.ShopItemInput{Name: "  ", Price: 10}); !errors.Is(err, ErrValidation) {
		t.Errorf("blank name: got %v, want ErrValidation", err)
	}
	if _, err := svc.CreateShopItem(models.ShopItemInput{Name: "Takeout", Price: 0}); !errors.Is(err, ErrValidation) {
		t.Errorf("zero price: got %v, want ErrValidation", err)
	}
	item, err := svc.CreateShopItem(models.ShopItemInput{Name: " Gaming evening ", Price: 50})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Name != "Gaming evening" || item.Price != 50 {
		t.Errorf("item = %+v, want trimmed name and price 50", item)
	}
	items, err := svc.ListShopItems()
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %v items, err %v; want 1", len(items), err)
	}
}

func TestShopPurchaseHappyPath(t *testing.T) {
	svc := newTestService(t) // seed balance: 252
	item, _ := svc.CreateShopItem(models.ShopItemInput{Name: "Takeout", Price: 30})
	res, err := svc.PurchaseShopItem(item.ID)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if res.Balance != 222 {
		t.Errorf("balance after = %d, want 222", res.Balance)
	}
	if res.Event.Amount != -30 || res.Event.Source != "purchase" || res.Event.ShopItemID == nil || *res.Event.ShopItemID != item.ID {
		t.Errorf("event = %+v, want -30 purchase referencing item %d", res.Event, item.ID)
	}
	bal, _ := svc.GoldBalance()
	if bal != 222 {
		t.Errorf("stored balance = %d, want 222", bal)
	}
}

func TestShopPurchaseInsufficientGold(t *testing.T) {
	svc := newTestService(t)
	item, _ := svc.CreateShopItem(models.ShopItemInput{Name: "New keyboard", Price: 999999})
	_, err := svc.PurchaseShopItem(item.ID)
	if !errors.Is(err, ErrValidation) {
		t.Errorf("got %v, want ErrValidation (400)", err)
	}
	if err == nil || !strings.Contains(err.Error(), "gold") {
		t.Errorf("error should mention gold: %v", err)
	}
	bal, _ := svc.GoldBalance()
	if bal != 252 {
		t.Errorf("balance changed to %d on failed purchase, want 252", bal)
	}
}

func TestShopArchivedAndMissing(t *testing.T) {
	svc := newTestService(t)
	item, _ := svc.CreateShopItem(models.ShopItemInput{Name: "Movie night", Price: 20})
	if err := svc.ArchiveShopItem(item.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	items, _ := svc.ListShopItems()
	if len(items) != 0 {
		t.Errorf("archived item still listed: %+v", items)
	}
	if _, err := svc.PurchaseShopItem(item.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("purchase archived: got %v, want ErrNotFound", err)
	}
	if _, err := svc.PurchaseShopItem(99999); !errors.Is(err, ErrNotFound) {
		t.Errorf("purchase missing: got %v, want ErrNotFound", err)
	}
	if _, err := svc.UpdateShopItem(item.ID, models.ShopItemPatch{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update archived: got %v, want ErrNotFound", err)
	}
}

// TestShopPurchaseConcurrentNoOverspend is the gold sibling of
// TestCompleteQuestConcurrentNoDoubleAward: two racing purchases of an item
// priced at the full balance — exactly one may succeed, balance must not go
// negative. Run with -race.
func TestShopPurchaseConcurrentNoOverspend(t *testing.T) {
	svc := newTestService(t)
	bal, _ := svc.GoldBalance() // 252
	item, _ := svc.CreateShopItem(models.ShopItemInput{Name: "Everything", Price: bal})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.PurchaseShopItem(item.ID)
		}(i)
	}
	wg.Wait()

	okCount := 0
	for _, e := range errs {
		if e == nil {
			okCount++
		} else if !errors.Is(e, ErrValidation) {
			t.Errorf("unexpected error kind: %v", e)
		}
	}
	if okCount != 1 {
		t.Errorf("%d purchases succeeded, want exactly 1", okCount)
	}
	after, _ := svc.GoldBalance()
	if after != 0 {
		t.Errorf("balance = %d, want 0 (never negative)", after)
	}
}

// Extend the JSON array contract to the new list endpoints: a user with no
// shop items / gold events must serialize [] not null.
func TestShopListsSerializeAsArrays(t *testing.T) {
	store, err := db.Open(t.TempDir() + "/bare.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	svc := New(store, 1) // no Seed: completely empty user

	items, err := svc.ListShopItems()
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if b, _ := json.Marshal(items); string(b) != "[]" {
		t.Errorf("shop items marshal = %s, want []", b)
	}
	events, err := svc.ListGoldEvents(10, "")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if b, _ := json.Marshal(events); string(b) != "[]" {
		t.Errorf("gold events marshal = %s, want []", b)
	}
}

// TestListGoldEventsSourceFilter guards the fix for Shop.tsx's purchase-history
// section: mints dominate the ledger (2-5 per quest completion), so filtering
// "purchase" client-side after fetching the last N events silently truncates
// to nothing once ~30 mints accumulate. The source filter must be applied at
// the query layer (idx_gold_events_source), not after truncation.
func TestListGoldEventsSourceFilter(t *testing.T) {
	svc := newTestService(t) // seeds one grant event of 252 gold
	item, err := svc.CreateShopItem(models.ShopItemInput{Name: "Book", Price: 20})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if _, err := svc.PurchaseShopItem(item.ID); err != nil {
		t.Fatalf("purchase: %v", err)
	}

	purchases, err := svc.ListGoldEvents(50, "purchase")
	if err != nil {
		t.Fatalf("list purchases: %v", err)
	}
	if len(purchases) != 1 || purchases[0].Source != "purchase" || purchases[0].Amount != -20 {
		t.Errorf("purchases = %+v, want exactly one purchase event of -20", purchases)
	}

	all, err := svc.ListGoldEvents(50, "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("all events = %+v, want 2 (grant + purchase)", all)
	}
	sources := map[string]bool{}
	for _, e := range all {
		sources[e.Source] = true
	}
	if !sources["grant"] || !sources["purchase"] {
		t.Errorf("all events sources = %v, want grant and purchase", sources)
	}
}
