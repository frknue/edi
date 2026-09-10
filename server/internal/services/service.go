// Package services is the application core: the "tool-like" functions that every
// client (web UI, CLI, mobile, AI agent) calls. There is no hidden data layer —
// the REST handlers and the agent tool registry both delegate here.
package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"edi/internal/db"
	"edi/internal/models"
	"edi/internal/tools"
)

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
	tools  *tools.Registry

	// Process-wide runtimes shared by every ForUser copy (pointers, so the
	// per-request shallow copy stays vet-clean): the OpenAI "Sign in with
	// ChatGPT" connect state (openai.go) and Telegram pairing (telegram.go).
	oauth    *oauthRuntime
	telegram *telegramRuntime
	hooks    *hookRuntime

	// completer, when set, replaces the live ChatGPT call — the offline test
	// seam for every JSON-parsing AI path (boss phases, break-down, shrink,
	// narration). Nil in production.
	completer func(instructions, prompt string) (string, error)
}

// SetCompleterForTest injects a canned model (tests only).
func (s *Service) SetCompleterForTest(fn func(instructions, prompt string) (string, error)) {
	s.completer = fn
}

// New builds a Service bound to the given user (the dev-fallback user 1 for
// the base service; per-request services come from ForUser).
func New(store *db.Store, userID int64) *Service {
	return &Service{store: store, userID: userID, tools: tools.NewRegistry(), oauth: &oauthRuntime{}, telegram: &telegramRuntime{}, hooks: &hookRuntime{}}
}

// UserID returns the user this service copy is bound to.
func (s *Service) UserID() int64 { return s.userID }

// ForUser returns a shallow copy of the service bound to another user. Cheap
// (per-request) by design: the store pool, tool registry, and OAuth runtime
// are shared pointers; only the user binding changes.
func (s *Service) ForUser(userID int64) *Service {
	c := *s
	c.userID = userID
	return &c
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
	if _, err := s.ApplyDecay(); err != nil {
		return nil, err
	}
	raw, err := s.store.ListAttributes(s.userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.Attribute, 0, len(raw))
	for _, a := range raw {
		out = append(out, enrichAttribute(a))
	}

	// Decay status is a hardcore-only surface: outside hardcore every
	// attribute reports no decay at all (nil), so no client renders rust.
	hardcore, err := s.hardcoreOn()
	if err != nil {
		return nil, err
	}
	if !hardcore {
		return out, nil
	}
	rest, err := s.RestState()
	if err != nil {
		return nil, err
	}
	floor, err := s.idleAnchorFloor()
	if err != nil {
		return nil, err
	}
	inputs, err := s.store.DecayInputs(s.userID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Decay = decayStatus(out[i], inputs[out[i].Key], rest, floor, time.Now().UTC())
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
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
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
	if err := s.validateRewards(in.AttributeRewards); err != nil {
		return err
	}
	if err := validateTrigger(&in.Trigger, &in.TriggerAt); err != nil {
		return err
	}
	return s.validateSubtasks(in.Subtasks)
}

// validateSubtasks checks bonus-objective titles and reward maps.
func (s *Service) validateSubtasks(subtasks []models.SubtaskInput) error {
	if len(subtasks) > 10 {
		return validationErr("a quest can have at most 10 subtasks")
	}
	for i, st := range subtasks {
		if strings.TrimSpace(st.Title) == "" {
			return validationErr("subtask %d needs a title", i+1)
		}
		if err := s.validateRewards(st.AttributeRewards); err != nil {
			return err
		}
	}
	return nil
}

// ToggleSubtask flips a bonus objective's done state (only while the quest is
// still completable).
func (s *Service) ToggleSubtask(questID, subtaskID int64) (models.Subtask, error) {
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.Subtask{}, err
	}
	visible, visibleErr := s.store.GetVisibleQuest(s.userID, questID)
	if visibleErr != nil && !errors.Is(visibleErr, db.ErrNotFound) {
		return models.Subtask{}, visibleErr
	}
	if visibleErr == nil && visible.SharedQuestID != nil {
		if !visible.AssignedToMe {
			return models.Subtask{}, validationErr("this quest is assigned to another board member")
		}
		questID = visible.ID
	}
	st, err := s.store.ToggleSubtask(s.userID, questID, subtaskID)
	switch {
	case errors.Is(err, db.ErrNotFound):
		return models.Subtask{}, ErrNotFound
	case errors.Is(err, db.ErrQuestNotCompletable):
		return models.Subtask{}, validationErr("this quest is already completed or archived")
	}
	return st, err
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

func (s *Service) rollOverRecurringQuests() error {
	rest, err := s.RestState()
	if err != nil {
		return err
	}
	var restSince *time.Time
	if rest.On {
		restSince = rest.Since
	}
	hardcore, err := s.hardcoreOn()
	if err != nil {
		return err
	}
	// Outside hardcore the stakes cursor still advances (so switching hardcore
	// on later never bills the past) but no XP is removed.
	_, err = s.store.RollOverRecurringQuests(s.userID, time.Now(), restSince, hardcore)
	return err
}

// ListQuests returns quests filtered by optional type and status.
func (s *Service) ListQuests(questType, status string) ([]models.Quest, error) {
	if questType != "" && !validTypes[questType] {
		return nil, validationErr("invalid type filter %q", questType)
	}
	if err := s.rollOverRecurringQuests(); err != nil {
		return nil, err
	}
	quests, err := s.store.ListVisibleQuests(s.userID, questType, status)
	return orEmpty(quests), err
}

// CreateQuest validates and persists a new quest.
func (s *Service) CreateQuest(in models.QuestInput) (models.Quest, error) {
	if err := s.validateQuestInput(&in); err != nil {
		return models.Quest{}, err
	}
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.Quest{}, err
	}
	if in.AssigneeIDs != nil {
		assignees, err := s.validateSharedAssignees(in.AssigneeIDs)
		if err != nil {
			return models.Quest{}, err
		}
		return s.store.InsertSharedQuest(s.userID, in, assignees)
	}
	return s.store.InsertQuest(s.userID, in, nil)
}

// UpdateQuest applies a partial patch (validating any provided fields).
func (s *Service) UpdateQuest(id int64, p models.QuestPatch) (models.Quest, error) {
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.Quest{}, err
	}
	shared, sharedErr := s.store.SharedQuestAccessForViewer(s.userID, id)
	isShared := sharedErr == nil
	if sharedErr != nil && !errors.Is(sharedErr, db.ErrNotFound) {
		return models.Quest{}, sharedErr
	}
	if !isShared {
		if _, err := s.store.GetQuest(s.userID, id); err != nil {
			return models.Quest{}, ErrNotFound
		}
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
	if p.Subtasks != nil {
		if err := s.validateSubtasks(*p.Subtasks); err != nil {
			return models.Quest{}, err
		}
	}
	if err := validateTrigger(p.Trigger, p.TriggerAt); err != nil {
		return models.Quest{}, err
	}
	if !isShared {
		return s.store.UpdateQuest(s.userID, id, p)
	}
	// Triggers are personal cues: on a shared quest they live on the caller's copy.
	if p.Trigger != nil || p.TriggerAt != nil {
		if _, err := s.store.UpdateQuest(s.userID, id, models.QuestPatch{Trigger: p.Trigger, TriggerAt: p.TriggerAt}); err != nil {
			return models.Quest{}, err
		}
	}
	contentChange := p.Title != nil || p.Description != nil || p.Type != nil || p.Difficulty != nil || p.AttributeRewards != nil || p.Subtasks != nil || p.DueDate != nil
	if contentChange {
		if err := s.store.UpdateSharedQuest(s.userID, id, p); errors.Is(err, db.ErrQuestNotCompletable) {
			return models.Quest{}, validationErr("a shared quest cannot be edited after one member completes it")
		} else if err != nil {
			return models.Quest{}, err
		}
	}
	if p.Status != nil && *p.Status == models.StatusActive {
		if !shared.Assigned {
			return models.Quest{}, validationErr("this quest is assigned to another board member")
		}
		if err := s.store.RestoreSharedQuestAssignment(s.userID, id); err != nil {
			return models.Quest{}, err
		}
	}
	if p.Status != nil && *p.Status == models.StatusArchived {
		if err := s.store.ArchiveSharedQuest(s.userID, id); err != nil {
			return models.Quest{}, err
		}
	}
	return s.store.GetVisibleQuest(s.userID, id)
}

// CompleteQuest completes a quest and returns rich feedback + a refreshed dashboard.
func (s *Service) CompleteQuest(id int64) (models.CompletionResult, error) {
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.CompletionResult{}, err
	}
	if _, err := s.ApplyDecay(); err != nil {
		return models.CompletionResult{}, err
	}
	visible, visibleErr := s.store.GetVisibleQuest(s.userID, id)
	if visibleErr != nil {
		return models.CompletionResult{}, ErrNotFound
	}
	if visible.SharedQuestID != nil {
		if !visible.AssignedToMe {
			return models.CompletionResult{}, validationErr("this quest is assigned to another board member")
		}
		if visible.MyStatus != models.StatusActive {
			return models.CompletionResult{}, validationErr("your part of this quest is not active")
		}
		id = visible.ID
	}
	quest, events, levelUps, gold, outcome, err := s.store.CompleteQuest(s.userID, id)
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
	if quest.SharedQuestID != nil {
		quest, err = s.store.GetVisibleQuest(s.userID, quest.ID)
		if err != nil {
			return models.CompletionResult{}, err
		}
	}
	return s.completionResult(quest, events, levelUps, gold, outcome)
}

// RecordSpontaneousQuest records something worthwhile the user already did.
// Creation and completion share one store transaction, so it never appears as
// an unfinished quest and it receives exactly the same auditable game rewards
// as any planned quest.
func (s *Service) RecordSpontaneousQuest(in models.QuestInput) (models.CompletionResult, error) {
	if in.Type == "" {
		in.Type = "side"
	}
	if len(in.Subtasks) > 0 {
		return models.CompletionResult{}, validationErr("spontaneous quests cannot have bonus objectives")
	}
	if in.AssigneeIDs != nil {
		return models.CompletionResult{}, validationErr("spontaneous wins are personal; create a shared quest before completing it")
	}
	if err := s.validateQuestInput(&in); err != nil {
		return models.CompletionResult{}, err
	}
	if _, err := s.ApplyDecay(); err != nil {
		return models.CompletionResult{}, err
	}
	quest, events, levelUps, gold, outcome, err := s.store.RecordSpontaneousQuest(s.userID, in)
	if err != nil {
		return models.CompletionResult{}, err
	}
	return s.completionResult(quest, events, levelUps, gold, outcome)
}

func (s *Service) completionResult(quest models.Quest, events []models.XPEvent, levelUps []models.LevelUp, gold int64, outcome db.CompletionOutcome) (models.CompletionResult, error) {
	// Post-commit, best-effort: badges must never fail a completion.
	unlocked := s.evaluateAchievements()

	dash, err := s.GetDashboard()
	if err != nil {
		return models.CompletionResult{}, err
	}
	return models.CompletionResult{
		Quest:                quest,
		XPEvents:             orEmpty(events),
		LevelUps:             orEmpty(levelUps),
		Gold:                 gold,
		Crit:                 outcome.Crit,
		ComboMultiplier:      outcome.ComboMultiplier,
		Drop:                 outcome.Drop,
		BoardClear:           outcome.BoardClear,
		AchievementsUnlocked: orEmpty(unlocked),
		Dashboard:            dash,
	}, nil
}

// SkipQuest marks a quest skipped (increments its skip counter).
func (s *Service) SkipQuest(id int64) (models.Quest, error) {
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.Quest{}, err
	}
	visible, err := s.store.GetVisibleQuest(s.userID, id)
	if err != nil {
		return models.Quest{}, ErrNotFound
	}
	if visible.SharedQuestID != nil {
		if !visible.AssignedToMe {
			return models.Quest{}, validationErr("this quest is assigned to another board member")
		}
		id = visible.ID
		if err := s.store.SkipSharedQuestAssignment(s.userID, id); errors.Is(err, db.ErrQuestNotCompletable) {
			return models.Quest{}, validationErr("your part of this quest is not active")
		} else if err != nil {
			return models.Quest{}, err
		}
		return s.store.GetVisibleQuest(s.userID, id)
	}
	if _, err := s.store.SkipQuest(s.userID, id); err != nil {
		return models.Quest{}, err
	}
	if visible.SharedQuestID != nil {
		return s.store.GetVisibleQuest(s.userID, id)
	}
	return s.store.GetQuest(s.userID, id)
}

// ArchiveQuest marks a quest archived.
func (s *Service) ArchiveQuest(id int64) (models.Quest, error) {
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.Quest{}, err
	}
	visible, err := s.store.GetVisibleQuest(s.userID, id)
	if err != nil {
		return models.Quest{}, ErrNotFound
	}
	if visible.SharedQuestID != nil {
		if err := s.store.ArchiveSharedQuest(s.userID, id); err != nil {
			return models.Quest{}, err
		}
		return s.store.GetVisibleQuest(s.userID, id)
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

// GoldBalance returns the spendable gold balance (SUM of the ledger, computed
// on read — same audit pattern as XP).
func (s *Service) GoldBalance() (int64, error) {
	return s.store.GoldBalance(s.userID)
}

// ListGoldEvents returns the most recent gold ledger rows. When source is
// non-empty (e.g. "purchase"), only events of that source are returned.
func (s *Service) ListGoldEvents(limit int, source string) ([]models.GoldEvent, error) {
	events, err := s.store.ListGoldEvents(s.userID, limit, source)
	return orEmpty(events), err
}

// journalDailyRewards is the XP for the first reflection of each local day —
// the habit of showing up, not volume (later entries the same day award nothing).
var journalDailyRewards = map[string]int64{"spirituality": 10, "discipline": 5}

func validMoodEnergy(mood, energy int) error {
	if mood < 1 || mood > 10 {
		return validationErr("mood must be between 1 and 10")
	}
	if energy < 1 || energy > 10 {
		return validationErr("energy must be between 1 and 10")
	}
	return nil
}

// CreateJournalEntry validates and stores a reflection; the first entry of the
// day awards XP (auditable, same path as quests/tools).
func (s *Service) CreateJournalEntry(in models.JournalInput) (models.JournalCreateResult, error) {
	if err := validMoodEnergy(in.Mood, in.Energy); err != nil {
		return models.JournalCreateResult{}, err
	}
	if _, err := s.ApplyDecay(); err != nil {
		return models.JournalCreateResult{}, err
	}
	entry, events, levelUps, gold, err := s.store.InsertJournal(s.userID, in, journalDailyRewards)
	if err != nil {
		return models.JournalCreateResult{}, err
	}
	return models.JournalCreateResult{Entry: entry, XPEvents: orEmpty(events), LevelUps: orEmpty(levelUps), Gold: gold}, nil
}

// UpdateJournalEntry applies a partial patch to a reflection.
func (s *Service) UpdateJournalEntry(id int64, p models.JournalPatch) (models.JournalEntry, error) {
	if p.Mood != nil || p.Energy != nil {
		mood, energy := 5, 5
		if p.Mood != nil {
			mood = *p.Mood
		}
		if p.Energy != nil {
			energy = *p.Energy
		}
		if err := validMoodEnergy(mood, energy); err != nil {
			return models.JournalEntry{}, err
		}
	}
	entry, err := s.store.UpdateJournal(s.userID, id, p)
	if errors.Is(err, db.ErrNotFound) {
		return models.JournalEntry{}, ErrNotFound
	}
	return entry, err
}

// DeleteJournalEntry removes a reflection (already-awarded XP stays — the audit
// log is immutable).
func (s *Service) DeleteJournalEntry(id int64) error {
	err := s.store.DeleteJournal(s.userID, id)
	if errors.Is(err, db.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// ListJournalEntries returns recent reflections, optionally filtered by a
// full-text search over notes.
func (s *Service) ListJournalEntries(limit int, search string) ([]models.JournalEntry, error) {
	entries, err := s.store.ListJournal(s.userID, limit, search)
	return orEmpty(entries), err
}

// --- dashboard --------------------------------------------------------------

// GetDashboard assembles the full main-screen payload in one call.
func (s *Service) GetDashboard() (models.Dashboard, error) {
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.Dashboard{}, err
	}
	decayed, err := s.ApplyDecay()
	if err != nil {
		return models.Dashboard{}, err
	}
	user, err := s.store.GetUser(s.userID)
	if err != nil {
		return models.Dashboard{}, err
	}
	attrs, err := s.ListAttributes()
	if err != nil {
		return models.Dashboard{}, err
	}
	visibleQuests, err := s.store.ListVisibleQuests(s.userID, "", models.StatusActive)
	if err != nil {
		return models.Dashboard{}, err
	}
	todayQuests := make([]models.Quest, 0, len(visibleQuests))
	for _, q := range visibleQuests {
		if q.AssignedToMe && q.MyStatus == models.StatusActive {
			todayQuests = append(todayQuests, q)
		}
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
	buffs, err := s.store.ActiveBuffs(s.userID, time.Now().UTC())
	if err != nil {
		return models.Dashboard{}, err
	}
	goldBalance, err := s.store.GoldBalance(s.userID)
	if err != nil {
		return models.Dashboard{}, err
	}
	rest, err := s.RestState()
	if err != nil {
		return models.Dashboard{}, err
	}
	hardcore, err := s.hardcoreOn()
	if err != nil {
		return models.Dashboard{}, err
	}
	dailyPenaltyXP, err := s.store.DailyQuestPenaltyToday(s.userID, time.Now())
	if err != nil {
		return models.Dashboard{}, err
	}
	dailiesToday, err := s.store.DailyQuestCountToday(s.userID, time.Now())
	if err != nil {
		return models.Dashboard{}, err
	}
	activeDays, err := s.activeDayStrip(activeDayStripLen, time.Now())
	if err != nil {
		return models.Dashboard{}, err
	}
	session, err := s.ActiveSession()
	if err != nil {
		return models.Dashboard{}, err
	}
	dailiesDone, err := s.store.DailiesDoneToday(s.userID, time.Now())
	if err != nil {
		return models.Dashboard{}, err
	}
	boardClearToday, err := s.store.BoardClearPaidToday(s.userID, time.Now())
	if err != nil {
		return models.Dashboard{}, err
	}
	xpToday, err := s.store.XPToday(s.userID, time.Now())
	if err != nil {
		return models.Dashboard{}, err
	}
	pity, err := s.store.LootPity(s.userID)
	if err != nil {
		return models.Dashboard{}, err
	}
	firstMove, err := s.firstMove(time.Now())
	if err != nil {
		return models.Dashboard{}, err
	}
	partner, err := s.partnerSession()
	if err != nil {
		return models.Dashboard{}, err
	}
	chapters, err := s.store.ListStoryChapters(s.userID, 1)
	if err != nil {
		return models.Dashboard{}, err
	}
	var latest *models.StoryChapter
	if len(chapters) > 0 {
		latest = &chapters[0]
	}
	loadout, err := s.Loadout()
	if err != nil {
		return models.Dashboard{}, err
	}
	gearGoal, err := s.GearGoalSummary(LevelForXP(func() int64 {
		var t int64
		for _, a := range attrs {
			t += a.TotalXP
		}
		return t
	}()), goldBalance)
	if err != nil {
		return models.Dashboard{}, err
	}

	var totalXP int64
	for _, a := range attrs {
		totalXP += a.TotalXP
	}
	charLevel, into, forNext, ratio := ProgressForXP(totalXP)
	character := models.CharacterSummary{
		Name: user.Name, Title: s.characterTitle(), Level: charLevel, TotalXP: totalXP,
		XPIntoLevel: into, XPForNextLevel: forNext, Progress: ratio,
	}

	goal := dailyGoal(dailiesToday)
	// With no dailies on the board, any completion closes the day.
	setDone := dailiesDone
	if dailiesToday == 0 {
		setDone = completedToday
	}
	dailyRatio := float64(setDone) / float64(goal)
	if dailyRatio > 1 {
		dailyRatio = 1
	}
	cleared := setDone >= goal
	dayState := "open"
	if cleared {
		dayState = "camp"
	}

	// Live payout on every card, then the recommendation from the same numbers.
	nth := completedToday + 1
	for i := range todayQuests {
		todayQuests[i].ProjectedXP = ProjectPayout(todayQuests[i], nth, buffs)
	}
	var firstMoveID int64
	if firstMove != nil && firstMove.Day == localDate(time.Now()).Format("2006-01-02") {
		firstMoveID = firstMove.Quest.ID
	}
	recommended, reason := recommendQuest(todayQuests, attrs, buffs, nth, firstMoveID)
	if recommended != nil {
		recommended.RecommendReason = reason
	}

	return models.Dashboard{
		User:             user,
		Character:        character,
		Attributes:       orEmpty(attrs),
		TodayQuests:      orEmpty(todayQuests),
		Streak:           streak,
		GoldBalance:      goldBalance,
		RestMode:         rest.On,
		RestSince:        rest.Since,
		Hardcore:         hardcore,
		DailyPenaltyXP:   dailyPenaltyXP,
		ActiveDays:       activeDays,
		ActiveSession:    session,
		RecentXPEvents:   orEmpty(events),
		RecommendedQuest: recommended,
		DailyProgress:    models.DailyProgress{CompletedToday: completedToday, Goal: goal, DailiesDone: setDone, Cleared: cleared, Ratio: dailyRatio, NextComboMultiplier: ComboMultiplier(completedToday + 1)},
		DayState:         dayState,
		XPToday:          xpToday,
		BoardClearToday:  boardClearToday,
		LootPity:         pity,
		FirstMove:        firstMove,
		PartnerSession:   partner,
		LatestChapter:    latest,
		Suggestions:      orEmpty(suggestions),
		ActiveBuffs:      orEmpty(buffs),
		Loadout:          orEmpty(loadout),
		GearGoal:         gearGoal,
		DecayedToday:     decayed,
	}, nil
}

// activeDayStripLen is how many local days the dashboard activity strip
// covers (the headline replaces the streak counter: "days you showed up").
const activeDayStripLen = 14

// dailyGoal is today's target: the daily quests on the board (active or
// already completed today), never below 1 — a closable set, not a fixed 5.
func dailyGoal(dailiesToday int) int {
	if dailiesToday < 1 {
		return 1
	}
	return dailiesToday
}

// activeDayStrip returns the last n local days (oldest first) flagged by
// whether at least one quest was completed that day.
func (s *Service) activeDayStrip(n int, now time.Time) ([]models.ActiveDay, error) {
	today := localDate(now)
	since := today.AddDate(0, 0, -(n - 1))
	set, err := s.store.ActiveDaySet(s.userID, since)
	if err != nil {
		return nil, err
	}
	out := make([]models.ActiveDay, 0, n)
	for i := 0; i < n; i++ {
		day := since.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		out = append(out, models.ActiveDay{Day: key, Active: set[key], Today: i == n-1})
	}
	return out, nil
}

// localDate truncates to the local calendar day (mirrors db.localDate).
func localDate(t time.Time) time.Time {
	l := t.Local()
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
}

// recommendQuest picks the next move: the pre-chosen first move if it is
// still active, else the non-boss quest with the best payout right now,
// nudged toward closing a near-level attribute and the weakest attribute.
// Bosses are deliberate, never a "pick something" answer. Returns the reason
// key so the UI can say why.
func recommendQuest(quests []models.Quest, attrs []models.Attribute, buffs []models.ActiveBuff, nth int, firstMoveID int64) (*models.Quest, string) {
	if len(quests) == 0 {
		return nil, ""
	}
	if firstMoveID != 0 {
		for i := range quests {
			if quests[i].ID == firstMoveID && quests[i].AssignedToMe && quests[i].MyStatus == models.StatusActive {
				return &quests[i], "first_move"
			}
		}
	}
	weakestKey := ""
	var weakestXP int64 = -1
	toNext := map[string]int64{}
	for _, a := range attrs {
		if weakestXP < 0 || a.TotalXP < weakestXP {
			weakestXP = a.TotalXP
			weakestKey = a.Key
		}
		toNext[a.Key] = a.XPForNextLevel - a.XPIntoLevel
	}
	nearLevel := func(q models.Quest) bool {
		for k, v := range q.AttributeRewards {
			if left, ok := toNext[k]; ok && left > 0 && v >= left {
				return true
			}
		}
		return false
	}
	var best *models.Quest
	var bestScore int64 = -1
	bestReason := ""
	for i := range quests {
		q := &quests[i]
		if q.Type == models.QuestTypeBoss {
			continue
		}
		score := ProjectPayout(*q, nth, buffs)
		reason := "default"
		switch {
		case nearLevel(*q):
			score += 40
			reason = "near_level"
		case buffApplies(*q, buffs):
			reason = "buff"
		case ComboMultiplier(nth) > 1.0:
			reason = "combo"
		case q.AttributeRewards[weakestKey] > 0:
			reason = "weakest"
		}
		if q.AttributeRewards[weakestKey] > 0 {
			score += 10
		}
		if score > bestScore {
			bestScore, best, bestReason = score, q, reason
		}
	}
	if best != nil {
		return best, bestReason
	}
	return &quests[0], "default"
}
