package openai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLiveComplete hits the real ChatGPT backend using the local Codex CLI's
// tokens. It only runs when EDI_LIVE_TEST=1 so normal `go test` stays offline.
//
//	EDI_LIVE_TEST=1 go test ./internal/openai -run TestLiveComplete -v
func TestLiveComplete(t *testing.T) {
	if os.Getenv("EDI_LIVE_TEST") != "1" {
		t.Skip("set EDI_LIVE_TEST=1 to run the live OpenAI test")
	}
	home, _ := os.UserHomeDir()
	raw, err := os.ReadFile(filepath.Join(home, ".codex", "auth.json"))
	if err != nil {
		t.Fatalf("read codex auth.json: %v", err)
	}
	var f struct {
		Tokens struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse: %v", err)
	}

	tok := TokensFromStored(f.Tokens.AccessToken, f.Tokens.RefreshToken, f.Tokens.IDToken, f.Tokens.AccountID)
	if time.Now().After(tok.ExpiresAt.Add(-time.Minute)) {
		t.Log("access token stale; refreshing")
		refreshed, err := Refresh(tok.RefreshToken)
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if refreshed.AccountID == "" {
			refreshed.AccountID = tok.AccountID
		}
		tok = refreshed
	}
	t.Logf("account_id=%s email=%s", tok.AccountID, tok.Email)

	candidates := []string{"gpt-5.5-codex", "gpt-5.5", "gpt-5.2-codex", "gpt-5.2", "gpt-5-codex", "gpt-5", "codex-mini-latest"}
	if m := os.Getenv("EDI_OPENAI_MODEL"); m != "" {
		candidates = []string{m}
	}
	var worked string
	for _, m := range candidates {
		out, err := Complete(tok.AccessToken, tok.AccountID, m,
			"You are a terse test probe.",
			"Reply with exactly the single word: PONG")
		if err != nil {
			t.Logf("  %-18s -> ERR %s", m, truncate(err.Error(), 120))
			continue
		}
		t.Logf("  %-18s -> OK %q", m, truncate(out, 60))
		if strings.Contains(strings.ToUpper(out), "PONG") && worked == "" {
			worked = m
		}
	}
	if worked == "" {
		t.Fatal("no candidate model worked")
	}
	t.Logf("WORKING MODEL: %s", worked)
}
