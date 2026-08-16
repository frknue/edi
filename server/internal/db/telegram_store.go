package db

import (
	"database/sql"
	"time"
)

// TelegramLink pairs a user with their Telegram chat.
type TelegramLink struct {
	UserID    int64
	ChatID    int64
	CreatedAt time.Time
}

// UpsertTelegramLink links a chat to a user. A user re-pairing from a new
// chat replaces their old link; a chat already linked to a DIFFERENT user is
// refused (unique constraint) — the service turns that into a friendly error.
func (s *Store) UpsertTelegramLink(userID, chatID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO telegram_links(user_id, chat_id, created_at) VALUES($1, $2, $3)
		 ON CONFLICT(user_id) DO UPDATE SET chat_id = excluded.chat_id, created_at = excluded.created_at`,
		userID, chatID, time.Now().UTC())
	return err
}

// GetTelegramLinkByChat resolves an incoming chat to its user (ErrNotFound if
// the chat is unpaired).
func (s *Store) GetTelegramLinkByChat(chatID int64) (TelegramLink, error) {
	var l TelegramLink
	err := s.db.QueryRow(`SELECT user_id, chat_id, created_at FROM telegram_links WHERE chat_id = $1`, chatID).
		Scan(&l.UserID, &l.ChatID, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return l, ErrNotFound
	}
	return l, err
}

// GetTelegramLinkByUser returns a user's link (ErrNotFound if unpaired).
func (s *Store) GetTelegramLinkByUser(userID int64) (TelegramLink, error) {
	var l TelegramLink
	err := s.db.QueryRow(`SELECT user_id, chat_id, created_at FROM telegram_links WHERE user_id = $1`, userID).
		Scan(&l.UserID, &l.ChatID, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return l, ErrNotFound
	}
	return l, err
}

// ListTelegramLinks returns every pairing (the scheduler fans out over these).
func (s *Store) ListTelegramLinks() ([]TelegramLink, error) {
	rows, err := s.db.Query(`SELECT user_id, chat_id, created_at FROM telegram_links ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TelegramLink
	for rows.Next() {
		var l TelegramLink
		if err := rows.Scan(&l.UserID, &l.ChatID, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeleteTelegramLinkByUser unpairs a user (no-op if unpaired).
func (s *Store) DeleteTelegramLinkByUser(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM telegram_links WHERE user_id = $1`, userID)
	return err
}

// DeleteTelegramLinkByChat unpairs a chat (no-op if unpaired).
func (s *Store) DeleteTelegramLinkByChat(chatID int64) error {
	_, err := s.db.Exec(`DELETE FROM telegram_links WHERE chat_id = $1`, chatID)
	return err
}
