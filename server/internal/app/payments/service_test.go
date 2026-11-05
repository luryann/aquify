package payments

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func runBoth(t *testing.T, fn func(t *testing.T, s *Service)) {
	t.Run("memory", func(t *testing.T) { fn(t, NewService()) })
	t.Run("postgres", func(t *testing.T) { fn(t, NewServiceWithPool(testutil.IsolatedSchemaPool(t))) })
}

func TestAccountsAreOneToOneAndIsolated(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service) {
		ctx := context.Background()
		a, b := uuid.NewString(), uuid.NewString()
		if _, err := s.Get(ctx, a); !errors.Is(err, ErrNotFound) {
			t.Fatalf("none yet: %v", err)
		}
		if _, err := s.Create(ctx, a, "acct_A"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Create(ctx, a, "acct_A2"); !errors.Is(err, ErrAlreadyConnected) {
			t.Fatalf("an org has one account: %v", err)
		}
		if _, err := s.Create(ctx, b, "acct_A"); !errors.Is(err, ErrAccountTaken) {
			t.Fatalf("a Stripe account belongs to one org: %v", err)
		}
		if _, err := s.Create(ctx, b, "acct_B"); err != nil {
			t.Fatal(err)
		}
		got, err := s.ByStripeAccount(ctx, "acct_B")
		if err != nil || got.OrgID != b {
			t.Fatalf("reverse lookup: %+v %v", got, err)
		}
		if upd, err := s.UpdateStatus(ctx, "acct_A", true, true, true); err != nil || !upd.ChargesEnabled {
			t.Fatalf("update: %+v %v", upd, err)
		}
		if other, _ := s.Get(ctx, b); other.ChargesEnabled {
			t.Fatal("updating A must not touch B")
		}
		if _, err := s.UpdateStatus(ctx, "acct_missing", true, true, true); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown account: %v", err)
		}
	})
}
