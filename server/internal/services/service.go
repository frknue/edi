// Package services is the application core: the "tool-like" functions that every
// client (web UI, CLI, mobile, AI agent) calls. There is no hidden data layer —
// the REST handlers and the agent tool registry both delegate here.
package services

import (
	"errors"
	"fmt"
	"sort"

	"edi/internal/db"
	"edi/internal/models"
)

// DailyGoal is the target number of completed quests per day.
const DailyGoal = 5

// ErrValidation is returned for bad client input (mapped to HTTP 400).
var ErrValidation = errors.New("validation error")

// ErrNotFound is returned when an entity does not exist (mapped to HTTP 404).
var ErrNotFound = errors.New("not found")

func validationErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// orEmpty guarantees a non-nil slice so the JSON contract always emits [] (not
// null) for array fields — important for every client (web, CLI, agent, mobile).
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Service bundles the store and the active user. MVP runs in single-user mode.
type Service struct {
	store  *db.Store
	userID int64
}

// New builds a Service for the given user (1 in single-user mode).
func New(store *db.Store, userID int64) *Service {
	return &Service{store: store, userID: userID}
}

var (
	validTypes        = map[string]bool{"daily": true, "weekly": true, "main": true, "side": true, "boss": true, "recovery": true}
	validDifficulties = map[string]bool{"trivial": true, "easy": true, "medium": true, "hard": true, "boss": true}
)

// enrichAttribute fills the derived level/progress fields from TotalXP.
func enrichAttribute(a models.Attribute) models.Attribute {
	lvl, into, forNext, ratio := ProgressForXP(a.TotalXP)
	a.Level = lvl
	a.XPIntoLevel = into
	a.XPForNextLevel = forNext
	a.Progress = ratio
	return a
}

// ListAttributes returns all attributes with derived level/progress.
func (s *Service) ListAttributes() ([]models.Attribute, error) {
	raw, err := s.store.ListAttributes(s.userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.Attribute, 0, len(raw))
	for _, a := range raw {
		out = append(out, enrichAttribute(a))
	}
	return out, nil
}

// GetWeakestAttribute returns the attribute with the least total XP.
func (s *Service) GetWeakestAttribute() (models.Attribute, error) {
	attrs, err := s.ListAttributes()
	if err != nil {
		return models.Attribute{}, err
	}
	if len(attrs) == 0 {
		return models.Attribute{}, ErrNotFound
	}
	weakest := attrs[0]
	for _, a := range attrs[1:] {
		if a.TotalXP < weakest.TotalXP {
			weakest = a
		}
	}
	return weakest, nil
}

// --- quests -----------------------------------------------------------------

func (s *Service) validateQuestInput(in *models.QuestInput) error {
	if in.Title == "" {
		return validationErr("title is required")
	}
	if in.Type == "" {
		in.Type = models.QuestTypeDaily
	}
	if !validTypes[in.Type] {
		return validationErr("invalid type %q", in.Type)
	}
	if in.Difficulty == "" {
		in.Difficulty = "easy"
	}
	if !validDifficulties[in.Difficulty] {
		return validationErr("invalid difficulty %q", in.Difficulty)
	}
	if in.AttributeRewards == nil {
		in.AttributeRewards = map[string]int64{}
	}
	return s.validateRewards(in.AttributeRewards)
}

// validateRewards ensures every reward key is a known attribute and every value
// is non-negative. Shared by CreateQuest, UpdateQuest, and AcceptSuggestion so the
// rules can't drift between paths.
func (s *Service) validateRewards(rewards map[string]int64) error {
	if len(rewards) == 0 {
		return nil
	}
	known, err := s.store.AttributeNames(s.userID)
	if err != nil {
		return err
	}
	for k, v := range rewards {
		if !knownKey(known, k) {
			return validationErr("unknown attribute %q in rewards", k)
		}
		if v < 0 {
			return validationErr("reward for %q must be >= 0", k)
		}
	}
	return nil
}

func knownKey(m map[string]string, k string) bool { _, ok := m[k]; return ok }

// ListQuests returns quests filtered by optional type and status.
func (s *Service) ListQuests(questType, status string) ([]models.Quest, error) {
	if questType != "" && !validTypes[questType] {
		return nil, validationErr("invalid type filter %q", questType)
	}
	quests, err := s.store.ListQuests(s.userID, questType, status)
	return orEmpty(quests), err
}

// CreateQuest validates and persists a new quest.
func (s *Service) CreateQuest(in models.QuestInput) (models.Quest, error) {
	if err := s.validateQuestInput(&in); err != nil {
		return models.Quest{}, err
	}
	return s.store.InsertQuest(s.userID, in, nil)
}

// UpdateQuest applies a partial patch (validating any provided fields).
func (s *Service) UpdateQuest(id int64, p models.QuestPatch) (models.Quest, error) {
	if _, err := s.store.GetQuest(s.userID, id); err != nil {
		return models.Quest{}, ErrNotFound
	}
	if p.Type != nil && !validTypes[*p.Type] {
		return models.Quest{}, validationErr("invalid type %q", *p.Type)
	}
	if p.Difficulty != nil && !validDifficulties[*p.Difficulty] {
		return models.Quest{}, validationErr("invalid difficulty %q", *p.Difficulty)
	}
	if p.Status != nil {
		switch *p.Status {
		case models.StatusActive, models.StatusArchived:
			// allowed via a generic patch (e.g. un-archive)
		case models.StatusCompleted:
			return models.Quest{}, validationErr("use POST /quests/:id/complete to complete a quest")
		case models.StatusSkipped:
			return models.Quest{}, validationErr("use POST /quests/:id/skip to skip a quest")
		default:
			return models.Quest{}, validationErr("invalid status %q", *p.Status)
		}
	}
	if p.AttributeRewards != nil {
		if err := s.validateRewards(*p.AttributeRewards); err != nil {
			return models.Quest{}, err
		}
	}
	return s.store.UpdateQuest(s.userID, id, p)
}

// CompleteQuest completes a quest and returns rich feedback + a refreshed dashboard.
func (s *Service) CompleteQuest(id int64) (models.CompletionResult, error) {
	quest, events, levelUps, err := s.store.CompleteQuest(s.userID, id)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			return models.CompletionResult{}, ErrNotFound
		case errors.Is(err, db.ErrQuestNotCompletable):
			// 400, not 500 — re-completing/double-tapping is a client condition.
			return models.CompletionResult{}, validationErr("%s", err.Error())
		default:
			return models.CompletionResult{}, err
		}
	}
	dash, err := s.GetDashboard()
	if err != nil {
		return models.CompletionResult{}, err
	}
	return models.CompletionResult{
		Quest:     quest,
		XPEvents:  orEmpty(events),
		LevelUps:  orEmpty(levelUps),
		Dashboard: dash,
	}, nil
}

// SkipQuest marks a quest skipped (increments its skip counter).
func (s *Service) SkipQuest(id int64) (models.Quest, error) {
	if _, err := s.store.GetQuest(s.userID, id); err != nil {
		return models.Quest{}, ErrNotFound
	}
	return s.store.SkipQuest(s.userID, id)
}

// ArchiveQuest marks a quest archived.
func (s *Service) ArchiveQuest(id int64) (models.Quest, error) {
	if _, err := s.store.GetQuest(s.userID, id); err != nil {
		return models.Quest{}, ErrNotFound
	}
	if err := s.store.SetQuestStatus(s.userID, id, models.StatusArchived); err != nil {
		return models.Quest{}, err
	}
	return s.store.GetQuest(s.userID, id)
}

// --- xp / journal -----------------------------------------------------------

// ListXPEvents returns the most recent XP audit events.
func (s *Service) ListXPEvents(limit int) ([]models.XPEvent, error) {
	events, err := s.store.ListXPEvents(s.userID, limit)
	return orEmpty(events), err
}

// CreateJournalEntry validates and stores a reflection.
func (s *Service) CreateJournalEntry(in models.JournalInput) (models.JournalEntry, error) {
	if in.Mood < 1 || in.Mood > 10 {
		return models.JournalEntry{}, validationErr("mood must be between 1 and 10")
	}
	if in.Energy < 1 || in.Energy > 10 {
		return models.JournalEntry{}, validationErr("energy must be between 1 and 10")
	}
	return s.store.InsertJournal(s.userID, in)
}

// ListJournalEntries returns recent reflections.
func (s *Service) ListJournalEntries(limit int) ([]models.JournalEntry, error) {
	entries, err := s.store.ListJournal(s.userID, limit)
	return orEmpty(entries), err
}

// --- dashboard --------------------------------------------------------------

// GetDashboard assembles the full main-screen payload in one call.
func (s *Service) GetDashboard() (models.Dashboard, error) {
	user, err := s.store.GetUser(s.userID)
	if err != nil {
		return models.Dashboard{}, err
	}
	attrs, err := s.ListAttributes()
	if err != nil {
		return models.Dashboard{}, err
	}
	todayQuests, err := s.store.ListQuests(s.userID, "", models.StatusActive)
	if err != nil {
		return models.Dashboard{}, err
	}
	streak, err := s.store.GetStreak(s.userID)
	if err != nil {
		return models.Dashboard{}, err
	}
	events, err := s.store.ListXPEvents(s.userID, 12)
	if err != nil {
		return models.Dashboard{}, err
	}
	completedToday, err := s.store.CompletedTodayCount(s.userID)
	if err != nil {
		return models.Dashboard{}, err
	}
	suggestions, err := s.store.ListSuggestions(s.userID, "pending")
	if err != nil {
		return models.Dashboard{}, err
	}

	var totalXP int64
	for _, a := range attrs {
		totalXP += a.TotalXP
	}
	charLevel, into, forNext, ratio := ProgressForXP(totalXP)
	character := models.CharacterSummary{
		Name: user.Name, Level: charLevel, TotalXP: totalXP,
		XPIntoLevel: into, XPForNextLevel: forNext, Progress: ratio,
	}

	goal := DailyGoal
	dailyRatio := float64(completedToday) / float64(goal)
	if dailyRatio > 1 {
		dailyRatio = 1
	}

	recommended := recommendQuest(todayQuests, attrs)

	return models.Dashboard{
		User:             user,
		Character:        character,
		Attributes:       orEmpty(attrs),
		TodayQuests:      orEmpty(todayQuests),
		Streak:           streak,
		RecentXPEvents:   orEmpty(events),
		RecommendedQuest: recommended,
		DailyProgress:    models.DailyProgress{CompletedToday: completedToday, Goal: goal, Ratio: dailyRatio},
		Suggestions:      orEmpty(suggestions),
	}, nil
}

// recommendQuest picks the next useful active quest: prefer the one that best
// rewards the weakest attribute, skipping boss quests (those are deliberate).
func recommendQuest(quests []models.Quest, attrs []models.Attribute) *models.Quest {
	if len(quests) == 0 {
		return nil
	}
	// weakest attribute key
	weakestKey := ""
	var weakestXP int64 = -1
	for _, a := range attrs {
		if weakestXP < 0 || a.TotalXP < weakestXP {
			weakestXP = a.TotalXP
			weakestKey = a.Key
		}
	}
	var best *models.Quest
	var bestReward int64 = -1
	for i := range quests {
		q := quests[i]
		if q.Type == models.QuestTypeBoss {
			continue
		}
		r := q.AttributeRewards[weakestKey]
		if r > bestReward {
			bestReward = r
			best = &quests[i]
		}
	}
	if best != nil && bestReward > 0 {
		return best
	}
	// Fallback: first non-boss active quest, else the first quest.
	for i := range quests {
		if quests[i].Type != models.QuestTypeBoss {
			return &quests[i]
		}
	}
	return &quests[0]
}

// sortAttributesByWeeklyXP is a helper used by the suggestion engine.
func sortAttributesByWeeklyXP(weekly map[string]int64, attrs []models.Attribute) []models.Attribute {
	sorted := make([]models.Attribute, len(attrs))
	copy(sorted, attrs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return weekly[sorted[i].Key] < weekly[sorted[j].Key]
	})
	return sorted
}
