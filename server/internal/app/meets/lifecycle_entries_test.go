package meets

import (
	"errors"
	"os"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func openMeetForCommitments(t *testing.T, svc *Service, admin *appmiddleware.Principal) Meet {
	t.Helper()
	meet, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
		Name: "Lock Meet", MeetType: "invitational", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-05",
		Events: []CreateMeetEventDraftInput{
			{EventNumber: "1", Stroke: "freestyle", Distance: 50, Course: "SCY", Gender: "O"},
			{EventNumber: "2", Stroke: "backstroke", Distance: 100, Course: "SCY", Gender: "O"},
		},
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, _, err := svc.TransitionMeetStatus(admin, "org-a", meet.ID, "published"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if meet, _, _, err = svc.PublishCommitmentRequest(admin, "org-a", meet.ID); err != nil {
		t.Fatalf("open commitments: %v", err)
	}
	return meet
}

func TestLockCommitmentsTurnsSelectionsIntoEntries(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	parent := &appmiddleware.Principal{UserID: "parent-1", HouseholdID: "hh-1", Roles: []string{"parent"}}
	svc.SeedSwimmer("org-a", "sw-yes", "hh-1")
	svc.SeedSwimmer("org-a", "sw-no", "hh-1")
	svc.SeedParentHousehold(parent.UserID, "hh-1")

	meet := openMeetForCommitments(t, svc, admin)
	commitments, err := svc.ListCommitments(admin, "org-a", meet.ID)
	if err != nil || len(commitments) != 2 {
		t.Fatalf("commitments: %d (%v)", len(commitments), err)
	}
	events, _ := svc.ListMeetEvents(admin, "org-a", meet.ID)
	ids := map[string]string{}
	for _, c := range commitments {
		ids[c.SwimmerID] = c.ID
	}
	if _, _, err := svc.RespondToCommitment(parent, "org-a", meet.ID, ids["sw-yes"], CommitmentRespondInput{Response: "committed_yes"}); err != nil {
		t.Fatalf("respond yes: %v", err)
	}
	if _, _, err := svc.RespondToCommitment(parent, "org-a", meet.ID, ids["sw-no"], CommitmentRespondInput{Response: "committed_no"}); err != nil {
		t.Fatalf("respond no: %v", err)
	}
	for _, ev := range events {
		if _, err := svc.SelectCommitmentEvent(parent, "org-a", meet.ID, ids["sw-yes"], SelectEventInput{MeetEventID: ev.ID}); err != nil {
			t.Fatalf("select: %v", err)
		}
	}

	if _, _, err := svc.LockCommitments(admin, "org-a", meet.ID); err != nil {
		t.Fatalf("LockCommitments: %v", err)
	}
	entries, err := svc.ListEntries(admin, "org-a", meet.ID)
	if err != nil || len(entries) != 2 {
		t.Fatalf("want 2 entries from selections, got %d (%v)", len(entries), err)
	}
	commitments, _ = svc.ListCommitments(admin, "org-a", meet.ID)
	for _, c := range commitments {
		want := map[string]string{"sw-yes": "locked", "sw-no": "committed_no"}[c.SwimmerID]
		if c.Status != want {
			t.Fatalf("swimmer %s: want %s, got %s", c.SwimmerID, want, c.Status)
		}
	}

	// Approve one entry, then mark both exported: only the approved one moves.
	if _, _, err := svc.ApproveEntry(admin, "org-a", meet.ID, entries[0].ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	marked, _, err := svc.MarkEntriesExported(admin, "org-a", meet.ID, []string{entries[0].ID, entries[1].ID})
	if err != nil || marked != 1 {
		t.Fatalf("MarkEntriesExported: marked %d (%v)", marked, err)
	}
	if _, _, err := svc.MarkEntriesExported(parent, "org-a", meet.ID, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent export: want ErrForbidden, got %v", err)
	}
	if _, _, err := svc.MarkEntriesExported(admin, "org-a", "missing", nil); !errors.Is(err, ErrMeetNotFound) {
		t.Fatalf("missing meet: want ErrMeetNotFound, got %v", err)
	}
}

func TestInviteSwimmerToOpenMeetsIsIdempotent(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	meet := openMeetForCommitments(t, svc, admin)

	created, err := svc.InviteSwimmerToOpenMeets("org-a", "sw-late", "hh-9", "admin-1")
	if err != nil || len(created) != 1 || created[0].MeetID != meet.ID || created[0].Status != "invited" {
		t.Fatalf("first invite: %+v (%v)", created, err)
	}
	again, err := svc.InviteSwimmerToOpenMeets("org-a", "sw-late", "hh-9", "admin-1")
	if err != nil || len(again) != 0 {
		t.Fatalf("second invite should be a no-op: %+v (%v)", again, err)
	}
	other, _ := svc.InviteSwimmerToOpenMeets("org-b", "sw-other", "hh-x", "admin-1")
	if len(other) != 0 {
		t.Fatal("swimmers must not be invited to another org's meets")
	}
	if _, err := svc.InviteSwimmerToOpenMeets("", "sw", "hh", "a"); err == nil {
		t.Fatal("missing org should error")
	}
}

func TestParseResultsFileFormats(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../../internal/meetfiles/testdata/fixtures/valid.sd3")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	result, _, err := ParseResultsFile("results.sd3", data)
	if err != nil || len(result.IndividualResults) != 2 {
		t.Fatalf("sd3: %v results=%v", err, result)
	}
	if !result.IndividualResults[0].FinalsTime.Valid {
		t.Fatal("expected a finals time on the first result")
	}
	if _, _, err := ParseResultsFile("", data); !errors.Is(err, ErrInvalidMeetFile) {
		t.Fatalf("empty name: want ErrInvalidMeetFile, got %v", err)
	}
	if _, _, err := ParseResultsFile("notes.txt", []byte("hello")); !errors.Is(err, ErrInvalidMeetFile) {
		t.Fatalf("unsupported type: want ErrInvalidMeetFile, got %v", err)
	}
	cl2, err := os.ReadFile("../../../../internal/meetfiles/testdata/fixtures/sample.cl2")
	if err != nil {
		t.Fatalf("cl2 fixture: %v", err)
	}
	if _, _, err := ParseResultsFile("results.cl2", cl2); err != nil {
		t.Fatalf("cl2: %v", err)
	}
}
