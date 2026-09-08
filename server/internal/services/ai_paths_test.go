package services

import (
	"strings"
	"testing"

	"edi/internal/models"
)

// The JSON-parsing AI paths, driven by a canned model: boss phases become
// subtasks (unknown reward keys pruned, never a 400), break-down replaces
// subtasks, shrink swaps the quest atomically, narration saves a chapter and
// feeds it back into the next prompt.
func TestForgeBossPhasesFromModel(t *testing.T) {
	svc := newTestService(t)
	svc.SetCompleterForTest(func(_, _ string) (string, error) {
		return `{"title":"Slay the Ledger Wyrm","description":"Do the tax return.","attribute_rewards":{"wealth":100,"discipline":50},
		"phases":[{"title":"Open the folder","attribute_rewards":{"wealth":5}},{"title":"Sort receipts","attribute_rewards":{"fitness":10}},
		{"title":"Fill the form","attribute_rewards":{"discipline":10}},{"title":"Submit","attribute_rewards":{"wealth":15}}]}`, nil
	})
	q, err := svc.ForgeBoss()
	if err != nil {
		t.Fatalf("forge: %v", err)
	}
	if q.Type != "boss" || len(q.Subtasks) != 4 {
		t.Fatalf("boss = %+v", q)
	}
	if q.Subtasks[1].AttributeRewards["fitness"] != 0 || len(q.Subtasks[1].AttributeRewards) != 0 {
		t.Errorf("unknown key not pruned: %+v", q.Subtasks[1].AttributeRewards)
	}
	if q.Subtasks[0].AttributeRewards["wealth"] != 5 {
		t.Errorf("phase rewards lost: %+v", q.Subtasks[0])
	}
}

func TestBreakDownQuestFromModel(t *testing.T) {
	svc := newTestService(t)
	q, _ := svc.CreateQuest(models.QuestInput{Title: "Write the report", AttributeRewards: map[string]int64{"focus": 40}})
	svc.SetCompleterForTest(func(_, prompt string) (string, error) {
		if !strings.Contains(prompt, "Write the report") {
			t.Errorf("prompt lacks the quest: %q", prompt)
		}
		return `{"steps":[{"title":"Open the doc","attribute_rewards":{"focus":3}},{"title":"Write one heading","attribute_rewards":{"focus":5}},{"title":"","attribute_rewards":{}},{"title":"Write 3 bullets","attribute_rewards":{"nonsense":9}}]}`, nil
	})
	got, err := svc.BreakDownQuest(q.ID)
	if err != nil {
		t.Fatalf("break down: %v", err)
	}
	if len(got.Subtasks) != 3 || got.Subtasks[0].Title != "Open the doc" || len(got.Subtasks[2].AttributeRewards) != 0 {
		t.Errorf("subtasks = %+v", got.Subtasks)
	}
	if got.Status != "active" || got.Title != "Write the report" {
		t.Errorf("quest changed: %+v", got)
	}
}

func TestShrinkQuestFromModel(t *testing.T) {
	svc := newTestService(t)
	big, _ := svc.CreateQuest(models.QuestInput{Title: "30 minute workout", Type: "daily", Trigger: "after coffee", AttributeRewards: map[string]int64{"strength": 40}})
	svc.SetCompleterForTest(func(_, _ string) (string, error) {
		return `{"title":"5 push-ups","description":"Just five.","difficulty":"trivial","attribute_rewards":{"strength":10}}`, nil
	})
	small, err := svc.ShrinkQuest(big.ID)
	if err != nil {
		t.Fatalf("shrink: %v", err)
	}
	if small.Title != "5 push-ups" || small.Type != "daily" || small.Trigger != "after coffee" || small.AttributeRewards["strength"] != 10 {
		t.Errorf("replacement = %+v", small)
	}
	if old, _ := svc.store.GetQuest(1, big.ID); old.Status != "archived" {
		t.Errorf("original = %s, want archived", old.Status)
	}
	// Garbage from the model is a 400, and nothing was archived.
	other, _ := svc.CreateQuest(models.QuestInput{Title: "Read", AttributeRewards: map[string]int64{"learning": 20}})
	svc.SetCompleterForTest(func(_, _ string) (string, error) { return "not json", nil })
	if _, err := svc.ShrinkQuest(other.ID); err == nil {
		t.Error("garbage accepted")
	}
	if q, _ := svc.store.GetQuest(1, other.ID); q.Status != "active" {
		t.Errorf("failed shrink archived the original: %s", q.Status)
	}
}

func TestStoryNarrationSavesAndRemembersChapters(t *testing.T) {
	svc := newTestService(t)
	var prompts []string
	svc.SetCompleterForTest(func(_, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		return "  The hero sharpens the blade.  ", nil
	})
	first, err := svc.StoryNarration()
	if err != nil || first != "The hero sharpens the blade." {
		t.Fatalf("narration = %q, %v", first, err)
	}
	if _, err := svc.StoryNarration(); err != nil {
		t.Fatal(err)
	}
	chapters, _ := svc.ListStoryChapters(5)
	if len(chapters) != 2 || chapters[0].Number != 2 {
		t.Fatalf("chapters = %+v", chapters)
	}
	if strings.Contains(prompts[0], "Previous chapters") || !strings.Contains(prompts[1], "Chapter 1: The hero sharpens the blade.") {
		t.Errorf("memory not fed back: first=%v second=%v", strings.Contains(prompts[0], "Previous"), strings.Contains(prompts[1], "Chapter 1"))
	}
	if strings.Contains(strings.ToLower(prompts[0]), "threat") {
		t.Error("prompt still mentions threats")
	}
}
