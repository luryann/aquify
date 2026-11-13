package orgs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xDeadService returns a Postgres-backed service whose pool is already
// closed, so every statement fails with a driver error.
func cov1xDeadService(t *testing.T) *Service {
	t.Helper()
	live := testutil.IsolatedSchemaPoolStrict(t)
	dead, err := pgxpool.NewWithConfig(context.Background(), live.Config())
	if err != nil {
		t.Fatal(err)
	}
	dead.Close()
	return NewServiceWithPool(dead)
}

func cov1xExec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func TestCov1xCreateValidation(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, _ bool) {
		ctx := context.Background()
		if _, err := s.Create(ctx, CreateInput{Slug: "ok-slug", Name: "   "}); err == nil || !strings.Contains(err.Error(), "name is required") {
			t.Fatalf("blank name: %v", err)
		}
		if _, err := s.Create(ctx, CreateInput{ID: "not-a-uuid", Name: "A"}); err == nil || !strings.Contains(err.Error(), "id must be a UUID") {
			t.Fatalf("bad id: %v", err)
		}
		id := uuid.NewString()
		o, err := s.Create(ctx, CreateInput{ID: id, Name: " Explicit ", Plan: "pro", Region: "us"})
		if err != nil || o.ID != id || o.Name != "Explicit" || o.Plan != "pro" || o.Region != "us" || o.Slug != "" {
			t.Fatalf("explicit id: %+v %v", o, err)
		}
		if _, err := s.Create(ctx, CreateInput{ID: id, Name: "Again"}); err == nil {
			t.Fatal("duplicate id must be refused")
		}
		// Two slug-less orgs coexist (NULL slugs are not unique).
		if _, err := s.Create(ctx, CreateInput{Name: "NoSlug1"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Create(ctx, CreateInput{Name: "NoSlug2"}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCov1xGetAndBySlugNotFound(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, _ bool) {
		ctx := context.Background()
		if _, err := s.Get(ctx, "definitely-not-a-uuid"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("non-uuid get: %v", err)
		}
		if _, err := s.BySlug(ctx, "  "); !errors.Is(err, ErrNotFound) {
			t.Fatalf("blank slug: %v", err)
		}
		if _, err := s.BySlug(ctx, "no-such-club"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown slug: %v", err)
		}
	})
}

func TestCov1xBySlugOrLegacy(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, _ bool) {
		ctx := context.Background()
		reg, err := s.Create(ctx, CreateInput{Slug: "registry-club", Name: "Registry"})
		if err != nil {
			t.Fatal(err)
		}
		legacyOrg, err := s.Create(ctx, CreateInput{Name: "Legacy"})
		if err != nil {
			t.Fatal(err)
		}
		never := func(string) (string, error) {
			t.Fatal("legacy lookup must not run when the registry owns the slug")
			return "", nil
		}
		if got, err := s.BySlugOrLegacy(ctx, " REGISTRY-CLUB ", never); err != nil || got.ID != reg.ID {
			t.Fatalf("registry hit: %+v %v", got, err)
		}
		if _, err := s.BySlugOrLegacy(ctx, "missing-club", nil); !errors.Is(err, ErrNotFound) {
			t.Fatalf("nil legacy: %v", err)
		}
		if _, err := s.BySlugOrLegacy(ctx, "missing-club", func(string) (string, error) { return "", errors.New("boom") }); !errors.Is(err, ErrNotFound) {
			t.Fatalf("legacy error maps to not found: %v", err)
		}
		if _, err := s.BySlugOrLegacy(ctx, "missing-club", func(string) (string, error) { return "", nil }); !errors.Is(err, ErrNotFound) {
			t.Fatalf("legacy empty owner: %v", err)
		}
		if _, err := s.BySlugOrLegacy(ctx, "missing-club", func(string) (string, error) { return uuid.NewString(), nil }); !errors.Is(err, ErrNotFound) {
			t.Fatalf("legacy owner unknown to registry: %v", err)
		}
		var asked string
		got, err := s.BySlugOrLegacy(ctx, "Old-Handle", func(slug string) (string, error) { asked = slug; return legacyOrg.ID, nil })
		if err != nil || got.ID != legacyOrg.ID || got.Slug != "old-handle" || asked != "old-handle" {
			t.Fatalf("legacy hit: %+v asked=%q err=%v", got, asked, err)
		}
		// The registry row itself is unchanged by the legacy handle.
		if again, _ := s.Get(ctx, legacyOrg.ID); again.Slug != "" {
			t.Fatalf("legacy slug leaked into registry: %q", again.Slug)
		}
	})
}

func TestCov1xListFilterAndOrder(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, _ bool) {
		ctx := context.Background()
		clock := time.Unix(5000, 0).UTC()
		s.now = func() time.Time { clock = clock.Add(time.Second); return clock }
		a, _ := s.Create(ctx, CreateInput{Name: "A"})
		b, _ := s.Create(ctx, CreateInput{Name: "B"})
		c, _ := s.Create(ctx, CreateInput{Name: "C"})
		if _, err := s.SetStatus(ctx, b.ID, StatusActive); err != nil {
			t.Fatal(err)
		}
		all, err := s.List(ctx, "")
		if err != nil || len(all) != 3 || all[0].ID != a.ID || all[1].ID != b.ID || all[2].ID != c.ID {
			t.Fatalf("all: %+v %v", all, err)
		}
		prov, err := s.List(ctx, StatusProvisioning)
		if err != nil || len(prov) != 2 || prov[0].ID != a.ID || prov[1].ID != c.ID {
			t.Fatalf("provisioning: %+v %v", prov, err)
		}
		none, err := s.List(ctx, StatusDeleted)
		if err != nil || none == nil || len(none) != 0 {
			t.Fatalf("empty list must be non-nil: %v %v", none, err)
		}
	})
}

func TestCov1xListTieBreaksOnID(t *testing.T) {
	s := NewService()
	fixed := time.Unix(9000, 0).UTC()
	s.now = func() time.Time { return fixed }
	ctx := context.Background()
	hi, lo := "ffffffff-ffff-4fff-8fff-ffffffffffff", "00000000-0000-4000-8000-000000000000"
	for _, id := range []string{hi, lo} {
		if _, err := s.Create(ctx, CreateInput{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.List(ctx, "")
	if err != nil || len(list) != 2 || list[0].ID != lo || list[1].ID != hi {
		t.Fatalf("equal timestamps must order by id: %+v %v", list, err)
	}
}

func TestCov1xSetStatusMemoryTimestamps(t *testing.T) {
	s := NewService()
	ctx := context.Background()
	if _, err := s.SetStatus(ctx, uuid.NewString(), StatusActive); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown org: %v", err)
	}
	o, _ := s.Create(ctx, CreateInput{Name: "A"})
	for _, to := range []string{StatusActive, StatusOffboarding} {
		if _, err := s.SetStatus(ctx, o.ID, to); err != nil {
			t.Fatal(err)
		}
	}
	off, _ := s.Get(ctx, o.ID)
	if off.OffboardingAt == nil {
		t.Fatal("offboarding must stamp OffboardingAt")
	}
	stamp := *off.OffboardingAt
	purging, err := s.SetStatus(ctx, o.ID, StatusPurging)
	if err != nil || purging.OffboardingAt == nil || !purging.OffboardingAt.Equal(stamp) {
		t.Fatalf("purging keeps the stamp: %+v %v", purging, err)
	}
	// A cancelled offboarding clears the stamp.
	p, _ := s.Create(ctx, CreateInput{Name: "B"})
	_, _ = s.SetStatus(ctx, p.ID, StatusActive)
	_, _ = s.SetStatus(ctx, p.ID, StatusOffboarding)
	back, err := s.SetStatus(ctx, p.ID, StatusSuspended)
	if err != nil || back.OffboardingAt != nil || back.SuspendedAt == nil {
		t.Fatalf("cancel offboarding: %+v %v", back, err)
	}
	if _, err := s.SetStatus(ctx, o.ID, StatusActive); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("purging -> active must be refused: %v", err)
	}
}

func TestCov1xValidSlug(t *testing.T) {
	for slug, want := range map[string]bool{"aqua-club": true, "abc": true, "ab": false, "Has-Upper": false, "bad slug": false, "": false} {
		if got := ValidSlug(slug); got != want {
			t.Errorf("ValidSlug(%q) = %v, want %v", slug, got, want)
		}
	}
}

func TestCov1xTombstoneMemoryReleasesSlug(t *testing.T) {
	s := NewService()
	ctx := context.Background()
	if err := s.Tombstone(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	o, _ := s.Create(ctx, CreateInput{Slug: "going-away", Name: "Gone"})
	if err := s.Tombstone(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, o.ID)
	if got.Status != StatusDeleted || got.Slug != "" {
		t.Fatalf("tombstone: %+v", got)
	}
	if _, err := s.BySlug(ctx, "going-away"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("slug must be released: %v", err)
	}
	if _, err := s.Create(ctx, CreateInput{Slug: "going-away", Name: "Reuse"}); err != nil {
		t.Fatalf("slug reusable: %v", err)
	}
}

func TestCov1xPostgresLifecycleThroughPurge(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	s := NewServiceWithPool(pool)
	ctx := context.Background()
	o, err := s.Create(ctx, CreateInput{Slug: "purge-me", Name: "Purge"})
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{StatusActive, StatusOffboarding} {
		if _, err := s.SetStatus(ctx, o.ID, to); err != nil {
			t.Fatal(err)
		}
	}
	off, _ := s.Get(ctx, o.ID)
	if off.OffboardingAt == nil {
		t.Fatal("offboarding stamp missing")
	}
	// Not yet purging: tombstone refuses and leaves the org untouched.
	if err := s.Tombstone(ctx, o.ID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("tombstone before purging: %v", err)
	}
	if still, _ := s.Get(ctx, o.ID); still.Slug != "purge-me" || still.Status != StatusOffboarding {
		t.Fatalf("refused tombstone changed the org: %+v", still)
	}
	if _, err := s.SetStatus(ctx, o.ID, StatusPurging); err != nil {
		t.Fatal(err)
	}
	if err := s.Tombstone(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	done, _ := s.Get(ctx, o.ID)
	if done.Status != StatusDeleted || done.Slug != "" {
		t.Fatalf("tombstoned: %+v", done)
	}
	if st, found, err := s.Status(ctx, o.ID); err != nil || !found || st != StatusDeleted {
		t.Fatalf("status cache must be invalidated: %q %v %v", st, found, err)
	}
	// Cancelling an offboarding on Postgres clears the stamp.
	p, _ := s.Create(ctx, CreateInput{Name: "Cancel"})
	_, _ = s.SetStatus(ctx, p.ID, StatusActive)
	_, _ = s.SetStatus(ctx, p.ID, StatusOffboarding)
	back, err := s.SetStatus(ctx, p.ID, StatusSuspended)
	if err != nil || back.OffboardingAt != nil || back.SuspendedAt == nil {
		t.Fatalf("cancel: %+v %v", back, err)
	}
}

func TestCov1xPostgresWriteFailuresSurface(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	s := NewServiceWithPool(pool)
	ctx := context.Background()
	o, _ := s.Create(ctx, CreateInput{Slug: "trigger-club", Name: "T"})
	_, _ = s.SetStatus(ctx, o.ID, StatusActive)
	_, _ = s.SetStatus(ctx, o.ID, StatusOffboarding)
	_, _ = s.SetStatus(ctx, o.ID, StatusPurging)
	live, _ := s.Create(ctx, CreateInput{Name: "Live"})

	// A trigger that silently drops updates simulates a concurrent transition.
	cov1xExec(t, pool, `CREATE FUNCTION cov1x_drop_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`)
	cov1xExec(t, pool, `CREATE TRIGGER cov1x_drop BEFORE UPDATE ON orgs FOR EACH ROW EXECUTE FUNCTION cov1x_drop_update()`)
	if _, err := s.SetStatus(ctx, live.ID, StatusActive); !errors.Is(err, ErrInvalidStatus) || !strings.Contains(err.Error(), "changed concurrently") {
		t.Fatalf("lost race: %v", err)
	}
	if err := s.Tombstone(ctx, o.ID); !errors.Is(err, ErrInvalidStatus) || !strings.Contains(err.Error(), "not being purged") {
		t.Fatalf("tombstone lost race: %v", err)
	}
	cov1xExec(t, pool, `DROP TRIGGER cov1x_drop ON orgs`)

	// A trigger that raises simulates a database failure on the write itself.
	cov1xExec(t, pool, `CREATE FUNCTION cov1x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x write failure'; END $$`)
	cov1xExec(t, pool, `CREATE TRIGGER cov1x_fail BEFORE UPDATE ON orgs FOR EACH ROW EXECUTE FUNCTION cov1x_fail()`)
	if _, err := s.SetStatus(ctx, live.ID, StatusActive); err == nil || !strings.Contains(err.Error(), "cov1x write failure") {
		t.Fatalf("set status write failure: %v", err)
	}
	if err := s.Tombstone(ctx, o.ID); err == nil || !strings.Contains(err.Error(), "cov1x write failure") {
		t.Fatalf("tombstone write failure: %v", err)
	}
	cov1xExec(t, pool, `DROP TRIGGER cov1x_fail ON orgs`)
	if got, _ := s.Get(ctx, live.ID); got.Status != StatusProvisioning {
		t.Fatalf("failed write must not change status: %s", got.Status)
	}

	cov1xExec(t, pool, `CREATE FUNCTION cov1x_fail_domain() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x domain failure'; END $$`)
	cov1xExec(t, pool, `CREATE TRIGGER cov1x_fail_domain BEFORE INSERT OR UPDATE ON org_domains FOR EACH ROW EXECUTE FUNCTION cov1x_fail_domain()`)
	if err := s.AddDomain(ctx, live.ID, "fail.example.org"); err == nil || !strings.Contains(err.Error(), "cov1x domain failure") || errors.Is(err, ErrDomainTaken) {
		t.Fatalf("add domain generic failure: %v", err)
	}
	// The failed insert left no row, so verification takes the not-found path.
	if err := s.VerifyDomain(ctx, live.ID, "fail.example.org"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("verify: %v", err)
	}
}

func TestCov1xPostgresVerifyDomainDatabaseError(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	s := NewServiceWithPool(pool)
	ctx := context.Background()
	o, _ := s.Create(ctx, CreateInput{Name: "D"})
	if err := s.AddDomain(ctx, o.ID, "verify.example.org"); err != nil {
		t.Fatal(err)
	}
	cov1xExec(t, pool, `CREATE FUNCTION cov1x_fail_verify() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x verify failure'; END $$`)
	cov1xExec(t, pool, `CREATE TRIGGER cov1x_fail_verify BEFORE UPDATE ON org_domains FOR EACH ROW EXECUTE FUNCTION cov1x_fail_verify()`)
	if err := s.VerifyDomain(ctx, o.ID, "verify.example.org"); err == nil || !strings.Contains(err.Error(), "cov1x verify failure") {
		t.Fatalf("verify failure: %v", err)
	}
	if _, err := s.ResolveDomain(ctx, "verify.example.org"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed verification must not resolve: %v", err)
	}
}

func TestCov1xStatusCacheAndErrors(t *testing.T) {
	s := NewService()
	ctx := context.Background()
	clock := time.Unix(100, 0)
	s.now = func() time.Time { return clock }
	s.SetStatusCacheTTL(time.Minute)
	id := uuid.NewString()
	// Unknown orgs are cached as not found within the TTL.
	if _, found, err := s.Status(ctx, id); err != nil || found {
		t.Fatalf("unknown: %v %v", found, err)
	}
	o, _ := s.Create(ctx, CreateInput{ID: id, Name: "Late"})
	if _, found, _ := s.Status(ctx, o.ID); found {
		t.Fatal("negative result should be served from cache inside the TTL")
	}
	clock = clock.Add(2 * time.Minute)
	if st, found, _ := s.Status(ctx, o.ID); !found || st != StatusProvisioning {
		t.Fatalf("after TTL: %q %v", st, found)
	}

	dead := cov1xDeadService(t)
	if _, _, err := dead.Status(ctx, uuid.NewString()); err == nil {
		t.Fatal("database failure must surface from Status")
	}
	if ok, err := dead.IsActive(ctx, uuid.NewString()); err == nil || ok {
		t.Fatalf("IsActive must fail closed on database error: %v %v", ok, err)
	}
}

func TestCov1xInvalidateDuringReadSkipsCache(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	s := NewServiceWithPool(pool)
	ctx := context.Background()
	o, _ := s.Create(ctx, CreateInput{Name: "Race"})
	s.SetStatusCacheTTL(time.Hour)

	// Hold the orgs table so Status blocks mid-read while we invalidate.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE orgs IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	type res struct {
		status string
		found  bool
		err    error
	}
	done := make(chan res, 1)
	go func() {
		st, f, e := s.Status(ctx, o.ID)
		done <- res{st, f, e}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE NOT granted AND locktype = 'relation' AND relation = 'orgs'::regclass`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("status read never blocked on the lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.invalidate(o.ID)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil || !r.found || r.status != StatusProvisioning {
		t.Fatalf("status: %+v", r)
	}
	s.mu.RLock()
	_, cached := s.cache[o.ID]
	s.mu.RUnlock()
	if cached {
		t.Fatal("a read that raced an invalidation must not populate the cache")
	}
}

func TestCov1xDatabaseFailuresPropagate(t *testing.T) {
	dead := cov1xDeadService(t)
	ctx := context.Background()
	id := uuid.NewString()
	if _, err := dead.Create(ctx, CreateInput{Slug: "dead-club", Name: "X"}); err == nil || !strings.Contains(err.Error(), "create org") || errors.Is(err, ErrSlugTaken) {
		t.Fatalf("create: %v", err)
	}
	if _, err := dead.Get(ctx, id); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	if _, err := dead.BySlug(ctx, "dead-club"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("by slug: %v", err)
	}
	called := false
	if _, err := dead.BySlugOrLegacy(ctx, "dead-club", func(string) (string, error) { called = true; return id, nil }); err == nil || errors.Is(err, ErrNotFound) || called {
		t.Fatalf("by slug or legacy must not mask a database error: %v called=%v", err, called)
	}
	if _, err := dead.List(ctx, ""); err == nil {
		t.Fatal("list must fail")
	}
	if _, err := dead.ActiveIDs(ctx); err == nil {
		t.Fatal("active ids must fail")
	}
	if _, err := dead.SetStatus(ctx, id, StatusActive); err == nil {
		t.Fatal("set status must fail")
	}
	if err := dead.AddDomain(ctx, id, "dead.example.org"); err == nil {
		t.Fatal("add domain must fail")
	}
	if err := dead.VerifyDomain(ctx, id, "dead.example.org"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("verify domain: %v", err)
	}
	if _, err := dead.ResolveDomain(ctx, "dead.example.org"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve domain: %v", err)
	}
	if err := dead.Tombstone(ctx, id); err == nil || errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("tombstone: %v", err)
	}
}

func TestCov1xCreatePostgresSlugAndIDConflicts(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	s := NewServiceWithPool(pool)
	ctx := context.Background()
	o, err := s.Create(ctx, CreateInput{Slug: "taken-slug", Name: "One"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, CreateInput{Slug: "taken-slug", Name: "Two"}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("slug: %v", err)
	}
	if _, err := s.Create(ctx, CreateInput{ID: o.ID, Name: "Three"}); err == nil || errors.Is(err, ErrSlugTaken) {
		t.Fatalf("id conflict is a generic error: %v", err)
	}
}

func TestCov1xDomainEdgeCases(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, _ bool) {
		ctx := context.Background()
		o, _ := s.Create(ctx, CreateInput{Name: "A"})
		if err := s.AddDomain(ctx, uuid.NewString(), "ghost.example.org"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown org: %v", err)
		}
		if err := s.VerifyDomain(ctx, o.ID, "never-added.example.org"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown domain: %v", err)
		}
		if err := s.AddDomain(ctx, o.ID, "edge.example.org"); err != nil {
			t.Fatal(err)
		}
		if err := s.VerifyDomain(ctx, o.ID, " Edge.Example.org "); err != nil {
			t.Fatal(err)
		}
		if err := s.VerifyDomain(ctx, o.ID, "edge.example.org"); err != nil {
			t.Fatalf("verify is idempotent: %v", err)
		}
		for _, host := range []string{"", "   ", ":443", "unknown.example.org"} {
			if _, err := s.ResolveDomain(ctx, host); !errors.Is(err, ErrNotFound) {
				t.Fatalf("host %q: %v", host, err)
			}
		}
		got, err := s.ResolveDomain(ctx, "edge.example.org")
		if err != nil || got.ID != o.ID {
			t.Fatalf("resolve w/o port: %+v %v", got, err)
		}
		if _, err := s.ResolveDomain(ctx, "[::1]"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("bracketed ipv6 host: %v", err)
		}
	})
}

func TestCov1xCanonicalID(t *testing.T) {
	const canon = "123e4567-e89b-12d3-a456-426614174000"
	cases := map[string]string{
		"  123E4567-E89B-12D3-A456-426614174000 ":       canon,
		"123e4567e89b12d3a456426614174000":              canon,
		"urn:uuid:123E4567-E89B-12D3-A456-426614174000": canon,
		"{123e4567-e89b-12d3-a456-426614174000}":        canon,
		"  Org-Demo ":                                   "org-demo",
		"":                                              "",
	}
	for in, want := range cases {
		if got := CanonicalID(in); got != want {
			t.Errorf("CanonicalID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov1xListSurfacesCorruptRow(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	s := NewServiceWithPool(pool)
	ctx := context.Background()
	// Relax the constraint inside this private schema so a NULL plan can exist.
	cov1xExec(t, pool, `ALTER TABLE orgs ALTER COLUMN plan DROP NOT NULL`)
	cov1xExec(t, pool, `INSERT INTO orgs (id, name, status, plan, region) VALUES ('`+uuid.NewString()+`', 'Corrupt', 'active', NULL, '')`)
	if list, err := s.List(ctx, ""); err == nil || list != nil {
		t.Fatalf("a row that cannot be scanned must fail the listing: %v %v", list, err)
	}
	if _, err := s.ActiveIDs(ctx); err == nil {
		t.Fatal("ActiveIDs must propagate the scan failure")
	}
}
