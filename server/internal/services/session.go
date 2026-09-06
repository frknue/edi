package services

import (
	"errors"
	"strings"
	"time"

	"edi/internal/db"
	"edi/internal/models"
)

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
	}
	return sess, err
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
