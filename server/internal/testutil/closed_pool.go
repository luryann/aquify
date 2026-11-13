package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ClosedPool returns a pool that is already closed, so every query through it fails the way an
// unreachable database does. pgxpool connects lazily, so no database or schema is needed (and
// nothing is contacted); use it instead of a migrated IsolatedSchemaPool whenever a test only
// needs a service whose every database call fails.
func ClosedPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://closed:closed@127.0.0.1:1/closed?sslmode=disable")
	if err != nil {
		t.Fatalf("build closed pool: %v", err)
	}
	pool.Close()
	return pool
}
