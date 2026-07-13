package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ModelsURL lists the models available to the account. CodexClientVersion must be
// recent enough — the endpoint returns an empty list for versions below a model's
// minimal_client_version. Bump this if OpenAI raises that floor.
const (
	ModelsURL          = "https://chatgpt.com/backend-api/codex/models"
	CodexClientVersion = "0.144.1"
)

// Model is one selectable model with its supported reasoning efforts.
type Model struct {
	Slug          string   `json:"slug"`
	DisplayName   string   `json:"display_name"`
	Description   string   `json:"description"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"default_effort"`
}

// ListModels fetches the account's available (listed) models.
func ListModels(accessToken, accountID string) ([]Model, error) {
	u := ModelsURL + "?" + url.Values{"client_version": {CodexClientVersion}}.Encode()
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("originator", "codex_cli_rs")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := readLimited(resp.Body, 1000)
		return nil, ErrUnauthorized{Body: body}
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := readLimited(resp.Body, 1000)
		return nil, fmt.Errorf("list models returned %d: %s", resp.StatusCode, truncate(body, 300))
	}

	var raw struct {
		Models []struct {
			Slug                   string `json:"slug"`
			DisplayName            string `json:"display_name"`
			Description            string `json:"description"`
			Visibility             string `json:"visibility"`
			DefaultReasoningLevel  string `json:"default_reasoning_level"`
			SupportedReasoningLvls []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}

	out := make([]Model, 0, len(raw.Models))
	for _, m := range raw.Models {
		if m.Visibility != "list" {
			continue // skip hidden/internal models
		}
		efforts := make([]string, 0, len(m.SupportedReasoningLvls))
		for _, e := range m.SupportedReasoningLvls {
			efforts = append(efforts, e.Effort)
		}
		out = append(out, Model{
			Slug:          m.Slug,
			DisplayName:   m.DisplayName,
			Description:   m.Description,
			Efforts:       efforts,
			DefaultEffort: m.DefaultReasoningLevel,
		})
	}
	return out, nil
}
