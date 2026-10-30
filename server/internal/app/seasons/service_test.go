package seasons

import "testing"

func TestCreateValidatesStatus(t *testing.T) {
	svc := NewService()
	if _, err := svc.Create("org-1", "2026 Season", "", "", "paused", "", "admin-1"); err != ErrInvalidStatus {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

func TestCreateReturnsCopyNotCachedPointer(t *testing.T) {
	svc := NewService()
	created, err := svc.Create("org-1", "2026 Season", "", "", "active", "", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	created.Name = "mutated by caller"
	got, err := svc.Get("org-1", created.ID)
	if err != nil || got.Name != "2026 Season" {
		t.Fatalf("cache aliased by Create result: %+v (%v)", got, err)
	}
}

func TestListIsOrgScopedAndSorted(t *testing.T) {
	svc := NewService()
	if _, err := svc.Create("org-1", "Older", "2024-09-01", "2025-06-30", "completed", "", "admin-1"); err != nil {
		t.Fatalf("create older: %v", err)
	}
	if _, err := svc.Create("org-1", "Newer", "2025-09-01", "2026-06-30", "active", "", "admin-1"); err != nil {
		t.Fatalf("create newer: %v", err)
	}
	if _, err := svc.Create("org-2", "Foreign", "2025-09-01", "2026-06-30", "active", "", "admin-2"); err != nil {
		t.Fatalf("create foreign: %v", err)
	}

	list, err := svc.List("org-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Name != "Newer" || list[1].Name != "Older" {
		t.Fatalf("unexpected list order: %+v", list)
	}
}

func TestUpdateAndDeleteAreOrgScoped(t *testing.T) {
	svc := NewService()
	season, err := svc.Create("org-1", "2026 Season", "", "", "upcoming", "", "admin-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	goals := "New goals"
	if _, err := svc.Update("org-2", season.ID, SeasonUpdates{Goals: &goals}, "admin-2"); err != ErrNotFound {
		t.Fatalf("foreign update: expected ErrNotFound, got %v", err)
	}
	updated, err := svc.Update("org-1", season.ID, SeasonUpdates{Goals: &goals}, "admin-1")
	if err != nil || updated.Goals != goals {
		t.Fatalf("update: err=%v season=%+v", err, updated)
	}

	if err := svc.Delete("org-2", season.ID); err != ErrNotFound {
		t.Fatalf("foreign delete: expected ErrNotFound, got %v", err)
	}
	if err := svc.Delete("org-1", season.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get("org-1", season.ID); err != ErrNotFound {
		t.Fatalf("get deleted: expected ErrNotFound, got %v", err)
	}
}
