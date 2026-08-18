package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"edi/internal/db/dbtest"
	"edi/internal/models"
	"edi/internal/openai"
	"edi/internal/services"
)

// scriptedLLM replays canned turns; each call also records what it saw so
// tests can assert on the replayed history.
type scriptedLLM struct {
	turns []openai.Turn
	seen  [][]openai.Item
	tools []openai.ToolDef
}

func (f *scriptedLLM) fn() LLM {
	return func(_ string, history []openai.Item, tools []openai.ToolDef) (openai.Turn, error) {
		f.seen = append(f.seen, append([]openai.Item{}, history...))
		f.tools = tools
		if len(f.turns) == 0 {
			return openai.Turn{}, errors.New("script exhausted")
		}
		t := f.turns[0]
		f.turns = f.turns[1:]
		return t, nil
	}
}

func callTurn(id, name, args string) openai.Turn {
	item, _ := json.Marshal(map[string]any{"type": "function_call", "call_id": id, "name": name, "arguments": args})
	return openai.Turn{
		ToolCalls: []openai.ToolCall{{CallID: id, Name: name, Arguments: json.RawMessage(args)}},
		Output:    []openai.Item{item},
	}
}

func textTurn(text string) openai.Turn {
	item, _ := json.Marshal(map[string]any{"type": "message", "role": "assistant",
		"content": []map[string]any{{"type": "output_text", "text": text}}})
	return openai.Turn{Text: text, Output: []openai.Item{item}}
}

func newChatSvc(t *testing.T) *services.Service {
	t.Helper()
	store := dbtest.Open(t)
	if err := store.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return services.New(store, 1)
}

// "add X as a quest" → the model calls create_quest → the quest exists, the
// tool result is fed back, the reply is the model's text.
func TestChatCreatesQuestThroughRegistry(t *testing.T) {
	svc := newChatSvc(t)
	reg := NewRegistry()
	llm := &scriptedLLM{turns: []openai.Turn{
		callTurn("c1", "create_quest", `{"title":"Evening run","type":"daily","difficulty":"easy","attribute_rewards":{"strength":30}}`),
		textTurn("Added “Evening run” as a daily (+30 strength)."),
	}}
	sessions := NewSessions()
	res, err := reg.Chat(svc, llm.fn(), sessions, "t:1", "hey add an evening run as a daily quest")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !strings.Contains(res.Reply, "Evening run") || len(res.ToolsUsed) != 1 || res.ToolsUsed[0] != "create_quest" {
		t.Fatalf("result = %+v", res)
	}
	quests, _ := svc.ListQuests("daily", "active")
	found := false
	for _, q := range quests {
		if q.Title == "Evening run" && q.AttributeRewards["strength"] == 30 {
			found = true
		}
	}
	if !found {
		t.Fatalf("quest not created via registry: %+v", quests)
	}
	// Turn 2 saw: user msg, function_call, function_call_output.
	if len(llm.seen) != 2 || len(llm.seen[1]) != 3 {
		t.Fatalf("history replay wrong: %d calls, second saw %d items", len(llm.seen), len(llm.seen[1]))
	}
	var out struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Output string `json:"output"`
	}
	_ = json.Unmarshal(llm.seen[1][2], &out)
	if out.Type != "function_call_output" || out.CallID != "c1" || !strings.Contains(out.Output, `"Evening run"`) {
		t.Fatalf("tool result item = %+v", out)
	}
	if len(llm.tools) != len(reg.Specs()) {
		t.Fatalf("model got %d tools, registry has %d", len(llm.tools), len(reg.Specs()))
	}
	// The next message in the same session carries the earlier exchange.
	llm.turns = []openai.Turn{textTurn("Sure.")}
	if _, err := reg.Chat(svc, llm.fn(), sessions, "t:1", "thanks"); err != nil {
		t.Fatal(err)
	}
	if got := len(llm.seen[2]); got != 5 { // 4 prior items + new user message
		t.Fatalf("session history not kept: %d items", got)
	}
	// Other sessions start clean.
	llm.turns = []openai.Turn{textTurn("Hi.")}
	if _, err := reg.Chat(svc, llm.fn(), sessions, "t:2", "hi"); err != nil {
		t.Fatal(err)
	}
	if got := len(llm.seen[3]); got != 1 {
		t.Fatalf("session isolation broken: %d items", got)
	}
	// A no-tool answer still serializes tools_used as [] (never null).
	llm.turns = []openai.Turn{textTurn("Hey.")}
	plain, _ := reg.Chat(svc, llm.fn(), sessions, "t:3", "hi")
	if b, _ := json.Marshal(plain); !strings.Contains(string(b), `"tools_used":[]`) {
		t.Fatalf("tools_used must be [] not null: %s", b)
	}
}

// "I finished X" → complete_quest; the completion result (XP, gold) reaches
// the model and the XP actually lands.
func TestChatCompletesQuest(t *testing.T) {
	svc := newChatSvc(t)
	reg := NewRegistry()
	quests, _ := svc.ListQuests("", "active")
	target := quests[0]
	before, _ := svc.GetDashboard()

	llm := &scriptedLLM{turns: []openai.Turn{
		callTurn("c1", "list_quests", `{"status":"active"}`),
		callTurn("c2", "complete_quest", fmt.Sprintf(`{"quest_id":%d}`, target.ID)),
		textTurn("Done — logged it."),
	}}
	res, err := reg.Chat(svc, llm.fn(), NewSessions(), "t:1", "I finished "+target.Title)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if strings.Join(res.ToolsUsed, ",") != "list_quests,complete_quest" {
		t.Fatalf("tools used = %v", res.ToolsUsed)
	}
	after, _ := svc.GetDashboard()
	if after.GoldBalance <= before.GoldBalance {
		t.Fatalf("completion did not award gold: %d → %d", before.GoldBalance, after.GoldBalance)
	}
	var out struct {
		Output string `json:"output"`
	}
	_ = json.Unmarshal(llm.seen[2][len(llm.seen[2])-1], &out)
	if !strings.Contains(out.Output, `"xp_events"`) {
		t.Fatalf("completion result not fed back: %s", out.Output)
	}
}

// A failing tool is reported back to the model (which can self-correct);
// repeated failures end the turn with a clean validation error, not a 500.
func TestChatToolErrorsFeedBackThenCap(t *testing.T) {
	svc := newChatSvc(t)
	reg := NewRegistry()
	bad := `{"title":"X","attribute_rewards":{"nope":10}}`
	llm := &scriptedLLM{turns: []openai.Turn{
		callTurn("c1", "create_quest", bad),
		callTurn("c2", "create_quest", `{"title":"Fixed","attribute_rewards":{"strength":10}}`),
		textTurn("Fixed it."),
	}}
	res, err := reg.Chat(svc, llm.fn(), NewSessions(), "t:1", "add X")
	if err != nil || res.Reply != "Fixed it." {
		t.Fatalf("self-correct: %v %+v", err, res)
	}
	var out struct {
		Output string `json:"output"`
	}
	_ = json.Unmarshal(llm.seen[1][2], &out)
	if !strings.Contains(out.Output, "unknown attribute") {
		t.Fatalf("error not fed back: %s", out.Output)
	}

	llm = &scriptedLLM{turns: []openai.Turn{
		callTurn("c1", "create_quest", bad), callTurn("c2", "create_quest", bad),
		callTurn("c3", "create_quest", bad), callTurn("c4", "create_quest", bad),
	}}
	_, err = reg.Chat(svc, llm.fn(), NewSessions(), "t:2", "add X")
	if !errors.Is(err, services.ErrValidation) || !strings.Contains(err.Error(), "repeated tool errors") {
		t.Fatalf("expected repeated-errors validation error, got %v", err)
	}
	if len(llm.seen) != 3 {
		t.Fatalf("expected the loop to stop after 3 failures, made %d model calls", len(llm.seen))
	}

	// Unknown tools count as failures too, not crashes.
	llm = &scriptedLLM{turns: []openai.Turn{callTurn("c1", "launch_rockets", `{}`), textTurn("No such thing.")}}
	if _, err := reg.Chat(svc, llm.fn(), NewSessions(), "t:3", "launch"); err != nil {
		t.Fatalf("unknown tool should be recoverable: %v", err)
	}
	if !strings.Contains(err2str(llm.seen[1][2]), "unknown tool") {
		t.Fatalf("unknown-tool error not fed back")
	}
}

func err2str(item openai.Item) string {
	var out struct {
		Output string `json:"output"`
	}
	_ = json.Unmarshal(item, &out)
	return out.Output
}

// The round cap stops a model that never answers.
func TestChatRoundCap(t *testing.T) {
	svc := newChatSvc(t)
	reg := NewRegistry()
	var turns []openai.Turn
	for i := 0; i < maxToolRounds+3; i++ {
		turns = append(turns, callTurn(fmt.Sprintf("c%d", i), "get_dashboard", `{}`))
	}
	llm := &scriptedLLM{turns: turns}
	_, err := reg.Chat(svc, llm.fn(), NewSessions(), "t:1", "loop forever")
	if !errors.Is(err, services.ErrValidation) || !strings.Contains(err.Error(), "too many steps") {
		t.Fatalf("expected round-cap error, got %v", err)
	}
	if len(llm.seen) != maxToolRounds+1 {
		t.Fatalf("made %d model calls, want %d", len(llm.seen), maxToolRounds+1)
	}
}

// Without a connected ChatGPT account the production LLM gates cleanly.
func TestChatNotConnected(t *testing.T) {
	svc := newChatSvc(t)
	reg := NewRegistry()
	_, err := reg.Chat(svc, svc.OpenAIConverse, NewSessions(), "t:1", "hello")
	if !errors.Is(err, services.ErrOpenAINotConnected) {
		t.Fatalf("expected ErrOpenAINotConnected, got %v", err)
	}
	if _, err := reg.Chat(svc, svc.OpenAIConverse, NewSessions(), "t:1", "   "); !errors.Is(err, services.ErrValidation) {
		t.Fatalf("empty message should be a validation error, got %v", err)
	}
}

func TestTrimHistoryKeepsCallPairs(t *testing.T) {
	var h []openai.Item
	for i := 0; i < 5; i++ {
		h = append(h, openai.UserMessage(fmt.Sprintf("u%d", i)))
		h = append(h, callTurn("c", "get_dashboard", `{}`).Output...)
		h = append(h, openai.ToolResult("c", "{}"))
	}
	got := trimHistory(h, 7)
	if len(got) != 6 { // cut lands on user u3: u3, call, out, u4, call, out
		t.Fatalf("trimmed to %d items", len(got))
	}
	var first struct{ Role string }
	_ = json.Unmarshal(got[0], &first)
	if first.Role != "user" {
		t.Fatalf("trim must start at a user message, got %s", got[0])
	}
}

func TestEncodeToolResultTruncates(t *testing.T) {
	big := models.Quest{Description: strings.Repeat("x", maxToolResultSize*2)}
	out := encodeToolResult(big)
	if len(out) > maxToolResultSize+64 || !strings.Contains(out, "truncated") {
		t.Fatalf("not truncated: %d bytes", len(out))
	}
}
