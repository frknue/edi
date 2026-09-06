package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"edi/internal/models"
)

// isUniqueViolation reports a Postgres unique_violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ListSupplements returns the active stack with the intake state for the given
// local day (YYYY-MM-DD), in display order.
func (s *Store) ListSupplements(userID int64, day string) ([]models.Supplement, error) {
	rows, err := s.db.Query(
		`SELECT sp.id, sp.name, sp.dose, sp.sort_order, sp.created_at, i.created_at
		 FROM supplements sp
		 LEFT JOIN supplement_intakes i
		   ON i.supplement_id = sp.id AND i.user_id = sp.user_id AND i.taken_on = $2
		 WHERE sp.user_id = $1 AND sp.archived_at IS NULL
		 ORDER BY sp.sort_order, sp.id`, userID, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Supplement
	for rows.Next() {
		var sp models.Supplement
		var takenAt sql.NullTime
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.Dose, &sp.SortOrder, &sp.CreatedAt, &takenAt); err != nil {
			return nil, err
		}
		if takenAt.Valid {
			t := takenAt.Time.UTC()
			sp.Taken, sp.TakenAt = true, &t
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// SupplementBonusPaid reports whether the all-taken bonus was already awarded
// on the given local day.
func (s *Store) SupplementBonusPaid(userID int64, day string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM supplement_intakes WHERE user_id = $1 AND taken_on = $2 AND bonus`, userID, day).Scan(&n)
	return n > 0, err
}

// InsertSupplement appends a supplement to the end of the stack.
func (s *Store) InsertSupplement(userID int64, in models.SupplementInput) (models.Supplement, error) {
	now := time.Now().UTC()
	var sp models.Supplement
	err := s.db.QueryRow(
		`INSERT INTO supplements(user_id, name, dose, sort_order, created_at)
		 VALUES($1, $2, $3, COALESCE((SELECT MAX(sort_order) + 1 FROM supplements WHERE user_id = $1), 0), $4)
		 RETURNING id, name, dose, sort_order, created_at`,
		userID, in.Name, in.Dose, now).Scan(&sp.ID, &sp.Name, &sp.Dose, &sp.SortOrder, &sp.CreatedAt)
	if isUniqueViolation(err) {
		return models.Supplement{}, ErrSupplementExists
	}
	return sp, err
}

// UpdateSupplement renames/re-doses an active supplement.
func (s *Store) UpdateSupplement(userID, id int64, in models.SupplementInput) (models.Supplement, error) {
	var sp models.Supplement
	err := s.db.QueryRow(
		`UPDATE supplements SET name = $3, dose = $4
		 WHERE id = $1 AND user_id = $2 AND archived_at IS NULL
		 RETURNING id, name, dose, sort_order, created_at`,
		id, userID, in.Name, in.Dose).Scan(&sp.ID, &sp.Name, &sp.Dose, &sp.SortOrder, &sp.CreatedAt)
	if err == sql.ErrNoRows {
		return models.Supplement{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return models.Supplement{}, ErrSupplementExists
	}
	return sp, err
}

// ArchiveSupplement soft-deletes a supplement; intakes (and their xp_events)
// stay for the audit trail and history.
func (s *Store) ArchiveSupplement(userID, id int64) error {
	res, err := s.db.Exec(
		`UPDATE supplements SET archived_at = $3 WHERE id = $1 AND user_id = $2 AND archived_at IS NULL`,
		id, userID, time.Now().UTC())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TakeSupplement records today's intake of one supplement and awards XP
// atomically: the per-item reward always, plus the bonus once per local day
// when this take completes the active stack. Everything — the already-taken
// check, the all-taken count, the xp_events, attribute bumps, gold and streak —
// happens inside one beginUserTx so concurrent takes cannot double-award.
func (s *Store) TakeSupplement(userID, id int64, itemRewards, bonusRewards map[string]int64) (models.SupplementIntake, bool, []models.XPEvent, []models.LevelUp, int64, error) {
	names, err := s.AttributeNames(userID)
	if err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}

	tx, err := s.beginUserTx(userID)
	if err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	day := now.Local().Format(dayFormat)

	var name string
	err = tx.QueryRow(`SELECT name FROM supplements WHERE id = $1 AND user_id = $2 AND archived_at IS NULL`, id, userID).Scan(&name)
	if err == sql.ErrNoRows {
		return models.SupplementIntake{}, false, nil, nil, 0, ErrNotFound
	}
	if err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}

	var already int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM supplement_intakes WHERE user_id = $1 AND supplement_id = $2 AND taken_on = $3`,
		userID, id, day).Scan(&already); err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}
	if already > 0 {
		return models.SupplementIntake{}, false, nil, nil, 0, ErrSupplementTaken
	}

	var intakeID int64
	if err := tx.QueryRow(
		`INSERT INTO supplement_intakes(user_id, supplement_id, taken_on, created_at) VALUES($1, $2, $3, $4) RETURNING id`,
		userID, id, day, now).Scan(&intakeID); err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}

	var events []models.XPEvent
	var levelUps []models.LevelUp
	var goldTotal, xpTotal int64
	award := func(rewards map[string]int64, note string) error {
		for _, key := range orderedKeys(rewards) {
			amount := rewards[key]
			if amount == 0 {
				continue
			}
			var oldXP int64
			if err := tx.QueryRow(`SELECT total_xp FROM attributes WHERE user_id = $1 AND key = $2`, userID, key).Scan(&oldXP); err != nil {
				continue // unknown attribute key — skip
			}
			var evID int64
			if err := tx.QueryRow(
				`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES($1, $2, $3, 'supplement', $4, $5, $6) RETURNING id`,
				userID, key, amount, intakeID, note, now).Scan(&evID); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + $1, peak_xp = GREATEST(peak_xp, total_xp + $1) WHERE user_id = $2 AND key = $3`, amount, userID, key); err != nil {
				return err
			}
			if g := goldForXP(amount); g > 0 {
				if _, err := insertGoldEventTx(tx, userID, g, "supplement", note, nil, now); err != nil {
					return err
				}
				goldTotal += g
			}
			xpTotal += amount
			sid := intakeID
			events = append(events, models.XPEvent{
				ID: evID, AttributeKey: key, AttributeName: names[key], Amount: amount,
				Source: "supplement", SourceID: &sid, Note: note, CreatedAt: now,
			})
			if from, to := levelFromTo(oldXP, oldXP+amount); to > from {
				levelUps = append(levelUps, models.LevelUp{
					AttributeKey: key, AttributeName: names[key], FromLevel: from, ToLevel: to,
				})
			}
		}
		return nil
	}

	if err := award(itemRewards, "Supplement · "+name); err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}

	// Bonus: this take completed the whole active stack and no bonus was paid
	// today (the list may have grown since an earlier bonus — pay once).
	bonus := false
	var active, takenToday, paid int
	if err := tx.QueryRow(
		`SELECT
		   (SELECT COUNT(1) FROM supplements WHERE user_id = $1 AND archived_at IS NULL),
		   (SELECT COUNT(1) FROM supplement_intakes i JOIN supplements sp ON sp.id = i.supplement_id
		      WHERE i.user_id = $1 AND i.taken_on = $2 AND sp.archived_at IS NULL),
		   (SELECT COUNT(1) FROM supplement_intakes WHERE user_id = $1 AND taken_on = $2 AND bonus)`,
		userID, day).Scan(&active, &takenToday, &paid); err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}
	if active > 0 && takenToday >= active && paid == 0 {
		bonus = true
		if err := award(bonusRewards, "Supplements · full stack"); err != nil {
			return models.SupplementIntake{}, false, nil, nil, 0, err
		}
	}

	if _, err := tx.Exec(`UPDATE supplement_intakes SET xp_awarded = $2, bonus = $3 WHERE id = $1`, intakeID, xpTotal, bonus); err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}
	if err := updateStreakTx(tx, userID, now); err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return models.SupplementIntake{}, false, nil, nil, 0, err
	}

	return models.SupplementIntake{
		ID: intakeID, SupplementID: id, Name: name, Day: day, XPAwarded: xpTotal, Bonus: bonus, CreatedAt: now,
	}, bonus, events, levelUps, goldTotal, nil
}

// SupplementHistory returns per-day intake summaries for local days >= since
// (YYYY-MM-DD), most recent first. Archived supplements still count for the
// days they were taken.
func (s *Store) SupplementHistory(userID int64, since string) ([]models.SupplementDay, error) {
	rows, err := s.db.Query(
		`SELECT i.taken_on, sp.name, i.xp_awarded, i.bonus
		 FROM supplement_intakes i JOIN supplements sp ON sp.id = i.supplement_id
		 WHERE i.user_id = $1 AND i.taken_on >= $2
		 ORDER BY i.taken_on DESC, i.id`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SupplementDay
	for rows.Next() {
		var day, name string
		var xp int64
		var bonus bool
		if err := rows.Scan(&day, &name, &xp, &bonus); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].Day != day {
			out = append(out, models.SupplementDay{Day: day})
		}
		d := &out[len(out)-1]
		d.Taken++
		d.XP += xp
		d.Bonus = d.Bonus || bonus
		d.Names = append(d.Names, name)
	}
	return out, rows.Err()
}
