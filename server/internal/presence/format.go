// Package presence is the Telegram channel of the edi server: a transport,
// exactly like the HTTP handlers — it parses messages, calls services.Service
// methods on the linked user, and formats replies. No business logic lives
// here, and it runs in-process (multi-tenant: each paired chat acts as its
// own user, no impersonation credentials needed).
package presence

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"edi/internal/models"
)

const helpText = `<b>edi</b> — your Life RPG, in your pocket

Just talk to me — "add a 20 min run as a daily", "I finished the tax return", "how's my streak?" (needs your ChatGPT connection).
/new — clear our conversation
/status — level, streak, gold, quests, decay
/quests — active quests with IDs
/done &lt;id&gt; — complete a quest
/supps — today's supplement stack
/supps &lt;name&gt; — take one (full stack = bonus XP)
/ward &lt;attribute&gt; — 7-day decay shield (30g, hardcore mode only)
/rest on|off — quiet mode: nudges stand down (and decay pauses in hardcore)
/story — a narrated episode of your saga (AI)
/boss — forge this week's boss quest (AI)
/briefing — get your briefing right now
/briefing HH:MM — set your daily briefing time
/nudge — check the nudge right now
/nudge HH:MM — set your evening nudge time
/unpair — disconnect this chat
/help — this message`

const notLinkedText = `This chat isn't linked to an edi account yet.

Open edi → AI Coach → <b>Link Telegram</b>, then send the code here:
/pair <i>code</i>`

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
		b.WriteString("☾ rest mode ON — nudges stand down\n")
	}
	if d.DailyPenaltyXP > 0 {
		fmt.Fprintf(&b, "⚠ missed dailies · -%d XP\n", d.DailyPenaltyXP)
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
// shows: while today's set is not yet cleared (completed < goal), at least
// one quest is open, and rest mode is off. It does NOT stand down after one
// completion — that taught the app to go quiet exactly when momentum
// existed. Easiest quest wins (difficulty, then lowest reward).
func nudgeQuest(d models.Dashboard) (*models.Quest, bool) {
	if d.RestMode || d.DailyProgress.CompletedToday >= d.DailyProgress.Goal || len(d.TodayQuests) == 0 {
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

// formatNudge renders the evening nudge as ONE answerable question: the
// smallest open quest and how to log it. Progress-aware — never "nothing
// logged" when something was.
func formatNudge(d models.Dashboard, q models.Quest) string {
	done, goal := d.DailyProgress.CompletedToday, d.DailyProgress.Goal
	if done > 0 {
		return fmt.Sprintf("🌙 %d/%d today. One more?\n%s\n\n/done %d closes it.", done, goal, questLine(q), q.ID)
	}
	return fmt.Sprintf("🌙 Nothing logged today. Smallest step:\n%s\n\n/done %d and the streak lives.", questLine(q), q.ID)
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
// what keeps a suspended/slept host (or a redeploy landing mid-window) from
// firing hours-late notifications.
const fireStaleAfter = 10 * time.Minute

// fireDue reports whether the scheduled fire time has arrived (due) and, if
// so, whether the wake-up came in too late to still send it (stale).
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

// formatSupplements renders today's stack as a checklist with the bonus state.
func formatSupplements(t models.SupplementsToday) string {
	if t.Total == 0 {
		return "💊 Your stack is empty — add supplements in the app (Tools → Supplements) or tell me \"add magnesium to my supplements\"."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "💊 <b>Supplements</b> · %d/%d today\n", t.Taken, t.Total)
	for _, sp := range t.Supplements {
		mark := "▢"
		if sp.Taken {
			mark = "✓"
		}
		fmt.Fprintf(&b, "%s %s", mark, html.EscapeString(sp.Name))
		if sp.Dose != "" {
			fmt.Fprintf(&b, " <i>%s</i>", html.EscapeString(sp.Dose))
		}
		b.WriteString("\n")
	}
	switch {
	case t.BonusAwarded:
		b.WriteString("\n🏅 Full stack — bonus paid.")
	default:
		b.WriteString("\n/supps <i>name</i> to take one · full stack pays a bonus")
	}
	return b.String()
}
