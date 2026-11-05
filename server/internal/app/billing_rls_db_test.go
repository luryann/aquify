package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// billing is written under each club's binding, a restarted instance
// reloads every club, provider-event dedupe still works across clubs, and the database
// refuses unbound reads.
func TestBillingRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	mem := members.NewServiceWithPool(appPool)
	svc := billing.NewServiceWithPool(appPool)
	staff := func(org string) *appmiddleware.Principal {
		id := uuid.NewString()
		if _, err := admin.Exec(ctx, `INSERT INTO users (id, org_id, email, oidc_subject, display_name, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, $1, 'Admin', 'active', now(), now())`, id, org, id+"@example.test"); err != nil {
			t.Fatal(err)
		}
		return &appmiddleware.Principal{UserID: id, OrgID: org, Roles: []string{"admin"}}
	}
	for _, org := range []string{orgA, orgB} {
		p := staff(org)
		hh, err := mem.CreateHousehold(p, members.CreateHouseholdInput{OrgID: org, HouseholdName: "H"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CreateInvoice(org, p.UserID, p, billing.CreateInvoiceInput{HouseholdID: hh.ID, InvoiceNumber: "INV-" + org[:6]}); err != nil {
			t.Fatal(err)
		}
	}
	restarted := billing.NewServiceWithPool(appPool)
	if err := restarted.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for _, org := range []string{orgA, orgB} {
		got, err := restarted.ListInvoices(org, staff(org), billing.ListInvoicesFilters{})
		if err != nil || len(got) != 1 || got[0].OrgID != org || got[0].InvoiceNumber != "INV-"+org[:6] {
			t.Fatalf("club %s sees only its invoice after a restart: %+v %v", org, got, err)
		}
	}

	// Provider-event dedupe is global: a second delivery of the same event is a duplicate,
	// whichever club claims it, and the ledger reads back per club.
	evt := "evt_" + uuid.NewString()
	if isNew, err := svc.RecordWebhookEvent("stripe", orgA, evt, "payment_intent.succeeded", []byte(`{}`)); err != nil || !isNew {
		t.Fatalf("first delivery is new: %v %v", isNew, err)
	}
	if err := svc.MarkWebhookProcessed("stripe", evt, nil, false); err != nil {
		t.Fatal(err)
	}
	if isNew, err := restarted.RecordWebhookEvent("stripe", orgA, evt, "payment_intent.succeeded", []byte(`{}`)); err != nil || isNew {
		t.Fatalf("a repeated delivery is not new (another instance): %v %v", isNew, err)
	}
	if got, err := svc.ListWebhookEvents(orgA, staff(orgA)); err != nil || len(got) != 1 {
		t.Fatalf("club A sees its event: %+v %v", got, err)
	}
	if got, err := svc.ListWebhookEvents(orgB, staff(orgB)); err != nil || len(got) != 0 {
		t.Fatalf("club B must not see club A's event: %+v %v", got, err)
	}

	for _, table := range []string{"invoices", "ledger_events", "webhook_events"} {
		var n int
		if err := appPool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("unbound session must see nothing in %s: %d %v", table, n, err)
		}
	}
}
