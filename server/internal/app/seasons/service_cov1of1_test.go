package seasons

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

func cov1xStr(s string) *string { return &s }

func cov1xDBService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	return NewServiceWithPool(pool), pool
}

func TestCov1xNewServiceWithNilPoolIsInMemory(t *testing.T) {
	svc := NewServiceWithPool(nil)
	if svc.repo != nil {
		t.Fatal("nil pool must leave the service in-memory")
	}
	list, err := svc.List("org-empty")
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty list must be non-nil empty slice: %#v %v", list, err)
	}
}

func TestCov1xInMemoryUpdateAppliesAllFieldsAndValidates(t *testing.T) {
	svc := NewService()
	s, err := svc.Create("org-1", "Base", "2026-01-01", "2026-12-31", "upcoming", "g", "creator")
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 121)
	if _, err := svc.Update("org-1", s.ID, SeasonUpdates{Name: &long}, "u"); err != ErrInvalidName {
		t.Fatalf("long name: %v", err)
	}
	if _, err := svc.Update("org-1", s.ID, SeasonUpdates{Status: cov1xStr("paused")}, "u"); err != ErrInvalidStatus {
		t.Fatalf("status: %v", err)
	}
	if _, err := svc.Update("org-1", s.ID, SeasonUpdates{StartDate: cov1xStr("2027-01-01")}, "u"); err != ErrInvalidDates {
		t.Fatalf("start after end: %v", err)
	}
	if _, err := svc.Update("org-1", s.ID, SeasonUpdates{EndDate: cov1xStr("bad")}, "u"); err != ErrInvalidDates {
		t.Fatalf("bad end: %v", err)
	}
	if _, err := svc.Update("org-1", s.ID, SeasonUpdates{StartDate: cov1xStr("bad")}, "u"); err != ErrInvalidDates {
		t.Fatalf("bad start: %v", err)
	}
	// Failed updates must not have mutated the cached record.
	got, _ := svc.Get("org-1", s.ID)
	if got.StartDate != "2026-01-01" || got.Status != "upcoming" || got.Name != "Base" {
		t.Fatalf("failed update leaked: %+v", got)
	}
	up, err := svc.Update("org-1", s.ID, SeasonUpdates{
		Name: cov1xStr(" Renamed "), StartDate: cov1xStr("2026-02-01"), EndDate: cov1xStr("2026-11-30"),
		Status: cov1xStr("active"), Goals: cov1xStr("new"),
	}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	if up.Name != "Renamed" || up.StartDate != "2026-02-01" || up.EndDate != "2026-11-30" ||
		up.Status != "active" || up.Goals != "new" || up.UpdatedBy != "editor" || up.CreatedBy != "creator" {
		t.Fatalf("unexpected update result: %+v", up)
	}
}

func TestCov1xInMemoryCreateRejectsLongNameAndBadEnd(t *testing.T) {
	svc := NewService()
	if _, err := svc.Create("o", strings.Repeat("é", 121), "", "", "active", "", "a"); err != ErrInvalidName {
		t.Fatalf("long name: %v", err)
	}
	if _, err := svc.Create("o", "S", "", "nope", "active", "", "a"); err != ErrInvalidDates {
		t.Fatalf("bad end: %v", err)
	}
	if _, err := svc.Create("o", "S", "", "2026-01-01", "active", "", "a"); err != nil {
		t.Fatalf("end only should be valid: %v", err)
	}
}

func TestCov1xDBCRUDRoundTripAndOrgScope(t *testing.T) {
	svc, _ := cov1xDBService(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()

	empty, err := svc.List(orgA)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty db list: %#v %v", empty, err)
	}

	older, err := svc.Create(orgA, "Older", "2024-09-01", "2025-06-30", "completed", "g1", "u1")
	if err != nil {
		t.Fatal(err)
	}
	newer, err := svc.Create(orgA, "Newer", "2025-09-01", "", "active", "", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if newer.EndDate != "" || newer.StartDate != "2025-09-01" || newer.CreatedBy != "u1" {
		t.Fatalf("round trip: %+v", newer)
	}
	if _, err := svc.Create(orgB, "Foreign", "", "", "active", "", "u2"); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(orgA)
	if err != nil || len(list) != 2 || list[0].ID != newer.ID || list[1].ID != older.ID {
		t.Fatalf("list: %+v %v", list, err)
	}

	got, err := svc.Get(orgA, older.ID)
	if err != nil || got.Goals != "g1" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := svc.Get(orgB, older.ID); err != ErrNotFound {
		t.Fatalf("foreign get: %v", err)
	}
	if _, err := svc.Get(orgA, "not-a-uuid"); err != ErrNotFound {
		t.Fatalf("bad id get: %v", err)
	}
	if _, err := svc.Get(orgA, uuid.NewString()); err != ErrNotFound {
		t.Fatalf("missing get: %v", err)
	}

	up, err := svc.Update(orgA, older.ID, SeasonUpdates{Goals: cov1xStr("g2"), Status: cov1xStr("active")}, "u9")
	if err != nil || up.Goals != "g2" || up.Status != "active" || up.UpdatedBy != "u9" {
		t.Fatalf("update: %+v %v", up, err)
	}
	if !up.UpdatedAt.After(up.CreatedAt) {
		t.Fatalf("updated_at not advanced: %+v", up)
	}
	if _, err := svc.Update(orgA, older.ID, SeasonUpdates{Name: cov1xStr("")}, "u9"); err != ErrInvalidName {
		t.Fatalf("mutate error must propagate: %v", err)
	}
	after, _ := svc.Get(orgA, older.ID)
	if after.Name != "Older" {
		t.Fatalf("rolled-back update leaked: %+v", after)
	}
	if _, err := svc.Update(orgB, older.ID, SeasonUpdates{}, "x"); err != ErrNotFound {
		t.Fatalf("foreign update: %v", err)
	}
	if _, err := svc.Update(orgA, "bad", SeasonUpdates{}, "x"); err != ErrNotFound {
		t.Fatalf("bad id update: %v", err)
	}

	if err := svc.Delete(orgA, "bad"); err != ErrNotFound {
		t.Fatalf("bad id delete: %v", err)
	}
	if err := svc.Delete(orgB, older.ID); err != ErrNotFound {
		t.Fatalf("foreign delete: %v", err)
	}
	if err := svc.Delete(orgA, older.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(orgA, older.ID); err != ErrNotFound {
		t.Fatalf("deleted still present: %v", err)
	}
}

func TestCov1xDBErrorsSurfaceWhenBrokenSchema(t *testing.T) {
	svc, pool := cov1xDBService(t)
	org := uuid.NewString()
	id := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `ALTER TABLE seasons RENAME COLUMN goals TO goals_cov1x`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(org, "S", "", "", "active", "", "u"); err == nil {
		t.Fatal("create must fail")
	}
	if _, err := svc.List(org); err == nil || !strings.Contains(err.Error(), "list seasons") {
		t.Fatalf("list: %v", err)
	}
	if _, err := svc.Get(org, id); err == nil || !strings.Contains(err.Error(), "get season") {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Update(org, id, SeasonUpdates{}, "u"); err == nil || !strings.Contains(err.Error(), "lock season") {
		t.Fatalf("update: %v", err)
	}
}

func TestCov1xDBUpdateFailsWhenWriteBackFails(t *testing.T) {
	svc, pool := cov1xDBService(t)
	org := uuid.NewString()
	s, err := svc.Create(org, "S", "", "", "active", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	// A CHECK-style trigger that rejects updates makes the in-tx upsert fail.
	for _, q := range []string{
		`CREATE FUNCTION cov1x_reject() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x rejected'; END $$`,
		`CREATE TRIGGER cov1x_trg BEFORE UPDATE ON seasons FOR EACH ROW EXECUTE FUNCTION cov1x_reject()`,
	} {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Update(org, s.ID, SeasonUpdates{Goals: cov1xStr("x")}, "u"); err == nil || !strings.Contains(err.Error(), "cov1x rejected") {
		t.Fatalf("expected write-back failure, got %v", err)
	}
}

func TestCov1xDBUpdateCommitFailure(t *testing.T) {
	svc, pool := cov1xDBService(t)
	org := uuid.NewString()
	s, err := svc.Create(org, "S", "", "", "active", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE FUNCTION cov1x_reject() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x deferred'; END $$`,
		`CREATE CONSTRAINT TRIGGER cov1x_ctrg AFTER UPDATE ON seasons DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cov1x_reject()`,
	} {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Update(org, s.ID, SeasonUpdates{Goals: cov1xStr("x")}, "u"); err == nil || !strings.Contains(err.Error(), "commit update") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	got, _ := svc.Get(org, s.ID)
	if got.Goals != "" {
		t.Fatalf("failed commit persisted: %+v", got)
	}
}

func TestCov1xDBBeginFailsOnClosedPool(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()
	s, err := svc.Create(org, "S", "", "", "active", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	if _, err := svc.Update(org, s.ID, SeasonUpdates{}, "u"); err == nil || !strings.Contains(err.Error(), "begin update") {
		t.Fatalf("expected begin failure, got %v", err)
	}
	if err := svc.Delete(org, s.ID); err == nil || err == ErrNotFound {
		t.Fatalf("delete on closed pool: %v", err)
	}
}

func TestCov1xInMemoryListTiebreaksEqualStartDatesByNewestCreated(t *testing.T) {
	svc := NewService()
	first, err := svc.Create("org-1", "First", "2026-01-01", "", "active", "", "a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create("org-1", "Second", "2026-01-01", "", "active", "", "a")
	if err != nil {
		t.Fatal(err)
	}
	// Force a deterministic creation order independent of clock resolution.
	svc.seasons[second.ID].CreatedAt = first.CreatedAt.Add(1)
	list, err := svc.List("org-1")
	if err != nil || len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("tiebreak order wrong: %+v %v", list, err)
	}
}
