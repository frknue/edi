package services

import (
	"testing"

	"edi/internal/models"
)

func createQuestWithSubtasks(t *testing.T, svc *Service) models.Quest {
	t.Helper()
	q, err := svc.CreateQuest(models.QuestInput{
		Title:            "Go to the gym",
		Type:             "daily",
		Difficulty:       "medium",
		AttributeRewards: map[string]int64{"strength": 40},
		Subtasks: []models.SubtaskInput{
			{Title: "Bike there instead of driving", AttributeRewards: map[string]int64{"health": 15, "discipline": 5}},
			{Title: "10 min extra stretching", AttributeRewards: map[string]int64{"health": 10, "strength": 5}},
		},
	})
	if err != nil {
		t.Fatalf("create quest: %v", err)
	}
	if len(q.Subtasks) != 2 {
		t.Fatalf("subtasks = %d, want 2", len(q.Subtasks))
	}
	return q
}

func TestSubtaskBonusOnlyWhenChecked(t *testing.T) {
	svc := newTestService(t)
	q := createQuestWithSubtasks(t, svc)

	before, _ := svc.ListAttributes()
	strBefore := attrByKey(before, "strength").TotalXP
	healthBefore := attrByKey(before, "health").TotalXP

	// Check ONLY the bike subtask.
	st, err := svc.ToggleSubtask(q.ID, q.Subtasks[0].ID)
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if !st.Done {
		t.Fatal("subtask should be done after toggle")
	}

	res, err := svc.CompleteQuest(q.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Events: base strength+40, bonus health+15, bonus discipline+5 (NOT the
	// unchecked stretching bonus).
	sum := map[string]int64{}
	for _, e := range res.XPEvents {
		sum[e.AttributeKey] += e.Amount
	}
	if sum["strength"] != 40 || sum["health"] != 15 || sum["discipline"] != 5 {
		t.Errorf("award sums = %v, want strength:40 health:15 discipline:5", sum)
	}

	after := res.Dashboard.Attributes
	if got := attrByKey(after, "strength").TotalXP; got != strBefore+40 {
		t.Errorf("strength = %d, want %d", got, strBefore+40)
	}
	if got := attrByKey(after, "health").TotalXP; got != healthBefore+15 {
		t.Errorf("health = %d, want %d", got, healthBefore+15)
	}

	// Bonus event is labeled with the subtask.
	foundBonus := false
	for _, e := range res.XPEvents {
		if e.AttributeKey == "health" && e.Note == "Go to the gym · Bike there instead of driving" {
			foundBonus = true
		}
	}
	if !foundBonus {
		t.Error("expected a bonus xp_event labeled with the subtask title")
	}
}

func TestSubtaskCumulativeLevelUp(t *testing.T) {
	svc := newTestService(t)
	// Spirituality seeds at 60 (level 1). Base 25 + bonus 25 crosses 100 -> level 2
	// only when counted cumulatively (neither alone crosses).
	q, err := svc.CreateQuest(models.QuestInput{
		Title:            "Meditation session",
		Type:             "daily",
		Difficulty:       "easy",
		AttributeRewards: map[string]int64{"spirituality": 25},
		Subtasks: []models.SubtaskInput{
			{Title: "Do it outdoors", AttributeRewards: map[string]int64{"spirituality": 25}},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ToggleSubtask(q.ID, q.Subtasks[0].ID); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	res, err := svc.CompleteQuest(q.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	var found *models.LevelUp
	for i, lu := range res.LevelUps {
		if lu.AttributeKey == "spirituality" {
			found = &res.LevelUps[i]
		}
	}
	if found == nil {
		t.Fatal("expected a spirituality level-up from cumulative base+bonus")
	}
	if found.FromLevel != 1 || found.ToLevel != 2 {
		t.Errorf("level-up = %d->%d, want 1->2", found.FromLevel, found.ToLevel)
	}
	// Exactly one level-up entry for the attribute (not one per event).
	count := 0
	for _, lu := range res.LevelUps {
		if lu.AttributeKey == "spirituality" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("spirituality level-ups = %d, want 1", count)
	}
}

func TestToggleSubtaskAfterCompletionRejected(t *testing.T) {
	svc := newTestService(t)
	q := createQuestWithSubtasks(t, svc)
	if _, err := svc.CompleteQuest(q.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := svc.ToggleSubtask(q.ID, q.Subtasks[0].ID); err == nil {
		t.Error("expected error toggling a subtask on a completed quest")
	}
	// Unknown ids -> not found.
	if _, err := svc.ToggleSubtask(999999, 1); err == nil {
		t.Error("expected error for unknown quest")
	}
}

func TestSubtaskValidation(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.CreateQuest(models.QuestInput{
		Title: "Bad subtask", Type: "daily",
		Subtasks: []models.SubtaskInput{{Title: "  ", AttributeRewards: map[string]int64{"health": 5}}},
	})
	if err == nil {
		t.Error("expected validation error for empty subtask title")
	}
	_, err = svc.CreateQuest(models.QuestInput{
		Title: "Bad reward", Type: "daily",
		Subtasks: []models.SubtaskInput{{Title: "x", AttributeRewards: map[string]int64{"nope": 5}}},
	})
	if err == nil {
		t.Error("expected validation error for unknown attribute in subtask rewards")
	}
}

func TestPatchReplacesSubtasks(t *testing.T) {
	svc := newTestService(t)
	q := createQuestWithSubtasks(t, svc)
	newSubs := []models.SubtaskInput{{Title: "New objective", AttributeRewards: map[string]int64{"focus": 10}}}
	updated, err := svc.UpdateQuest(q.ID, models.QuestPatch{Subtasks: &newSubs})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(updated.Subtasks) != 1 || updated.Subtasks[0].Title != "New objective" {
		t.Errorf("subtasks after patch = %+v, want the single replacement", updated.Subtasks)
	}
}
