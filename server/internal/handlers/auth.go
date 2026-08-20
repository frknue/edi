package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"edi/internal/models"
	"edi/internal/services"
)

// ctxKeyUser carries the authenticated user id through the request context.
type ctxKey int

const ctxKeyUser ctxKey = iota

// forUser returns the service bound to the request's authenticated user.
// The auth middleware guarantees the value is present on every /api route.
func (h *Handlers) forUser(r *http.Request) *services.Service {
	if id, ok := r.Context().Value(ctxKeyUser).(int64); ok {
		return h.svc.ForUser(id)
	}
	// Unreachable behind the middleware; fail safe to the dev-fallback user
	// rather than panicking.
	return h.svc
}

// devUserID is who anonymous requests act as when the server runs WITHOUT
// EDI_TOKEN (tokenless localhost dev mode — same behavior as the single-user
// era). With EDI_TOKEN set, anonymous /api requests are 401s instead.
const devUserID int64 = 1

// authOpen lists the /api paths that skip authentication: liveness, the auth
// discovery endpoint, and registration (which authenticates via invite code).
func authOpen(r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/health",
		r.URL.Path == "/api/auth/config",
		r.URL.Path == "/api/auth/register" && r.Method == http.MethodPost:
		return true
	}
	return false
}

// authMW resolves `Authorization: Bearer <token>` (or X-API-Key) to a user and
// stores the id in the context. tokenMode is true when EDI_TOKEN is set: then
// every /api route except the open list requires a valid token. Static SPA
// assets are never gated (the app shell is public, the data behind it is not).
func (h *Handlers) authMW(tokenMode bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || authOpen(r) {
			next.ServeHTTP(w, r)
			return
		}
		if got := bearerToken(r); got != "" {
			id, err := h.svc.AuthenticateToken(got)
			if err == nil {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyUser, id)))
				return
			}
			if !errors.Is(err, services.ErrUnauthorized) {
				writeError(w, err)
				return
			}
			// fall through to 401 (or dev fallback when tokenless)
		}
		if tokenMode {
			w.Header().Set("WWW-Authenticate", `Bearer realm="edi"`)
			writeJSON(w, http.StatusUnauthorized, errorBody{Error: "missing or invalid API token"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyUser, devUserID)))
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

// --- auth & user endpoints ----------------------------------------------------

// authConfig is the pre-auth discovery endpoint: the client learns whether the
// server requires a token and whether self-serve registration is open.
func (h *Handlers) authConfig(tokenMode bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{
			"auth_required":     tokenMode,
			"registration_open": services.RegistrationOpen(),
		})
	}
}

func (h *Handlers) register(w http.ResponseWriter, r *http.Request) {
	var in models.RegisterInput
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	created, err := h.svc.RegisterUser(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	u, err := h.forUser(r).Me()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// requireAdmin wraps admin-only handlers.
func (h *Handlers) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := h.forUser(r).Me()
		if err != nil {
			writeError(w, err)
			return
		}
		if !u.IsAdmin {
			writeJSON(w, http.StatusForbidden, errorBody{Error: "admin only"})
			return
		}
		next(w, r)
	}
}

// --- telegram pairing ----------------------------------------------------------

func (h *Handlers) telegramStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.forUser(r).TelegramStatus()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handlers) telegramPairCode(w http.ResponseWriter, r *http.Request) {
	code, err := h.forUser(r).CreateTelegramPairCode()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, code)
}

func (h *Handlers) telegramPushTimes(w http.ResponseWriter, r *http.Request) {
	out, err := h.forUser(r).TelegramPushTimes()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) setTelegramPushTimes(w http.ResponseWriter, r *http.Request) {
	var p models.TelegramPushTimesPatch
	if err := decodeBody(r, &p); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.forUser(r).SetTelegramPushTimes(p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) telegramUnlink(w http.ResponseWriter, r *http.Request) {
	if err := h.forUser(r).UnlinkTelegram(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"linked": false})
}

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.forUser(r).ListUsers()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	created, err := h.forUser(r).CreateUser(in.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handlers) rotateUserToken(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	created, err := h.forUser(r).RotateUserToken(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}
