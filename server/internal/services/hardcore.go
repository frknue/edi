package services

import (
	"time"

	"edi/internal/models"
)

// Hardcore mode is the opt-in punishment layer: attribute decay, missed-daily
// stakes, and wards. It is OFF by default — absence is the symptom the app
// exists to help with, so billing for it is a design flaw, not a feature.
// The engines stay intact behind this flag; when it is off they never write
// a negative xp_event, and the read side hides every decay/penalty surface.
const (
	settingHardcore      = "hardcore_mode"  // "1" on, "" off
	settingHardcoreSince = "hardcore_since" // RFC3339, written on the off->on transition
)

// HardcoreState reports whether the punishment layer is live.
func (s *Service) HardcoreState() (models.HardcoreState, error) {
	mode, err := s.store.GetSetting(s.userID, settingHardcore)
	if err != nil {
		return models.HardcoreState{}, err
	}
	st := models.HardcoreState{On: mode == "1"}
	if st.On {
		if since, err := s.hardcoreSince(); err == nil && since != nil {
			st.Since = since
		}
	}
	return st, nil
}

// SetHardcoreMode turns the punishment layer on or off. Turning it on anchors
// every idle clock at NOW (hardcore_since) so a month of decay-free idleness
// is never billed retroactively; the daily-stake cursor is likewise settled
// through yesterday under the CURRENT (off) rules before the switch.
func (s *Service) SetHardcoreMode(on bool) (models.HardcoreState, error) {
	current, err := s.HardcoreState()
	if err != nil {
		return models.HardcoreState{}, err
	}
	if current.On == on {
		return current, nil
	}
	if err := s.rollOverRecurringQuests(); err != nil {
		return models.HardcoreState{}, err
	}
	if on {
		if err := s.store.SetSetting(s.userID, settingHardcoreSince, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return models.HardcoreState{}, err
		}
		if err := s.store.SetSetting(s.userID, settingHardcore, "1"); err != nil {
			return models.HardcoreState{}, err
		}
	} else if err := s.store.SetSetting(s.userID, settingHardcore, ""); err != nil {
		return models.HardcoreState{}, err
	}
	return s.HardcoreState()
}

// hardcoreOn is the cheap boolean read used by the engines.
func (s *Service) hardcoreOn() (bool, error) {
	mode, err := s.store.GetSetting(s.userID, settingHardcore)
	return mode == "1", err
}

// hardcoreSince returns when hardcore mode was last turned on (nil if never).
func (s *Service) hardcoreSince() (*time.Time, error) {
	raw, err := s.store.GetSetting(s.userID, settingHardcoreSince)
	if err != nil || raw == "" {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, nil
	}
	return &t, nil
}

// idleAnchorFloor is the earliest moment an idle clock may start from: the
// later of "rest mode ended" and "hardcore mode turned on". Nil when neither
// ever happened.
func (s *Service) idleAnchorFloor() (*time.Time, error) {
	ended, err := s.restEndedAt()
	if err != nil {
		return nil, err
	}
	since, err := s.hardcoreSince()
	if err != nil {
		return nil, err
	}
	switch {
	case ended == nil:
		return since, nil
	case since == nil:
		return ended, nil
	case since.After(*ended):
		return since, nil
	default:
		return ended, nil
	}
}
