package meets

import (
	"errors"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestMeetCommitmentEventSelectionLifecycle(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	parent := &appmiddleware.Principal{UserID: "parent-1", HouseholdID: "household-1", Roles: []string{"parent"}}

	svc.SeedSwimmer("org-a", "swimmer-1", "household-1")
	svc.SeedParentHousehold(parent.UserID, "household-1")

	meet, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
		Name: "Summer Invite", MeetType: "invitational", Course: "SCY", StartDate: "2026-07-10", EndDate: "2026-07-10",
		Sessions:       []CreateMeetSessionInput{{SessionNumber: 1, Name: "Morning"}},
		Events:         []CreateMeetEventDraftInput{{SessionNumber: 1, EventNumber: "1", Stroke: "freestyle", Distance: 100, Course: "SCY", Gender: "F"}},
		SignupSettings: &CreateMeetSignupSettingsInput{SelfSignupEnabled: true, MaxEntriesPerSwimmer: 3},
	})
	if err != nil {
		t.Fatalf("CreateMeet() error = %v", err)
	}
	if _, _, err := svc.TransitionMeetStatus(admin, "org-a", meet.ID, "published"); err != nil {
		t.Fatalf("TransitionMeetStatus(published) error = %v", err)
	}
	meet, commitments, _, err := svc.PublishCommitmentRequest(admin, "org-a", meet.ID)
	if err != nil {
		t.Fatalf("PublishCommitmentRequest() error = %v", err)
	}
	if meet.Status != "commitment_open" || len(commitments) != 1 {
		t.Fatalf("PublishCommitmentRequest() meet=%+v commitments=%+v, want open with one commitment", meet, commitments)
	}

	events, err := svc.ListMeetEvents(parent, "org-a", meet.ID)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListEvents() len = %d, want 1", len(events))
	}
	if _, err := svc.SelectCommitmentEvent(parent, "org-a", meet.ID, commitments[0].ID, SelectEventInput{MeetEventID: events[0].ID}); !errors.Is(err, ErrNotCommittedYes) {
		t.Fatalf("SelectCommitmentEvent() before yes error = %v, want %v", err, ErrNotCommittedYes)
	}

	commitment, _, err := svc.RespondToCommitment(parent, "org-a", meet.ID, commitments[0].ID, CommitmentRespondInput{Response: "committed_yes", Notes: "Available"})
	if err != nil {
		t.Fatalf("RespondToCommitment() error = %v", err)
	}
	if commitment.Status != "committed_yes" {
		t.Fatalf("RespondToCommitment() status = %q, want committed_yes", commitment.Status)
	}

	selection, err := svc.SelectCommitmentEvent(parent, "org-a", meet.ID, commitment.ID, SelectEventInput{MeetEventID: events[0].ID})
	if err != nil {
		t.Fatalf("SelectCommitmentEvent() error = %v", err)
	}
	if selection.MeetEventID != events[0].ID {
		t.Fatalf("SelectCommitmentEvent() = %+v, want event %s", selection, events[0].ID)
	}
	if _, err := svc.SelectCommitmentEvent(parent, "org-a", meet.ID, commitment.ID, SelectEventInput{MeetEventID: events[0].ID}); !errors.Is(err, ErrDuplicateSelection) {
		t.Fatalf("SelectCommitmentEvent() duplicate error = %v, want %v", err, ErrDuplicateSelection)
	}
}
