package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"edi/internal/agent"
	"edi/internal/apiclient"
	"edi/internal/db"
	"edi/internal/handlers"
	"edi/internal/services"
	"edi/internal/telegram"
)

// newTestAPI boots the REAL edi API (handlers over a seeded temp SQLite DB)
// on an httptest server — the bot exercises the same code path production
// uses, proving the "no hidden data path" rule end to end.
func newTestAPI(t *testing.T) (*apiclient.Client, *services.Service) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := services.New(store, 1)
	h := handlers.New(svc, agent.NewRegistry(svc))
	srv := httptest.NewServer(handlers.NewRouter(h, "", ""))
	t.Cleanup(srv.Close)
	return apiclient.New(srv.URL), svc
}

// stubTelegram records every sendMessage text and returns ok for everything.
func stubTelegram(t *testing.T) (*telegram.Client, *[]string) {
	t.Helper()
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			_ = r.ParseForm()
			sent = append(sent, r.Form.Get("text"))
		}
		w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	t.Cleanup(srv.Close)
	tg := telegram.New("TEST")
	tg.BaseURL = srv.URL
	return tg, &sent
}

func TestQuestsThenDoneRoundTrip(t *testing.T) {
	api, svc := newTestAPI(t)

	listing := handleCommand(api, "/quests")
	if !strings.Contains(listing, "30 minute workout") {
		t.Fatalf("/quests missing seed quest:\n%s", listing)
	}

	// Extract the workout's real id via the API (same source the user reads).
	quests, err := api.ListQuests("", "active")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var id int64
	for _, q := range quests {
		if q.Title == "30 minute workout" {
			id = q.ID
		}
	}
	if id == 0 {
		t.Fatal("workout quest not found")
	}

	reply := handleCommand(api, fmt.Sprintf("/done %d", id))
	if !strings.Contains(reply, "complete") || !strings.Contains(reply, "+50 XP") {
		t.Errorf("/done reply = %q, want completion with +50 XP", reply)
	}

	// The completion genuinely landed: re-completing is rejected by the API.
	again := handleCommand(api, fmt.Sprintf("/done %d", id))
	if !strings.Contains(again, "⚠") {
		t.Errorf("second /done should surface the API error, got %q", again)
	}

	// And the service shows it completed.
	dash, err := svc.GetDashboard()
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if dash.DailyProgress.CompletedToday != 1 {
		t.Errorf("completed_today = %d, want 1", dash.DailyProgress.CompletedToday)
	}
}

func TestBriefingAndNudgePushes(t *testing.T) {
	api, _ := newTestAPI(t)
	tg, sent := stubTelegram(t)

	if err := sendBriefing(api, tg, 777); err != nil {
		t.Fatalf("briefing: %v", err)
	}
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "edi briefing") {
		t.Fatalf("briefing not sent: %v", *sent)
	}

	// Nothing completed yet in this fresh DB -> nudge fires.
	if err := sendNudge(api, tg, 777); err != nil {
		t.Fatalf("nudge: %v", err)
	}
	if len(*sent) != 2 || !strings.Contains((*sent)[1], "/done") {
		t.Fatalf("nudge not sent: %v", *sent)
	}

	// Complete something; the nudge must go silent.
	quests, _ := api.ListQuests("", "active")
	if _, err := api.CompleteQuest(quests[0].ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := sendNudge(api, tg, 777); err != nil {
		t.Fatalf("nudge 2: %v", err)
	}
	if len(*sent) != 2 {
		t.Errorf("nudge fired despite a completion today: %v", *sent)
	}

	// Unauthorized chats: handled in pollLoop by chat-id filtering (covered
	// by inspection; pollLoop loops forever so it is not driven here).
}
