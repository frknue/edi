package services

import (
	"time"

	"edi/internal/models"
)

// Rest mode pauses ALL decay (vacation / sick weeks). It is free but loud:
// the dashboard shows a banner while it is on. Turning it off restarts every
// attribute's idle clock from that moment (see the decay engine).
const (
	settingRestMode    = "rest_mode"     // "1" on, "" off
	settingRestSince   = "rest_since"    // RFC3339
	settingRestEndedAt = "rest_ended_at" // RFC3339, written when turned off
)

// SetRestMode turns rest mode on or off and returns the new state.
//
// Only a genuine transition writes a clock: off->on writes rest_since,
// on->off writes rest_ended_at. A redundant call (already in the requested
// state) is a no-op — it must NOT rewrite either clock, or it would grant
// every attribute a fresh grace period (or restart the "since" clock) for
// free. See the decay engine: the idle anchor is max(lastActivity,
// rest_ended_at), so a stray rest_ended_at bump silently erases idle days.
func (s *Service) SetRestMode(on bool) (models.RestState, error) {
	current, err := s.RestState()
	if err != nil {
		return models.RestState{}, err
	}
	if current.On == on {
		return current, nil
	}
	if on {
		if err := s.store.SetSetting(s.userID, settingRestMode, "1"); err != nil {
			return models.RestState{}, err
		}
		if err := s.store.SetSetting(s.userID, settingRestSince, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return models.RestState{}, err
		}
	} else {
		if err := s.store.SetSetting(s.userID, settingRestMode, ""); err != nil {
			return models.RestState{}, err
		}
		if err := s.store.SetSetting(s.userID, settingRestEndedAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return models.RestState{}, err
		}
	}
	return s.RestState()
}

// RestState reports the current rest mode.
func (s *Service) RestState() (models.RestState, error) {
	mode, err := s.store.GetSetting(s.userID, settingRestMode)
	if err != nil {
		return models.RestState{}, err
	}
	st := models.RestState{On: mode == "1"}
	if st.On {
		if raw, err := s.store.GetSetting(s.userID, settingRestSince); err == nil && raw != "" {
			if t, err := time.Parse(time.RFC3339, raw); err == nil {
				st.Since = &t
			}
		}
	}
	return st, nil
}

// restEndedAt returns when rest mode was last turned off (nil if never).
// The decay engine treats it as an idle-clock reset point.
func (s *Service) restEndedAt() (*time.Time, error) {
	raw, err := s.store.GetSetting(s.userID, settingRestEndedAt)
	if err != nil || raw == "" {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, nil // unreadable timestamp: treat as never
	}
	return &t, nil
}
