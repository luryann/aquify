package app

import (
	"errors"
	"fmt"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/members"
)

// collectHouseholdExportData assembles everything the club holds about one
// household for a right-of-access data export: the household and its
// contacts and swimmers (including medical/emergency fields), group
// memberships, registrations and waiver acceptances, and invoices with their
// line items, payments, and refunds.
func collectHouseholdExportData(orgID, householdID string, membersSvc *members.Service, groupsSvc *groups.Service, billingSvc *billing.Service, extra ...householdExportSource) (map[string]any, error) {
	if membersSvc == nil {
		return nil, errors.New("members service is not configured")
	}
	out := map[string]any{}

	var household *members.Household
	for _, h := range membersSvc.SnapshotHouseholds(orgID) {
		if h.ID == householdID {
			hh := h
			household = &hh
			break
		}
	}
	if household == nil {
		return nil, errors.New("household not found")
	}
	out["household"] = household
	out["contacts"] = filter(membersSvc.SnapshotContacts(orgID), func(c members.Contact) bool { return c.HouseholdID == householdID })
	swimmers := filter(membersSvc.SnapshotSwimmers(orgID), func(s members.Swimmer) bool { return s.HouseholdID == householdID })
	out["swimmers"] = swimmers
	swimmerIDs := map[string]bool{}
	for _, s := range swimmers {
		swimmerIDs[s.ID] = true
	}
	out["group_memberships"] = filter(membersSvc.SnapshotSwimmerGroupLinks(orgID), func(l members.SwimmerGroupLink) bool { return swimmerIDs[l.SwimmerID] })

	if groupsSvc != nil {
		registrations := filter(groupsSvc.SnapshotRegistrations(orgID), func(r groups.Registration) bool { return r.HouseholdID == householdID })
		out["registrations"] = registrations
		regIDs := map[string]bool{}
		for _, r := range registrations {
			regIDs[r.ID] = true
		}
		out["waiver_acceptances"] = filter(groupsSvc.SnapshotWaivers(orgID), func(w groups.WaiverAcceptance) bool { return regIDs[w.RegistrationID] })
	}

	if billingSvc != nil {
		invoices := filter(billingSvc.SnapshotInvoices(orgID), func(i billing.Invoice) bool { return i.HouseholdID == householdID })
		out["invoices"] = invoices
		invoiceIDs := map[string]bool{}
		for _, i := range invoices {
			invoiceIDs[i.ID] = true
		}
		out["invoice_items"] = filter(billingSvc.SnapshotInvoiceItems(orgID), func(i billing.InvoiceItem) bool { return invoiceIDs[i.InvoiceID] })
		out["payments"] = filter(billingSvc.SnapshotPayments(orgID), func(p billing.Payment) bool { return invoiceIDs[p.InvoiceID] })
		out["refunds"] = filter(billingSvc.SnapshotRefunds(orgID), func(r billing.Refund) bool { return invoiceIDs[r.InvoiceID] })
	}

	// Every other domain that holds household records contributes through a
	// source; a failing source fails the export rather than silently omitting
	// personal data from a right-of-access response.
	for _, src := range extra {
		data, err := src.collect(orgID, householdID, swimmerIDs)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", src.name, err)
		}
		for _, k := range src.keys {
			if _, ok := data[k]; !ok {
				return nil, fmt.Errorf("export %s: declared key %q missing from source output", src.name, k)
			}
		}
		for k, v := range data {
			out[k] = v
		}
	}
	return out, nil
}

func filter[T any](items []T, keep func(T) bool) []T {
	out := make([]T, 0)
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	return out
}
