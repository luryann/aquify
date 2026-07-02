package training

import (
	"errors"
	"testing"
)

func TestPracticeCRUDValidationAndSorting(t *testing.T) {
	t.Parallel()

	svc := NewService()
	if _, err := svc.Create("org-a", "2026-07-02", "group-a", "invalid", "Bad", 0, nil, PracticeStrokes{}, "", "coach"); !errors.Is(err, ErrInvalidType) {
		t.Fatalf("Create() invalid type error = %v, want %v", err, ErrInvalidType)
	}

	first, err := svc.Create("org-a", "2026-07-01", "group-a", "water", "Main set", 3200, nil, PracticeStrokes{Free: 1200}, "Bring fins", "coach")
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := svc.Create("org-a", "2026-07-03", "group-a", "dryland", "Strength", 0, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	items, err := svc.List("org-a")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("List() = %+v, want newest practice first", items)
	}

	nextDescription := "Threshold set"
	nextType := "video"
	updated, err := svc.Update("org-a", first.ID, PracticeUpdates{Description: &nextDescription, Type: &nextType}, "coach-2")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Description != nextDescription || updated.Type != nextType || updated.UpdatedBy != "coach-2" {
		t.Fatalf("Update() = %+v, want patched fields", updated)
	}

	if err := svc.Delete("org-a", first.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Get("org-a", first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v, want %v", err, ErrNotFound)
	}
}
