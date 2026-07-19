package main

import (
	"strings"
	"testing"

	"edi/internal/models"
)

func TestStatusBlockContent(t *testing.T) {
	d := models.Dashboard{
		Character:     models.CharacterSummary{Level: 12},
		Streak:        models.Streak{Current: 4},
		GoldBalance:   213,
		TodayQuests:   []models.Quest{{ID: 1, Title: "x"}, {ID: 2, Title: "y"}, {ID: 3, Title: "z"}},
		DailyProgress: models.DailyProgress{CompletedToday: 2, Goal: 5},
		Attributes: []models.Attribute{
			{Key: "focus", Name: "Focus", Decay: &models.AttributeDecay{State: "decaying", IdleDays: 6, ProjectedDailyLoss: 6}},
			{Key: "strength", Name: "Strength", Decay: &models.AttributeDecay{State: "fresh"}},
		},
	}
	out := statusBlock(d)
	for _, want := range []string{"Lv 12", "4", "213g", "3 quests open", "2 done today", "Focus", "-6"} {
		if !strings.Contains(out, want) {
			t.Errorf("status block missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "rest") {
		t.Errorf("rest banner shown while rest off:\n%s", out)
	}

	d.RestMode = true
	if out := statusBlock(d); !strings.Contains(strings.ToLower(out), "rest") {
		t.Errorf("rest banner missing while rest on:\n%s", out)
	}
}
