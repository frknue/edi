package db

// Keep this private mirror in sync with services.MissedDailyPenalty. The store
// needs the calculation inside the same transaction that writes the XP ledger,
// and importing services here would create a package cycle.
const (
	dailyPenaltyPercent int64 = 25
	dailyPenaltyMinimum int64 = 5
)

func dailyQuestPenalty(reward int64) int64 {
	if reward <= 0 {
		return 0
	}
	penalty := reward * dailyPenaltyPercent / 100
	if penalty < dailyPenaltyMinimum {
		penalty = dailyPenaltyMinimum
	}
	if penalty > reward {
		penalty = reward
	}
	return penalty
}
