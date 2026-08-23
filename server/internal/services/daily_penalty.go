package services

// MissedDailyPenaltyPercent is the share of each base attribute reward lost
// when an active daily quest is not completed by the end of its local day.
const MissedDailyPenaltyPercent int64 = 25

// MissedDailyPenaltyMinimum prevents tiny rewards from making a miss
// meaningless. The loss is still capped at the reward and at the attribute's
// current XP by the transactional store path.
const MissedDailyPenaltyMinimum int64 = 5

// MissedDailyPenalty returns the XP owed for one attribute reward. This pure
// rule is mirrored by db.dailyQuestPenalty, which runs inside the audit tx.
func MissedDailyPenalty(reward int64) int64 {
	if reward <= 0 {
		return 0
	}
	penalty := reward * MissedDailyPenaltyPercent / 100
	if penalty < MissedDailyPenaltyMinimum {
		penalty = MissedDailyPenaltyMinimum
	}
	if penalty > reward {
		penalty = reward
	}
	return penalty
}
