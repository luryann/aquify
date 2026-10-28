package app

import (
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

// performanceDirectory adapts the members and meets services to the
// performance.Directory interface (swimmer DOB/gender/groups for age-group
// records and relays; meet names/dates for swim dates).
type performanceDirectory struct {
	members *members.Service
	meets   *meets.Service
}

func newPerformanceDirectory(membersSvc *members.Service, meetsSvc *meets.Service) *performanceDirectory {
	return &performanceDirectory{members: membersSvc, meets: meetsSvc}
}

func (d *performanceDirectory) SwimmerProfiles(orgID string) []performance.SwimmerProfile {
	if d.members == nil {
		return nil
	}
	groupsBySwimmer := map[string][]string{}
	for _, link := range d.members.SnapshotSwimmerGroupLinks(orgID) {
		groupsBySwimmer[link.SwimmerID] = append(groupsBySwimmer[link.SwimmerID], link.GroupCode)
	}
	swimmers := d.members.SnapshotSwimmers(orgID)
	out := make([]performance.SwimmerProfile, 0, len(swimmers))
	for _, sw := range swimmers {
		out = append(out, performance.SwimmerProfile{
			ID:            sw.ID,
			HouseholdID:   sw.HouseholdID,
			FirstName:     sw.FirstName,
			LastName:      sw.LastName,
			PreferredName: sw.PreferredName,
			DateOfBirth:   sw.DateOfBirth,
			Gender:        sw.Gender,
			GroupCodes:    groupsBySwimmer[sw.ID],
		})
	}
	return out
}

func (d *performanceDirectory) Meets(orgID string) []performance.MeetInfo {
	if d.meets == nil {
		return nil
	}
	snapshot := d.meets.SnapshotMeets(orgID)
	out := make([]performance.MeetInfo, 0, len(snapshot))
	for _, m := range snapshot {
		out = append(out, performance.MeetInfo{ID: m.ID, Name: m.Name, StartDate: m.StartDate, Course: m.Course})
	}
	return out
}
