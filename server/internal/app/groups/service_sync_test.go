package groups

import (
	"context"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A failed Postgres re-sync must surface to the caller; ListRegistrations used
// to swallow it and answer with an empty list as though nobody had registered.
func TestListRegistrationsReturnsSyncError(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody:nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	svc := NewService()
	svc.repo = newRepository(pool)

	admin := &appmiddleware.Principal{UserID: "u1", Roles: []string{"admin"}}
	regs, err := svc.ListRegistrations(admin, "org-1")
	if err == nil {
		t.Fatalf("expected sync error, got %d registrations", len(regs))
	}
}
