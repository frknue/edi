package services

import (
	"encoding/json"
	"errors"

	"edi/internal/models"
	"edi/internal/tools"
)

// ListTools returns the available guided instruments.
func (s *Service) ListTools() []models.ToolDefinition {
	return s.tools.Definitions()
}

// CompleteTool validates a tool payload, records the completion, and awards XP
// (auditable, with the reward overlay data + refreshed dashboard).
func (s *Service) CompleteTool(key string, payload json.RawMessage) (models.ToolCompletionResult, error) {
	tool := s.tools.Get(key)
	if tool == nil {
		return models.ToolCompletionResult{}, ErrNotFound
	}
	clean, summary, err := tool.Validate([]byte(payload))
	if err != nil {
		if errors.Is(err, tools.ErrInvalid) {
			return models.ToolCompletionResult{}, validationErr("%s", err.Error())
		}
		return models.ToolCompletionResult{}, err
	}

	def := tool.Definition()
	entry, events, levelUps, gold, err := s.store.CompleteTool(s.userID, def.Key, def.Name, clean, summary, def.AttributeRewards)
	if err != nil {
		return models.ToolCompletionResult{}, err
	}
	dash, err := s.GetDashboard()
	if err != nil {
		return models.ToolCompletionResult{}, err
	}
	return models.ToolCompletionResult{
		Entry:     entry,
		XPEvents:  orEmpty(events),
		LevelUps:  orEmpty(levelUps),
		Gold:      gold,
		Dashboard: dash,
	}, nil
}

// ListToolEntries returns recent completions of a tool.
func (s *Service) ListToolEntries(key string, limit int) ([]models.ToolEntry, error) {
	if s.tools.Get(key) == nil {
		return nil, ErrNotFound
	}
	entries, err := s.store.ListToolEntries(s.userID, key, limit)
	return orEmpty(entries), err
}
