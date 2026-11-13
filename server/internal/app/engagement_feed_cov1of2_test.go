package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	"github.com/luryann/aquify/server/internal/app/training"
)

type cov1xClubFixture struct {
	router   http.Handler
	org      string
	admin    string
	coach    string
	parent   string
	hh       members.Household
	swimmer  members.Swimmer
	billing  *billing.Service
	training *training.Service
}

// cov1xNewClubFixture wires real in-memory services behind the real router: one
// household with a swimmer in the GOLD group, plus a parent, coach and admin login.
func cov1xNewClubFixture(t *testing.T) *cov1xClubFixture {
	t.Helper()
	f := &cov1xClubFixture{org: "org-cov1x-club"}
	admin := cov1xAdmin(f.org)
	membersSvc := members.NewService()
	groupsSvc := groups.NewService()
	meetsSvc := meets.NewService()
	f.billing = billing.NewService()
	f.training = training.NewService()
	f.router = NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		RouterDeps{Members: membersSvc, Groups: groupsSvc, Meets: meetsSvc, Billing: f.billing, Training: f.training})

	var err error
	if f.hh, err = membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: f.org, HouseholdName: "Feed Family"}); err != nil {
		t.Fatal(err)
	}
	if f.swimmer, _, err = membersSvc.CreateSwimmer(admin, f.org, members.CreateSwimmerInput{HouseholdID: f.hh.ID, FirstName: "Fern", LastName: "Feed", DateOfBirth: "2012-05-06", Gender: "female"}); err != nil {
		t.Fatal(err)
	}
	if _, err := membersSvc.AssignSwimmerToGroup(admin, f.org, f.swimmer.ID, members.AssignSwimmerGroupInput{GroupCode: "GOLD", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := groupsSvc.CreateGroup(admin, f.org, groups.CreateGroupInput{Code: "GOLD", Name: "Gold Squad"}); err != nil {
		t.Fatal(err)
	}
	f.admin = issueSession(t, f.router, sessionSpec{UserID: "cov1x-admin", OrgID: f.org, Roles: []string{"admin"}, MFA: true})
	f.coach = issueSession(t, f.router, sessionSpec{UserID: "cov1x-coach", OrgID: f.org, Roles: []string{"coach"}})
	f.parent = issueSession(t, f.router, sessionSpec{UserID: "cov1x-parent", OrgID: f.org, HouseholdID: f.hh.ID, Roles: []string{"parent"}})

	f.createMeets(t, meetsSvc)
	return f
}

func cov1xDay(offset int) string { return time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02") }

func (f *cov1xClubFixture) createMeets(t *testing.T, svc *meets.Service) {
	t.Helper()
	admin := cov1xAdmin(f.org)
	mk := func(in meets.CreateMeetInput, to ...string) {
		m, _, err := svc.CreateMeet(admin, f.org, in)
		if err != nil {
			t.Fatal(err)
		}
		for _, status := range to {
			if status == "commitment_open" {
				if _, _, _, err := svc.PublishCommitmentRequest(admin, f.org, m.ID); err != nil {
					t.Fatalf("%s open commitments: %v", in.Name, err)
				}
				continue
			}
			if _, _, err := svc.TransitionMeetStatus(admin, f.org, m.ID, status); err != nil {
				t.Fatalf("%s -> %s: %v", in.Name, status, err)
			}
		}
	}
	mk(meets.CreateMeetInput{Name: "Spring Invitational", StartDate: cov1xDay(20), EndDate: cov1xDay(21), HostOrg: "Otters", Location: "Aquatic Center"}, "published")
	mk(meets.CreateMeetInput{Name: "One Day Dual", StartDate: cov1xDay(30)}, "published")
	mk(meets.CreateMeetInput{Name: "Open Commitments Classic", StartDate: cov1xDay(40), CommitmentDeadline: cov1xDay(10)}, "published", "commitment_open")
	mk(meets.CreateMeetInput{Name: "Unpublished Draft", StartDate: cov1xDay(25)})
	mk(meets.CreateMeetInput{Name: "Ancient History Meet", StartDate: cov1xDay(-300), EndDate: cov1xDay(-299)}, "published")
	mk(meets.CreateMeetInput{Name: "Far Future Meet", StartDate: cov1xDay(700)}, "published")
}

func (f *cov1xClubFixture) do(t *testing.T, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doOrgRequest(f.router, method, path, token, f.org, body)
}

func (f *cov1xClubFixture) addPractice(t *testing.T, date, group, kind, desc string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"date": date, "group_id": group, "type": kind, "description": desc})
	rr := f.do(t, f.coach, http.MethodPost, "/api/v1/practices", string(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create practice %q: %d %s", desc, rr.Code, rr.Body.String())
	}
	var out struct {
		Practice struct{ ID string } `json:"practice"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Practice.ID
}

func TestCov1xCalendarFeedIncludesMeetsAndHouseholdPractices(t *testing.T) {
	f := cov1xNewClubFixture(t)
	if rr := f.do(t, f.admin, http.MethodPut, "/api/v1/org-settings", `{"team_name":"Cov Aquatics","timezone":"America/Chicago"}`); rr.Code != http.StatusOK {
		t.Fatalf("org settings: %d %s", rr.Code, rr.Body.String())
	}
	f.addPractice(t, cov1xDay(5), "", "water", "Team distance set")
	f.addPractice(t, cov1xDay(6), "GOLD", "dryland", "Gold core circuit")
	f.addPractice(t, cov1xDay(7), "GOLD", "water", "Gold sprint set")
	f.addPractice(t, cov1xDay(8), "SILVER", "water", "Silver only practice")
	f.addPractice(t, cov1xDay(9), "", "rest", "Rest day")
	f.addPractice(t, cov1xDay(-400), "", "water", "Ancient practice")

	rr := f.do(t, f.parent, http.MethodPost, "/api/v1/calendar/feed-token", "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("feed token: %d %s", rr.Code, rr.Body.String())
	}
	var tok struct {
		FeedPath string `json:"feed_path"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &tok); err != nil || tok.FeedPath == "" {
		t.Fatalf("feed path: %q %v", tok.FeedPath, err)
	}
	// The feed itself is public: the unguessable token is the credential.
	feed := doRequest(f.router, http.MethodGet, tok.FeedPath, "", "", "")
	if feed.Code != http.StatusOK || !strings.HasPrefix(feed.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("feed: %d %s", feed.Code, feed.Header().Get("Content-Type"))
	}
	ics := feed.Body.String()

	for _, want := range []string{
		"X-WR-CALNAME:Cov Aquatics calendar", "X-WR-TIMEZONE:America/Chicago",
		"SUMMARY:Spring Invitational", "Hosted by Otters", "LOCATION:Aquatic Center",
		"SUMMARY:One Day Dual", "SUMMARY:Open Commitments Classic",
		"SUMMARY:Commitment deadline — Open Commitments Classic", "Respond yes/no for each swimmer",
		"SUMMARY:Practice — All groups", "SUMMARY:Dryland — Gold Squad", "SUMMARY:Practice — Gold Squad",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("feed is missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"Unpublished Draft", "Ancient History Meet", "Far Future Meet",
		"Silver only practice", "Rest day", "Ancient practice",
	} {
		if strings.Contains(ics, unwanted) {
			t.Errorf("feed must not contain %q", unwanted)
		}
	}
	// The one-day meet has no end date: it spans exactly its start day.
	oneDay := strings.ReplaceAll(cov1xDay(30), "-", "")
	if !strings.Contains(ics, "DTSTART;VALUE=DATE:"+oneDay) {
		t.Errorf("one-day meet start date missing")
	}
}

func TestCov1xAttendanceSummaryReadsTrainingMarks(t *testing.T) {
	f := cov1xNewClubFixture(t)
	today := time.Now().UTC().Format("2006-01-02")
	p1 := f.addPractice(t, today, "", "water", "Today's practice")
	f.addPractice(t, today, "", "water", "Unmarked practice")
	if rr := f.do(t, f.coach, http.MethodPut, "/api/v1/practices/"+p1+"/attendance/"+f.swimmer.ID, `{"status":"present"}`); rr.Code != http.StatusOK {
		t.Fatalf("mark attendance: %d %s", rr.Code, rr.Body.String())
	}
	rr := f.do(t, f.parent, http.MethodGet, "/api/v1/swimmers/"+f.swimmer.ID+"/attendance-summary?days=30", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("summary: %d %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Summary struct {
			SwimmerID         string `json:"swimmer_id"`
			To                string `json:"to"`
			EligiblePractices int    `json:"eligible_practices"`
			Present           int    `json:"present"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Summary.SwimmerID != f.swimmer.ID || out.Summary.To != today || out.Summary.EligiblePractices != 2 || out.Summary.Present != 1 {
		t.Fatalf("unexpected summary %+v (window must end on the server's current UTC date)", out.Summary)
	}
}

func TestCov1xClubEventFeeUsesClubTimezoneAndIsReleasedWhenRSVPLeaves(t *testing.T) {
	f := cov1xNewClubFixture(t)
	if rr := f.do(t, f.admin, http.MethodPut, "/api/v1/org-settings", `{"team_name":"Cov Aquatics","timezone":"America/Los_Angeles"}`); rr.Code != http.StatusOK {
		t.Fatalf("org settings: %d %s", rr.Code, rr.Body.String())
	}
	// 03:00 UTC is still the previous evening on the US west coast.
	start := time.Now().UTC().AddDate(0, 0, 14).Truncate(24 * time.Hour).Add(3 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"title": "Banquet", "category": "banquet", "starts_at": start.Format(time.RFC3339),
		"rsvp_enabled": true, "rsvp_mode": "household", "fee_cents": 1500, "audience": "all", "status": "scheduled",
	})
	rr := f.do(t, f.admin, http.MethodPost, "/api/v1/club-events", string(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create event: %d %s", rr.Code, rr.Body.String())
	}
	var created struct {
		Event struct{ ID string } `json:"event"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil || created.Event.ID == "" {
		t.Fatalf("event id: %v %s", err, rr.Body.String())
	}
	path := "/api/v1/club-events/" + created.Event.ID + "/rsvps"
	rr = f.do(t, f.parent, http.MethodPut, path, `{"household_id":"`+f.hh.ID+`","response":"yes","guest_count":1}`)
	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("rsvp yes: %d %s", rr.Code, rr.Body.String())
	}

	invoices := f.billing.SnapshotInvoices(f.org)
	if len(invoices) != 1 {
		t.Fatalf("expected one RSVP invoice, got %+v", invoices)
	}
	inv := invoices[0]
	wantDue := start.In(mustLoadLocation(t, "America/Los_Angeles")).Format("2006-01-02")
	if inv.Status != "issued" || inv.TotalCents != 3000 || inv.HouseholdID != f.hh.ID || inv.DueDate != wantDue {
		t.Fatalf("invoice %+v: want issued, 3000c, household %s, due %s (club-local date)", inv, f.hh.ID, wantDue)
	}
	if wantDue == start.Format("2006-01-02") {
		t.Fatal("fixture must straddle a UTC date boundary to prove the timezone is applied")
	}

	rr = f.do(t, f.parent, http.MethodPut, path, `{"household_id":"`+f.hh.ID+`","response":"no"}`)
	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("rsvp no: %d %s", rr.Code, rr.Body.String())
	}
	if got := f.billing.SnapshotInvoices(f.org); len(got) != 1 || got[0].Status != "voided" {
		t.Fatalf("leaving an unpaid RSVP must void its invoice: %+v", got)
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
