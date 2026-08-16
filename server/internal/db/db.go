// Package db owns all persistence: opening the PostgreSQL pool, running
// embedded migrations, and the Store type that exposes typed CRUD methods.
// Higher layers (services) never write SQL directly.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"edi/migrations"

	_ "github.com/jackc/pgx/v5/stdlib" // pure-Go PostgreSQL driver (no CGO)
)

// Sentinel errors the service layer translates into HTTP status codes.
var (
	// ErrNotFound — the requested row does not exist.
	ErrNotFound = errors.New("not found")
	// ErrQuestNotCompletable — the quest exists but is already completed/archived.
	ErrQuestNotCompletable = errors.New("quest is already completed or archived")
	// ErrSuggestionNotPending — the suggestion was already accepted/dismissed.
	ErrSuggestionNotPending = errors.New("suggestion already resolved")
	// ErrInsufficientGold — the balance cannot cover the purchase.
	ErrInsufficientGold = errors.New("not enough gold")
)

// Store wraps the database handle and provides domain persistence methods.
type Store struct {
	db   *sql.DB
	dice *rng // game-layer randomness (crits, loot); see game_math.go
}

// Open connects to the PostgreSQL database at url (a postgres:// DSN), then
// runs any pending embedded migrations.
func Open(url string) (*Store, error) {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	// Modest pool for a small self-hosted app. Correctness does NOT depend on
	// pool size: every read-then-write transaction serializes per user via
	// beginUserTx's advisory lock.
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	s := &Store{db: sqlDB, dice: newRNG()}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// DB exposes the underlying handle (used by tests and graceful shutdown).
func (s *Store) DB() *sql.DB { return s.db }

// SetRollForTest fixes the store's dice to a deterministic roll function —
// a TEST hook (cross-package, so it must be exported); never call it from
// production code paths.
func (s *Store) SetRollForTest(roll func() float64) { s.dice.roll = roll }

// Close closes the connection pool.
func (s *Store) Close() error { return s.db.Close() }

// beginUserTx opens a transaction whose FIRST statement takes the per-user
// advisory lock. This serializes all of a user's writing transactions —
// exactly the guarantee the SQLite single-writer connection used to provide —
// while different users proceed in parallel. Every read-then-write tx
// (completion gates, gold balance checks, decay idempotency, first-entry-of-
// the-day checks) MUST start here, before its first read.
func (s *Store) beginUserTx(userID int64) (*sql.Tx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// migrate applies any embedded migration files not yet recorded in
// schema_migrations, in lexical filename order, each inside a transaction.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at timestamptz NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		if err := s.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = $1`, name).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists > 0 {
			continue
		}
		sqlBytes, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES ($1, $2)`, name, time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

// --- time helpers -----------------------------------------------------------
//
// Timestamps are timestamptz; time.Time values pass in and out of the driver
// directly. The only remaining helpers deal with nullable columns and the
// local-day bounds used by streak/daily-XP/decay math (computed in Go so SQL
// never needs a zone name).

func timePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	t := nt.Time
	return &t
}

func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

// localDayBounds returns [start, end) of the local calendar day containing t.
func localDayBounds(t time.Time) (time.Time, time.Time) {
	l := t.Local()
	start := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
	return start, start.AddDate(0, 0, 1)
}
