package billing

import "testing"

// The snapshot loader must install the same derived state as the DB loader:
// amounts recomputed from payment records and the same lookup indexes.
func TestLoadRuntimeSnapshotRebuildsDerivedState(t *testing.T) {
	data := []byte(`{
		"invoices": {"inv-1": {"id":"inv-1","org_id":"org-1","invoice_number":"INV-1","status":"issued","total_cents":10000,"amount_paid_cents":0,"amount_due_cents":10000}},
		"payments": {"inv-1": [{"id":"pay-1","org_id":"org-1","invoice_id":"inv-1","amount_cents":4000,"method":"stripe","status":"succeeded","stripe_payment_intent_id":"pi_1"}]}
	}`)
	svc := NewService()
	if err := svc.LoadRuntimeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	inv := svc.invoices["inv-1"]
	if inv.AmountPaidCents != 4000 || inv.AmountDueCents != 6000 {
		t.Fatalf("amounts not derived from payments: paid=%d due=%d", inv.AmountPaidCents, inv.AmountDueCents)
	}
	if svc.stripeCreditIndex == nil {
		t.Fatal("stripe credit index not initialised")
	}
	if svc.stripeIntentIndex["pi_1"] == nil || !svc.invoiceNums[invoiceNumKey("org-1", "INV-1")] {
		t.Fatal("lookup indexes missing")
	}
}
