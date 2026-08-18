package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"edi/internal/openai"
	"edi/internal/services"
)

// Conversational agent: free-text messages ("hey, add X as a quest", "I just
// finished the run") are answered by the user's connected ChatGPT model, which
// acts through the SAME tool registry every other client uses. The model never
// touches data directly — every action is a registry.Invoke on the user-bound
// Service, so all invariants (completion gate, XP audit, validation) hold.

// LLM runs one conversation round. Production passes svc.OpenAIConverse;
// tests inject a scripted fake so the loop is exercised offline.
type LLM func(instructions string, history []openai.Item, tools []openai.ToolDef) (openai.Turn, error)

const (
	maxToolRounds     = 8                // tool-call rounds per user message
	maxToolFailures   = 3                // consecutive failing tool calls before giving up
	maxToolResultSize = 6 * 1024         // bytes of a tool result fed back to the model
	chatDeadline      = 90 * time.Second // wall clock per user message
	sessionTTL        = 2 * time.Hour    // idle time before a conversation is forgotten
	sessionMaxItems   = 80               // history items kept per conversation
)

// ChatResult is the agent's answer to one user message.
type ChatResult struct {
	Reply     string   `json:"reply"`
	ToolsUsed []string `json:"tools_used"` // tool names in call order (for transparency)
}

// Sessions keeps recent conversation history per key (e.g. "telegram:<chat>")
// in memory. History is a convenience, not a record: a restart forgets it,
// which is fine — every action already landed through the service layer.
type Sessions struct {
	mu   sync.Mutex
	byID map[string]*session
}

type session struct {
	mu      sync.Mutex // serializes turns of one conversation
	history []openai.Item
	touched time.Time
}

// NewSessions returns an empty in-memory conversation store.
func NewSessions() *Sessions { return &Sessions{byID: map[string]*session{}} }

// get returns (creating) the session for key and sweeps idle ones.
func (s *Sessions) get(key string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.byID {
		if k != key && now.Sub(v.touched) > sessionTTL {
			delete(s.byID, k)
		}
	}
	sess := s.byID[key]
	if sess == nil || now.Sub(sess.touched) > sessionTTL {
		sess = &session{}
		s.byID[key] = sess
	}
	sess.touched = now
	return sess
}

// Reset forgets one conversation.
func (s *Sessions) Reset(key string) {
	s.mu.Lock()
	delete(s.byID, key)
	s.mu.Unlock()
}

// Chat answers one user message in the conversation `key`, calling tools as
// needed. Turns of the same conversation are serialized; the history is
// updated only when the exchange completes.
func (r *Registry) Chat(svc *services.Service, llm LLM, sessions *Sessions, key, message string) (ChatResult, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return ChatResult{}, fmt.Errorf("%w: say something first", services.ErrValidation)
	}
	sess := sessions.get(key)
	sess.mu.Lock()
	defer sess.mu.Unlock()

	instructions, err := r.chatInstructions(svc)
	if err != nil {
		return ChatResult{}, err
	}
	history := append(append([]openai.Item{}, sess.history...), openai.UserMessage(message))
	result, history, err := r.runLoop(svc, llm, instructions, history)
	if err != nil {
		return ChatResult{}, err
	}
	if len(history) > sessionMaxItems {
		history = trimHistory(history, sessionMaxItems)
	}
	sess.history = history
	sess.touched = time.Now()
	return result, nil
}

// runLoop drives model ↔ tools until the model answers in text.
func (r *Registry) runLoop(svc *services.Service, llm LLM, instructions string, history []openai.Item) (ChatResult, []openai.Item, error) {
	tools := r.toolDefs()
	result := ChatResult{ToolsUsed: []string{}} // never null on the wire
	start := time.Now()
	failures := 0
	for round := 0; ; round++ {
		turn, err := llm(instructions, history, tools)
		if err != nil {
			return ChatResult{}, nil, err
		}
		history = append(history, turn.Output...)
		if len(turn.ToolCalls) == 0 {
			result.Reply = strings.TrimSpace(turn.Text)
			if result.Reply == "" {
				result.Reply = "Done."
			}
			return result, history, nil
		}
		if round >= maxToolRounds || time.Since(start) > chatDeadline {
			return ChatResult{}, nil, fmt.Errorf("%w: that took too many steps — try a smaller request", services.ErrValidation)
		}
		for _, call := range turn.ToolCalls {
			result.ToolsUsed = append(result.ToolsUsed, call.Name)
			out, invokeErr := r.Invoke(svc, call.Name, call.Arguments)
			var payload string
			if invokeErr != nil {
				failures++
				payload = fmt.Sprintf(`{"error":%q}`, invokeErr.Error())
				log.Printf("agent chat: tool %s failed: %v", call.Name, invokeErr)
			} else {
				failures = 0
				payload = encodeToolResult(out)
			}
			history = append(history, openai.ToolResult(call.CallID, payload))
		}
		if failures >= maxToolFailures {
			return ChatResult{}, nil, fmt.Errorf("%w: I couldn't complete that (repeated tool errors) — try rephrasing", services.ErrValidation)
		}
	}
}

// toolDefs maps the registry to function tools (sorted by name, like Specs).
func (r *Registry) toolDefs() []openai.ToolDef {
	specs := r.Specs()
	out := make([]openai.ToolDef, 0, len(specs))
	for _, s := range specs {
		out = append(out, openai.ToolDef{Name: s.Name, Description: s.Description, Parameters: s.InputSchema})
	}
	return out
}

// encodeToolResult serializes a tool result for the model, truncating huge
// payloads (dashboards, long lists) so a chat turn stays cheap.
func encodeToolResult(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "unserializable result: "+err.Error())
	}
	if len(b) > maxToolResultSize {
		return string(b[:maxToolResultSize]) + fmt.Sprintf(`… [truncated, %d bytes total]`, len(b))
	}
	return string(b)
}

// trimHistory drops the oldest items but never splits a function_call from
// its function_call_output: it cuts at the first user message that keeps the
// tail within max.
func trimHistory(h []openai.Item, max int) []openai.Item {
	cut := len(h) - max
	for i := cut; i < len(h); i++ {
		var it struct {
			Type string `json:"type"`
			Role string `json:"role"`
		}
		if json.Unmarshal(h[i], &it) == nil && it.Type == "message" && it.Role == "user" {
			return append([]openai.Item{}, h[i:]...)
		}
	}
	return append([]openai.Item{}, h[cut:]...)
}

// chatInstructions builds the system prompt from live state: who the player
// is, their attribute keys (the model must use these exactly) and the rules
// of engagement.
func (r *Registry) chatInstructions(svc *services.Service) (string, error) {
	attrs, err := svc.ListAttributes()
	if err != nil {
		return "", err
	}
	sort.Slice(attrs, func(i, j int) bool { return attrs[i].Key < attrs[j].Key })
	var keys []string
	for _, a := range attrs {
		keys = append(keys, fmt.Sprintf("%s (%s, Lv %d)", a.Key, a.Name, a.Level))
	}
	now := time.Now()
	return fmt.Sprintf(chatInstructionsTemplate, now.Format("Monday 2006-01-02 15:04"), strings.Join(keys, "; ")), nil
}

const chatInstructionsTemplate = `You are "edi", the companion of a life-RPG: the player's REAL life is the campaign. Real-life actions are quests; completing them awards XP to life attributes and gold. You chat with the player over a messenger and act for them through tools.

Now: %s (the player's local time).
Attribute keys (use ONLY these in attribute_rewards): %s

How to act:
- Do things, don't describe them: when the player asks for an action, call the tool, then confirm briefly with the real numbers the tool returned (XP, gold, level-ups, drops).
- "Add X" / "new quest X" / "I want to do X" → create_quest. Choose type (daily habit / weekly ritual / main goal / side extra / boss / recovery) and difficulty from the wording; pick 1-3 attributes it genuinely trains, XP in multiples of 5 scaling with difficulty (trivial ~15, easy ~30, medium ~50, hard ~90, boss ~150+). Ask a question ONLY if the request is truly ambiguous; otherwise make a sensible call and mention it.
- "I did / finished / completed X" → list_quests (status active) first. If an active quest matches (fuzzy: same activity), complete_quest it. If nothing matches, record_spontaneous_quest so the win still counts. Never complete something the player didn't say they did.
- Questions about progress, streak, gold, decay → get_dashboard (and list_quests, list_gold_events…) and answer with the real figures.
- Never invent data, ids or results. Ids come from tool output. If a tool errors, adapt (fix the input) or tell the player plainly.
- Only act on what the player asked; don't fire extra actions (no unrequested completions, purchases, wards, rest mode).

Style: reply in the player's language. Plain text only — no markdown, no HTML, no headings. Short (1-4 lines), warm, a light retro-RPG flavor without cheese. Emoji sparingly.`
