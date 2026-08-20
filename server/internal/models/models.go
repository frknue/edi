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
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

// RegisterInput is the self-serve signup payload. The invite code must match
// the server's EDI_INVITE_CODE (registration is disabled when that is unset).
type RegisterInput struct {
	Name       string `json:"name"`
	InviteCode string `json:"invite_code"`
}

// CreatedUser is returned once at user creation/token rotation: the only time
// the plaintext token is ever visible (the server stores just its hash).
type CreatedUser struct {
	User  User   `json:"user"`
	Token string `json:"token"`
}

// Attribute is a trainable life stat. TotalXP is the stored truth; Level and the
// progress fields are derived from it via the XP formula.
type Attribute struct {
	ID      int64  `json:"id"`
	UserID  int64  `json:"-"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	TotalXP int64  `json:"total_xp"`
	PeakXP  int64  `json:"-"` // highest total_xp ever reached; anchors the decay floor

	// Derived fields (computed on read, never stored).
	Level          int             `json:"level"`
	XPIntoLevel    int64           `json:"xp_into_level"`
	XPForNextLevel int64           `json:"xp_for_next_level"`
	Progress       float64         `json:"progress"`        // 0..1 toward next level
	Decay          *AttributeDecay `json:"decay,omitempty"` // computed on read
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

// Achievement is one badge from the catalog (earned or still open).
type Achievement struct {
	Key       string     `json:"key"`
	Name      string     `json:"name"`
	Desc      string     `json:"desc"`
	Icon      string     `json:"icon"`
	Title     string     `json:"title,omitempty"` // character title it grants
	Earned    bool       `json:"earned"`
	AwardedAt *time.Time `json:"awarded_at,omitempty"`
}

// ItemDrop is one piece of loot: a completion may drop a trophy, a temporal
// XP buff (auto-active until local midnight), or an instant gold cache.
type ItemDrop struct {
	ID        int64      `json:"id"`
	Key       string     `json:"key"`
	Name      string     `json:"name"`
	Icon      string     `json:"icon"`
	Rarity    string     `json:"rarity"` // common | uncommon | rare | epic | legendary
	Kind      string     `json:"kind"`   // trophy | buff | gold
	Flavor    string     `json:"flavor"`
	Percent   int        `json:"percent,omitempty"`   // buff strength
	Attribute string     `json:"attribute,omitempty"` // buff target ("" = all)
	Gold      int64      `json:"gold,omitempty"`      // gold-cache size
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	DroppedAt time.Time  `json:"dropped_at,omitzero"`
}

// ActiveBuff is a running loot buff shown on the dashboard and applied (as
// auditable 'buff' xp_events) to completions until it expires.
type ActiveBuff struct {
	ItemKey   string    `json:"item_key"`
	Attribute string    `json:"attribute"` // "" = all attributes
	Percent   int       `json:"percent"`
	ExpiresAt time.Time `json:"expires_at"`
}

// TelegramStatus is a user's view of the Telegram presence channel.
type TelegramStatus struct {
	Configured  bool   `json:"configured"`   // bot token set + bot reachable
	Linked      bool   `json:"linked"`       // this user has a paired chat
	BotUsername string `json:"bot_username"` // for the t.me deep link
}

// TelegramPushTimes are the per-user HH:MM (server-local) push times; "" means
// the server default (EDI_BRIEFING_TIME / EDI_NUDGE_TIME) applies.
type TelegramPushTimes struct {
	Briefing string `json:"briefing"`
	Nudge    string `json:"nudge"`
}

// TelegramPushTimesPatch is a partial update; "" clears an override.
type TelegramPushTimesPatch struct {
	Briefing *string `json:"briefing,omitempty"`
	Nudge    *string `json:"nudge,omitempty"`
}

// TelegramPairCode is a short-lived, single-use code that links a Telegram
// chat to this account (shown once, like tokens).
type TelegramPairCode struct {
	Code        string    `json:"code"`
	BotUsername string    `json:"bot_username"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// QuestDraftRequest asks the AI to propose the mechanical parts of a quest
// (type, difficulty, XP rewards) from what the user has typed so far.
type QuestDraftRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// QuestDraft is the AI's proposal. It is a suggestion only — the user still
// edits and submits the form themselves.
type QuestDraft struct {
	Type             string           `json:"type"`
	Difficulty       string           `json:"difficulty"`
	AttributeRewards map[string]int64 `json:"attribute_rewards"`
	Reason           string           `json:"reason"`
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
	Gold     int64        `json:"gold"`
}

// CharacterSummary is the aggregate level across all attributes.
type CharacterSummary struct {
	Name           string  `json:"name"`
	Title          string  `json:"title"` // earned via achievements ("" = none yet)
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
	// NextComboMultiplier is what the NEXT completion today will pay (combo
	// chain: back-to-back completions the same local day multiply XP).
	NextComboMultiplier float64 `json:"next_combo_multiplier"`
}

// Dashboard is the single payload that powers the main screen.
type Dashboard struct {
	User             User              `json:"user"`
	Character        CharacterSummary  `json:"character"`
	Attributes       []Attribute       `json:"attributes"`
	TodayQuests      []Quest           `json:"today_quests"`
	Streak           Streak            `json:"streak"`
	GoldBalance      int64             `json:"gold_balance"`
	RestMode         bool              `json:"rest_mode"`
	RestSince        *time.Time        `json:"rest_since,omitempty"`
	DecayedToday     int64             `json:"decayed_today"` // XP removed by this request's decay catch-up
	RecentXPEvents   []XPEvent         `json:"recent_xp_events"`
	RecommendedQuest *Quest            `json:"recommended_quest"`
	DailyProgress    DailyProgress     `json:"daily_progress"`
	Suggestions      []AgentSuggestion `json:"pending_suggestions"`
	ActiveBuffs      []ActiveBuff      `json:"active_buffs"` // running loot buffs
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
	Quest    Quest     `json:"completed_quest"`
	XPEvents []XPEvent `json:"xp_events"`
	LevelUps []LevelUp `json:"level_ups"`
	Gold     int64     `json:"gold"` // gold minted by this completion
	// Game-layer outcomes: Crit doubles the payout (bonus xp_events with
	// source 'crit'); ComboMultiplier is the chain multiplier this completion
	// paid at (1.0 = no combo; bonus rows use source 'combo').
	Crit            bool      `json:"crit"`
	ComboMultiplier float64   `json:"combo_multiplier"`
	Drop            *ItemDrop `json:"drop,omitempty"` // loot, if the dice smiled
	// Achievements newly unlocked by this completion (evaluated post-commit).
	AchievementsUnlocked []Achievement `json:"achievements_unlocked"`
	Dashboard            Dashboard     `json:"dashboard"`
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
	Gold      int64     `json:"gold"`
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

// GoldEvent is the immutable audit record of a single gold change (mint or spend).
type GoldEvent struct {
	ID         int64     `json:"id"`
	Amount     int64     `json:"amount"` // positive = mint, negative = purchase
	Source     string    `json:"source"` // quest, subtask, tool, journal, purchase, grant
	Label      string    `json:"label,omitempty"`
	ShopItemID *int64    `json:"shop_item_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ShopItem is a self-defined real-life reward purchasable with gold. Items are
// repeatable; archiving (not deleting) removes them from the shop while keeping
// purchase history labels intact.
type ShopItem struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"-"`
	Name       string     `json:"name"`
	Price      int64      `json:"price"`
	CreatedAt  time.Time  `json:"created_at"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// ShopItemInput is the payload for creating a shop item.
type ShopItemInput struct {
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

// ShopItemPatch is a partial update; nil fields are left untouched.
type ShopItemPatch struct {
	Name  *string `json:"name,omitempty"`
	Price *int64  `json:"price,omitempty"`
}

// PurchaseResult is returned after buying a shop item.
type PurchaseResult struct {
	Item    ShopItem  `json:"item"`
	Event   GoldEvent `json:"event"`
	Balance int64     `json:"balance"` // balance after the purchase
}

// Ward is a gold-bought decay shield for one attribute. Rows are never
// deleted: lapsed windows still exclude the days they covered from billing.
type Ward struct {
	ID           int64     `json:"id"`
	AttributeKey string    `json:"attribute_key"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// WardResult is returned after buying a ward.
type WardResult struct {
	Ward    Ward  `json:"ward"`
	Balance int64 `json:"balance"` // gold balance after the purchase
}

// RestState reports whether decay is paused (vacation/sick mode).
type RestState struct {
	On    bool       `json:"on"`
	Since *time.Time `json:"since,omitempty"`
}

// AttributeDecay describes an attribute's decay state, computed on read.
// State is one of: fresh (active today), grace (idle 1-3 days), decaying
// (idle beyond grace), warded (shielded by an active ward), rest (rest mode).
type AttributeDecay struct {
	State              string     `json:"state"`
	IdleDays           int        `json:"idle_days"`
	WardedUntil        *time.Time `json:"warded_until,omitempty"`
	ProjectedDailyLoss int64      `json:"projected_daily_loss"` // 0 unless decaying
	FloorLevel         int        `json:"floor_level"`
}
