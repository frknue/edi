package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edi/internal/agent"
	"edi/internal/db/dbtest"
	"edi/internal/services"
)

func newTestRouter(t *testing.T, token string) http.Handler {
	t.Helper()
	store := dbtest.Open(t)
	if err := store.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := services.New(store, 1)
	if token != "" {
		// Same bootstrap as main.go: EDI_TOKEN becomes user 1's login token.
		if err := svc.AdoptEnvToken(token); err != nil {
			t.Fatalf("adopt token: %v", err)
		}
	}
	return NewRouter(New(svc, agent.NewRegistry()), "", token != "")
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

// The multi-tenant HTTP contract: registration + per-user tokens + admin gating.
func TestMultiTenantHTTPFlow(t *testing.T) {
	t.Setenv("EDI_INVITE_CODE", "sesame")
	router := newTestRouter(t, "s3cret") // s3cret adopted as user 1 (admin)

	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rdr)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// auth config is open and reports the server's mode.
	rec := do(http.MethodGet, "/api/auth/config", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/config = %d, want 200 (open)", rec.Code)
	}
	var cfg map[string]bool
	_ = json.Unmarshal(rec.Body.Bytes(), &cfg)
	if !cfg["auth_required"] || !cfg["registration_open"] {
		t.Errorf("auth config = %v, want auth_required + registration_open", cfg)
	}

	// register is open (invite code IS the credential).
	rec = do(http.MethodPost, "/api/auth/register", "", `{"name":"Ada","invite_code":"sesame"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d (%s), want 201", rec.Code, rec.Body.String())
	}
	var created struct {
		User  struct{ ID int64 }
		Token string
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Token == "" {
		t.Fatal("register returned no token")
	}

	// Each token resolves to its own user.
	rec = do(http.MethodGet, "/api/me", "s3cret", "")
	var me1 struct {
		ID      int64 `json:"id"`
		IsAdmin bool  `json:"is_admin"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &me1)
	if rec.Code != http.StatusOK || me1.ID != 1 || !me1.IsAdmin {
		t.Errorf("me(user1) = %d %+v, want 200 id=1 admin", rec.Code, me1)
	}
	rec = do(http.MethodGet, "/api/me", created.Token, "")
	var me2 struct {
		ID      int64 `json:"id"`
		IsAdmin bool  `json:"is_admin"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &me2)
	if rec.Code != http.StatusOK || me2.ID != created.User.ID || me2.IsAdmin {
		t.Errorf("me(new user) = %d %+v, want 200 id=%d non-admin", rec.Code, me2, created.User.ID)
	}

	// HTTP-level isolation: the new (blank) user sees no quests; user 1 sees the seed.
	rec = do(http.MethodGet, "/api/quests", created.Token, "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("new user's quests = %d %s, want 200 []", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodGet, "/api/quests", "s3cret", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) == "[]" {
		t.Errorf("user 1's quests = %d (empty=%v), want the seeded list", rec.Code, rec.Body.String() == "[]")
	}

	// Admin endpoints: user 1 yes, the new user 403.
	if rec = do(http.MethodGet, "/api/admin/users", "s3cret", ""); rec.Code != http.StatusOK {
		t.Errorf("admin list as user 1 = %d, want 200", rec.Code)
	}
	if rec = do(http.MethodGet, "/api/admin/users", created.Token, ""); rec.Code != http.StatusForbidden {
		t.Errorf("admin list as non-admin = %d, want 403", rec.Code)
	}

	// Wrong invite code -> 400, not a user.
	if rec = do(http.MethodPost, "/api/auth/register", "", `{"name":"Eve","invite_code":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("register with wrong code = %d, want 400", rec.Code)
	}
}
