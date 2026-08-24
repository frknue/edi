package services

import (
	"strings"
	"sync"
	"testing"
	"time"

	"edi/internal/models"
)

func archiveSeedDailies(t *testing.T, svc *Service) {
	t.Helper()
	if _, err := svc.store.DB().Exec(
		`UPDATE quests SET status = 'archived' WHERE user_id = $1 AND type = 'daily'`,
		svc.userID); err != nil {
		t.Fatalf("archive seed dailies: %v", err)
	}
}

func TestMissedDailyPenaltyMath(t *testing.T) {
	for _, tc := range []struct {
		reward int64
		want   int64
	}{{0, 0}, {-10, 0}, {5, 5}, {10, 5}, {40, 10}, {100, 25}} {
		if got := MissedDailyPenalty(tc.reward); got != tc.want {
			t.Errorf("MissedDailyPenalty(%d) = %d, want %d", tc.reward, got, tc.want)
		}
	}
}

func TestMissedDailyQuestPenaltyIsAuditableAndIdempotent(t *testing.T) {
	svc := newTestService(t)
	now := time.Now().UTC()
	if removed, err := svc.store.RollOverRecurringQuests(svc.userID, now, nil); err != nil || removed != 0 {
		t.Fatalf("prime daily cursor = %d, %v", removed, err)
	}
	archiveSeedDailies(t, svc)
	q, err := svc.CreateQuest(models.QuestInput{
		Title:            "Deliberate practice",
		Type:             models.QuestTypeDaily,
		Difficulty:       "medium",
		AttributeRewards: map[string]int64{"strength": 40, "discipline": 10},
	})
	if err != nil {
		t.Fatalf("create daily: %v", err)
	}

	future := now.AddDate(0, 0, 1)
	removed, err := svc.store.RollOverRecurringQuests(svc.userID, future, nil)
	if err != nil {
		t.Fatalf("assess missed daily: %v", err)
	}
	if removed != 15 {
		t.Fatalf("removed = %d, want 15", removed)
	}
	again, err := svc.store.RollOverRecurringQuests(svc.userID, future, nil)
	if err != nil || again != 0 {
		t.Fatalf("second assessment = %d, %v; want idempotent 0", again, err)
	}

	updated, err := svc.store.GetQuest(svc.userID, q.ID)
	if err != nil {
		t.Fatalf("get quest: %v", err)
	}
	if updated.Status != models.StatusActive || updated.SkipCount != 1 {
		t.Errorf("quest after miss = status %q, skips %d; want active/1", updated.Status, updated.SkipCount)
	}

	rows, err := svc.store.DB().Query(
		`SELECT attribute_key, amount, note FROM xp_events
		 WHERE user_id = $1 AND source = 'daily_penalty' AND source_id = $2 ORDER BY attribute_key`,
		svc.userID, q.ID)
	if err != nil {
		t.Fatalf("query penalty events: %v", err)
	}
	defer rows.Close()
	got := map[string]int64{}
	for rows.Next() {
		var key, note string
		var amount int64
		if err := rows.Scan(&key, &amount, &note); err != nil {
			t.Fatalf("scan penalty event: %v", err)
		}
		got[key] = amount
		if !strings.Contains(note, "missed daily") || !strings.Contains(note, q.Title) {
			t.Errorf("penalty note = %q", note)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("penalty rows: %v", err)
	}
	if got["strength"] != -10 || got["discipline"] != -5 || len(got) != 2 {
		t.Errorf("penalty events = %v, want strength:-10 discipline:-5", got)
	}

	var drift int
	if err := svc.store.DB().QueryRow(
		`SELECT COUNT(1) FROM attributes a
		 WHERE a.user_id = $1 AND a.total_xp != (
		   SELECT COALESCE(SUM(e.amount), 0) FROM xp_events e
		   WHERE e.user_id = a.user_id AND e.attribute_key = a.key
		 )`, svc.userID).Scan(&drift); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if drift != 0 {
		t.Errorf("XP audit drift on %d attributes", drift)
	}
}

func TestCompletedDailyQuestAvoidsPenaltyAndRollsOver(t *testing.T) {
	svc := newTestService(t)
	now := time.Now().UTC()
	if _, err := svc.store.RollOverRecurringQuests(svc.userID, now, nil); err != nil {
		t.Fatalf("prime daily cursor: %v", err)
	}
	archiveSeedDailies(t, svc)
	q, err := svc.CreateQuest(models.QuestInput{
		Title:            "Finish me",
		Type:             models.QuestTypeDaily,
		Difficulty:       "easy",
		AttributeRewards: map[string]int64{"focus": 20},
	})
	if err != nil {
		t.Fatalf("create daily: %v", err)
	}
	if _, err := svc.CompleteQuest(q.ID); err != nil {
		t.Fatalf("complete daily: %v", err)
	}
	removed, err := svc.store.RollOverRecurringQuests(svc.userID, now.AddDate(0, 0, 1), nil)
	if err != nil {
		t.Fatalf("next-day rollover: %v", err)
	}
	if removed != 0 {
		t.Errorf("completed daily lost %d XP, want 0", removed)
	}
	updated, err := svc.store.GetQuest(svc.userID, q.ID)
	if err != nil {
		t.Fatalf("get rolled-over quest: %v", err)
	}
	if updated.Status != models.StatusActive || updated.CompletedAt != nil {
		t.Errorf("rolled-over quest = status %q completed_at %v", updated.Status, updated.CompletedAt)
	}
}

func TestRestModeWaivesMissedDailyPenalty(t *testing.T) {
	svc := newTestService(t)
	now := time.Now().UTC()
	if _, err := svc.store.RollOverRecurringQuests(svc.userID, now, nil); err != nil {
		t.Fatalf("prime daily cursor: %v", err)
	}
	archiveSeedDailies(t, svc)
	q, err := svc.CreateQuest(models.QuestInput{
		Title:            "Paused daily",
		Type:             models.QuestTypeDaily,
		Difficulty:       "easy",
		AttributeRewards: map[string]int64{"health": 20},
	})
	if err != nil {
		t.Fatalf("create daily: %v", err)
	}
	restSince := now
	removed, err := svc.store.RollOverRecurringQuests(svc.userID, now.AddDate(0, 0, 1), &restSince)
	if err != nil {
		t.Fatalf("rest rollover: %v", err)
	}
	if removed != 0 {
		t.Errorf("rest day lost %d XP, want 0", removed)
	}
	var penalties int
	if err := svc.store.DB().QueryRow(
		`SELECT COUNT(1) FROM xp_events WHERE user_id = $1 AND source = 'daily_penalty' AND source_id = $2`,
		svc.userID, q.ID).Scan(&penalties); err != nil {
		t.Fatalf("count penalties: %v", err)
	}
	if penalties != 0 {
		t.Errorf("rest day wrote %d penalty events", penalties)
	}
}

func TestMissedDailyPenaltyConcurrentSingleApplication(t *testing.T) {
	svc := newTestService(t)
	now := time.Now().UTC()
	if _, err := svc.store.RollOverRecurringQuests(svc.userID, now, nil); err != nil {
		t.Fatalf("prime daily cursor: %v", err)
	}
	archiveSeedDailies(t, svc)
	q, err := svc.CreateQuest(models.QuestInput{
		Title:            "Race-safe daily",
		Type:             models.QuestTypeDaily,
		Difficulty:       "medium",
		AttributeRewards: map[string]int64{"focus": 40},
	})
	if err != nil {
		t.Fatalf("create daily: %v", err)
	}

	future := now.AddDate(0, 0, 1)
	var wg sync.WaitGroup
	results := make(chan int64, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			removed, err := svc.store.RollOverRecurringQuests(svc.userID, future, nil)
			results <- removed
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent rollover: %v", err)
		}
	}
	var total int64
	for removed := range results {
		total += removed
	}
	if total != 10 {
		t.Errorf("concurrent removed total = %d, want 10", total)
	}
	var events int
	if err := svc.store.DB().QueryRow(
		`SELECT COUNT(1) FROM xp_events WHERE user_id = $1 AND source = 'daily_penalty' AND source_id = $2`,
		svc.userID, q.ID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 1 {
		t.Errorf("penalty events = %d, want 1", events)
	}
}

func TestDailyPenaltyFirstRunDoesNotChargeExistingQuests(t *testing.T) {
	svc := newTestService(t)
	removed, err := svc.store.RollOverRecurringQuests(svc.userID, time.Now().AddDate(0, 0, 30), nil)
	if err != nil {
		t.Fatalf("first rollover: %v", err)
	}
	if removed != 0 {
		t.Errorf("first rollover retroactively removed %d XP", removed)
	}
}
