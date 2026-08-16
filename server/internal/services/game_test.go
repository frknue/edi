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

// seqRoll returns a roll func that replays a fixed dice sequence.
// CompleteQuest consumes dice in order: crit, drop, rarity, item-index.
func seqRoll(rolls ...float64) func() float64 {
	i := 0
	return func() float64 {
		v := rolls[i%len(rolls)]
		i++
		return v
	}
}

// A forced drop lands in the inventory inside the same completion, buff drops
// auto-activate until midnight, and the buff pays on the NEXT completion as
// auditable 'buff' rows.
func TestLootDropAndBuff(t *testing.T) {
	svc := newTestService(t)

	mk := func(title string) models.Quest {
		q, err := svc.CreateQuest(models.QuestInput{Title: title, AttributeRewards: map[string]int64{"focus": 40}})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return q
	}
	q1, q2 := mk("dropper"), mk("buffed")

	// no crit (0.99) → drop (0.0) → epic rarity (0.96) → first item (0.0)
	// = Prism of Momentum: +25% ALL XP until midnight.
	svc.store.SetRollForTest(seqRoll(0.99, 0.0, 0.96, 0.0))
	r1, err := svc.CompleteQuest(q1.ID)
	if err != nil {
		t.Fatalf("q1: %v", err)
	}
	if r1.Drop == nil || r1.Drop.Key != "prism_of_momentum" || r1.Drop.Rarity != "epic" {
		t.Fatalf("drop = %+v, want the epic prism", r1.Drop)
	}
	if r1.Drop.ExpiresAt == nil {
		t.Fatal("buff drop must carry its expiry")
	}
	if buffs := r1.Dashboard.ActiveBuffs; len(buffs) != 1 || buffs[0].Percent != 25 {
		t.Fatalf("dashboard buffs = %+v, want the +25%% prism", buffs)
	}

	// Inventory has it.
	items, err := svc.ListItems()
	if err != nil || len(items) != 1 || items[0].Key != "prism_of_momentum" {
		t.Fatalf("inventory = %+v (%v), want the prism", items, err)
	}

	// Next completion: no crit, no drop — but the buff pays +25% on base 40
	// (=10), PLUS the combo ×1.1 (2nd of the day, +4).
	svc.store.SetRollForTest(func() float64 { return 0.99 })
	r2, err := svc.CompleteQuest(q2.ID)
	if err != nil {
		t.Fatalf("q2: %v", err)
	}
	var buffXP, comboXP int64
	for _, e := range r2.XPEvents {
		switch e.Source {
		case "buff":
			buffXP += e.Amount
		case "combo":
			comboXP += e.Amount
		}
	}
	if buffXP != 10 {
		t.Errorf("buff bonus = %d, want 10 (+25%% of 40)", buffXP)
	}
	if comboXP != 4 {
		t.Errorf("combo bonus = %d, want 4", comboXP)
	}
	if auditDrift(t, svc) != 0 {
		t.Error("XP audit invariant violated after loot + buff")
	}
}

// Gold-cache drops write an auditable gold_events row of source 'loot'.
func TestLootGoldCache(t *testing.T) {
	svc := newTestService(t)
	q, _ := svc.CreateQuest(models.QuestInput{Title: "cache", AttributeRewards: map[string]int64{"wealth": 10}})
	before, _ := svc.GoldBalance()

	// no crit → drop → common (0.1) → item index 1 = Copper Cache (5g).
	svc.store.SetRollForTest(seqRoll(0.99, 0.0, 0.1, 0.34))
	r, err := svc.CompleteQuest(q.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if r.Drop == nil || r.Drop.Kind != "gold" {
		t.Fatalf("drop = %+v, want a gold cache", r.Drop)
	}
	after, _ := svc.GoldBalance()
	wantMint := int64(1) // 10 XP -> 1g
	if after-before != wantMint+r.Drop.Gold {
		t.Errorf("gold delta = %d, want %d (mint) + %d (cache)", after-before, wantMint, r.Drop.Gold)
	}
}

// After pityAfter dropless completions, the next one is guaranteed loot.
func TestLootPity(t *testing.T) {
	svc := newTestService(t)
	svc.store.SetRollForTest(func() float64 { return 0.5 }) // never crit (0.5>0.15? no wait)

	// 0.5 > 0.15 -> no crit; 0.5 > 0.25 -> no natural drop. Rarity on the pity
	// drop rolls 0.5 -> common.
	var drops int
	for i := 1; i <= 8; i++ {
		q, _ := svc.CreateQuest(models.QuestInput{Title: "grind", AttributeRewards: map[string]int64{"discipline": 10}})
		r, err := svc.CompleteQuest(q.ID)
		if err != nil {
			t.Fatalf("complete %d: %v", i, err)
		}
		if r.Drop != nil {
			drops++
			if i <= 6 {
				t.Errorf("unexpected natural drop on completion %d with a 0.5 roll", i)
			}
		}
	}
	if drops != 1 {
		t.Errorf("drops in 8 pity-grind completions = %d, want exactly 1 (the pity drop on #7)", drops)
	}
}

// Achievements: earned once, idempotent, hidden until earned, titles resolve.
func TestAchievements(t *testing.T) {
	svc := newTestService(t)
	svc.store.SetRollForTest(func() float64 { return 0.5 }) // no crit/drop noise

	// The hall starts with the visible catalog, nothing earned, hidden ones absent.
	hall, err := svc.ListAchievements()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, a := range hall {
		if a.Earned {
			t.Errorf("fresh user already earned %s", a.Key)
		}
		if a.Key == "jackpot" || a.Key == "early_bird" || a.Key == "night_owl" {
			t.Errorf("hidden achievement %s visible before earning", a.Key)
		}
	}

	// First completion -> First Blood (and streak seeds "On Fire": longest=7
	// from the demo seed, so habitual/on_fire fire too — assert first_blood
	// specifically plus that re-evaluation doesn't re-award).
	q := findQuestByTitle(t, svc, "Read 15 pages")
	res, err := svc.CompleteQuest(q.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	got := map[string]bool{}
	for _, a := range res.AchievementsUnlocked {
		got[a.Key] = true
	}
	if !got["first_blood"] {
		t.Errorf("first completion unlocked %v, want first_blood among them", got)
	}

	// Second completion: nothing re-awards.
	q2, _ := svc.CreateQuest(models.QuestInput{Title: "again", AttributeRewards: map[string]int64{"focus": 5}})
	res2, err := svc.CompleteQuest(q2.ID)
	if err != nil {
		t.Fatalf("complete 2: %v", err)
	}
	for _, a := range res2.AchievementsUnlocked {
		if a.Key == "first_blood" {
			t.Error("first_blood re-awarded")
		}
	}

	// The seed's 7-day longest streak earns "Habitual" -> title "the Consistent".
	dash, _ := svc.GetDashboard()
	if dash.Character.Title == "" {
		t.Error("expected a character title after habitual unlocked")
	}

	// Hall now shows earned entries with timestamps.
	hall, _ = svc.ListAchievements()
	earned := 0
	for _, a := range hall {
		if a.Earned {
			earned++
			if a.AwardedAt == nil {
				t.Errorf("%s earned without a timestamp", a.Key)
			}
		}
	}
	if earned < 2 {
		t.Errorf("earned = %d, want at least first_blood + streak badges", earned)
	}
}
