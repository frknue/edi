package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetUpdatesParsesMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botTEST/getUpdates" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("offset"); got != "42" {
			t.Errorf("offset = %s, want 42", got)
		}
		w.Write([]byte(`{"ok":true,"result":[
			{"update_id":100,"message":{"text":"/status","chat":{"id":777}}},
			{"update_id":101,"message":{"text":"/done 3","chat":{"id":777}}}
		]}`))
	}))
	defer srv.Close()

	c := New("TEST")
	c.BaseURL = srv.URL
	updates, err := c.GetUpdates(42, 30)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if len(updates) != 2 || updates[0].UpdateID != 100 || updates[1].Message.Text != "/done 3" || updates[0].Message.Chat.ID != 777 {
		t.Errorf("updates = %+v", updates)
	}
}

func TestSendMessageFormAndErrors(t *testing.T) {
	var got map[string]string
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = map[string]string{
			"chat_id":    r.Form.Get("chat_id"),
			"text":       r.Form.Get("text"),
			"parse_mode": r.Form.Get("parse_mode"),
		}
		if fail {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "Bad Request: chat not found"})
			return
		}
		w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	c := New("TEST")
	c.BaseURL = srv.URL
	if err := c.SendMessage(777, "<b>hi</b>"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got["chat_id"] != "777" || got["text"] != "<b>hi</b>" || got["parse_mode"] != "HTML" {
		t.Errorf("form = %v", got)
	}
	fail = true
	if err := c.SendMessage(777, "x"); err == nil {
		t.Error("expected error when ok:false, got nil")
	}
}

func TestButtonsAndCallbacks(t *testing.T) {
	var forms []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f := map[string]string{"path": r.URL.Path}
		for k := range r.Form {
			f[k] = r.Form.Get(k)
		}
		forms = append(forms, f)
		if r.URL.Path == "/botTEST/getUpdates" {
			w.Write([]byte(`{"ok":true,"result":[{"update_id":7,"callback_query":{"id":"cb1","data":"done:12","message":{"message_id":99,"chat":{"id":5,"type":"private"}}}}]}`))
			return
		}
		w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()
	c := New("TEST")
	c.BaseURL = srv.URL

	rows := [][]Button{{{Text: "Done", Data: "done:12"}, {Text: "Start", Data: "go:12"}}, {{Text: "Not now", Data: "snooze"}}}
	if err := c.SendMessageWithButtons(5, "hi", rows); err != nil {
		t.Fatal(err)
	}
	var kb map[string][][]Button
	if err := json.Unmarshal([]byte(forms[0]["reply_markup"]), &kb); err != nil || len(kb["inline_keyboard"]) != 2 || kb["inline_keyboard"][0][1].Data != "go:12" {
		t.Errorf("reply_markup = %s (%v)", forms[0]["reply_markup"], err)
	}
	if err := c.EditMessageText(5, 99, "done", nil); err != nil {
		t.Fatal(err)
	}
	if f := forms[1]; f["path"] != "/botTEST/editMessageText" || f["message_id"] != "99" || f["reply_markup"] != "" {
		t.Errorf("edit form = %v", f)
	}
	if err := c.AnswerCallbackQuery("cb1", "ok"); err != nil {
		t.Fatal(err)
	}
	if f := forms[2]; f["path"] != "/botTEST/answerCallbackQuery" || f["callback_query_id"] != "cb1" {
		t.Errorf("answer form = %v", f)
	}
	updates, err := c.GetUpdates(0, 1)
	if err != nil || len(updates) != 1 || updates[0].CallbackQuery == nil || updates[0].CallbackQuery.Data != "done:12" || updates[0].CallbackQuery.Message.MessageID != 99 {
		t.Errorf("callback update = %+v, %v", updates, err)
	}
	if forms[3]["allowed_updates"] != `["message","callback_query"]` {
		t.Errorf("allowed_updates = %s", forms[3]["allowed_updates"])
	}
}
