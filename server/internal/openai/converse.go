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

// Multi-turn, tool-calling variant of Complete. The Responses API is
// stateless here (store=false), so the caller keeps the conversation as a
// list of raw items and replays it every turn: user messages, the model's own
// output items (assistant messages, function_call items and the encrypted
// reasoning items that must travel with them) and function_call_output items
// carrying tool results.

// Item is one Responses-API input/output item, kept as raw JSON so model
// output (including encrypted reasoning) is replayed losslessly.
type Item = json.RawMessage

// ToolDef is a function tool the model may call.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is one function call the model requested in a turn.
type ToolCall struct {
	CallID    string
	Name      string
	Arguments json.RawMessage
}

// Turn is the result of one Converse round.
type Turn struct {
	Text      string     // assistant text (may be empty when only tools were called)
	ToolCalls []ToolCall // tool calls to satisfy before the next round
	Output    []Item     // every output item, to append to the history verbatim
}

// UserMessage builds a user input item.
func UserMessage(text string) Item {
	b, _ := json.Marshal(map[string]any{
		"type": "message", "role": "user",
		"content": []map[string]any{{"type": "input_text", "text": text}},
	})
	return b
}

// ToolResult builds a function_call_output item answering a ToolCall.
func ToolResult(callID, output string) Item {
	b, _ := json.Marshal(map[string]any{
		"type": "function_call_output", "call_id": callID, "output": output,
	})
	return b
}

// Converse runs one round: instructions + full history in, the model's output
// items out. Tool calls are returned, not executed — the caller runs them,
// appends ToolResult items and calls again.
func Converse(accessToken, accountID, model, effort, instructions string, input []Item, tools []ToolDef) (Turn, error) {
	if model == "" {
		model = DefaultModel
	}
	if input == nil {
		input = []Item{}
	}
	toolDefs := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		toolDefs = append(toolDefs, map[string]any{
			"type": "function", "name": t.Name, "description": t.Description,
			"parameters": params, "strict": false,
		})
	}
	reqBody := map[string]any{
		"model":               model,
		"instructions":        instructions,
		"input":               input,
		"tools":               toolDefs,
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
		"store":               false,
		"stream":              true,
		"include":             []string{"reasoning.encrypted_content"},
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

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Turn{}, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := readLimited(resp.Body, 2000)
		return Turn{}, ErrUnauthorized{Body: body}
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := readLimited(resp.Body, 2000)
		return Turn{}, fmt.Errorf("openai responses returned %d: %s", resp.StatusCode, truncate(body, 500))
	}
	return parseTurnSSE(resp.Body)
}

// parseTurnSSE collects the completed output items of a streamed response.
// It reads `response.output_item.done` events (each a finished item) and
// falls back to the `response.completed` payload's output array; both are
// kept raw so reasoning items replay unchanged.
func parseTurnSSE(r interface{ Read([]byte) (int, error) }) (Turn, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var doneItems []Item
	var completed []Item
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
			Item     json.RawMessage `json:"item"`
			Response json.RawMessage `json:"response"`
			Error    json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		switch {
		case ev.Type == "response.output_item.done" && len(ev.Item) > 0:
			doneItems = append(doneItems, Item(ev.Item))
		case ev.Type == "response.completed" && len(ev.Response) > 0:
			var resp struct {
				Output []json.RawMessage `json:"output"`
			}
			if json.Unmarshal(ev.Response, &resp) == nil {
				for _, o := range resp.Output {
					completed = append(completed, Item(o))
				}
			}
		case ev.Type == "response.failed", strings.HasSuffix(ev.Type, ".error"), ev.Type == "error", len(ev.Error) > 0:
			apiErr = data
		}
	}
	if err := sc.Err(); err != nil {
		return Turn{}, fmt.Errorf("read stream: %w", err)
	}
	items := completed
	if len(items) == 0 {
		items = doneItems
	}
	if len(items) == 0 {
		if apiErr != "" {
			return Turn{}, fmt.Errorf("openai stream error: %s", truncate(apiErr, 400))
		}
		return Turn{}, fmt.Errorf("openai returned no output")
	}
	return turnFromItems(items), nil
}

// turnFromItems extracts text and tool calls from raw output items.
func turnFromItems(items []Item) Turn {
	t := Turn{Output: items}
	var text strings.Builder
	for _, raw := range items {
		var it struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(raw, &it) != nil {
			continue
		}
		switch it.Type {
		case "message":
			for _, c := range it.Content {
				if c.Type == "output_text" {
					text.WriteString(c.Text)
				}
			}
		case "function_call":
			args := json.RawMessage(it.Arguments)
			if !json.Valid(args) {
				args = json.RawMessage(`{}`)
			}
			t.ToolCalls = append(t.ToolCalls, ToolCall{CallID: it.CallID, Name: it.Name, Arguments: args})
		}
	}
	t.Text = text.String()
	return t
}
