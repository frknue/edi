// Package models holds the core domain entities and the API response shapes.
// These structs are the single source of truth for the JSON contract shared by
// the web UI, CLI, mobile client, and AI agent.
package models

import "time"

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
	SkipCount        int              `json:"skip_count"`
	CreatedAt        time.Time        `json:"created_at"`
	CompletedAt      *time.Time       `json:"completed_at"`
	DueDate          *time.Time       `json:"due_date"`
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
	DueDate          *time.Time        `json:"due_date,omitempty"`
}

// JournalInput is the payload for creating a journal entry.
type JournalInput struct {
	Mood   int    `json:"mood"`
	Energy int    `json:"energy"`
	Notes  string `json:"notes"`
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
