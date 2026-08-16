package services

import (
	"errors"
	"testing"
	"time"
)

func TestParseOAuthCallback(t *testing.T) {
	cases := []struct {
		in       string
		code, st string
		wantErr  bool
	}{
		{"http://localhost:1455/auth/callback?code=abc&state=xyz", "abc", "xyz", false},
		{"  http://localhost:1455/auth/callback?state=xyz&code=abc&extra=1 ", "abc", "xyz", false},
		{"code=abc&state=xyz", "abc", "xyz", false},
		{"?code=abc&state=xyz", "abc", "xyz", false},
		{"", "", "", true},
		{"http://localhost:1455/auth/callback", "", "", true},
		{"code=abc", "", "", true},
		{"just some text", "", "", true},
	}
	for _, c := range cases {
		code, st, err := parseOAuthCallback(c.in)
		if c.wantErr {
			if !errors.Is(err, ErrValidation) {
				t.Errorf("parse(%q) err = %v, want ErrValidation", c.in, err)
			}
			continue
		}
		if err != nil || code != c.code || st != c.st {
			t.Errorf("parse(%q) = (%q, %q, %v), want (%q, %q, nil)", c.in, code, st, err, c.code, c.st)
		}
	}
}

// The failure paths of the manual completion (the happy path needs the real
// OpenAI token endpoint — covered by the live flow, not unit tests).
func TestCompleteOpenAIConnectValidation(t *testing.T) {
	svc := newTestService(t)

	// No pending flow.
	if _, err := svc.CompleteOpenAIConnect("code=a&state=b"); !errors.Is(err, ErrValidation) {
		t.Fatalf("complete without pending = %v, want ErrValidation", err)
	}

	// Wrong state burns the flow.
	svc.oauth.setPending(&oauthPending{userID: 1, verifier: "v", state: "right", expiresAt: time.Now().Add(time.Minute)})
	if _, err := svc.CompleteOpenAIConnect("code=a&state=wrong"); !errors.Is(err, ErrValidation) {
		t.Fatalf("wrong state = %v, want ErrValidation", err)
	}
	if p := svc.oauth.takePending(1); p != nil {
		t.Error("flow must burn on a failed attempt")
	}

	// Expired flow.
	svc.oauth.setPending(&oauthPending{userID: 1, verifier: "v", state: "s", expiresAt: time.Now().Add(-time.Minute)})
	if _, err := svc.CompleteOpenAIConnect("code=a&state=s"); !errors.Is(err, ErrValidation) {
		t.Fatalf("expired flow = %v, want ErrValidation", err)
	}

	// Pending flows are per-user: user 2's completion can't consume user 1's.
	svc.oauth.setPending(&oauthPending{userID: 1, verifier: "v", state: "s1", expiresAt: time.Now().Add(time.Minute)})
	other := svc.ForUser(2)
	if _, err := other.CompleteOpenAIConnect("code=a&state=s1"); !errors.Is(err, ErrValidation) {
		t.Fatalf("cross-user complete = %v, want ErrValidation (no pending for user 2)", err)
	}
	if p := svc.oauth.takePending(1); p == nil {
		t.Error("user 1's pending flow must survive user 2's attempt")
	}

	// Starting again replaces the flow; the state-scan path burns on take.
	svc.oauth.setPending(&oauthPending{userID: 1, verifier: "v1", state: "old", expiresAt: time.Now().Add(time.Minute)})
	svc.oauth.setPending(&oauthPending{userID: 1, verifier: "v2", state: "new", expiresAt: time.Now().Add(time.Minute)})
	if p := svc.oauth.takePendingByState("old"); p != nil {
		t.Error("replaced flow must be gone")
	}
	if p := svc.oauth.takePendingByState("new"); p == nil || p.verifier != "v2" {
		t.Errorf("takePendingByState(new) = %+v, want the replacing flow", p)
	}
	if p := svc.oauth.takePendingByState("new"); p != nil {
		t.Error("state-scan take must be single-use")
	}
}
