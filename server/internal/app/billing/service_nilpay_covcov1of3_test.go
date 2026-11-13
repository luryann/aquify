package billing

import "testing"

func TestCov1xRebuildIndexesIgnoresNilPaymentEntries(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.mu.Lock()
	defer svc.mu.Unlock()

	// A nil slot (e.g. from a partially loaded snapshot) must be skipped, not dereferenced.
	svc.payments["orphan"] = []*Payment{nil, {ID: "p-real", InvoiceID: "orphan", OrgID: cov1xOrg, AmountCents: 100, Status: "succeeded", StripePaymentIntentID: "pi_real"}}

	svc.rebuildIndexesLocked()

	if len(svc.paymentsByID) != 1 || svc.paymentsByID["p-real"] == nil {
		t.Fatalf("paymentsByID: %+v", svc.paymentsByID)
	}
	if len(svc.stripeIntentIndex) != 1 || svc.stripeIntentIndex["pi_real"] == nil {
		t.Fatalf("stripeIntentIndex: %+v", svc.stripeIntentIndex)
	}
}
