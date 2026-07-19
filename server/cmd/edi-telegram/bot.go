package main

import (
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"

	"edi/internal/apiclient"
	"edi/internal/models"
)

const helpText = `<b>edi</b> — your Life RPG, in your pocket

/status — level, streak, gold, quests, decay
/quests — active quests with IDs
/done &lt;id&gt; — complete a quest
/ward &lt;attribute&gt; — 7-day decay shield (30g)
/rest on|off — pause/resume decay
/help — this message`

// questLine renders one quest as "#id Title (N XP)".
func questLine(q models.Quest) string {
	return fmt.Sprintf("#%d %s <i>(%d XP)</i>", q.ID, html.EscapeString(q.Title), q.TotalReward())
}

// decayLines lists decaying attributes, worst (highest daily loss) first.
func decayLines(attrs []models.Attribute) []string {
	var decaying []models.Attribute
	for _, a := range attrs {
		if a.Decay != nil && a.Decay.State == "decaying" {
			decaying = append(decaying, a)
		}
	}
	sort.Slice(decaying, func(i, j int) bool {
		return decaying[i].Decay.ProjectedDailyLoss > decaying[j].Decay.ProjectedDailyLoss
	})
	var out []string
	for _, a := range decaying {
		out = append(out, fmt.Sprintf("⚠ %s — %dd idle, -%d XP/day",
			html.EscapeString(a.Name), a.Decay.IdleDays, a.Decay.ProjectedDailyLoss))
	}
	return out
}

// statusCore is the shared body of the briefing and /status replies.
func statusCore(d models.Dashboard) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Lv %d · streak %d🔥 · %dg\n", d.Character.Level, d.Streak.Current, d.GoldBalance)
	fmt.Fprintf(&b, "%d quests open · %d/%d done today\n", len(d.TodayQuests), d.DailyProgress.CompletedToday, d.DailyProgress.Goal)
	if d.RestMode {
		b.WriteString("☾ rest mode ON — decay paused\n")
	}
	for _, line := range decayLines(d.Attributes) {
		b.WriteString(line + "\n")
	}
	return b.String()
}

// formatBriefing renders the morning push.
func formatBriefing(d models.Dashboard) string {
	var b strings.Builder
	b.WriteString("☀ <b>edi briefing</b>\n")
	b.WriteString(statusCore(d))
	if len(d.TodayQuests) > 0 {
		b.WriteString("\nToday:\n")
		for _, q := range d.TodayQuests {
			b.WriteString(questLine(q) + "\n")
		}
		b.WriteString("\nComplete with /done <i>id</i>")
	}
	return b.String()
}

// formatStatus renders the /status reply.
func formatStatus(d models.Dashboard) string {
	return "<b>edi status</b>\n" + statusCore(d)
}

// formatQuests renders the /quests reply.
func formatQuests(quests []models.Quest) string {
	if len(quests) == 0 {
		return "No active quests. Add some in the app — or enjoy the calm."
	}
	var b strings.Builder
	b.WriteString("<b>Active quests</b>\n")
	for _, q := range quests {
		b.WriteString(questLine(q) + "\n")
	}
	b.WriteString("\nComplete with /done <i>id</i>")
	return b.String()
}

var difficultyRank = map[string]int{"trivial": 0, "easy": 1, "medium": 2, "hard": 3, "boss": 4}

// nudgeQuest decides whether the evening nudge fires and which quest it
// shows: only when nothing was completed today, at least one quest is open,
// and rest mode is off. Easiest quest wins (difficulty, then lowest reward).
func nudgeQuest(d models.Dashboard) (*models.Quest, bool) {
	if d.RestMode || d.DailyProgress.CompletedToday > 0 || len(d.TodayQuests) == 0 {
		return nil, false
	}
	best := d.TodayQuests[0]
	for _, q := range d.TodayQuests[1:] {
		if difficultyRank[q.Difficulty] < difficultyRank[best.Difficulty] ||
			(difficultyRank[q.Difficulty] == difficultyRank[best.Difficulty] && q.TotalReward() < best.TotalReward()) {
			best = q
		}
	}
	return &best, true
}

// nextFire returns the next local occurrence of hhmm ("15:04") after now.
// Unparseable input falls back to 08:00.
func nextFire(now time.Time, hhmm string) time.Time {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		t, _ = time.Parse("15:04", "08:00")
	}
	local := now.Local()
	fire := time.Date(local.Year(), local.Month(), local.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
	if !fire.After(local) {
		fire = fire.AddDate(0, 0, 1)
	}
	return fire
}

// fireStaleAfter is how far past a scheduled fire time a wake-up may still
// send the push. Beyond this the push is skipped, never replayed — this is
// what keeps a suspended/slept host from firing hours-late notifications.
const fireStaleAfter = 10 * time.Minute

// fireDue reports whether the scheduled fire time has arrived (due) and, if
// so, whether the wake-up came in too late to still send it (stale). due is
// true at/after fire; stale is true once now is more than fireStaleAfter
// past fire.
func fireDue(now, fire time.Time) (due bool, stale bool) {
	if now.Before(fire) {
		return false, false
	}
	return true, now.Sub(fire) > fireStaleAfter
}

// parseCommand splits "/done 42" into ("done", "42"). A "@botname" suffix on
// the command (Telegram group convention) is stripped.
func parseCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	parts := strings.Fields(text)
	cmd := strings.TrimPrefix(parts[0], "/")
	if at := strings.IndexByte(cmd, '@'); at >= 0 {
		cmd = cmd[:at]
	}
	arg := strings.Join(parts[1:], " ")
	return strings.ToLower(cmd), arg
}

// handleCommand executes one incoming message and returns the HTML reply.
// API errors come back as friendly one-liners — apiclient already unwraps
// the server's {error} body into a clean message.
func handleCommand(api *apiclient.Client, text string) string {
	cmd, arg := parseCommand(text)
	switch cmd {
	case "status":
		d, err := api.Dashboard()
		if err != nil {
			return "⚠ " + html.EscapeString(err.Error())
		}
		return formatStatus(d)

	case "quests":
		quests, err := api.ListQuests("", "active")
		if err != nil {
			return "⚠ " + html.EscapeString(err.Error())
		}
		return formatQuests(quests)

	case "done":
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return "Usage: /done <i>id</i> — get ids from /quests"
		}
		result, err := api.CompleteQuest(id)
		if err != nil {
			return "⚠ " + html.EscapeString(err.Error())
		}
		var xp int64
		for _, e := range result.XPEvents {
			xp += e.Amount
		}
		reply := fmt.Sprintf("✓ <b>%s</b> complete!\n+%d XP · +%dg · streak %d🔥",
			html.EscapeString(result.Quest.Title), xp, result.Gold, result.Dashboard.Streak.Current)
		for _, lu := range result.LevelUps {
			reply += fmt.Sprintf("\n⬆ %s reached Lv %d!", html.EscapeString(lu.AttributeName), lu.ToLevel)
		}
		return reply

	case "ward":
		if arg == "" {
			return "Usage: /ward <i>attribute</i> (e.g. /ward focus)"
		}
		res, err := api.WardAttribute(strings.ToLower(arg))
		if err != nil {
			return "⚠ " + html.EscapeString(err.Error())
		}
		return fmt.Sprintf("🛡 %s warded until %s. Balance: %dg",
			html.EscapeString(res.Ward.AttributeKey), res.Ward.ExpiresAt.Local().Format("Jan 2 15:04"), res.Balance)

	case "rest":
		switch arg {
		case "on", "off":
			state, err := api.SetRestMode(arg == "on")
			if err != nil {
				return "⚠ " + html.EscapeString(err.Error())
			}
			if state.On {
				return "☾ Rest mode ON — decay paused. Recover well."
			}
			return "☀ Rest mode OFF — idle clocks restarted."
		default:
			return "Usage: /rest on|off"
		}

	case "help", "start":
		return helpText

	default:
		return helpText
	}
}
