package app

import (
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/members"
)

// newBillingRosterSource projects the members/groups domains into the
// billing roster view used by billing runs, upcoming charges, and reports.
// members.SnapshotRoster re-syncs DB-backed state and copies households,
// swimmers and links under one lock; no synthetic principal is involved.
func newBillingRosterSource(membershipService *members.Service, groupsService *groups.Service) billing.RosterSource {
	return func(orgID string) (billing.RosterSnapshot, error) {
		var snap billing.RosterSnapshot
		if membershipService == nil {
			return snap, billing.ErrRosterUnavailable
		}
		raw, err := membershipService.SnapshotRoster(orgID)
		if err != nil {
			return snap, err
		}
		for _, h := range raw.Households {
			snap.Households = append(snap.Households, billing.RosterHousehold{ID: h.ID, Name: h.HouseholdName, Status: h.Status})
		}
		for _, sw := range raw.Swimmers {
			snap.Swimmers = append(snap.Swimmers, billing.RosterSwimmer{
				ID: sw.ID, HouseholdID: sw.HouseholdID, FirstName: sw.FirstName, LastName: sw.LastName,
				DateOfBirth: sw.DateOfBirth, Status: sw.RegistrationStatus,
			})
		}
		for _, link := range raw.GroupLinks {
			snap.GroupLinks = append(snap.GroupLinks, billing.RosterGroupLink{
				SwimmerID: link.SwimmerID, GroupCode: link.GroupCode, IsPrimary: link.IsPrimary, AssignedAt: link.AssignedAt,
			})
		}
		if groupsService != nil {
			codeByID := map[string]string{}
			for _, g := range groupsService.SnapshotGroups(orgID) {
				codeByID[g.ID] = g.Code
				snap.Groups = append(snap.Groups, billing.RosterGroup{Code: g.Code, Name: g.Name})
			}
			for _, reg := range groupsService.SnapshotRegistrations(orgID) {
				snap.Registrations = append(snap.Registrations, billing.RosterRegistration{
					ID: reg.ID, HouseholdID: reg.HouseholdID, SwimmerID: reg.SwimmerID, GroupCode: codeByID[reg.GroupID],
					Season: reg.Season, Status: reg.Status, RegistrationDate: reg.RegistrationDate,
				})
			}
		}
		return snap, nil
	}
}
