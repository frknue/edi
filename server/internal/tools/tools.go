// Package tools defines "tools" — guided instruments that award XP when
// completed (e.g. the Daily Mood Log). Each tool validates its own payload and
// returns a short human summary; XP rewards come from the definition. New tools
// implement the Tool interface and register in Registry.
package tools

import "edi/internal/models"

// Tool is one guided instrument.
type Tool interface {
	Definition() models.ToolDefinition
	// Validate checks the raw payload, returns a cleaned/normalized version and a
	// one-line summary for the history feed. It must return a services-mappable
	// validation error on bad input.
	Validate(payload []byte) (clean []byte, summary string, err error)
}

// Registry holds the available tools in display order.
type Registry struct {
	order []string
	byKey map[string]Tool
}

// NewRegistry builds the default tool set.
func NewRegistry() *Registry {
	r := &Registry{byKey: map[string]Tool{}}
	r.add(DailyMoodLog{})
	return r
}

func (r *Registry) add(t Tool) {
	key := t.Definition().Key
	r.order = append(r.order, key)
	r.byKey[key] = t
}

// Definitions returns all tool definitions in display order.
func (r *Registry) Definitions() []models.ToolDefinition {
	out := make([]models.ToolDefinition, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.byKey[k].Definition())
	}
	return out
}

// Get returns a tool by key (nil if unknown).
func (r *Registry) Get(key string) Tool {
	return r.byKey[key]
}
