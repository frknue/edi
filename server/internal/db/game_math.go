package db

import (
	"crypto/rand"
	"encoding/binary"
	mrand "math/rand/v2"
	"sync"
)

// Game-layer tuning (mirrored in services/game.go for display — keep in sync).
//
// Crit: every quest completion rolls once; a crit doubles the ENTIRE payout
// (base + subtask bonuses) via separate auditable xp_events (source 'crit').
// Combo: the Nth completion of the same local day pays a chain multiplier;
// the extra lands as source 'combo' rows. Both stack additively:
//
//	total = base × (1 + crit(1.0) + (combo−1))
const critChance = 0.15

// comboMultiplier returns the chain multiplier for the nth completion of the
// local day (1-based). 1st ×1.0, 2nd ×1.1, 3rd ×1.2, 4th ×1.35, 5th+ ×1.5.
func comboMultiplier(nth int) float64 {
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

// rng is the store's dice: a mutex-guarded PCG seeded from crypto/rand.
// Tests inject a deterministic roll via SetRollForTest (export_test.go);
// production code has no way to bias it.
type rng struct {
	mu   sync.Mutex
	src  *mrand.Rand
	roll func() float64 // test override; nil in production
}

func newRNG() *rng {
	var seed [16]byte
	_, _ = rand.Read(seed[:])
	return &rng{src: mrand.New(mrand.NewPCG(
		binary.LittleEndian.Uint64(seed[:8]),
		binary.LittleEndian.Uint64(seed[8:]),
	))}
}

// Float64 returns a uniform roll in [0,1).
func (r *rng) Float64() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.roll != nil {
		return r.roll()
	}
	return r.src.Float64()
}
