package services

import (
	"sort"
	"time"

	"edi/internal/models"
)

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

// BoardClearRewards mirrors db.boardClearRewards (display only).
var BoardClearRewards = map[string]int64{"discipline": 15, "focus": 10}

// ProjectPayout is what completing q pays RIGHT NOW, mirroring the award
// pipeline in db.completeQuest minus the crit roll: base rewards plus
// checked subtask bonuses, the combo bonus for the nth completion of the
// day, and active buff bonuses. Read-side only; TestProjectionMatchesAward
// keeps it honest against the real award path.
func ProjectPayout(q models.Quest, nth int, buffs []models.ActiveBuff) int64 {
	type award struct {
		key    string
		amount int64
	}
	var base []award
	for _, key := range sortedKeys(q.AttributeRewards) {
		if v := q.AttributeRewards[key]; v != 0 {
			base = append(base, award{key, v})
		}
	}
	for _, st := range q.Subtasks {
		if !st.Done {
			continue
		}
		for _, key := range sortedKeys(st.AttributeRewards) {
			if v := st.AttributeRewards[key]; v != 0 {
				base = append(base, award{key, v})
			}
		}
	}
	var total int64
	for _, a := range base {
		total += a.amount
	}
	if m := ComboMultiplier(nth); m > 1.0 {
		for _, a := range base {
			if bonus := int64(float64(a.amount) * (m - 1.0)); bonus > 0 {
				total += bonus
			}
		}
	}
	for _, a := range base {
		pct := 0
		for _, b := range buffs {
			if b.Attribute == "" || b.Attribute == a.key {
				pct += b.Percent
			}
		}
		if pct > 0 {
			if bonus := int64(float64(a.amount) * float64(pct) / 100.0); bonus > 0 {
				total += bonus
			}
		}
	}
	return total
}

// buffApplies reports whether any active buff boosts one of q's rewards.
func buffApplies(q models.Quest, buffs []models.ActiveBuff) bool {
	for _, b := range buffs {
		if b.Attribute == "" {
			return len(q.AttributeRewards) > 0
		}
		if q.AttributeRewards[b.Attribute] > 0 {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ListItems returns the user's loot inventory (newest first).
func (s *Service) ListItems() ([]models.ItemDrop, error) {
	out, err := s.store.ListItems(s.userID, 200)
	return orEmpty(out), err
}

// ActiveBuffs returns the user's running loot buffs.
func (s *Service) ActiveBuffs() ([]models.ActiveBuff, error) {
	out, err := s.store.ActiveBuffs(s.userID, time.Now().UTC())
	return orEmpty(out), err
}
