package presence

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"edi/internal/agent"
	"edi/internal/db/dbtest"
	"edi/internal/models"
	"edi/internal/openai"
	"edi/internal/services"
	"edi/internal/telegram"
)

// stubTelegram records every sendMessage text and returns ok for everything —
// the runner is exercised against real services (dbtest Postgres), only the
// Telegram wire is faked.
func stubTelegram(t *testing.T) (*telegram.Client, *[]string) {
	t.Helper()
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") || strings.HasSuffix(r.URL.Path, "/editMessageText") {
			_ = r.ParseForm()
			// Edits are recorded with a marker so tests can tell receipts
			// from fresh sends; a keyboard is appended as its raw JSON.
			text := r.Form.Get("text")
			if strings.HasSuffix(r.URL.Path, "/editMessageText") {
				text = "EDIT:" + text
			}
			if m := r.Form.Get("reply_markup"); m != "" {
				text += "\nKEYBOARD:" + m
			}
			sent = append(sent, text)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"edi_test_bot"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)
	tg := telegram.New("test-token")
	tg.BaseURL = srv.URL
	return tg, &sent
}

func newTestRunner(t *testing.T) (*Runner, *services.Service, *[]string) {
	t.Helper()
	store := dbtest.Open(t)
	if err := store.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := services.New(store, 1)
	svc.SetTelegramBotInfo("edi_test_bot")
	tg, sent := stubTelegram(t)
	return New(svc, tg, agent.NewRegistry(), "08:00", "20:00"), svc, sent
}

// The Telegram sibling of TestTenantIsolation: two users, two chats — each
// chat acts as ITS user only.
func TestPresenceMultiUserIsolation(t *testing.T) {
	r, svc, _ := newTestRunner(t)
	t.Setenv("EDI_INVITE_CODE", "sesame")

	const chatA, chatB = int64(1001), int64(2002)

	// Unknown chats only get pairing help — no account data, no commands.
	if got := r.handleMessage(chatA, "/status"); !strings.Contains(got, "isn't linked") {
		t.Fatalf("unlinked /status = %q, want pairing help", got)
	}
	if got := r.handleMessage(chatA, "/done 1"); !strings.Contains(got, "isn't linked") {
		t.Fatalf("unlinked /done = %q, want pairing help", got)
	}

	// Pair chat A to user 1 (seeded) and chat B to a fresh user 2.
	codeA, err := svc.CreateTelegramPairCode()
	if err != nil {
		t.Fatalf("pair code A: %v", err)
	}
	if got := r.handleMessage(chatA, "/pair "+codeA.Code); !strings.Contains(got, "Linked to") {
		t.Fatalf("pair A = %q", got)
	}

	created, err := svc.RegisterUser(models.RegisterInput{Name: "Blank", InviteCode: "sesame"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	codeB, err := svc.ForUser(created.User.ID).CreateTelegramPairCode()
	if err != nil {
		t.Fatalf("pair code B: %v", err)
	}
	// Deep-link form: "/start <code>" must pair exactly like /pair.
	if got := r.handleMessage(chatB, "/start "+codeB.Code); !strings.Contains(got, "Linked to <b>Blank</b>") {
		t.Fatalf("deep-link pair B = %q", got)
	}

	// A pair code is single-use: replaying it fails.
	if got := r.handleMessage(int64(3003), "/pair "+codeA.Code); !strings.Contains(got, "unknown or expired") {
		t.Fatalf("replayed code = %q, want rejection", got)
	}

	// Chat A (user 1, seeded) sees quests; chat B (blank user) sees none.
	if got := r.handleMessage(chatA, "/quests"); !strings.Contains(got, "30 minute workout") {
		t.Fatalf("chat A /quests = %q, want the seeded list", got)
	}
	if got := r.handleMessage(chatB, "/quests"); !strings.Contains(got, "No active quests") {
		t.Fatalf("chat B /quests = %q, want empty", got)
	}

	// Chat B cannot complete user 1's quest by id — 404s like any client.
	quests, _ := svc.ListQuests("", "active")
	if len(quests) == 0 {
		t.Fatal("no seeded quests")
	}
	target := quests[0]
	if got := r.handleMessage(chatB, "/done "+itoa(target.ID)); !strings.Contains(got, "⚠") {
		t.Fatalf("chat B completing A's quest = %q, want an error", got)
	}
	// And user 1's quest is untouched.
	after, _ := svc.ListQuests("", "active")
	found := false
	for _, q := range after {
		if q.ID == target.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("A's quest must still be active after B's attempt")
	}

	// Chat A completes it for real.
	if got := r.handleMessage(chatA, "/done "+itoa(target.ID)); !strings.Contains(got, "complete!") {
		t.Fatalf("chat A /done = %q", got)
	}

	// Unpair: chat B loses access.
	if got := r.handleMessage(chatB, "/unpair"); !strings.Contains(got, "Unlinked") {
		t.Fatalf("/unpair = %q", got)
	}
	if got := r.handleMessage(chatB, "/status"); !strings.Contains(got, "isn't linked") {
		t.Fatalf("post-unpair /status = %q, want pairing help", got)
	}
}

// A chat can belong to one user only; a user re-pairing moves their link.
func TestPresencePairingCollisions(t *testing.T) {
	r, svc, _ := newTestRunner(t)
	t.Setenv("EDI_INVITE_CODE", "sesame")

	const chat = int64(5005)
	codeA, _ := svc.CreateTelegramPairCode()
	if got := r.handleMessage(chat, "/pair "+codeA.Code); !strings.Contains(got, "Linked to") {
		t.Fatalf("pair = %q", got)
	}

	// Another user trying to claim the SAME chat is refused.
	created, err := svc.RegisterUser(models.RegisterInput{Name: "Other", InviteCode: "sesame"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	codeB, _ := svc.ForUser(created.User.ID).CreateTelegramPairCode()
	if got := r.handleMessage(chat, "/pair "+codeB.Code); !strings.Contains(got, "already linked") {
		t.Fatalf("chat takeover = %q, want refusal", got)
	}

	// User 1 re-pairing from a NEW chat moves the link there.
	codeA2, _ := svc.CreateTelegramPairCode()
	if got := r.handleMessage(int64(6006), "/pair "+codeA2.Code); !strings.Contains(got, "Linked to") {
		t.Fatalf("re-pair = %q", got)
	}
	if uid, err := svc.UserIDForTelegramChat(6006); err != nil || uid != 1 {
		t.Fatalf("new chat resolves to (%d, %v), want (1, nil)", uid, err)
	}
	if _, err := svc.UserIDForTelegramChat(chat); err == nil {
		t.Fatal("old chat must be unlinked after re-pair")
	}
}

// /briefing and /nudge store per-user times and reject garbage.
func TestPresencePushTimeCommands(t *testing.T) {
	r, svc, _ := newTestRunner(t)
	const chat = int64(7007)
	code, _ := svc.CreateTelegramPairCode()
	r.handleMessage(chat, "/pair "+code.Code)

	if got := r.handleMessage(chat, "/briefing 07:15"); !strings.Contains(got, "07:15") {
		t.Fatalf("/briefing = %q", got)
	}
	if v, _ := svc.TelegramPushTime("briefing"); v != "07:15" {
		t.Errorf("stored briefing time = %q, want 07:15", v)
	}
	if got := r.handleMessage(chat, "/nudge 25:99"); !strings.Contains(got, "⚠") {
		t.Fatalf("bad time = %q, want validation error", got)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// /briefing and /nudge with no argument fire on demand instead of waiting for
// the scheduled time.
func TestPresenceOnDemandPushes(t *testing.T) {
	r, svc, _ := newTestRunner(t)
	const chat = int64(8008)
	code, _ := svc.CreateTelegramPairCode()
	r.handleMessage(chat, "/pair "+code.Code)

	// Briefing now: full dashboard briefing (seeded user has quests).
	if got := r.handleMessage(chat, "/briefing"); !strings.Contains(got, "edi briefing") || !strings.Contains(got, "/done") {
		t.Fatalf("/briefing now = %q, want the briefing", got)
	}

	// Nudge now: nothing completed today -> names the easiest quest.
	if got := r.handleMessage(chat, "/nudge"); !strings.Contains(got, "Nothing logged today") {
		t.Fatalf("/nudge now = %q, want the nudge", got)
	}

	// One completion does NOT silence it: the nudge acknowledges progress
	// and asks for one more until today's set is cleared.
	quests, _ := svc.ListQuests("", "active")
	if _, err := svc.CompleteQuest(quests[0].ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got := r.handleMessage(chat, "/nudge"); !strings.Contains(got, "today. One more?") {
		t.Fatalf("/nudge after progress = %q, want a progress-aware nudge", got)
	}
	// Clearing the set (every daily done) stands it down.
	for _, q := range quests[1:] {
		if q.Type == "daily" {
			if _, err := svc.CompleteQuest(q.ID); err != nil {
				t.Fatalf("complete %d: %v", q.ID, err)
			}
		}
	}
	d, _ := svc.GetDashboard()
	if d.DailyProgress.CompletedToday < d.DailyProgress.Goal {
		t.Fatalf("set not cleared: %d/%d", d.DailyProgress.CompletedToday, d.DailyProgress.Goal)
	}
	if got := r.handleMessage(chat, "/nudge"); !strings.Contains(got, "Nothing to nudge about") {
		t.Fatalf("/nudge after clearing the set = %q, want stand-down message", got)
	}

	// Setting times still works with an argument.
	if got := r.handleMessage(chat, "/briefing 06:45"); !strings.Contains(got, "06:45") {
		t.Fatalf("/briefing HH:MM = %q", got)
	}
}

// Free text (no leading slash) is conversation, not a command: unpaired chats
// get pairing help, paired chats without a ChatGPT connection get the connect
// hint, and with a (faked) model the reply is the agent's text — HTML-escaped
// whole, since it may quote user-derived titles.
func TestPresenceFreeTextChat(t *testing.T) {
	r, svc, _ := newTestRunner(t)
	const chat = int64(7007)

	if !isFreeText("hey add a run") || isFreeText("/status") || isFreeText("  ") {
		t.Fatal("isFreeText misclassifies")
	}
	if !isPrivateChat("private") || !isPrivateChat("") || isPrivateChat("group") || isPrivateChat("supergroup") {
		t.Fatal("free-text chat must be private-chat only")
	}
	if got := r.chatReply(chat, "hey add a run"); !strings.Contains(got, "isn't linked") {
		t.Fatalf("unlinked chat = %q, want pairing help", got)
	}
	code, _ := svc.CreateTelegramPairCode()
	r.handleMessage(chat, "/pair "+code.Code)

	// No ChatGPT connection → clear hint, not an internal error.
	if got := r.chatReply(chat, "hey add a run"); !strings.Contains(got, "Connect ChatGPT") {
		t.Fatalf("not-connected reply = %q", got)
	}

	// Fake model: creates a quest with a title containing HTML, then answers
	// with the model text (which quotes that title).
	r.llmFor = func(*services.Service) agent.LLM {
		calls := 0
		return func(_ string, _ []openai.Item, _ []openai.ToolDef) (openai.Turn, error) {
			calls++
			if calls == 1 {
				item, _ := json.Marshal(map[string]any{"type": "function_call", "call_id": "c1", "name": "create_quest", "arguments": `{"title":"<b>Run</b> & stretch"}`})
				return openai.Turn{ToolCalls: []openai.ToolCall{{CallID: "c1", Name: "create_quest", Arguments: json.RawMessage(`{"title":"<b>Run</b> & stretch"}`)}}, Output: []openai.Item{item}}, nil
			}
			return openai.Turn{Text: "Added <b>Run</b> & stretch as a daily."}, nil
		}
	}
	got := r.chatReply(chat, "add run & stretch as a daily")
	if !strings.Contains(got, "&lt;b&gt;Run&lt;/b&gt; &amp; stretch") || strings.Contains(got, "<b>") {
		t.Fatalf("model text must be escaped whole, got %q", got)
	}
	quests, _ := svc.ListQuests("daily", "active")
	found := false
	for _, q := range quests {
		found = found || q.Title == "<b>Run</b> & stretch"
	}
	if !found {
		t.Fatal("chat did not create the quest through the registry")
	}
	if got := r.handleMessage(chat, "/new"); !strings.Contains(got, "cleared") {
		t.Fatalf("/new = %q", got)
	}
}

// The scheduler re-anchors when the push time changes through the service —
// whichever client wrote it (web/CLI/agent via POST /api/telegram/push-times,
// or /briefing in chat). Clearing ("") falls back to the server default.
func TestPresenceScheduleFollowsServiceSetting(t *testing.T) {
	r, svc, sent := newTestRunner(t)
	const chat = int64(9009)
	code, _ := svc.CreateTelegramPairCode()
	r.handleMessage(chat, "/pair "+code.Code)
	user := svc.UserID()
	now := time.Date(2026, 8, 20, 9, 0, 0, 0, time.Local)
	noop := func(*services.Service) (push, error) { return push{}, nil }

	r.tick(now, user, chat, "briefing", "08:00", noop) // anchors on default
	if f := r.fires[fireKey{user, "briefing"}]; f.hhmm != "08:00" || f.at.Hour() != 8 {
		t.Fatalf("initial anchor = %+v, want default 08:00", f)
	}

	hh := "10:30"
	if _, err := svc.SetTelegramPushTimes(models.TelegramPushTimesPatch{Briefing: &hh}); err != nil {
		t.Fatal(err)
	}
	r.tick(now.Add(time.Minute), user, chat, "briefing", "08:00", noop)
	f := r.fires[fireKey{user, "briefing"}]
	if f.hhmm != "10:30" || f.at.Hour() != 10 || f.at.Minute() != 30 || f.at.Day() != now.Day() {
		t.Fatalf("after change = %+v, want re-anchored to today 10:30", f)
	}
	if len(*sent) != 0 {
		t.Fatalf("re-anchoring must never fire: %v", *sent)
	}

	empty := ""
	got, err := svc.SetTelegramPushTimes(models.TelegramPushTimesPatch{Briefing: &empty})
	if err != nil || got.Briefing != "" {
		t.Fatalf("clear: got %+v err %v", got, err)
	}
	r.tick(now.Add(2*time.Minute), user, chat, "briefing", "08:00", noop)
	if f := r.fires[fireKey{user, "briefing"}]; f.hhmm != "08:00" {
		t.Fatalf("after clear = %+v, want default 08:00", f)
	}
	bad := "25:99"
	if _, err := svc.SetTelegramPushTimes(models.TelegramPushTimesPatch{Nudge: &bad}); !errors.Is(err, services.ErrValidation) {
		t.Fatalf("bad time err = %v, want ErrValidation", err)
	}
}

// /supps lists the stack; /supps <name> takes one (full stack pays the bonus).
func TestPresenceSupplementCommands(t *testing.T) {
	r, svc, _ := newTestRunner(t)
	const chat = int64(8008)
	code, _ := svc.CreateTelegramPairCode()
	r.handleMessage(chat, "/pair "+code.Code)

	if got := r.handleMessage(chat, "/supps"); !strings.Contains(got, "stack is empty") {
		t.Fatalf("/supps on empty stack = %q", got)
	}
	if _, err := svc.AddSupplement(models.SupplementInput{Name: "Magnesium", Dose: "400 mg"}); err != nil {
		t.Fatal(err)
	}
	if got := r.handleMessage(chat, "/supps"); !strings.Contains(got, "0/1 today") || !strings.Contains(got, "▢ Magnesium") {
		t.Fatalf("/supps = %q", got)
	}
	got := r.handleMessage(chat, "/supps magn")
	if !strings.Contains(got, "FULL STACK") || !strings.Contains(got, "1/1 today") || !strings.Contains(got, "+18 XP") {
		t.Fatalf("/supps magn = %q", got)
	}
	if got := r.handleMessage(chat, "/supps magnesium"); !strings.Contains(got, "already taken") {
		t.Fatalf("second take = %q, want already-taken", got)
	}
	if got := r.handleMessage(chat, "/supps nope"); !strings.Contains(got, "⚠") {
		t.Fatalf("unknown = %q, want ⚠", got)
	}
	if got := r.handleMessage(chat, "/supps"); !strings.Contains(got, "✓ Magnesium") || !strings.Contains(got, "bonus paid") {
		t.Fatalf("/supps after = %q", got)
	}
}

// Active quest mode from the pocket: /go starts (the dashboard shows it
// running), /now reports it, /stop pauses with an optional resume note.
func TestPresenceActiveModeCommands(t *testing.T) {
	r, svc, _ := newTestRunner(t)
	const chat = int64(4242)
	code, _ := svc.CreateTelegramPairCode()
	r.handleMessage(chat, "/pair "+code.Code)
	quests, _ := svc.ListQuests("", "active")
	id := strconv.FormatInt(quests[0].ID, 10)

	if got := r.handleMessage(chat, "/now"); !strings.Contains(got, "Nothing running") {
		t.Fatalf("/now idle = %q", got)
	}
	if got := r.handleMessage(chat, "/go "+id); !strings.Contains(got, "is running") {
		t.Fatalf("/go = %q", got)
	}
	if d, _ := svc.GetDashboard(); d.ActiveSession == nil || d.ActiveSession.QuestID != quests[0].ID {
		t.Fatalf("dashboard session after /go = %+v", d.ActiveSession)
	}
	if got := r.handleMessage(chat, "/status"); !strings.Contains(got, "▶ running: "+quests[0].Title) {
		t.Errorf("/status lacks the running quest: %q", got)
	}
	if got := r.handleMessage(chat, "/stop find the shoes"); !strings.Contains(got, "paused") || !strings.Contains(got, "find the shoes") {
		t.Fatalf("/stop = %q", got)
	}
	if all, _ := svc.ListQuests("", "active"); all[0].ID != quests[0].ID || all[0].ResumeNote != "find the shoes" {
		t.Errorf("resume note on %d = %q", all[0].ID, all[0].ResumeNote)
	}
	if got := r.handleMessage(chat, "/stop"); !strings.Contains(got, "⚠") {
		t.Errorf("/stop with nothing running = %q, want a warning", got)
	}
	if got := r.handleMessage(chat, "/go abc"); !strings.Contains(got, "Usage") {
		t.Errorf("/go abc = %q", got)
	}
}

// The scheduled nudge carries buttons; a tap acts on the linked user and
// turns the nudge into a receipt without buttons.
func TestPresenceNudgeButtons(t *testing.T) {
	r, svc, sent := newTestRunner(t)
	const chat = int64(5151)
	code, _ := svc.CreateTelegramPairCode()
	r.handleMessage(chat, "/pair "+code.Code)

	p, err := r.buildNudge(svc)
	if err != nil || p.text == "" || len(p.buttons) != 2 || !strings.HasPrefix(p.buttons[0][0].Data, "go:") || !strings.HasPrefix(p.buttons[0][1].Data, "done:") {
		t.Fatalf("nudge push = %+v, %v", p, err)
	}
	// Fire it through the scheduler path: the send carries the keyboard.
	now := time.Now()
	build := func(*services.Service) (push, error) { return p, nil }
	r.fires[fireKey{svc.UserID(), "nudge"}] = fire{at: now.Add(-time.Second), hhmm: "20:00"}
	r.tick(now, svc.UserID(), chat, "nudge", "20:00", build)
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "KEYBOARD:") || !strings.Contains((*sent)[0], `"go:`) {
		t.Fatalf("scheduled nudge send = %v, want text + keyboard", *sent)
	}

	// Tap "Start": the message becomes a running receipt, no keyboard.
	goData := p.buttons[0][0].Data
	r.handleCallback(&telegram.CallbackQuery{ID: "cb1", Data: goData, Message: &telegram.UpdateMessage{MessageID: 77, Text: "nudge", Chat: telegram.Chat{ID: chat, Type: "private"}}})
	if last := (*sent)[len(*sent)-1]; !strings.HasPrefix(last, "EDIT:") || !strings.Contains(last, "is running") || strings.Contains(last, "KEYBOARD:") {
		t.Fatalf("after Start tap = %q", last)
	}
	if d, _ := svc.GetDashboard(); d.ActiveSession == nil {
		t.Fatal("Start tap did not open a session")
	}
	// Tap "Done": completes, receipt shows XP and progress; the session closed.
	doneData := p.buttons[0][1].Data
	before, _ := svc.GetDashboard()
	r.handleCallback(&telegram.CallbackQuery{ID: "cb2", Data: doneData, Message: &telegram.UpdateMessage{MessageID: 77, Text: "nudge", Chat: telegram.Chat{ID: chat, Type: "private"}}})
	after, _ := svc.GetDashboard()
	if after.DailyProgress.CompletedToday != before.DailyProgress.CompletedToday+1 || after.ActiveSession != nil {
		t.Fatalf("Done tap: completed %d -> %d, session %+v", before.DailyProgress.CompletedToday, after.DailyProgress.CompletedToday, after.ActiveSession)
	}
	if last := (*sent)[len(*sent)-1]; !strings.Contains(last, "complete ·") {
		t.Errorf("Done receipt = %q", last)
	}
	// A second tap on the same stale keyboard is a no-op with a warning toast.
	r.handleCallback(&telegram.CallbackQuery{ID: "cb3", Data: doneData, Message: &telegram.UpdateMessage{MessageID: 77, Text: "nudge", Chat: telegram.Chat{ID: chat, Type: "private"}}})
	if final, _ := svc.GetDashboard(); final.DailyProgress.CompletedToday != after.DailyProgress.CompletedToday {
		t.Error("stale Done tap double-completed")
	}
	// "Not this one" rotates to another quest WITH a keyboard; "Not tonight" closes it.
	r.handleCallback(&telegram.CallbackQuery{ID: "cb4", Data: "another:" + strings.TrimPrefix(doneData, "done:"), Message: &telegram.UpdateMessage{MessageID: 77, Text: "nudge", Chat: telegram.Chat{ID: chat, Type: "private"}}})
	if last := (*sent)[len(*sent)-1]; !strings.Contains(last, "KEYBOARD:") || strings.Contains(last, strings.TrimPrefix(doneData, "done:")+`"`) {
		t.Errorf("another = %q, want a different quest with buttons", last)
	}
	r.handleCallback(&telegram.CallbackQuery{ID: "cb5", Data: "snooze", Message: &telegram.UpdateMessage{MessageID: 77, Text: "nudge", Chat: telegram.Chat{ID: chat, Type: "private"}}})
	if last := (*sent)[len(*sent)-1]; !strings.Contains(last, "Not tonight") || strings.Contains(last, "KEYBOARD:") {
		t.Errorf("snooze = %q", last)
	}
	// An unlinked chat's tap does nothing.
	r.handleCallback(&telegram.CallbackQuery{ID: "cb6", Data: doneData, Message: &telegram.UpdateMessage{MessageID: 1, Text: "x", Chat: telegram.Chat{ID: 999, Type: "private"}}})
}
