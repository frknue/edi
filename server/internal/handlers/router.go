package handlers

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NewRouter builds the full HTTP handler: API routes + optional static client.
// If clientDir is non-empty and exists, the built SPA is served from it (true
// single-binary self-hosting); otherwise only the API is served (dev mode uses
// the Vite dev server + proxy).
//
// apiToken, when non-empty, gates every /api route (except /api/health) behind
// `Authorization: Bearer <token>` — the shared-secret auth that lets external
// agents (Codex, OpenClaw-style bots, remote CLIs) connect safely. Empty token
// preserves the tokenless localhost default.
func NewRouter(h *Handlers, clientDir, apiToken string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", h.health)

	mux.HandleFunc("GET /api/dashboard", h.getDashboard)
	mux.HandleFunc("GET /api/attributes", h.getAttributes)

	mux.HandleFunc("GET /api/quests", h.listQuests)
	mux.HandleFunc("POST /api/quests", h.createQuest)
	mux.HandleFunc("PATCH /api/quests/{id}", h.updateQuest)
	mux.HandleFunc("POST /api/quests/{id}/complete", h.completeQuest)
	mux.HandleFunc("POST /api/quests/{id}/skip", h.skipQuest)
	mux.HandleFunc("POST /api/quests/{id}/archive", h.archiveQuest)

	mux.HandleFunc("GET /api/xp-events", h.getXPEvents)

	// Tools — guided instruments that award XP (e.g. the Daily Mood Log).
	mux.HandleFunc("GET /api/tools", h.listGuidedTools)
	mux.HandleFunc("POST /api/tools/{key}/complete", h.completeTool)
	mux.HandleFunc("GET /api/tools/{key}/entries", h.listToolEntries)

	mux.HandleFunc("GET /api/journal", h.listJournal)
	mux.HandleFunc("POST /api/journal", h.createJournal)

	mux.HandleFunc("GET /api/agent/suggestions", h.listSuggestions)
	mux.HandleFunc("POST /api/agent/suggestions/generate", h.generateSuggestions)
	mux.HandleFunc("POST /api/agent/suggestions/{id}/accept", h.acceptSuggestion)
	mux.HandleFunc("POST /api/agent/suggestions/{id}/dismiss", h.dismissSuggestion)

	// OpenAI (ChatGPT subscription) connection powering AI features.
	mux.HandleFunc("GET /api/openai/status", h.openaiStatus)
	mux.HandleFunc("POST /api/openai/connect", h.openaiConnect)
	mux.HandleFunc("POST /api/openai/import-codex", h.openaiImportCodex)
	mux.HandleFunc("POST /api/openai/disconnect", h.openaiDisconnect)
	mux.HandleFunc("GET /api/openai/models", h.openaiModels)
	mux.HandleFunc("POST /api/openai/config", h.openaiConfig)

	// Agent-ready tool interface (discovery + invocation).
	mux.HandleFunc("GET /api/agent/tools", h.listTools)
	mux.HandleFunc("POST /api/agent/tools/{name}/invoke", h.invokeTool)

	// Optional: serve the built SPA.
	if clientDir != "" {
		if _, err := os.Stat(clientDir); err == nil {
			mux.Handle("/", spaFileServer(clientDir))
		}
	}

	return withMiddleware(authMW(apiToken, mux))
}

// authMW enforces bearer-token auth on /api routes when a token is configured.
// /api/health stays open for liveness probes; static SPA assets are not gated
// (the app shell is public, the data behind it is not).
func authMW(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	tok := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		got := bearerToken(r)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), tok) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="edi"`)
			writeJSON(w, http.StatusUnauthorized, errorBody{Error: "missing or invalid API token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the credential from "Authorization: Bearer <t>" or,
// as a fallback for simple clients, the X-API-Key header.
func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return r.Header.Get("X-API-Key")
}

// spaFileServer serves static assets and falls back to index.html for client-side routes.
func spaFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean(r.URL.Path)
		if clean == "/" {
			http.ServeFile(w, r, index)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, clean)); err == nil {
			fs.ServeHTTP(w, r)
			return
		}
		// Unknown non-API path -> SPA entry point.
		http.ServeFile(w, r, index)
	})
}

// --- middleware -------------------------------------------------------------

// maxBodyBytes caps request bodies (generous for this data model) to prevent a
// runaway client/agent from exhausting memory during JSON decode.
const maxBodyBytes = 1 << 20 // 1 MiB

func withMiddleware(next http.Handler) http.Handler {
	return recoverMW(corsMW(bodyLimitMW(loggingMW(next))))
}

// bodyLimitMW caps the size of every request body.
func bodyLimitMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// corsMW reflects the Origin only for loopback origins. The browser still blocks
// cross-origin reads from arbitrary sites (mitigating the localhost-CSRF / DNS-
// rebinding class), while the dev (localhost:5173) and prod (served origin) clients,
// plus same-origin CLI/agent callers, keep working. There is no auth in single-user
// MVP mode, so this is the relevant boundary.
func corsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isLoopbackOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackOrigin reports whether an Origin header refers to localhost / 127.0.0.1
// (any port/scheme). Non-browser clients (curl, CLI, agent) send no Origin and are
// unaffected.
func isLoopbackOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	host := origin
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return host == "localhost" || host == "127.0.0.1" || host == "[::1]" || host == "::1"
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func loggingMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

func recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
