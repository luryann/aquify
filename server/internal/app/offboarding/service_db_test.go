package offboarding

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/testutil"
)

type fixture struct {
	svc   *Service
	pool  *pgxpool.Pool
	reg   *orgs.Service
	store *storage.StubStorage
	kc    *keycloak.StubClient
	a, b  *orgs.Org
	now   time.Time
}

func seedOrg(t *testing.T, f *fixture, o *orgs.Org, suffix string) {
	t.Helper()
	ctx := context.Background()
	hh, sw := uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, $3, 'active')`, []any{hh, o.ID, "HH " + suffix}},
		{`INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'S', '2012-01-01', 'female', 'active')`, []any{sw, o.ID, hh, "Kid" + suffix}},
		{`INSERT INTO messages (id, org_id, household_id, swimmer_id, sender_user_id, sender_role, body) VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3::uuid, gen_random_uuid(), 'parent', $4)`, []any{o.ID, hh, sw, "msg " + suffix}},
		{`INSERT INTO org_invites (org_id, code_hash, role, expires_at) VALUES ($1::uuid, $2, 'parent', now() + interval '1 day')`, []any{o.ID, "hash-" + suffix}},
		{`INSERT INTO audit_events (id, org_id, actor_user_id, action, resource_type, resource_id) VALUES ($1, $2::uuid, 'u', 'a', 'r', '1')`, []any{uuid.NewString(), o.ID}},
	} {
		if _, err := f.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed %s: %v\n%s", suffix, err, q.sql)
		}
	}
	_ = f.store.PutObject(ctx, "documents/"+o.ID+"/sub/file-"+suffix+".pdf", "application/pdf", []byte("pdf "+suffix))
	if _, err := f.kc.CreateUser(ctx, "user-"+suffix+"@example.com", "U", suffix, o.ID); err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	f := &fixture{pool: pool, reg: orgs.NewServiceWithPool(pool), store: storage.NewStubStorage("http://stub"), kc: keycloak.NewStubClient(), now: time.Now()}
	ctx := context.Background()
	mk := func(slug string) *orgs.Org {
		o, err := f.reg.Create(ctx, orgs.CreateInput{Slug: slug, Name: slug})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.reg.SetStatus(ctx, o.ID, orgs.StatusActive); err != nil {
			t.Fatal(err)
		}
		return o
	}
	f.a, f.b = mk("club-a"), mk("club-b")
	seedOrg(t, f, f.a, "a")
	seedOrg(t, f, f.b, "b")
	f.svc = &Service{Pool: pool, Orgs: f.reg, Store: f.store, Keycloak: f.kc, CoolingOff: time.Hour, Now: func() time.Time { return f.now }}
	return f
}

func TestExportContainsOnlyThisOrgsDataAndNoSecrets(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Export(context.Background(), f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := f.store.GetObject(context.Background(), res.Key)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, file := range zr.File {
		rc, _ := file.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		names[file.Name] = string(b)
	}
	if !strings.Contains(names["households.ndjson"], "HH a") || strings.Contains(names["households.ndjson"], "HH b") {
		t.Fatalf("households must be exactly org A's: %q", names["households.ndjson"])
	}
	for _, secret := range []string{"org_invites.ndjson", "sessions.ndjson", "mfa_factors.ndjson", "usa_swimming_integrations.ndjson"} {
		if _, found := names[secret]; found {
			t.Fatalf("%s must never be exported", secret)
		}
	}
	for name, body := range names {
		if strings.Contains(body, f.b.ID) || strings.Contains(body, "hash-a") {
			t.Fatalf("%s leaks another org's id or a secret hash: %.120s", name, body)
		}
	}
	if !strings.Contains(names["audit_events.ndjson"], f.a.ID) || res.Files != 1 || !strings.Contains(names["files.json"], "file-a.pdf") {
		t.Fatalf("audit events and the file manifest belong in the export: files=%d manifest=%s", res.Files, names["files.json"])
	}
}

func TestPurgeIsGuardedAndRemovesOnlyThisOrg(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Purge(ctx, f.a.ID, "club-a"); !errors.Is(err, ErrNotOffboarding) {
		t.Fatalf("an active org cannot be purged: %v", err)
	}
	if _, err := f.reg.SetStatus(ctx, f.a.ID, orgs.StatusOffboarding); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Purge(ctx, f.a.ID, "club-a"); !errors.Is(err, ErrCoolingOff) {
		t.Fatalf("cooling-off must be enforced: %v", err)
	}
	f.now = f.now.Add(2 * time.Hour)
	if _, err := f.svc.Purge(ctx, f.a.ID, "wrong"); !errors.Is(err, ErrConfirmMismatch) {
		t.Fatalf("typed confirmation must match: %v", err)
	}
	f.svc.PlatformOrgID = f.a.ID
	if _, err := f.svc.Purge(ctx, f.a.ID, "club-a"); !errors.Is(err, ErrProtectedOrg) {
		t.Fatalf("the platform org is protected: %v", err)
	}
	f.svc.PlatformOrgID = ""

	res, err := f.svc.Purge(ctx, f.a.ID, "club-a")
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesDeleted != 1 || res.UsersDeleted != 1 || res.RowsDeleted["households"] != 1 || res.RowsDeleted["org_invites"] != 1 { // swimmers/messages go with the household (ON DELETE CASCADE)
		t.Fatalf("purge result: %+v", res)
	}
	count := func(table, org string) int {
		var n int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE org_id = $1::uuid`, org).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, tbl := range []string{"households", "swimmers", "messages", "org_invites"} {
		if count(tbl, f.a.ID) != 0 {
			t.Errorf("%s still has org A rows", tbl)
		}
		if count(tbl, f.b.ID) != 1 {
			t.Errorf("%s lost org B's row", tbl)
		}
	}
	if count("audit_events", f.a.ID) != 1 {
		t.Error("the audit trail is retained")
	}
	if ok, _ := f.store.Exists(ctx, "documents/"+f.b.ID+"/sub/file-b.pdf"); !ok {
		t.Error("org B's files must survive")
	}
	if ok, _ := f.store.Exists(ctx, "documents/"+f.a.ID+"/sub/file-a.pdf"); ok {
		t.Error("org A's files must be gone")
	}
	if left, _ := f.kc.ListUsers(ctx, f.b.ID, "", 50, 0); len(left) != 1 {
		t.Error("org B's identity users must survive")
	}
	got, err := f.reg.Get(ctx, f.a.ID)
	if err != nil || got.Status != orgs.StatusDeleted || got.Slug != "" {
		t.Fatalf("tombstone: %+v %v", got, err)
	}
	// The released slug can be taken again; the old org id never is.
	if _, err := f.reg.Create(ctx, orgs.CreateInput{Slug: "club-a", Name: "New club-a"}); err != nil {
		t.Fatalf("slug should be reusable after purge: %v", err)
	}
}

func TestCancellingOffboardingResetsTheClock(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	o, _ := f.reg.SetStatus(ctx, f.a.ID, orgs.StatusOffboarding)
	if o.OffboardingAt == nil {
		t.Fatal("offboarding_at must be set")
	}
	back, err := f.reg.SetStatus(ctx, f.a.ID, orgs.StatusSuspended)
	if err != nil || back.OffboardingAt != nil {
		t.Fatalf("leaving offboarding clears the clock: %+v %v", back, err)
	}
}

// Tenancy tracker item 7.5: a single org can be recovered from its export without
// touching any other org.
func TestExportThenPurgeThenRestoreRecoversOnlyThatOrg(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	exp, err := f.svc.Export(ctx, f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(exp.Key, ExportKeyPrefix(f.a.ID)) {
		t.Fatalf("export key %q must live under %q", exp.Key, ExportKeyPrefix(f.a.ID))
	}

	if _, err := f.reg.SetStatus(ctx, f.a.ID, orgs.StatusOffboarding); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(2 * time.Hour)
	if _, err := f.svc.Purge(ctx, f.a.ID, "club-a"); err != nil {
		t.Fatal(err)
	}
	// The archive must survive the purge: it is the only recovery path.
	archive, _, err := f.store.GetObject(ctx, exp.Key)
	if err != nil || len(archive) == 0 {
		t.Fatalf("offboarding archive must survive purge: %v", err)
	}
	count := func(table, org string) int {
		var n int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE org_id = $1::uuid`, org).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("households", f.a.ID) != 0 {
		t.Fatal("precondition: purged")
	}

	res, err := f.svc.Restore(ctx, f.a.ID, archive)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	for _, tbl := range []string{"households", "swimmers", "messages"} {
		if count(tbl, f.a.ID) != 1 {
			t.Errorf("%s not restored", tbl)
		}
		if count(tbl, f.b.ID) != 1 {
			t.Errorf("%s: org B must be untouched", tbl)
		}
	}
	if res.RowsRestored["households"] != 1 || count("org_invites", f.a.ID) != 0 {
		t.Fatalf("restored rows %+v; credentials/invites must not come back", res.RowsRestored)
	}
	if count("audit_events", f.a.ID) != 1 {
		t.Error("audit rows already existed and must not be duplicated")
	}
	got, _ := f.reg.Get(ctx, f.a.ID)
	if got.Status != orgs.StatusSuspended {
		t.Fatalf("a restored org comes back suspended, got %s", got.Status)
	}
	// Idempotent: restoring again adds nothing.
	if again, err := f.svc.Restore(ctx, f.a.ID, archive); err != nil || again.RowsRestored["households"] != 0 {
		t.Fatalf("second restore must be a no-op: %+v %v", again, err)
	}
}

func TestRestoreNeverInsertsRowsOfAnotherOrg(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	expB, err := f.svc.Export(ctx, f.b.ID)
	if err != nil {
		t.Fatal(err)
	}
	archiveB, _, _ := f.store.GetObject(ctx, expB.Key)
	// Loading B's archive "into A" must not create any of B's rows under A or duplicate B's.
	res, err := f.svc.Restore(ctx, f.a.ID, archiveB)
	if err != nil {
		t.Fatal(err)
	}
	for table, n := range res.RowsRestored {
		if n != 0 {
			t.Errorf("%s: %d rows of another org were inserted", table, n)
		}
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM households WHERE org_id = $1::uuid`, f.a.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("org A must still have exactly its own household, got %d", n)
	}
}

// Under row-level security the application's own role only sees rows of the org it
// binds, so offboarding must bind the org for every table it touches. This runs the
// whole export/purge/restore cycle as that role over a table that has RLS (volunteer).
func TestOffboardingWorksUnderRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	reg := orgs.NewServiceWithPool(admin)
	mk := func(slug string) *orgs.Org {
		o, _ := reg.Create(ctx, orgs.CreateInput{Slug: slug, Name: slug})
		_, _ = reg.SetStatus(ctx, o.ID, orgs.StatusActive)
		return o
	}
	a, b := mk("rls-a"), mk("rls-b")
	for _, o := range []*orgs.Org{a, b} {
		if _, err := admin.Exec(ctx, `INSERT INTO volunteer_jobs (id, org_id, event_name, role, title, starts_at, ends_at, capacity, hour_credit, created_at, updated_at, created_by, updated_by)
			VALUES (gen_random_uuid(), $1::uuid, 'E', 'timer', $2, now(), now() + interval '1 hour', 1, 1, now(), now(), 'x', 'x')`, o.ID, "job of "+o.Slug); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	store := storage.NewStubStorage("http://stub")
	svc := &Service{Pool: appPool, Orgs: reg, Store: store, Keycloak: keycloak.NewStubClient(), CoolingOff: time.Hour, Now: func() time.Time { return now }}

	exp, err := svc.Export(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Tables["volunteer_jobs"] != 1 {
		t.Fatalf("the RLS-protected table must still be exported for the org: %v", exp.Tables)
	}
	archive, _, _ := store.GetObject(ctx, exp.Key)
	if _, err := reg.SetStatus(ctx, a.ID, orgs.StatusOffboarding); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if res, err := svc.Purge(ctx, a.ID, "rls-a"); err != nil || res.RowsDeleted["volunteer_jobs"] != 1 {
		t.Fatalf("purge under RLS: %+v %v", res, err)
	}
	var left, other int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM volunteer_jobs WHERE org_id = $1::uuid`, a.ID).Scan(&left)
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM volunteer_jobs WHERE org_id = $1::uuid`, b.ID).Scan(&other)
	if left != 0 || other != 1 {
		t.Fatalf("purge removed A=%d left, B=%d (want 0 and 1)", left, other)
	}
	if res, err := svc.Restore(ctx, a.ID, archive); err != nil || res.RowsRestored["volunteer_jobs"] != 1 {
		t.Fatalf("restore under RLS: %+v %v", res, err)
	}
}

// A purge claims the org (purging) so a concurrent cancel is refused, a failed purge can be
// retried, and the tombstone step is atomic (slug and status change together).
func TestPurgeClaimsOrgAndTombstoneIsAtomic(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.reg.SetStatus(ctx, f.a.ID, orgs.StatusOffboarding); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(2 * time.Hour)
	// Simulate a purge that claimed the org and then died.
	claimed, err := f.reg.SetStatus(ctx, f.a.ID, orgs.StatusPurging)
	if err != nil || claimed.OffboardingAt == nil {
		t.Fatalf("claim must keep the offboarding clock: %+v %v", claimed, err)
	}
	if _, err := f.reg.SetStatus(ctx, f.a.ID, orgs.StatusSuspended); !errors.Is(err, orgs.ErrInvalidStatus) {
		t.Fatalf("cancel must be refused while purging: %v", err)
	}
	if _, err := f.svc.Purge(ctx, f.a.ID, "club-a"); err != nil {
		t.Fatalf("a purging org must be retryable: %v", err)
	}
	got, _ := f.reg.Get(ctx, f.a.ID)
	if got.Status != orgs.StatusDeleted || got.Slug != "" {
		t.Fatalf("tombstone must set deleted and release the slug together: %+v", got)
	}
}
