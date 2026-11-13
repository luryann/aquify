package payments

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov1xPaymentsLookupMissesAndWhitespace(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service) {
		ctx := context.Background()
		if _, err := s.ByStripeAccount(ctx, "   "); !errors.Is(err, ErrNotFound) {
			t.Fatalf("blank account id must be not found: %v", err)
		}
		if _, err := s.ByStripeAccount(ctx, "acct_cov1x_unknown"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown account id must be not found: %v", err)
		}
		org := uuid.NewString()
		created, err := s.Create(ctx, "  "+org+" ", " acct_cov1x_trim ")
		if err != nil {
			t.Fatal(err)
		}
		if created.OrgID != org || created.StripeAccountID != "acct_cov1x_trim" {
			t.Fatalf("inputs must be trimmed: %+v", created)
		}
		if created.ChargesEnabled || created.PayoutsEnabled || created.DetailsSubmitted {
			t.Fatalf("flags start false: %+v", created)
		}
		got, err := s.ByStripeAccount(ctx, "  acct_cov1x_trim ")
		if err != nil || got.OrgID != org {
			t.Fatalf("trimmed reverse lookup: %+v %v", got, err)
		}
	})
}

func TestCov1xPaymentsCreateRequiresBothIDs(t *testing.T) {
	runBoth(t, func(t *testing.T, s *Service) {
		ctx := context.Background()
		for _, c := range [][2]string{{"", "acct_x"}, {uuid.NewString(), "  "}, {"", ""}} {
			_, err := s.Create(ctx, c[0], c[1])
			if err == nil || !strings.Contains(err.Error(), "required") {
				t.Fatalf("Create(%q,%q) must be rejected, got %v", c[0], c[1], err)
			}
		}
	})
}

func TestCov1xPaymentsPostgresWrapsUnexpectedErrors(t *testing.T) {
	s := NewServiceWithPool(testCov1xPool(t))
	_, err := s.Create(context.Background(), "not-a-uuid", "acct_cov1x_bad")
	if err == nil || !strings.Contains(err.Error(), "create payment account") {
		t.Fatalf("non-uuid org must surface a wrapped error, got %v", err)
	}
	if errors.Is(err, ErrAlreadyConnected) || errors.Is(err, ErrAccountTaken) {
		t.Fatalf("must not be mapped to a domain error: %v", err)
	}
}

func testCov1xPool(t *testing.T) *pgxpool.Pool { return testutil.IsolatedSchemaPool(t) }
