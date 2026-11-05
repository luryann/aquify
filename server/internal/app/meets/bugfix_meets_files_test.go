package meets

import (
	"errors"
	"testing"
	"time"

	"github.com/luryann/aquify/meetfiles"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestDeriveSessionDraftsCapsHugeDateRange(t *testing.T) {
	t.Parallel()
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)
	drafts := deriveSessionDrafts(&meetfiles.ParsedMeetDefinition{MeetStartDate: &start, MeetEndDate: &end})
	if len(drafts) != maxDerivedMeetDays {
		t.Fatalf("drafts = %d, want %d", len(drafts), maxDerivedMeetDays)
	}
}

func TestCreateMeetValidatesDatesAndBounds(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	for name, in := range map[string]CreateMeetInput{
		"bad start":     {Name: "M", StartDate: "12/05/2026"},
		"end<start":     {Name: "M", StartDate: "2026-12-06", EndDate: "2026-12-05"},
		"bad deadline":  {Name: "M", EntryDeadline: "tomorrow"},
		"bad sess date": {Name: "M", Sessions: []CreateMeetSessionInput{{Name: "s", SessionNumber: 1, SessionDate: "x"}}},
		"neg fee":       {Name: "M", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50, EntryFeeCents: -1}}},
	} {
		if _, _, err := svc.CreateMeet(admin, "org-a", in); !errors.Is(err, ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
	if _, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{Name: "ok", StartDate: "2026-12-05", EndDate: "2026-12-05"}); err != nil {
		t.Fatalf("valid meet rejected: %v", err)
	}
}

func TestRelaySelectionsDoNotCountTowardCapAndLockKeepsIsRelay(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	parent := &appmiddleware.Principal{UserID: "parent-1", HouseholdID: "hh-1", Roles: []string{"parent"}}
	svc.SeedSwimmer("org-a", "sw-1", "hh-1")
	svc.SeedParentHousehold(parent.UserID, "hh-1")

	meet, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
		Name: "Relay Meet", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-05",
		SignupSettings: &CreateMeetSignupSettingsInput{SelfSignupEnabled: true, MaxEntriesPerSwimmer: 2},
		Events: []CreateMeetEventDraftInput{
			{EventNumber: "1", Stroke: "freestyle", Distance: 50, Gender: "O"},
			{EventNumber: "2", Stroke: "backstroke", Distance: 50, Gender: "O"},
			{EventNumber: "3", Stroke: "breaststroke", Distance: 50, Gender: "O"},
			{EventNumber: "4", Stroke: "freestyle", Distance: 200, Gender: "O", IsRelay: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.TransitionMeetStatus(admin, "org-a", meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.PublishCommitmentRequest(admin, "org-a", meet.ID); err != nil {
		t.Fatal(err)
	}
	cmts, _ := svc.ListCommitments(admin, "org-a", meet.ID)
	if _, _, err := svc.RespondToCommitment(parent, "org-a", meet.ID, cmts[0].ID, CommitmentRespondInput{Response: "committed_yes"}); err != nil {
		t.Fatal(err)
	}
	events, _ := svc.ListMeetEvents(admin, "org-a", meet.ID)
	byNum := map[string]string{}
	for _, e := range events {
		byNum[e.EventNumber] = e.ID
	}
	for _, n := range []string{"4", "1", "2"} {
		if _, err := svc.SelectCommitmentEvent(parent, "org-a", meet.ID, cmts[0].ID, SelectEventInput{MeetEventID: byNum[n]}); err != nil {
			t.Fatalf("select %s: %v", n, err)
		}
	}
	if _, err := svc.SelectCommitmentEvent(parent, "org-a", meet.ID, cmts[0].ID, SelectEventInput{MeetEventID: byNum["3"]}); !errors.Is(err, ErrMaxEntriesExceeded) {
		t.Fatalf("3rd individual should exceed cap of 2 only after 2 individuals: %v", err)
	}
	if _, err := svc.SelectCommitmentEvent(parent, "org-a", meet.ID, cmts[0].ID, SelectEventInput{MeetEventID: byNum["1"], SeedTimeMs: intPtr(-5)}); err == nil {
		t.Fatal("negative seed accepted")
	}
	if _, _, err := svc.LockCommitments(admin, "org-a", meet.ID); err != nil {
		t.Fatal(err)
	}
	entries, _ := svc.ListEntries(admin, "org-a", meet.ID)
	relays := 0
	for _, e := range entries {
		if e.MeetEventID == byNum["4"] {
			relays++
			if !e.IsRelay {
				t.Fatal("relay entry stored with IsRelay=false")
			}
		}
	}
	if relays != 1 {
		t.Fatalf("relay entries = %d", relays)
	}
}
