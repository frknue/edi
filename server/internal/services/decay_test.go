package services

import (
	"errors"
	"sync"
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

func TestRestMode(t *testing.T) {
	svc := newTestService(t)

	st, err := svc.RestState()
	if err != nil || st.On {
		t.Fatalf("initial rest state = %+v, %v; want off", st, err)
	}

	on, err := svc.SetRestMode(true)
	if err != nil || !on.On || on.Since == nil {
		t.Fatalf("rest on = %+v, %v; want on with since", on, err)
	}

	off, err := svc.SetRestMode(false)
	if err != nil || off.On {
		t.Fatalf("rest off = %+v, %v", off, err)
	}
	ended, err := svc.restEndedAt()
	if err != nil || ended == nil {
		t.Fatalf("rest_ended_at = %v, %v; want a timestamp", ended, err)
	}
	if time.Since(*ended) > time.Minute {
		t.Errorf("rest_ended_at %v not recent", *ended)
	}
}

// backdateAttribute makes an attribute look idle by shifting all its positive
// xp_events into the past. Test-only trick; the engine only reads times.
func backdateAttribute(t *testing.T, svc *Service, key string, daysAgo int) {
	t.Helper()
	ts := time.Now().UTC().AddDate(0, 0, -daysAgo).Format("2006-01-02T15:04:05.000000000Z07:00")
	if _, err := svc.store.DB().Exec(
		`UPDATE xp_events SET created_at = ? WHERE user_id = 1 AND attribute_key = ? AND amount > 0`, ts, key); err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

func TestDecayCatchUp(t *testing.T) {
	svc := newTestService(t)
	backdateAttribute(t, svc, "strength", 10) // 520 XP, 10 idle days

	removed, err := svc.ApplyDecay()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Idle days 4..10 = 7 billable days at max(5, 520/100)=5 each = 35.
	if removed != 35 {
		t.Errorf("removed = %d, want 35", removed)
	}
	attrs, _ := svc.ListAttributes()
	if got := attrByKey(attrs, "strength").TotalXP; got != 485 {
		t.Errorf("strength total = %d, want 485", got)
	}
	// Idempotent: a second catch-up the same day removes nothing.
	again, err := svc.ApplyDecay()
	if err != nil || again != 0 {
		t.Errorf("second apply removed %d (err %v), want 0", again, err)
	}
	// Audit invariant intact: total == SUM(events).
	var sum int64
	svc.store.DB().QueryRow(`SELECT COALESCE(SUM(amount),0) FROM xp_events WHERE user_id=1 AND attribute_key='strength'`).Scan(&sum)
	if sum != 485 {
		t.Errorf("event sum = %d, want 485", sum)
	}
	// Peak untouched by decay.
	if got := attrByKey(attrs, "strength").PeakXP; got != 520 {
		t.Errorf("peak = %d, want 520", got)
	}
}

func TestDecayGraceAndFloor(t *testing.T) {
	svc := newTestService(t)

	// 3 idle days = still in grace: nothing happens.
	backdateAttribute(t, svc, "focus", 3)
	if removed, _ := svc.ApplyDecay(); removed != 0 {
		t.Errorf("grace period removed %d, want 0", removed)
	}

	// Floor: spirituality has 60 XP (peak level 1 -> floor 0), min-5 bleed,
	// long idleness must stop at 0, never negative.
	backdateAttribute(t, svc, "spirituality", 60)
	if _, err := svc.ApplyDecay(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	attrs, _ := svc.ListAttributes()
	if got := attrByKey(attrs, "spirituality").TotalXP; got < 0 {
		t.Errorf("spirituality total = %d, went below 0", got)
	}
}

func TestDecaySkipsRest(t *testing.T) {
	svc := newTestService(t)

	// Rest mode on: no decay at all.
	backdateAttribute(t, svc, "wealth", 10)
	if _, err := svc.SetRestMode(true); err != nil {
		t.Fatalf("rest on: %v", err)
	}
	if removed, _ := svc.ApplyDecay(); removed != 0 {
		t.Errorf("decay ran during rest: removed %d", removed)
	}
	// Rest off: EVERY idle clock restarts — still no decay right after.
	if _, err := svc.SetRestMode(false); err != nil {
		t.Fatalf("rest off: %v", err)
	}
	if removed, _ := svc.ApplyDecay(); removed != 0 {
		t.Errorf("decay after rest-off removed %d, want 0 (clock restarted)", removed)
	}
}

// Separate service from the rest test: turning rest off resets ALL idle
// clocks, which would zero out the ward scenario below.
func TestDecayWardExcludesCoveredDays(t *testing.T) {
	svc := newTestService(t)

	// A fresh ward covers today, so today's bill is excluded, but the
	// uncovered past days still bill. learning: 300 XP, idle 6 days -> days
	// 4,5,6 billable; day 6 (today) covered by ward -> 2 days * 5 = 10.
	if _, err := svc.WardAttribute("learning"); err != nil {
		t.Fatalf("ward: %v", err)
	}
	backdateAttribute(t, svc, "learning", 6)
	removed, err := svc.ApplyDecay()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if removed != 10 {
		t.Errorf("warded decay removed %d, want 10 (today excluded by ward)", removed)
	}
}

func TestDecayConcurrentSingleApplication(t *testing.T) {
	svc := newTestService(t)
	backdateAttribute(t, svc, "creativity", 8) // 170 XP: days 4..8 = 5 days * 5 = 25

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.ApplyDecay()
		}()
	}
	wg.Wait()

	var n int
	svc.store.DB().QueryRow(`SELECT COUNT(1) FROM xp_events WHERE user_id=1 AND attribute_key='creativity' AND source='decay'`).Scan(&n)
	if n != 5 {
		t.Errorf("decay events = %d, want exactly 5 (no double billing)", n)
	}
}
