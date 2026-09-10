package services

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"edi/internal/models"
)

func TestCosmeticCatalogIntegrity(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range cosmeticCatalog {
		if seen[d.Key] {
			t.Errorf("duplicate key %q", d.Key)
		}
		seen[d.Key] = true
		if !validCosmeticSlot(d.Slot) {
			t.Errorf("%s: unknown slot %q", d.Key, d.Slot)
		}
		if _, ok := rarityRank[d.Rarity]; !ok {
			t.Errorf("%s: unknown rarity %q", d.Key, d.Rarity)
		}
		if d.Price <= 0 || d.MinLevel < 1 || d.Shape == "" || !strings.HasPrefix(d.Color, "#") {
			t.Errorf("%s: bad def %+v", d.Key, d)
		}
	}
}

func TestCosmeticBuyEquipUnequip(t *testing.T) {
	svc := newTestService(t) // seed balance 252
	cat, err := svc.ListCosmetics()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if cat.Balance != 252 || len(cat.Items) != len(cosmeticCatalog) || len(cat.Loadout) != 0 {
		t.Fatalf("catalog = balance %d, %d items, %d worn; want 252, %d, 0", cat.Balance, len(cat.Items), len(cat.Loadout), len(cosmeticCatalog))
	}

	res, err := svc.BuyCosmetic("Wooden_Staff") // key lookup is case-insensitive
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if res.Balance != 237 || res.Event.Amount != -15 || res.Event.Source != "cosmetic" || res.Event.Label != "Wooden Staff" {
		t.Errorf("result = %+v", res)
	}
	if !res.Item.Owned || !res.Item.Equipped {
		t.Errorf("bought item not owned+equipped: %+v", res.Item)
	}
	if len(res.Loadout) != 1 || res.Loadout[0].Slot != "weapon" || res.Loadout[0].Key != "wooden_staff" {
		t.Errorf("loadout after buy = %+v", res.Loadout)
	}

	// Bought once, forever: a second buy is a 400 and spends nothing.
	if _, err := svc.BuyCosmetic("wooden_staff"); !errors.Is(err, ErrValidation) {
		t.Errorf("double buy: got %v, want ErrValidation", err)
	}
	if bal, _ := svc.GoldBalance(); bal != 237 {
		t.Errorf("balance after rejected double buy = %d, want 237", bal)
	}

	// A second weapon replaces the first in the slot; the first stays owned.
	if _, err := svc.BuyCosmetic("longsword"); err != nil {
		t.Fatalf("buy longsword: %v", err)
	}
	cat, _ = svc.ListCosmetics()
	var staff, sword models.CosmeticItem
	for _, it := range cat.Items {
		switch it.Key {
		case "wooden_staff":
			staff = it
		case "longsword":
			sword = it
		}
	}
	if !staff.Owned || staff.Equipped || !sword.Owned || !sword.Equipped {
		t.Errorf("staff=%+v sword=%+v", staff, sword)
	}

	// Re-equip the staff, then empty the slot.
	lo, err := svc.EquipCosmetic("wooden_staff")
	if err != nil || len(lo) != 1 || lo[0].Key != "wooden_staff" {
		t.Fatalf("equip: %v, %+v", err, lo)
	}
	lo, err = svc.UnequipCosmetic("weapon")
	if err != nil || len(lo) != 0 {
		t.Fatalf("unequip: %v, %+v", err, lo)
	}
	if _, err := svc.UnequipCosmetic("weapon"); err != nil {
		t.Errorf("unequip empty slot should be a no-op: %v", err)
	}
	if _, err := svc.UnequipCosmetic("hat"); !errors.Is(err, ErrValidation) {
		t.Errorf("unknown slot: got %v, want ErrValidation", err)
	}

	// The dashboard carries the loadout.
	if _, err := svc.EquipCosmetic("longsword"); err != nil {
		t.Fatal(err)
	}
	dash, err := svc.GetDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if len(dash.Loadout) != 1 || dash.Loadout[0].Key != "longsword" || dash.Loadout[0].Color == "" {
		t.Errorf("dashboard loadout = %+v", dash.Loadout)
	}
}

func TestCosmeticClientErrors(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.BuyCosmetic("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown key buy: got %v, want ErrNotFound", err)
	}
	if _, err := svc.EquipCosmetic("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown key equip: got %v, want ErrNotFound", err)
	}
	if _, err := svc.EquipCosmetic("iron_helm"); !errors.Is(err, ErrValidation) {
		t.Errorf("equip unowned: got %v, want ErrValidation", err)
	}
	// Level gate: the seeded hero is far below 10.
	cat, _ := svc.ListCosmetics()
	if cat.Level >= 10 {
		t.Skipf("seed level %d too high for the gate test", cat.Level)
	}
	_, err := svc.BuyCosmetic("crown_of_dawn")
	if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "level") {
		t.Errorf("below min level: got %v, want ErrValidation mentioning level", err)
	}
	// Not enough gold: a freshly registered hero has zero gold.
	empty := blankUser(t, svc)
	if _, err := empty.BuyCosmetic("wooden_staff"); !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "gold") {
		t.Errorf("no gold: got %v, want ErrValidation mentioning gold", err)
	}
	if bal, _ := empty.GoldBalance(); bal != 0 {
		t.Errorf("balance = %d after failed buy, want 0", bal)
	}
}

// TestCosmeticConcurrentSinglePurchase: two racing buys of the same piece —
// exactly one succeeds, one gold row, one ownership row. Run with -race.
func TestCosmeticConcurrentSinglePurchase(t *testing.T) {
	svc := newTestService(t)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.BuyCosmetic("slime")
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		if e == nil {
			ok++
		} else if !errors.Is(e, ErrValidation) {
			t.Errorf("unexpected error kind: %v", e)
		}
	}
	if ok != 1 {
		t.Errorf("%d buys succeeded, want exactly 1", ok)
	}
	var goldRows, ownRows int
	if err := svc.store.DB().QueryRow(`SELECT COUNT(*) FROM gold_events WHERE user_id = 1 AND source = 'cosmetic'`).Scan(&goldRows); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.DB().QueryRow(`SELECT COUNT(*) FROM cosmetic_items WHERE user_id = 1`).Scan(&ownRows); err != nil {
		t.Fatal(err)
	}
	if goldRows != 1 || ownRows != 1 {
		t.Errorf("gold rows %d, owned rows %d; want 1 and 1", goldRows, ownRows)
	}
	if bal, _ := svc.GoldBalance(); bal != 252-30 {
		t.Errorf("balance = %d, want 222", bal)
	}
}

// blankUser registers a second, empty hero (no seed data, zero gold).
func blankUser(t *testing.T, svc *Service) *Service {
	t.Helper()
	t.Setenv("EDI_INVITE_CODE", "sesame")
	created, err := svc.RegisterUser(models.RegisterInput{Name: "Blank", InviteCode: "sesame"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return svc.ForUser(created.User.ID)
}

func TestCosmeticListsSerializeAsArrays(t *testing.T) {
	svc := blankUser(t, newTestService(t))
	cat, err := svc.ListCosmetics()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(cat)
	if strings.Contains(string(b), `"loadout":null`) || strings.Contains(string(b), `"items":null`) {
		t.Errorf("null slice in catalog: %s", b)
	}
	lo, err := svc.UnequipCosmetic("pet")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(lo); string(b) != "[]" {
		t.Errorf("unequip result = %s, want []", b)
	}
}
