// Package agent exposes the service layer as a registry of named "tools" with
// JSON Schemas. This is the seam where a future AI agent (or an MCP bridge)
// plugs in: every tool simply forwards to the same services.Service the REST API
// uses, so the agent shares one documented data path with all other clients.
package agent

import (
	"encoding/json"
	"fmt"
	"sort"

	"edi/internal/models"
	"edi/internal/services"
)

// Tool is one callable capability.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	handler     func(input json.RawMessage) (any, error)
}

// Spec is the public, handler-free description of a tool (for discovery).
type Spec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Registry holds all tools backed by a single Service.
type Registry struct {
	svc   *services.Service
	tools []Tool
	index map[string]int
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

const emptySchema = `{"type":"object","properties":{},"additionalProperties":false}`

// NewRegistry wires every service method to a tool definition.
func NewRegistry(svc *services.Service) *Registry {
	r := &Registry{svc: svc, index: map[string]int{}}

	add := func(name, desc, schema string, h func(json.RawMessage) (any, error)) {
		r.tools = append(r.tools, Tool{Name: name, Description: desc, InputSchema: raw(schema), handler: h})
	}

	add("get_dashboard", "Return the full dashboard: character level, attributes, today's quests, streak, recent XP, recommended quest, and pending suggestions.",
		emptySchema, func(json.RawMessage) (any, error) { return svc.GetDashboard() })

	add("list_quests", "List quests, optionally filtered by type and status.",
		`{"type":"object","properties":{"type":{"type":"string","enum":["daily","weekly","main","side","boss","recovery"]},"status":{"type":"string","enum":["active","completed","skipped","archived"]}}}`,
		func(in json.RawMessage) (any, error) {
			var p struct{ Type, Status string }
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.ListQuests(p.Type, p.Status)
		})

	add("create_quest", "Create a new quest with attribute XP rewards, optionally with bonus-objective subtasks (each with its own bonus rewards, awarded only if checked before completion).",
		`{"type":"object","required":["title"],"properties":{"title":{"type":"string"},"description":{"type":"string"},"type":{"type":"string","enum":["daily","weekly","main","side","boss","recovery"]},"difficulty":{"type":"string","enum":["trivial","easy","medium","hard","boss"]},"attribute_rewards":{"type":"object","additionalProperties":{"type":"integer"}},"subtasks":{"type":"array","items":{"type":"object","required":["title"],"properties":{"title":{"type":"string"},"attribute_rewards":{"type":"object","additionalProperties":{"type":"integer"}}}}}}}`,
		func(in json.RawMessage) (any, error) {
			var p models.QuestInput
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.CreateQuest(p)
		})

	add("toggle_subtask", "Toggle a quest subtask (bonus objective) done/undone. Checked subtasks add their bonus rewards when the quest is completed.",
		`{"type":"object","required":["quest_id","subtask_id"],"properties":{"quest_id":{"type":"integer"},"subtask_id":{"type":"integer"}}}`,
		func(in json.RawMessage) (any, error) {
			var p struct {
				QuestID   int64 `json:"quest_id"`
				SubtaskID int64 `json:"subtask_id"`
			}
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			if p.QuestID == 0 || p.SubtaskID == 0 {
				return nil, fmt.Errorf("%w: quest_id and subtask_id are required", services.ErrValidation)
			}
			return svc.ToggleSubtask(p.QuestID, p.SubtaskID)
		})

	add("update_quest", "Update fields of an existing quest by id.",
		`{"type":"object","required":["id"],"properties":{"id":{"type":"integer"},"title":{"type":"string"},"description":{"type":"string"},"type":{"type":"string"},"difficulty":{"type":"string"},"status":{"type":"string"},"attribute_rewards":{"type":"object","additionalProperties":{"type":"integer"}}}}`,
		func(in json.RawMessage) (any, error) {
			var p struct {
				ID    int64 `json:"id"`
				Patch models.QuestPatch
			}
			// The patch fields sit at the top level alongside id.
			if err := decode(in, &p.Patch); err != nil {
				return nil, err
			}
			var idHolder struct {
				ID int64 `json:"id"`
			}
			if err := decode(in, &idHolder); err != nil {
				return nil, err
			}
			if idHolder.ID == 0 {
				return nil, fmt.Errorf("id is required")
			}
			return svc.UpdateQuest(idHolder.ID, p.Patch)
		})

	add("complete_quest", "Complete a quest; awards XP, writes audit events, updates the streak, and returns a refreshed dashboard.",
		idSchema("quest_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "quest_id")
			if err != nil {
				return nil, err
			}
			return svc.CompleteQuest(id)
		})

	add("skip_quest", "Skip a quest (increments its skip counter).",
		idSchema("quest_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "quest_id")
			if err != nil {
				return nil, err
			}
			return svc.SkipQuest(id)
		})

	add("archive_quest", "Archive a quest so it no longer appears in active lists.",
		idSchema("quest_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "quest_id")
			if err != nil {
				return nil, err
			}
			return svc.ArchiveQuest(id)
		})

	add("create_journal_entry", "Record a daily reflection with mood and energy (1-10) and free-text notes.",
		`{"type":"object","required":["mood","energy"],"properties":{"mood":{"type":"integer","minimum":1,"maximum":10},"energy":{"type":"integer","minimum":1,"maximum":10},"notes":{"type":"string"}}}`,
		func(in json.RawMessage) (any, error) {
			var p models.JournalInput
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.CreateJournalEntry(p)
		})

	add("list_journal_entries", "List recent journal reflections.",
		`{"type":"object","properties":{"limit":{"type":"integer"}}}`,
		func(in json.RawMessage) (any, error) {
			var p struct {
				Limit int `json:"limit"`
			}
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.ListJournalEntries(p.Limit, "")
		})

	add("get_weakest_attribute", "Return the attribute with the least total XP (useful for choosing what to train next).",
		emptySchema, func(json.RawMessage) (any, error) { return svc.GetWeakestAttribute() })

	add("generate_suggestions", "Run the rule-based engine and return pending suggestions.",
		emptySchema, func(json.RawMessage) (any, error) { return svc.GenerateSuggestions() })

	add("accept_suggestion", "Accept a suggestion, creating a real quest from it.",
		idSchema("suggestion_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "suggestion_id")
			if err != nil {
				return nil, err
			}
			return svc.AcceptSuggestion(id)
		})

	add("dismiss_suggestion", "Dismiss a suggestion without creating a quest.",
		idSchema("suggestion_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "suggestion_id")
			if err != nil {
				return nil, err
			}
			return svc.DismissSuggestion(id)
		})

	add("list_shop_items", "List the reward shop: self-defined real-life rewards purchasable with gold.",
		emptySchema, func(json.RawMessage) (any, error) { return svc.ListShopItems() })

	add("create_shop_item", "Add a reward to the shop (a real-life indulgence with a gold price).",
		`{"type":"object","required":["name","price"],"properties":{"name":{"type":"string"},"price":{"type":"integer","minimum":1}}}`,
		func(in json.RawMessage) (any, error) {
			var p models.ShopItemInput
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.CreateShopItem(p)
		})

	add("update_shop_item", "Update the name or price of an active shop item.",
		`{"type":"object","required":["item_id"],"properties":{"item_id":{"type":"integer"},"name":{"type":"string"},"price":{"type":"integer","minimum":1}}}`,
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "item_id")
			if err != nil {
				return nil, err
			}
			var p models.ShopItemPatch
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.UpdateShopItem(id, p)
		})

	add("archive_shop_item", "Archive a shop item so it no longer appears in the shop (purchase history keeps its label).",
		idSchema("item_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "item_id")
			if err != nil {
				return nil, err
			}
			if err := svc.ArchiveShopItem(id); err != nil {
				return nil, err
			}
			return map[string]bool{"archived": true}, nil
		})

	add("purchase_shop_item", "Spend gold to buy a reward from the shop. Fails with a validation error if the balance is too low.",
		idSchema("item_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "item_id")
			if err != nil {
				return nil, err
			}
			return svc.PurchaseShopItem(id)
		})

	add("list_gold_events", "List recent gold ledger entries (mints and purchases). The balance is SUM(amount) and also appears on the dashboard as gold_balance. Optionally filter to one source (e.g. \"purchase\") to see history for that source without mints crowding it out.",
		`{"type":"object","properties":{"limit":{"type":"integer"},"source":{"type":"string"}}}`,
		func(in json.RawMessage) (any, error) {
			var p struct {
				Limit  int    `json:"limit"`
				Source string `json:"source"`
			}
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.ListGoldEvents(p.Limit, p.Source)
		})

	for i, t := range r.tools {
		r.index[t.Name] = i
	}
	return r
}

// Specs returns the discoverable tool list (sorted by name).
func (r *Registry) Specs() []Spec {
	out := make([]Spec, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, Spec{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Invoke runs a tool by name with raw JSON input.
func (r *Registry) Invoke(name string, input json.RawMessage) (any, error) {
	i, ok := r.index[name]
	if !ok {
		return nil, fmt.Errorf("%w: unknown tool %q", services.ErrNotFound, name)
	}
	return r.tools[i].handler(input)
}

// --- decode helpers ---------------------------------------------------------

func decode(in json.RawMessage, dst any) error {
	if len(in) == 0 || string(in) == "null" {
		return nil
	}
	return json.Unmarshal(in, dst)
}

func idSchema(field string) string {
	return fmt.Sprintf(`{"type":"object","required":["%s"],"properties":{"%s":{"type":"integer"}}}`, field, field)
}

func decodeID(in json.RawMessage, field string) (int64, error) {
	m := map[string]json.RawMessage{}
	if err := decode(in, &m); err != nil {
		return 0, err
	}
	rawID, ok := m[field]
	if !ok {
		// also accept "id"
		rawID, ok = m["id"]
	}
	if !ok {
		return 0, fmt.Errorf("%w: %s is required", services.ErrValidation, field)
	}
	var id int64
	if err := json.Unmarshal(rawID, &id); err != nil {
		return 0, err
	}
	return id, nil
}
