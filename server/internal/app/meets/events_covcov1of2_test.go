package meets

import (
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

// Event numbers are unique per meet through the API, but legacy or migrated
// rows can share one; listing must still be deterministic (ordered by ID).
func TestCov1xListMeetEventsTiesBreakByID(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	meet, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
		Name: "Tie Meet", MeetType: "invitational", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-05",
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	svc.mu.Lock()
	for _, id := range []string{"ev-c", "ev-a", "ev-b"} {
		svc.events[id] = MeetEvent{ID: id, OrgID: "org-a", MeetID: meet.ID, EventNumber: "5", Stroke: "freestyle", Distance: 50, Course: "SCY", Gender: "O"}
	}
	svc.mu.Unlock()
	events, err := svc.ListMeetEvents(admin, "org-a", meet.ID)
	if err != nil || len(events) != 3 {
		t.Fatalf("events = %d err=%v", len(events), err)
	}
	for i, want := range []string{"ev-a", "ev-b", "ev-c"} {
		if events[i].ID != want {
			t.Fatalf("events[%d] = %q, want %q", i, events[i].ID, want)
		}
	}
}
