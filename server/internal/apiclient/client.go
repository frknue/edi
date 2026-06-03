// Package apiclient is a typed Go client for the Life RPG REST API. It is the
// shared foundation for non-web clients (CLI, MCP bridge, future mobile) and is
// deliberately a *pure HTTP client* — it talks to the same documented endpoints
// the web UI uses, proving there is no privileged/hidden data path.
package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"liferpg/internal/models"
)

// Client talks to a running Life RPG server.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for baseURL (e.g. "http://localhost:8080").
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// do performs a request, decoding a JSON {error} body into a Go error on >=400.
func (c *Client) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed (is the server running at %s?): %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error != "" {
			return fmt.Errorf("%s (HTTP %d)", e.Error, resp.StatusCode)
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// --- typed REST surface -----------------------------------------------------

func (c *Client) Dashboard() (models.Dashboard, error) {
	var d models.Dashboard
	err := c.do(http.MethodGet, "/api/dashboard", nil, &d)
	return d, err
}

func (c *Client) ListQuests(questType, status string) ([]models.Quest, error) {
	q := url.Values{}
	if questType != "" {
		q.Set("type", questType)
	}
	if status != "" {
		q.Set("status", status)
	}
	path := "/api/quests"
	if s := q.Encode(); s != "" {
		path += "?" + s
	}
	var out []models.Quest
	err := c.do(http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) CreateQuest(in models.QuestInput) (models.Quest, error) {
	var qst models.Quest
	err := c.do(http.MethodPost, "/api/quests", in, &qst)
	return qst, err
}

func (c *Client) CompleteQuest(id int64) (models.CompletionResult, error) {
	var r models.CompletionResult
	err := c.do(http.MethodPost, fmt.Sprintf("/api/quests/%d/complete", id), nil, &r)
	return r, err
}

func (c *Client) SkipQuest(id int64) (models.Quest, error) {
	var qst models.Quest
	err := c.do(http.MethodPost, fmt.Sprintf("/api/quests/%d/skip", id), nil, &qst)
	return qst, err
}

func (c *Client) ArchiveQuest(id int64) (models.Quest, error) {
	var qst models.Quest
	err := c.do(http.MethodPost, fmt.Sprintf("/api/quests/%d/archive", id), nil, &qst)
	return qst, err
}

func (c *Client) ListJournal(limit int) ([]models.JournalEntry, error) {
	var out []models.JournalEntry
	err := c.do(http.MethodGet, fmt.Sprintf("/api/journal?limit=%d", limit), nil, &out)
	return out, err
}

func (c *Client) CreateJournal(in models.JournalInput) (models.JournalEntry, error) {
	var e models.JournalEntry
	err := c.do(http.MethodPost, "/api/journal", in, &e)
	return e, err
}

func (c *Client) ListSuggestions(status string) ([]models.AgentSuggestion, error) {
	path := "/api/agent/suggestions"
	if status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	var out []models.AgentSuggestion
	err := c.do(http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) GenerateSuggestions() ([]models.AgentSuggestion, error) {
	var out []models.AgentSuggestion
	err := c.do(http.MethodPost, "/api/agent/suggestions/generate", nil, &out)
	return out, err
}

func (c *Client) AcceptSuggestion(id int64) (models.Quest, error) {
	var qst models.Quest
	err := c.do(http.MethodPost, fmt.Sprintf("/api/agent/suggestions/%d/accept", id), nil, &qst)
	return qst, err
}

func (c *Client) DismissSuggestion(id int64) (models.AgentSuggestion, error) {
	var s models.AgentSuggestion
	err := c.do(http.MethodPost, fmt.Sprintf("/api/agent/suggestions/%d/dismiss", id), nil, &s)
	return s, err
}

// --- agent tool surface (used by the MCP bridge) ----------------------------

// ToolSpec mirrors one entry from GET /api/agent/tools.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ListTools returns the discoverable agent tool catalog.
func (c *Client) ListTools() ([]ToolSpec, error) {
	var out struct {
		Tools []ToolSpec `json:"tools"`
	}
	err := c.do(http.MethodGet, "/api/agent/tools", nil, &out)
	return out.Tools, err
}

// InvokeTool calls a named agent tool with raw JSON arguments and returns the raw
// JSON result (the same service path the REST API and web UI use).
func (c *Client) InvokeTool(name string, args json.RawMessage) (json.RawMessage, error) {
	var body any
	if len(args) > 0 {
		body = args
	}
	var out struct {
		Tool   string          `json:"tool"`
		Result json.RawMessage `json:"result"`
	}
	if err := c.do(http.MethodPost, "/api/agent/tools/"+url.PathEscape(name)+"/invoke", body, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}
