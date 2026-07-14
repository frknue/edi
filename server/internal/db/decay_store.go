package db

import (
	"fmt"
	"strings"
	"time"
)

// dayFormat ("2006-01-02") is shared with the streak code in store.go.

// localDate truncates a time to its local calendar date.
func localDate(t time.Time) time.Time {
	l := t.Local()
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
}

// localDaysBetween counts whole local calendar days from a to b (DST-safe).
func localDaysBetween(a, b time.Time) int {
	return int(localDate(b).Sub(localDate(a)).Hours()/24 + 0.5)
}

// ApplyDecay is the lazy decay catch-up: for every attribute it bills each
// idle day beyond the grace period as ONE negative xp_events row
// (source='decay', note 'decay · YYYY-MM-DD') plus the matching total_xp
// decrement — all in one transaction, idempotent per attribute per local day
// (the billed dates encoded in the notes are re-read inside the tx; the
// single-writer connection serializes racing callers). peak_xp is never
// touched. Days covered by a ward window are excluded; restEndedAt (nullable)
// resets the idle anchor; the caller skips the call entirely while rest mode
// is on. Returns the total XP removed.
func (s *Store) ApplyDecay(userID int64, restEndedAt *time.Time, now time.Time) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	nowStr := formatTime(now)
	today := localDate(now)

	type attrRow struct {
		key   string
		total int64
		peak  int64
	}
	rows, err := tx.Query(`SELECT key, total_xp, peak_xp FROM attributes WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	var attrs []attrRow
	for rows.Next() {
		var a attrRow
		if err := rows.Scan(&a.key, &a.total, &a.peak); err != nil {
			rows.Close()
			return 0, err
		}
		attrs = append(attrs, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var totalRemoved int64
	for _, a := range attrs {
		// Idle anchor: last positive activity, or rest end, whichever is later.
		var lastAct string
		err := tx.QueryRow(
			`SELECT MAX(created_at) FROM xp_events WHERE user_id = ? AND attribute_key = ? AND amount > 0`,
			userID, a.key).Scan(&lastAct)
		if err != nil || lastAct == "" {
			continue // never trained: nothing to decay from
		}
		anchor := mustParseTime(lastAct)
		if restEndedAt != nil && restEndedAt.After(anchor) {
			anchor = *restEndedAt
		}

		idleDays := localDaysBetween(anchor, now)
		if idleDays <= decayGraceDays {
			continue
		}

		// Dates already billed (from decay-event notes), inside the tx.
		billed := map[string]bool{}
		brows, err := tx.Query(
			`SELECT note FROM xp_events WHERE user_id = ? AND attribute_key = ? AND source = 'decay'`,
			userID, a.key)
		if err != nil {
			return 0, err
		}
		for brows.Next() {
			var note string
			if err := brows.Scan(&note); err != nil {
				brows.Close()
				return 0, err
			}
			if d, ok := strings.CutPrefix(note, "decay · "); ok {
				billed[d] = true
			}
		}
		brows.Close()
		if err := brows.Err(); err != nil {
			return 0, err
		}

		// Ward windows (lapsed ones still exclude the days they covered).
		type window struct{ from, to time.Time }
		var wards []window
		wrows, err := tx.Query(
			`SELECT created_at, expires_at FROM wards WHERE user_id = ? AND attribute_key = ?`, userID, a.key)
		if err != nil {
			return 0, err
		}
		for wrows.Next() {
			var created, expires string
			if err := wrows.Scan(&created, &expires); err != nil {
				wrows.Close()
				return 0, err
			}
			wards = append(wards, window{localDate(mustParseTime(created)), localDate(mustParseTime(expires))})
		}
		wrows.Close()
		if err := wrows.Err(); err != nil {
			return 0, err
		}
		covered := func(day time.Time) bool {
			for _, w := range wards {
				if !day.Before(w.from) && !day.After(w.to) {
					return true
				}
			}
			return false
		}

		floor := decayFloorXP(a.peak)
		running := a.total
		anchorDay := localDate(anchor)
		for d := decayGraceDays + 1; d <= idleDays; d++ {
			day := anchorDay.AddDate(0, 0, d)
			if day.After(today) {
				break
			}
			dayStr := day.Format(dayFormat)
			if billed[dayStr] || covered(day) {
				continue
			}
			if running <= floor {
				break
			}
			amt := decayDailyAmount(running)
			if running-amt < floor {
				amt = running - floor // partial final bleed down to the floor
			}
			if amt <= 0 {
				break
			}
			if _, err := tx.Exec(
				`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES(?, ?, ?, 'decay', NULL, ?, ?)`,
				userID, a.key, -amt, fmt.Sprintf("decay · %s", dayStr), nowStr); err != nil {
				return 0, err
			}
			if _, err := tx.Exec(
				`UPDATE attributes SET total_xp = total_xp - ? WHERE user_id = ? AND key = ?`,
				amt, userID, a.key); err != nil {
				return 0, err
			}
			running -= amt
			totalRemoved += amt
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return totalRemoved, nil
}
