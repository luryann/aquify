package meets

import (
	"errors"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestMeetRulesDraftHiddenCourseDefaultAndStatusGate(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	parent := &appmiddleware.Principal{UserID: "parent-1", HouseholdID: "h1", Roles: []string{"parent"}}

	meet, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
		Name: "LC Meet", MeetType: "invitational", Course: "LCM", StartDate: "2026-07-10", EndDate: "2026-07-10",
		SignupSettings: &CreateMeetSignupSettingsInput{MaxEntriesPerSwimmer: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetMeet(parent, "org-a", meet.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Fatalf("parent GetMeet on draft = %v, want not found", err)
	}
	if list, _ := svc.ListMeets(parent, "org-a"); len(list) != 0 {
		t.Fatalf("parent sees draft meets: %d", len(list))
	}
	if _, err := svc.ListMeetEvents(parent, "org-a", meet.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Fatalf("parent ListMeetEvents on draft = %v", err)
	}
	if list, _ := svc.ListMeets(admin, "org-a"); len(list) != 1 {
		t.Fatalf("admin should see draft")
	}

	ev, _, err := svc.CreateMeetEvent(admin, "org-a", meet.ID, CreateMeetEventInput{EventNumber: "1", Stroke: "freestyle", Distance: 100})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Course != "LCM" {
		t.Fatalf("event course = %q, want LCM", ev.Course)
	}

	svc.mu.Lock()
	m := svc.meets[meet.ID]
	m.Status = "entry_finalized"
	svc.meets[meet.ID] = m
	svc.mu.Unlock()
	if _, _, err := svc.CreateMeetEvent(admin, "org-a", meet.ID, CreateMeetEventInput{EventNumber: "2", Stroke: "freestyle", Distance: 50}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("CreateMeetEvent on finalized = %v", err)
	}
	if _, _, err := svc.CreateMeetSession(admin, "org-a", meet.ID, CreateMeetSessionInput{Name: "x", SessionNumber: 1}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("CreateMeetSession on finalized = %v", err)
	}
}

func TestProposeEntryMaxAndReviveScratched(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	meet, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
		Name: "M", MeetType: "invitational", Course: "SCY", StartDate: "2026-07-10", EndDate: "2026-07-10",
		Events: []CreateMeetEventDraftInput{
			{EventNumber: "1", Stroke: "freestyle", Distance: 50, Course: "SCY", Gender: "O"},
			{EventNumber: "2", Stroke: "freestyle", Distance: 100, Course: "SCY", Gender: "O"},
		},
		SignupSettings: &CreateMeetSignupSettingsInput{MaxEntriesPerSwimmer: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := svc.ListMeetEvents(admin, "org-a", meet.ID)
	svc.mu.Lock()
	m := svc.meets[meet.ID]
	m.Status = "commitment_closed"
	svc.meets[meet.ID] = m
	svc.mu.Unlock()

	e1, _, err := svc.ProposeEntry(admin, "org-a", meet.ID, ProposeEntryInput{SwimmerID: "s1", MeetEventID: evs[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ProposeEntry(admin, "org-a", meet.ID, ProposeEntryInput{SwimmerID: "s1", MeetEventID: evs[1].ID}); !errors.Is(err, ErrMaxEntriesExceeded) {
		t.Fatalf("second entry = %v, want max exceeded", err)
	}
	if _, _, err := svc.ScratchEntry(admin, "org-a", meet.ID, e1.ID, "oops"); err != nil {
		t.Fatal(err)
	}
	re, _, err := svc.ProposeEntry(admin, "org-a", meet.ID, ProposeEntryInput{SwimmerID: "s1", MeetEventID: evs[0].ID})
	if err != nil {
		t.Fatalf("re-propose scratched: %v", err)
	}
	if re.ID != e1.ID || re.Status != "submitted" || re.ScratchReason != "" {
		t.Fatalf("revived entry = %+v", re)
	}
}
