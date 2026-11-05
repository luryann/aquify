package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/invites"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/payments"
	"github.com/luryann/aquify/server/internal/testutil"
)

// domains, invites and payment accounts are isolated by the database,
// and the three pre-auth lookups (host, Stripe account) each reach exactly one row.
func TestRegistryTablesRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)

	reg := orgs.NewServiceWithPool(appPool)
	if err := reg.AddDomain(ctx, orgA, "a.example.test"); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddDomain(ctx, orgB, "b.example.test"); err != nil {
		t.Fatal(err)
	}
	if err := reg.VerifyDomain(ctx, orgB, "b.example.test"); err != nil {
		t.Fatal(err)
	}
	if err := reg.VerifyDomain(ctx, orgA, "b.example.test"); !errors.Is(err, orgs.ErrNotFound) {
		t.Fatalf("club A must not verify club B's domain: %v", err)
	}
	if o, err := reg.ResolveDomain(ctx, "b.example.test"); err != nil || o.ID != orgB {
		t.Fatalf("a verified host resolves to its club before any club is known: %+v %v", o, err)
	}
	if _, err := reg.ResolveDomain(ctx, "a.example.test"); !errors.Is(err, orgs.ErrNotFound) {
		t.Fatalf("an unverified host must not resolve: %v", err)
	}

	inv := invites.NewServiceWithPool(appPool)
	_, codeA, err := inv.Create(ctx, orgA, "", invites.CreateInput{Label: "A", Role: "parent", MaxUses: 5, TTL: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inv.Redeem(ctx, orgB, codeA); !errors.Is(err, invites.ErrInvalidCode) {
		t.Fatalf("club B must not redeem club A's invite: %v", err)
	}
	if got, err := inv.Redeem(ctx, orgA, codeA); err != nil || got.OrgID != orgA || got.Uses != 1 {
		t.Fatalf("club A redeems its own invite: %+v %v", got, err)
	}

	pay := payments.NewServiceWithPool(appPool)
	acctA, acctB := "acct_"+orgA[:8], "acct_"+orgB[:8]
	if _, err := pay.Create(ctx, orgA, acctA); err != nil {
		t.Fatal(err)
	}
	if _, err := pay.Create(ctx, orgB, acctB); err != nil {
		t.Fatal(err)
	}
	if a, err := pay.ByStripeAccount(ctx, acctB); err != nil || a.OrgID != orgB {
		t.Fatalf("a webhook for club B's account is attributed to club B: %+v %v", a, err)
	}
	if a, err := pay.UpdateStatus(ctx, acctB, true, true, true); err != nil || !a.ChargesEnabled || a.OrgID != orgB {
		t.Fatalf("the account status update reaches only that account: %+v %v", a, err)
	}
	if a, err := pay.Get(ctx, orgA); err != nil || a.ChargesEnabled {
		t.Fatalf("club A's account was not touched: %+v %v", a, err)
	}
	for _, table := range []string{"org_domains", "org_invites", "org_payment_accounts"} {
		var n int
		if err := appPool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("unbound session must see nothing in %s: %d %v", table, n, err)
		}
	}
}
