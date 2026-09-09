package services

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"edi/internal/models"
)

func addSupp(t *testing.T, svc *Service, name string) models.Supplement {
	t.Helper()
	sp, err := svc.AddSupplement(models.SupplementInput{Name: name})
	if err != nil {
		t.Fatalf("add %s: %v", name, err)
	}
	return sp
}

func sumXP(events []models.XPEvent) int64 {
	var n int64
	for _, e := range events {
		n += e.Amount
	}
	return n
}

func TestSupplementTakeAwardsAuditableXP(t *testing.T) {
	svc := newTestService(t)
	before, _ := svc.ListAttributes()
	healthBefore := attrByKey(before, "health").TotalXP

	d := addSupp(t, svc, "Vitamin D")
	addSupp(t, svc, "Magnesium")

	res, err := svc.TakeSupplement(d.ID)
	if err != nil {
		t.Fatalf("take: %v", err)
	}
	if res.BonusAwarded {
		t.Error("bonus paid with one of two taken")
	}
	if len(res.XPEvents) != 1 || res.XPEvents[0].Source != "supplement" || res.XPEvents[0].Amount != 3 {
		t.Fatalf("events = %+v, want one 3 XP supplement event", res.XPEvents)
	}
	if got := attrByKey(res.Dashboard.Attributes, "health").TotalXP; got != healthBefore+3 {
		t.Errorf("health = %d, want %d", got, healthBefore+3)
	}
	if res.Today.Taken != 1 || res.Today.Total != 2 || res.Today.AllTaken {
		t.Errorf("today = %+v", res.Today)
	}
	if res.Gold < 1 {
		t.Errorf("gold = %d, want >= 1 (min mint)", res.Gold)
	}

	// Second take the same day → validation error, no extra XP.
	if _, err := svc.TakeSupplement(d.ID); err == nil || !strings.Contains(err.Error(), "already taken") {
		t.Fatalf("double take err = %v, want already-taken validation error", err)
	}
	after, _ := svc.ListAttributes()
	if got := attrByKey(after, "health").TotalXP; got != healthBefore+3 {
		t.Errorf("health after double take = %d, want %d", got, healthBefore+3)
	}
}

func TestSupplementBonusOncePerDay(t *testing.T) {
	svc := newTestService(t)
	before, _ := svc.ListAttributes()
	healthBefore := attrByKey(before, "health").TotalXP
	discBefore := attrByKey(before, "discipline").TotalXP

	a := addSupp(t, svc, "Omega-3")
	b := addSupp(t, svc, "Zinc")

	if _, err := svc.TakeSupplement(a.ID); err != nil {
		t.Fatal(err)
	}
	res, err := svc.TakeSupplement(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.BonusAwarded || !res.Intake.Bonus {
		t.Fatal("expected the bonus on the take that completed the stack")
	}
	// 3 (item) + 10 (bonus health) + 5 (bonus discipline)
	if got := sumXP(res.XPEvents); got != 18 {
		t.Errorf("xp on completing take = %d, want 18", got)
	}
	if res.Intake.XPAwarded != 18 {
		t.Errorf("intake.xp_awarded = %d, want 18", res.Intake.XPAwarded)
	}
	if !res.Today.AllTaken || !res.Today.BonusAwarded {
		t.Errorf("today = %+v, want all_taken + bonus_awarded", res.Today)
	}
	attrs, _ := svc.ListAttributes()
	if got := attrByKey(attrs, "health").TotalXP; got != healthBefore+3+3+10 {
		t.Errorf("health = %d, want %d", got, healthBefore+16)
	}
	if got := attrByKey(attrs, "discipline").TotalXP; got != discBefore+5 {
		t.Errorf("discipline = %d, want %d", got, discBefore+5)
	}

	// Adding a third supplement later the same day and taking it pays the item
	// reward only — the bonus is once per local day.
	c := addSupp(t, svc, "Creatine")
	res3, err := svc.TakeSupplement(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res3.BonusAwarded || sumXP(res3.XPEvents) != 3 {
		t.Errorf("third take: bonus=%v xp=%d, want no bonus / 3 XP", res3.BonusAwarded, sumXP(res3.XPEvents))
	}
	if !res3.Today.AllTaken || !res3.Today.BonusAwarded {
		t.Errorf("today after third = %+v", res3.Today)
	}

	hist, err := svc.SupplementHistory(70)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 || hist[0].Taken != 3 || !hist[0].Bonus || hist[0].XP != 24 {
		t.Errorf("history = %+v, want one day, 3 taken, bonus, 24 XP", hist)
	}
}

// The saved stack recurs without re-adding items, even after days away. Only
// today's intake state and bonus reset; previous intakes and XP stay intact.
func TestSupplementsReappearDaily(t *testing.T) {
	for _, daysAgo := range []int{1, 3} {
		t.Run((time.Duration(daysAgo) * 24 * time.Hour).String(), func(t *testing.T) {
			svc := newTestService(t)
			a := addSupp(t, svc, "Vitamin D")
			b := addSupp(t, svc, "Magnesium")
			for _, sp := range []models.Supplement{a, b} {
				if _, err := svc.TakeSupplement(sp.ID); err != nil {
					t.Fatal(err)
				}
			}

			// Move the completed day's intakes into the past to simulate returning
			// on a new local day without changing the wall clock or erasing history.
			past := time.Now().AddDate(0, 0, -daysAgo)
			if _, err := svc.store.DB().Exec(
				`UPDATE supplement_intakes SET taken_on = $1, created_at = $2 WHERE user_id = $3`,
				localDay(past), past, svc.userID); err != nil {
				t.Fatal(err)
			}

			today, err := svc.ListSupplements()
			if err != nil {
				t.Fatal(err)
			}
			if today.Day != localDay(time.Now()) || today.Total != 2 || len(today.Supplements) != 2 || today.Taken != 0 || today.AllTaken || today.BonusAwarded {
				t.Fatalf("new day = %+v, want same two supplements with no takes or bonus", today)
			}
			for i, original := range []models.Supplement{a, b} {
				sp := today.Supplements[i]
				if sp.ID != original.ID || sp.Name != original.Name || sp.Taken || sp.TakenAt != nil {
					t.Fatalf("supplement = %+v, want original %+v available again", sp, original)
				}
				res, err := svc.TakeSupplement(sp.ID)
				if err != nil {
					t.Fatal(err)
				}
				if res.BonusAwarded != (i == 1) {
					t.Errorf("take %d bonus = %v, want bonus only on the last take", i, res.BonusAwarded)
				}
			}
			if _, err := svc.TakeSupplement(a.ID); !errors.Is(err, ErrValidation) {
				t.Fatalf("repeat take on new day = %v, want validation error", err)
			}
			history, err := svc.SupplementHistory(7)
			if err != nil {
				t.Fatal(err)
			}
			if len(history) != 2 {
				t.Fatalf("history = %+v, want both days preserved", history)
			}
			for i, day := range []string{today.Day, localDay(past)} {
				h := history[i]
				if h.Day != day || h.Taken != 2 || !h.Bonus || h.XP != 21 {
					t.Errorf("history day = %+v, want %s with two takes and 21 XP", h, day)
				}
			}
			if auditDrift(t, svc) != 0 {
				t.Error("XP audit invariant violated across days")
			}
		})
	}
}

// Two goroutines take the last two supplements of the stack at the same time:
// exactly one of them may complete the stack and pay the bonus (run with -race).
func TestSupplementConcurrentSingleBonus(t *testing.T) {
	svc := newTestService(t)
	a := addSupp(t, svc, "Iron")
	b := addSupp(t, svc, "B12")

	var wg sync.WaitGroup
	results := make([]models.SupplementTakeResult, 2)
	errs := make([]error, 2)
	for i, id := range []int64{a.ID, b.ID} {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			results[i], errs[i] = svc.TakeSupplement(id)
		}(i, id)
	}
	wg.Wait()
	bonuses := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("take %d: %v", i, errs[i])
		}
		if results[i].BonusAwarded {
			bonuses++
		}
	}
	if bonuses != 1 {
		t.Fatalf("bonus paid %d times, want exactly 1", bonuses)
	}
	events, _ := svc.ListXPEvents(50)
	var total int64
	for _, e := range events {
		if e.Source == "supplement" {
			total += e.Amount
		}
	}
	if total != 3+3+15 {
		t.Errorf("supplement xp total = %d, want 21", total)
	}
}

func TestSupplementValidationAndNotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.AddSupplement(models.SupplementInput{Name: "   "}); err == nil {
		t.Error("blank name: expected validation error")
	}
	addSupp(t, svc, "Vitamin C")
	if _, err := svc.AddSupplement(models.SupplementInput{Name: "vitamin c"}); err == nil {
		t.Error("duplicate name: expected validation error")
	}
	if _, err := svc.TakeSupplement(999999); err != ErrNotFound {
		t.Errorf("take unknown = %v, want ErrNotFound", err)
	}
	if err := svc.ArchiveSupplement(999999); err != ErrNotFound {
		t.Errorf("archive unknown = %v, want ErrNotFound", err)
	}
	// Name lookup: exact, prefix, ambiguous.
	addSupp(t, svc, "Vitamin D3")
	if sp, err := svc.FindSupplement("VITAMIN C"); err != nil || sp.Name != "Vitamin C" {
		t.Errorf("exact lookup = %+v, %v", sp, err)
	}
	if sp, err := svc.FindSupplement("vitamin d"); err != nil || sp.Name != "Vitamin D3" {
		t.Errorf("prefix lookup = %+v, %v", sp, err)
	}
	if _, err := svc.FindSupplement("vit"); err == nil {
		t.Error("ambiguous lookup: expected error")
	}
	if _, err := svc.FindSupplement("nope"); err != ErrNotFound {
		t.Errorf("missing lookup = %v, want ErrNotFound", err)
	}
}

func TestSupplementArchiveKeepsHistoryAndEmptiesCleanly(t *testing.T) {
	svc := newTestService(t)
	// JSON contract on a blank stack: empty lists are [] never null.
	if empty, _ := svc.SupplementHistory(7); empty == nil {
		t.Error("history is nil, want empty slice")
	}
	if today, _ := svc.ListSupplements(); today.Supplements == nil {
		t.Error("supplements is nil, want empty slice")
	}
	a := addSupp(t, svc, "Ashwagandha")
	if _, err := svc.TakeSupplement(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveSupplement(a.ID); err != nil {
		t.Fatal(err)
	}
	today, err := svc.ListSupplements()
	if err != nil {
		t.Fatal(err)
	}
	if today.Total != 0 || today.AllTaken {
		t.Errorf("today after archive = %+v, want empty stack, not all_taken", today)
	}
	if _, err := svc.TakeSupplement(a.ID); err != ErrNotFound {
		t.Errorf("take archived = %v, want ErrNotFound", err)
	}
	hist, _ := svc.SupplementHistory(7)
	if len(hist) != 1 || hist[0].Taken != 1 {
		t.Errorf("history after archive = %+v, want the take kept", hist)
	}
}

// Concurrent adds of the same name: the partial unique index guarantees exactly
// one lands and the loser gets a validation error, not a 500.
func TestSupplementConcurrentDuplicateAdd(t *testing.T) {
	svc := newTestService(t)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.AddSupplement(models.SupplementInput{Name: "Vitamin K2"})
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrValidation):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d adds succeeded, want 1", ok)
	}
	sp := addSupp(t, svc, "Vitamin K1")
	if _, err := svc.UpdateSupplement(sp.ID, models.SupplementInput{Name: "vitamin k2"}); !errors.Is(err, ErrValidation) {
		t.Errorf("rename onto existing = %v, want validation error", err)
	}
	// Archived names are free again.
	if err := svc.ArchiveSupplement(sp.ID); err != nil {
		t.Fatal(err)
	}
	addSupp(t, svc, "Vitamin K1")
}
