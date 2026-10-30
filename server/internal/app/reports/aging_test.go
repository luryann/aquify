package reports

import (
	"errors"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestAgingNeverBucketsVoidedInvoiceAsOverdue(t *testing.T) {
	b := billing.NewService()
	admin := &appmiddleware.Principal{UserID: "u", Roles: []string{"admin"}, OrgID: "org-1"}
	inv, err := b.CreateInvoice("org-1", "u", admin, billing.CreateInvoiceInput{HouseholdID: "hh", InvoiceNumber: "INV-1", DueDate: "2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddInvoiceItem("org-1", "u", admin, inv.ID, billing.AddInvoiceItemInput{Description: "d", Quantity: 1, UnitAmountCents: 5000, Category: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.IssueInvoice("org-1", "u", admin, inv.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.VoidInvoice("org-1", "u", admin, inv.ID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, nil, nil, b)
	rep, err := svc.RunInvoiceAgingReport("org-1", admin, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("rows = %d", len(rep.Rows))
	}
	if got := rep.Rows[0].AgingBucket; got != "voided" {
		t.Fatalf("voided invoice bucket = %q, want voided", got)
	}
	if rep.Rows[0].IsOutstanding || rep.OutstandingInvoiceCount != 0 {
		t.Fatal("voided invoice must not count as outstanding")
	}
}

type failDeleteStore struct {
	storage.ObjectStorage
	fail bool
}

func (f *failDeleteStore) Delete(key string) error {
	if f.fail {
		return errors.New("s3 down")
	}
	return f.ObjectStorage.Delete(key)
}

func TestExpireArtifactsKeepsArtifactWhenDeleteFails(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	fs := &failDeleteStore{ObjectStorage: svc.store, fail: true}
	svc.store = fs
	doc := &CSVDocument{Filename: "aging.csv", Data: []byte("x")}
	if _, err := svc.StoreCSVArtifact("org", "aging", time.Unix(1, 1).UTC(), doc); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(365 * 24 * time.Hour)
	if got := svc.ExpireArtifacts(future); len(got) != 0 {
		t.Fatalf("expired %d despite failed delete", len(got))
	}
	fs.fail = false
	if got := svc.ExpireArtifacts(future); len(got) != 1 {
		t.Fatalf("retry expired %d, want 1", len(got))
	}
}
