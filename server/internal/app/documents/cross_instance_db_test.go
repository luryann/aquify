package documents

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestDBCrossInstanceTypesAndSubmissions(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh, user := uuid.NewString(), uuid.NewString(), uuid.NewString()
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool)

	dt, err := a.CreateDocumentType(org, "Waiver", "waiver", "", "household", true, true, nil, user)
	if err != nil {
		t.Fatal(err)
	}
	if list, err := b.ListDocumentTypes(org, false); err != nil || len(list) != 1 {
		t.Fatalf("B types = %d err=%v", len(list), err)
	}
	sub, err := b.CreateSubmission(org, dt.ID, hh, "", user)
	if err != nil {
		t.Fatalf("B create submission: %v", err)
	}
	if got, err := a.GetSubmission(org, sub.ID); err != nil || got.DocumentTypeName != "Waiver" {
		t.Fatalf("A get = %+v err=%v", got, err)
	}
	if _, err := a.AttachFile(org, sub.ID, "k", "f.pdf", "application/pdf", 10, user); err != nil {
		t.Fatalf("A attach: %v", err)
	}
	if _, err := b.VerifySubmission(org, sub.ID, user, "ok"); err != nil {
		t.Fatalf("B verify: %v", err)
	}
	if got, err := a.GetSubmission(org, sub.ID); err != nil || got.Status != "verified" {
		t.Fatalf("A after verify = %+v err=%v", got, err)
	}
	// Cross-instance delete is observed (cache is replaced, not merged).
	if err := b.DeleteDocumentType(org, dt.ID, user); err != nil {
		t.Fatal(err)
	}
	if list, err := a.ListDocumentTypes(org, false); err != nil || len(list) != 0 {
		t.Fatalf("A types after delete = %d err=%v", len(list), err)
	}
	if _, err := a.CreateSubmission(org, dt.ID, hh, "", user); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("expected ErrTypeNotFound, got %v", err)
	}
}
