package services

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"edi/internal/db"
	"edi/internal/models"
)

// The shutdown ritual: in camp the user picks "the first thing tomorrow";
// the recommender honors it on that day and the morning briefing leads
// with it. Dismissing it costs nothing and never counts as a skip.
const settingFirstMove = "first_move" // "YYYY-MM-DD|questID"

// SetFirstMove pins questID as the first move of today or tomorrow.
func (s *Service) SetFirstMove(in models.FirstMoveInput) (*models.FirstMove, error) {
	visible, err := s.store.GetVisibleQuest(s.userID, in.QuestID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !visible.AssignedToMe {
		return nil, validationErr("this quest is assigned to another board member")
	}
	day := localDate(time.Now())
	if in.Tomorrow {
		day = day.AddDate(0, 0, 1)
	}
	value := fmt.Sprintf("%s|%d", day.Format("2006-01-02"), visible.ID)
	if err := s.store.SetSetting(s.userID, settingFirstMove, value); err != nil {
		return nil, err
	}
	return &models.FirstMove{Day: day.Format("2006-01-02"), Quest: visible}, nil
}

// ClearFirstMove drops the pin (never a skip, never counted).
func (s *Service) ClearFirstMove() error {
	return s.store.SetSetting(s.userID, settingFirstMove, "")
}

// firstMove returns the pinned quest for today or tomorrow (nil when unset,
// stale, or no longer active).
func (s *Service) firstMove(now time.Time) (*models.FirstMove, error) {
	raw, err := s.store.GetSetting(s.userID, settingFirstMove)
	if err != nil || raw == "" {
		return nil, err
	}
	dayStr, idStr, ok := strings.Cut(raw, "|")
	if !ok {
		return nil, nil
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, nil
	}
	today := localDate(now).Format("2006-01-02")
	tomorrow := localDate(now).AddDate(0, 0, 1).Format("2006-01-02")
	if dayStr != today && dayStr != tomorrow {
		return nil, nil // stale: yesterday's pin is gone, no guilt object
	}
	q, err := s.store.GetVisibleQuest(s.userID, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if !q.AssignedToMe || q.MyStatus == models.StatusArchived {
		return nil, nil
	}
	// Today's pin must still be doable; tomorrow's may be a daily completed
	// today (it rolls back to active at midnight).
	if dayStr == today && q.MyStatus != models.StatusActive {
		return nil, nil
	}
	return &models.FirstMove{Day: dayStr, Quest: q}, nil
}
