package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/attendance"
	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/events"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/roster"
	"github.com/dareaquatics/aquify/server/internal/app/wellness"
	"github.com/go-chi/chi/v5"
)

type tokenResolver map[string]*appmiddleware.Principal

func (t tokenResolver) ResolvePrincipal(token string) (*appmiddleware.Principal, bool) {
	p, ok := t[token]
	return p, ok
}

const engOrg = "org-eng"

type fakePractices struct{}

func (fakePractices) Practices(string) ([]attendance.Practice, error) {
	return []attendance.Practice{{ID: "p1", Date: time.Now().UTC().Format("2006-01-02"), GroupID: "SENIOR", Type: "water"}}, nil
}
func (fakePractices) Attendance(string, string) ([]attendance.Record, error) {
	return []attendance.Record{{SwimmerID: "sw-a", Status: "present"}}, nil
}

type engagementFixture struct {
	router  http.Handler
	events  *events.Service
	audit   *audit.Service
	welness *wellness.Service
}

func newEngagementFixture(t *testing.T) engagementFixture {
	t.Helper()
	dir := &roster.StaticDirectory{
		SwimmerList: []roster.Swimmer{
			{ID: "sw-a", OrgID: engOrg, HouseholdID: "hh-a", FirstName: "Ada", LastName: "A", DisplayName: "Ada A", GroupCodes: []string{"SENIOR"}},
			{ID: "sw-a2", OrgID: engOrg, HouseholdID: "hh-a", FirstName: "Abe", LastName: "A", DisplayName: "Abe A", GroupCodes: []string{"AGE"}},
			{ID: "sw-b", OrgID: engOrg, HouseholdID: "hh-b", FirstName: "Bo", LastName: "B", DisplayName: "Bo B", GroupCodes: []string{"SENIOR"}},
		},
		Households: map[string]string{"hh-a": "A Family", "hh-b": "B Family"},
	}
	resolver := tokenResolver{
		"admin":   {UserID: "u-admin", OrgID: engOrg, Roles: []string{"admin"}},
		"coach":   {UserID: "u-coach", OrgID: engOrg, Roles: []string{"coach"}},
		"parentA": {UserID: "u-pa", OrgID: engOrg, HouseholdID: "hh-a", Roles: []string{"parent"}},
		"parentB": {UserID: "u-pb", OrgID: engOrg, HouseholdID: "hh-b", Roles: []string{"parent"}},
		"swimA":   {UserID: "u-sa", OrgID: engOrg, HouseholdID: "hh-a", SwimmerID: "sw-a", Roles: []string{"swimmer"}},
		"swimA2":  {UserID: "u-sa2", OrgID: engOrg, HouseholdID: "hh-a", SwimmerID: "sw-a2", Roles: []string{"swimmer"}},
	}
	evSvc := events.NewService()
	auditSvc := audit.NewService()
	wSvc := wellness.NewService()
	ev := NewClubEventsHandlers(evSvc, dir, auditSvc, nil)
	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		ev.RegisterPublicRoutes(api)
		api.Group(func(p chi.Router) {
			p.Use(appmiddleware.Auth(resolver))
			p.Use(appmiddleware.RequireOrgScope)
			ev.RegisterRoutes(p)
			NewWellnessHandlers(wSvc, dir).RegisterRoutes(p)
			NewAttendanceSummaryHandlers(fakePractices{}, dir, func() string { return time.Now().UTC().Format("2006-01-02") }).RegisterRoutes(p)
		})
	})
	return engagementFixture{router: r, events: evSvc, audit: auditSvc, welness: wSvc}
}

func (f engagementFixture) do(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", engOrg)
	}
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	return rr
}

func expectStatus(t *testing.T, rr *httptest.ResponseRecorder, want int, label string) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("%s: want %d, got %d: %s", label, want, rr.Code, rr.Body.String())
	}
}

func createClubEvent(t *testing.T, f engagementFixture, extra string) string {
	t.Helper()
	start := time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339)
	body := `{"title":"Pizza Social","category":"social","starts_at":"` + start + `","rsvp_enabled":true` + extra + `}`
	rr := f.do(t, http.MethodPost, "/api/v1/club-events", "coach", body)
	expectStatus(t, rr, http.StatusCreated, "create event")
	var out struct {
		Event events.Event `json:"event"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return out.Event.ID
}

func TestClubEventsRBACAndAudience(t *testing.T) {
	f := newEngagementFixture(t)
	expectStatus(t, f.do(t, http.MethodPost, "/api/v1/club-events", "parentA", `{"title":"x","category":"social","starts_at":"2030-01-01T00:00:00Z"}`), http.StatusForbidden, "parent create")
	expectStatus(t, f.do(t, http.MethodPost, "/api/v1/club-events", "coach", `{"title":"x","category":"party","starts_at":"2030-01-01T00:00:00Z"}`), http.StatusUnprocessableEntity, "bad category")
	allID := createClubEvent(t, f, "")
	ageID := createClubEvent(t, f, `,"audience":"groups","group_codes":["age"]`)
	if events, _ := f.audit.Query(audit.Filter{OrgID: engOrg}); len(events) < 2 {
		t.Fatalf("expected audit events for creates, got %d", len(events))
	}

	// Parent B (only SENIOR swimmer) cannot see the AGE-only event.
	rr := f.do(t, http.MethodGet, "/api/v1/club-events", "parentB", "")
	expectStatus(t, rr, http.StatusOK, "parentB list")
	if strings.Contains(rr.Body.String(), ageID) || !strings.Contains(rr.Body.String(), allID) {
		t.Fatalf("parentB audience filtering wrong: %s", rr.Body.String())
	}
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/club-events/"+ageID, "parentB", ""), http.StatusNotFound, "parentB hidden get")
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/club-events/"+ageID, "parentA", ""), http.StatusOK, "parentA get")
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/club-events/"+ageID+"/rsvps/export", "parentA", ""), http.StatusForbidden, "parent export")
	expectStatus(t, f.do(t, http.MethodDelete, "/api/v1/club-events/"+allID, "swimA", ""), http.StatusForbidden, "swimmer delete")
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/club-events", "", ""), http.StatusUnauthorized, "anon list")
}

func TestClubEventRSVPHouseholdScoping(t *testing.T) {
	f := newEngagementFixture(t)
	hhID := createClubEvent(t, f, `,"capacity":4`)
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/club-events/"+hhID+"/rsvps", "parentA", `{"response":"yes","guest_count":2}`), http.StatusOK, "parentA rsvp")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/club-events/"+hhID+"/rsvps", "parentB", `{"household_id":"hh-a","response":"no"}`), http.StatusForbidden, "parentB foreign household")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/club-events/"+hhID+"/rsvps", "parentB", `{"response":"yes","guest_count":1}`), http.StatusConflict, "capacity")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/club-events/"+hhID+"/rsvps", "swimA", `{"response":"yes"}`), http.StatusForbidden, "swimmer household-mode")

	rr := f.do(t, http.MethodGet, "/api/v1/club-events/"+hhID+"/rsvps", "parentB", "")
	expectStatus(t, rr, http.StatusOK, "parentB rsvps")
	if strings.Contains(rr.Body.String(), "hh-a") {
		t.Fatalf("parentB must not see household A RSVPs: %s", rr.Body.String())
	}
	rr = f.do(t, http.MethodGet, "/api/v1/club-events/"+hhID+"/rsvps/export", "coach", "")
	expectStatus(t, rr, http.StatusOK, "coach export")
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") || !strings.Contains(rr.Body.String(), "A Family,,yes,2,3") {
		t.Fatalf("unexpected csv (%s): %s", ct, rr.Body.String())
	}

	swID := createClubEvent(t, f, `,"rsvp_mode":"swimmer"`)
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/club-events/"+swID+"/rsvps", "swimA", `{"response":"maybe"}`), http.StatusOK, "swimmer self rsvp")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/club-events/"+swID+"/rsvps", "parentA", `{"swimmer_id":"sw-b","response":"yes"}`), http.StatusForbidden, "parent foreign swimmer")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/club-events/"+swID+"/rsvps", "parentA", `{"swimmer_id":"sw-a2","response":"yes"}`), http.StatusOK, "parent own swimmer")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/club-events/"+swID+"/rsvps", "coach", `{"swimmer_id":"sw-b","response":"no"}`), http.StatusOK, "coach on behalf")

	rr = f.do(t, http.MethodGet, "/api/v1/club-events/"+swID+"/rsvps", "swimA", "")
	if strings.Contains(rr.Body.String(), "sw-a2") || !strings.Contains(rr.Body.String(), "sw-a") {
		t.Fatalf("swimmer must only see own RSVP: %s", rr.Body.String())
	}
}

func TestCalendarFeedTokenAndICS(t *testing.T) {
	f := newEngagementFixture(t)
	createClubEvent(t, f, "")
	ageOnly := createClubEvent(t, f, `,"audience":"groups","group_codes":["AGE"]`)
	expectStatus(t, f.do(t, http.MethodPost, "/api/v1/calendar/feed-token", "swimA", ""), http.StatusForbidden, "swimmer feed")
	expectStatus(t, f.do(t, http.MethodPost, "/api/v1/calendar/feed-token?household_id=hh-a", "parentB", ""), http.StatusForbidden, "foreign feed")
	expectStatus(t, f.do(t, http.MethodPost, "/api/v1/calendar/feed-token", "admin", ""), http.StatusBadRequest, "admin without household")
	rr := f.do(t, http.MethodPost, "/api/v1/calendar/feed-token", "parentB", "")
	expectStatus(t, rr, http.StatusCreated, "rotate")
	var out struct {
		FeedPath string `json:"feed_path"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)

	ics := f.do(t, http.MethodGet, out.FeedPath, "", "")
	expectStatus(t, ics, http.StatusOK, "feed")
	if !strings.HasPrefix(ics.Header().Get("Content-Type"), "text/calendar") || !strings.Contains(ics.Body.String(), "SUMMARY:Pizza Social") || strings.Contains(ics.Body.String(), ageOnly) {
		t.Fatalf("unexpected feed: %s", ics.Body.String())
	}
	status := f.do(t, http.MethodGet, "/api/v1/calendar/feed-token", "parentB", "")
	if !strings.Contains(status.Body.String(), `"active":true`) || strings.Contains(status.Body.String(), "token_hash") {
		t.Fatalf("status leaked or inactive: %s", status.Body.String())
	}
	expectStatus(t, f.do(t, http.MethodDelete, "/api/v1/calendar/feed-token", "parentB", ""), http.StatusOK, "revoke")
	expectStatus(t, f.do(t, http.MethodGet, out.FeedPath, "", ""), http.StatusNotFound, "revoked feed")
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/calendar/feed/not-a-real-token-but-long-enough-xxxxxxxx.ics", "", ""), http.StatusNotFound, "bogus feed")
}

func TestWellnessPrivacyAndHeatmap(t *testing.T) {
	f := newEngagementFixture(t)
	body := `{"sleep_hours":7.5,"sleep_quality":4,"soreness":2,"energy":4,"stress":2,"mood":4,"note":"slept ok"}`
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/wellness/checkins", "swimA", body), http.StatusCreated, "swimmer self check-in")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/wellness/checkins", "swimA", body), http.StatusOK, "upsert same day")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/wellness/checkins", "swimA", strings.Replace(body, `{`, `{"swimmer_id":"sw-a2",`, 1)), http.StatusForbidden, "swimmer for sibling")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/wellness/checkins", "parentA", strings.Replace(body, `{`, `{"swimmer_id":"sw-a2",`, 1)), http.StatusCreated, "parent on behalf")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/wellness/checkins", "parentB", strings.Replace(body, `{`, `{"swimmer_id":"sw-a",`, 1)), http.StatusForbidden, "foreign parent")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/wellness/checkins", "coach", strings.Replace(body, `{`, `{"swimmer_id":"sw-a",`, 1)), http.StatusForbidden, "coach submit")
	expectStatus(t, f.do(t, http.MethodPut, "/api/v1/wellness/checkins", "swimA", `{"sleep_hours":7,"sleep_quality":9,"soreness":2,"energy":4,"stress":2,"mood":4}`), http.StatusUnprocessableEntity, "bad scale")

	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/wellness/swimmers/sw-a", "parentB", ""), http.StatusForbidden, "foreign trend")
	rr := f.do(t, http.MethodGet, "/api/v1/wellness/swimmers/sw-a", "parentA", "")
	expectStatus(t, rr, http.StatusOK, "household trend")
	if !strings.Contains(rr.Body.String(), "slept ok") {
		t.Fatalf("household should see note: %s", rr.Body.String())
	}
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/wellness/swimmers/sw-a", "coach", ""), http.StatusOK, "coach trend")
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/wellness/heatmap?group=senior", "parentA", ""), http.StatusForbidden, "parent heatmap")
	rr = f.do(t, http.MethodGet, "/api/v1/wellness/heatmap?group=senior&days=14", "coach", "")
	expectStatus(t, rr, http.StatusOK, "coach heatmap")
	var hm struct {
		Days     []string `json:"days"`
		Swimmers []struct {
			SwimmerID string `json:"swimmer_id"`
		} `json:"swimmers"`
		CheckedInToday int `json:"checked_in_today"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &hm)
	if len(hm.Days) != 14 || len(hm.Swimmers) != 2 || hm.CheckedInToday != 1 {
		t.Fatalf("unexpected heatmap: %s", rr.Body.String())
	}
}

func TestAttendanceSummaryScoping(t *testing.T) {
	f := newEngagementFixture(t)
	rr := f.do(t, http.MethodGet, "/api/v1/swimmers/sw-a/attendance-summary", "coach", "")
	expectStatus(t, rr, http.StatusOK, "coach summary")
	if !strings.Contains(rr.Body.String(), `"attendance_pct":100`) {
		t.Fatalf("unexpected summary: %s", rr.Body.String())
	}
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/swimmers/sw-a/attendance-summary", "parentA", ""), http.StatusOK, "own household")
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/swimmers/sw-a/attendance-summary", "parentB", ""), http.StatusForbidden, "foreign household")
	expectStatus(t, f.do(t, http.MethodGet, "/api/v1/swimmers/nope/attendance-summary", "coach", ""), http.StatusNotFound, "unknown swimmer")
}
