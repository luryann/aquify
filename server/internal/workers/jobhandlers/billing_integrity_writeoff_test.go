package jobhandlers

import "testing"

// A written-off invoice is exempt from the "total - paid = due" rule: the debt was forgiven, so
// an unpaid invoice with a non-zero total is consistent. The billing service also forces the
// balance of a written-off invoice to zero whenever it loads state, so the integrity job's
// "written off with nonzero balance" guard cannot be reached through a real load.
func TestNightlyIntegrityAcceptsWrittenOffInvoices(t *testing.T) {
	t.Parallel()

	for name, dueCents := range map[string]int{
		"stored with no balance":                      0,
		"stored with a stale balance, zeroed on load": 250,
	} {
		dueCents := dueCents
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			st := cov1xGoodState()
			inv := st.Invoices["inv-1"]
			inv.Status = "written_off"
			inv.AmountDueCents = dueCents // total 1000, nothing paid
			st.Invoices["inv-1"] = inv
			if err := cov1xRunIntegrity(t, st); err != nil {
				t.Fatalf("a written-off invoice must pass the integrity check: %v", err)
			}
		})
	}
}
