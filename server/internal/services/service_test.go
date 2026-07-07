package services

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"edi/internal/db"
	"edi/internal/models"
)

// newTestService spins up a fresh, seeded SQLite DB in a temp dir.
func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return New(store, 1)
}

func findQuestByTitle(t *testing.T, svc *Service, title string) models.Quest {
	t.Helper()
	quests, err := svc.ListQuests("", "")
	if err != nil {
		t.Fatalf("list quests: %v", err)
	}
	for _, q := range quests {
		if q.Title == title {
			return q
		}
	}
	t.Fatalf("quest %q not found", title)
	return models.Quest{}
}

func attrByKey(attrs []models.Attribute, key string) models.Attribute {
	for _, a := range attrs {
		if a.Key == key {
			return a
		}
	}
	return models.Attribute{}
}

func TestCompleteQuestAwardsXPAndCreatesEvents(t *testing.T) {
	svc := newTestService(t)

	before, err := svc.ListAttributes()
	if err != nil {
		t.Fatalf("list attributes: %v", err)
	}
	strBefore := attrByKey(before, "strength").TotalXP   // seed: 520
	disBefore := attrByKey(before, "discipline").TotalXP // seed: 380

	workout := findQuestByTitle(t, svc, "30 minute workout") // {strength:40, discipline:10}

	result, err := svc.CompleteQuest(workout.ID)
	if err != nil {
		t.Fatalf("complete quest: %v", err)
	}

	// 1. Quest marked completed.
	if result.Quest.Status != models.StatusCompleted {
		t.Errorf("status = %q, want completed", result.Quest.Status)
	}
	if result.Quest.CompletedAt == nil {
		t.Error("completed_at not set")
	}

	// 2. One XP event per rewarded attribute.
	if len(result.XPEvents) != 2 {
		t.Fatalf("got %d xp events, want 2", len(result.XPEvents))
	}
	gotAmounts := map[string]int64{}
	for _, e := range result.XPEvents {
		gotAmounts[e.AttributeKey] = e.Amount
		if e.Source != "quest" {
			t.Errorf("event source = %q, want quest", e.Source)
		}
	}
	if gotAmounts["strength"] != 40 || gotAmounts["discipline"] != 10 {
		t.Errorf("event amounts = %v, want strength:40 discipline:10", gotAmounts)
	}

	// 3. Attribute totals updated.
	after := result.Dashboard.Attributes
	if got := attrByKey(after, "strength").TotalXP; got != strBefore+40 {
		t.Errorf("strength total = %d, want %d", got, strBefore+40)
	}
	if got := attrByKey(after, "discipline").TotalXP; got != disBefore+10 {
		t.Errorf("discipline total = %d, want %d", got, disBefore+10)
	}

	// 4. Audit trail: the events are persisted and retrievable.
	events, err := svc.ListXPEvents(100)
	if err != nil {
		t.Fatalf("list xp events: %v", err)
	}
	questEvents := 0
	for _, e := range events {
		if e.Source == "quest" && e.SourceID != nil && *e.SourceID == workout.ID {
			questEvents++
		}
	}
	if questEvents != 2 {
		t.Errorf("persisted quest xp events = %d, want 2", questEvents)
	}

	// 5. Completing twice is rejected.
	if _, err := svc.CompleteQuest(workout.ID); err == nil {
		t.Error("expected error completing an already-completed quest")
	}
}

// Regression for the critical concurrency bug: N goroutines completing the SAME
// quest must result in exactly one completion and a single XP award (no double XP).
func TestCompleteQuestConcurrentNoDoubleAward(t *testing.T) {
	svc := newTestService(t)
	workout := findQuestByTitle(t, svc, "30 minute workout") // strength:40, discipline:10

	before, _ := svc.ListAttributes()
	strBefore := attrByKey(before, "strength").TotalXP

	const N = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if _, err := svc.CompleteQuest(workout.ID); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("got %d successful completions under concurrency, want exactly 1", successes)
	}
	after, _ := svc.ListAttributes()
	if got := attrByKey(after, "strength").TotalXP; got != strBefore+40 {
		t.Errorf("strength = %d, want %d (XP must be awarded exactly once)", got, strBefore+40)
	}

	// Exactly two xp_events (strength + discipline) and one completion for this quest.
	events, _ := svc.ListXPEvents(1000)
	questEvents := 0
	for _, e := range events {
		if e.Source == "quest" && e.SourceID != nil && *e.SourceID == workout.ID {
			questEvents++
		}
	}
	if questEvents != 2 {
		t.Errorf("xp_events for quest = %d, want 2 (single completion)", questEvents)
	}
}

func TestCompleteQuestLevelUp(t *testing.T) {
	svc := newTestService(t)

	// Create a quest that pushes Spirituality (seed 60, level 1) past 100 -> level 2.
	q, err := svc.CreateQuest(models.QuestInput{
		Title:            "Deep meditation",
		Type:             "daily",
		Difficulty:       "medium",
		AttributeRewards: map[string]int64{"spirituality": 60},
	})
	if err != nil {
		t.Fatalf("create quest: %v", err)
	}
	result, err := svc.CompleteQuest(q.ID)
	if err != nil {
		t.Fatalf("complete quest: %v", err)
	}
	if len(result.LevelUps) != 1 {
		t.Fatalf("got %d level-ups, want 1", len(result.LevelUps))
	}
	lu := result.LevelUps[0]
	if lu.AttributeKey != "spirituality" || lu.FromLevel != 1 || lu.ToLevel != 2 {
		t.Errorf("level-up = %+v, want spirituality 1->2", lu)
	}
}

// insertTestSuggestion adds a pending suggestion directly via the store (suggestions
// are normally produced by the LLM, which is unavailable/undesired in unit tests).
func insertTestSuggestion(t *testing.T, svc *Service) models.AgentSuggestion {
	t.Helper()
	sug, err := svc.store.InsertSuggestion(1, models.AgentSuggestion{
		Type:   "low_attribute",
		Title:  "Add a Health quest",
		Reason: "Health is your lowest attribute.",
		SuggestedQuest: models.QuestInput{
			Title: "Drink water & 15-min mobility", Type: "daily", Difficulty: "easy",
			AttributeRewards: map[string]int64{"health": 30},
		},
	})
	if err != nil {
		t.Fatalf("insert suggestion: %v", err)
	}
	return sug
}

func TestAcceptSuggestionCreatesQuest(t *testing.T) {
	svc := newTestService(t)
	target := insertTestSuggestion(t, svc)

	questsBefore, _ := svc.ListQuests("", "")

	quest, err := svc.AcceptSuggestion(target.ID)
	if err != nil {
		t.Fatalf("accept suggestion: %v", err)
	}
	if quest.Title != target.SuggestedQuest.Title {
		t.Errorf("quest title = %q, want %q", quest.Title, target.SuggestedQuest.Title)
	}
	if quest.Status != models.StatusActive {
		t.Errorf("new quest status = %q, want active", quest.Status)
	}

	questsAfter, _ := svc.ListQuests("", "")
	if len(questsAfter) != len(questsBefore)+1 {
		t.Errorf("quest count = %d, want %d", len(questsAfter), len(questsBefore)+1)
	}

	// Suggestion is now accepted and linked to the new quest.
	updated, err := svc.store.GetSuggestion(1, target.ID)
	if err != nil {
		t.Fatalf("get suggestion: %v", err)
	}
	if updated.Status != "accepted" {
		t.Errorf("suggestion status = %q, want accepted", updated.Status)
	}
	if updated.CreatedQuestID == nil || *updated.CreatedQuestID != quest.ID {
		t.Errorf("created_quest_id = %v, want %d", updated.CreatedQuestID, quest.ID)
	}

	// Re-accepting is rejected.
	if _, err := svc.AcceptSuggestion(target.ID); err == nil {
		t.Error("expected error re-accepting a resolved suggestion")
	}
}

func TestDismissSuggestion(t *testing.T) {
	svc := newTestService(t)
	target := insertTestSuggestion(t, svc)
	sug, err := svc.DismissSuggestion(target.ID)
	if err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if sug.Status != "dismissed" {
		t.Errorf("status = %q, want dismissed", sug.Status)
	}
}

// AI features must be gated on a connected OpenAI account — no offline fallback.
func TestGenerateSuggestionsRequiresOpenAI(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.GenerateSuggestions()
	if !errors.Is(err, ErrOpenAINotConnected) {
		t.Fatalf("GenerateSuggestions without OpenAI = %v, want ErrOpenAINotConnected", err)
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("ErrOpenAINotConnected should map to a 400 (wrap ErrValidation), got %v", err)
	}

	status, err := svc.OpenAIStatus()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Connected {
		t.Error("OpenAIStatus.Connected = true on a fresh DB, want false")
	}
}

func TestJournalValidation(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateJournalEntry(models.JournalInput{Mood: 0, Energy: 5}); err == nil {
		t.Error("expected validation error for mood 0")
	}
	entry, err := svc.CreateJournalEntry(models.JournalInput{Mood: 8, Energy: 7, Notes: "good"})
	if err != nil {
		t.Fatalf("create journal: %v", err)
	}
	if entry.ID == 0 {
		t.Error("journal entry id not set")
	}
	list, err := svc.ListJournalEntries(10)
	if err != nil {
		t.Fatalf("list journal: %v", err)
	}
	if len(list) < 2 { // seeded one + this one
		t.Errorf("journal entries = %d, want >= 2", len(list))
	}
}

// Regression: empty list responses must serialize as JSON [] (not null), so all
// clients can safely read .length / iterate.
func TestEmptyListsSerializeAsArrays(t *testing.T) {
	svc := newTestService(t)

	// Resolve every pending suggestion so the list becomes empty.
	pending, _ := svc.ListSuggestions("pending")
	for _, s := range pending {
		if _, err := svc.DismissSuggestion(s.ID); err != nil {
			t.Fatalf("dismiss: %v", err)
		}
	}

	got, err := svc.ListSuggestions("pending")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil {
		t.Fatal("ListSuggestions returned nil slice, want empty slice")
	}

	dash, err := svc.GetDashboard()
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if dash.Suggestions == nil {
		t.Error("dashboard.Suggestions is nil")
	}
	b, err := json.Marshal(dash)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"pending_suggestions":null`) {
		t.Error("pending_suggestions serialized as null, want []")
	}
	if strings.Contains(string(b), `:null`) && strings.Contains(string(b), `"today_quests":null`) {
		t.Error("today_quests serialized as null, want []")
	}
}

func TestGetWeakestAttribute(t *testing.T) {
	svc := newTestService(t)
	weakest, err := svc.GetWeakestAttribute()
	if err != nil {
		t.Fatalf("weakest: %v", err)
	}
	// Seed makes Spirituality the lowest at 60 XP.
	if weakest.Key != "spirituality" {
		t.Errorf("weakest = %q, want spirituality", weakest.Key)
	}
}
