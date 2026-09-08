package services

import (
	"errors"
	"testing"
	"time"

	"edi/internal/models"
)

// Triggers: validated HH:MM, persisted on create/update, due once per quest
// per local day, capped per day.
func TestTriggersDueOncePerDay(t *testing.T) {
	svc := newTestService(t)
	now := time.Now()
	hhmm := now.Local().Format("15:04")

	if _, err := svc.CreateQuest(models.QuestInput{Title: "bad", TriggerAt: "25:99"}); !errors.Is(err, ErrValidation) {
		t.Errorf("bad trigger_at = %v, want ErrValidation", err)
	}
	q, err := svc.CreateQuest(models.QuestInput{Title: "Tax letter", Trigger: "after coffee", TriggerAt: hhmm, AttributeRewards: map[string]int64{"wealth": 20}})
	if err != nil || q.Trigger != "after coffee" || q.TriggerAt != hhmm {
		t.Fatalf("create with trigger = %+v, %v", q, err)
	}
	due, err := svc.DueTriggers(now)
	if err != nil || len(due) != 1 || due[0].ID != q.ID {
		t.Fatalf("due = %+v, %v; want the quest", due, err)
	}
	if err := svc.MarkTriggerFired(q.ID, now); err != nil {
		t.Fatal(err)
	}
	if due, _ := svc.DueTriggers(now); len(due) != 0 {
		t.Errorf("fired twice in one day: %+v", due)
	}
	// Clearing the anchor via patch removes it from the schedule.
	empty := ""
	if _, err := svc.UpdateQuest(q.ID, models.QuestPatch{TriggerAt: &empty}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.store.GetQuest(1, q.ID); got.TriggerAt != "" || got.Trigger != "after coffee" {
		t.Errorf("after clearing anchor: %+v", got)
	}
	// Per-day cap: the 6th due trigger waits.
	for i := 0; i < 6; i++ {
		if _, err := svc.CreateQuest(models.QuestInput{Title: "t", TriggerAt: hhmm}); err != nil {
			t.Fatal(err)
		}
	}
	due, _ = svc.DueTriggers(now)
	if len(due) != maxTriggerFiresPerDay-1 {
		t.Errorf("due after one fire = %d, want %d (cap %d)", len(due), maxTriggerFiresPerDay-1, maxTriggerFiresPerDay)
	}
}

// Shrink swaps the quest atomically: the original is archived, the smaller
// one is active, the trigger is inherited, and skip counts start at zero.
// The AI path itself is gated on the ChatGPT connection.
func TestReplaceQuestAtomicAndShrinkGated(t *testing.T) {
	svc := newTestService(t)
	big, err := svc.CreateQuest(models.QuestInput{Title: "30 minute workout", Type: "daily", Trigger: "after coffee", TriggerAt: "07:30", AttributeRewards: map[string]int64{"strength": 40}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SkipQuest(big.ID); err != nil {
		t.Fatal(err)
	}
	small, err := svc.replaceQuest(big.ID, models.QuestInput{Title: "5 push-ups", Type: "daily", Difficulty: "trivial", AttributeRewards: map[string]int64{"strength": 10}})
	if err != nil || small.Status != "active" || small.Trigger != "after coffee" || small.TriggerAt != "07:30" || small.SkipCount != 0 {
		t.Fatalf("replace = %+v, %v", small, err)
	}
	if old, _ := svc.store.GetQuest(1, big.ID); old.Status != "archived" {
		t.Errorf("original status = %s, want archived", old.Status)
	}
	// A second replace of the archived original is a client error, not a duplicate.
	if _, err := svc.replaceQuest(big.ID, models.QuestInput{Title: "again"}); !errors.Is(err, ErrValidation) {
		t.Errorf("double replace = %v, want ErrValidation", err)
	}
	if _, err := svc.ShrinkQuest(small.ID); !errors.Is(err, ErrOpenAINotConnected) {
		t.Errorf("shrink without AI = %v, want ErrOpenAINotConnected", err)
	}
	if _, err := svc.BreakDownQuest(small.ID); !errors.Is(err, ErrOpenAINotConnected) {
		t.Errorf("breakdown without AI = %v, want ErrOpenAINotConnected", err)
	}
	retired, err := svc.RetireQuest(small.ID)
	if err != nil || retired.Status != "archived" || retired.SkipCount != 0 {
		t.Errorf("retire = %+v, %v", retired, err)
	}
	if auditDrift(t, svc) != 0 {
		t.Error("audit invariant violated")
	}
}

// Story chapters are remembered in order; the dashboard carries the latest.
func TestStoryChaptersRemembered(t *testing.T) {
	svc := newTestService(t)
	for _, text := range []string{"The hero wakes.", "The hero walks."} {
		if _, err := svc.store.InsertStoryChapter(1, text, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	chapters, err := svc.ListStoryChapters(10)
	if err != nil || len(chapters) != 2 || chapters[0].Number != 2 || chapters[1].Number != 1 {
		t.Fatalf("chapters = %+v, %v", chapters, err)
	}
	dash, _ := svc.GetDashboard()
	if dash.LatestChapter == nil || dash.LatestChapter.Text != "The hero walks." {
		t.Errorf("latest chapter = %+v", dash.LatestChapter)
	}
	if _, err := svc.StoryNarration(); !errors.Is(err, ErrOpenAINotConnected) {
		t.Errorf("narration without AI = %v", err)
	}
}
