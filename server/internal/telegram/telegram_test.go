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
