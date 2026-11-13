package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/testutil"
)

func cov1xLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestCov1xRegisterPlatformOrgCreatesActiveOrgOnce(t *testing.T) {
	t.Parallel()
	registry := orgs.NewService()
	logger, logs := cov1xLogger()
	id := uuid.NewString()

	registerPlatformOrg(logger, registry, id)
	got, err := registry.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != orgs.StatusActive || got.Slug != "platform" || got.Name != "Aquify Platform" {
		t.Fatalf("unexpected platform org %+v", got)
	}
	// Idempotent: a second call neither fails nor logs.
	registerPlatformOrg(logger, registry, id)
	if logs.Len() != 0 {
		t.Fatalf("unexpected logs: %s", logs.String())
	}
}

func TestCov1xRegisterPlatformOrgLogsRegistrationFailure(t *testing.T) {
	t.Parallel()
	registry := orgs.NewService()
	logger, logs := cov1xLogger()
	registerPlatformOrg(logger, registry, "not-a-uuid")
	if !strings.Contains(logs.String(), "platform org registration failed") {
		t.Fatalf("expected a registration failure log, got %q", logs.String())
	}
	if list, _ := registry.List(context.Background(), ""); len(list) != 0 {
		t.Fatalf("no org should have been created: %+v", list)
	}
}

func TestCov1xRegisterPlatformOrgLogsLookupFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	closed := cov1xClosedCopy(t, pool)
	logger, logs := cov1xLogger()
	registerPlatformOrg(logger, orgs.NewServiceWithPool(closed), uuid.NewString())
	if !strings.Contains(logs.String(), "platform org lookup failed") {
		t.Fatalf("expected a lookup failure log, got %q", logs.String())
	}
}

// cov1xClosedCopy returns a pool to the same database that is already closed,
// so every statement on it fails.
func cov1xClosedCopy(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	p, err := pgxpool.NewWithConfig(context.Background(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
	return p
}

func TestCov1xRegisterPlatformOrgLogsActivationFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE FUNCTION cov1x_block_org_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'orgs are frozen'; END $$`,
		`CREATE TRIGGER cov1x_freeze BEFORE UPDATE ON orgs FOR EACH ROW EXECUTE FUNCTION cov1x_block_org_update()`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	registry := orgs.NewServiceWithPool(pool)
	logger, logs := cov1xLogger()
	id := uuid.NewString()
	registerPlatformOrg(logger, registry, id)
	if !strings.Contains(logs.String(), "platform org activation failed") {
		t.Fatalf("expected an activation failure log, got %q", logs.String())
	}
	got, err := registry.Get(ctx, id)
	if err != nil || got.Status != orgs.StatusProvisioning {
		t.Fatalf("org should remain provisioning: %+v %v", got, err)
	}
}

func TestCov1xClubResolverFindBySlug(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registry := orgs.NewService()
	settings := orgsettings.NewService()
	resolver := clubResolver{registry: registry, settings: settings}

	active, err := registry.Create(ctx, orgs.CreateInput{Slug: "dolphins", Name: "Dolphins"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.SetStatus(ctx, active.ID, orgs.StatusActive); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Create(ctx, orgs.CreateInput{Slug: "sharks", Name: "Sharks"}); err != nil {
		t.Fatal(err)
	}

	got, err := resolver.FindBySlug("Dolphins")
	if err != nil || got == nil || got.OrgID != active.ID || got.PublicSlug != "dolphins" {
		t.Fatalf("active registry club: %+v %v", got, err)
	}
	// A club that is not active accepts no sign-ups.
	if got, err := resolver.FindBySlug("sharks"); err != nil || got != nil {
		t.Fatalf("inactive club must resolve to nil, got %+v %v", got, err)
	}
	if got, err := resolver.FindBySlug("nope"); err != nil || got != nil {
		t.Fatalf("unknown slug must resolve to nil, got %+v %v", got, err)
	}

	// Legacy org_settings slug for an org that is in the registry.
	legacy, err := registry.Create(ctx, orgs.CreateInput{Name: "Legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.SetStatus(ctx, legacy.ID, orgs.StatusActive); err != nil {
		t.Fatal(err)
	}
	slug := "old-club"
	if _, err := settings.Update(legacy.ID, orgsettings.Updates{PublicSlug: &slug}, "cov1x"); err != nil {
		t.Fatal(err)
	}
	got, err = resolver.FindBySlug("old-club")
	if err != nil || got == nil || got.OrgID != legacy.ID || got.PublicSlug != "old-club" {
		t.Fatalf("legacy slug: %+v %v", got, err)
	}
}

func TestCov1xClubResolverPropagatesRegistryErrors(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	resolver := clubResolver{registry: orgs.NewServiceWithPool(cov1xClosedCopy(t, pool)), settings: orgsettings.NewService()}
	got, err := resolver.FindBySlug("anything")
	if err == nil || got != nil {
		t.Fatalf("a registry outage must be an error, not 'club not found': %+v %v", got, err)
	}
}
