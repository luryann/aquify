package documents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

func cov1xExec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// --- in-memory behavior -----------------------------------------------------

func TestCov1xDocsInMemoryTypeUpdateListDelete(t *testing.T) {
	t.Parallel()
	svc := NewService()
	dt, err := svc.CreateDocumentType("o1", "  Waiver  ", "", " desc ", "", false, true, nil, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if dt.Name != "Waiver" || dt.Category != "other" || dt.AppliesTo != "swimmer" || dt.Description != "desc" {
		t.Fatalf("defaults not applied: %+v", dt)
	}
	other, err := svc.CreateDocumentType("o1", "Medical", "medical", "", "household", true, false, nil, "u1")
	if err != nil {
		t.Fatal(err)
	}

	months := 6
	upd, err := svc.UpdateDocumentType("o1", dt.ID, map[string]interface{}{
		"name": " Renamed ", "category": "waiver", "description": " new ", "is_required": true,
		"applies_to": "all", "expiry_months": &months, "is_active": false,
	}, "u2")
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != "Renamed" || upd.Category != "waiver" || upd.Description != "new" || !upd.IsRequired ||
		upd.AppliesTo != "all" || upd.ExpiryMonths == nil || *upd.ExpiryMonths != 6 || upd.IsActive || upd.UpdatedBy != "u2" {
		t.Fatalf("update not applied: %+v", upd)
	}
	if _, err := svc.UpdateDocumentType("o1", other.ID, map[string]interface{}{"name": "renamed"}, "u2"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("want ErrDuplicateName, got %v", err)
	}
	if _, err := svc.UpdateDocumentType("o2", dt.ID, nil, "u2"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("cross-org update: want ErrTypeNotFound, got %v", err)
	}

	all, err := svc.ListDocumentTypes("o1", false)
	if err != nil || len(all) != 2 || all[0].Name != "Medical" || all[1].Name != "Renamed" {
		t.Fatalf("list all = %+v, %v", all, err)
	}
	active, err := svc.ListDocumentTypes("o1", true)
	if err != nil || len(active) != 0 {
		t.Fatalf("list active = %+v, %v", active, err)
	}
	none, err := svc.ListDocumentTypes("nobody", false)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("empty org list must be non-nil empty slice, got %#v, %v", none, err)
	}

	if err := svc.DeleteDocumentType("o2", dt.ID, "u"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("cross-org delete: want ErrTypeNotFound, got %v", err)
	}
	if err := svc.DeleteDocumentType("o1", dt.ID, "u"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteDocumentType("o1", dt.ID, "u"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("second delete: want ErrTypeNotFound, got %v", err)
	}
}

func TestCov1xDocsInMemorySubmissions(t *testing.T) {
	t.Parallel()
	svc := NewService()
	swimmerType, _ := svc.CreateDocumentType("o1", "Medical", "medical", "", "swimmer", true, true, nil, "u")
	householdType, _ := svc.CreateDocumentType("o1", "Waiver", "waiver", "", "household", true, true, nil, "u")
	inactive, _ := svc.CreateDocumentType("o1", "Old", "other", "", "all", false, false, nil, "u")

	if _, err := svc.CreateSubmission("o1", "missing", "h1", "", "p"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("unknown type: %v", err)
	}
	if _, err := svc.CreateSubmission("o2", swimmerType.ID, "h1", "", "p"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("cross-org type: %v", err)
	}
	if _, err := svc.CreateSubmission("o1", inactive.ID, "h1", "", "p"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("inactive type: %v", err)
	}
	if _, err := svc.CreateSubmission("o1", householdType.ID, "h1", "s1", "p"); !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("swimmer submission against household-only type: %v", err)
	}
	noExpiry, err := svc.CreateSubmission("o1", householdType.ID, "h1", "", "p")
	if err != nil || noExpiry.ExpiresAt != nil || noExpiry.DocumentTypeName != "Waiver" {
		t.Fatalf("household submission: %+v, %v", noExpiry, err)
	}
	s2, err := svc.CreateSubmission("o1", swimmerType.ID, "h2", "s2", "p")
	if err != nil {
		t.Fatal(err)
	}

	// review requires "submitted"
	if _, err := svc.VerifySubmission("o1", s2.ID, "admin", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("verify pending: %v", err)
	}
	if _, err := svc.VerifySubmission("o2", s2.ID, "admin", ""); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("verify cross-org: %v", err)
	}
	if _, err := svc.AttachFile("o2", s2.ID, "k", "f", "m", 1, "p"); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("attach cross-org: %v", err)
	}
	if _, err := svc.AttachFile("o1", s2.ID, "key-2", "f.pdf", "application/pdf", 10, "p"); err != nil {
		t.Fatal(err)
	}
	rej, err := svc.RejectSubmission("o1", s2.ID, "admin", "  blurry ")
	if err != nil || rej.Status != "rejected" || rej.ReviewNotes != "blurry" || rej.VerifiedAt != nil || rej.VerifiedBy != "" {
		t.Fatalf("reject: %+v, %v", rej, err)
	}
	// rejected submissions may be re-attached
	if again, err := svc.AttachFile("o1", s2.ID, "key-2b", "g.pdf", "application/pdf", 12, "p"); err != nil || again.Status != "submitted" || again.FileObjectKey != "key-2b" {
		t.Fatalf("re-attach after reject: %+v, %v", again, err)
	}

	got, err := svc.GetSubmission("o1", s2.ID)
	if err != nil || got.DocumentTypeName != "Medical" {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if _, err := svc.GetSubmission("o1", "nope"); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	// Type rename is denormalized on read; deleted type keeps the stored name.
	if _, err := svc.UpdateDocumentType("o1", swimmerType.ID, map[string]interface{}{"name": "Physical"}, "u"); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetSubmission("o1", s2.ID); got.DocumentTypeName != "Physical" {
		t.Fatalf("type name not denormalized: %q", got.DocumentTypeName)
	}
	if err := svc.DeleteDocumentType("o1", swimmerType.ID, "u"); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetSubmission("o1", s2.ID); got.DocumentTypeName != "Medical" {
		t.Fatalf("stored name expected after type delete, got %q", got.DocumentTypeName)
	}

	allSubs, err := svc.ListSubmissions("o1", "")
	if err != nil || len(allSubs) != 2 {
		t.Fatalf("list all: %+v, %v", allSubs, err)
	}
	if !allSubs[0].CreatedAt.Before(allSubs[1].CreatedAt) && !allSubs[0].CreatedAt.Equal(allSubs[1].CreatedAt) {
		t.Fatal("submissions must be sorted by creation time")
	}
	h2, err := svc.ListSubmissions("o1", "h2")
	if err != nil || len(h2) != 1 || h2[0].ID != s2.ID || h2[0].DocumentTypeName != "Medical" {
		t.Fatalf("household filter: %+v, %v", h2, err)
	}
	empty, err := svc.ListSubmissions("o2", "")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list: %#v, %v", empty, err)
	}

	keys, err := svc.HouseholdObjectKeys(context.Background(), "o1", "h2")
	if err != nil || len(keys) != 1 || keys[0] != "key-2b" {
		t.Fatalf("keys: %v, %v", keys, err)
	}
	if keys, _ := svc.HouseholdObjectKeys(context.Background(), "o1", "h1"); len(keys) != 0 {
		t.Fatalf("h1 has no files, got %v", keys)
	}
	erased, err := svc.EraseHouseholdData(context.Background(), "o1", "h2")
	if err != nil || len(erased) != 1 || erased[0] != "key-2b" {
		t.Fatalf("erase: %v, %v", erased, err)
	}
	if _, err := svc.GetSubmission("o1", s2.ID); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("erased submission still readable: %v", err)
	}
	if _, err := svc.GetSubmission("o1", noExpiry.ID); err != nil {
		t.Fatalf("other household must be untouched: %v", err)
	}
	if err := svc.Refresh(context.Background(), "o1"); err != nil {
		t.Fatalf("in-memory refresh is a no-op: %v", err)
	}
}

func TestCov1xDocsExpiryComputed(t *testing.T) {
	t.Parallel()
	svc := NewService()
	m := 12
	dt, _ := svc.CreateDocumentType("o", "Med", "medical", "", "swimmer", true, true, &m, "u")
	sub, err := svc.CreateSubmission("o", dt.ID, "h", "s", "p")
	if err != nil || sub.ExpiresAt == nil {
		t.Fatalf("%+v %v", sub, err)
	}
	want := sub.CreatedAt.AddDate(1, 0, 0)
	if !sub.ExpiresAt.Equal(want) {
		t.Fatalf("expires %v want %v", sub.ExpiresAt, want)
	}
	zero := 0
	dt2, _ := svc.CreateDocumentType("o", "Zero", "medical", "", "swimmer", true, true, &zero, "u")
	if s, _ := svc.CreateSubmission("o", dt2.ID, "h", "s", "p"); s.ExpiresAt != nil {
		t.Fatal("zero-month expiry must not set ExpiresAt")
	}
}

// --- Postgres-backed behavior ------------------------------------------------

func TestCov1xDocsDBRoundTripAndRefreshAll(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, org2, hh, user, swimmerX := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	a := NewServiceWithPool(pool)
	m := 3
	dt, err := a.CreateDocumentType(org, "Medical", "medical", "d", "swimmer", true, true, &m, user)
	if err != nil {
		t.Fatal(err)
	}
	dt2, err := a.CreateDocumentType(org2, "Other Club Form", "other", "", "all", false, true, nil, user)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := a.CreateSubmission(org, dt.ID, hh, swimmerX, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AttachFile(org, sub.ID, "k/1", "f.pdf", "application/pdf", 42, user); err != nil {
		t.Fatal(err)
	}
	if _, err := a.VerifySubmission(org, sub.ID, user, "ok"); err != nil {
		t.Fatal(err)
	}

	// A fresh instance observes everything persisted (empty org id loads every club).
	b := NewServiceWithPool(pool)
	if err := b.Refresh(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if got, err := b.GetSubmission(org, sub.ID); err != nil || got.Status != "verified" || got.FileSizeBytes != 42 ||
		got.VerifiedBy != user || got.SwimmerID != swimmerX || got.DocumentTypeName != "Medical" {
		t.Fatalf("persisted submission: %+v, %v", got, err)
	}
	if types, _ := b.ListDocumentTypes(org2, false); len(types) != 1 || types[0].ID != dt2.ID {
		t.Fatalf("org2 types after full refresh: %+v", types)
	}
	if types, _ := b.ListDocumentTypes(org, false); len(types) != 1 || types[0].ExpiryMonths == nil || *types[0].ExpiryMonths != 3 {
		t.Fatalf("org types: %+v", types)
	}

	// Delete through one instance disappears for the other; erase removes rows.
	if err := a.DeleteDocumentType(org2, dt2.ID, user); err != nil {
		t.Fatal(err)
	}
	if types, _ := b.ListDocumentTypes(org2, false); len(types) != 0 {
		t.Fatalf("deleted type still visible: %+v", types)
	}
	keys, err := b.EraseHouseholdData(context.Background(), org, hh)
	if err != nil || len(keys) != 1 || keys[0] != "k/1" {
		t.Fatalf("erase: %v, %v", keys, err)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM document_submissions WHERE household_id = $1::uuid`, hh).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows remain after erase: %d, %v", n, err)
	}
}

func TestCov1xDocsDBClosedPoolSurfacesLoadErrors(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool)
	dt, err := svc.CreateDocumentType(org, "Waiver", "waiver", "", "household", true, true, nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := svc.CreateSubmission(org, dt.ID, hh, "", "u")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()

	ctx := context.Background()
	calls := map[string]func() error{
		"create type": func() error { _, e := svc.CreateDocumentType(org, "X", "", "", "", false, true, nil, "u"); return e },
		"update type": func() error { _, e := svc.UpdateDocumentType(org, dt.ID, nil, "u"); return e },
		"delete type": func() error { return svc.DeleteDocumentType(org, dt.ID, "u") },
		"list types":  func() error { _, e := svc.ListDocumentTypes(org, false); return e },
		"create sub":  func() error { _, e := svc.CreateSubmission(org, dt.ID, hh, "", "u"); return e },
		"attach":      func() error { _, e := svc.AttachFile(org, sub.ID, "k", "f", "m", 1, "u"); return e },
		"verify":      func() error { _, e := svc.VerifySubmission(org, sub.ID, "u", ""); return e },
		"get sub":     func() error { _, e := svc.GetSubmission(org, sub.ID); return e },
		"list subs":   func() error { _, e := svc.ListSubmissions(org, ""); return e },
		"refresh":     func() error { return svc.Refresh(ctx, org) },
		"object keys": func() error { _, e := svc.HouseholdObjectKeys(ctx, org, hh); return e },
		"erase":       func() error { _, e := svc.EraseHouseholdData(ctx, org, hh); return e },
	}
	for name, call := range calls {
		err := call()
		if err == nil {
			t.Errorf("%s: expected error from closed pool", name)
			continue
		}
		if errors.Is(err, ErrTypeNotFound) || errors.Is(err, ErrSubmissionNotFound) || errors.Is(err, ErrDuplicateName) {
			t.Errorf("%s: load failure must not masquerade as a domain error: %v", name, err)
		}
	}
	// The helpers wrap the refresh failure with context.
	if _, err := svc.EraseHouseholdData(ctx, org, hh); err == nil || !strings.Contains(err.Error(), "erase household document submissions: refresh") {
		t.Fatalf("erase error should be wrapped: %v", err)
	}
	if _, err := svc.HouseholdObjectKeys(ctx, org, hh); err == nil || !strings.Contains(err.Error(), "list household document keys: refresh") {
		t.Fatalf("keys error should be wrapped: %v", err)
	}
}

func TestCov1xDocsDBLoadSubmissionsQueryFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	cov1xExec(t, pool, `ALTER TABLE document_submissions RENAME COLUMN review_notes TO review_notes_x`)
	_, err := svc.repo.LoadSubmissions(context.Background(), uuid.NewString())
	if err == nil || !strings.Contains(err.Error(), "documents: load document_submissions") {
		t.Fatalf("want wrapped load error, got %v", err)
	}
	if _, err := svc.ListDocumentTypes(uuid.NewString(), false); err == nil {
		t.Fatal("LoadAll must propagate the submissions load failure")
	}
}

func TestCov1xDocsDBWriteFailuresAreReportedAndNotCached(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool)
	dt, err := svc.CreateDocumentType(org, "Waiver", "waiver", "", "household", true, true, nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := svc.CreateSubmission(org, dt.ID, hh, "", "u")
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := svc.CreateSubmission(org, dt.ID, hh, "", "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AttachFile(org, submitted.ID, "k", "f", "m", 1, "u"); err != nil {
		t.Fatal(err)
	}

	// Type writes: block a name and any soft delete.
	cov1xExec(t, pool, `ALTER TABLE document_types ADD CONSTRAINT cov1x_name CHECK (name <> 'Blocked') NOT VALID`)
	cov1xExec(t, pool, `ALTER TABLE document_types ADD CONSTRAINT cov1x_nodelete CHECK (deleted_at IS NULL) NOT VALID`)
	if _, err := svc.CreateDocumentType(org, "Blocked", "", "", "", false, true, nil, "u"); err == nil || errors.Is(err, ErrDuplicateName) {
		t.Fatalf("check violation must surface as a plain error, got %v", err)
	}
	if _, err := svc.UpdateDocumentType(org, dt.ID, map[string]interface{}{"name": "Blocked"}, "u"); err == nil || errors.Is(err, ErrDuplicateName) {
		t.Fatalf("update: check violation must surface as a plain error, got %v", err)
	}
	if err := svc.DeleteDocumentType(org, dt.ID, "u"); err == nil {
		t.Fatal("delete should fail when the soft-delete update is rejected")
	}
	if types, _ := svc.ListDocumentTypes(org, false); len(types) != 1 || types[0].Name != "Waiver" {
		t.Fatalf("failed writes must leave the type untouched: %+v", types)
	}

	// Submission writes: block every upsert that touches status changes or inserts.
	cov1xExec(t, pool, `ALTER TABLE document_submissions ADD CONSTRAINT cov1x_status CHECK (status = 'pending') NOT VALID`)
	if _, err := svc.CreateSubmission(org, dt.ID, hh, "", "u"); err != nil {
		t.Fatalf("pending insert is allowed: %v", err)
	}
	if _, err := svc.AttachFile(org, sub.ID, "k2", "f", "m", 1, "u"); err == nil {
		t.Fatal("attach should surface the upsert failure")
	}
	if _, err := svc.VerifySubmission(org, submitted.ID, "u", ""); err == nil {
		t.Fatal("verify should surface the upsert failure")
	}
	cov1xExec(t, pool, `ALTER TABLE document_submissions DROP CONSTRAINT cov1x_status`)
	cov1xExec(t, pool, `ALTER TABLE document_submissions ADD CONSTRAINT cov1x_nopending CHECK (status <> 'pending') NOT VALID`)
	if _, err := svc.CreateSubmission(org, dt.ID, hh, "", "u"); err == nil {
		t.Fatal("create submission should surface the insert failure")
	}
	got, err := svc.GetSubmission(org, sub.ID)
	if err != nil || got.Status != "pending" || got.FileObjectKey != "" {
		t.Fatalf("failed attach must not change state: %+v, %v", got, err)
	}
}

func TestCov1xDocsDBEraseFailureKeepsRows(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool)
	dt, err := svc.CreateDocumentType(org, "Waiver", "waiver", "", "household", true, true, nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := svc.CreateSubmission(org, dt.ID, hh, "", "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AttachFile(org, sub.ID, "keep/me", "f", "m", 1, "u"); err != nil {
		t.Fatal(err)
	}
	cov1xExec(t, pool, `CREATE FUNCTION cov1x_block_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x delete blocked'; END $$`)
	cov1xExec(t, pool, `CREATE TRIGGER cov1x_no_delete BEFORE DELETE ON document_submissions FOR EACH ROW EXECUTE FUNCTION cov1x_block_delete()`)

	keys, err := svc.EraseHouseholdData(context.Background(), org, hh)
	if err == nil || !strings.Contains(err.Error(), "cov1x delete blocked") || keys != nil {
		t.Fatalf("want wrapped db error and no keys, got keys=%v err=%v", keys, err)
	}
	if got, err := svc.HouseholdObjectKeys(context.Background(), org, hh); err != nil || len(got) != 1 || got[0] != "keep/me" {
		t.Fatalf("rows and keys must remain for retry: %v, %v", got, err)
	}
}

func TestCov1xDocsDBSoftDeleteMissingTypeIsNoop(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	if err := svc.repo.SoftDeleteDocumentType(context.Background(), uuid.NewString(), uuid.NewString(), "u"); err != nil {
		t.Fatalf("deleting a nonexistent type should only warn, got %v", err)
	}
	pool.Close()
	if err := svc.repo.SoftDeleteDocumentType(context.Background(), uuid.NewString(), uuid.NewString(), "u"); err == nil {
		t.Fatal("closed pool should surface an error")
	}
}

func TestCov1xDocsIsUniqueViolationAndNullableInt(t *testing.T) {
	t.Parallel()
	if isUniqueViolation(errors.New("boom")) || isUniqueViolation(nil) {
		t.Fatal("non-pg errors are not unique violations")
	}
	if nullableInt64(0) != nil || nullableInt64(7) != int64(7) {
		t.Fatal("nullableInt64 must map 0 to nil and pass other values through")
	}
}

func TestCov1xDocsNilPoolAndCacheLimits(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithPool(nil)
	if svc.repo != nil {
		t.Fatal("nil pool must yield an in-memory service")
	}
	svc.SetOrgCacheLimits(0, 0)
	if _, err := svc.CreateDocumentType("o", "A", "", "", "", false, true, nil, "u"); err != nil {
		t.Fatal(err)
	}
}

func TestCov1xDocsDBCacheEvictionReloadsFromPostgres(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	svc.SetOrgCacheLimits(0, 1) // keep at most one club cached
	orgA, orgB := uuid.NewString(), uuid.NewString()
	if _, err := svc.CreateDocumentType(orgA, "A form", "", "", "", false, true, nil, "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDocumentType(orgB, "B form", "", "", "", false, true, nil, "u"); err != nil {
		t.Fatal(err)
	}
	svc.mu.RLock()
	cached := len(svc.docTypes)
	svc.mu.RUnlock()
	if cached != 1 {
		t.Fatalf("max-orgs=1 should evict the idle club, cache holds %d types", cached)
	}
	if types, err := svc.ListDocumentTypes(orgA, false); err != nil || len(types) != 1 || types[0].Name != "A form" {
		t.Fatalf("evicted club must reload on demand: %+v, %v", types, err)
	}
}

// Another instance can win the unique-name race between this instance's
// in-memory check and its insert. A trigger deterministically plays that
// competitor: when a type named exactly "Raced"/"Raced Two" is written, it
// first inserts a same-org row differing only by case, so the unique index on
// (org_id, lower(name)) rejects the write and the service maps it cleanly.
func TestCov1xDocsDBUniqueIndexRaceMapsToDuplicateName(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	svc := NewServiceWithPool(pool)
	cov1xExec(t, pool, `CREATE FUNCTION cov1x_compete() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r document_types;
BEGIN
  IF NEW.name IN ('Raced', 'Raced Two') THEN
    r := NEW;
    r.id := gen_random_uuid();
    r.name := upper(NEW.name);
    INSERT INTO document_types SELECT (r).*;
  END IF;
  RETURN NEW;
END $$`)
	cov1xExec(t, pool, `CREATE TRIGGER cov1x_compete_ins BEFORE INSERT ON document_types FOR EACH ROW EXECUTE FUNCTION cov1x_compete()`)
	cov1xExec(t, pool, `CREATE TRIGGER cov1x_compete_upd BEFORE UPDATE ON document_types FOR EACH ROW EXECUTE FUNCTION cov1x_compete()`)

	if _, err := svc.CreateDocumentType(org, "Raced", "", "", "", false, true, nil, "u"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("create: want ErrDuplicateName, got %v", err)
	}
	dt, err := svc.CreateDocumentType(org, "Plain", "", "", "", false, true, nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateDocumentType(org, dt.ID, map[string]interface{}{"name": "Raced Two"}, "u"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("update: want ErrDuplicateName, got %v", err)
	}
	if types, _ := svc.ListDocumentTypes(org, false); len(types) != 1 || types[0].Name != "Plain" {
		t.Fatalf("failed writes must leave only the original type: %+v", types)
	}
}
