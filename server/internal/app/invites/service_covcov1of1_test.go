package invites

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func cov1xCanceled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestCov1xCreateRejectsTTLOverCeiling(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, orgA, _ string) {
		_, _, err := s.Create(context.Background(), orgA, "", CreateInput{Role: "parent", TTL: maxTTL + 1})
		if err == nil || !strings.Contains(err.Error(), "ttl must be at most 90 days") {
			t.Fatalf("want ttl ceiling error, got %v", err)
		}
	})
}

func TestCov1xCreateRecordsCreatedBy(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, orgA, _ string) {
		ctx := context.Background()
		by := uuid.NewString()
		if s.pool != nil {
			if _, err := s.pool.Exec(ctx, `INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1::uuid, $2::uuid, $3, $4)`,
				by, orgA, by+"@example.test", "sub-"+by); err != nil {
				t.Fatalf("seed creator: %v", err)
			}
		}
		inv, _, err := s.Create(ctx, orgA, by, CreateInput{Role: "coach", Label: "  Staff  "})
		if err != nil {
			t.Fatal(err)
		}
		if inv.CreatedBy != by || inv.Label != "Staff" {
			t.Fatalf("created invite: %+v", inv)
		}
		list, err := s.List(ctx, orgA)
		if err != nil || len(list) != 1 || list[0].CreatedBy != by || list[0].Label != "Staff" {
			t.Fatalf("list: %+v %v", list, err)
		}
	})
}

func TestCov1xPostgresFailurePaths(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	s := NewServiceWithPool(pool)
	org := uuid.NewString()

	if _, _, err := s.Create(cov1xCanceled(), org, "", CreateInput{Role: "parent"}); err == nil || !strings.Contains(err.Error(), "create invite") {
		t.Fatalf("create with canceled ctx: %v", err)
	}
	if _, err := s.List(cov1xCanceled(), org); err == nil {
		t.Fatal("list with canceled ctx must fail")
	}
	if err := s.Revoke(cov1xCanceled(), org, uuid.NewString()); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("revoke with canceled ctx must surface the db error: %v", err)
	}
	if err := s.Revoke(context.Background(), org, "not-a-uuid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed id: %v", err)
	}
	if err := s.Revoke(context.Background(), org, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestCov1xListSurfacesScanError(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	s := NewServiceWithPool(pool)
	ctx := context.Background()
	org := uuid.NewString()
	if _, _, err := s.Create(ctx, org, "", CreateInput{Role: "parent", Label: "x"}); err != nil {
		t.Fatal(err)
	}
	// Change expires_at to text so scanning it into a time.Time fails.
	if _, err := pool.Exec(ctx, `ALTER TABLE org_invites ALTER COLUMN expires_at TYPE text USING 'soon'`); err != nil {
		t.Fatalf("alter: %v", err)
	}
	if list, err := s.List(ctx, org); err == nil {
		t.Fatalf("expected scan error, got %+v", list)
	}
}
