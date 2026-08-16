// Package dbtest gives tests isolated PostgreSQL stores: each call creates a
// throwaway schema in the test database, runs the real migrations into it, and
// drops it on cleanup — so tests exercise the exact production engine while
// staying independent (including across packages running in parallel).
package dbtest

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"

	"edi/internal/db"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DefaultURL is tried when EDI_TEST_DATABASE_URL is unset (local dev:
// `createdb edi_test` once).
const DefaultURL = "postgres://localhost:5432/edi_test?sslmode=disable"

// Open returns a Store backed by a fresh schema in the test database.
//
// Skip/fail semantics (deliberate): with EDI_TEST_DATABASE_URL explicitly set
// (CI), an unreachable database FAILS the test — CI can never silently skip.
// Unset (e.g. the Docker image build, which has no Postgres), an unreachable
// default skips the test loudly; the full suite runs locally and in CI.
func Open(t *testing.T) *db.Store {
	t.Helper()

	url, explicit := os.LookupEnv("EDI_TEST_DATABASE_URL")
	if !explicit {
		url = DefaultURL
	}

	admin, err := sql.Open("pgx", url)
	if err == nil {
		err = admin.Ping()
	}
	if err != nil {
		if explicit {
			t.Fatalf("EDI_TEST_DATABASE_URL is set but unreachable (%s): %v", url, err)
		}
		t.Skipf("postgres not reachable at %s — DB-backed test skipped (createdb edi_test to run it)", url)
	}

	schema := fmt.Sprintf("test_%08x", rand.Uint32()) //nolint:gosec — uniqueness, not security
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	store, err := db.Open(url + sep + "options=-csearch_path%3D" + schema)
	if err != nil {
		t.Fatalf("open store in schema %s: %v", schema, err)
	}

	t.Cleanup(func() {
		store.Close()
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Logf("drop test schema %s: %v", schema, err)
		}
		admin.Close()
	})
	return store
}
