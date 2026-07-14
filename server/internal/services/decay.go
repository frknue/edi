package services

import (
	"time"

	"edi/internal/db"
	"edi/internal/models"
)

// Decay & stakes: neglected attributes bleed XP. Pure math lives here (like
// the level formula in xp.go); db/decay_math.go keeps private mirrors to
// avoid an import cycle — change both together.

const (
	// DecayGraceDays is how many idle days cost nothing.
	DecayGraceDays = 3
	// DecayMinPerDay is the minimum XP one billable idle day costs.
	DecayMinPerDay int64 = 5
	// WardCostGold is the flat gold price of a 7-day Maintenance Ward.
	WardCostGold int64 = 30
	// WardDays is how long one ward purchase shields an attribute.
	WardDays = 7
)

// DailyDecay returns the XP one billable idle day costs an attribute with
// the given current total: 1% of the total, minimum DecayMinPerDay, never
// more than the total itself. Non-positive totals cost nothing.
func DailyDecay(totalXP int64) int64 {
	if totalXP <= 0 {
		return 0
	}
	d := totalXP / 100
	if d < DecayMinPerDay {
		d = DecayMinPerDay
	}
	if d > totalXP {
		d = totalXP
	}
	return d
}

// DecayFloor returns the XP an attribute can never decay below: the start of
// (peak level - 2). Peaks at level <=3 floor at 0.
func DecayFloor(peakXP int64) int64 {
	return XPForLevel(LevelForXP(peakXP) - 2)
}

// ApplyDecay runs the lazy decay catch-up unless rest mode is on. It is
// called at the top of attribute-touching reads and before completions, so
// decay is always applied before new state is read or awarded. Returns the
// XP removed by this call (0 when nothing was owed).
func (s *Service) ApplyDecay() (int64, error) {
	rest, err := s.RestState()
	if err != nil {
		return 0, err
	}
	if rest.On {
		return 0, nil
	}
	ended, err := s.restEndedAt()
	if err != nil {
		return 0, err
	}
	return s.store.ApplyDecay(s.userID, ended, time.Now().UTC())
}

// decayStatus computes the read-side decay state for one attribute.
func decayStatus(a models.Attribute, in db.DecayInput, rest models.RestState, restEnded *time.Time, now time.Time) *models.AttributeDecay {
	d := &models.AttributeDecay{FloorLevel: LevelForXP(DecayFloor(a.PeakXP))}

	anchor := time.Time{}
	if in.LastActivity != nil {
		anchor = *in.LastActivity
	}
	if restEnded != nil && restEnded.After(anchor) {
		anchor = *restEnded
	}
	if !anchor.IsZero() {
		d.IdleDays = localDaysBetween(anchor, now)
	}
	d.WardedUntil = in.WardExpiry

	switch {
	case rest.On:
		d.State = "rest"
	case in.WardExpiry != nil:
		d.State = "warded"
	case d.IdleDays == 0:
		d.State = "fresh"
	case d.IdleDays <= DecayGraceDays:
		d.State = "grace"
	default:
		d.State = "decaying"
		d.ProjectedDailyLoss = DailyDecay(a.TotalXP)
		if DecayFloor(a.PeakXP) >= a.TotalXP {
			d.ProjectedDailyLoss = 0 // already at the floor: nothing more to lose
		}
	}
	return d
}

// localDaysBetween mirrors db.localDaysBetween (kept in sync).
func localDaysBetween(a, b time.Time) int {
	al, bl := a.Local(), b.Local()
	ad := time.Date(al.Year(), al.Month(), al.Day(), 0, 0, 0, 0, time.Local)
	bd := time.Date(bl.Year(), bl.Month(), bl.Day(), 0, 0, 0, 0, time.Local)
	return int(bd.Sub(ad).Hours()/24 + 0.5)
}
