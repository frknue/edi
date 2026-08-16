package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"edi/internal/models"
)

// Quest drafting: the user types a title (and optionally a description) and the
// connected ChatGPT model proposes the mechanical parts — type, difficulty and
// attribute XP. The draft is a SUGGESTION only: it is returned to the form for
// the user to edit and submit, never written to the DB here.

// DraftQuest asks the model to classify a quest idea. Requires a connected
// OpenAI account (returns ErrOpenAINotConnected otherwise).
func (s *Service) DraftQuest(in models.QuestDraftRequest) (models.QuestDraft, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return models.QuestDraft{}, validationErr("write a title first")
	}

	known, err := s.store.AttributeNames(s.userID)
	if err != nil {
		return models.QuestDraft{}, err
	}

	raw, err := s.completeWithOpenAI(draftInstructions(known), draftPrompt(title, in.Description))
	if err != nil {
		return models.QuestDraft{}, err // includes ErrOpenAINotConnected
	}

	var parsed models.QuestDraft
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &parsed); err != nil {
		return models.QuestDraft{}, fmt.Errorf("%w: the model returned an unexpected response, try again", ErrValidation)
	}

	draft := sanitizeDraft(parsed, known)
	// Guarantee the draft is actually creatable if the user submits it as-is.
	check := models.QuestInput{
		Title:            title,
		Type:             draft.Type,
		Difficulty:       draft.Difficulty,
		AttributeRewards: draft.AttributeRewards,
	}
	if err := s.validateQuestInput(&check); err != nil {
		return models.QuestDraft{}, err
	}
	return draft, nil
}

// sanitizeDraft coerces whatever the model returned into something the quest
// form can apply: valid type/difficulty (falling back to the form defaults),
// only known attribute keys, positive XP rounded to the ±5 steps the UI uses.
func sanitizeDraft(d models.QuestDraft, known map[string]string) models.QuestDraft {
	out := models.QuestDraft{
		Type:             strings.ToLower(strings.TrimSpace(d.Type)),
		Difficulty:       strings.ToLower(strings.TrimSpace(d.Difficulty)),
		AttributeRewards: map[string]int64{},
		Reason:           strings.TrimSpace(d.Reason),
	}
	if !validTypes[out.Type] {
		out.Type = models.QuestTypeDaily
	}
	if !validDifficulties[out.Difficulty] {
		out.Difficulty = "easy"
	}
	for k, v := range d.AttributeRewards {
		key := strings.ToLower(strings.TrimSpace(k))
		if !knownKey(known, key) || v <= 0 {
			continue
		}
		if v > maxDraftXP {
			v = maxDraftXP
		}
		out.AttributeRewards[key] = roundTo5(v)
	}
	// Keep the draft focused: at most 3 attributes, the biggest rewards first.
	if len(out.AttributeRewards) > maxDraftAttributes {
		keys := make([]string, 0, len(out.AttributeRewards))
		for k := range out.AttributeRewards {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if out.AttributeRewards[keys[i]] != out.AttributeRewards[keys[j]] {
				return out.AttributeRewards[keys[i]] > out.AttributeRewards[keys[j]]
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys[maxDraftAttributes:] {
			delete(out.AttributeRewards, k)
		}
	}
	if len(out.Reason) > 200 {
		out.Reason = strings.TrimSpace(out.Reason[:200]) + "…"
	}
	return out
}

const (
	maxDraftXP         = 200 // per attribute
	maxDraftAttributes = 3
)

// roundTo5 snaps XP to the 5-point steps the reward steppers use (min 5).
func roundTo5(v int64) int64 {
	r := ((v + 2) / 5) * 5
	if r < 5 {
		r = 5
	}
	return r
}

func draftInstructions(known map[string]string) string {
	keys := make([]string, 0, len(known))
	for k := range known {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return `You are the in-game game-designer for "edi", a life-RPG habit app. Real-life actions are "quests"; ` +
		`completing one awards XP to life attributes. The user has typed a quest idea and wants you to fill in the ` +
		`mechanics: which kind of quest it is, how demanding it is, and which attributes it should train.

Attribute keys (use ONLY these): ` + strings.Join(keys, ", ") + `

quest type (how it recurs / what role it plays) is one of: daily, weekly, main, side, boss, recovery
difficulty (how demanding it is) is one of: trivial, easy, medium, hard, boss
Note "boss" appears in both lists and they are independent — a hard one-off does not have to be type "boss".

Respond with ONLY a JSON object, no prose or markdown fences, matching exactly:
{"type":"daily|weekly|main|side|boss|recovery","difficulty":"trivial|easy|medium|hard|boss","attribute_rewards":{"<attribute_key>":<integer, multiple of 5>},"reason":"one short sentence explaining the call"}

Rules:
- Pick 1 to 3 attributes that the action genuinely trains. Do not spread XP over everything.
- Total XP across attributes scales with difficulty: trivial ~15, easy ~30, medium ~50, hard ~90, boss ~150+.
- Recovery quests are restful (rest, recuperation, deliberate downtime) and feel soft.
- Habits that repeat every day are "daily"; weekly rituals are "weekly"; a big one-off goal is "main" or "boss"; an optional extra is "side".
- Keep the reason under 20 words, concrete, and about THIS quest.`
}

func draftPrompt(title, description string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Quest title: %q\n", title)
	if d := strings.TrimSpace(description); d != "" {
		fmt.Fprintf(&b, "Description: %s\n", d)
	}
	b.WriteString("\nPropose the type, difficulty and attribute XP as specified.")
	return b.String()
}
