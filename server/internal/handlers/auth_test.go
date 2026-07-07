package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"edi/internal/agent"
	"edi/internal/db"
	"edi/internal/services"
)

func newTestRouter(t *testing.T, token string) http.Handler {
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
	return NewRouter(New(svc, agent.NewRegistry(svc)), "", token)
}

func TestAuthDisabledByDefault(t *testing.T) {
	router := newTestRouter(t, "")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("tokenless server: GET /api/dashboard = %d, want 200", rec.Code)
	}
}

func TestAuthEnforcedWhenTokenSet(t *testing.T) {
	router := newTestRouter(t, "s3cret")

	cases := []struct {
		name   string
		header map[string]string
		want   int
	}{
		{"no credentials", nil, http.StatusUnauthorized},
		{"wrong bearer", map[string]string{"Authorization": "Bearer nope"}, http.StatusUnauthorized},
		{"correct bearer", map[string]string{"Authorization": "Bearer s3cret"}, http.StatusOK},
		{"x-api-key fallback", map[string]string{"X-API-Key": "s3cret"}, http.StatusOK},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
		for k, v := range c.header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
	}

	// Health stays open for probes.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/health without token = %d, want 200 (exempt)", rec.Code)
	}

	// Mutations are gated too.
	req := httptest.NewRequest(http.MethodPost, "/api/quests/1/complete", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST complete without token = %d, want 401", rec.Code)
	}
}
