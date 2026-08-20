package presence

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"edi/internal/agent"
	"edi/internal/services"
	"edi/internal/telegram"
)

// Runner is the in-process Telegram presence channel: one long-poll loop for
// every paired user, plus per-user briefing/nudge schedulers.
type Runner struct {
	svc *services.Service // base service; per-chat work runs on svc.ForUser
	tg  *telegram.Client

	// Free-text chat: the user's ChatGPT model acting through the shared tool
	// registry (internal/agent). llmFor is swappable for offline tests.
	registry *agent.Registry
	sessions *agent.Sessions
	llmFor   func(*services.Service) agent.LLM

	defaultBriefing string // HH:MM fallbacks when a user hasn't set their own
	defaultNudge    string

	mu    sync.Mutex
	fires map[fireKey]fire // next scheduled fire per (user, kind)
}

// fire is one scheduled push plus the HH:MM it was derived from — when the
// setting changes (from ANY client: web, CLI, agent tool, /briefing), the
// next tick sees the mismatch and re-anchors.
type fire struct {
	at   time.Time
	hhmm string
}

type fireKey struct {
	userID int64
	kind   string // "briefing" | "nudge"
}

// New builds a runner. defaultBriefing/defaultNudge must be valid HH:MM.
func New(svc *services.Service, tg *telegram.Client, registry *agent.Registry, defaultBriefing, defaultNudge string) *Runner {
	return &Runner{
		svc: svc, tg: tg, registry: registry, sessions: agent.NewSessions(),
		llmFor:          func(s *services.Service) agent.LLM { return s.OpenAIConverse },
		defaultBriefing: defaultBriefing, defaultNudge: defaultNudge, fires: map[fireKey]fire{},
	}
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
			msg := u.Message
			if isFreeText(msg.Text) && isPrivateChat(msg.Chat.Type) {
				// Chat replies take seconds (LLM + tools): answer off the poll
				// loop so one slow conversation never stalls the other users'
				// commands. Turns of the same chat are serialized in Sessions.
				go r.answerChat(msg.Chat.ID, msg.Text)
				continue
			}
			reply := r.handleMessage(msg.Chat.ID, msg.Text)
			if reply == "" {
				continue
			}
			if err := r.tg.SendMessage(msg.Chat.ID, reply); err != nil {
				log.Printf("telegram sendMessage: %v", err)
			}
		}
	}
}

// isFreeText reports whether a message is conversation rather than a slash
// command (which parseCommand handles).
func isFreeText(text string) bool {
	text = strings.TrimSpace(text)
	return text != "" && !strings.HasPrefix(text, "/")
}

// isPrivateChat gates free-text chat to 1:1 chats: in a group (privacy mode
// off) every member's chatter would spend the paired user's ChatGPT quota and
// act on THEIR account. Group free text falls through to the command path
// (help), like before. "" (older payloads, tests) counts as private.
func isPrivateChat(chatType string) bool { return chatType == "" || chatType == "private" }

// answerChat sends one free-text message through the agent and replies,
// keeping Telegram's ~5s typing indicator alive while the model works.
func (r *Runner) answerChat(chatID int64, text string) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(4 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = r.tg.SendTyping(chatID)
			case <-done:
				return
			}
		}
	}()
	reply := r.chatReply(chatID, text)
	close(done)
	if err := r.tg.SendMessage(chatID, reply); err != nil {
		log.Printf("telegram sendMessage: %v", err)
	}
}

// chatReply produces the HTML reply for a free-text message: pairing help
// for unknown chats, the AI-connection hint when no ChatGPT account is
// linked, otherwise the agent's answer. Model text is escaped whole — it may
// quote user-derived titles and SendMessage speaks HTML.
func (r *Runner) chatReply(chatID int64, text string) string {
	userID, err := r.svc.UserIDForTelegramChat(chatID)
	if err != nil {
		return notLinkedText
	}
	svc := r.svc.ForUser(userID)
	if r.registry == nil {
		return helpText
	}
	_ = r.tg.SendTyping(chatID)
	res, err := r.registry.Chat(svc, r.llmFor(svc), r.sessions, sessionKey(chatID), text)
	if err != nil {
		if errors.Is(err, services.ErrOpenAINotConnected) {
			return "💬 Free-text chat needs your ChatGPT account: open edi → AI → <b>Connect ChatGPT</b>. Slash commands work without it — /help"
		}
		log.Printf("telegram chat for user %d: %v", userID, err)
		return "⚠ " + html.EscapeString(userMessage(err))
	}
	return html.EscapeString(res.Reply)
}

func sessionKey(chatID int64) string { return "telegram:" + strconv.FormatInt(chatID, 10) }

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
		if result.Crit {
			reply = "💥 <b>CRITICAL HIT!</b>\n" + reply
		}
		if result.ComboMultiplier > 1 {
			reply += fmt.Sprintf("\n🔗 combo ×%.2g — keep the chain going", result.ComboMultiplier)
		}
		if d := result.Drop; d != nil {
			reply += fmt.Sprintf("\n%s <b>%s DROP!</b> %s — %s", d.Icon, strings.ToUpper(d.Rarity), html.EscapeString(d.Name), html.EscapeString(d.Flavor))
		}
		for _, lu := range result.LevelUps {
			reply += fmt.Sprintf("\n⬆ %s reached Lv %d!", html.EscapeString(lu.AttributeName), lu.ToLevel)
		}
		for _, a := range result.AchievementsUnlocked {
			reply += fmt.Sprintf("\n🏆 <b>Achievement unlocked:</b> %s %s", a.Icon, html.EscapeString(a.Name))
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
		// No argument = send it right now; an HH:MM argument sets the daily time.
		if arg == "" {
			d, err := svc.GetDashboard()
			if err != nil {
				return "⚠ " + html.EscapeString(userMessage(err))
			}
			if cmd == "briefing" {
				return formatBriefing(d)
			}
			q, ok := nudgeQuest(d)
			if !ok {
				return "Nothing to nudge about — you've logged progress today (or rest mode is on). 🔥"
			}
			return fmt.Sprintf("🌙 Nothing logged today. Smallest step:\n%s\n\n/done %d and the streak lives.", questLine(*q), q.ID)
		}
		if err := svc.SetTelegramPushTime(cmd, arg); err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		return fmt.Sprintf("✓ %s time set to %s (your local server time)", cmd, html.EscapeString(arg))

	case "story":
		story, err := r.narrate(svc)
		if err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		return "📜 <i>" + html.EscapeString(story) + "</i>"

	case "boss":
		q, err := svc.ForgeBoss()
		if err != nil {
			return "⚠ " + html.EscapeString(userMessage(err))
		}
		return fmt.Sprintf("⚔️ <b>A boss has been forged:</b>\n%s\n<i>%s</i>\n\n/done %d when you bring it down.",
			questLine(q), html.EscapeString(q.Description), q.ID)

	case "new", "forget":
		r.sessions.Reset(sessionKey(chatID))
		return "🧹 Conversation cleared — fresh start."

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

// narrationTimeout bounds every LLM narration call — a hung model must never
// stall a chat reply or the push loop.
const narrationTimeout = 15 * time.Second

// narrate runs StoryNarration with a hard deadline (the stray goroutine of a
// timed-out call finishes in the background and is discarded).
func (r *Runner) narrate(svc *services.Service) (string, error) {
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		s, err := svc.StoryNarration()
		ch <- res{s, err}
	}()
	select {
	case out := <-ch:
		return out.s, out.err
	case <-time.After(narrationTimeout):
		return "", fmt.Errorf("the narrator took too long — try again")
	}
}

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
			r.tick(now, l.UserID, l.ChatID, "briefing", r.defaultBriefing, r.buildBriefing)
			r.tick(now, l.UserID, l.ChatID, "nudge", r.defaultNudge, r.buildNudge)
		}
	}
}

// tick advances one (user, kind) schedule: initializes the next fire on first
// sight, fires when due (retrying 3× at 30s spacing), and skips stale fires.
func (r *Runner) tick(now time.Time, userID, chatID int64, kind, defaultHHMM string, build func(*services.Service) (string, error)) {
	svc := r.svc.ForUser(userID)
	hhmm, err := svc.TelegramPushTime(kind)
	if err != nil || hhmm == "" {
		hhmm = defaultHHMM
	}

	key := fireKey{userID, kind}
	r.mu.Lock()
	f, seen := r.fires[key]
	if !seen || f.hhmm != hhmm {
		// First sight, or the time was changed since we anchored — re-derive
		// from now, never firing retroactively.
		r.fires[key] = fire{at: nextFire(now, hhmm), hhmm: hhmm}
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	due, stale := fireDue(now, f.at)
	if !due {
		return
	}
	if stale {
		log.Printf("telegram %s for user %d skipped: woke %s past fire time", kind, userID, now.Sub(f.at).Round(time.Second))
	} else if msg, err := build(svc); err != nil {
		log.Printf("telegram %s for user %d failed to build: %v", kind, userID, err)
	} else if msg != "" { // "" = nothing to push (e.g. nudge stands down)
		// The message is built ONCE (an LLM narration must not re-bill);
		// only the Telegram send retries.
		for attempt := 0; attempt < 3; attempt++ {
			if err = r.tg.SendMessage(chatID, msg); err == nil {
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
	r.fires[key] = fire{at: nextFire(time.Now(), hhmm), hhmm: hhmm}
	r.mu.Unlock()
}

// buildBriefing renders the morning briefing, opening with a narrated
// episode when the user has AI connected (fail-soft: any narration problem
// falls back to the plain briefing).
func (r *Runner) buildBriefing(svc *services.Service) (string, error) {
	d, err := svc.GetDashboard()
	if err != nil {
		return "", err
	}
	msg := formatBriefing(d)
	if story, err := r.narrate(svc); err == nil && story != "" {
		msg = "📜 <i>" + html.EscapeString(story) + "</i>\n\n" + msg
	}
	return msg, nil
}

// buildNudge renders the evening nudge — "" when it stands down.
func (r *Runner) buildNudge(svc *services.Service) (string, error) {
	d, err := svc.GetDashboard()
	if err != nil {
		return "", err
	}
	q, ok := nudgeQuest(d)
	if !ok {
		return "", nil
	}
	return fmt.Sprintf("🌙 Nothing logged today. Smallest step:\n%s\n\n/done %d and the streak lives.", questLine(*q), q.ID), nil
}
