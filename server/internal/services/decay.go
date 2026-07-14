package services

import "time"

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
