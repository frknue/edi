package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"edi/internal/models"
)

// ErrInvalid is a sentinel the service layer maps to HTTP 400. (Kept local so
// this package doesn't import services.)
var ErrInvalid = errors.New("invalid tool input")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// EmotionCategories are Dr. Burns' negative-emotion groups (Daily Mood Log).
var EmotionCategories = map[string]bool{
	"sad": true, "anxious": true, "guilty": true, "inferior": true,
	"lonely": true, "embarrassed": true, "hopeless": true, "frustrated": true,
	"angry": true, "other": true,
}

// Distortions are the 10 cognitive distortions (codes).
var Distortions = map[string]bool{
	"AON": true, // All-or-Nothing Thinking
	"OG":  true, // Overgeneralization
	"MF":  true, // Mental Filter
	"DP":  true, // Discounting the Positive
	"JC":  true, // Jumping to Conclusions (Mind-Reading / Fortune-Telling)
	"MAG": true, // Magnification / Minimization
	"ER":  true, // Emotional Reasoning
	"SH":  true, // Should Statements
	"LAB": true, // Labeling
	"SB":  true, // Self-Blame / Other-Blame
}

// DailyMoodLog implements Dr. David Burns' Daily Mood Log (TEAM-CBT).
type DailyMoodLog struct{}

func (DailyMoodLog) Definition() models.ToolDefinition {
	return models.ToolDefinition{
		Key:      "daily_mood_log",
		Name:     "Daily Mood Log",
		Tagline:  "Dr. David Burns · TEAM-CBT",
		Category: "wellbeing",
		Description: "Turn an upsetting moment around: name the feelings, catch the " +
			"negative thoughts and their distortions, then write a truer, kinder response.",
		AttributeRewards: map[string]int64{"health": 25, "spirituality": 15, "discipline": 10},
	}
}

func inRange(v int) bool { return v >= 0 && v <= 100 }

func (DailyMoodLog) Validate(payload []byte) ([]byte, string, error) {
	var m models.MoodLog
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	if err := dec.Decode(&m); err != nil {
		return nil, "", invalid("could not parse mood log: %v", err)
	}

	if strings.TrimSpace(m.Event) == "" {
		return nil, "", invalid("describe the upsetting event")
	}
	if len(m.Emotions) == 0 {
		return nil, "", invalid("rate at least one emotion")
	}
	for _, e := range m.Emotions {
		if !EmotionCategories[e.Category] {
			return nil, "", invalid("unknown emotion %q", e.Category)
		}
		if !inRange(e.Before) || !inRange(e.After) {
			return nil, "", invalid("emotion ratings must be 0-100")
		}
	}
	if len(m.Thoughts) == 0 {
		return nil, "", invalid("add at least one negative thought")
	}
	for i, t := range m.Thoughts {
		if strings.TrimSpace(t.Thought) == "" {
			return nil, "", invalid("negative thought %d is empty", i+1)
		}
		if !inRange(t.BeliefBefore) || !inRange(t.BeliefAfter) || !inRange(t.PositiveBelief) {
			return nil, "", invalid("belief ratings must be 0-100")
		}
		for _, d := range t.Distortions {
			if !Distortions[d] {
				return nil, "", invalid("unknown distortion code %q", d)
			}
		}
	}

	// Normalize (trim strings) and re-marshal so stored data is clean.
	m.Event = strings.TrimSpace(m.Event)
	for i := range m.Thoughts {
		m.Thoughts[i].Thought = strings.TrimSpace(m.Thoughts[i].Thought)
		m.Thoughts[i].PositiveThought = strings.TrimSpace(m.Thoughts[i].PositiveThought)
	}
	clean, _ := json.Marshal(m)
	return clean, summarize(m), nil
}

// summarize produces a one-line history label: emotion drop + thought count.
func summarize(m models.MoodLog) string {
	var before, after, n int
	for _, e := range m.Emotions {
		before += e.Before
		after += e.After
		n++
	}
	avgBefore, avgAfter := 0, 0
	if n > 0 {
		avgBefore = before / n
		avgAfter = after / n
	}
	return fmt.Sprintf("Mood %d%%→%d%% · %d thought%s reframed", avgBefore, avgAfter,
		len(m.Thoughts), plural(len(m.Thoughts)))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
