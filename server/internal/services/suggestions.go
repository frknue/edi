package services

import (
	"errors"
	"fmt"
	"time"

	"edi/internal/db"
	"edi/internal/models"
)

// Rule-based agent suggestions. This is intentionally a small, explainable engine
// that lives behind the same service API a future LLM agent would call — swapping
// it for an LLM later requires no client changes.

// ListSuggestions returns suggestions, optionally filtered by status.
func (s *Service) ListSuggestions(status string) ([]models.AgentSuggestion, error) {
	out, err := s.store.ListSuggestions(s.userID, status)
	return orEmpty(out), err
}

// GenerateSuggestions evaluates the rules against recent activity, persists any
// new pending suggestions (deduped by type), and returns the full pending list.
func (s *Service) GenerateSuggestions() ([]models.AgentSuggestion, error) {
	now := time.Now()
	weekAgo := now.Add(-7 * 24 * time.Hour)

	attrs, err := s.ListAttributes()
	if err != nil {
		return nil, err
	}
	weekly, err := s.store.WeeklyXPByAttribute(s.userID, weekAgo)
	if err != nil {
		return nil, err
	}

	var candidates []models.AgentSuggestion

	// Rule 1 — low attribute this week: recommend a quest for the weakest attribute
	// (measured by XP earned in the last 7 days).
	if len(attrs) > 0 {
		ranked := sortAttributesByWeeklyXP(weekly, attrs)
		weakest := ranked[0]
		candidates = append(candidates, models.AgentSuggestion{
			Type:   "low_attribute",
			Title:  fmt.Sprintf("Train %s — it's lagging this week", weakest.Name),
			Reason: fmt.Sprintf("%s earned only %d XP in the last 7 days, your lowest. A small daily quest will rebalance you.", weakest.Name, weekly[weakest.Key]),
			SuggestedQuest: models.QuestInput{
				Title:            fmt.Sprintf("%s boost", weakest.Name),
				Description:      fmt.Sprintf("A focused action to grow %s.", weakest.Name),
				Type:             models.QuestTypeDaily,
				Difficulty:       "easy",
				AttributeRewards: map[string]int64{weakest.Key: 30},
			},
		})
	}

	// Rule 2 — strong recent Focus: if >= 3 distinct Focus quests completed in the
	// last week, suggest a harder Focus challenge.
	focusCount, err := s.store.DistinctQuestsRewardingAttributeSince(s.userID, "focus", weekAgo)
	if err != nil {
		return nil, err
	}
	if focusCount >= 3 {
		candidates = append(candidates, models.AgentSuggestion{
			Type:   "level_up_focus",
			Title:  "Level up: take on a harder Focus quest",
			Reason: fmt.Sprintf("You completed %d Focus quests this week. Time to raise the bar.", focusCount),
			SuggestedQuest: models.QuestInput{
				Title:            "2-hour deep focus block",
				Description:      "One hard problem, two hours, zero context switching.",
				Type:             models.QuestTypeMain,
				Difficulty:       "hard",
				AttributeRewards: map[string]int64{"focus": 90, "discipline": 25},
			},
		})
	}

	// Rule 3 — repeatedly skipped quest: suggest an easier version.
	skipped, err := s.store.QuestsSkippedRepeatedly(s.userID, 2)
	if err != nil {
		return nil, err
	}
	if len(skipped) > 0 {
		q := skipped[0]
		easier := halveRewards(q.AttributeRewards)
		src := q.ID
		candidates = append(candidates, models.AgentSuggestion{
			Type:          "make_easier",
			Title:         fmt.Sprintf("Make \"%s\" easier", q.Title),
			Reason:        fmt.Sprintf("You've skipped this %d times. Shrinking it makes it easy to start.", q.SkipCount),
			SourceQuestID: &src,
			SuggestedQuest: models.QuestInput{
				Title:            "Mini: " + q.Title,
				Description:      "A smaller, friendlier version to rebuild momentum.",
				Type:             models.QuestTypeRecovery,
				Difficulty:       "trivial",
				AttributeRewards: easier,
			},
		})
	}

	// Rule 4 — high sustained activity: suggest a recovery quest to avoid burnout.
	activeDays, err := s.store.ActiveDaysSince(s.userID, now.Add(-4*24*time.Hour))
	if err != nil {
		return nil, err
	}
	recentCompletions, err := s.store.CompletionsSince(s.userID, now.Add(-3*24*time.Hour))
	if err != nil {
		return nil, err
	}
	if activeDays >= 3 || recentCompletions >= 8 {
		candidates = append(candidates, models.AgentSuggestion{
			Type:   "recovery",
			Title:  "Schedule a recovery day",
			Reason: "You've been highly active lately. Intentional recovery protects long-term progress.",
			SuggestedQuest: models.QuestInput{
				Title:            "Recovery walk & stretch",
				Description:      "Gentle movement, no targets. Rest is part of the game.",
				Type:             models.QuestTypeRecovery,
				Difficulty:       "trivial",
				AttributeRewards: map[string]int64{"health": 20, "spirituality": 15},
			},
		})
	}

	// Persist new candidates, skipping types that already have a pending suggestion.
	for _, c := range candidates {
		exists, err := s.store.HasPendingSuggestionOfType(s.userID, c.Type, c.SourceQuestID)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		if _, err := s.store.InsertSuggestion(s.userID, c); err != nil {
			return nil, err
		}
	}

	out, err := s.store.ListSuggestions(s.userID, "pending")
	return orEmpty(out), err
}

// AcceptSuggestion turns a pending suggestion into a real quest and marks it accepted.
func (s *Service) AcceptSuggestion(id int64) (models.Quest, error) {
	sug, err := s.store.GetSuggestion(s.userID, id)
	if err != nil {
		return models.Quest{}, ErrNotFound
	}
	if sug.Status != "pending" {
		return models.Quest{}, validationErr("suggestion already %s", sug.Status)
	}
	in := sug.SuggestedQuest
	if err := s.validateQuestInput(&in); err != nil {
		return models.Quest{}, err
	}
	quest, err := s.store.AcceptSuggestion(s.userID, id, in)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			return models.Quest{}, ErrNotFound
		case errors.Is(err, db.ErrSuggestionNotPending):
			return models.Quest{}, validationErr("suggestion already resolved")
		default:
			return models.Quest{}, err
		}
	}
	return quest, nil
}

// DismissSuggestion marks a pending suggestion dismissed.
func (s *Service) DismissSuggestion(id int64) (models.AgentSuggestion, error) {
	sug, err := s.store.GetSuggestion(s.userID, id)
	if err != nil {
		return models.AgentSuggestion{}, ErrNotFound
	}
	if sug.Status != "pending" {
		return models.AgentSuggestion{}, validationErr("suggestion already %s", sug.Status)
	}
	if err := s.store.ResolveSuggestion(s.userID, id, "dismissed", nil); err != nil {
		return models.AgentSuggestion{}, err
	}
	return s.store.GetSuggestion(s.userID, id)
}

func halveRewards(r map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for k, v := range r {
		nv := v / 2
		if nv < 5 {
			nv = 5
		}
		out[k] = nv
	}
	if len(out) == 0 {
		out["discipline"] = 10
	}
	return out
}
