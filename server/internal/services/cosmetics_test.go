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

	staffPrice := priceOf(t, cat, "wooden_staff")
	res, err := svc.BuyCosmetic("Wooden_Staff") // key lookup is case-insensitive
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if res.Balance != 252-staffPrice || res.Event.Amount != -staffPrice || res.Event.Source != "cosmetic" || !strings.HasPrefix(res.Event.Label, "Wooden Staff") {
		t.Errorf("result = %+v (staff price %d)", res, staffPrice)
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
	if bal, _ := svc.GoldBalance(); bal != 252-staffPrice {
		t.Errorf("balance after rejected double buy = %d, want %d", bal, 252-staffPrice)
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
	cat, _ := svc.ListCosmetics()
	if bal, _ := svc.GoldBalance(); bal != 252-priceOf(t, cat, "slime") {
		t.Errorf("balance = %d, want %d", bal, 252-priceOf(t, cat, "slime"))
	}
}

// priceOf reads the effective (deal-aware) price of a catalog item.
func priceOf(t *testing.T, cat models.CosmeticCatalog, key string) int64 {
	t.Helper()
	for _, it := range cat.Items {
		if it.Key == key {
			return it.Price
		}
	}
	t.Fatalf("no item %q in catalog", key)
	return 0
}

func TestDailyDealDeterministicAndCharged(t *testing.T) {
	svc := newTestService(t)
	a, _ := svc.ListCosmetics()
	b, _ := svc.ListCosmetics()
	if a.Deal == nil || b.Deal == nil || a.Deal.Key != b.Deal.Key {
		t.Fatalf("deal should be stable within a day: %+v vs %+v", a.Deal, b.Deal)
	}
	d := a.Deal
	want := d.ListPrice - d.ListPrice*dealPercent/100
	if want < 1 {
		want = 1
	}
	if d.Price != want || d.Percent != dealPercent || d.Day == "" {
		t.Errorf("deal = %+v, want price %d", d, want)
	}
	item := cosmeticByKey[d.Key]
	if item.Key == "" {
		t.Fatalf("deal key %q not in catalog", d.Key)
	}
	if priceOf(t, a, d.Key) != d.Price {
		t.Errorf("item price %d != deal price %d", priceOf(t, a, d.Key), d.Price)
	}
	// The exact deal item may be level-locked for the seed hero; the charged
	// price is checked through the same function the purchase uses.
	if item.MinLevel <= a.Level && a.Balance >= d.Price {
		res, err := svc.BuyCosmetic(d.Key)
		if err != nil {
			t.Fatalf("buy deal: %v", err)
		}
		if res.Event.Amount != -d.Price || !strings.Contains(res.Event.Label, "daily deal") {
			t.Errorf("deal purchase event = %+v, want -%d with a deal label", res.Event, d.Price)
		}
		after, _ := svc.ListCosmetics()
		if after.Deal != nil && after.Deal.Key == d.Key {
			t.Errorf("deal still points at the bought piece: %+v", after.Deal)
		}
	}
	// Buying a DIFFERENT piece must not move the deal (rendezvous hashing).
	before, _ := svc.ListCosmetics()
	if before.Deal != nil {
		for _, it := range before.Items {
			if it.Key != before.Deal.Key && it.Unlocked && !it.Owned && it.Price <= before.Balance {
				if _, err := svc.BuyCosmetic(it.Key); err != nil {
					t.Fatalf("buy other: %v", err)
				}
				break
			}
		}
		afterOther, _ := svc.ListCosmetics()
		if afterOther.Deal == nil || afterOther.Deal.Key != before.Deal.Key {
			t.Errorf("deal moved after buying another piece: %+v → %+v", before.Deal, afterOther.Deal)
		}
	}
	// Different users get (potentially) different deals but each stays fixed.
	other := blankUser(t, svc)
	o1, _ := other.ListCosmetics()
	o2, _ := other.ListCosmetics()
	if o1.Deal == nil || o1.Deal.Key != o2.Deal.Key {
		t.Errorf("second user's deal unstable: %+v vs %+v", o1.Deal, o2.Deal)
	}
}

func TestGearGoal(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.SetGearGoal("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown goal: got %v, want ErrNotFound", err)
	}
	// A locked piece is a fine goal (saving while leveling).
	g, err := svc.SetGearGoal("crown_of_dawn")
	if err != nil || g == nil || g.Item.Key != "crown_of_dawn" {
		t.Fatalf("set locked goal: %v %+v", err, g)
	}
	if g.Balance != 252 || g.Missing != g.Item.Price-252 || g.Progress <= 0 || g.Progress >= 1 {
		t.Errorf("goal math = %+v", g)
	}
	dash, err := svc.GetDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dash.GearGoal == nil || dash.GearGoal.Item.Key != "crown_of_dawn" {
		t.Errorf("dashboard gear_goal = %+v", dash.GearGoal)
	}
	cat, _ := svc.ListCosmetics()
	if cat.Goal == nil || cat.Goal.Item.Key != "crown_of_dawn" || cat.OwnedCount != 0 || cat.Total != len(cosmeticCatalog) || cat.AvgQuestGold < 1 {
		t.Errorf("catalog goal/meta = %+v %d/%d avg %d", cat.Goal, cat.OwnedCount, cat.Total, cat.AvgQuestGold)
	}
	// The next-unlock carrot is the next catalog TIER, not the next level:
	// its XP distance is measured from the tier's level threshold.
	nu := cat.NextUnlock
	if nu == nil || nu.Level <= cat.Level || len(nu.Keys) == 0 {
		t.Fatalf("next_unlock = %+v (level %d)", nu, cat.Level)
	}
	_, total, _ := svc.characterProgress()
	if nu.XPToGo != XPForLevel(nu.Level)-total {
		t.Errorf("next_unlock xp_to_go = %d, want %d", nu.XPToGo, XPForLevel(nu.Level)-total)
	}
	for _, d := range cosmeticCatalog {
		if d.MinLevel > cat.Level && d.MinLevel < nu.Level {
			t.Errorf("tier %d (%s) is lower than next_unlock %d", d.MinLevel, d.Key, nu.Level)
		}
	}
	// Re-target to an affordable piece: progress caps at 1, missing 0.
	g, err = svc.SetGearGoal("slime")
	if err != nil || g.Missing != 0 || g.Progress != 1 {
		t.Fatalf("affordable goal: %v %+v", err, g)
	}
	// Buying the goal clears it; setting an owned piece as goal is a 400.
	if _, err := svc.BuyCosmetic("slime"); err != nil {
		t.Fatal(err)
	}
	cat, _ = svc.ListCosmetics()
	if cat.Goal != nil {
		t.Errorf("goal should clear after buying it: %+v", cat.Goal)
	}
	if v, _ := svc.store.GetSetting(1, settingGearGoal); v != "" {
		t.Errorf("gear_goal setting = %q after purchase, want empty", v)
	}
	if _, err := svc.SetGearGoal("slime"); !errors.Is(err, ErrValidation) {
		t.Errorf("owned goal: got %v, want ErrValidation", err)
	}
	if err := svc.ClearGearGoal(); err != nil {
		t.Errorf("clear (idempotent): %v", err)
	}
	if v, _ := svc.store.GetSetting(1, settingGearGoal); v != "" {
		t.Errorf("gear_goal setting = %q after clear, want empty", v)
	}
	// An owned goal set directly (e.g. bought from another client) reads as nil.
	if err := svc.store.SetSetting(1, settingGearGoal, "slime"); err != nil {
		t.Fatal(err)
	}
	cat, _ = svc.ListCosmetics()
	if cat.Goal != nil {
		t.Errorf("owned goal should resolve to nil, got %+v", cat.Goal)
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
