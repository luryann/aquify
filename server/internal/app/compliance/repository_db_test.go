package compliance

import (
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/google/uuid"
)

// CompleteDataExport replaces the request's expiry with the download link's
// expiry; that must survive a reload from Postgres.
func TestCompleteDataExportExpiryPersists(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool, 30, 30)

	req, err := svc.CreateDataExportRequest(org, hh, "parent-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MarkDataExportProcessing(org, req.ID); err != nil {
		t.Fatal(err)
	}
	linkExpiry := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if _, err := svc.CompleteDataExport(org, req.ID, "https://example.test/x.zip", linkExpiry); err != nil {
		t.Fatal(err)
	}

	reloaded := NewServiceWithPool(pool, 30, 30)
	got, err := reloaded.GetDataExportRequest(org, req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(linkExpiry) {
		t.Fatalf("reloaded expires_at = %v, want %v", got.ExpiresAt, linkExpiry)
	}
}
