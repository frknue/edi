package services

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"edi/internal/db"
	"edi/internal/models"
)

// Auth model: token-based, no passwords. Every user owns one bearer token
// (shown once at creation/rotation; the server stores its SHA-256). All
// clients — web UI, CLI, MCP, Telegram — present it as `Authorization:
// Bearer <token>`, exactly like the old shared EDI_TOKEN, which now simply
// becomes user 1's token via AdoptEnvToken at startup.

// ErrUnauthorized marks a missing/invalid bearer token (mapped to HTTP 401).
var ErrUnauthorized = errors.New("unauthorized")

// mintToken returns a fresh 48-hex-char bearer token.
func mintToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken is the stored form of a token: SHA-256 hex. Deterministic (it is
// a lookup key), and enough for 192-bit random tokens — this is not a
// password, so no salt/KDF is needed.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// AuthenticateToken resolves a bearer token to a user id.
func (s *Service) AuthenticateToken(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, ErrUnauthorized
	}
	id, err := s.store.GetUserIDByTokenHash(hashToken(raw))
	if errors.Is(err, db.ErrNotFound) {
		return 0, ErrUnauthorized
	}
	return id, err
}

// Me returns the authenticated user.
func (s *Service) Me() (models.User, error) {
	u, err := s.store.GetUser(s.userID)
	if errors.Is(err, db.ErrNotFound) {
		return models.User{}, ErrNotFound
	}
	return u, err
}

// RegistrationOpen reports whether self-serve signup is enabled (an invite
// code is configured via EDI_INVITE_CODE).
func RegistrationOpen() bool { return os.Getenv("EDI_INVITE_CODE") != "" }

// RegisterUser creates a new user (fresh level-1 character) from a name and
// the server's invite code, returning the one-time-visible token.
func (s *Service) RegisterUser(in models.RegisterInput) (models.CreatedUser, error) {
	code := os.Getenv("EDI_INVITE_CODE")
	if code == "" {
		return models.CreatedUser{}, validationErr("registration is disabled on this server (EDI_INVITE_CODE is not set)")
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(in.InviteCode)), []byte(code)) != 1 {
		return models.CreatedUser{}, validationErr("wrong invite code")
	}
	return s.createUser(in.Name, false)
}

// CreateUser is the admin path for adding a user (no invite code involved).
// The HTTP layer gates it on the caller being an admin.
func (s *Service) CreateUser(name string) (models.CreatedUser, error) {
	return s.createUser(name, false)
}

func (s *Service) createUser(name string, isAdmin bool) (models.CreatedUser, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.CreatedUser{}, validationErr("name is required")
	}
	if len(name) > 40 {
		return models.CreatedUser{}, validationErr("name is too long (max 40 characters)")
	}
	token, err := mintToken()
	if err != nil {
		return models.CreatedUser{}, err
	}
	u, err := s.store.CreateUserWithDefaults(name, isAdmin)
	if err != nil {
		return models.CreatedUser{}, err
	}
	if err := s.store.SetUserTokenHash(u.ID, hashToken(token)); err != nil {
		return models.CreatedUser{}, err
	}
	return models.CreatedUser{User: u, Token: token}, nil
}

// ListUsers returns all users (admin surface).
func (s *Service) ListUsers() ([]models.User, error) {
	out, err := s.store.ListUsers()
	return orEmpty(out), err
}

// RotateUserToken mints a fresh token for a user (recovery for a lost token).
// User 1 is refused: their token is adopted from EDI_TOKEN at every boot, so a
// rotation here would be silently reverted on restart — change the env var
// instead.
func (s *Service) RotateUserToken(targetID int64) (models.CreatedUser, error) {
	if targetID == adoptedUserID {
		return models.CreatedUser{}, validationErr("user 1's token comes from EDI_TOKEN — change the env var and restart instead")
	}
	u, err := s.store.GetUser(targetID)
	if errors.Is(err, db.ErrNotFound) {
		return models.CreatedUser{}, ErrNotFound
	}
	if err != nil {
		return models.CreatedUser{}, err
	}
	token, err := mintToken()
	if err != nil {
		return models.CreatedUser{}, err
	}
	if err := s.store.SetUserTokenHash(targetID, hashToken(token)); err != nil {
		return models.CreatedUser{}, err
	}
	return models.CreatedUser{User: u, Token: token}, nil
}

// adoptedUserID is the user EDI_TOKEN maps onto: the original single-user id.
const adoptedUserID int64 = 1

// AdoptEnvToken is the startup bootstrap for token mode (EDI_TOKEN set):
// it guarantees user 1 exists (creating a blank admin "Hero" on an empty
// database) and idempotently (re)binds EDI_TOKEN as user 1's login token.
// Properties this buys:
//   - zero-migration upgrade: the token that worked against the single-user
//     server keeps working, now scoped to user 1;
//   - built-in recovery: user 1 can always get back in by setting EDI_TOKEN
//     and restarting (which is why RotateUserToken refuses user 1).
func (s *Service) AdoptEnvToken(envToken string) error {
	if envToken == "" {
		return nil
	}
	n, err := s.store.CountUsers()
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err := s.store.CreateUserWithDefaults("Hero", true); err != nil {
			return err
		}
	}
	return s.store.SetUserTokenHash(adoptedUserID, hashToken(envToken))
}
