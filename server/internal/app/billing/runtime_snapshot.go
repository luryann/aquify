package billing

import "encoding/json"

type runtimeSnapshot struct {
	Invoices      map[string]*Invoice             `json:"invoices"`
	Items         map[string][]*InvoiceItem       `json:"items"`
	Payments      map[string][]*Payment           `json:"payments"`
	Refunds       map[string][]*Refund            `json:"refunds"`
	Ledger        []*LedgerEvent                  `json:"ledger"`
	WebhookEvents map[string]*WebhookEventRecord  `json:"webhook_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Invoices:      cloneInvoices(s.invoices),
		Items:         cloneInvoiceItemLists(s.items),
		Payments:      clonePaymentLists(s.payments),
		Refunds:       cloneRefundLists(s.refunds),
		Ledger:        cloneLedgerEvents(s.ledger),
		WebhookEvents: cloneWebhookEvents(s.webhookEvents),
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
	s.invoices = cloneInvoices(snapshot.Invoices)
	s.items = cloneInvoiceItemLists(snapshot.Items)
	s.payments = clonePaymentLists(snapshot.Payments)
	s.refunds = cloneRefundLists(snapshot.Refunds)
	s.ledger = cloneLedgerEvents(snapshot.Ledger)
	s.webhookEvents = cloneWebhookEvents(snapshot.WebhookEvents)

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

func cloneInvoices(in map[string]*Invoice) map[string]*Invoice {
	out := make(map[string]*Invoice, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = nil
			continue
		}
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneInvoiceItemLists(in map[string][]*InvoiceItem) map[string][]*InvoiceItem {
	out := make(map[string][]*InvoiceItem, len(in))
	for k, values := range in {
		cloned := make([]*InvoiceItem, 0, len(values))
		for _, value := range values {
			if value == nil {
				cloned = append(cloned, nil)
				continue
			}
			cp := *value
			cloned = append(cloned, &cp)
		}
		out[k] = cloned
	}
	return out
}

func clonePaymentLists(in map[string][]*Payment) map[string][]*Payment {
	out := make(map[string][]*Payment, len(in))
	for k, values := range in {
		cloned := make([]*Payment, 0, len(values))
		for _, value := range values {
			if value == nil {
				cloned = append(cloned, nil)
				continue
			}
			cp := *value
			cloned = append(cloned, &cp)
		}
		out[k] = cloned
	}
	return out
}

func cloneRefundLists(in map[string][]*Refund) map[string][]*Refund {
	out := make(map[string][]*Refund, len(in))
	for k, values := range in {
		cloned := make([]*Refund, 0, len(values))
		for _, value := range values {
			if value == nil {
				cloned = append(cloned, nil)
				continue
			}
			cp := *value
			cloned = append(cloned, &cp)
		}
		out[k] = cloned
	}
	return out
}

func cloneLedgerEvents(in []*LedgerEvent) []*LedgerEvent {
	out := make([]*LedgerEvent, 0, len(in))
	for _, value := range in {
		if value == nil {
			out = append(out, nil)
			continue
		}
		cp := *value
		out = append(out, &cp)
	}
	return out
}

func cloneWebhookEvents(in map[string]*WebhookEventRecord) map[string]*WebhookEventRecord {
	out := make(map[string]*WebhookEventRecord, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = nil
			continue
		}
		cp := *v
		out[k] = &cp
	}
	return out
}
