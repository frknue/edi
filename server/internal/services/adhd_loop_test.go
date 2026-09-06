package services

import (
	"errors"
	"testing"
	"time"

	"edi/internal/models"
)

// The punishment layer is OFF by default: no decay, no daily stakes, no
// wards, no decay status on attributes — the miss counter still counts.
func TestHardcoreOffNeverRemovesXP(t *testing.T) {
	svc := newTestService(t)
	if st, _ := svc.HardcoreState(); st.On {
		t.Fatal("hardcore must default to off")
	}
	backdateAttribute(t, svc, "strength", 10)
	before := attrByKey(mustAttrs(t, svc), "strength").TotalXP

	removed, err := svc.ApplyDecay()
	if err != nil || removed != 0 {
		t.Fatalf("ApplyDecay outside hardcore = %d, %v; want 0", removed, err)
	}
	attrs := mustAttrs(t, svc)
	if got := attrByKey(attrs, "strength"); got.TotalXP != before || got.Decay != nil {
		t.Errorf("strength = %d XP, decay %+v; want %d and nil decay", got.TotalXP, got.Decay, before)
	}

	// A missed daily: the cursor advances and the miss is counted, no XP moves.
	now := time.Now().UTC()
	if _, err := svc.store.RollOverRecurringQuests(svc.userID, now, nil, false); err != nil {
		t.Fatalf("prime cursor: %v", err)
	}
	archiveSeedDailies(t, svc)
	q, err := svc.CreateQuest(models.QuestInput{Title: "Stretch", Type: "daily", AttributeRewards: map[string]int64{"health": 40}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	removed, err = svc.store.RollOverRecurringQuests(svc.userID, now.AddDate(0, 0, 1), nil, false)
	if err != nil || removed != 0 {
		t.Fatalf("missed daily outside hardcore removed %d XP (%v), want 0", removed, err)
	}
	if got, _ := svc.store.GetQuest(svc.userID, q.ID); got.SkipCount != 1 {
		t.Errorf("skip_count = %d, want 1 (silent avoidance signal keeps counting)", got.SkipCount)
	}
	var penalties int
	svc.store.DB().QueryRow(`SELECT COUNT(1) FROM xp_events WHERE user_id = 1 AND amount < 0`).Scan(&penalties)
	if penalties != 0 {
		t.Errorf("%d negative xp_events written outside hardcore", penalties)
	}

	if _, err := svc.WardAttribute("strength"); !errors.Is(err, ErrValidation) {
		t.Errorf("ward outside hardcore = %v, want ErrValidation", err)
	}
	dash, err := svc.GetDashboard()
	if err != nil || dash.Hardcore || dash.DecayedToday != 0 || dash.DailyPenaltyXP != 0 {
		t.Errorf("dashboard = hardcore %v, decayed %d, penalty %d (%v)", dash.Hardcore, dash.DecayedToday, dash.DailyPenaltyXP, err)
	}
	if auditDrift(t, svc) != 0 {
		t.Error("audit invariant violated")
	}
}

// Switching hardcore ON after a long idle stretch must never bill the past:
// the idle clocks anchor at the switch, and the stakes cursor is settled first.
func TestHardcoreOnAfterIdleDoesNotRetroBill(t *testing.T) {
	svc := newTestService(t)
	backdateAttribute(t, svc, "strength", 30)
	archiveSeedDailies(t, svc)
	q, err := svc.CreateQuest(models.QuestInput{Title: "Old daily", Type: "daily", AttributeRewards: map[string]int64{"health": 40}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.store.DB().Exec(`UPDATE quests SET created_at = $1 WHERE id = $2`, time.Now().AddDate(0, 0, -10), q.ID); err != nil {
		t.Fatalf("backdate quest: %v", err)
	}
	before := attrByKey(mustAttrs(t, svc), "strength").TotalXP

	st, err := svc.SetHardcoreMode(true)
	if err != nil || !st.On || st.Since == nil {
		t.Fatalf("hardcore on = %+v, %v", st, err)
	}
	removed, err := svc.ApplyDecay()
	if err != nil || removed != 0 {
		t.Fatalf("decay right after switching on = %d, %v; want 0", removed, err)
	}
	attrs := mustAttrs(t, svc)
	got := attrByKey(attrs, "strength")
	if got.TotalXP != before {
		t.Errorf("strength = %d, want untouched %d", got.TotalXP, before)
	}
	if got.Decay == nil || got.Decay.State != "fresh" || got.Decay.IdleDays != 0 {
		t.Errorf("decay status after switch = %+v, want fresh/0 idle (anchored at the switch)", got.Decay)
	}
	if dash, _ := svc.GetDashboard(); !dash.Hardcore || dash.DailyPenaltyXP != 0 {
		t.Errorf("dashboard hardcore=%v penalty=%d", dash.Hardcore, dash.DailyPenaltyXP)
	}
	// Redundant call is a no-op that keeps the anchor.
	again, _ := svc.SetHardcoreMode(true)
	if again.Since == nil || !again.Since.Equal(*st.Since) {
		t.Errorf("redundant switch moved the anchor: %v -> %v", st.Since, again.Since)
	}
	// Off again: decay status disappears.
	if off, err := svc.SetHardcoreMode(false); err != nil || off.On {
		t.Fatalf("hardcore off = %+v, %v", off, err)
	}
	if a := attrByKey(mustAttrs(t, svc), "strength"); a.Decay != nil {
		t.Errorf("decay status still present after switching off: %+v", a.Decay)
	}
}

// A one-day gap is bridged for free (streak continues), at most once per
// 7 days; longer gaps and a second gap inside the cooldown still reset.
func TestStreakAutoMend(t *testing.T) {
	svc := newTestService(t)
	day := func(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }
	setStreak := func(current int, lastActive, lastMend string) {
		t.Helper()
		var mend any
		if lastMend != "" {
			mend = lastMend
		}
		if _, err := svc.store.DB().Exec(
			`UPDATE streaks SET current_count = $1, longest_count = $1, last_active_date = $2, last_mend_date = $3 WHERE user_id = 1`,
			current, lastActive, mend); err != nil {
			t.Fatalf("set streak: %v", err)
		}
	}
	quests, _ := svc.ListQuests("", "active")
	next := 0
	complete := func() models.Streak {
		t.Helper()
		if _, err := svc.CompleteQuest(quests[next].ID); err != nil {
			t.Fatalf("complete: %v", err)
		}
		next++
		s, _ := svc.store.GetStreak(1)
		return s
	}

	// Last active the day before yesterday, never mended: bridged.
	setStreak(5, day(-2), "")
	if s := complete(); s.Current != 6 || s.LastMendDate == nil || *s.LastMendDate != day(0) {
		t.Fatalf("mend: streak = %+v, want 6 with last_mend_date today", s)
	}
	// Same gap again, but a mend was used today: reset.
	setStreak(6, day(-2), day(0))
	if s := complete(); s.Current != 1 {
		t.Errorf("gap inside cooldown: streak = %d, want 1", s.Current)
	}
	// Mend older than the cooldown: available again.
	setStreak(3, day(-2), day(-8))
	if s := complete(); s.Current != 4 {
		t.Errorf("gap after cooldown: streak = %d, want 4", s.Current)
	}
	// A two-day gap is never bridged.
	setStreak(9, day(-3), "")
	if s := complete(); s.Current != 1 {
		t.Errorf("two-day gap: streak = %d, want 1", s.Current)
	}
	// Consecutive days still just increment (no mend consumed).
	setStreak(2, day(-1), "")
	if s := complete(); s.Current != 3 || s.LastMendDate != nil {
		t.Errorf("consecutive: streak = %+v, want 3 without a mend", s)
	}
}

// Buff drops last 24h or 3 uses: after three completions the buff is spent
// and no longer listed or paid.
func TestBuffSpentAfterThreeUses(t *testing.T) {
	svc := newTestService(t)
	mk := func(title string) models.Quest {
		q, err := svc.CreateQuest(models.QuestInput{Title: title, AttributeRewards: map[string]int64{"focus": 40}})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return q
	}
	q0 := mk("dropper")
	svc.store.SetRollForTest(seqRoll(0.99, 0.0, 0.96, 0.0)) // epic prism, +25% ALL
	r0, err := svc.CompleteQuest(q0.ID)
	if err != nil || r0.Drop == nil || r0.Drop.Key != "prism_of_momentum" {
		t.Fatalf("drop = %+v, %v", r0.Drop, err)
	}
	if exp := r0.Drop.ExpiresAt; exp == nil || exp.Sub(time.Now()) < 23*time.Hour {
		t.Errorf("buff expiry %v, want ~24h out (not midnight)", exp)
	}
	if b := r0.Dashboard.ActiveBuffs; len(b) != 1 || b[0].UsesLeft == nil || *b[0].UsesLeft != 3 {
		t.Fatalf("active buffs = %+v, want one with 3 uses", b)
	}
	svc.store.SetRollForTest(func() float64 { return 0.99 })
	buffXP := func(r models.CompletionResult) int64 {
		var n int64
		for _, e := range r.XPEvents {
			if e.Source == "buff" {
				n += e.Amount
			}
		}
		return n
	}
	for i := 1; i <= 3; i++ {
		r, err := svc.CompleteQuest(mk("use").ID)
		if err != nil {
			t.Fatalf("use %d: %v", i, err)
		}
		if buffXP(r) != 10 {
			t.Errorf("use %d paid %d buff XP, want 10", i, buffXP(r))
		}
		if i < 3 {
			if b := r.Dashboard.ActiveBuffs; len(b) != 1 || *b[0].UsesLeft != 3-i {
				t.Errorf("after use %d buffs = %+v", i, b)
			}
		} else if len(r.Dashboard.ActiveBuffs) != 0 {
			t.Errorf("buff still active after 3 uses: %+v", r.Dashboard.ActiveBuffs)
		}
	}
	r, _ := svc.CompleteQuest(mk("after").ID)
	if buffXP(r) != 0 {
		t.Errorf("spent buff still paid %d", buffXP(r))
	}
	if auditDrift(t, svc) != 0 {
		t.Error("audit invariant violated")
	}
}

// The daily goal is the closable set of today's dailies (active or already
// done today), never a fixed 5, never below 1.
func TestDailyGoalFollowsTheBoard(t *testing.T) {
	svc := newTestService(t)
	quests, _ := svc.ListQuests("daily", "active")
	dash, _ := svc.GetDashboard()
	if dash.DailyProgress.Goal != len(quests) || len(quests) < 2 {
		t.Fatalf("goal = %d, want the %d active dailies", dash.DailyProgress.Goal, len(quests))
	}
	if _, err := svc.CompleteQuest(quests[0].ID); err != nil {
		t.Fatal(err)
	}
	dash, _ = svc.GetDashboard()
	if dash.DailyProgress.Goal != len(quests) || dash.DailyProgress.CompletedToday != 1 {
		t.Errorf("after one completion: %d/%d, want 1/%d (done dailies stay in the set)", dash.DailyProgress.CompletedToday, dash.DailyProgress.Goal, len(quests))
	}
	archiveSeedDailies(t, svc)
	dash, _ = svc.GetDashboard()
	if dash.DailyProgress.Goal != 1 {
		t.Errorf("goal with no dailies = %d, want 1", dash.DailyProgress.Goal)
	}
	if n := len(dash.ActiveDays); n != activeDayStripLen || !dash.ActiveDays[n-1].Today || !dash.ActiveDays[n-1].Active {
		t.Errorf("active days = %+v, want %d entries ending in an active today", dash.ActiveDays, activeDayStripLen)
	}
}

func mustAttrs(t *testing.T, svc *Service) []models.Attribute {
	t.Helper()
	attrs, err := svc.ListAttributes()
	if err != nil {
		t.Fatalf("attributes: %v", err)
	}
	return attrs
}
