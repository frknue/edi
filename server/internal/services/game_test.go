package services

import (
	"testing"

	"edi/internal/models"
)

func TestComboMultiplierTiers(t *testing.T) {
	want := map[int]float64{0: 1.0, 1: 1.0, 2: 1.1, 3: 1.2, 4: 1.35, 5: 1.5, 9: 1.5}
	for nth, m := range want {
		if got := ComboMultiplier(nth); got != m {
			t.Errorf("ComboMultiplier(%d) = %v, want %v", nth, got, m)
		}
	}
}

// auditDrift returns how many attributes violate total_xp == SUM(xp_events).
func auditDrift(t *testing.T, svc *Service) int {
	t.Helper()
	var n int
	if err := svc.store.DB().QueryRow(`SELECT COUNT(*) FROM attributes a
		WHERE a.total_xp != (SELECT COALESCE(SUM(amount),0) FROM xp_events e
		WHERE e.attribute_key = a.key AND e.user_id = a.user_id)`).Scan(&n); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	return n
}

// A critical hit doubles the entire payout via separate auditable 'crit' rows.
func TestCompleteQuestCrit(t *testing.T) {
	svc := newTestService(t)
	svc.store.SetRollForTest(func() float64 { return 0.0 }) // always crit

	before, _ := svc.ListAttributes()
	strBefore := attrByKey(before, "strength").TotalXP

	workout := findQuestByTitle(t, svc, "30 minute workout") // strength:40, discipline:10
	res, err := svc.CompleteQuest(workout.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !res.Crit {
		t.Fatal("expected crit with a forced 0.0 roll")
	}
	if res.ComboMultiplier != 1.0 {
		t.Errorf("first completion of the day combo = %v, want 1.0", res.ComboMultiplier)
	}

	// Base rows + crit rows, amounts doubled overall.
	var critRows int
	var critXP int64
	for _, e := range res.XPEvents {
		if e.Source == "crit" {
			critRows++
			critXP += e.Amount
		}
	}
	if critRows != 2 || critXP != 50 {
		t.Errorf("crit rows = %d (%d XP), want 2 rows / 50 XP", critRows, critXP)
	}
	after, _ := svc.ListAttributes()
	if got := attrByKey(after, "strength").TotalXP; got != strBefore+80 {
		t.Errorf("strength = %d, want %d (40 base + 40 crit)", got, strBefore+80)
	}
	if auditDrift(t, svc) != 0 {
		t.Error("XP audit invariant violated after crit")
	}
}

// The combo chain pays growing bonuses for back-to-back completions, skipping
// bonuses that would floor to zero.
func TestCompleteQuestComboChain(t *testing.T) {
	svc := newTestService(t)
	svc.store.SetRollForTest(func() float64 { return 1.0 }) // never crit

	mk := func(title string, xp int64) models.Quest {
		q, err := svc.CreateQuest(models.QuestInput{Title: title, AttributeRewards: map[string]int64{"focus": xp}})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return q
	}
	q1, q2, q3 := mk("c1", 40), mk("c2", 40), mk("tiny", 5)

	r1, err := svc.CompleteQuest(q1.ID)
	if err != nil {
		t.Fatalf("q1: %v", err)
	}
	if r1.ComboMultiplier != 1.0 {
		t.Errorf("1st completion combo = %v, want 1.0", r1.ComboMultiplier)
	}

	r2, err := svc.CompleteQuest(q2.ID)
	if err != nil {
		t.Fatalf("q2: %v", err)
	}
	if r2.ComboMultiplier != 1.1 {
		t.Errorf("2nd completion combo = %v, want 1.1", r2.ComboMultiplier)
	}
	var comboXP int64
	for _, e := range r2.XPEvents {
		if e.Source == "combo" {
			comboXP += e.Amount
		}
	}
	if comboXP != 4 { // floor(40 × 0.1)
		t.Errorf("combo bonus = %d, want 4", comboXP)
	}

	// 3rd completion is ×1.2 — but a 5-XP quest floors to 1 (5×0.2); a
	// zero-bonus row must never be written, a positive one must.
	r3, err := svc.CompleteQuest(q3.ID)
	if err != nil {
		t.Fatalf("q3: %v", err)
	}
	if r3.ComboMultiplier != 1.2 {
		t.Errorf("3rd completion combo = %v, want 1.2", r3.ComboMultiplier)
	}
	for _, e := range r3.XPEvents {
		if e.Amount == 0 {
			t.Errorf("zero-amount xp_event written: %+v", e)
		}
	}

	// Dashboard advertises the NEXT multiplier.
	if r3.Dashboard.DailyProgress.NextComboMultiplier != 1.35 {
		t.Errorf("next combo = %v, want 1.35", r3.Dashboard.DailyProgress.NextComboMultiplier)
	}
	if auditDrift(t, svc) != 0 {
		t.Error("XP audit invariant violated after combo chain")
	}
}
