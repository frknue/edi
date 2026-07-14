// Package db owns all persistence: opening the SQLite connection, running
// embedded migrations, and the Store type that exposes typed CRUD methods.
// Higher layers (services) never write SQL directly.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"edi/migrations"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)
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

// timeLayout is the canonical on-disk timestamp format: RFC3339 with FIXED-WIDTH
// nanoseconds. Fixed width matters because timestamps are stored as TEXT and some
// queries compare them with `created_at >= ?`; variable-width fractional seconds
// (time.RFC3339Nano trims trailing zeros) would sort incorrectly at sub-second
// boundaries since '.' < 'Z' lexicographically.
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// Store wraps the database handle and provides domain persistence methods.
type Store struct {
	db *sql.DB
}

// Open connects to the SQLite database at path, applying sensible pragmas for a
// self-hosted single-user app, then runs migrations.
func Open(path string) (*Store, error) {
	dsn := buildDSN(path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Serialize access: simplest reliable way to avoid "database is locked" for
	// the WAL writer in a single-user app.
	sqlDB.SetMaxOpenConns(1)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: sqlDB}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func buildDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return "file:" + path + "?" + q.Encode()
}

// DB exposes the underlying handle (used by tests and graceful shutdown).
func (s *Store) DB() *sql.DB { return s.db }

// Close folds the WAL back into the main database file (best-effort) and closes
// the connection, so the on-disk file is tidy after a graceful shutdown.
func (s *Store) Close() error {
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		// Non-fatal: the WAL is still durable and will be folded in on next open.
		_ = err
	}
	return s.db.Close()
}

// migrate applies any embedded migration files not yet recorded in
// schema_migrations, in lexical filename order, each inside a transaction.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
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
		if err := s.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, name).Scan(&exists); err != nil {
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
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, name, nowString()); err != nil {
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

func nowString() string { return time.Now().UTC().Format(timeLayout) }

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func formatTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(timeLayout)
}

// parseTime parses a stored timestamp; tolerant of a few layouts.
func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{timeLayout, time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time %q", s)
}

func mustParseTime(s string) time.Time {
	t, _ := parseTime(s)
	return t
}

func parseTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil
	}
	return &t
}
