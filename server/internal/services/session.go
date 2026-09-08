package services

import (
	"errors"
	"strings"
	"sync"
	"time"

	"edi/internal/db"
	"edi/internal/models"
)

// hookRuntime holds process-wide callbacks fired after a quest starts —
// the body-double ping: presence tells the board partner. Shared by every
// ForUser copy (pointer), like the OAuth and Telegram runtimes.
type hookRuntime struct {
	mu      sync.Mutex
	onStart []func(userID int64, sess models.QuestSession)
}

// OnQuestStart registers a callback run (in its own goroutine) after a
// session opens. Transports use it; it never affects the result.
func (s *Service) OnQuestStart(fn func(userID int64, sess models.QuestSession)) {
	s.hooks.mu.Lock()
	defer s.hooks.mu.Unlock()
	s.hooks.onStart = append(s.hooks.onStart, fn)
}

func (s *Service) fireStartHooks(sess models.QuestSession) {
	s.hooks.mu.Lock()
	fns := append([]func(int64, models.QuestSession){}, s.hooks.onStart...)
	s.hooks.mu.Unlock()
	for _, fn := range fns {
		go fn(s.userID, sess)
	}
}

// BoardPartner returns the other member of the user's quest board.
func (s *Service) BoardPartner() (int64, string, bool, error) {
	return s.store.BoardPartner(s.userID)
}

// partnerSession is what the board partner is working on right now.
func (s *Service) partnerSession() (*models.PartnerSession, error) {
	partnerID, name, ok, err := s.store.BoardPartner(s.userID)
	if err != nil || !ok {
		return nil, err
	}
	sess, err := s.store.ActiveQuestSession(partnerID, time.Now().UTC())
	if err != nil || sess == nil {
		return nil, err
	}
	return &models.PartnerSession{Name: name, Title: sess.Title, ElapsedSeconds: sess.ElapsedSeconds}, nil
}

// Active quest mode: Start opens a session on a quest (the home screen
// becomes the running quest with a timer), Stop closes it and asks for the
// next physical action (stored as the quest's resume note), Complete is the
// finisher (closed inside the completion tx). Sessions are presence during
// the action — they NEVER write XP; the client ticks cosmetically.

// maxResumeNote bounds the landing answer (one line, not an essay).
const maxResumeNote = 280

// StartQuest opens a session on the quest, auto-closing any other running
// session (reason 'switched'). Shared quests resolve to the caller's copy.
func (s *Service) StartQuest(id int64) (models.QuestSession, error) {
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.QuestSession{}, err
	}
	visible, err := s.store.GetVisibleQuest(s.userID, id)
	if err != nil {
		return models.QuestSession{}, ErrNotFound
	}
	if visible.SharedQuestID != nil {
		if !visible.AssignedToMe {
			return models.QuestSession{}, validationErr("this quest is assigned to another board member")
		}
		if visible.MyStatus != models.StatusActive {
			return models.QuestSession{}, validationErr("your part of this quest is not active")
		}
		id = visible.ID
	}
	sess, err := s.store.StartQuestSession(s.userID, id, time.Now().UTC())
	switch {
	case errors.Is(err, db.ErrNotFound):
		return models.QuestSession{}, ErrNotFound
	case errors.Is(err, db.ErrQuestNotCompletable):
		return models.QuestSession{}, validationErr("only an active quest can be started")
	case err != nil:
		return models.QuestSession{}, err
	}
	s.fireStartHooks(sess)
	return sess, nil
}

// StopQuest closes the running session and stores the landing note ("next
// physical action") on the quest. 400 when nothing is running.
func (s *Service) StopQuest(in models.StopSessionInput) (models.QuestSession, error) {
	note := strings.TrimSpace(in.Note)
	if len(note) > maxResumeNote {
		return models.QuestSession{}, validationErr("the resume note must be at most %d characters", maxResumeNote)
	}
	sess, err := s.store.StopQuestSession(s.userID, note, time.Now().UTC())
	if errors.Is(err, db.ErrNotFound) {
		return models.QuestSession{}, validationErr("no quest is running")
	}
	return sess, err
}

// ActiveSession returns the running session (nil when none), expiring a
// session left over from a previous local day.
func (s *Service) ActiveSession() (*models.QuestSession, error) {
	return s.store.ActiveQuestSession(s.userID, time.Now().UTC())
}

// ListQuestSessions returns recent sessions, newest first.
func (s *Service) ListQuestSessions(limit int) ([]models.QuestSession, error) {
	out, err := s.store.ListQuestSessions(s.userID, limit, time.Now().UTC())
	return orEmpty(out), err
}
