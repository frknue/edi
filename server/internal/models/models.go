// Package models holds the core domain entities and the API response shapes.
// These structs are the single source of truth for the JSON contract shared by
// the web UI, CLI, mobile client, and AI agent.
package models

import (
	"encoding/json"
	"time"
)

// Quest types and difficulties (kept as plain strings for storage simplicity).
const (
	QuestTypeDaily    = "daily"
	QuestTypeWeekly   = "weekly"
	QuestTypeMain     = "main"
	QuestTypeSide     = "side"
	QuestTypeBoss     = "boss"
	QuestTypeRecovery = "recovery"

	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusSkipped   = "skipped"
	StatusArchived  = "archived"
)

// User is the single account in MVP single-user mode.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Attribute is a trainable life stat. TotalXP is the stored truth; Level and the
// progress fields are derived from it via the XP formula.
type Attribute struct {
	ID      int64  `json:"id"`
	UserID  int64  `json:"-"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	TotalXP int64  `json:"total_xp"`

	// Derived fields (computed on read, never stored).
	Level          int     `json:"level"`
	XPIntoLevel    int64   `json:"xp_into_level"`
	XPForNextLevel int64   `json:"xp_for_next_level"`
	Progress       float64 `json:"progress"` // 0..1 toward next level
}

// Quest is a real-life action the user can complete for XP.
type Quest struct {
	ID               int64            `json:"id"`
	UserID           int64            `json:"-"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	Type             string           `json:"type"`
	Difficulty       string           `json:"difficulty"`
	Status           string           `json:"status"`
	AttributeRewards map[string]int64 `json:"attribute_rewards"`
	Subtasks         []Subtask        `json:"subtasks"`
	SkipCount        int              `json:"skip_count"`
	CreatedAt        time.Time        `json:"created_at"`
	CompletedAt      *time.Time       `json:"completed_at"`
	DueDate          *time.Time       `json:"due_date"`
}

// Subtask is an optional bonus objective on a quest. Checking it before the
// quest is completed awards its own AttributeRewards on top of the quest's.
// Subtasks never block completion.
type Subtask struct {
	ID               int64            `json:"id"`
	QuestID          int64            `json:"quest_id"`
	Title            string           `json:"title"`
	AttributeRewards map[string]int64 `json:"attribute_rewards"`
	Done             bool             `json:"done"`
}

// SubtaskInput is the payload for creating a subtask (inline with a quest).
type SubtaskInput struct {
	Title            string           `json:"title"`
	AttributeRewards map[string]int64 `json:"attribute_rewards"`
}

// TotalReward is the sum of XP across all rewarded attributes.
func (q Quest) TotalReward() int64 {
	var sum int64
	for _, v := range q.AttributeRewards {
		sum += v
	}
	return sum
}

// XPEvent is the immutable audit record of a single attribute XP change.
type XPEvent struct {
	ID            int64     `json:"id"`
	AttributeKey  string    `json:"attribute_key"`
	AttributeName string    `json:"attribute_name,omitempty"`
	Amount        int64     `json:"amount"`
	Source        string    `json:"source"`
	SourceID      *int64    `json:"source_id,omitempty"`
	Note          string    `json:"note,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Streak tracks consecutive active days.
type Streak struct {
	Current        int     `json:"current"`
	Longest        int     `json:"longest"`
	LastActiveDate *string `json:"last_active_date"` // YYYY-MM-DD
}

// JournalEntry is a daily reflection.
type JournalEntry struct {
	ID        int64     `json:"id"`
	Mood      int       `json:"mood"`   // 1..10
	Energy    int       `json:"energy"` // 1..10
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

// AgentSuggestion is a rule-based (later LLM-based) recommendation. When accepted
// it spawns a real Quest from SuggestedQuest.
type AgentSuggestion struct {
	ID             int64      `json:"id"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	Reason         string     `json:"reason"`
	SuggestedQuest QuestInput `json:"suggested_quest"`
	Status         string     `json:"status"`
	CreatedQuestID *int64     `json:"created_quest_id,omitempty"`
	SourceQuestID  *int64     `json:"source_quest_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

// QuestInput is the payload for creating/updating a quest and the template stored
// inside a suggestion.
type QuestInput struct {
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	Type             string           `json:"type"`
	Difficulty       string           `json:"difficulty"`
	AttributeRewards map[string]int64 `json:"attribute_rewards"`
	Subtasks         []SubtaskInput   `json:"subtasks,omitempty"`
	DueDate          *time.Time       `json:"due_date,omitempty"`
}

// QuestPatch is a partial update; nil fields are left untouched.
type QuestPatch struct {
	Title            *string           `json:"title,omitempty"`
	Description      *string           `json:"description,omitempty"`
	Type             *string           `json:"type,omitempty"`
	Difficulty       *string           `json:"difficulty,omitempty"`
	Status           *string           `json:"status,omitempty"`
	AttributeRewards *map[string]int64 `json:"attribute_rewards,omitempty"`
	// Subtasks, when present, replaces the quest's subtask list (done flags reset).
	Subtasks *[]SubtaskInput `json:"subtasks,omitempty"`
	DueDate  *time.Time      `json:"due_date,omitempty"`
}

// JournalInput is the payload for creating a journal entry.
type JournalInput struct {
	Mood   int    `json:"mood"`
	Energy int    `json:"energy"`
	Notes  string `json:"notes"`
}

// JournalPatch is a partial update to an entry; nil fields are left untouched.
type JournalPatch struct {
	Mood   *int    `json:"mood,omitempty"`
	Energy *int    `json:"energy,omitempty"`
	Notes  *string `json:"notes,omitempty"`
}

// JournalCreateResult is returned on create. The first entry of a local day
// awards XP (auditable via xp_events, source='journal'); later entries that day
// return empty XPEvents.
type JournalCreateResult struct {
	Entry    JournalEntry `json:"entry"`
	XPEvents []XPEvent    `json:"xp_events"`
	LevelUps []LevelUp    `json:"level_ups"`
}

// CharacterSummary is the aggregate level across all attributes.
type CharacterSummary struct {
	Name           string  `json:"name"`
	Level          int     `json:"level"`
	TotalXP        int64   `json:"total_xp"`
	XPIntoLevel    int64   `json:"xp_into_level"`
	XPForNextLevel int64   `json:"xp_for_next_level"`
	Progress       float64 `json:"progress"`
}

// DailyProgress drives the "today" indicator.
type DailyProgress struct {
	CompletedToday int     `json:"completed_today"`
	Goal           int     `json:"goal"`
	Ratio          float64 `json:"ratio"`
}

// Dashboard is the single payload that powers the main screen.
type Dashboard struct {
	User             User              `json:"user"`
	Character        CharacterSummary  `json:"character"`
	Attributes       []Attribute       `json:"attributes"`
	TodayQuests      []Quest           `json:"today_quests"`
	Streak           Streak            `json:"streak"`
	RecentXPEvents   []XPEvent         `json:"recent_xp_events"`
	RecommendedQuest *Quest            `json:"recommended_quest"`
	DailyProgress    DailyProgress     `json:"daily_progress"`
	Suggestions      []AgentSuggestion `json:"pending_suggestions"`
}

// LevelUp reports an attribute crossing a level boundary during a completion.
type LevelUp struct {
	AttributeKey  string `json:"attribute_key"`
	AttributeName string `json:"attribute_name"`
	FromLevel     int    `json:"from_level"`
	ToLevel       int    `json:"to_level"`
}

// CompletionResult is returned after completing a quest so clients can render
// rewarding feedback and refresh state in one round-trip.
type CompletionResult struct {
	Quest     Quest     `json:"completed_quest"`
	XPEvents  []XPEvent `json:"xp_events"`
	LevelUps  []LevelUp `json:"level_ups"`
	Dashboard Dashboard `json:"dashboard"`
}

// ToolDefinition describes a guided instrument that awards XP when completed.
type ToolDefinition struct {
	Key              string           `json:"key"`
	Name             string           `json:"name"`
	Tagline          string           `json:"tagline"`
	Description      string           `json:"description"`
	Category         string           `json:"category"`
	AttributeRewards map[string]int64 `json:"attribute_rewards"`
}

// ToolEntry is a stored completion of a tool (its structured data + XP awarded).
type ToolEntry struct {
	ID        int64           `json:"id"`
	ToolKey   string          `json:"tool_key"`
	Data      json.RawMessage `json:"data"`
	XPAwarded int64           `json:"xp_awarded"`
	CreatedAt time.Time       `json:"created_at"`
	Summary   string          `json:"summary,omitempty"`
}

// ToolCompletionResult is returned after completing a tool (mirrors quest completion).
type ToolCompletionResult struct {
	Entry     ToolEntry `json:"entry"`
	XPEvents  []XPEvent `json:"xp_events"`
	LevelUps  []LevelUp `json:"level_ups"`
	Dashboard Dashboard `json:"dashboard"`
}

// --- Daily Mood Log (Dr. David Burns / TEAM-CBT) ---------------------------

// MoodEmotion is one rated feeling: intensity before and after (0-100).
type MoodEmotion struct {
	Category string `json:"category"` // e.g. "sad", "anxious"
	Before   int    `json:"before"`
	After    int    `json:"after"`
}

// MoodThought is one automatic negative thought worked through the triple column.
type MoodThought struct {
	Thought         string   `json:"thought"`
	BeliefBefore    int      `json:"belief_before"` // 0-100
	Distortions     []string `json:"distortions"`   // distortion codes
	PositiveThought string   `json:"positive_thought"`
	PositiveBelief  int      `json:"positive_belief"` // 0-100
	BeliefAfter     int      `json:"belief_after"`    // 0-100, re-rated negative belief
}

// MoodLog is the full Daily Mood Log payload.
type MoodLog struct {
	Event    string        `json:"event"`
	Emotions []MoodEmotion `json:"emotions"`
	Thoughts []MoodThought `json:"thoughts"`
}

// MoodAssistInput asks the AI coach to help with one negative thought. Mode is
// "distortions" (identify the distortions) or "responses" (suggest rational
// responses). Event/Emotions give optional context.
type MoodAssistInput struct {
	Mode        string   `json:"mode"`
	Event       string   `json:"event"`
	Thought     string   `json:"thought"`
	Distortions []string `json:"distortions"`
}

// MoodDistortionHit is one detected cognitive distortion with a short rationale.
type MoodDistortionHit struct {
	Code string `json:"code"`
	Why  string `json:"why"`
}

// MoodResponseIdea is one suggested rational response tagged with the CBT method.
type MoodResponseIdea struct {
	Technique string `json:"technique"`
	Text      string `json:"text"`
}

// MoodAssistResult is the AI coach's reply. When Crisis is true the coaching
// fields are empty and CrisisMessage carries a supportive, resource-pointing note.
type MoodAssistResult struct {
	Mode          string              `json:"mode"`
	Distortions   []MoodDistortionHit `json:"distortions,omitempty"`
	Responses     []MoodResponseIdea  `json:"responses,omitempty"`
	Crisis        bool                `json:"crisis"`
	CrisisMessage string              `json:"crisis_message,omitempty"`
}

// OpenAIStatus describes the ChatGPT-subscription connection powering AI features.
type OpenAIStatus struct {
	Connected     bool       `json:"connected"`
	Email         string     `json:"email,omitempty"`
	AccountID     string     `json:"account_id,omitempty"`
	Model         string     `json:"model,omitempty"`
	Effort        string     `json:"effort,omitempty"`
	EffortOptions []string   `json:"effort_options,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}
