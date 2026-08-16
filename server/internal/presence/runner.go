package presence

import (
	"context"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"edi/internal/services"
	"edi/internal/telegram"
)

// Runner is the in-process Telegram presence channel: one long-poll loop for
// every paired user, plus per-user briefing/nudge schedulers.
type Runner struct {
	svc *services.Service // base service; per-chat work runs on svc.ForUser
	tg  *telegram.Client

	defaultBriefing string // HH:MM fallbacks when a user hasn't set their own
	defaultNudge    string

	mu    sync.Mutex
	fires map[fireKey]time.Time // next scheduled fire per (user, kind)
}

type fireKey struct {
	userID int64
	kind   string // "briefing" | "nudge"
}

// New builds a runner. defaultBriefing/defaultNudge must be valid HH:MM.
func New(svc *services.Service, tg *telegram.Client, defaultBriefing, defaultNudge string) *Runner {
	return &Runner{svc: svc, tg: tg, defaultBriefing: defaultBriefing, defaultNudge: defaultNudge, fires: map[fireKey]time.Time{}}
}

// Run starts the channel: resolves the bot identity, then long-polls updates
// and ticks the push scheduler until ctx is done. Only ONE process may poll a
// bot token (Telegram 409s concurrent getUpdates) — so exactly one
// environment sets TELEGRAM_BOT_TOKEN.
func (r *Runner) Run(ctx context.Context) {
	me, err := r.tg.GetMe()
	if err != nil {
		log.Printf("telegram disabled: getMe failed: %v", err)
		return
	}
	r.svc.SetTelegramBotInfo(me.Username)
	log.Printf("telegram presence up as @%s (briefing %s, nudge %s local)", me.Username, r.defaultBriefing, r.defaultNudge)

	go r.pushLoop(ctx)
	r.pollLoop(ctx)
}

// pollLoop is the main long-poll cycle. Updates are ack'd via the offset,
// which lives in memory: a redeploy can replay the last batch — safe, because
// every state-changing command is idempotent server-side (the completion
// gate, upsert pairing) or harmlessly repeatable.
func (r *Runner) pollLoop(ctx context.Context) {
	var offset int64
	for ctx.Err() == nil {
		updates, err := r.tg.GetUpdates(offset, 50)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("telegram getUpdates: %v (retrying in 5s)", err)
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil {
				continue
			}
			reply := r.handleMessage(u.Message.Chat.ID, u.Message.Text)
			if reply == "" {
				continue
			}
			if err := r.tg.SendMessage(u.Message.Chat.ID, reply); err != nil {
				log.Printf("telegram sendMessage: %v", err)
			}
		}
	}
}

// handleMessage routes one incoming message. Unpaired chats only get pairing
// help — every account action requires a link.
func (r *Runner) handleMessage(chatID int64, text string) string {
	cmd, arg := parseCommand(text)
	if cmd == "" {
		return ""
	}

	// Pairing works from any chat. The t.me deep link arrives as "/start <code>".
	if cmd == "pair" || (cmd == "start" && arg != "") {
		if arg == "" {
			return "Usage: /pair <i>code</i> — get a code from the app (AI Coach → Link Telegram)"
		}
		u, err := r.svc.ClaimTelegramPairCode(arg, chatID)
		if err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		return fmt.Sprintf("✓ Linked to <b>%s</b>. You're set — try /status", html.EscapeString(u.Name))
	}

	userID, err := r.svc.UserIDForTelegramChat(chatID)
	if err != nil {
		return notLinkedText
	}
	return r.handleCommand(r.svc.ForUser(userID), chatID, cmd, arg)
}

// handleCommand executes one command for a linked user and returns the HTML
// reply. Service errors come back as friendly one-liners.
func (r *Runner) handleCommand(svc *services.Service, chatID int64, cmd, arg string) string {
	switch cmd {
	case "status":
		d, err := svc.GetDashboard()
		if err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		return formatStatus(d)

	case "quests":
		quests, err := svc.ListQuests("", "active")
		if err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		return formatQuests(quests)

	case "done":
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return "Usage: /done <i>id</i> — get ids from /quests"
		}
		result, err := svc.CompleteQuest(id)
		if err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
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
		res, err := svc.WardAttribute(strings.ToLower(arg))
		if err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		return fmt.Sprintf("🛡 %s warded until %s. Balance: %dg",
			html.EscapeString(res.Ward.AttributeKey), res.Ward.ExpiresAt.Local().Format("Jan 2 15:04"), res.Balance)

	case "rest":
		switch arg {
		case "on", "off":
			state, err := svc.SetRestMode(arg == "on")
			if err != nil {
				return "⚠ " + html.EscapeString(userMessage(err))
			}
			if state.On {
				return "☾ Rest mode ON — decay paused. Recover well."
			}
			return "☀ Rest mode OFF — idle clocks restarted."
		default:
			return "Usage: /rest on|off"
		}

	case "briefing", "nudge":
		if arg == "" {
			return fmt.Sprintf("Usage: /%s <i>HH:MM</i> (e.g. /%s 07:30)", cmd, cmd)
		}
		if err := svc.SetTelegramPushTime(cmd, arg); err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		r.resetFire(chatOwner(svc), cmd)
		return fmt.Sprintf("✓ %s time set to %s (your local server time)", cmd, html.EscapeString(arg))

	case "unpair":
		if err := svc.UnlinkTelegramChat(chatID); err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		return "Unlinked. Pair again anytime with a fresh code from the app."

	case "help", "start":
		return helpText

	default:
		return helpText
	}
}

// chatOwner extracts the bound user id (svc is always a ForUser copy here).
func chatOwner(svc *services.Service) int64 { return svc.UserID() }

// userMessage strips the internal "validation error: " prefix for chat display.
func userMessage(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && strings.HasPrefix(msg, "validation error") {
		msg = msg[i+2:]
	}
	return msg
}

// --- push scheduler -----------------------------------------------------------

// pushCheckInterval bounds how long the scheduler sleeps in one stretch, so it
// re-checks the wall clock periodically instead of trusting one long monotonic
// sleep — the monotonic clock pauses across host suspend, which otherwise
// makes pushes fire hours late.
const pushCheckInterval = time.Minute

// pushLoop ticks the wall clock and fires due pushes for every paired user at
// their configured (or default) local time. Fire times initialize to the NEXT
// occurrence — nothing fires just because the server started after the
// scheduled minute (no duplicate briefings on redeploy; a push whose window
// was crossed during a redeploy is skipped, never replayed).
func (r *Runner) pushLoop(ctx context.Context) {
	for {
		select {
		case <-time.After(pushCheckInterval):
		case <-ctx.Done():
			return
		}
		links, err := r.svc.ListTelegramLinks()
		if err != nil {
			log.Printf("telegram scheduler: list links: %v", err)
			continue
		}
		now := time.Now()
		for _, l := range links {
			r.tick(now, l.UserID, l.ChatID, "briefing", r.defaultBriefing, r.sendBriefing)
			r.tick(now, l.UserID, l.ChatID, "nudge", r.defaultNudge, r.sendNudge)
		}
	}
}

// tick advances one (user, kind) schedule: initializes the next fire on first
// sight, fires when due (retrying 3× at 30s spacing), and skips stale fires.
func (r *Runner) tick(now time.Time, userID, chatID int64, kind, defaultHHMM string, send func(*services.Service, int64) error) {
	svc := r.svc.ForUser(userID)
	hhmm, err := svc.TelegramPushTime(kind)
	if err != nil || hhmm == "" {
		hhmm = defaultHHMM
	}

	key := fireKey{userID, kind}
	r.mu.Lock()
	fire, seen := r.fires[key]
	if !seen {
		fire = nextFire(now, hhmm)
		r.fires[key] = fire
	}
	r.mu.Unlock()
	if !seen {
		return
	}

	due, stale := fireDue(now, fire)
	if !due {
		// A changed time setting re-anchors via resetFire; otherwise wait.
		return
	}
	if stale {
		log.Printf("telegram %s for user %d skipped: woke %s past fire time", kind, userID, now.Sub(fire).Round(time.Second))
	} else {
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if err = send(svc, chatID); err == nil {
				break
			}
			if attempt < 2 {
				time.Sleep(30 * time.Second)
			}
		}
		if err != nil {
			log.Printf("telegram %s for user %d failed: %v", kind, userID, err)
		}
	}
	r.mu.Lock()
	r.fires[key] = nextFire(time.Now(), hhmm)
	r.mu.Unlock()
}

// resetFire clears a schedule so the next tick re-derives it (after /briefing
// or /nudge changes the time).
func (r *Runner) resetFire(userID int64, kind string) {
	r.mu.Lock()
	delete(r.fires, fireKey{userID, kind})
	r.mu.Unlock()
}

// sendBriefing pushes the morning briefing.
func (r *Runner) sendBriefing(svc *services.Service, chatID int64) error {
	d, err := svc.GetDashboard()
	if err != nil {
		return err
	}
	return r.tg.SendMessage(chatID, formatBriefing(d))
}

// sendNudge pushes the evening nudge — only when nudgeQuest says so.
func (r *Runner) sendNudge(svc *services.Service, chatID int64) error {
	d, err := svc.GetDashboard()
	if err != nil {
		return err
	}
	q, ok := nudgeQuest(d)
	if !ok {
		return nil
	}
	msg := fmt.Sprintf("🌙 Nothing logged today. Smallest step:\n%s\n\n/done %d and the streak lives.", questLine(*q), q.ID)
	return r.tg.SendMessage(chatID, msg)
}
