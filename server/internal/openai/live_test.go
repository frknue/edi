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

	out, err := Complete(tok.AccessToken, tok.AccountID, DefaultModel, "low",
		"You are a terse test probe.",
		"Reply with exactly the single word: PONG")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	t.Logf("model output: %q", out)
	if !strings.Contains(strings.ToUpper(out), "PONG") {
		t.Errorf("expected PONG, got %q", out)
	}
}

// TestLiveConverseToolCall probes the function-calling contract of the codex
// backend under store=false: turn 1 must produce a function_call, replaying
// the history + a function_call_output must yield text in turn 2.
//
//	EDI_LIVE_TEST=1 go test ./internal/openai -run TestLiveConverseToolCall -v
func TestLiveConverseToolCall(t *testing.T) {
	if os.Getenv("EDI_LIVE_TEST") != "1" {
		t.Skip("set EDI_LIVE_TEST=1 to run the live OpenAI test")
	}
	tok := liveTokens(t)
	tools := []ToolDef{{
		Name:        "get_gold",
		Description: "Return the player's current gold balance.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}}
	history := []Item{UserMessage("How much gold do I have? Use the tool, then answer in one short sentence.")}
	turn, err := Converse(tok.AccessToken, tok.AccountID, DefaultModel, "low", "You are a terse RPG assistant.", history, tools)
	if err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	t.Logf("turn 1: text=%q calls=%d items=%d", turn.Text, len(turn.ToolCalls), len(turn.Output))
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "get_gold" {
		t.Fatalf("expected one get_gold call, got %+v", turn.ToolCalls)
	}
	history = append(history, turn.Output...)
	history = append(history, ToolResult(turn.ToolCalls[0].CallID, `{"gold":137}`))
	turn2, err := Converse(tok.AccessToken, tok.AccountID, DefaultModel, "low", "You are a terse RPG assistant.", history, tools)
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	t.Logf("turn 2: text=%q calls=%d", turn2.Text, len(turn2.ToolCalls))
	if !strings.Contains(turn2.Text, "137") {
		t.Errorf("expected the answer to mention 137, got %q", turn2.Text)
	}
}

func liveTokens(t *testing.T) Tokens {
	t.Helper()
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
		refreshed, err := Refresh(tok.RefreshToken)
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if refreshed.AccountID == "" {
			refreshed.AccountID = tok.AccountID
		}
		tok = refreshed
	}
	return tok
}
