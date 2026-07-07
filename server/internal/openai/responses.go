package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ResponsesURL is the ChatGPT backend endpoint that bills to the subscription.
const ResponsesURL = "https://chatgpt.com/backend-api/codex/responses"

// DefaultModel is used when EDI_OPENAI_MODEL is unset. gpt-5.5 is what the Codex
// ChatGPT-subscription backend accepts (verified live); codex-suffixed and older
// gpt-5.x ids are rejected for ChatGPT accounts.
const DefaultModel = "gpt-5.5"

// ErrUnauthorized signals an expired/invalid access token (caller should refresh).
type ErrUnauthorized struct{ Body string }

func (e ErrUnauthorized) Error() string {
	return "openai: unauthorized (" + truncate(e.Body, 200) + ")"
}

// Complete sends a single-turn prompt and returns the model's text output. It
// speaks the streaming (SSE) protocol the backend requires (store=false) but
// accumulates the full text before returning. effort selects the reasoning
// depth ("minimal"|"low"|"medium"|"high"); empty uses the model default.
func Complete(accessToken, accountID, model, effort, instructions, prompt string) (string, error) {
	if model == "" {
		model = DefaultModel
	}
	reqBody := map[string]any{
		"model":        model,
		"instructions": instructions,
		"input": []map[string]any{{
			"type": "message",
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text",
				"text": prompt,
			}},
		}},
		"store":   false,
		"stream":  true,
		"include": []string{"reasoning.encrypted_content"},
	}
	if effort != "" {
		reqBody["reasoning"] = map[string]any{"effort": effort}
	}
	buf, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, ResponsesURL, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := readLimited(resp.Body, 2000)
		return "", ErrUnauthorized{Body: body}
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := readLimited(resp.Body, 2000)
		return "", fmt.Errorf("openai responses returned %d: %s", resp.StatusCode, truncate(body, 500))
	}

	return parseSSE(resp.Body)
}

// parseSSE reads the event stream and accumulates output text. It collects
// `response.output_text.delta` events and falls back to the final
// `response.completed` payload's output text.
func parseSSE(r interface{ Read([]byte) (int, error) }) (string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var deltas strings.Builder
	var finalText string
	var apiErr string

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var ev struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			Response json.RawMessage `json:"response"`
			Error    json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		switch {
		case ev.Type == "response.output_text.delta":
			deltas.WriteString(ev.Delta)
		case ev.Type == "response.completed" && len(ev.Response) > 0:
			finalText = extractOutputText(ev.Response)
		case strings.Contains(ev.Type, "error") || len(ev.Error) > 0:
			apiErr = data
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read stream: %w", err)
	}

	if deltas.Len() > 0 {
		return deltas.String(), nil
	}
	if finalText != "" {
		return finalText, nil
	}
	if apiErr != "" {
		return "", fmt.Errorf("openai stream error: %s", truncate(apiErr, 400))
	}
	return "", fmt.Errorf("openai returned no text output")
}

// extractOutputText pulls concatenated text from a Responses `response` object.
func extractOutputText(raw json.RawMessage) string {
	var r struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	if r.OutputText != "" {
		return r.OutputText
	}
	var b strings.Builder
	for _, o := range r.Output {
		if o.Type != "message" {
			continue
		}
		for _, c := range o.Content {
			if c.Type == "output_text" {
				b.WriteString(c.Text)
			}
		}
	}
	return b.String()
}

func readLimited(r interface{ Read([]byte) (int, error) }, n int) (string, error) {
	buf := make([]byte, n)
	total := 0
	for total < n {
		m, err := r.Read(buf[total:])
		total += m
		if err != nil {
			break
		}
	}
	return string(buf[:total]), nil
}
