package services

import "math"

// XP / level math. Kept as pure functions so they are trivial to unit-test and
// reuse from any client.
//
// MVP level formula:
//
//	level = floor(sqrt(total_xp / 100)) + 1
//
// Inverting it, the XP threshold at which a given level begins is:
//
//	xpForLevel(L) = (L-1)^2 * 100

// LevelForXP returns the 1-based level for a given total XP.
func LevelForXP(totalXP int64) int {
	if totalXP < 0 {
		totalXP = 0
	}
	return int(math.Floor(math.Sqrt(float64(totalXP)/100.0))) + 1
}

// XPForLevel returns the total XP required to reach the start of a level.
func XPForLevel(level int) int64 {
	if level < 1 {
		level = 1
	}
	l := int64(level - 1)
	return l * l * 100
}

// ProgressForXP decomposes a total XP value into its level and progress toward
// the next level.
//
//	level          – current 1-based level
//	xpIntoLevel    – XP earned since the current level began
//	xpForNextLevel – XP span of the current level (next threshold - current)
//	ratio          – xpIntoLevel / xpForNextLevel, clamped to [0,1]
func ProgressForXP(totalXP int64) (level int, xpIntoLevel, xpForNextLevel int64, ratio float64) {
	level = LevelForXP(totalXP)
	current := XPForLevel(level)
	next := XPForLevel(level + 1)
	xpIntoLevel = totalXP - current
	xpForNextLevel = next - current
	if xpForNextLevel > 0 {
		ratio = float64(xpIntoLevel) / float64(xpForNextLevel)
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return level, xpIntoLevel, xpForNextLevel, ratio
}
