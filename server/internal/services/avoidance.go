package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"edi/internal/db"
	"edi/internal/models"
)

// The avoidance kit: a quest skipped or missed repeatedly is a step that is
// too big, not a lazy person. Three exits, none of them a counter:
//   - BreakDownQuest: the AI writes 3-5 tiny phases as subtasks (the first is
//     a 2-minute start) — the quest stays, it just gets a first foothold.
//   - ShrinkQuest: the AI proposes a smaller version; the original is
//     archived and the smaller one created in ONE tx (store.ReplaceQuest).
//   - RetireQuest: archive with dignity (a choice, no XP loss, no counter).
// The AI paths are gated on the ChatGPT connection like every AI feature.

// StrugglingSkips is the skip/miss count from which a quest is "struggling".
const StrugglingSkips = 2

const breakDownInstructions = `You are the coach of "edi", a life-RPG. The player keeps avoiding a quest. ` +
	`Break it into 3 to 5 ORDERED tiny steps. The FIRST step must take under 2 minutes and require no decision ` +
	`(open the file, put on the shoes, lay out the pen). Each step is one concrete physical action. Keep the ` +
	`player's language. Respond with ONLY a JSON object, no prose or fences:
{"steps":[{"title":"string","attribute_rewards":{"<attribute_key>":<integer 3-10>}}]}
attribute_rewards keys must be from the given attribute keys; one key per step.`

// BreakDownQuest replaces the quest's subtasks with AI-written tiny steps.
func (s *Service) BreakDownQuest(id int64) (models.Quest, error) {
	q, err := s.store.GetVisibleQuest(s.userID, id)
	if err != nil {
		return models.Quest{}, ErrNotFound
	}
	if !q.AssignedToMe {
		return models.Quest{}, validationErr("this quest is assigned to another board member")
	}
	if q.MyStatus == models.StatusCompleted || q.MyStatus == models.StatusArchived {
		return models.Quest{}, validationErr("only an open quest can be broken down")
	}
	keys, err := s.attributeKeys()
	if err != nil {
		return models.Quest{}, err
	}
	prompt := fmt.Sprintf("Attribute keys: %s\nQuest: %q\nDescription: %q\nType: %s, difficulty: %s, rewards: %v\nSkipped/missed %d times.\n\nBreak it down.",
		strings.Join(keys, ", "), q.Title, q.Description, q.Type, q.Difficulty, q.AttributeRewards, q.SkipCount)
	raw, err := s.completeWithOpenAI(breakDownInstructions, prompt)
	if err != nil {
		return models.Quest{}, err
	}
	var parsed struct {
		Steps []models.SubtaskInput `json:"steps"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &parsed); err != nil || len(parsed.Steps) == 0 {
		return models.Quest{}, fmt.Errorf("%w: the model returned an unexpected response, try again", ErrValidation)
	}
	var steps []models.SubtaskInput
	for _, st := range parsed.Steps {
		st.Title = strings.TrimSpace(st.Title)
		if st.Title == "" || len(steps) >= 5 {
			continue
		}
		if st.AttributeRewards == nil {
			st.AttributeRewards = map[string]int64{}
		}
		steps = append(steps, st)
	}
	if err := s.validateSubtasks(steps); err != nil {
		return models.Quest{}, err
	}
	return s.UpdateQuest(id, models.QuestPatch{Subtasks: &steps})
}

const shrinkInstructions = `You are the coach of "edi", a life-RPG. The player keeps avoiding a quest: it is too big. ` +
	`Propose a SMALLER version of the same activity that takes a third of the effort or less and still counts ` +
	`(e.g. "30 minute workout" → "5 push-ups", "Read 15 pages" → "Read one page"). Keep the same attributes, ` +
	`scale the XP down proportionally (minimum 5 per attribute, multiples of 5). Keep the player's language. ` +
	`Respond with ONLY a JSON object, no prose or fences:
{"title":"string","description":"string","difficulty":"trivial|easy","attribute_rewards":{"<attribute_key>":<integer>}}`

// ShrinkQuest asks the AI for a smaller version and swaps it in atomically.
func (s *Service) ShrinkQuest(id int64) (models.Quest, error) {
	q, err := s.store.GetQuest(s.userID, id)
	if err != nil {
		return models.Quest{}, ErrNotFound
	}
	if q.SharedQuestID != nil {
		return models.Quest{}, validationErr("shared quests cannot be shrunk — create a smaller personal quest instead")
	}
	if q.Status != models.StatusActive && q.Status != models.StatusSkipped {
		return models.Quest{}, validationErr("only an open quest can be shrunk")
	}
	prompt := fmt.Sprintf("Quest: %q\nDescription: %q\nType: %s, difficulty: %s, rewards: %v\nSkipped/missed %d times.\n\nPropose the smaller version.",
		q.Title, q.Description, q.Type, q.Difficulty, q.AttributeRewards, q.SkipCount)
	raw, err := s.completeWithOpenAI(shrinkInstructions, prompt)
	if err != nil {
		return models.Quest{}, err
	}
	var parsed struct {
		Title            string           `json:"title"`
		Description      string           `json:"description"`
		Difficulty       string           `json:"difficulty"`
		AttributeRewards map[string]int64 `json:"attribute_rewards"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &parsed); err != nil {
		return models.Quest{}, fmt.Errorf("%w: the model returned an unexpected response, try again", ErrValidation)
	}
	in := models.QuestInput{
		Title:            strings.TrimSpace(parsed.Title),
		Description:      strings.TrimSpace(parsed.Description),
		Type:             q.Type,
		Difficulty:       orFallback(parsed.Difficulty, "easy"),
		AttributeRewards: parsed.AttributeRewards,
		DueDate:          q.DueDate,
	}
	if err := s.validateQuestInput(&in); err != nil {
		return models.Quest{}, err
	}
	return s.replaceQuest(id, in)
}

// replaceQuest is the atomic swap behind ShrinkQuest (and tests).
func (s *Service) replaceQuest(oldID int64, in models.QuestInput) (models.Quest, error) {
	created, err := s.store.ReplaceQuest(s.userID, oldID, in)
	switch {
	case errors.Is(err, db.ErrNotFound):
		return models.Quest{}, ErrNotFound
	case errors.Is(err, db.ErrQuestNotCompletable):
		return models.Quest{}, validationErr("only an open quest can be replaced")
	}
	return created, err
}

// RetireQuest archives a quest as a deliberate choice. Same store path as
// ArchiveQuest; the name is the point — nothing is lost, nothing is counted.
func (s *Service) RetireQuest(id int64) (models.Quest, error) {
	return s.ArchiveQuest(id)
}

func (s *Service) attributeKeys() ([]string, error) {
	names, err := s.store.AttributeNames(s.userID)
	if err != nil {
		return nil, err
	}
	return sortedStringKeys(names), nil
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
