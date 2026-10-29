package billing

import (
	"encoding/json"

	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
)

type runtimeSnapshot struct {
	Invoices        map[string]*Invoice                 `json:"invoices"`
	Items           map[string][]*InvoiceItem           `json:"items"`
	Payments        map[string][]*Payment               `json:"payments"`
	Refunds         map[string][]*Refund                `json:"refunds"`
	Ledger          []*LedgerEvent                      `json:"ledger"`
	WebhookEvents   map[string]*WebhookEventRecord      `json:"webhook_events"`
	AutopayProfiles map[string]*HouseholdAutopayProfile `json:"autopay_profiles"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Invoices:        runtimepersist.ClonePtrMap(s.invoices),
		Items:           runtimepersist.ClonePtrListMap(s.items),
		Payments:        runtimepersist.ClonePtrListMap(s.payments),
		Refunds:         runtimepersist.ClonePtrListMap(s.refunds),
		Ledger:          runtimepersist.ClonePtrs(s.ledger),
		WebhookEvents:   runtimepersist.ClonePtrMap(s.webhookEvents),
		AutopayProfiles: runtimepersist.ClonePtrMap(s.autopayProfiles),
	}
	return json.Marshal(snapshot)
}

func (s *Service) LoadRuntimeSnapshot(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	var snapshot runtimeSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.invoices = runtimepersist.ClonePtrMap(snapshot.Invoices)
	s.items = runtimepersist.ClonePtrListMap(snapshot.Items)
	s.payments = runtimepersist.ClonePtrListMap(snapshot.Payments)
	s.refunds = runtimepersist.ClonePtrListMap(snapshot.Refunds)
	s.ledger = runtimepersist.ClonePtrs(snapshot.Ledger)
	s.webhookEvents = runtimepersist.ClonePtrMap(snapshot.WebhookEvents)
	s.autopayProfiles = runtimepersist.ClonePtrMap(snapshot.AutopayProfiles)

	s.rebuildIndexesLocked()
	return nil
}
