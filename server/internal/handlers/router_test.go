package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"edi/internal/agent"
	"edi/internal/db/dbtest"
	"edi/internal/services"
)

// newTestRouterWithClient serves a stub SPA so the static fallback is live.
func newTestRouterWithClient(t *testing.T) http.Handler {
	t.Helper()
	store := dbtest.Open(t)
	if err := store.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	clientDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(clientDir, "index.html"), []byte("<!doctype html><title>edi</title>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	svc := services.New(store, 1)
	return NewRouter(New(svc, agent.NewRegistry()), clientDir, false)
}

// An unmatched /api route must be a JSON 404, not the SPA shell — otherwise a
// client parsing the response sees HTML ("Unexpected token '<'") and a stale
// server looks like a broken feature instead of a missing endpoint.
func TestUnknownAPIRouteReturnsJSON404(t *testing.T) {
	router := newTestRouterWithClient(t)

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil),
		httptest.NewRequest(http.MethodPost, "/api/quests/definitely-not-a-route", nil),
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", req.Method, req.URL.Path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s %s content-type = %q, want JSON", req.Method, req.URL.Path, ct)
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s %s body is not JSON (%v): %s", req.Method, req.URL.Path, err, rec.Body.String())
		}
		if body.Error == "" {
			t.Errorf("%s %s: want a non-empty error message", req.Method, req.URL.Path)
		}
	}
}

// Client-side routes still fall back to the app shell.
func TestUnknownAppRouteServesSPA(t *testing.T) {
	router := newTestRouterWithClient(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/quests", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /quests = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Errorf("GET /quests should serve index.html, got: %s", rec.Body.String())
	}
}

// POST /api/agent/chat is gated like every AI feature: without a ChatGPT
// connection it is a clean 400, and an empty message is 400 too.
func TestAgentChatEndpointGates(t *testing.T) {
	srv := httptest.NewServer(newTestRouter(t, ""))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/agent/chat", "application/json", strings.NewReader(`{"message":"add a run"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "not connected") {
		t.Fatalf("chat without OpenAI = %d %s, want 400 not-connected", resp.StatusCode, body)
	}
	resp2, err := http.Post(srv.URL+"/api/agent/chat", "application/json", strings.NewReader(`{"message":""}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty message = %d, want 400", resp2.StatusCode)
	}
}

// Active quest mode routes: start opens a session, GET /api/session shows
// it, stop closes it with a note; client mistakes are 400s, not 500s.
func TestQuestSessionRoutes(t *testing.T) {
	srv := httptest.NewServer(newTestRouter(t, ""))
	defer srv.Close()
	get := func(path string) (int, map[string]any) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}
	post := func(path, payload string) (int, map[string]any) {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}
	if code, _ := post("/api/session/stop", `{"note":"x"}`); code != http.StatusBadRequest {
		t.Fatalf("stop with nothing running = %d, want 400", code)
	}
	resp, err := http.Get(srv.URL + "/api/quests?status=active")
	if err != nil {
		t.Fatal(err)
	}
	var quests []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&quests)
	resp.Body.Close()
	id := int64(quests[0]["id"].(float64))
	code, sess := post(fmt.Sprintf("/api/quests/%d/start", id), "")
	if code != http.StatusOK || sess["running"] != true {
		t.Fatalf("start = %d %v", code, sess)
	}
	if code, body := get("/api/session"); code != http.StatusOK || body["session"] == nil {
		t.Fatalf("GET session = %d %v", code, body)
	}
	code, stopped := post("/api/session/stop", `{"note":"open the file"}`)
	if code != http.StatusOK || stopped["note"] != "open the file" || stopped["reason"] != "stopped" {
		t.Fatalf("stop = %d %v", code, stopped)
	}
	if code, body := get("/api/session"); code != http.StatusOK || body["session"] != nil {
		t.Fatalf("GET session after stop = %d %v", code, body)
	}
	if code, _ := post("/api/quests/999999/start", ""); code != http.StatusNotFound {
		t.Fatalf("start unknown = %d, want 404", code)
	}
}

// TestCosmeticRoutes: the wardrobe over HTTP — unknown key 404, unowned
// equip / bad slot / double buy 400, happy path 200 with the loadout.
func TestCosmeticRoutes(t *testing.T) {
	srv := httptest.NewServer(newTestRouter(t, ""))
	defer srv.Close()
	post := func(path, payload string) (int, []byte) {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf strings.Builder
		_, _ = io.Copy(&buf, resp.Body)
		return resp.StatusCode, []byte(buf.String())
	}
	if code, _ := post("/api/cosmetics/nope/buy", ""); code != http.StatusNotFound {
		t.Errorf("buy unknown = %d, want 404", code)
	}
	if code, _ := post("/api/cosmetics/iron_helm/equip", ""); code != http.StatusBadRequest {
		t.Errorf("equip unowned = %d, want 400", code)
	}
	if code, _ := post("/api/cosmetics/unequip", `{"slot":"hat"}`); code != http.StatusBadRequest {
		t.Errorf("unequip bad slot = %d, want 400", code)
	}
	code, body := post("/api/cosmetics/slime/buy", "")
	if code != http.StatusOK {
		t.Fatalf("buy = %d %s", code, body)
	}
	var res map[string]any
	_ = json.Unmarshal(body, &res)
	if lo, _ := res["loadout"].([]any); len(lo) != 1 {
		t.Errorf("loadout after buy = %v", res["loadout"])
	}
	if code, _ := post("/api/cosmetics/slime/buy", ""); code != http.StatusBadRequest {
		t.Errorf("double buy = %d, want 400", code)
	}
	resp, err := http.Get(srv.URL + "/api/cosmetics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var cat map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&cat)
	if resp.StatusCode != http.StatusOK || cat["items"] == nil || cat["loadout"] == nil {
		t.Errorf("GET cosmetics = %d %v", resp.StatusCode, cat)
	}
}
