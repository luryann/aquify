package documents

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// A write issued with the wrong org must not touch another org's row, even when
// the caller somehow holds a valid row ID (tenancy tracker item 4.1).
func TestDBSoftDeleteDocumentTypeIgnoresOtherOrg(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	orgA, orgB, user := uuid.NewString(), uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool)

	dt, err := svc.CreateDocumentType(orgA, "Waiver", "waiver", "", "household", true, true, nil, user)
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.repo.SoftDeleteDocumentType(context.Background(), orgB, dt.ID, user)

	var deleted bool
	if err := pool.QueryRow(context.Background(), `SELECT deleted_at IS NOT NULL FROM document_types WHERE id = $1::uuid`, dt.ID).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("org B's request soft-deleted org A's document type")
	}
	if err := svc.repo.SoftDeleteDocumentType(context.Background(), orgA, dt.ID, user); err != nil {
		t.Fatalf("owning org must still be able to delete: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT deleted_at IS NOT NULL FROM document_types WHERE id = $1::uuid`, dt.ID).Scan(&deleted); err != nil || !deleted {
		t.Fatalf("owning org delete did not apply (deleted=%v err=%v)", deleted, err)
	}
}
