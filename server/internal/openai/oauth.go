// Package openai implements "Sign in with ChatGPT" (the same OAuth flow the
// OpenAI Codex CLI uses) so edi can call models against the user's own ChatGPT
// subscription. It also talks to the ChatGPT backend `responses` endpoint.
//
// This uses OpenAI's public Codex client credentials and the chatgpt.com backend
// API — the same surface Codex/opencode use for subscription-billed calls. These
// endpoints are not part of the documented platform API and may change.
package openai

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	AuthorizeURL = "https://auth.openai.com/oauth/authorize"
	TokenURL     = "https://auth.openai.com/oauth/token"
	RedirectURI  = "http://localhost:1455/auth/callback"
	Scope        = "openid profile email offline_access"
	// authClaim is the namespaced claim on the id/access token that carries the
	// ChatGPT account id required by the backend responses endpoint.
	authClaim = "https://api.openai.com/auth"
)

// PKCE holds a verifier/challenge pair for one authorization attempt.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a fresh PKCE pair (S256).
func NewPKCE() (PKCE, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return PKCE{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return PKCE{Verifier: verifier, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// RandomState returns an opaque state token.
func RandomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthorizeURLFor builds the browser authorization URL.
func AuthorizeURLFor(challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", ClientID)
	q.Set("redirect_uri", RedirectURI)
	q.Set("scope", Scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", "codex_cli_rs")
	return AuthorizeURL + "?" + q.Encode()
}

// Tokens is the result of a code exchange or refresh, plus derived identity.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	AccountID    string
	Email        string
	ExpiresAt    time.Time
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// ExchangeCode swaps an authorization code for tokens.
func ExchangeCode(code, verifier string) (Tokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {RedirectURI},
	}
	return doTokenRequest(form)
}

// Refresh obtains a new access token from a refresh token.
func Refresh(refreshToken string) (Tokens, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ClientID},
		"refresh_token": {refreshToken},
		"scope":         {Scope},
	}
	t, err := doTokenRequest(form)
	if err != nil {
		return t, err
	}
	// Some refresh responses omit the refresh token — keep the existing one.
	if t.RefreshToken == "" {
		t.RefreshToken = refreshToken
	}
	return t, nil
}

func doTokenRequest(form url.Values) (Tokens, error) {
	req, _ := http.NewRequest(http.MethodPost, TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return Tokens{}, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Tokens{}, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return Tokens{}, fmt.Errorf("token response missing access_token")
	}
	t := Tokens{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		IDToken:      tr.IDToken,
		ExpiresAt:    time.Now().Add(time.Duration(maxInt(tr.ExpiresIn, 3600)) * time.Second),
	}
	// Identity comes from the id_token (preferred) or access token claims.
	enrichIdentity(&t)
	return t, nil
}

// enrichIdentity fills AccountID/Email from JWT claims (best-effort).
func enrichIdentity(t *Tokens) {
	for _, tok := range []string{t.IDToken, t.AccessToken} {
		claims := decodeClaims(tok)
		if claims == nil {
			continue
		}
		if t.Email == "" {
			if e, ok := claims["email"].(string); ok {
				t.Email = e
			}
		}
		if t.AccountID == "" {
			if auth, ok := claims[authClaim].(map[string]any); ok {
				if id, ok := auth["chatgpt_account_id"].(string); ok {
					t.AccountID = id
				}
			}
		}
	}
}

// TokensFromStored builds a Tokens value from already-obtained token strings
// (e.g. imported from the Codex CLI's auth.json). Expiry and email are recovered
// from the access-token claims; a missing/expired `exp` yields a past time so the
// first use triggers a refresh.
func TokensFromStored(access, refresh, id, accountID string) Tokens {
	t := Tokens{AccessToken: access, RefreshToken: refresh, IDToken: id, AccountID: accountID}
	if claims := decodeClaims(access); claims != nil {
		if exp, ok := claims["exp"].(float64); ok {
			t.ExpiresAt = time.Unix(int64(exp), 0)
		}
	}
	if t.ExpiresAt.IsZero() {
		t.ExpiresAt = time.Now().Add(-time.Minute) // force refresh on first use
	}
	enrichIdentity(&t) // fills email; keeps provided accountID if claim absent
	if t.AccountID == "" {
		t.AccountID = accountID
	}
	return t
}

// decodeClaims decodes a JWT payload without verifying the signature (the token
// is issued to us over TLS; we only read our own identity claims).
func decodeClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return nil
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
