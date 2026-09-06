package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"edi/internal/models"
)

// Story mode: the user's own ChatGPT model narrates their real progress as a
// running saga (briefing episodes, /story on demand) and forges boss quests
// from their actual weak spots. Gated on the connection like every AI feature.

const narrationInstructions = `You are the game-master narrator of "edi", a life-RPG. The player's REAL ` +
	`daily life is the campaign. Write a terse, vivid episode narration — 2 to 3 sentences, second person, ` +
	`present tense, phosphor-terminal fantasy tone (think retro RPG, not purple prose). Reference their real ` +
	`numbers and quest names. You are the hero's companion, never their judge: no threats, no guilt, no ` +
	`"you should have". If the hero has been away, welcome them back like someone who missed them and point ` +
	`at ONE small next step; if the streak lives, honor it. NEVER invent activities they didn't do. ` +
	`Respond with the narration text only — no quotes, no markdown, no preamble.`

// StoryNarration generates a short in-world recap of the player's current
// state (used by the morning briefing and the /story command).
func (s *Service) StoryNarration() (string, error) {
	dash, err := s.GetDashboard()
	if err != nil {
		return "", err
	}
	events, err := s.store.ListXPEvents(s.userID, 15)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Hero: %s", dash.Character.Name)
	if dash.Character.Title != "" {
		fmt.Fprintf(&b, " %q", dash.Character.Title)
	}
	fmt.Fprintf(&b, ", level %d. Streak %d days. Gold %d.\n", dash.Character.Level, dash.Streak.Current, dash.GoldBalance)
	fmt.Fprintf(&b, "Open quests today: %d.\n", len(dash.TodayQuests))
	for _, q := range dash.TodayQuests {
		fmt.Fprintf(&b, "- %q [%s/%s]\n", q.Title, q.Type, q.Difficulty)
	}
	b.WriteString("Recent deeds (XP ledger, newest first):\n")
	for _, e := range events {
		fmt.Fprintf(&b, "- %+d %s (%s: %s)\n", e.Amount, e.AttributeKey, e.Source, e.Note)
	}
	for _, a := range dash.Attributes {
		if a.Decay != nil && a.Decay.State == "decaying" {
			// Hardcore only: the player opted into stakes, so the narrator may name them.
			fmt.Fprintf(&b, "Hardcore mode — %s is rusting (%d idle days, -%d XP/day).\n", a.Name, a.Decay.IdleDays, a.Decay.ProjectedDailyLoss)
		}
	}
	idle := 0
	for i := len(dash.ActiveDays) - 1; i >= 0; i-- {
		if dash.ActiveDays[i].Active {
			break
		}
		idle++
	}
	if idle >= 2 {
		fmt.Fprintf(&b, "The hero has been away for %d days (no quests completed).\n", idle)
	}
	b.WriteString("\nNarrate the current episode.")

	out, err := s.completeWithOpenAI(narrationInstructions, b.String())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

const forgeBossInstructions = `You are the dungeon-master of "edi", a life-RPG where real-life actions are ` +
	`quests. Forge ONE epic BOSS quest: a big, concrete, real-world challenge the player can complete within a ` +
	`week, aimed at their WEAKEST attributes (the boss "guards" what they neglect). Give it a menacing RPG boss ` +
	`name in the title tied to the real activity (e.g. "Slay the Ledger Wyrm — do your full tax return"). The ` +
	`description states the real-world completion condition in 1-2 sentences.

Respond with ONLY a JSON object, no prose or fences:
{"title":"string","description":"string","attribute_rewards":{"<attribute_key>":<integer, multiple of 5>},"phases":[{"title":"string","attribute_rewards":{"<attribute_key>":<integer 5-20>}}]}

Rules:
- attribute_rewards: 2-3 of the player's weakest attribute keys, total 120-200 XP.
- phases: 3 to 5 ORDERED, concrete, small real-world steps that together defeat the boss (each one doable in one sitting; the first must be a 5-minute start). Each phase carries 5-20 XP on one of the boss's attributes. Phases are the boss's HP bar — checking one is a hit.
- The challenge must be genuinely completable within a week by one person.
- Do not duplicate an active quest.`

// ForgeBoss asks the model to design a boss quest from the player's weak
// spots and creates it (type boss, difficulty boss). The weekly ritual.
func (s *Service) ForgeBoss() (models.Quest, error) {
	dash, err := s.GetDashboard()
	if err != nil {
		return models.Quest{}, err
	}
	weekly, err := s.store.WeeklyXPByAttribute(s.userID, time.Now().Add(-7*24*time.Hour))
	if err != nil {
		return models.Quest{}, err
	}

	attrs := append([]models.Attribute(nil), dash.Attributes...)
	sort.Slice(attrs, func(i, j int) bool { return attrs[i].TotalXP < attrs[j].TotalXP })

	var b strings.Builder
	fmt.Fprintf(&b, "Player: %s, level %d.\n", dash.Character.Name, dash.Character.Level)
	b.WriteString("Attributes, weakest first (key: level, total XP, XP this week):\n")
	for _, a := range attrs {
		fmt.Fprintf(&b, "- %s: Lv%d, %d XP, %d this week\n", a.Key, a.Level, a.TotalXP, weekly[a.Key])
	}
	b.WriteString("Active quests (do NOT duplicate):\n")
	for _, q := range dash.TodayQuests {
		fmt.Fprintf(&b, "- %q\n", q.Title)
	}
	b.WriteString("\nForge the boss as specified.")

	raw, err := s.completeWithOpenAI(forgeBossInstructions, b.String())
	if err != nil {
		return models.Quest{}, err // includes ErrOpenAINotConnected
	}
	var parsed struct {
		Title            string           `json:"title"`
		Description      string           `json:"description"`
		AttributeRewards map[string]int64 `json:"attribute_rewards"`
		Phases           []struct {
			Title            string           `json:"title"`
			AttributeRewards map[string]int64 `json:"attribute_rewards"`
		} `json:"phases"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &parsed); err != nil {
		return models.Quest{}, fmt.Errorf("%w: the model returned an unexpected response, try again", ErrValidation)
	}
	// Phases become subtasks: the boss's HP bar. Bad ones are dropped, not fatal.
	var phases []models.SubtaskInput
	for _, ph := range parsed.Phases {
		title := strings.TrimSpace(ph.Title)
		if title == "" || len(phases) >= 5 {
			continue
		}
		if ph.AttributeRewards == nil {
			ph.AttributeRewards = map[string]int64{}
		}
		phases = append(phases, models.SubtaskInput{Title: title, AttributeRewards: ph.AttributeRewards})
	}
	in := models.QuestInput{
		Title:            strings.TrimSpace(parsed.Title),
		Description:      strings.TrimSpace(parsed.Description),
		Type:             "boss",
		Difficulty:       "boss",
		AttributeRewards: parsed.AttributeRewards,
		Subtasks:         phases,
	}
	if err := s.validateQuestInput(&in); err != nil {
		return models.Quest{}, err
	}
	return s.store.InsertQuest(s.userID, in, nil)
}
