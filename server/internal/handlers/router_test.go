package handlers

import (
	"encoding/json"
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
