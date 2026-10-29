package app

import (
	"context"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/events"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/volunteer"
	"github.com/dareaquatics/aquify/server/internal/app/wellness"
)

// householdExportSource contributes one domain's household records to a
// right-of-access data export. collect returns top-level export keys.
type householdExportSource struct {
	name    string
	keys    []string // export keys the source always emits (empty slice when no rows)
	collect func(orgID, householdID string, swimmerIDs map[string]bool) (map[string]any, error)
}

// householdExportDeps are the domain services (beyond members, groups and
// billing, which the collector reads directly) that hold household records.
type householdExportDeps struct {
	meets     *meets.Service
	volunteer *volunteer.Service
	wellness  *wellness.Service
	events    *events.Service
	documents *documents.Service
	comms     *comms.Service
}

// exportSystemPrincipal is the trusted identity for export reads; the caller
// (compliance job) has already authorized the request.
func exportSystemPrincipal(orgID string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "system:data-export", OrgID: orgID, Roles: []string{"admin"}}
}

// buildHouseholdExportSources returns one source per wired domain. Add a
// domain that stores household or swimmer records here; the coverage test in
// household_export_test.go fails if a required key stops being emitted.
func buildHouseholdExportSources(d householdExportDeps) []householdExportSource {
	var out []householdExportSource

	if d.meets != nil {
		out = append(out, householdExportSource{
			name: "meets", keys: []string{"meet_commitments", "meet_event_selections", "meet_entries"},
			collect: func(orgID, householdID string, swimmerIDs map[string]bool) (map[string]any, error) {
				sys := exportSystemPrincipal(orgID)
				commitments := []meets.Commitment{}
				selections := []meets.EventSelection{}
				entries := []meets.Entry{}
				for _, m := range d.meets.SnapshotMeets(orgID) {
					list, err := d.meets.ListCommitments(sys, orgID, m.ID)
					if err != nil {
						return nil, err
					}
					for _, c := range list {
						if c.HouseholdID != householdID {
							continue
						}
						commitments = append(commitments, c)
						sel, err := d.meets.ListCommitmentEventSelections(sys, orgID, m.ID, c.ID)
						if err != nil {
							return nil, err
						}
						selections = append(selections, sel...)
					}
					es, err := d.meets.ListEntries(sys, orgID, m.ID)
					if err != nil {
						return nil, err
					}
					for _, e := range es {
						if swimmerIDs[e.SwimmerID] {
							entries = append(entries, e)
						}
					}
				}
				return map[string]any{"meet_commitments": commitments, "meet_event_selections": selections, "meet_entries": entries}, nil
			},
		})
	}

	if d.volunteer != nil {
		out = append(out, householdExportSource{
			name: "volunteer", keys: []string{"volunteer_signups", "volunteer_credits", "volunteer_buyouts"},
			collect: func(orgID, householdID string, _ map[string]bool) (map[string]any, error) {
				ctx := context.Background()
				sys := exportSystemPrincipal(orgID)
				signups, err := d.volunteer.ListSignups(ctx, sys, orgID, volunteer.SignupFilter{HouseholdID: householdID})
				if err != nil {
					return nil, err
				}
				credits, err := d.volunteer.ListCredits(ctx, sys, orgID, "", householdID)
				if err != nil {
					return nil, err
				}
				allBuyouts, err := d.volunteer.ListBuyouts(ctx, sys, orgID, "")
				if err != nil {
					return nil, err
				}
				buyouts := []*volunteer.Buyout{}
				for _, b := range allBuyouts {
					if b.HouseholdID == householdID {
						buyouts = append(buyouts, b)
					}
				}
				if signups == nil {
					signups = []volunteer.SignupView{}
				}
				if credits == nil {
					credits = []*volunteer.Credit{}
				}
				return map[string]any{"volunteer_signups": signups, "volunteer_credits": credits, "volunteer_buyouts": buyouts}, nil
			},
		})
	}

	if d.wellness != nil {
		out = append(out, householdExportSource{
			name: "wellness", keys: []string{"wellness_checkins"},
			collect: func(orgID, householdID string, swimmerIDs map[string]bool) (map[string]any, error) {
				ids := make([]string, 0, len(swimmerIDs))
				for id := range swimmerIDs {
					ids = append(ids, id)
				}
				rows, err := d.wellness.List(orgID, ids, "0001-01-01", "9999-12-31")
				if err != nil {
					return nil, err
				}
				if rows == nil {
					rows = []*wellness.CheckIn{}
				}
				return map[string]any{"wellness_checkins": rows}, nil
			},
		})
	}

	if d.events != nil {
		out = append(out, householdExportSource{
			name: "club events", keys: []string{"club_event_rsvps"},
			collect: func(orgID, householdID string, _ map[string]bool) (map[string]any, error) {
				evs, err := d.events.List(orgID, events.ListFilter{})
				if err != nil {
					return nil, err
				}
				ids := make([]string, 0, len(evs))
				for _, e := range evs {
					ids = append(ids, e.ID)
				}
				byEvent, err := d.events.ListRSVPsForEvents(orgID, ids)
				if err != nil {
					return nil, err
				}
				rsvps := []*events.RSVP{}
				for _, list := range byEvent {
					for _, r := range list {
						if r.HouseholdID == householdID {
							rsvps = append(rsvps, r)
						}
					}
				}
				return map[string]any{"club_event_rsvps": rsvps}, nil
			},
		})
	}

	if d.documents != nil {
		out = append(out, householdExportSource{
			name: "documents", keys: []string{"document_submissions"},
			collect: func(orgID, householdID string, _ map[string]bool) (map[string]any, error) {
				rows, err := d.documents.ListSubmissions(orgID, householdID)
				if err != nil {
					return nil, err
				}
				if rows == nil {
					rows = []documents.DocumentSubmission{}
				}
				return map[string]any{"document_submissions": rows}, nil
			},
		})
	}

	if d.comms != nil {
		out = append(out, householdExportSource{
			name: "communications", keys: []string{"messages", "message_edits", "message_attachments", "message_reads", "communication_preferences"},
			collect: func(orgID, householdID string, _ map[string]bool) (map[string]any, error) {
				x := d.comms.ExportHouseholdData(orgID, householdID)
				return map[string]any{
					"messages": x.Messages, "message_edits": x.MessageEdits, "message_attachments": x.MessageAttachments,
					"message_reads": x.MessageReads, "communication_preferences": x.Preferences,
				}, nil
			},
		})
	}
	return out
}
