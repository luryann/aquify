package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/events"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/volunteer"
	"github.com/dareaquatics/aquify/server/internal/app/wellness"
	"github.com/dareaquatics/aquify/server/internal/testutil"
)

// requiredHouseholdExportKeys is the contract for a right-of-access export:
// every domain that stores household or swimmer records must appear. Adding a
// personal-data domain means adding its keys here AND a source for it.
var requiredHouseholdExportKeys = []string{
	"household", "contacts", "swimmers", "group_memberships", // members
	"registrations", "waiver_acceptances", // groups
	"invoices", "invoice_items", "payments", "refunds", // billing
	"meet_commitments", "meet_event_selections", "meet_entries", // meets
	"volunteer_signups", "volunteer_credits", "volunteer_buyouts", // volunteer
	"wellness_checkins", "club_event_rsvps", "document_submissions", // wellness, events, documents
	"messages", "message_edits", "message_attachments", "message_reads", "communication_preferences", // comms
}

func TestHouseholdExportCoversEveryPersonalDataDomain(t *testing.T) {
	org := testutil.DefaultOrgID
	admin := testutil.AdminPrincipal()

	membersSvc := members.NewService()
	hh, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Export Family"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Other Family"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := membersSvc.CreateSwimmer(admin, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "Ann", LastName: "Export", DateOfBirth: "2012-04-05", Gender: "female"})
	if err != nil {
		t.Fatal(err)
	}

	wellnessSvc := wellness.NewService()
	if _, _, err := wellnessSvc.Upsert(org, wellness.Input{
		SwimmerID: sw.ID, HouseholdID: hh.ID, Date: "", SubmittedBy: "parent-1", SubmittedRole: "parent",
		Metrics: wellness.Metrics{SleepHours: 8, SleepQuality: 4, Soreness: 2, Energy: 4, Stress: 2, Mood: 4},
	}); err != nil {
		t.Fatal(err)
	}

	eventsSvc := events.NewService()
	ev, err := eventsSvc.Create(org, "admin-1", events.EventInput{
		Title: "Banquet", Category: "banquet", StartsAt: time.Now().Add(72 * time.Hour), RSVPEnabled: true, RSVPMode: "household", Audience: "all",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventsSvc.Respond(org, ev.ID, "p", events.RSVPInput{HouseholdID: hh.ID, Response: "yes"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := eventsSvc.Respond(org, ev.ID, "p", events.RSVPInput{HouseholdID: other.ID, Response: "yes"}, true); err != nil {
		t.Fatal(err)
	}

	docsSvc := documents.NewService()
	dt, err := docsSvc.CreateDocumentType(org, "Waiver", "waiver", "", "household", true, true, nil, "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := docsSvc.CreateSubmission(org, dt.ID, hh.ID, "", "parent-1"); err != nil {
		t.Fatal(err)
	}

	commsSvc := comms.NewService()
	if _, err := commsSvc.SendMessage(comms.CreateMessageInput{OrgID: org, HouseholdID: hh.ID, SwimmerID: sw.ID, Body: "secret-body-hh", SenderUserID: "coach-1", SenderRole: "coach"}); err != nil {
		t.Fatal(err)
	}
	if _, err := commsSvc.SendMessage(comms.CreateMessageInput{OrgID: org, HouseholdID: other.ID, SwimmerID: "sw-other", Body: "foreign-body", SenderUserID: "coach-1", SenderRole: "coach"}); err != nil {
		t.Fatal(err)
	}

	sources := buildHouseholdExportSources(householdExportDeps{
		meets: meets.NewService(), volunteer: volunteer.NewService(), wellness: wellnessSvc,
		events: eventsSvc, documents: docsSvc, comms: commsSvc,
	})
	data, err := collectHouseholdExportData(org, hh.ID, membersSvc, groups.NewService(), billing.NewService(), sources...)
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range requiredHouseholdExportKeys {
		if _, ok := data[key]; !ok {
			t.Errorf("export is missing required domain key %q", key)
		}
	}

	raw, _ := json.Marshal(data)
	body := string(raw)
	for _, want := range []string{"secret-body-hh", `"wellness_checkins":[{`, `"club_event_rsvps":[{`, `"document_submissions":[{`} {
		if !strings.Contains(body, want) {
			t.Errorf("export lacks expected household data %q", want)
		}
	}
	if strings.Contains(body, "foreign-body") || strings.Contains(body, other.ID) {
		t.Error("export leaked another household's records")
	}
}

func TestHouseholdExportFailsWhenADomainCannotBeRead(t *testing.T) {
	src := householdExportSource{name: "broken", collect: func(string, string, map[string]bool) (map[string]any, error) {
		return nil, errBrokenExport
	}}
	membersSvc := members.NewService()
	admin := testutil.AdminPrincipal()
	hh, _ := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: testutil.DefaultOrgID, HouseholdName: "F"})
	if _, err := collectHouseholdExportData(testutil.DefaultOrgID, hh.ID, membersSvc, nil, nil, src); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("a failing domain must fail the export, got %v", err)
	}
}

var errBrokenExport = &exportTestError{}

type exportTestError struct{}

func (*exportTestError) Error() string { return "domain unavailable" }
