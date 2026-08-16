package presence

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"edi/internal/db/dbtest"
	"edi/internal/models"
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
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			_ = r.ParseForm()
			sent = append(sent, r.Form.Get("text"))
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
	return New(svc, tg, "08:00", "20:00"), svc, sent
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

	// After completing something today, the nudge stands down.
	quests, _ := svc.ListQuests("", "active")
	if _, err := svc.CompleteQuest(quests[0].ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got := r.handleMessage(chat, "/nudge"); !strings.Contains(got, "Nothing to nudge about") {
		t.Fatalf("/nudge after progress = %q, want stand-down message", got)
	}

	// Setting times still works with an argument.
	if got := r.handleMessage(chat, "/briefing 06:45"); !strings.Contains(got, "06:45") {
		t.Fatalf("/briefing HH:MM = %q", got)
	}
}
