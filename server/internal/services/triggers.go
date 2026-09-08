package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"edi/internal/models"
)

// If-then triggers (implementation intentions): a quest carries a cue in
// plain words ("after coffee") and, optionally, a clock anchor (HH:MM
// local). At the anchor the presence scheduler sends ONE line — "Coffee's
// done → 10 min on the tax letter. Start?" — with Start / Done / Not now
// buttons. The decision is moved out of the moment it would have to be
// made. Fires at most once per quest per local day, and at most
// maxTriggerFiresPerDay per user so the cue never becomes spam.

const (
	maxTriggerLen          = 120
	maxTriggerFiresPerDay  = 5
	settingTriggerFired    = "trigger_fired_" // + questID → YYYY-MM-DD
	settingTriggerFireDate = "trigger_fires_date"
	settingTriggerFires    = "trigger_fires_count"
)

var hhmmRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// validateTrigger normalizes the cue and the clock anchor.
func validateTrigger(trigger, at *string) error {
	if trigger != nil {
		*trigger = strings.TrimSpace(*trigger)
		if len(*trigger) > maxTriggerLen {
			return validationErr("trigger must be at most %d characters", maxTriggerLen)
		}
	}
	if at != nil {
		*at = strings.TrimSpace(*at)
		if *at != "" && !hhmmRe.MatchString(*at) {
			return validationErr("trigger_at must be HH:MM (24h), got %q", *at)
		}
	}
	return nil
}

// DueTriggers returns the quests whose clock anchor is this minute and that
// have not fired today (respecting the per-day cap). Read-only; the caller
// marks what it actually sent with MarkTriggerFired.
func (s *Service) DueTriggers(now time.Time) ([]models.Quest, error) {
	quests, err := s.store.DueTriggers(s.userID, now.Local().Format("15:04"))
	if err != nil {
		return nil, err
	}
	today := localDate(now).Format("2006-01-02")
	fired, err := s.triggerFiresToday(today)
	if err != nil {
		return nil, err
	}
	var out []models.Quest
	for _, q := range quests {
		if fired >= maxTriggerFiresPerDay {
			break
		}
		last, err := s.store.GetSetting(s.userID, settingTriggerFired+fmt.Sprint(q.ID))
		if err != nil {
			return nil, err
		}
		if last == today {
			continue
		}
		out = append(out, q)
		fired++
	}
	return orEmpty(out), nil
}

// MarkTriggerFired records that the prompt for questID went out today.
func (s *Service) MarkTriggerFired(questID int64, now time.Time) error {
	today := localDate(now).Format("2006-01-02")
	if err := s.store.SetSetting(s.userID, settingTriggerFired+fmt.Sprint(questID), today); err != nil {
		return err
	}
	n, err := s.triggerFiresToday(today)
	if err != nil {
		return err
	}
	if err := s.store.SetSetting(s.userID, settingTriggerFireDate, today); err != nil {
		return err
	}
	return s.store.SetSetting(s.userID, settingTriggerFires, fmt.Sprint(n+1))
}

func (s *Service) triggerFiresToday(today string) (int, error) {
	date, err := s.store.GetSetting(s.userID, settingTriggerFireDate)
	if err != nil || date != today {
		return 0, err
	}
	raw, err := s.store.GetSetting(s.userID, settingTriggerFires)
	if err != nil {
		return 0, err
	}
	var n int
	fmt.Sscanf(raw, "%d", &n)
	return n, nil
}
