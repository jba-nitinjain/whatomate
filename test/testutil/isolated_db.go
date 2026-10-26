package testutil

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SetupIsolatedTestDB returns a database handle bound to a private, freshly
// migrated Postgres schema that only the calling test can see. The schema is
// dropped when the test finishes.
//
// Use it for tests that exercise intentionally global behaviour (super-admin
// listings, cross-organization scans such as chat repair) where the shared
// test database would otherwise leak rows from sibling tests into the result,
// or where the code under test would mutate other tests' fixtures.
//
// Requires TEST_DATABASE_URL; the test is skipped when it is not set.
func SetupIsolatedTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database test")
	}

	admin := SetupTestDB(t)
	schema := "iso_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)).Error; err != nil {
		t.Fatalf("failed to create isolated schema: %v", err)
	}

	isoDSN, err := withSearchPath(dsn, schema)
	if err != nil {
		t.Fatalf("failed to build isolated DSN: %v", err)
	}
	db, err := gorm.Open(postgres.Open(isoDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open isolated test database: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		admin.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
	})

	if err := runMigrations(db); err != nil {
		t.Fatalf("failed to migrate isolated schema: %v", err)
	}
	return db
}

// withSearchPath pins every pooled connection of dsn to the given schema.
// Both URL ("postgres://...") and key/value ("host=... dbname=...") DSNs are supported.
func withSearchPath(dsn, schema string) (string, error) {
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	return fmt.Sprintf("%s search_path=%s", dsn, schema), nil
}
