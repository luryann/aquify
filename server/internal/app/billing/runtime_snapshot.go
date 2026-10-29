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

	s.itemsByID = make(map[string]*InvoiceItem)
	for _, items := range s.items {
		for _, item := range items {
			if item != nil {
				s.itemsByID[item.ID] = item
			}
		}
	}

	s.paymentsByID = make(map[string]*Payment)
	s.stripeIntentIndex = make(map[string]*Payment)
	for _, payments := range s.payments {
		for _, payment := range payments {
			if payment == nil {
				continue
			}
			s.paymentsByID[payment.ID] = payment
			if payment.StripePaymentIntentID != "" {
				s.stripeIntentIndex[payment.StripePaymentIntentID] = payment
			}
		}
	}

	s.refundsByID = make(map[string]*Refund)
	for _, refunds := range s.refunds {
		for _, refund := range refunds {
			if refund != nil {
				s.refundsByID[refund.ID] = refund
			}
		}
	}

	s.invoiceNums = make(map[string]bool)
	for _, invoice := range s.invoices {
		if invoice != nil {
			s.invoiceNums[invoice.OrgID+"|"+invoice.InvoiceNumber] = true
		}
	}
	return nil
}
