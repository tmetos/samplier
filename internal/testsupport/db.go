// Package testsupport provides shared test helpers, including a PostGIS
// connection that skips integration tests when no database is available.
package testsupport

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DatabaseURL returns the test database URL from the environment.
func DatabaseURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://samplier:samplier@localhost:5432/samplier"
}

// Pool returns a connection pool, skipping the test when unavailable.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(DatabaseURL())
	if err != nil {
		t.Skipf("invalid database URL: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	// Match production behaviour: dates must not shift across timezones.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET TIME ZONE 'UTC'")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Cleanup removes rows created by a test, restricted to a test project code.
func Cleanup(t *testing.T, pool *pgxpool.Pool, projectCode string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `DELETE FROM project WHERE code = $1`, projectCode)
	if err != nil {
		t.Logf("cleanup failed for %s: %v", projectCode, err)
	}
}
