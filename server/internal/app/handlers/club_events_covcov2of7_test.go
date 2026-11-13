package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/events"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/roster"
	"github.com/luryann/aquify/server/internal/testutil"
)

var cov2xNow = time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)

// cov2xDirFault is a roster directory whose lookups can be made to fail, the
// way a database outage behind the real directory would.
type cov2xDirFault struct {
	*roster.StaticDirectory
	idsErr, swimmersErr, namesErr error
}

func (d *cov2xDirFault) Swimmers(org string) ([]roster.Swimmer, error) {
	if d.swimmersErr != nil {
		return nil, d.swimmersErr
	}
	return d.StaticDirectory.Swimmers(org)
}

func (d *cov2xDirFault) HouseholdIDsForPrincipal(p *appmiddleware.Principal, org string) ([]string, error) {
	if d.idsErr != nil {
		return nil, d.idsErr
	}
	return d.StaticDirectory.HouseholdIDsForPrincipal(p, org)
}

func (d *cov2xDirFault) HouseholdNames(org string) (map[string]string, error) {
	if d.namesErr != nil {
		return nil, d.namesErr
	}
	return d.StaticDirectory.HouseholdNames(org)
}

func (d *cov2xDirFault) heal() { d.idsErr, d.swimmersErr, d.namesErr = nil, nil, nil }

// cov2xBiller is a fake of the billing system the events service charges RSVP fees to.
type cov2xBiller struct {
	mu       sync.Mutex
	charges  []events.Charge
	released []string
}

func (b *cov2xBiller) ChargeRSVP(_ string, c events.Charge) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.charges = append(b.charges, c)
	return fmt.Sprintf("inv-%d", len(b.charges)), nil
}

func (b *cov2xBiller) ReleaseCharge(_ string, invoiceID, _ string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.released = append(b.released, invoiceID)
	return true, nil
}

func cov2xEvTokens(org string) tokenResolver {
	return tokenResolver{
		"admin":       {UserID: "u-admin", OrgID: org, Roles: []string{"admin"}},
		"adminParent": {UserID: "u-adminparent", OrgID: org, HouseholdID: "hh-a", Roles: []string{"admin", "parent"}},
		"coach":       {UserID: "u-coach", OrgID: org, Roles: []string{"coach"}},
		"parentA":     {UserID: "u-pa", OrgID: org, HouseholdID: "hh-a", Roles: []string{"parent"}},
		"parentB":     {UserID: "u-pb", OrgID: org, HouseholdID: "hh-b", Roles: []string{"parent"}},
		"parentMulti": {UserID: "u-pm", OrgID: org, Roles: []string{"parent"}},
		"parentNone":  {UserID: "u-pn", OrgID: org, Roles: []string{"parent"}},
		"swimA":       {UserID: "u-sa", OrgID: org, HouseholdID: "hh-a", SwimmerID: "sw-a", Roles: []string{"swimmer"}},
		"swimNobody":  {UserID: "u-sn", OrgID: org, HouseholdID: "hh-a", Roles: []string{"swimmer"}},
		"operator":    {UserID: "u-op", OrgID: org, Roles: []string{"platform_admin"}},
	}
}

type cov2xEvFixture struct {
	t      *testing.T
	org    string
	svc    *events.Service
	dir    *cov2xDirFault
	audit  *audit.Service
	biller *cov2xBiller
	h      *ClubEventsHandlers
	router http.Handler
}

func cov2xNewEvFixtureWith(t *testing.T, org string, svc *events.Service) *cov2xEvFixture {
	t.Helper()
	dir := &cov2xDirFault{StaticDirectory: &roster.StaticDirectory{
		SwimmerList: []roster.Swimmer{
			{ID: "sw-a", OrgID: org, HouseholdID: "hh-a", FirstName: "Ada", LastName: "A", DisplayName: "Ada A", GroupCodes: []string{"SENIOR"}},
			{ID: "sw-a2", OrgID: org, HouseholdID: "hh-a", FirstName: "Abe", LastName: "A", DisplayName: "Abe A", GroupCodes: []string{"AGE"}},
			{ID: "sw-b", OrgID: org, HouseholdID: "hh-b", FirstName: "Bo", LastName: "B", DisplayName: "Bo B", GroupCodes: []string{"SENIOR"}},
			{ID: "sw-c", OrgID: org, HouseholdID: "hh-c", FirstName: "Cy", LastName: "C", DisplayName: "Cy C", GroupCodes: []string{"AGE"}},
		},
		ParentHouseholds: map[string][]string{"u-pm": {"hh-c", "hh-b"}},
		Households:       map[string]string{"hh-a": "A Family", "hh-b": "B Family", "hh-c": "C Family"},
	}}
	svc.SetClock(func() time.Time { return cov2xNow })
	biller := &cov2xBiller{}
	svc.SetBiller(biller)
	auditSvc := audit.NewService()
	h := NewClubEventsHandlers(svc, dir, auditSvc, nil)
	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		h.RegisterPublicRoutes(api)
		api.Group(func(p chi.Router) {
			p.Use(appmiddleware.Auth(cov2xEvTokens(org)))
			p.Use(appmiddleware.RequireOrgScope)
			h.RegisterRoutes(p)
		})
	})
	return &cov2xEvFixture{t: t, org: org, svc: svc, dir: dir, audit: auditSvc, biller: biller, h: h, router: r}
}

func cov2xNewEvFixture(t *testing.T) *cov2xEvFixture {
	return cov2xNewEvFixtureWith(t, "org-cov2x-ev", events.NewService())
}

func (f *cov2xEvFixture) do(method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", f.org)
	}
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	return rr
}

// expect issues a request and requires the status and (when non-empty) the error code.
func (f *cov2xEvFixture) expect(status int, code, method, path, token, body string) map[string]any {
	f.t.Helper()
	rr := f.do(method, path, token, body)
	if rr.Code != status {
		f.t.Fatalf("%s %s as %s: want %d, got %d: %s", method, path, token, status, rr.Code, rr.Body.String())
	}
	m := cov2xDecode(f.t, rr)
	if code != "" && cov2xErrCode(m) != code {
		f.t.Fatalf("%s %s as %s: want code %q, got %s", method, path, token, code, rr.Body.String())
	}
	return m
}

func (f *cov2xEvFixture) actions() map[string]int {
	f.t.Helper()
	evs, err := f.audit.Query(audit.Filter{OrgID: f.org, Limit: 500})
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]int{}
	for _, ev := range evs {
		out[ev.Action]++
	}
	return out
}

func cov2xEventBody(title string, extra string) string {
	return fmt.Sprintf(`{"title":%q,"category":"social","starts_at":%q,"rsvp_enabled":true%s}`,
		title, cov2xNow.Add(5*24*time.Hour).Format(time.RFC3339), extra)
}

// create makes an event as coach and returns its ID.
func (f *cov2xEvFixture) create(title, extra string) string {
	f.t.Helper()
	m := f.expect(http.StatusCreated, "", http.MethodPost, "/club-events", "coach", cov2xEventBody(title, extra))
	return cov2xStr(f.t, cov2xObj(f.t, m, "event"), "id")
}

func (f *cov2xEvFixture) rsvpPath(id string) string { return "/club-events/" + id + "/rsvps" }

func TestCov2xClubEventsRequireAuthRoleAndOrg(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	id := f.create("Pizza", "")
	routes := []cov2xRoute{
		{http.MethodGet, "/club-events", false},
		{http.MethodGet, "/club-events/" + id, false},
		{http.MethodGet, f.rsvpPath(id), false},
		{http.MethodPut, f.rsvpPath(id), false},
	}
	authOnly := chi.NewRouter()
	authOnly.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov2xEvTokens(f.org)))
		f.h.RegisterRoutes(p)
	})
	for _, rt := range append(routes,
		cov2xRoute{http.MethodPost, "/club-events", false},
		cov2xRoute{http.MethodPut, "/club-events/" + id, false},
		cov2xRoute{http.MethodDelete, "/club-events/" + id, false},
		cov2xRoute{http.MethodGet, "/club-events/" + id + "/rsvps/export", false},
		cov2xRoute{http.MethodGet, "/calendar/feed-token", false},
		cov2xRoute{http.MethodPost, "/calendar/feed-token", false},
		cov2xRoute{http.MethodDelete, "/calendar/feed-token", false},
	) {
		f.expect(http.StatusUnauthorized, "", rt.method, rt.path, "", "{}")
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer admin")
		rr := httptest.NewRecorder()
		authOnly.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest || cov2xErrCode(cov2xDecode(t, rr)) != "missing_org_scope" {
			t.Fatalf("%s %s without org: %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
	}
	// An operator role with no club role has no business in club events.
	for _, rt := range routes {
		f.expect(http.StatusForbidden, "forbidden", rt.method, rt.path, "operator", "{}")
	}
}

func TestCov2xClubEventsListFiltersAndAudienceScoping(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	day := func(d int) string { return cov2xNow.AddDate(0, 0, d).Format("2006-01-02") }
	f.create("Early", fmt.Sprintf(`,"starts_at":%q`, day(5)+"T18:00:00Z"))
	f.expect(http.StatusCreated, "", http.MethodPost, "/club-events", "coach",
		fmt.Sprintf(`{"title":"AGE only","category":"clinic","starts_at":%q,"audience":"groups","group_codes":["age"]}`, day(10)+"T18:00:00Z"))
	f.expect(http.StatusCreated, "", http.MethodPost, "/club-events", "coach",
		fmt.Sprintf(`{"title":"All day","category":"banquet","starts_at":%q,"all_day":true}`, day(15)+"T00:00:00Z"))

	titles := func(m map[string]any) []string {
		out := []string{}
		for _, raw := range m["events"].([]any) {
			out = append(out, raw.(map[string]any)["title"].(string))
		}
		return out
	}
	all := f.expect(http.StatusOK, "", http.MethodGet, "/club-events", "admin", "")
	if got := strings.Join(titles(all), ","); got != "Early,AGE only,All day" || cov2xNum(all, "total") != 3 {
		t.Fatalf("staff see everything in start order, got %s (%v)", got, all)
	}
	if _, has := all["events"].([]any)[0].(map[string]any)["my_rsvps"]; has {
		t.Fatal("staff views must not carry per-family fields")
	}

	// Audience scoping: B family has no AGE swimmer.
	if got := strings.Join(titles(f.expect(http.StatusOK, "", http.MethodGet, "/club-events", "parentB", "")), ","); got != "Early,All day" {
		t.Fatalf("parentB: %s", got)
	}
	pa := f.expect(http.StatusOK, "", http.MethodGet, "/club-events", "parentA", "")
	if got := strings.Join(titles(pa), ","); got != "Early,AGE only,All day" {
		t.Fatalf("parentA: %s", got)
	}
	ageView := pa["events"].([]any)[1].(map[string]any)
	if ids := ageView["eligible_swimmer_ids"].([]any); len(ids) != 1 || ids[0] != "sw-a2" {
		t.Fatalf("only the AGE swimmer is eligible: %v", ageView)
	}
	if es := ageView["eligible_swimmers"].([]any); len(es) != 1 || es[0].(map[string]any)["display_name"] != "Abe A" || len(es[0].(map[string]any)["group_codes"].([]any)) != 0 {
		t.Fatalf("eligible swimmers must be minimal projections: %v", ageView)
	}
	// A swimmer sees only the events their own groups reach.
	if got := strings.Join(titles(f.expect(http.StatusOK, "", http.MethodGet, "/club-events", "swimA", "")), ","); got != "Early,All day" {
		t.Fatalf("swimA: %s", got)
	}
	if got := strings.Join(titles(f.expect(http.StatusOK, "", http.MethodGet, "/club-events", "swimNobody", "")), ","); got != "Early,All day" {
		t.Fatalf("swimmer account without a swimmer: %s", got)
	}

	// Time window filters accept dates and RFC3339; "to" includes its whole day.
	for _, tc := range []struct{ q, want string }{
		{"?from=" + day(6), "AGE only,All day"},
		{"?from=" + day(6) + "T00:00:00Z", "AGE only,All day"},
		{"?to=" + day(5), "Early"},
		{"?to=" + day(4), ""},
		{"?from=" + day(6) + "&to=" + day(10), "AGE only"},
	} {
		if got := strings.Join(titles(f.expect(http.StatusOK, "", http.MethodGet, "/club-events"+tc.q, "admin", "")), ","); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.q, got, tc.want)
		}
	}
	f.expect(http.StatusBadRequest, "bad_request", http.MethodGet, "/club-events?from=yesterday", "admin", "")
	f.expect(http.StatusBadRequest, "bad_request", http.MethodGet, "/club-events?to=2030-13-40", "admin", "")

	page := f.expect(http.StatusOK, "", http.MethodGet, "/club-events?limit=1&offset=1", "admin", "")
	if got := strings.Join(titles(page), ","); got != "AGE only" || cov2xNum(page, "total") != 3 || cov2xNum(page, "limit") != 1 || cov2xNum(page, "offset") != 1 {
		t.Fatalf("pagination: %v", page)
	}
}

func TestCov2xClubEventLifecycleAndAudit(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	f.expect(http.StatusBadRequest, "", http.MethodPost, "/club-events", "coach", `{nope`)
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPost, "/club-events", "coach", `{"category":"social"}`)
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPost, "/club-events", "coach",
		cov2xEventBody("Bad audience", `,"audience":"groups"`))
	for _, tok := range []string{"parentA", "swimA"} {
		f.expect(http.StatusForbidden, "forbidden", http.MethodPost, "/club-events", tok, cov2xEventBody("x", ""))
	}

	id := f.create("Banquet", `,"capacity":10`)
	got := cov2xObj(t, f.expect(http.StatusOK, "", http.MethodGet, "/club-events/"+id, "admin", ""), "event")
	if got["title"] != "Banquet" || got["rsvp_open"] != true {
		t.Fatalf("get: %v", got)
	}
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, "/club-events/"+uuid.NewString(), "admin", "")

	update := func(token, extra string) map[string]any {
		return f.expect(http.StatusOK, "", http.MethodPut, "/club-events/"+id, token, cov2xEventBody("Banquet renamed", extra))
	}
	if ev := cov2xObj(t, update("coach", `,"capacity":12`), "event"); ev["title"] != "Banquet renamed" || cov2xNum(ev, "capacity") != 12 {
		t.Fatalf("update: %v", ev)
	}
	f.expect(http.StatusForbidden, "forbidden", http.MethodPut, "/club-events/"+id, "parentA", cov2xEventBody("x", ""))
	f.expect(http.StatusNotFound, "not_found", http.MethodPut, "/club-events/"+uuid.NewString(), "coach", cov2xEventBody("x", ""))
	f.expect(http.StatusBadRequest, "", http.MethodPut, "/club-events/"+id, "coach", `{nope`)
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPut, "/club-events/"+id, "coach", cov2xEventBody("x", `,"fee_cents":-5`))

	// Cancelling is audited as its own action and blocks further RSVPs.
	update("coach", `,"status":"cancelled"`)
	f.expect(http.StatusConflict, "event_cancelled", http.MethodPut, f.rsvpPath(id), "coach", `{"household_id":"hh-a","response":"yes"}`)

	f.expect(http.StatusForbidden, "forbidden", http.MethodDelete, "/club-events/"+id, "parentA", "")
	f.expect(http.StatusNotFound, "not_found", http.MethodDelete, "/club-events/"+uuid.NewString(), "coach", "")
	if res := f.expect(http.StatusOK, "", http.MethodDelete, "/club-events/"+id, "coach", ""); res["deleted"] != true {
		t.Fatalf("delete: %v", res)
	}
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, "/club-events/"+id, "admin", "")

	acts := f.actions()
	for _, want := range []string{"club_event.create", "club_event.update", "club_event.cancel", "club_event.delete"} {
		if acts[want] == 0 {
			t.Fatalf("missing audit action %s: %v", want, acts)
		}
	}
	if acts["club_event.cancel"] != 1 || acts["club_event.update"] != 1 {
		t.Fatalf("cancel must not also be logged as a plain update: %v", acts)
	}
}

func TestCov2xClubEventUpdateConflictsWithExistingRSVPs(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	id := f.create("Dinner", `,"capacity":5`)
	f.expect(http.StatusOK, "", http.MethodPut, f.rsvpPath(id), "parentA", `{"response":"yes","guest_count":2}`)

	f.expect(http.StatusConflict, "rsvp_mode_locked", http.MethodPut, "/club-events/"+id, "coach", cov2xEventBody("Dinner", `,"rsvp_mode":"swimmer","capacity":5`))
	f.expect(http.StatusConflict, "capacity_below_headcount", http.MethodPut, "/club-events/"+id, "coach", cov2xEventBody("Dinner", `,"capacity":2`))
	ev := cov2xObj(t, f.expect(http.StatusOK, "", http.MethodGet, "/club-events/"+id, "admin", ""), "event")
	if cov2xNum(ev, "capacity") != 5 || ev["rsvp_mode"] != "household" {
		t.Fatalf("rejected updates must not change the event: %v", ev)
	}
}

func TestCov2xClubEventHouseholdRSVPsAndFees(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	id := f.create("Gala", `,"fee_cents":1000,"capacity":6`)
	path := f.rsvpPath(id)

	res := f.expect(http.StatusOK, "", http.MethodPut, path, "parentA", `{"response":"yes","guest_count":1,"note":"  vegetarian "}`)
	rsvp := cov2xObj(t, res, "rsvp")
	if res["billing"] != "charged" || rsvp["household_id"] != "hh-a" || cov2xNum(rsvp, "guest_count") != 1 || rsvp["note"] != "vegetarian" || rsvp["invoice_id"] != "inv-1" {
		t.Fatalf("first RSVP: %v", res)
	}
	if len(f.biller.charges) != 1 || f.biller.charges[0].Quantity != 2 || f.biller.charges[0].UnitAmountCents != 1000 || f.biller.charges[0].HouseholdID != "hh-a" {
		t.Fatalf("fee must cover the family headcount: %+v", f.biller.charges)
	}
	res = f.expect(http.StatusOK, "", http.MethodPut, path, "parentA", `{"response":"no"}`)
	if res["billing"] != "released" || len(f.biller.released) != 1 || f.biller.released[0] != "inv-1" {
		t.Fatalf("declining must release the unpaid fee: %v released=%v", res, f.biller.released)
	}
	acts := f.actions()
	if acts["club_event.rsvp.fee_charged"] != 1 || acts["club_event.rsvp.fee_released"] != 1 || acts["club_event.rsvp.staff_update"] != 0 {
		t.Fatalf("billing audit: %v", acts)
	}

	// Scope rules.
	f.expect(http.StatusForbidden, "forbidden", http.MethodPut, path, "swimA", `{"response":"yes"}`)
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPut, path, "parentA", `{"household_id":"hh-b","response":"yes"}`)
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPut, path, "parentNone", `{"response":"yes"}`)
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPut, path, "parentMulti", `{"response":"yes"}`) // ambiguous: three households
	f.expect(http.StatusOK, "", http.MethodPut, path, "parentMulti", `{"household_id":"hh-c","response":"maybe"}`)
	f.expect(http.StatusOK, "", http.MethodPut, path, "parentB", `{"response":"yes","guest_count":3}`)
	f.expect(http.StatusConflict, "event_full", http.MethodPut, path, "parentMulti", `{"household_id":"hh-b","response":"yes","guest_count":6}`)

	// Body validation.
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPut, path, "parentA", `{"response":"perhaps"}`)
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPut, path, "parentA", `{"response":"yes","guest_count":51}`)
	f.expect(http.StatusBadRequest, "", http.MethodPut, path, "parentA", `{nope`)
	f.expect(http.StatusNotFound, "not_found", http.MethodPut, f.rsvpPath(uuid.NewString()), "parentA", `{"response":"yes"}`)

	// Staff may answer on behalf of a family and are audited doing so.
	f.expect(http.StatusOK, "", http.MethodPut, path, "coach", `{"household_id":"hh-a","response":"yes"}`)
	if f.actions()["club_event.rsvp.staff_update"] != 1 {
		t.Fatalf("staff RSVP must be audited: %v", f.actions())
	}
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPut, path, "coach", `{"response":"yes"}`)                         // no household
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPut, path, "coach", `{"household_id":"hh-zzz","response":"yes"}`) // unknown household
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPut, path, "coach", `{"swimmer_id":"sw-zzz","response":"yes"}`)
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPut, path, "coach", `{"swimmer_id":"sw-a","household_id":"hh-b","response":"yes"}`)
	if r := f.expect(http.StatusOK, "", http.MethodPut, path, "coach", `{"swimmer_id":"sw-b","response":"no"}`); cov2xObj(t, r, "rsvp")["household_id"] != "hh-b" {
		t.Fatalf("household must come from the swimmer: %v", r)
	}
}

func TestCov2xClubEventRSVPWindowAndAvailability(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	deadline := cov2xNow.Add(-24 * time.Hour).Format(time.RFC3339)
	closed := f.create("Closed", fmt.Sprintf(`,"rsvp_deadline":%q`, deadline))
	f.expect(http.StatusConflict, "rsvp_closed", http.MethodPut, f.rsvpPath(closed), "parentA", `{"response":"yes"}`)
	if v := cov2xObj(t, f.expect(http.StatusOK, "", http.MethodGet, "/club-events/"+closed, "parentA", ""), "event"); v["rsvp_open"] != false {
		t.Fatalf("closed event must report rsvp_open=false: %v", v)
	}
	f.expect(http.StatusOK, "", http.MethodPut, f.rsvpPath(closed), "coach", `{"household_id":"hh-a","response":"yes"}`) // staff bypass the window

	off := f.expect(http.StatusCreated, "", http.MethodPost, "/club-events", "coach",
		fmt.Sprintf(`{"title":"Info only","category":"social","starts_at":%q}`, cov2xNow.Add(48*time.Hour).Format(time.RFC3339)))
	offID := cov2xStr(t, cov2xObj(t, off, "event"), "id")
	f.expect(http.StatusConflict, "rsvp_disabled", http.MethodPut, f.rsvpPath(offID), "coach", `{"household_id":"hh-a","response":"yes"}`)
}

func TestCov2xClubEventSwimmerRSVPsAndRoster(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	id := f.create("Relay day", `,"rsvp_mode":"swimmer"`)
	path := f.rsvpPath(id)

	// A swimmer answers for themself; a body swimmer_id is ignored for them.
	f.expect(http.StatusOK, "", http.MethodPut, path, "swimA", `{"swimmer_id":"sw-a2","response":"yes"}`)
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPut, path, "swimNobody", `{"response":"yes"}`)
	f.expect(http.StatusOK, "", http.MethodPut, path, "parentA", `{"swimmer_id":"sw-a2","response":"maybe"}`)
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPut, path, "parentA", `{"swimmer_id":"sw-b","response":"yes"}`)
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPut, path, "parentA", `{"response":"yes"}`)
	f.expect(http.StatusOK, "", http.MethodPut, path, "parentB", `{"swimmer_id":"sw-b","response":"yes"}`)
	f.expect(http.StatusOK, "", http.MethodPut, path, "coach", `{"swimmer_id":"sw-c","response":"no"}`)
	f.expect(http.StatusUnprocessableEntity, "validation_error", http.MethodPut, path, "coach", `{"household_id":"hh-a","response":"yes"}`) // swimmer mode needs a swimmer

	// Staff roster is ordered yes, maybe, no; then household; then swimmer.
	staff := f.expect(http.StatusOK, "", http.MethodGet, path, "coach", "")
	var order []string
	for _, raw := range staff["rsvps"].([]any) {
		row := raw.(map[string]any)
		order = append(order, row["response"].(string)+":"+row["swimmer_name"].(string)+"@"+row["household_name"].(string))
	}
	if got := strings.Join(order, ","); got != "yes:Ada A@A Family,yes:Bo B@B Family,maybe:Abe A@A Family,no:Cy C@C Family" {
		t.Fatalf("roster order: %s", got)
	}
	if sum := cov2xObj(t, staff, "summary"); cov2xNum(sum, "yes") != 2 || cov2xNum(sum, "maybe") != 1 || cov2xNum(sum, "no") != 1 || cov2xNum(sum, "yes_headcount") != 2 {
		t.Fatalf("summary: %v", sum)
	}
	// A parent sees every row of their household (siblings included) and nobody else's; the summary still covers everyone.
	pa := f.expect(http.StatusOK, "", http.MethodGet, path, "parentA", "")
	if rows := pa["rsvps"].([]any); len(rows) != 2 || rows[0].(map[string]any)["household_id"] != "hh-a" || rows[1].(map[string]any)["household_id"] != "hh-a" {
		t.Fatalf("parentA rows: %v", rows)
	}
	if rows := f.expect(http.StatusOK, "", http.MethodGet, path, "swimA", "")["rsvps"].([]any); len(rows) != 1 || rows[0].(map[string]any)["swimmer_id"] != "sw-a" {
		t.Fatalf("a swimmer sees only their own RSVP, not a sibling's: %v", rows)
	}
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, f.rsvpPath(uuid.NewString()), "coach", "")

	// Same-response rows are ordered by household then swimmer name.
	f.expect(http.StatusOK, "", http.MethodPut, path, "coach", `{"swimmer_id":"sw-a","response":"yes"}`)
	f.expect(http.StatusOK, "", http.MethodPut, path, "coach", `{"swimmer_id":"sw-a2","response":"yes"}`)
	order = nil
	for _, raw := range f.expect(http.StatusOK, "", http.MethodGet, path, "coach", "")["rsvps"].([]any) {
		row := raw.(map[string]any)
		order = append(order, row["response"].(string)+":"+row["swimmer_name"].(string))
	}
	if got := strings.Join(order, ","); got != "yes:Abe A,yes:Ada A,yes:Bo B,no:Cy C" {
		t.Fatalf("tie-break order: %s", got)
	}

	// Audience-restricted swimmer event: only eligible swimmers may be answered for.
	age := f.create("AGE clinic", `,"rsvp_mode":"swimmer","audience":"groups","group_codes":["AGE"]`)
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPut, f.rsvpPath(age), "parentA", `{"swimmer_id":"sw-a","response":"yes"}`)
	f.expect(http.StatusOK, "", http.MethodPut, f.rsvpPath(age), "parentA", `{"swimmer_id":"sw-a2","response":"yes"}`)
	f.expect(http.StatusNotFound, "not_found", http.MethodPut, f.rsvpPath(age), "parentB", `{"swimmer_id":"sw-b","response":"yes"}`)
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, f.rsvpPath(age), "parentB", "")
}

func TestCov2xClubEventHouseholdRSVPVisibleToSwimmersOfThatHousehold(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	id := f.create("Picnic", "")
	f.expect(http.StatusOK, "", http.MethodPut, f.rsvpPath(id), "parentA", `{"response":"yes"}`)
	f.expect(http.StatusOK, "", http.MethodPut, f.rsvpPath(id), "parentB", `{"response":"no"}`)
	rows := f.expect(http.StatusOK, "", http.MethodGet, f.rsvpPath(id), "swimA", "")["rsvps"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["household_id"] != "hh-a" {
		t.Fatalf("a swimmer sees their household's RSVP and nobody else's: %v", rows)
	}
	view := cov2xObj(t, f.expect(http.StatusOK, "", http.MethodGet, "/club-events/"+id, "swimA", ""), "event")
	if mine := view["my_rsvps"].([]any); len(mine) != 1 {
		t.Fatalf("my_rsvps: %v", mine)
	}
}

func TestCov2xClubEventExportRSVPs(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	id := f.create("=Spring Social, 2030!", `,"fee_cents":500`)
	f.expect(http.StatusOK, "", http.MethodPut, f.rsvpPath(id), "parentA", `{"response":"yes","guest_count":2,"note":"+1 cmd"}`)
	f.expect(http.StatusOK, "", http.MethodPut, f.rsvpPath(id), "parentB", `{"response":"no","note":"@away"}`)

	rr := f.do(http.MethodGet, f.rsvpPath(id)+"/export", "coach", "")
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/csv") || rr.Header().Get("Cache-Control") != "no-store" ||
		rr.Header().Get("Content-Disposition") != `attachment; filename="spring-social-2030-rsvps.csv"` {
		t.Fatalf("headers: %d %v", rr.Code, rr.Header())
	}
	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "Event,Household,Swimmer,Response,Guests,Headcount,Note,Invoice ID,Responded At") {
		t.Fatalf("csv shape: %q", rr.Body.String())
	}
	if !strings.Contains(lines[1], `"'=Spring Social, 2030!",A Family,,yes,2,3,'+1 cmd,inv-1,`) {
		t.Fatalf("yes row must neutralise formulas and carry headcount + invoice: %q", lines[1])
	}
	if !strings.Contains(lines[2], `B Family,,no,0,,'@away,,`) {
		t.Fatalf("no row: %q", lines[2])
	}
	if f.actions()["club_event.rsvp.export"] != 1 {
		t.Fatalf("export must be audited: %v", f.actions())
	}
	f.expect(http.StatusForbidden, "forbidden", http.MethodGet, f.rsvpPath(id)+"/export", "parentA", "")
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, f.rsvpPath(uuid.NewString())+"/export", "coach", "")
}

func TestCov2xClubEventDirectoryOutagesAreInternalErrors(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	hh := f.create("Household", "")
	sw := f.create("Swimmer", `,"rsvp_mode":"swimmer"`)
	boom := errors.New("directory unavailable: secret-detail")

	check := func(label string, tc ...struct {
		method, path, token, body string
	}) {
		t.Helper()
		for _, c := range tc {
			rr := f.do(c.method, c.path, c.token, c.body)
			if rr.Code != http.StatusInternalServerError || cov2xErrCode(cov2xDecode(t, rr)) != "internal_error" || strings.Contains(rr.Body.String(), "secret-detail") {
				t.Fatalf("%s: %s %s as %s: want clean 500, got %d %s", label, c.method, c.path, c.token, rr.Code, rr.Body.String())
			}
		}
		f.dir.heal()
	}
	type req = struct{ method, path, token, body string }

	f.dir.idsErr = boom
	check("household lookup",
		req{http.MethodGet, "/club-events", "parentA", ""},
		req{http.MethodGet, "/club-events/" + hh, "parentA", ""},
		req{http.MethodPut, f.rsvpPath(hh), "parentA", `{"response":"yes"}`},
		req{http.MethodGet, "/calendar/feed-token", "parentA", ""},
		req{http.MethodPost, "/calendar/feed-token", "parentA", ""},
		req{http.MethodDelete, "/calendar/feed-token", "parentA", ""},
	)
	f.dir.swimmersErr = boom
	check("swimmer lookup",
		req{http.MethodGet, "/club-events", "parentA", ""},
		req{http.MethodGet, f.rsvpPath(hh), "coach", ""},
		req{http.MethodGet, f.rsvpPath(hh) + "/export", "coach", ""},
		req{http.MethodPut, f.rsvpPath(sw), "coach", `{"swimmer_id":"sw-a","response":"yes"}`},
	)
	f.dir.namesErr = boom
	check("household names",
		req{http.MethodGet, f.rsvpPath(hh), "coach", ""},
		req{http.MethodGet, f.rsvpPath(hh) + "/export", "coach", ""},
		req{http.MethodPut, f.rsvpPath(hh), "coach", `{"household_id":"hh-a","response":"yes"}`},
		req{http.MethodGet, "/calendar/feed-token?household_id=hh-a", "admin", ""},
	)
	// Healthy again: the same calls succeed, proving the failures were the outage.
	f.expect(http.StatusOK, "", http.MethodGet, f.rsvpPath(hh), "coach", "")
}

func TestCov2xWriteServiceErrorContract(t *testing.T) {
	t.Parallel()
	h := &ClubEventsHandlers{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&events.ValidationError{Field: "title", Message: "is required"}, http.StatusUnprocessableEntity, "validation_error"},
		{events.ErrNotFound, http.StatusNotFound, "not_found"},
		{events.ErrRSVPModeLocked, http.StatusConflict, "rsvp_mode_locked"},
		{events.ErrCapacityBelowYes, http.StatusConflict, "capacity_below_headcount"},
		{events.ErrEventFull, http.StatusConflict, "event_full"},
		{events.ErrRSVPClosed, http.StatusConflict, "rsvp_closed"},
		{events.ErrRSVPDisabled, http.StatusConflict, "rsvp_disabled"},
		{events.ErrEventCancelled, http.StatusConflict, "event_cancelled"},
		{fmt.Errorf("%w: response must be yes", events.ErrInvalidRSVP), http.StatusUnprocessableEntity, "validation_error"},
		{events.ErrFeedTokenMissing, http.StatusNotFound, "not_found"},
		{errors.New("pq: broken"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		h.writeServiceError(rr, httptest.NewRequest(http.MethodGet, "/x", nil), tc.err, "fallback message")
		if rr.Code != tc.status || cov2xErrCode(cov2xDecode(t, rr)) != tc.code {
			t.Fatalf("%v: got %d %s, want %d %s", tc.err, rr.Code, rr.Body.String(), tc.status, tc.code)
		}
		if tc.code == "internal_error" && (!strings.Contains(rr.Body.String(), "fallback message") || strings.Contains(rr.Body.String(), "pq: broken")) {
			t.Fatalf("internal errors must show only the fallback: %s", rr.Body.String())
		}
	}
}

func TestCov2xClubEventHelpers(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"": "", "plain": "plain", "=SUM(A1)": "'=SUM(A1)", "+1": "'+1", "-1": "'-1", "@x": "'@x", "\tx": "'\tx", "\rx": "'\rx", "a=b": "a=b"} {
		if got := csvSafe(in); got != want {
			t.Fatalf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Pizza Social":                    "pizza-social",
		"  --Hello,   World!!  ":          "hello-world",
		"!!!":                             "event",
		"":                                "event",
		strings.Repeat("a", 80):           strings.Repeat("a", 60),
		strings.Repeat("ab ", 30) + "end": strings.TrimRight(strings.ReplaceAll(strings.Repeat("ab ", 30), " ", "-"), "-")[:60],
	} {
		if got := slugify(in); got != strings.Trim(want, "-") {
			t.Fatalf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
	for r, want := range map[string]int{"yes": 0, "maybe": 1, "no": 2, "": 2} {
		if got := responseRank(r); got != want {
			t.Fatalf("responseRank(%q) = %d, want %d", r, got, want)
		}
	}
	if tm, err := parseTimeParam("  ", false); tm != nil || err != nil {
		t.Fatalf("blank must mean no bound: %v %v", tm, err)
	}
	if tm, _ := parseTimeParam("2030-06-20T10:30:00Z", true); !tm.Equal(time.Date(2030, 6, 20, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("RFC3339 is taken verbatim even for end-of-day: %v", tm)
	}
	if tm, _ := parseTimeParam("2030-06-20", false); !tm.Equal(time.Date(2030, 6, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date start: %v", tm)
	}
	if tm, _ := parseTimeParam("2030-06-20", true); !tm.Equal(time.Date(2030, 6, 21, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date end-of-day: %v", tm)
	}
	if _, err := parseTimeParam("20/06/2030", false); err == nil {
		t.Fatal("non-ISO dates must be rejected")
	}
}

func TestCov2xClubEventCalendarItemAndSortKey(t *testing.T) {
	t.Parallel()
	start := time.Date(2030, 7, 1, 18, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	timed := ClubEventCalendarItem(&events.Event{ID: "e1", Title: "Social", Category: "team_meeting", Description: "Bring food", Location: "Pool",
		StartsAt: start, EndsAt: &end, RSVPEnabled: true, FeeCents: 1250, Status: "scheduled", UpdatedAt: cov2xNow})
	if timed.UID != "club-event-e1@aquify" || timed.Title != "Social" || timed.Categories != "team meeting" || timed.AllDay || !timed.Start.Equal(start) ||
		timed.End == nil || !timed.End.Equal(end) || timed.Cancelled ||
		timed.Description != "Bring food\n\nRSVP in the parent portal. Fee: $12.50 per person." {
		t.Fatalf("timed item: %+v", timed)
	}
	free := ClubEventCalendarItem(&events.Event{ID: "e2", Title: "Free", StartsAt: start, RSVPEnabled: true, Status: "scheduled"})
	if free.Description != "RSVP in the parent portal." {
		t.Fatalf("free RSVP line: %q", free.Description)
	}
	allDay := ClubEventCalendarItem(&events.Event{ID: "e3", Title: "Camp", StartsAt: time.Date(2030, 7, 4, 0, 0, 0, 0, time.UTC), AllDay: true, Status: "scheduled"})
	if !allDay.AllDay || allDay.StartDate != "2030-07-04" || allDay.EndDate != "2030-07-04" || allDay.Description != "" {
		t.Fatalf("all-day item: %+v", allDay)
	}
	multi := time.Date(2030, 7, 6, 0, 0, 0, 0, time.UTC)
	span := ClubEventCalendarItem(&events.Event{ID: "e4", Title: "Camp", StartsAt: time.Date(2030, 7, 4, 0, 0, 0, 0, time.UTC), EndsAt: &multi, AllDay: true, Status: "cancelled"})
	if span.EndDate != "2030-07-06" || !span.Cancelled || span.Title != "Cancelled: Camp" {
		t.Fatalf("multi-day cancelled item: %+v", span)
	}
	if calendarSortKey(allDay) != "2030-07-04T00:00:00Z" || calendarSortKey(timed) != "2030-07-01T18:00:00Z" {
		t.Fatalf("sort keys: %q %q", calendarSortKey(allDay), calendarSortKey(timed))
	}
}

func TestCov2xCalendarFeedTokenOwnership(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)

	if st := f.expect(http.StatusOK, "", http.MethodGet, "/calendar/feed-token", "parentA", ""); st["active"] != false || st["feed"] != nil {
		t.Fatalf("no feed yet: %v", st)
	}
	f.expect(http.StatusBadRequest, "bad_request", http.MethodPost, "/calendar/feed-token", "admin", "")
	f.expect(http.StatusNotFound, "not_found", http.MethodPost, "/calendar/feed-token?household_id=hh-ghost", "admin", "")
	f.expect(http.StatusForbidden, "forbidden", http.MethodPost, "/calendar/feed-token", "coach", "")
	f.expect(http.StatusForbidden, "forbidden", http.MethodPost, "/calendar/feed-token", "swimA", "")
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPost, "/calendar/feed-token", "parentNone", "")
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPost, "/calendar/feed-token?household_id=hh-a", "parentB", "")

	// An admin names the household explicitly.
	adm := f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token?household_id=hh-b", "admin", "")
	if adm["household_id"] != "hh-b" || adm["active"] != true || !strings.HasPrefix(adm["feed_path"].(string), "/api/v1/calendar/feed/") || !strings.HasSuffix(adm["feed_path"].(string), ".ics") {
		t.Fatalf("admin rotate: %v", adm)
	}
	// An admin who is also a parent defaults to their own household.
	if r := f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token", "adminParent", ""); r["household_id"] != "hh-a" {
		t.Fatalf("admin+parent default: %v", r)
	}
	// A multi-household parent defaults to the lowest linked household, and may name any linked one.
	if r := f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token", "parentMulti", ""); r["household_id"] != "hh-b" {
		t.Fatalf("multi default: %v", r)
	}
	if r := f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token?household_id=hh-c", "parentMulti", ""); r["household_id"] != "hh-c" {
		t.Fatalf("multi explicit: %v", r)
	}
	f.expect(http.StatusForbidden, "household_scope_violation", http.MethodPost, "/calendar/feed-token?household_id=hh-a", "parentMulti", "")

	// Status never reveals the secret; rotating invalidates the old link; revoking twice is a 404.
	first := f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token", "parentA", "")
	if rr := f.do(http.MethodGet, "/calendar/feed-token", "parentA", ""); !strings.Contains(rr.Body.String(), `"active":true`) ||
		strings.Contains(rr.Body.String(), first["token"].(string)) || strings.Contains(rr.Body.String(), "token_hash") {
		t.Fatalf("status must not leak the secret: %s", rr.Body.String())
	}
	second := f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token", "parentA", "")
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, first["feed_path"].(string)[len("/api/v1"):], "", "")
	f.expect(http.StatusOK, "", http.MethodGet, second["feed_path"].(string)[len("/api/v1"):], "", "")
	if r := f.expect(http.StatusOK, "", http.MethodDelete, "/calendar/feed-token", "parentA", ""); r["active"] != false {
		t.Fatalf("revoke: %v", r)
	}
	f.expect(http.StatusNotFound, "not_found", http.MethodDelete, "/calendar/feed-token", "parentA", "")
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, second["feed_path"].(string)[len("/api/v1"):], "", "")
	acts := f.actions()
	if acts["calendar_feed.rotate"] != 6 || acts["calendar_feed.revoke"] != 1 {
		t.Fatalf("feed audit: %v", acts)
	}
}

func TestCov2xCalendarFeedContents(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	var gotSwimmers []roster.Swimmer
	var gotFrom, gotTo time.Time
	f.h.feedSource = func(org string, sw []roster.Swimmer, from, to time.Time) ([]events.CalendarItem, error) {
		gotSwimmers, gotFrom, gotTo = sw, from, to
		return []events.CalendarItem{{UID: "meet-1@aquify", Title: "Home Meet", Start: cov2xNow.Add(24 * time.Hour)}}, nil
	}
	f.create("Pizza Night", `,"fee_cents":500`)
	f.expect(http.StatusCreated, "", http.MethodPost, "/club-events", "coach",
		fmt.Sprintf(`{"title":"AGE Clinic","category":"clinic","starts_at":%q,"audience":"groups","group_codes":["AGE"]}`, cov2xNow.Add(72*time.Hour).Format(time.RFC3339)))
	f.expect(http.StatusCreated, "", http.MethodPost, "/club-events", "coach",
		fmt.Sprintf(`{"title":"Camp, Day","category":"other","starts_at":%q,"all_day":true,"status":"cancelled"}`, cov2xNow.Add(240*time.Hour).Format("2006-01-02")+"T00:00:00Z"))

	feed := func(token string) string {
		path := f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token", token, "")["feed_path"].(string)
		rr := f.do(http.MethodGet, strings.TrimPrefix(path, "/api/v1"), "", "")
		if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/calendar") ||
			rr.Header().Get("X-Robots-Tag") != "noindex" || !strings.Contains(rr.Header().Get("Cache-Control"), "private") {
			t.Fatalf("feed headers: %d %v", rr.Code, rr.Header())
		}
		return strings.ReplaceAll(rr.Body.String(), "\r\n ", "")
	}
	a := feed("parentA")
	if !strings.Contains(a, "X-WR-CALNAME:Swim club calendar") || strings.Contains(a, "X-WR-TIMEZONE") {
		t.Fatalf("default calendar name: %s", a)
	}
	for _, want := range []string{"SUMMARY:Home Meet", "SUMMARY:Pizza Night", "SUMMARY:AGE Clinic", "SUMMARY:Cancelled: Camp\\, Day", "STATUS:CANCELLED", "DTSTART;VALUE=DATE:" + cov2xNow.Add(240*time.Hour).Format("20060102"), "Fee: $5.00 per person"} {
		if !strings.Contains(a, want) {
			t.Fatalf("feed missing %q:\n%s", want, a)
		}
	}
	if !(strings.Index(a, "Home Meet") < strings.Index(a, "AGE Clinic") && strings.Index(a, "AGE Clinic") < strings.Index(a, "Pizza Night") && strings.Index(a, "Pizza Night") < strings.Index(a, "Camp")) {
		t.Fatalf("items must be in start order:\n%s", a)
	}
	if len(gotSwimmers) != 2 || gotSwimmers[0].HouseholdID != "hh-a" || !gotFrom.Equal(cov2xNow.AddDate(0, 0, -60)) || !gotTo.Equal(cov2xNow.AddDate(1, 0, 0)) {
		t.Fatalf("feed source input: %v %v %v", gotSwimmers, gotFrom, gotTo)
	}
	// Audience filtering is per household: B family has no AGE swimmer.
	if b := feed("parentB"); strings.Contains(b, "AGE Clinic") || !strings.Contains(b, "Pizza Night") {
		t.Fatalf("household B feed:\n%s", b)
	}

	f.h.SetCalendarProfile(func(string) (string, string) { return "Dolphins", "America/Chicago" })
	if c := feed("parentA"); !strings.Contains(c, "X-WR-CALNAME:Dolphins calendar") || !strings.Contains(c, "X-WR-TIMEZONE:America/Chicago") {
		t.Fatalf("club profile:\n%s", c)
	}
	f.h.SetCalendarProfile(func(string) (string, string) { return "  ", "Europe/Paris" })
	if c := feed("parentA"); !strings.Contains(c, "X-WR-CALNAME:Swim club calendar") || !strings.Contains(c, "X-WR-TIMEZONE:Europe/Paris") {
		t.Fatalf("unnamed club keeps the timezone:\n%s", c)
	}
}

func TestCov2xCalendarFeedFailureModes(t *testing.T) {
	t.Parallel()
	f := cov2xNewEvFixture(t)
	path := strings.TrimPrefix(f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token", "parentA", "")["feed_path"].(string), "/api/v1")
	f.expect(http.StatusOK, "", http.MethodGet, path, "", "")
	f.expect(http.StatusOK, "", http.MethodGet, strings.TrimSuffix(path, ".ics"), "", "") // suffix is optional
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, "/calendar/feed/short.ics", "", "")
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, "/calendar/feed/"+strings.Repeat("x", 48)+".ics", "", "")

	internal := func(label string) {
		t.Helper()
		rr := f.do(http.MethodGet, path, "", "")
		if rr.Code != http.StatusInternalServerError || cov2xErrCode(cov2xDecode(t, rr)) != "internal_error" || strings.Contains(rr.Body.String(), "secret-detail") {
			t.Fatalf("%s: want clean 500, got %d %s", label, rr.Code, rr.Body.String())
		}
	}
	// Suspended clubs stop serving their feeds; an unreadable tenant registry fails closed.
	f.h.SetOrgActive(func(context.Context, string) (bool, error) { return false, nil })
	f.expect(http.StatusNotFound, "not_found", http.MethodGet, path, "", "")
	f.h.SetOrgActive(func(context.Context, string) (bool, error) { return false, errors.New("registry secret-detail") })
	internal("org registry error")
	f.h.SetOrgActive(func(_ context.Context, org string) (bool, error) { return org == f.org, nil })
	f.expect(http.StatusOK, "", http.MethodGet, path, "", "")

	f.dir.swimmersErr = errors.New("roster secret-detail")
	internal("roster error")
	f.dir.heal()
	f.h.feedSource = func(string, []roster.Swimmer, time.Time, time.Time) ([]events.CalendarItem, error) {
		return nil, errors.New("meets secret-detail")
	}
	internal("feed source error")
}

// With Postgres behind the events service, storage failures surface as clean 500s
// on every route instead of leaking driver detail or masquerading as 404s.
func TestCov2xClubEventsStorageFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org, hhU := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'DB Family')`, hhU, org); err != nil {
		t.Fatal(err)
	}
	f := cov2xNewEvFixtureWith(t, org, events.NewServiceWithPool(pool))
	f.dir.Households[hhU] = "DB Family"

	id := f.create("Stored", "")
	feedPath := strings.TrimPrefix(f.expect(http.StatusCreated, "", http.MethodPost, "/calendar/feed-token?household_id="+hhU, "admin", "")["feed_path"].(string), "/api/v1")
	f.expect(http.StatusOK, "", http.MethodGet, feedPath, "", "")
	f.expect(http.StatusOK, "", http.MethodGet, "/club-events/"+id, "admin", "")

	clean500 := func(label, method, path, token, body string) {
		t.Helper()
		rr := f.do(method, path, token, body)
		if rr.Code != http.StatusInternalServerError || cov2xErrCode(cov2xDecode(t, rr)) != "internal_error" ||
			strings.Contains(rr.Body.String(), "club_event") || strings.Contains(rr.Body.String(), "column") {
			t.Fatalf("%s: %s %s: want clean 500, got %d %s", label, method, path, rr.Code, rr.Body.String())
		}
	}
	// Break a column every query on the table touches (a bare table rename would
	// fall through to a same-named table in a shared schema on the search path).
	rename := func(table, column string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `ALTER TABLE `+table+` RENAME COLUMN `+column+` TO `+column+`_cov2x_gone`); err != nil {
			t.Fatal(err)
		}
	}

	rename("club_event_rsvps", "response")
	body := cov2xEventBody("Stored", "")
	for _, c := range []struct{ m, p, tok, b string }{
		{http.MethodGet, "/club-events", "admin", ""},
		{http.MethodGet, "/club-events/" + id, "admin", ""},
		{http.MethodGet, f.rsvpPath(id), "admin", ""},
		{http.MethodGet, f.rsvpPath(id) + "/export", "admin", ""},
		{http.MethodPut, f.rsvpPath(id), "coach", `{"household_id":"` + hhU + `","response":"yes"}`},
		{http.MethodPut, "/club-events/" + id, "coach", body},
		{http.MethodDelete, "/club-events/" + id, "coach", ""},
	} {
		clean500("rsvp table gone", c.m, c.p, c.tok, c.b)
	}

	rename("club_events", "title")
	for _, c := range []struct{ m, p, tok, b string }{
		{http.MethodGet, "/club-events", "admin", ""},
		{http.MethodGet, "/club-events/" + id, "admin", ""},
		{http.MethodGet, f.rsvpPath(id), "admin", ""},
		{http.MethodGet, f.rsvpPath(id) + "/export", "admin", ""},
		{http.MethodPost, "/club-events", "coach", body},
		{http.MethodPut, "/club-events/" + id, "coach", body},
		{http.MethodDelete, "/club-events/" + id, "coach", ""},
		{http.MethodPut, f.rsvpPath(id), "coach", `{"household_id":"` + hhU + `","response":"yes"}`},
		{http.MethodGet, feedPath, "", ""}, // token still resolves, but the events behind the feed are unreadable
	} {
		clean500("events table gone", c.m, c.p, c.tok, c.b)
	}

	rename("calendar_feed_tokens", "revoked_at")
	for _, c := range []struct{ m, p, tok, b string }{
		{http.MethodGet, "/calendar/feed-token?household_id=" + hhU, "admin", ""},
		{http.MethodPost, "/calendar/feed-token?household_id=" + hhU, "admin", ""},
		{http.MethodDelete, "/calendar/feed-token?household_id=" + hhU, "admin", ""},
		{http.MethodGet, feedPath, "", ""},
	} {
		clean500("token table gone", c.m, c.p, c.tok, c.b)
	}
}
