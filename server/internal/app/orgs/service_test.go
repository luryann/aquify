package orgs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

// runBoth runs fn against the in-memory registry and, when DATABASE_URL is set,
// against Postgres, so both implementations keep the same behaviour.
func runBoth(t *testing.T, fn func(t *testing.T, s *Service, strict bool)) {
	t.Run("memory", func(t *testing.T) { fn(t, NewService(), false) })
	t.Run("postgres", func(t *testing.T) {
		var pool *pgxpool.Pool = testutil.IsolatedSchemaPoolStrict(t)
		fn(t, NewServiceWithPool(pool), true)
	})
}

func TestLifecycleAndSlugRules(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, _ bool) {
		ctx := context.Background()
		o, err := s.Create(ctx, CreateInput{Slug: "Aqua-Club", Name: "Aqua"})
		if err != nil || o.Status != StatusProvisioning || o.Slug != "aqua-club" {
			t.Fatalf("create: %+v %v", o, err)
		}
		if _, err := s.Create(ctx, CreateInput{Slug: "aqua-club", Name: "Dup"}); !errors.Is(err, ErrSlugTaken) {
			t.Fatalf("dup slug: %v", err)
		}
		if _, err := s.Create(ctx, CreateInput{Slug: "x", Name: "Bad"}); !errors.Is(err, ErrInvalidSlug) {
			t.Fatalf("bad slug: %v", err)
		}
		if _, err := s.SetStatus(ctx, o.ID, StatusSuspended); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("provisioning -> suspended must be refused: %v", err)
		}
		if _, err := s.SetStatus(ctx, o.ID, StatusActive); err != nil {
			t.Fatal(err)
		}
		sus, err := s.SetStatus(ctx, o.ID, StatusSuspended)
		if err != nil || sus.SuspendedAt == nil {
			t.Fatalf("suspend: %+v %v", sus, err)
		}
		act, err := s.SetStatus(ctx, o.ID, StatusActive)
		if err != nil || act.SuspendedAt != nil {
			t.Fatalf("resume: %+v %v", act, err)
		}
		if got, err := s.BySlug(ctx, "AQUA-CLUB"); err != nil || got.ID != o.ID {
			t.Fatalf("by slug: %+v %v", got, err)
		}
		if _, err := s.Get(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown org: %v", err)
		}
	})
}

func TestIsActiveFollowsStatusAndFailsClosedWithDB(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, strict bool) {
		ctx := context.Background()
		s.SetStatusCacheTTL(0)
		unknown, err := s.IsActive(ctx, uuid.NewString())
		if err != nil || unknown != !strict {
			t.Fatalf("unknown org active=%v (want %v) err=%v", unknown, !strict, err)
		}
		o, _ := s.Create(ctx, CreateInput{Name: "A"})
		if ok, _ := s.IsActive(ctx, o.ID); ok {
			t.Fatal("provisioning org must not be active")
		}
		_, _ = s.SetStatus(ctx, o.ID, StatusActive)
		if ok, _ := s.IsActive(ctx, o.ID); !ok {
			t.Fatal("active org must be active")
		}
		_, _ = s.SetStatus(ctx, o.ID, StatusSuspended)
		if ok, _ := s.IsActive(ctx, o.ID); ok {
			t.Fatal("suspended org must not be active")
		}
	})
}

func TestStatusCacheExpiresAndInvalidatesOnChange(t *testing.T) {
	s := NewService()
	clock := time.Unix(1000, 0)
	s.now = func() time.Time { return clock }
	s.SetStatusCacheTTL(10 * time.Second)
	ctx := context.Background()
	o, _ := s.Create(ctx, CreateInput{Name: "A"})
	_, _ = s.SetStatus(ctx, o.ID, StatusActive)
	if ok, _ := s.IsActive(ctx, o.ID); !ok {
		t.Fatal("expected active")
	}
	// SetStatus on this instance invalidates immediately.
	_, _ = s.SetStatus(ctx, o.ID, StatusSuspended)
	if ok, _ := s.IsActive(ctx, o.ID); ok {
		t.Fatal("local suspension must be visible immediately")
	}
	// Another instance's change becomes visible once the TTL elapses.
	s.mu.Lock()
	s.mem[o.ID].Status = StatusActive
	s.mu.Unlock()
	if ok, _ := s.IsActive(ctx, o.ID); ok {
		t.Fatal("cached value should still be served inside the TTL")
	}
	clock = clock.Add(11 * time.Second)
	if ok, _ := s.IsActive(ctx, o.ID); !ok {
		t.Fatal("cache must refresh after the TTL")
	}
}

func TestDomainsResolveOnlyWhenVerified(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, _ bool) {
		ctx := context.Background()
		a, _ := s.Create(ctx, CreateInput{Name: "A"})
		b, _ := s.Create(ctx, CreateInput{Name: "B"})
		if err := s.AddDomain(ctx, a.ID, "Swim.Example.org"); err != nil {
			t.Fatal(err)
		}
		if err := s.AddDomain(ctx, b.ID, "swim.example.org"); !errors.Is(err, ErrDomainTaken) {
			t.Fatalf("domain collision: %v", err)
		}
		if err := s.AddDomain(ctx, a.ID, "not a domain"); !errors.Is(err, ErrInvalidDomain) {
			t.Fatalf("invalid domain: %v", err)
		}
		if _, err := s.ResolveDomain(ctx, "swim.example.org"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unverified domain must not resolve: %v", err)
		}
		if err := s.VerifyDomain(ctx, b.ID, "swim.example.org"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("another org must not verify it: %v", err)
		}
		if err := s.VerifyDomain(ctx, a.ID, "swim.example.org"); err != nil {
			t.Fatal(err)
		}
		got, err := s.ResolveDomain(ctx, "SWIM.example.org:443")
		if err != nil || got.ID != a.ID {
			t.Fatalf("resolve: %+v %v", got, err)
		}
	})
}

func TestActiveIDsListsOnlyActiveOrgs(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, _ bool) {
		ctx := context.Background()
		a, _ := s.Create(ctx, CreateInput{Name: "A"})
		b, _ := s.Create(ctx, CreateInput{Name: "B"})
		_, _ = s.Create(ctx, CreateInput{Name: "C"})
		_, _ = s.SetStatus(ctx, a.ID, StatusActive)
		_, _ = s.SetStatus(ctx, b.ID, StatusActive)
		_, _ = s.SetStatus(ctx, b.ID, StatusSuspended)
		ids, err := s.ActiveIDs(ctx)
		if err != nil || len(ids) != 1 || ids[0] != a.ID {
			t.Fatalf("active ids: %v %v", ids, err)
		}
	})
}
