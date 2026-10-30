package documents

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDocumentTypeUpdateDeleteAndDuplicateNames(t *testing.T) {
	t.Parallel()
	svc := NewService()
	waiver, err := svc.CreateDocumentType("org-a", "Liability Waiver", "waiver", "", "household", true, true, nil, "admin-1")
	if err != nil {
		t.Fatalf("create waiver: %v", err)
	}
	if _, err := svc.CreateDocumentType("org-a", "Medical Form", "medical", "", "swimmer", true, true, nil, "admin-1"); err != nil {
		t.Fatalf("create medical: %v", err)
	}

	if _, err := svc.UpdateDocumentType("org-a", waiver.ID, map[string]interface{}{"name": "medical form"}, "admin-1"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("rename onto existing name: want ErrDuplicateName, got %v", err)
	}
	months := 12
	updated, err := svc.UpdateDocumentType("org-a", waiver.ID, map[string]interface{}{
		"name": "Season Waiver", "category": "release", "description": "  signed yearly  ",
		"is_required": false, "applies_to": "all", "expiry_months": &months, "is_active": false,
	}, "admin-2")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Season Waiver" || updated.Category != "release" || updated.Description != "signed yearly" ||
		updated.IsRequired || updated.AppliesTo != "all" || updated.ExpiryMonths == nil || *updated.ExpiryMonths != 12 || updated.IsActive {
		t.Fatalf("update not applied: %+v", updated)
	}
	if _, err := svc.UpdateDocumentType("org-b", waiver.ID, map[string]interface{}{"name": "x"}, "x"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("cross-org update: want ErrTypeNotFound, got %v", err)
	}

	active, _ := svc.ListDocumentTypes("org-a", true)
	all, _ := svc.ListDocumentTypes("org-a", false)
	if len(active) != 1 || len(all) != 2 {
		t.Fatalf("activeOnly filter: active=%d all=%d", len(active), len(all))
	}

	if err := svc.DeleteDocumentType("org-b", waiver.ID, "x"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("cross-org delete: want ErrTypeNotFound, got %v", err)
	}
	if err := svc.DeleteDocumentType("org-a", waiver.ID, "admin-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if all, _ := svc.ListDocumentTypes("org-a", false); len(all) != 1 {
		t.Fatalf("deleted type still listed: %d", len(all))
	}
}

func TestEraseHouseholdDataRemovesOnlyThatHousehold(t *testing.T) {
	t.Parallel()
	svc := NewService()
	dt, _ := svc.CreateDocumentType("org-a", "Waiver", "waiver", "", "household", true, true, nil, "admin-1")
	keep, _ := svc.CreateSubmission("org-a", dt.ID, "hh-keep", "", "parent-1")
	gone, _ := svc.CreateSubmission("org-a", dt.ID, "hh-gone", "", "parent-2")
	if _, err := svc.AttachFile("org-a", gone.ID, "docs/hh-gone/waiver.pdf", "waiver.pdf", "application/pdf", 10, "parent-2"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	keys, err := svc.EraseHouseholdData(context.Background(), "org-a", "hh-gone")
	if err != nil || len(keys) != 1 || keys[0] != "docs/hh-gone/waiver.pdf" {
		t.Fatalf("erase: keys=%v err=%v", keys, err)
	}
	if _, err := svc.GetSubmission("org-a", gone.ID); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("erased submission still present: %v", err)
	}
	if _, err := svc.GetSubmission("org-a", keep.ID); err != nil {
		t.Fatalf("other household's submission must survive: %v", err)
	}
}

// When the database delete fails, the cached submissions (and the object keys
// they hold) must survive so a retry can still purge the files from storage.
func TestEraseHouseholdDataKeepsCacheWhenDatabaseDeleteFails(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.submissions["s1"] = &DocumentSubmission{ID: "s1", OrgID: "org-a", HouseholdID: "hh-1", FileObjectKey: "docs/hh-1/a.pdf"}

	// Port 1 refuses connections immediately, so the DELETE fails.
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc.repo = newRepository(pool)

	if _, err := svc.EraseHouseholdData(context.Background(), "org-a", "hh-1"); err == nil {
		t.Fatal("expected an error when the database delete fails")
	}
	if _, err := svc.GetSubmission("org-a", "s1"); err != nil {
		t.Fatalf("submission dropped from cache despite failed database delete: %v", err)
	}
}
