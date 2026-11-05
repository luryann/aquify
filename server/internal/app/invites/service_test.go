package invites

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func runBoth(t *testing.T, fn func(t *testing.T, s *Service, orgA, orgB string)) {
	t.Run("memory", func(t *testing.T) { fn(t, NewService(), uuid.NewString(), uuid.NewString()) })
	t.Run("postgres", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		fn(t, NewServiceWithPool(pool), uuid.NewString(), uuid.NewString())
	})
}

func TestInviteIsBoundToOrgRoleAndLimits(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, orgA, orgB string) {
		ctx := context.Background()
		inv, code, err := s.Create(ctx, orgA, "", CreateInput{Role: "parent", MaxUses: 2, Label: "Spring"})
		if err != nil || len(code) != 14 {
			t.Fatalf("create: %v code=%q", err, code)
		}
		if _, err := s.Redeem(ctx, orgB, code); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("a code must not redeem in another org: %v", err)
		}
		got, err := s.Redeem(ctx, orgA, "  "+code[:4]+" "+code[5:]+" ")
		if err != nil || got.ID != inv.ID || got.Role != "parent" || got.Uses != 1 {
			t.Fatalf("redeem (normalised input): %+v %v", got, err)
		}
		if _, err := s.Redeem(ctx, orgA, code); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Redeem(ctx, orgA, code); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("exhausted code: %v", err)
		}
		if _, err := s.Redeem(ctx, orgA, ""); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("empty code: %v", err)
		}
	})
}

func TestInviteRevokeExpireReleaseAndValidation(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, orgA, orgB string) {
		ctx := context.Background()
		if _, _, err := s.Create(ctx, orgA, "", CreateInput{Role: "admin"}); !errors.Is(err, ErrInvalidRole) {
			t.Fatalf("admin invites must be impossible: %v", err)
		}
		if _, _, err := s.Create(ctx, orgA, "", CreateInput{Role: "parent", MaxUses: 100000}); err == nil {
			t.Fatal("max_uses ceiling")
		}
		inv, code, _ := s.Create(ctx, orgA, "", CreateInput{Role: "coach"})
		if err := s.Revoke(ctx, orgB, inv.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("another org must not revoke: %v", err)
		}
		if err := s.Revoke(ctx, orgA, inv.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Redeem(ctx, orgA, code); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("revoked: %v", err)
		}

		short, scode, _ := s.Create(ctx, orgA, "", CreateInput{Role: "parent", TTL: time.Second})
		_ = short
		s.now = func() time.Time { return time.Now().Add(time.Minute) }
		if _, err := s.Redeem(ctx, orgA, scode); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expired: %v", err)
		}
		s.now = time.Now

		one, ocode, _ := s.Create(ctx, orgA, "", CreateInput{Role: "swimmer"})
		got, _ := s.Redeem(ctx, orgA, ocode)
		if _, err := s.Redeem(ctx, orgA, ocode); !errors.Is(err, ErrInvalidCode) {
			t.Fatal("single use must be spent")
		}
		if err := s.Release(ctx, orgA, got.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Redeem(ctx, orgA, ocode); err != nil {
			t.Fatalf("released use must be redeemable again: %v", err)
		}
		_ = one
		list, _ := s.List(ctx, orgA)
		if len(list) != 3 {
			t.Fatalf("list: %d", len(list))
		}
		if other, _ := s.List(ctx, orgB); len(other) != 0 {
			t.Fatal("other org must see nothing")
		}
	})
}

func TestConcurrentRedeemNeverExceedsMaxUses(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service, orgA, _ string) {
		ctx := context.Background()
		_, code, _ := s.Create(ctx, orgA, "", CreateInput{Role: "parent", MaxUses: 3})
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok := 0
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.Redeem(ctx, orgA, code); err == nil {
					mu.Lock()
					ok++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if ok != 3 {
			t.Fatalf("%d redemptions succeeded, want exactly 3", ok)
		}
	})
}
