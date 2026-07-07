package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"edi/internal/models"
	"edi/internal/tools"
)

// safetyPreamble is prepended to every mood-log AI call. The tool is self-help,
// not therapy — and genuine risk is routed to support rather than "coached".
const safetyPreamble = `You are a warm, encouraging CBT self-help coach inside a Daily Mood Log tool ` +
	`(Dr. David Burns' method). You are NOT a therapist and must not diagnose or provide medical or ` +
	`psychological treatment. Be brief, kind, and collaborative; help the person do their own work.

SAFETY: If the thought or event suggests possible risk of suicide, self-harm, harming others, or abuse, ` +
	`do NOT provide coaching. Instead set "crisis": true and put a short, compassionate note in ` +
	`"crisis_message" that validates their pain and encourages reaching out to a trusted person or a mental-health ` +
	`professional now — and to contact local emergency services, or in the US call or text 988 (Suicide & Crisis ` +
	`Lifeline). Otherwise set "crisis": false.`

// ToolAssist runs an AI assist for a tool. Currently only the Daily Mood Log
// supports assists. Requires a connected OpenAI account.
func (s *Service) ToolAssist(key string, payload json.RawMessage) (models.MoodAssistResult, error) {
	if key != "daily_mood_log" {
		return models.MoodAssistResult{}, validationErr("AI assist is not available for this tool")
	}
	var in models.MoodAssistInput
	if err := json.Unmarshal(payload, &in); err != nil {
		return models.MoodAssistResult{}, validationErr("bad assist request: %v", err)
	}
	if strings.TrimSpace(in.Thought) == "" {
		return models.MoodAssistResult{}, validationErr("write the negative thought first")
	}

	var instructions, prompt string
	switch in.Mode {
	case "distortions":
		instructions = distortionsInstructions()
		prompt = assistContext(in)
	case "responses":
		instructions = responsesInstructions()
		prompt = assistContext(in)
	default:
		return models.MoodAssistResult{}, validationErr("unknown assist mode %q", in.Mode)
	}

	raw, err := s.completeWithOpenAI(instructions, prompt)
	if err != nil {
		return models.MoodAssistResult{}, err // includes ErrOpenAINotConnected
	}

	var out models.MoodAssistResult
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &out); err != nil {
		return models.MoodAssistResult{}, fmt.Errorf("%w: the coach returned an unexpected response, try again", ErrValidation)
	}
	out.Mode = in.Mode
	if out.Crisis {
		// Never surface coaching alongside a crisis response.
		out.Distortions = nil
		out.Responses = nil
		return out, nil
	}
	// Keep only real distortion codes.
	filtered := out.Distortions[:0]
	for _, d := range out.Distortions {
		if tools.Distortions[strings.ToUpper(strings.TrimSpace(d.Code))] {
			d.Code = strings.ToUpper(strings.TrimSpace(d.Code))
			filtered = append(filtered, d)
		}
	}
	out.Distortions = filtered
	return out, nil
}

func assistContext(in models.MoodAssistInput) string {
	var b strings.Builder
	if strings.TrimSpace(in.Event) != "" {
		fmt.Fprintf(&b, "Situation: %s\n", strings.TrimSpace(in.Event))
	}
	fmt.Fprintf(&b, "Negative thought: %q\n", strings.TrimSpace(in.Thought))
	if len(in.Distortions) > 0 {
		fmt.Fprintf(&b, "Distortions the user already tagged: %s\n", strings.Join(in.Distortions, ", "))
	}
	return b.String()
}

func distortionsInstructions() string {
	return safetyPreamble + `

TASK: Identify which of Dr. Burns' 10 cognitive distortions are present in the negative thought. Use ONLY these codes:
AON (All-or-Nothing), OG (Overgeneralization), MF (Mental Filter), DP (Discounting the Positive),
JC (Jumping to Conclusions), MAG (Magnification/Minimization), ER (Emotional Reasoning),
SH (Should Statements), LAB (Labeling), SB (Self/Other-Blame).

Respond with ONLY this JSON object, no prose or fences:
{"crisis":false,"crisis_message":"","distortions":[{"code":"AON","why":"one short phrase explaining how it shows up here"}]}
Include only distortions genuinely present (usually 1-4). If crisis, set crisis fields and leave distortions empty.`
}

func responsesInstructions() string {
	return safetyPreamble + `

TASK: Suggest 2-3 rational responses to the negative thought. Each must be (1) 100% true — never a rationalization — and (2) likely to reduce belief in the negative thought. Write each in the user's own first-person voice, warm and specific. Tag each with the TEAM-CBT method it uses (e.g. "Examine the Evidence", "Double-Standard", "Socratic Method", "Cost-Benefit Analysis", "Reattribution", "Semantic Method", "Be Specific").

Respond with ONLY this JSON object, no prose or fences:
{"crisis":false,"crisis_message":"","responses":[{"technique":"Double-Standard","text":"a positive response in first person"}]}
If crisis, set crisis fields and leave responses empty.`
}
