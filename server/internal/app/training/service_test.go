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

func TestPracticeUpdateDeleteAndOrgScoping(t *testing.T) {
	svc := NewService()
	p, err := svc.Create("org-a", "2026-10-27", "SENIOR", "water", "Aerobic set", 3000, nil, PracticeStrokes{}, "", "coach-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	date, group, typ, desc, notes := "2026-10-28", "AGE", "dryland", "Core circuit", "bring mats"
	yards, attendance := 0, 14
	strokes := PracticeStrokes{}
	updated, err := svc.Update("org-a", p.ID, PracticeUpdates{
		Date: &date, GroupID: &group, Type: &typ, Description: &desc,
		YardageTarget: &yards, Attendance: &attendance, Strokes: &strokes, Notes: &notes,
	}, "coach-2")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Date != date || updated.GroupID != group || updated.Type != typ || updated.Notes != notes ||
		updated.Attendance == nil || *updated.Attendance != 14 || updated.UpdatedBy != "coach-2" {
		t.Fatalf("update not applied: %+v", updated)
	}

	bad := "swim"
	if _, err := svc.Update("org-a", p.ID, PracticeUpdates{Type: &bad}, "coach-2"); err != ErrInvalidType {
		t.Fatalf("invalid type: want ErrInvalidType, got %v", err)
	}
	if _, err := svc.Update("org-b", p.ID, PracticeUpdates{Notes: &notes}, "x"); err != ErrNotFound {
		t.Fatalf("cross-org update: want ErrNotFound, got %v", err)
	}
	if err := svc.Delete("org-b", p.ID); err != ErrNotFound {
		t.Fatalf("cross-org delete: want ErrNotFound, got %v", err)
	}
	if err := svc.Delete("org-a", p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get("org-a", p.ID); err != ErrNotFound {
		t.Fatalf("Get after delete: want ErrNotFound, got %v", err)
	}
}

func TestPracticeAttendanceUpsertsPerSwimmer(t *testing.T) {
	svc := NewService()
	p, err := svc.Create("org-a", "2026-10-27", "SENIOR", "water", "", 0, nil, PracticeStrokes{}, "", "coach-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.SetAttendance("org-a", p.ID, "sw-1", "late", "coach-1"); err != ErrInvalidAttendance {
		t.Fatalf("invalid status: want ErrInvalidAttendance, got %v", err)
	}
	if _, err := svc.SetAttendance("org-a", "missing", "sw-1", "present", "coach-1"); err != ErrNotFound {
		t.Fatalf("missing practice: want ErrNotFound, got %v", err)
	}
	first, err := svc.SetAttendance("org-a", p.ID, "sw-1", "present", "coach-1")
	if err != nil {
		t.Fatalf("SetAttendance: %v", err)
	}
	second, err := svc.SetAttendance("org-a", p.ID, "sw-1", "excused", "coach-2")
	if err != nil {
		t.Fatalf("SetAttendance update: %v", err)
	}
	if second.ID != first.ID || second.Status != "excused" || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("expected in-place update of the same record, got %+v then %+v", first, second)
	}
	if _, err := svc.SetAttendance("org-a", p.ID, "sw-2", "absent", "coach-1"); err != nil {
		t.Fatalf("SetAttendance sw-2: %v", err)
	}

	rows, err := svc.ListAttendance("org-a", p.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("ListAttendance: want 2 rows, got %d (%v)", len(rows), err)
	}
	if _, err := svc.ListAttendance("org-b", p.ID); err != ErrNotFound {
		t.Fatalf("cross-org attendance: want ErrNotFound, got %v", err)
	}
}

func TestPracticeDateValidated(t *testing.T) {
	svc := NewService()
	if _, err := svc.Create("org-a", "07/02/2026", "g", "water", "x", 0, nil, PracticeStrokes{}, "", "c"); !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("Create bad date: %v", err)
	}
	p, err := svc.Create("org-a", "2026-07-02", "g", "water", "x", 0, nil, PracticeStrokes{}, "", "c")
	if err != nil {
		t.Fatal(err)
	}
	bad := "tomorrow"
	if _, err := svc.Update("org-a", p.ID, PracticeUpdates{Date: &bad}, "c"); !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("Update bad date: %v", err)
	}
}
