package db

import (
	"database/sql"
	"time"

	"edi/internal/models"
)

// CreateAccountInvite stores only the hash of an admin-minted onboarding
// code. There is no read-before-write step, so no per-user advisory lock is
// required here.
func (s *Store) CreateAccountInvite(createdBy int64, codeHash string, expiresAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO account_invites(code_hash, created_by, created_at, expires_at)
		 VALUES($1, $2, $3, $4)`,
		codeHash, createdBy, time.Now().UTC(), expiresAt)
	return err
}

// CreateUserFromAccountInvite atomically claims an unexpired invitation and
// creates a fully initialized user. UPDATE locks the matching invite row, so
// concurrent submissions cannot both consume the same code.
func (s *Store) CreateUserFromAccountInvite(
	codeHash, name, tokenHash string,
	now time.Time,
) (models.User, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.User{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var inviteID int64
	if err := tx.QueryRow(
		`UPDATE account_invites
		 SET consumed_at = $1
		 WHERE code_hash = $2 AND consumed_at IS NULL AND expires_at > $1
		 RETURNING id`, now, codeHash).Scan(&inviteID); err != nil {
		if err == sql.ErrNoRows {
			return models.User{}, ErrInvalidAccountInvite
		}
		return models.User{}, err
	}

	u, err := createUserWithDefaultsTx(tx, name, false, tokenHash, now)
	if err != nil {
		return models.User{}, err
	}
	if _, err := tx.Exec(`UPDATE account_invites SET consumed_by = $1 WHERE id = $2`, u.ID, inviteID); err != nil {
		return models.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.User{}, err
	}
	return u, nil
}
