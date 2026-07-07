package services

import (
	"os"
	"testing"
)

// TestLiveGenerateSuggestions runs the full LLM suggestion pipeline against the
// real ChatGPT subscription, using the local Codex CLI's tokens (imported into a
// throwaway DB). Only runs when EDI_LIVE_TEST=1.
//
//	EDI_LIVE_TEST=1 go test ./internal/services -run TestLiveGenerateSuggestions -v
func TestLiveGenerateSuggestions(t *testing.T) {
	if os.Getenv("EDI_LIVE_TEST") != "1" {
		t.Skip("set EDI_LIVE_TEST=1 to run the live OpenAI suggestion test")
	}
	svc := newTestService(t)

	status, err := svc.ImportCodexCredentials()
	if err != nil {
		t.Fatalf("import codex creds: %v", err)
	}
	if !status.Connected {
		t.Fatal("expected connected after import")
	}
	t.Logf("connected as %s (model %s)", status.Email, status.Model)

	suggestions, err := svc.GenerateSuggestions()
	if err != nil {
		t.Fatalf("generate suggestions: %v", err)
	}
	if len(suggestions) < 1 {
		t.Fatalf("expected >=1 LLM suggestion, got 0")
	}
	for i, s := range suggestions {
		t.Logf("suggestion %d: [%s] %q -> quest %q %v", i+1, s.Type, s.Title, s.SuggestedQuest.Title, s.SuggestedQuest.AttributeRewards)
		if s.Title == "" || s.SuggestedQuest.Title == "" || len(s.SuggestedQuest.AttributeRewards) == 0 {
			t.Errorf("suggestion %d is incomplete: %+v", i+1, s)
		}
	}

	// Accept the first one and confirm it becomes a real quest.
	q, err := svc.AcceptSuggestion(suggestions[0].ID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	t.Logf("accepted -> created quest #%d %q", q.ID, q.Title)
}
