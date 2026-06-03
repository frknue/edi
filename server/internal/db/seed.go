package db

import (
	"encoding/json"
	"time"

	"liferpg/internal/models"
)

// DefaultAttributes is the canonical attribute set every user starts with.
var DefaultAttributes = []struct {
	Key  string
	Name string
}{
	{"strength", "Strength"},
	{"discipline", "Discipline"},
	{"focus", "Focus"},
	{"health", "Health"},
	{"wealth", "Wealth"},
	{"relationships", "Relationships"},
	{"learning", "Learning"},
	{"creativity", "Creativity"},
	{"spirituality", "Spirituality"},
}

// Seed populates demo data on a fresh database (no-op if a user already exists).
// It guarantees the audit invariant: each attribute's total_xp equals the sum
// of its xp_events.
func (s *Store) Seed() error {
	n, err := s.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()

	res, err := tx.Exec(`INSERT INTO users(name, created_at) VALUES(?, ?)`, "Hero", formatTime(now.AddDate(0, 0, -10)))
	if err != nil {
		return err
	}
	userID, _ := res.LastInsertId()

	// Attributes with starting XP (Health & Spirituality intentionally low so the
	// rule-based suggestion engine has something to recommend).
	startingXP := map[string]int64{
		"strength":      520,
		"discipline":    380,
		"focus":         610,
		"health":        90,
		"wealth":        250,
		"relationships": 140,
		"learning":      300,
		"creativity":    170,
		"spirituality":  60,
	}
	for i, a := range DefaultAttributes {
		xp := startingXP[a.Key]
		if _, err := tx.Exec(`INSERT INTO attributes(user_id, key, name, total_xp, created_at) VALUES(?, ?, ?, ?, ?)`,
			userID, a.Key, a.Name, xp, formatTime(now.AddDate(0, 0, -10))); err != nil {
			return err
		}
		// One audit event per attribute so totals == sum(xp_events). Stagger the
		// timestamps so the recent-XP feed looks alive.
		created := now.Add(-time.Duration(i*7) * time.Hour)
		if _, err := tx.Exec(
			`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES(?, ?, ?, 'seed', NULL, ?, ?)`,
			userID, a.Key, xp, "Starting progress", formatTime(created)); err != nil {
			return err
		}
	}

	// Streak: 3-day streak ending yesterday, so the first completion today bumps it to 4.
	if _, err := tx.Exec(`INSERT INTO streaks(user_id, current_count, longest_count, last_active_date) VALUES(?, 3, 7, ?)`,
		userID, now.Local().AddDate(0, 0, -1).Format(dayFormat)); err != nil {
		return err
	}

	// Quests.
	seedQuests := []models.QuestInput{
		{Title: "30 minute workout", Description: "Get the blood moving — strength or cardio.", Type: "daily", Difficulty: "medium", AttributeRewards: map[string]int64{"strength": 40, "discipline": 10}},
		{Title: "90 minute deep work session", Description: "Single task, no distractions, phone away.", Type: "daily", Difficulty: "hard", AttributeRewards: map[string]int64{"focus": 60, "discipline": 20}},
		{Title: "Read 15 pages", Description: "Any book that grows you.", Type: "daily", Difficulty: "easy", AttributeRewards: map[string]int64{"learning": 30}},
		{Title: "Review finances", Description: "Check spending and update the budget.", Type: "daily", Difficulty: "medium", AttributeRewards: map[string]int64{"wealth": 35, "discipline": 10}},
		{Title: "Call or message someone important", Description: "Reach out and actually connect.", Type: "daily", Difficulty: "easy", AttributeRewards: map[string]int64{"relationships": 40}},
		{Title: "Recovery walk", Description: "Gentle, no pace goal. Just breathe.", Type: "recovery", Difficulty: "trivial", AttributeRewards: map[string]int64{"health": 20, "spirituality": 10}},
		{Title: "Weekly review & plan", Description: "Reflect on the week and set next week's targets.", Type: "weekly", Difficulty: "medium", AttributeRewards: map[string]int64{"discipline": 30, "focus": 20}},
		{Title: "Build MVP landing page", Description: "Ship the first public page for your project.", Type: "boss", Difficulty: "boss", AttributeRewards: map[string]int64{"creativity": 90, "focus": 60, "wealth": 40}},
	}
	for _, q := range seedQuests {
		rewards, _ := json.Marshal(q.AttributeRewards)
		if _, err := tx.Exec(
			`INSERT INTO quests(user_id, title, description, type, difficulty, status, attribute_rewards, created_at) VALUES(?, ?, ?, ?, ?, 'active', ?, ?)`,
			userID, q.Title, q.Description, q.Type, q.Difficulty, string(rewards), formatTime(now.AddDate(0, 0, -2))); err != nil {
			return err
		}
	}

	// Journal entry.
	if _, err := tx.Exec(`INSERT INTO journal_entries(user_id, mood, energy, notes, created_at) VALUES(?, ?, ?, ?, ?)`,
		userID, 7, 6, "Felt good today. Shipped the first slice of the project and kept my focus block.", formatTime(now.AddDate(0, 0, -1))); err != nil {
		return err
	}

	// Two pending agent suggestions.
	suggestions := []models.AgentSuggestion{
		{
			Type:   "low_attribute",
			Title:  "Add a Health quest",
			Reason: "Health is your lowest attribute. A small daily habit will move it fast.",
			SuggestedQuest: models.QuestInput{
				Title: "Drink water & 15-min mobility", Description: "Hydrate and loosen up.", Type: "daily", Difficulty: "easy",
				AttributeRewards: map[string]int64{"health": 30},
			},
		},
		{
			Type:   "recovery",
			Title:  "Schedule a recovery day",
			Reason: "You've been active several days in a row — protect the streak with intentional recovery.",
			SuggestedQuest: models.QuestInput{
				Title: "Recovery walk & stretch", Description: "Easy movement, no targets.", Type: "recovery", Difficulty: "trivial",
				AttributeRewards: map[string]int64{"health": 20, "spirituality": 15},
			},
		},
	}
	for _, sug := range suggestions {
		tmpl, _ := json.Marshal(sug.SuggestedQuest)
		if _, err := tx.Exec(
			`INSERT INTO agent_suggestions(user_id, type, title, reason, suggested_quest, status, created_at) VALUES(?, ?, ?, ?, ?, 'pending', ?)`,
			userID, sug.Type, sug.Title, sug.Reason, string(tmpl), formatTime(now.AddDate(0, 0, -1))); err != nil {
			return err
		}
	}

	return tx.Commit()
}
