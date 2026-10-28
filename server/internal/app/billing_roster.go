package app

import (
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// newBillingRosterSource projects the members/groups domains into the
// billing roster view used by billing runs, upcoming charges, and reports.
// ListHouseholds runs first so DB-backed members state is re-synced before
// the swimmer/link snapshots are taken.
func newBillingRosterSource(membershipService *members.Service, groupsService *groups.Service) billing.RosterSource {
	return func(orgID string) (billing.RosterSnapshot, error) {
		var snap billing.RosterSnapshot
		if membershipService == nil {
			return snap, billing.ErrRosterUnavailable
		}
		system := &appmiddleware.Principal{UserID: "system:billing", OrgID: orgID, Roles: []string{"admin"}}
		households, err := membershipService.ListHouseholds(system, orgID)
		if err != nil {
			return snap, err
		}
		for _, h := range households {
			snap.Households = append(snap.Households, billing.RosterHousehold{ID: h.ID, Name: h.HouseholdName, Status: h.Status})
		}
		for _, sw := range membershipService.SnapshotSwimmers(orgID) {
			snap.Swimmers = append(snap.Swimmers, billing.RosterSwimmer{
				ID: sw.ID, HouseholdID: sw.HouseholdID, FirstName: sw.FirstName, LastName: sw.LastName,
				DateOfBirth: sw.DateOfBirth, Status: sw.RegistrationStatus,
			})
		}
		for _, link := range membershipService.SnapshotSwimmerGroupLinks(orgID) {
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
