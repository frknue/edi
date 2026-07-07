package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"edi/internal/db"
	"edi/internal/models"
	"edi/internal/openai"
)

// ErrOpenAINotConnected means no ChatGPT subscription is linked, so AI features
// are unavailable (mapped to 400 with a clear message).
var ErrOpenAINotConnected = fmt.Errorf("%w: OpenAI is not connected — connect your ChatGPT account to use AI features", ErrValidation)

type oauthPending struct {
	verifier  string
	state     string
	expiresAt time.Time
}

// Settings keys.
const (
	settingModel  = "openai_model"
	settingEffort = "openai_effort"
)

// EffortLevels are the reasoning depths the ChatGPT backend accepts (verified
// live), fastest→deepest. "medium" is the default.
var EffortLevels = []string{"none", "low", "medium", "high", "xhigh"}

func validEffort(e string) bool {
	for _, v := range EffortLevels {
		if v == e {
			return true
		}
	}
	return false
}

// openAIModel returns the configured model (setting → EDI_OPENAI_MODEL → default).
func (s *Service) openAIModel() string {
	if m, _ := s.store.GetSetting(s.userID, settingModel); m != "" {
		return m
	}
	if m := os.Getenv("EDI_OPENAI_MODEL"); m != "" {
		return m
	}
	return openai.DefaultModel
}

// openAIEffort returns the configured reasoning effort (setting → env → medium).
func (s *Service) openAIEffort() string {
	if e, _ := s.store.GetSetting(s.userID, settingEffort); validEffort(e) {
		return e
	}
	if e := os.Getenv("EDI_OPENAI_EFFORT"); validEffort(e) {
		return e
	}
	return "medium"
}

// SetOpenAIConfig updates the model and/or reasoning effort. Empty fields are
// left unchanged; an invalid effort is rejected.
func (s *Service) SetOpenAIConfig(model, effort string) (models.OpenAIStatus, error) {
	if effort != "" {
		if !validEffort(effort) {
			return models.OpenAIStatus{}, validationErr("invalid effort %q (allowed: %v)", effort, EffortLevels)
		}
		if err := s.store.SetSetting(s.userID, settingEffort, effort); err != nil {
			return models.OpenAIStatus{}, err
		}
	}
	if model != "" {
		if err := s.store.SetSetting(s.userID, settingModel, model); err != nil {
			return models.OpenAIStatus{}, err
		}
	}
	return s.OpenAIStatus()
}

// OpenAIStatus reports whether a ChatGPT subscription is connected and the
// current model/effort configuration.
func (s *Service) OpenAIStatus() (models.OpenAIStatus, error) {
	creds, err := s.store.GetOpenAICredentials(s.userID)
	if err != nil {
		return models.OpenAIStatus{}, err
	}
	if creds == nil {
		return models.OpenAIStatus{Connected: false}, nil
	}
	exp := creds.ExpiresAt
	return models.OpenAIStatus{
		Connected:     true,
		Email:         creds.Email,
		AccountID:     creds.AccountID,
		Model:         s.openAIModel(),
		Effort:        s.openAIEffort(),
		EffortOptions: EffortLevels,
		ExpiresAt:     &exp,
	}, nil
}

// StartOpenAIConnect begins the "Sign in with ChatGPT" flow: it generates PKCE
// state, spins up the one-shot localhost:1455 callback listener (the redirect URI
// registered for the Codex client), and returns the browser authorization URL.
func (s *Service) StartOpenAIConnect() (string, error) {
	pkce, err := openai.NewPKCE()
	if err != nil {
		return "", err
	}
	state, err := openai.RandomState()
	if err != nil {
		return "", err
	}

	s.oauthMu.Lock()
	s.oauthPending = &oauthPending{verifier: pkce.Verifier, state: state, expiresAt: time.Now().Add(10 * time.Minute)}
	s.oauthMu.Unlock()

	if err := s.startCallbackServer(); err != nil {
		return "", fmt.Errorf("start callback listener on :1455: %w", err)
	}
	return openai.AuthorizeURLFor(pkce.Challenge, state), nil
}

// startCallbackServer starts (once) an HTTP server on :1455 to catch the OAuth
// redirect. It shuts itself down after a successful exchange or a timeout.
func (s *Service) startCallbackServer() error {
	s.oauthMu.Lock()
	if s.oauthServer != nil {
		s.oauthMu.Unlock()
		return nil // already listening
	}
	mux := http.NewServeMux()
	srv := &http.Server{Addr: "localhost:1455", Handler: mux}
	s.oauthServer = srv
	s.oauthMu.Unlock()

	mux.HandleFunc("/auth/callback", s.handleOAuthCallback)

	go func() {
		_ = srv.ListenAndServe() // returns on Shutdown
		s.oauthMu.Lock()
		s.oauthServer = nil
		s.oauthMu.Unlock()
	}()
	// Safety net: tear down if the user never completes the flow.
	go func() {
		time.Sleep(10 * time.Minute)
		s.shutdownCallbackServer()
	}()
	return nil
}

func (s *Service) shutdownCallbackServer() {
	s.oauthMu.Lock()
	srv := s.oauthServer
	s.oauthMu.Unlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

func (s *Service) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	s.oauthMu.Lock()
	pending := s.oauthPending
	s.oauthMu.Unlock()

	fail := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, callbackHTML("Connection failed", msg, false))
	}

	if pending == nil || time.Now().After(pending.expiresAt) {
		fail("The sign-in request expired. Start again from edi.")
		return
	}
	if code == "" || state != pending.state {
		fail("Invalid callback (state mismatch). Start again from edi.")
		return
	}

	tokens, err := openai.ExchangeCode(code, pending.verifier)
	if err != nil {
		fail("Token exchange failed: " + err.Error())
		return
	}
	if err := s.saveTokens(tokens); err != nil {
		fail("Could not save credentials: " + err.Error())
		return
	}

	s.oauthMu.Lock()
	s.oauthPending = nil
	s.oauthMu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, callbackHTML("Connected to ChatGPT", "You can close this tab and return to edi.", true))

	go s.shutdownCallbackServer()
}

func (s *Service) saveTokens(t openai.Tokens) error {
	return s.store.SaveOpenAICredentials(s.userID, db.OpenAICredentials{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		IDToken:      t.IDToken,
		AccountID:    t.AccountID,
		Email:        t.Email,
		ExpiresAt:    t.ExpiresAt,
	})
}

// DisconnectOpenAI removes the stored credentials.
func (s *Service) DisconnectOpenAI() error {
	s.shutdownCallbackServer()
	return s.store.DeleteOpenAICredentials(s.userID)
}

// ImportCodexCredentials reads ~/.codex/auth.json (written by `codex login`) and
// stores those tokens — an instant setup path for users already signed into the
// Codex CLI, no browser round-trip.
func (s *Service) ImportCodexCredentials() (models.OpenAIStatus, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return models.OpenAIStatus{}, err
	}
	path := filepath.Join(home, ".codex", "auth.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return models.OpenAIStatus{}, validationErr("could not read %s (is the Codex CLI signed in?): %v", path, err)
	}
	var f struct {
		Tokens struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return models.OpenAIStatus{}, validationErr("could not parse Codex auth.json: %v", err)
	}
	if f.Tokens.AccessToken == "" || f.Tokens.RefreshToken == "" {
		return models.OpenAIStatus{}, validationErr("Codex auth.json has no OAuth tokens (not signed in with ChatGPT?)")
	}
	t := openai.TokensFromStored(f.Tokens.AccessToken, f.Tokens.RefreshToken, f.Tokens.IDToken, f.Tokens.AccountID)
	if err := s.saveTokens(t); err != nil {
		return models.OpenAIStatus{}, err
	}
	return s.OpenAIStatus()
}

// accessToken returns a currently-valid access token + account id, refreshing and
// persisting if the stored token is near expiry. Returns ErrOpenAINotConnected
// when nothing is linked.
func (s *Service) accessToken() (token, accountID string, err error) {
	creds, err := s.store.GetOpenAICredentials(s.userID)
	if err != nil {
		return "", "", err
	}
	if creds == nil {
		return "", "", ErrOpenAINotConnected
	}
	if time.Now().Before(creds.ExpiresAt.Add(-60 * time.Second)) {
		return creds.AccessToken, creds.AccountID, nil
	}
	// Refresh.
	refreshed, err := openai.Refresh(creds.RefreshToken)
	if err != nil {
		return "", "", fmt.Errorf("%w: OpenAI session expired, please reconnect (%v)", ErrValidation, err)
	}
	if refreshed.AccountID == "" {
		refreshed.AccountID = creds.AccountID
	}
	if err := s.saveTokens(refreshed); err != nil {
		return "", "", err
	}
	return refreshed.AccessToken, refreshed.AccountID, nil
}

// completeWithOpenAI runs one prompt through the subscription model, refreshing
// the token once on a 401.
func (s *Service) completeWithOpenAI(instructions, prompt string) (string, error) {
	token, accountID, err := s.accessToken()
	if err != nil {
		return "", err
	}
	model, effort := s.openAIModel(), s.openAIEffort()
	out, err := openai.Complete(token, accountID, model, effort, instructions, prompt)
	var unauth openai.ErrUnauthorized
	if errors.As(err, &unauth) {
		// Force a refresh and retry once.
		creds, e := s.store.GetOpenAICredentials(s.userID)
		if e != nil || creds == nil {
			return "", ErrOpenAINotConnected
		}
		refreshed, e := openai.Refresh(creds.RefreshToken)
		if e != nil {
			return "", fmt.Errorf("%w: OpenAI session expired, please reconnect", ErrValidation)
		}
		if refreshed.AccountID == "" {
			refreshed.AccountID = creds.AccountID
		}
		if e := s.saveTokens(refreshed); e != nil {
			return "", e
		}
		out, err = openai.Complete(refreshed.AccessToken, refreshed.AccountID, model, effort, instructions, prompt)
	}
	return out, err
}

func callbackHTML(title, message string, ok bool) string {
	color := "#ff5c5c"
	if ok {
		color = "#3ee594"
	}
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><title>%s</title>
<style>body{background:#07080c;color:#e9ebf5;font-family:system-ui,sans-serif;display:grid;place-items:center;height:100vh;margin:0}
.card{text-align:center;padding:2rem 2.5rem;border:1px solid #232a3d;border-radius:16px;background:#10131e}
h1{color:%s;font-size:1.2rem;margin:0 0 .5rem}p{color:#8b91a8;margin:0}</style></head>
<body><div class="card"><h1>%s</h1><p>%s</p></div></body></html>`, title, color, title, message)
}
