package services

import (
	"errors"
	"testing"

	"edi/internal/models"
)

func TestDraftQuestRequiresTitle(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.DraftQuest(models.QuestDraftRequest{Title: "   "})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("DraftQuest without a title = %v, want ErrValidation", err)
	}
}

func TestDraftQuestRequiresOpenAI(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.DraftQuest(models.QuestDraftRequest{Title: "30 minute run"})
	if !errors.Is(err, ErrOpenAINotConnected) {
		t.Fatalf("DraftQuest without OpenAI = %v, want ErrOpenAINotConnected", err)
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("ErrOpenAINotConnected should map to a 400 (wrap ErrValidation), got %v", err)
	}
}

// The model's output is untrusted: bad enums fall back to the form defaults,
// unknown/zero attributes are dropped, XP snaps to the ±5 steps the UI uses,
// and the draft never carries more than 3 attributes.
func TestSanitizeDraft(t *testing.T) {
	known := map[string]string{
		"strength": "Strength", "discipline": "Discipline", "focus": "Focus",
		"health": "Health", "learning": "Learning",
	}

	got := sanitizeDraft(models.QuestDraft{
		Type:       "  DAILY ",
		Difficulty: "impossible", // not a real difficulty
		AttributeRewards: map[string]int64{
			"Strength":   22, // mixed case + rounds to 20
			"discipline": 8,  // rounds to 10
			"focus":      13, // rounds to 15
			"health":     3,  // rounds up to the 5 minimum
			"learning":   0,  // dropped: not positive
			"charisma":   40, // dropped: unknown attribute
		},
		Reason: "  trains the legs  ",
	}, known)

	if got.Type != "daily" {
		t.Errorf("type = %q, want daily", got.Type)
	}
	if got.Difficulty != "easy" {
		t.Errorf("difficulty = %q, want the easy fallback", got.Difficulty)
	}
	if got.Reason != "trains the legs" {
		t.Errorf("reason = %q, want it trimmed", got.Reason)
	}
	if len(got.AttributeRewards) != maxDraftAttributes {
		t.Fatalf("rewards = %v, want the top %d only", got.AttributeRewards, maxDraftAttributes)
	}
	want := map[string]int64{"strength": 20, "focus": 15, "discipline": 10}
	for k, v := range want {
		if got.AttributeRewards[k] != v {
			t.Errorf("rewards[%s] = %d, want %d (in %v)", k, got.AttributeRewards[k], v, got.AttributeRewards)
		}
	}
	for _, gone := range []string{"charisma", "learning", "Strength"} {
		if _, ok := got.AttributeRewards[gone]; ok {
			t.Errorf("rewards should not contain %q: %v", gone, got.AttributeRewards)
		}
	}
	// Every reward is a usable 5-step value.
	for k, v := range got.AttributeRewards {
		if v%5 != 0 || v < 5 {
			t.Errorf("rewards[%s] = %d, want a positive multiple of 5", k, v)
		}
	}
}

func TestSanitizeDraftClampsXP(t *testing.T) {
	got := sanitizeDraft(models.QuestDraft{
		Type:             "boss",
		Difficulty:       "boss",
		AttributeRewards: map[string]int64{"strength": 9999},
	}, map[string]string{"strength": "Strength"})

	if got.AttributeRewards["strength"] != maxDraftXP {
		t.Errorf("rewards[strength] = %d, want the %d cap", got.AttributeRewards["strength"], maxDraftXP)
	}
	if got.Type != "boss" || got.Difficulty != "boss" {
		t.Errorf("type/difficulty = %q/%q, want boss/boss kept", got.Type, got.Difficulty)
	}
}
