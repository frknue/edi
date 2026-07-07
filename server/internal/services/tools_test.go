package services

import (
	"encoding/json"
	"testing"
)

func TestListToolsIncludesDailyMoodLog(t *testing.T) {
	svc := newTestService(t)
	tools := svc.ListTools()
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}
	found := false
	for _, tl := range tools {
		if tl.Key == "daily_mood_log" {
			found = true
			if tl.AttributeRewards["health"] != 25 || tl.AttributeRewards["spirituality"] != 15 || tl.AttributeRewards["discipline"] != 10 {
				t.Errorf("unexpected rewards: %v", tl.AttributeRewards)
			}
		}
	}
	if !found {
		t.Error("daily_mood_log not in tools list")
	}
}

func TestCompleteToolAwardsAuditableXP(t *testing.T) {
	svc := newTestService(t)

	before, _ := svc.ListAttributes()
	healthBefore := attrByKey(before, "health").TotalXP

	payload := json.RawMessage(`{
		"event":"Got harsh feedback on my project",
		"emotions":[{"category":"anxious","before":80,"after":40},{"category":"inferior","before":70,"after":25}],
		"thoughts":[{"thought":"I'm not good enough","belief_before":90,"distortions":["LAB","AON"],"positive_thought":"One piece of feedback isn't a verdict on my worth","positive_belief":80,"belief_after":30}]
	}`)

	res, err := svc.CompleteTool("daily_mood_log", payload)
	if err != nil {
		t.Fatalf("complete tool: %v", err)
	}
	if len(res.XPEvents) != 3 {
		t.Fatalf("got %d xp events, want 3 (health/spirituality/discipline)", len(res.XPEvents))
	}
	for _, e := range res.XPEvents {
		if e.Source != "tool" {
			t.Errorf("event source = %q, want tool", e.Source)
		}
	}
	after := res.Dashboard.Attributes
	if got := attrByKey(after, "health").TotalXP; got != healthBefore+25 {
		t.Errorf("health = %d, want %d", got, healthBefore+25)
	}
	if res.Entry.XPAwarded != 50 {
		t.Errorf("xp_awarded = %d, want 50", res.Entry.XPAwarded)
	}
	if res.Entry.Summary == "" {
		t.Error("expected a summary")
	}

	// Persisted + listable.
	entries, err := svc.ListToolEntries("daily_mood_log", 10)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
}

func TestCompleteToolValidation(t *testing.T) {
	svc := newTestService(t)
	cases := map[string]string{
		"empty event":        `{"event":"","emotions":[{"category":"sad","before":50,"after":10}],"thoughts":[{"thought":"x","belief_before":50,"belief_after":10,"positive_belief":50}]}`,
		"no emotions":        `{"event":"e","emotions":[],"thoughts":[{"thought":"x","belief_before":50,"belief_after":10,"positive_belief":50}]}`,
		"bad rating":         `{"event":"e","emotions":[{"category":"sad","before":150,"after":10}],"thoughts":[{"thought":"x","belief_before":50,"belief_after":10,"positive_belief":50}]}`,
		"unknown emotion":    `{"event":"e","emotions":[{"category":"jealous","before":50,"after":10}],"thoughts":[{"thought":"x","belief_before":50,"belief_after":10,"positive_belief":50}]}`,
		"unknown distortion": `{"event":"e","emotions":[{"category":"sad","before":50,"after":10}],"thoughts":[{"thought":"x","belief_before":50,"belief_after":10,"positive_belief":50,"distortions":["ZZ"]}]}`,
	}
	for name, body := range cases {
		if _, err := svc.CompleteTool("daily_mood_log", json.RawMessage(body)); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	// Unknown tool -> not found.
	if _, err := svc.CompleteTool("nope", json.RawMessage(`{}`)); err == nil {
		t.Error("unknown tool: expected error")
	}
}
