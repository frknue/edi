package db

import (
	"database/sql"
	"time"
)

// OpenAICredentials is the stored ChatGPT-subscription OAuth state for a user.
type OpenAICredentials struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	AccountID    string
	Email        string
	ExpiresAt    time.Time
	UpdatedAt    time.Time
}

// GetOpenAICredentials returns the stored credentials, or (nil, nil) if none.
func (s *Store) GetOpenAICredentials(userID int64) (*OpenAICredentials, error) {
	var c OpenAICredentials
	err := s.db.QueryRow(
		`SELECT access_token, refresh_token, id_token, account_id, email, expires_at, updated_at
		 FROM openai_credentials WHERE user_id = $1`, userID).
		Scan(&c.AccessToken, &c.RefreshToken, &c.IDToken, &c.AccountID, &c.Email, &c.ExpiresAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// SaveOpenAICredentials upserts the credentials for a user.
func (s *Store) SaveOpenAICredentials(userID int64, c OpenAICredentials) error {
	_, err := s.db.Exec(
		`INSERT INTO openai_credentials(user_id, access_token, refresh_token, id_token, account_id, email, expires_at, updated_at)
		 VALUES($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT(user_id) DO UPDATE SET
		   access_token=excluded.access_token,
		   refresh_token=excluded.refresh_token,
		   id_token=excluded.id_token,
		   account_id=excluded.account_id,
		   email=excluded.email,
		   expires_at=excluded.expires_at,
		   updated_at=excluded.updated_at`,
		userID, c.AccessToken, c.RefreshToken, c.IDToken, c.AccountID, c.Email,
		c.ExpiresAt.UTC(), time.Now().UTC())
	return err
}

// DeleteOpenAICredentials removes any stored credentials for a user.
func (s *Store) DeleteOpenAICredentials(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM openai_credentials WHERE user_id = $1`, userID)
	return err
}

// GetSetting returns a per-user setting value, or "" if unset.
func (s *Store) GetSetting(userID int64, key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE user_id = $1 AND key = $2`, userID, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// SetSetting upserts a per-user setting.
func (s *Store) SetSetting(userID int64, key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO app_settings(user_id, key, value) VALUES($1, $2, $3)
		 ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value`,
		userID, key, value)
	return err
}
