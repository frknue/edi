package services

// Game-layer display math — mirrors db/game_math.go (keep in sync; the store
// owns the awarding, this feeds dashboards and previews).

// ComboMultiplier returns the chain multiplier the nth completion of the
// local day pays (1-based). 1st ×1.0, 2nd ×1.1, 3rd ×1.2, 4th ×1.35, 5th+ ×1.5.
func ComboMultiplier(nth int) float64 {
	switch {
	case nth <= 1:
		return 1.0
	case nth == 2:
		return 1.1
	case nth == 3:
		return 1.2
	case nth == 4:
		return 1.35
	default:
		return 1.5
	}
}
