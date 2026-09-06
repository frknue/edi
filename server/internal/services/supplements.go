package services

import (
	"errors"
	"strings"
	"time"

	"edi/internal/db"
	"edi/internal/models"
)

// Supplements: a personal daily stack. Every supplement taken pays a small,
// auditable reward (xp_events source='supplement'); taking the whole active
// stack in one local day pays the bonus once. No un-take and no clawback —
// same rule as quests and the journal.
var (
	supplementItemRewards  = map[string]int64{"health": 3}
	supplementBonusRewards = map[string]int64{"health": 10, "discipline": 5}
)

const (
	maxSupplements        = 30
	maxSupplementNameLen  = 60
	maxSupplementDoseLen  = 40
	supplementHistoryDays = 70 // 10 weeks — matches the journal heatmap
)

func localDay(t time.Time) string { return t.Local().Format("2006-01-02") }

// ListSupplements returns today's stack with progress and the reward schedule.
func (s *Service) ListSupplements() (models.SupplementsToday, error) {
	day := localDay(time.Now())
	list, err := s.store.ListSupplements(s.userID, day)
	if err != nil {
		return models.SupplementsToday{}, err
	}
	paid, err := s.store.SupplementBonusPaid(s.userID, day)
	if err != nil {
		return models.SupplementsToday{}, err
	}
	taken := 0
	for _, sp := range list {
		if sp.Taken {
			taken++
		}
	}
	return models.SupplementsToday{
		Day:          day,
		Supplements:  orEmpty(list),
		Taken:        taken,
		Total:        len(list),
		AllTaken:     len(list) > 0 && taken == len(list),
		BonusAwarded: paid,
		ItemRewards:  supplementItemRewards,
		BonusRewards: supplementBonusRewards,
	}, nil
}

func cleanSupplementInput(in models.SupplementInput) (models.SupplementInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Dose = strings.TrimSpace(in.Dose)
	if in.Name == "" {
		return in, validationErr("name is required")
	}
	if len(in.Name) > maxSupplementNameLen {
		return in, validationErr("name must be at most %d characters", maxSupplementNameLen)
	}
	if len(in.Dose) > maxSupplementDoseLen {
		return in, validationErr("dose must be at most %d characters", maxSupplementDoseLen)
	}
	return in, nil
}

// AddSupplement appends a supplement to the stack.
func (s *Service) AddSupplement(in models.SupplementInput) (models.Supplement, error) {
	in, err := cleanSupplementInput(in)
	if err != nil {
		return models.Supplement{}, err
	}
	existing, err := s.store.ListSupplements(s.userID, localDay(time.Now()))
	if err != nil {
		return models.Supplement{}, err
	}
	if len(existing) >= maxSupplements {
		return models.Supplement{}, validationErr("the stack is full (%d supplements)", maxSupplements)
	}
	for _, sp := range existing {
		if strings.EqualFold(sp.Name, in.Name) {
			return models.Supplement{}, validationErr("%q is already in your stack", sp.Name)
		}
	}
	sp, err := s.store.InsertSupplement(s.userID, in)
	if errors.Is(err, db.ErrSupplementExists) {
		return models.Supplement{}, validationErr("%q is already in your stack", in.Name)
	}
	return sp, err
}

// UpdateSupplement renames or re-doses a supplement.
func (s *Service) UpdateSupplement(id int64, in models.SupplementInput) (models.Supplement, error) {
	in, err := cleanSupplementInput(in)
	if err != nil {
		return models.Supplement{}, err
	}
	sp, err := s.store.UpdateSupplement(s.userID, id, in)
	switch {
	case errors.Is(err, db.ErrNotFound):
		return models.Supplement{}, ErrNotFound
	case errors.Is(err, db.ErrSupplementExists):
		return models.Supplement{}, validationErr("%q is already in your stack", in.Name)
	}
	return sp, err
}

// ArchiveSupplement removes a supplement from the stack (history is kept).
func (s *Service) ArchiveSupplement(id int64) error {
	err := s.store.ArchiveSupplement(s.userID, id)
	if errors.Is(err, db.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// FindSupplement resolves a supplement in the active stack by (case-insensitive)
// name, exact match first, then unique prefix — for chat ("took my magnesium").
func (s *Service) FindSupplement(name string) (models.Supplement, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.Supplement{}, validationErr("supplement name is required")
	}
	list, err := s.store.ListSupplements(s.userID, localDay(time.Now()))
	if err != nil {
		return models.Supplement{}, err
	}
	var prefix []models.Supplement
	for _, sp := range list {
		if strings.EqualFold(sp.Name, name) {
			return sp, nil
		}
		if strings.HasPrefix(strings.ToLower(sp.Name), strings.ToLower(name)) {
			prefix = append(prefix, sp)
		}
	}
	if len(prefix) == 1 {
		return prefix[0], nil
	}
	if len(prefix) > 1 {
		return models.Supplement{}, validationErr("%q is ambiguous — be more specific", name)
	}
	return models.Supplement{}, ErrNotFound
}

// TakeSupplement records today's intake and awards XP (plus the bonus when the
// take completes the stack). A second take on the same local day is rejected.
func (s *Service) TakeSupplement(id int64) (models.SupplementTakeResult, error) {
	if _, err := s.ApplyDecay(); err != nil {
		return models.SupplementTakeResult{}, err
	}
	intake, bonus, events, levelUps, gold, err := s.store.TakeSupplement(s.userID, id, supplementItemRewards, supplementBonusRewards)
	switch {
	case errors.Is(err, db.ErrNotFound):
		return models.SupplementTakeResult{}, ErrNotFound
	case errors.Is(err, db.ErrSupplementTaken):
		return models.SupplementTakeResult{}, validationErr("%s", err.Error())
	case err != nil:
		return models.SupplementTakeResult{}, err
	}
	today, err := s.ListSupplements()
	if err != nil {
		return models.SupplementTakeResult{}, err
	}
	dash, err := s.GetDashboard()
	if err != nil {
		return models.SupplementTakeResult{}, err
	}
	return models.SupplementTakeResult{
		Intake:       intake,
		BonusAwarded: bonus,
		XPEvents:     orEmpty(events),
		LevelUps:     orEmpty(levelUps),
		Gold:         gold,
		Today:        today,
		Dashboard:    dash,
	}, nil
}

// SupplementHistory returns per-day summaries for the last `days` local days
// (default 70), most recent first.
func (s *Service) SupplementHistory(days int) ([]models.SupplementDay, error) {
	if days <= 0 || days > 365 {
		days = supplementHistoryDays
	}
	since := localDay(time.Now().AddDate(0, 0, -(days - 1)))
	out, err := s.store.SupplementHistory(s.userID, since)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Names = orEmpty(out[i].Names)
	}
	return orEmpty(out), nil
}
