package main

import (
	"strings"
	"testing"
	"time"

	"edi/internal/models"
)

func sampleDashboard() models.Dashboard {
	return models.Dashboard{
		Character:   models.CharacterSummary{Name: "Hero", Level: 6},
		Streak:      models.Streak{Current: 4},
		GoldBalance: 213,
		TodayQuests: []models.Quest{
			{ID: 7, Title: "30 minute <workout>", Difficulty: "medium", AttributeRewards: map[string]int64{"strength": 40, "discipline": 10}},
			{ID: 9, Title: "Read 15 pages", Difficulty: "easy", AttributeRewards: map[string]int64{"learning": 30}},
		},
		DailyProgress: models.DailyProgress{CompletedToday: 2, Goal: 5},
		Attributes: []models.Attribute{
			{Key: "focus", Name: "Focus", Decay: &models.AttributeDecay{State: "decaying", IdleDays: 6, ProjectedDailyLoss: 6}},
			{Key: "strength", Name: "Strength", Decay: &models.AttributeDecay{State: "fresh"}},
		},
	}
}

func TestFormatBriefingContent(t *testing.T) {
	out := formatBriefing(sampleDashboard())
	for _, want := range []string{"Lv 6", "4", "213g", "#7", "#9", "Read 15 pages", "Focus", "-6"} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<workout>") {
		t.Errorf("quest title not HTML-escaped:\n%s", out)
	}
	if !strings.Contains(out, "&lt;workout&gt;") {
		t.Errorf("expected escaped title:\n%s", out)
	}
}

func TestFormatStatusRestBanner(t *testing.T) {
	d := sampleDashboard()
	d.RestMode = true
	out := formatStatus(d)
	if !strings.Contains(strings.ToLower(out), "rest") {
		t.Errorf("status missing rest banner:\n%s", out)
	}
}

func TestFormatQuestsEmpty(t *testing.T) {
	out := formatQuests(nil)
	if !strings.Contains(strings.ToLower(out), "no active quests") {
		t.Errorf("empty quests message = %q", out)
	}
}

func TestNudgeQuestConditions(t *testing.T) {
	d := sampleDashboard()

	// Completed today > 0: no nudge.
	if _, ok := nudgeQuest(d); ok {
		t.Error("nudge fired despite completions today")
	}

	// Nothing done: nudge the easiest (easy beats medium).
	d.DailyProgress.CompletedToday = 0
	q, ok := nudgeQuest(d)
	if !ok || q.ID != 9 {
		t.Errorf("nudge = %+v/%v, want quest 9 (easy)", q, ok)
	}

	// Difficulty tie: lower total reward wins.
	d.TodayQuests = []models.Quest{
		{ID: 1, Title: "A", Difficulty: "easy", AttributeRewards: map[string]int64{"focus": 50}},
		{ID: 2, Title: "B", Difficulty: "easy", AttributeRewards: map[string]int64{"focus": 20}},
	}
	if q, _ := nudgeQuest(d); q.ID != 2 {
		t.Errorf("tie-break picked %d, want 2 (lower reward)", q.ID)
	}

	// Rest mode: no nudge.
	d.RestMode = true
	if _, ok := nudgeQuest(d); ok {
		t.Error("nudge fired during rest mode")
	}
	d.RestMode = false

	// No quests: no nudge.
	d.TodayQuests = nil
	if _, ok := nudgeQuest(d); ok {
		t.Error("nudge fired with no quests")
	}
}

func TestNextFire(t *testing.T) {
	now := time.Date(2026, 7, 15, 7, 0, 0, 0, time.Local)
	fire := nextFire(now, "08:00")
	if fire.Hour() != 8 || fire.Day() != 15 {
		t.Errorf("before target: fire = %v, want today 08:00", fire)
	}
	now = time.Date(2026, 7, 15, 9, 0, 0, 0, time.Local)
	fire = nextFire(now, "08:00")
	if fire.Hour() != 8 || fire.Day() != 16 {
		t.Errorf("after target: fire = %v, want tomorrow 08:00", fire)
	}
	// Invalid HH:MM falls back to 08:00 rather than panicking.
	fire = nextFire(now, "not-a-time")
	if fire.Hour() != 8 {
		t.Errorf("invalid input: fire = %v, want 08:00 fallback", fire)
	}
}

func TestFireDue(t *testing.T) {
	fire := time.Date(2026, 7, 15, 8, 0, 0, 0, time.Local)

	if due, stale := fireDue(fire.Add(-time.Minute), fire); due || stale {
		t.Errorf("before fire: due=%v stale=%v, want false,false", due, stale)
	}
	if due, stale := fireDue(fire.Add(time.Minute), fire); !due || stale {
		t.Errorf("just after fire: due=%v stale=%v, want true,false", due, stale)
	}
	if due, stale := fireDue(fire.Add(11*time.Minute), fire); !due || !stale {
		t.Errorf("11m after fire: due=%v stale=%v, want true,true", due, stale)
	}
}

// TestNextFireDSTSafe pins down that "next occurrence" is derived by
// constructing tomorrow's wall-clock time via time.Date (which normalizes
// correctly across a DST transition), not by adding a fixed 24h duration
// (which would misfire — landing an hour off — whenever the intervening
// night crosses a spring-forward/fall-back boundary).
func TestNextFireDSTSafe(t *testing.T) {
	now := time.Date(2026, 7, 15, 21, 30, 0, 0, time.Local)
	fire := nextFire(now, "20:00")
	want := time.Date(2026, 7, 16, 20, 0, 0, 0, time.Local)
	if !fire.Equal(want) {
		t.Errorf("nextFire(%v, 20:00) = %v, want %v", now, fire, want)
	}
	if fire.Hour() != 20 || fire.Minute() != 0 {
		t.Errorf("nextFire wall-clock = %02d:%02d, want 20:00", fire.Hour(), fire.Minute())
	}
}

func TestParseCommand(t *testing.T) {
	cases := []struct{ in, cmd, arg string }{
		{"/status", "status", ""},
		{"/done 42", "done", "42"},
		{"/done   42  ", "done", "42"},
		{"/ward focus", "ward", "focus"},
		{"/rest on", "rest", "on"},
		{"/help@edi_bot", "help", ""},
		{"hello there", "hello", "there"},
		{"", "", ""},
	}
	for _, c := range cases {
		cmd, arg := parseCommand(c.in)
		if cmd != c.cmd || arg != c.arg {
			t.Errorf("parseCommand(%q) = %q,%q want %q,%q", c.in, cmd, arg, c.cmd, c.arg)
		}
	}
}
