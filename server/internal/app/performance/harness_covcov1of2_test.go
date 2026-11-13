package performance

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// cov1xClosedRepo returns a repository whose pool is already closed, so every
// database call fails immediately (a real driver error, not a mock).
func cov1xClosedRepo(t *testing.T) *Repository {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://127.0.0.1:1/none?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	return newRepository(pool)
}

// cov1xUnloaded returns a database-mode service whose database is unreachable and
// whose club has never been loaded, so every ensureOrg fails.
func cov1xUnloaded(t *testing.T) *Service {
	t.Helper()
	s := NewService()
	s.repo = cov1xClosedRepo(t)
	return s
}

// cov1xBreakPersistence marks org as freshly loaded (so reads and ensureOrg stay on
// the hot path) and then makes every repository write fail.
func cov1xBreakPersistence(t *testing.T, s *Service, org string) {
	t.Helper()
	s.SetRefreshInterval(time.Hour)
	s.touchLoaded(org)
	s.markTouched(org)
	s.repo = cov1xClosedRepo(t)
}
