package db

// Private mirrors of services.DailyDecay / services.DecayFloor (and the
// xpForLevel inverse), duplicated to avoid a db->services import cycle —
// the same trick as levelForXP/goldForXP. Keep in sync with
// services/decay.go and services/xp.go.

const (
	decayGraceDays       = 3
	decayMinPerDay int64 = 5
)

func decayDailyAmount(totalXP int64) int64 {
	if totalXP <= 0 {
		return 0
	}
	d := totalXP / 100
	if d < decayMinPerDay {
		d = decayMinPerDay
	}
	if d > totalXP {
		d = totalXP
	}
	return d
}

func decayFloorXP(peakXP int64) int64 {
	return xpForLevel(levelForXP(peakXP) - 2)
}
