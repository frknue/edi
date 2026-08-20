package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"edi/internal/db"
	"edi/internal/models"
)

// Telegram presence: pairing between edi users and Telegram chats. The bot
// itself (long-poll loop, commands, pushes) lives in internal/presence — a
// transport like the HTTP handlers; all state changes go through here.

// telegramRuntime is process-wide pairing state shared by ForUser copies
// (same pattern as oauthRuntime): short-lived pair codes and the bot identity.
type telegramRuntime struct {
	mu          sync.Mutex
	codes       map[string]telegramPairCode // by code
	botUsername string                      // set by the presence runner at startup
	configured  bool                        // TELEGRAM_BOT_TOKEN present + bot reachable
}

type telegramPairCode struct {
	userID    int64
	expiresAt time.Time
}

// SetTelegramBotInfo is called once by the presence runner after getMe.
func (s *Service) SetTelegramBotInfo(username string) {
	s.telegram.mu.Lock()
	defer s.telegram.mu.Unlock()
	s.telegram.botUsername = username
	s.telegram.configured = true
}

// CreateTelegramPairCode mints a short-lived, single-use code the user sends
// to the bot (/pair <code>, or via the t.me deep link's /start payload). The
// code is a bearer credential for this account's Telegram access: 8 random
// hex chars, 10-minute TTL, burned on first claim attempt.
func (s *Service) CreateTelegramPairCode() (models.TelegramPairCode, error) {
	s.telegram.mu.Lock()
	defer s.telegram.mu.Unlock()
	if !s.telegram.configured {
		return models.TelegramPairCode{}, validationErr("Telegram is not configured on this server (TELEGRAM_BOT_TOKEN unset)")
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return models.TelegramPairCode{}, err
	}
	code := hex.EncodeToString(b)
	if s.telegram.codes == nil {
		s.telegram.codes = map[string]telegramPairCode{}
	}
	now := time.Now()
	for c, e := range s.telegram.codes {
		if now.After(e.expiresAt) || e.userID == s.userID { // sweep expired + replace own
			delete(s.telegram.codes, c)
		}
	}
	s.telegram.codes[code] = telegramPairCode{userID: s.userID, expiresAt: now.Add(10 * time.Minute)}
	return models.TelegramPairCode{
		Code:        code,
		BotUsername: s.telegram.botUsername,
		ExpiresAt:   s.telegram.codes[code].expiresAt.UTC(),
	}, nil
}

// ClaimTelegramPairCode links chatID to the code's user. Called by the
// presence runner (no user context — the code IS the credential). Burns the
// code on every attempt.
func (s *Service) ClaimTelegramPairCode(code string, chatID int64) (models.User, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	s.telegram.mu.Lock()
	entry, ok := s.telegram.codes[code]
	delete(s.telegram.codes, code)
	s.telegram.mu.Unlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return models.User{}, validationErr("that pairing code is unknown or expired — get a fresh one from the app")
	}
	if err := s.store.UpsertTelegramLink(entry.userID, chatID); err != nil {
		// The chat_id unique constraint: this chat already belongs to someone.
		return models.User{}, validationErr("this chat is already linked to another account — /unpair there first")
	}
	u, err := s.store.GetUser(entry.userID)
	if err != nil {
		return models.User{}, err
	}
	return u, nil
}

// TelegramStatus is the web UI's view: is the bot configured, is THIS user
// linked, and what bot to deep-link to.
func (s *Service) TelegramStatus() (models.TelegramStatus, error) {
	s.telegram.mu.Lock()
	configured, bot := s.telegram.configured, s.telegram.botUsername
	s.telegram.mu.Unlock()
	st := models.TelegramStatus{Configured: configured, BotUsername: bot}
	if !configured {
		return st, nil
	}
	_, err := s.store.GetTelegramLinkByUser(s.userID)
	switch {
	case err == nil:
		st.Linked = true
	case errors.Is(err, db.ErrNotFound):
	default:
		return st, err
	}
	return st, nil
}

// UnlinkTelegram removes the current user's pairing (web-side unlink).
func (s *Service) UnlinkTelegram() error { return s.store.DeleteTelegramLinkByUser(s.userID) }

// UnlinkTelegramChat removes a chat's pairing (the /unpair command).
func (s *Service) UnlinkTelegramChat(chatID int64) error {
	return s.store.DeleteTelegramLinkByChat(chatID)
}

// UserIDForTelegramChat resolves an incoming chat to its user.
func (s *Service) UserIDForTelegramChat(chatID int64) (int64, error) {
	l, err := s.store.GetTelegramLinkByChat(chatID)
	if errors.Is(err, db.ErrNotFound) {
		return 0, ErrNotFound
	}
	return l.UserID, err
}

// ListTelegramLinks feeds the push scheduler.
func (s *Service) ListTelegramLinks() ([]db.TelegramLink, error) {
	return s.store.ListTelegramLinks()
}

// Per-user push times (falling back to the server defaults in the runner).
const (
	settingBriefingTime = "telegram_briefing_time"
	settingNudgeTime    = "telegram_nudge_time"
)

// TelegramPushTime returns the user's configured HH:MM for "briefing" or
// "nudge" ("" = use the server default).
func (s *Service) TelegramPushTime(kind string) (string, error) {
	switch kind {
	case "briefing":
		return s.store.GetSetting(s.userID, settingBriefingTime)
	case "nudge":
		return s.store.GetSetting(s.userID, settingNudgeTime)
	}
	return "", validationErr("unknown push kind %q", kind)
}

// TelegramPushTimes returns both per-user push times ("" = server default).
func (s *Service) TelegramPushTimes() (models.TelegramPushTimes, error) {
	b, err := s.TelegramPushTime("briefing")
	if err != nil {
		return models.TelegramPushTimes{}, err
	}
	n, err := s.TelegramPushTime("nudge")
	if err != nil {
		return models.TelegramPushTimes{}, err
	}
	return models.TelegramPushTimes{Briefing: b, Nudge: n}, nil
}

// SetTelegramPushTimes applies a partial update (nil = leave unchanged).
// Every client — web, CLI, agent, Telegram — lands here; the scheduler picks
// the change up on its next tick, whoever wrote it.
func (s *Service) SetTelegramPushTimes(p models.TelegramPushTimesPatch) (models.TelegramPushTimes, error) {
	if p.Briefing != nil {
		if err := s.SetTelegramPushTime("briefing", *p.Briefing); err != nil {
			return models.TelegramPushTimes{}, err
		}
	}
	if p.Nudge != nil {
		if err := s.SetTelegramPushTime("nudge", *p.Nudge); err != nil {
			return models.TelegramPushTimes{}, err
		}
	}
	return s.TelegramPushTimes()
}

// SetTelegramPushTime stores a per-user HH:MM for "briefing" or "nudge".
// An empty string clears the override (back to the server default).
func (s *Service) SetTelegramPushTime(kind, hhmm string) error {
	if _, err := time.Parse("15:04", hhmm); hhmm != "" && err != nil {
		return validationErr("%q is not a valid time — use HH:MM (e.g. 07:30)", hhmm)
	}
	switch kind {
	case "briefing":
		return s.store.SetSetting(s.userID, settingBriefingTime, hhmm)
	case "nudge":
		return s.store.SetSetting(s.userID, settingNudgeTime, hhmm)
	}
	return validationErr("unknown push kind %q", kind)
}
