package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/volunteer"
	"github.com/luryann/aquify/server/internal/testutil"
)

const (
	cov5xOrg   = "5c0a0000-0000-4000-8000-000000000001"
	cov5xHH    = "5c0a0000-0000-4000-8000-0000000000b1"
	cov5xAdmin = "5c0a0000-0000-4000-8000-0000000000a1"
)

func cov5xTokens() tokenResolver {
	return tokenResolver{
		"admin":   {UserID: cov5xAdmin, OrgID: cov5xOrg, Roles: []string{"admin"}},
		"coach":   {UserID: "5c0a0000-0000-4000-8000-0000000000a2", OrgID: cov5xOrg, Roles: []string{"coach"}},
		"parent":  {UserID: "5c0a0000-0000-4000-8000-0000000000a3", OrgID: cov5xOrg, HouseholdID: cov5xHH, Roles: []string{"parent"}},
		"swimmer": {UserID: "5c0a0000-0000-4000-8000-0000000000a4", OrgID: cov5xOrg, HouseholdID: cov5xHH, Roles: []string{"swimmer"}},
	}
}

// cov5xRouter mounts routes behind the real Auth + RequireOrgScope middleware.
func cov5xRouter(register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov5xTokens()))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return r
}

// cov5xAuthOnly authenticates but never installs the org scope.
func cov5xAuthOnly(register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov5xTokens()))
		register(p)
	})
	return r
}

func cov5xDo(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cov5xOrg)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func cov5xWant(t *testing.T, label string, rr *httptest.ResponseRecorder, status int, bodySubstr string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("%s: want status %d, got %d: %s", label, status, rr.Code, rr.Body.String())
	}
	if bodySubstr != "" && !strings.Contains(rr.Body.String(), bodySubstr) {
		t.Fatalf("%s: want body containing %q, got %s", label, bodySubstr, rr.Body.String())
	}
}

type cov5xRoute struct{ method, path, body string }

func cov5xVolunteerRoutes() []cov5xRoute {
	job, sg, hh, season := uuid.NewString(), uuid.NewString(), cov5xHH, uuid.NewString()
	return []cov5xRoute{
		{"GET", "/volunteer/settings", ""},
		{"PUT", "/volunteer/settings", `{"cancellation_cutoff_hours":12}`},
		{"GET", "/volunteer/templates", ""},
		{"GET", "/volunteer/jobs", ""},
		{"POST", "/volunteer/jobs", `{"event_name":"Dual","role":"timer","title":"Timer","starts_at":"2031-05-01T10:00:00Z","ends_at":"2031-05-01T12:00:00Z","capacity":2}`},
		{"POST", "/volunteer/jobs/bulk", `{"event_name":"Dual","shifts":[{"starts_at":"2031-05-01T10:00:00Z","ends_at":"2031-05-01T12:00:00Z"}]}`},
		{"GET", "/volunteer/jobs/" + job, ""},
		{"PATCH", "/volunteer/jobs/" + job, `{"title":"New title"}`},
		{"DELETE", "/volunteer/jobs/" + job, ""},
		{"POST", "/volunteer/jobs/" + job + "/signups", `{"volunteer_name":"Pat Parent","household_id":"` + hh + `"}`},
		{"GET", "/volunteer/signups", ""},
		{"POST", "/volunteer/signups/" + sg + "/cancel", `{"reason":"sick"}`},
		{"POST", "/volunteer/signups/" + sg + "/check-in", `{"status":"worked"}`},
		{"GET", "/volunteer/requirements", ""},
		{"PUT", "/volunteer/requirements", `{"season_id":"` + season + `","required_hours":4}`},
		{"DELETE", "/volunteer/requirements/" + uuid.NewString(), ""},
		{"GET", "/volunteer/credits", ""},
		{"POST", "/volunteer/credits", `{"season_id":"` + season + `","household_id":"` + hh + `","hours":1.5,"reason":"Setup crew"}`},
		{"GET", "/volunteer/ledger?season_id=" + season, ""},
		{"GET", "/volunteer/households/" + hh + "/ledger", ""},
		{"GET", "/volunteer/buyouts", ""},
		{"POST", "/volunteer/buyouts", `{"season_id":"` + season + `","household_id":"` + hh + `"}`},
	}
}

func cov5xVolunteerRouter(svc *volunteer.Service) http.Handler {
	h := NewVolunteerHandlers(svc, nil)
	return cov5xRouter(h.RegisterRoutes)
}

func TestCov5xVolunteerRoutesRequirePrincipalAndOrgScope(t *testing.T) {
	t.Parallel()
	register := NewVolunteerHandlers(volunteer.NewService(), nil).RegisterRoutes
	bare := chi.NewRouter()
	register(bare)
	authOnly := cov5xAuthOnly(register)
	for _, rt := range cov5xVolunteerRoutes() {
		rr := cov5xDo(bare, rt.method, rt.path, "", rt.body)
		if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "unauthorized") {
			t.Errorf("%s %s without principal: %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		req.Header.Set("Authorization", "Bearer admin")
		rec := httptest.NewRecorder()
		authOnly.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing_org_scope") {
			t.Errorf("%s %s without org scope: %d %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
	}
}

// Every route reports a storage failure as an opaque 500 and never leaks the cause.
func TestCov5xVolunteerRoutesReportStorageFailuresAsInternalError(t *testing.T) {
	router := cov5xVolunteerRouter(volunteer.NewServiceWithPool(testutil.ClosedPool(t)))
	for _, rt := range cov5xVolunteerRoutes() {
		if rt.path == "/volunteer/templates" {
			continue
		}
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rr := cov5xDo(router, rt.method, rt.path, "admin", rt.body)
			if rt.method == "POST" && rt.path == "/volunteer/buyouts" {
				// Billing is checked before storage is touched.
				cov5xWant(t, "buyout without billing", rr, http.StatusServiceUnavailable, "billing_unavailable")
				return
			}
			cov5xWant(t, "storage failure", rr, http.StatusInternalServerError, "internal_error")
			if strings.Contains(rr.Body.String(), "closed pool") {
				t.Errorf("leaked the storage error: %s", rr.Body.String())
			}
		})
	}
}

func TestCov5xVolunteerMalformedBodiesAreRejected(t *testing.T) {
	t.Parallel()
	router := cov5xVolunteerRouter(volunteer.NewService())
	signup := uuid.NewString()
	for _, rt := range []cov5xRoute{
		{"PUT", "/volunteer/settings", ""},
		{"POST", "/volunteer/jobs", ""},
		{"POST", "/volunteer/jobs/bulk", ""},
		{"PATCH", "/volunteer/jobs/" + uuid.NewString(), ""},
		{"POST", "/volunteer/jobs/" + uuid.NewString() + "/signups", ""},
		{"POST", "/volunteer/signups/" + signup + "/cancel", `{"reason":`},
		{"POST", "/volunteer/signups/" + signup + "/check-in", ""},
		{"PUT", "/volunteer/requirements", ""},
		{"POST", "/volunteer/credits", ""},
		{"POST", "/volunteer/buyouts", ""},
	} {
		rr := cov5xDo(router, rt.method, rt.path, "admin", rt.body)
		cov5xWant(t, rt.method+" "+rt.path, rr, http.StatusBadRequest, "invalid_request_body")
	}
	// Syntactically valid but failing struct validation.
	for _, rt := range []cov5xRoute{
		{"POST", "/volunteer/jobs", `{"role":"timer"}`},
		{"POST", "/volunteer/jobs/bulk", `{"shifts":[]}`},
		{"POST", "/volunteer/signups/" + signup + "/check-in", `{"status":"maybe"}`},
		{"POST", "/volunteer/signups/" + signup + "/cancel", `{"reason":"` + strings.Repeat("x", 501) + `"}`},
		{"PUT", "/volunteer/requirements", `{"required_hours":-1}`},
		{"POST", "/volunteer/credits", `{"hours":1}`},
		{"POST", "/volunteer/buyouts", `{"season_id":"s"}`},
	} {
		rr := cov5xDo(router, rt.method, rt.path, "admin", rt.body)
		cov5xWant(t, rt.method+" "+rt.path, rr, http.StatusUnprocessableEntity, "validation_error")
	}
}

func TestCov5xVolunteerRejectsUnparseableTimestamps(t *testing.T) {
	t.Parallel()
	router := cov5xVolunteerRouter(volunteer.NewService())
	good := "2031-05-01T10:00:00Z"

	cov5xWant(t, "create bad end", cov5xDo(router, "POST", "/volunteer/jobs", "admin",
		`{"role":"timer","title":"T","starts_at":"`+good+`","ends_at":"tomorrow","capacity":1}`), http.StatusUnprocessableEntity, "RFC3339")

	cov5xWant(t, "bulk bad shift", cov5xDo(router, "POST", "/volunteer/jobs/bulk", "admin",
		`{"shifts":[{"starts_at":"soon","ends_at":"`+good+`"}]}`), http.StatusUnprocessableEntity, "Every shift")

	id := uuid.NewString()
	cov5xWant(t, "patch bad start", cov5xDo(router, "PATCH", "/volunteer/jobs/"+id, "admin", `{"starts_at":"x"}`),
		http.StatusUnprocessableEntity, "starts_at must be")
	cov5xWant(t, "patch bad end", cov5xDo(router, "PATCH", "/volunteer/jobs/"+id, "admin", `{"ends_at":"x"}`),
		http.StatusUnprocessableEntity, "ends_at must be")
	// Both bounds parse, so the request reaches the service, which cannot find the job.
	cov5xWant(t, "patch unknown job", cov5xDo(router, "PATCH", "/volunteer/jobs/"+id, "admin",
		`{"starts_at":"2031-05-02","ends_at":"`+good+`"}`), http.StatusNotFound, "Volunteer job not found")

	for _, q := range []string{"from=garbage", "to=garbage"} {
		rr := cov5xDo(router, "GET", "/volunteer/jobs?"+q, "admin", "")
		cov5xWant(t, q, rr, http.StatusBadRequest, "must be RFC3339 or YYYY-MM-DD")
	}
}

func TestCov5xVolunteerRolesAreEnforcedBeforeWork(t *testing.T) {
	t.Parallel()
	router := cov5xVolunteerRouter(volunteer.NewService())
	jobID := uuid.NewString()
	for _, tc := range []struct {
		token string
		rt    cov5xRoute
	}{
		{"parent", cov5xRoute{"GET", "/volunteer/templates", ""}},
		{"parent", cov5xRoute{"POST", "/volunteer/jobs", `{}`}},
		{"parent", cov5xRoute{"POST", "/volunteer/jobs/bulk", `{}`}},
		{"parent", cov5xRoute{"PATCH", "/volunteer/jobs/" + jobID, `{}`}},
		{"parent", cov5xRoute{"POST", "/volunteer/signups/" + uuid.NewString() + "/check-in", `{}`}},
		{"coach", cov5xRoute{"POST", "/volunteer/buyouts", `{}`}},
		{"parent", cov5xRoute{"GET", "/volunteer/settings", ""}},
		{"swimmer", cov5xRoute{"PUT", "/volunteer/requirements", `{"season_id":"s","required_hours":1}`}},
		{"coach", cov5xRoute{"PUT", "/volunteer/settings", `{"reminders_enabled":false}`}},
	} {
		rr := cov5xDo(router, tc.rt.method, tc.rt.path, tc.token, tc.rt.body)
		cov5xWant(t, tc.token+" "+tc.rt.method+" "+tc.rt.path, rr, http.StatusForbidden, "forbidden")
	}
	cov5xWant(t, "coach reads templates", cov5xDo(router, "GET", "/volunteer/templates", "coach", ""), http.StatusOK, "templates")
}

func TestCov5xVolunteerNotFoundAndLookupErrors(t *testing.T) {
	t.Parallel()
	router := cov5xVolunteerRouter(volunteer.NewService())
	missing := uuid.NewString()
	for _, rt := range []struct {
		route cov5xRoute
		want  string
	}{
		{cov5xRoute{"GET", "/volunteer/jobs/" + missing, ""}, "Volunteer job not found"},
		{cov5xRoute{"DELETE", "/volunteer/jobs/" + missing, ""}, "Volunteer job not found"},
		{cov5xRoute{"POST", "/volunteer/jobs/" + missing + "/signups", `{"volunteer_name":"A B","household_id":"` + cov5xHH + `"}`}, "not_found"},
		{cov5xRoute{"POST", "/volunteer/signups/" + missing + "/cancel", ``}, "Volunteer signup not found"},
		{cov5xRoute{"POST", "/volunteer/signups/" + missing + "/check-in", `{"status":"worked"}`}, "Volunteer signup not found"},
		{cov5xRoute{"DELETE", "/volunteer/requirements/" + missing, ""}, "Requirement not found"},
	} {
		rr := cov5xDo(router, rt.route.method, rt.route.path, "admin", rt.route.body)
		cov5xWant(t, rt.route.method+" "+rt.route.path, rr, http.StatusNotFound, rt.want)
	}
	// The season ledger needs a season id.
	cov5xWant(t, "ledger without season", cov5xDo(router, "GET", "/volunteer/ledger", "admin", ""), http.StatusBadRequest, "season_id is required")
}

func TestCov5xWriteVolunteerErrorMapsEveryDomainError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&volunteer.ValidationError{Field: "capacity", Message: "bad"}, http.StatusUnprocessableEntity, "validation_error"},
		{volunteer.ErrForbidden, http.StatusForbidden, "forbidden"},
		{volunteer.ErrJobNotFound, http.StatusNotFound, "not_found"},
		{volunteer.ErrSignupNotFound, http.StatusNotFound, "not_found"},
		{volunteer.ErrRequirementNotFound, http.StatusNotFound, "not_found"},
		{volunteer.ErrSeasonNotFound, http.StatusNotFound, "season_not_found"},
		{volunteer.ErrMeetNotFound, http.StatusNotFound, "meet_not_found"},
		{volunteer.ErrHouseholdNotFound, http.StatusNotFound, "household_not_found"},
		{volunteer.ErrJobFull, http.StatusConflict, "job_full"},
		{volunteer.ErrDuplicateSignup, http.StatusConflict, "duplicate_signup"},
		{volunteer.ErrJobStarted, http.StatusConflict, "job_started"},
		{volunteer.ErrCancellationClosed, http.StatusConflict, "cancellation_window_closed"},
		{volunteer.ErrSignupNotActive, http.StatusConflict, "signup_not_active"},
		{volunteer.ErrInvalidTransition, http.StatusConflict, "invalid_transition"},
		{volunteer.ErrBuyoutNotFound, http.StatusNotFound, "not_found"},
		{volunteer.ErrJobHasWorkedSignups, http.StatusConflict, "job_has_worked_signups"},
		{volunteer.ErrCapacityBelowFilled, http.StatusConflict, "capacity_below_filled"},
		{volunteer.ErrCertificationRequired, http.StatusUnprocessableEntity, "certification_required"},
		{volunteer.ErrAgeRequirement, http.StatusUnprocessableEntity, "age_requirement"},
		{volunteer.ErrBuyoutExists, http.StatusConflict, "buyout_exists"},
		{volunteer.ErrNothingOwed, http.StatusConflict, "nothing_owed"},
		{volunteer.ErrNoBuyoutRate, http.StatusConflict, "no_buyout_rate"},
		{volunteer.ErrBillingUnavailable, http.StatusServiceUnavailable, "billing_unavailable"},
		{fmt.Errorf("wrapped: %w", volunteer.ErrJobFull), http.StatusConflict, "job_full"},
		{errors.New("pq: connection reset"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		writeVolunteerError(rr, httptest.NewRequest("GET", "/x", nil), tc.err)
		if rr.Code != tc.status || !strings.Contains(rr.Body.String(), `"`+tc.code+`"`) {
			t.Errorf("%v: got %d %s, want %d %s", tc.err, rr.Code, rr.Body.String(), tc.status, tc.code)
		}
		if tc.status == http.StatusInternalServerError && strings.Contains(rr.Body.String(), "connection reset") {
			t.Errorf("internal errors must not reach the client: %s", rr.Body.String())
		}
	}
}

func TestCov5xParseTimestampAndRangeBounds(t *testing.T) {
	t.Parallel()
	if _, ok := parseTimestamp("   "); ok {
		t.Fatal("blank input must not parse")
	}
	if _, ok := parseTimestamp("31/12/2031"); ok {
		t.Fatal("unsupported layout must not parse")
	}
	got, ok := parseTimestamp("2031-05-01T10:00:00-07:00")
	if !ok || got.Format("2006-01-02T15:04:05Z07:00") != "2031-05-01T17:00:00Z" {
		t.Fatalf("RFC3339 must be normalised to UTC, got %v %v", got, ok)
	}
	day, ok := parseTimestamp(" 2031-05-01 ")
	if !ok || day.Hour() != 0 || day.Day() != 1 {
		t.Fatalf("date-only: %v %v", day, ok)
	}
	// A date-only upper bound is advanced a day so the whole date is included.
	up, ok := parseRangeBound("2031-05-01", true)
	if !ok || up.Day() != 2 {
		t.Fatalf("date-only upper bound: %v %v", up, ok)
	}
	lo, _ := parseRangeBound("2031-05-01", false)
	if lo.Day() != 1 {
		t.Fatalf("date-only lower bound must not move: %v", lo)
	}
	ts, _ := parseRangeBound("2031-05-01T08:00:00Z", true)
	if ts.Day() != 1 || ts.Hour() != 8 {
		t.Fatalf("timestamp upper bound must not move: %v", ts)
	}
	if _, ok := parseRangeBound("nope", true); ok {
		t.Fatal("garbage bound must not parse")
	}
}

// End-to-end happy path through the real in-memory service, covering the list
// summary maths, bulk creation, audit-free operation and the parent/staff splits.
func TestCov5xVolunteerWorkflowInMemory(t *testing.T) {
	t.Parallel()
	svc := volunteer.NewService()
	router := cov5xVolunteerRouter(svc)

	rr := cov5xDo(router, "POST", "/volunteer/jobs/bulk", "coach",
		`{"event_name":"Dual Meet","shifts":[{"starts_at":"2031-05-01T10:00:00Z","ends_at":"2031-05-01T12:00:00Z","label":"AM"}]}`)
	cov5xWant(t, "bulk", rr, http.StatusCreated, `"count"`)
	body := cov5xParse(t, rr)
	count := int(body["count"].(float64))
	if count < 2 {
		t.Fatalf("the default template should create several roles, got %d", count)
	}

	rr = cov5xDo(router, "GET", "/volunteer/jobs?limit=2&from=2031-05-01&to=2031-05-01", "admin", "")
	cov5xWant(t, "list", rr, http.StatusOK, "summary")
	list := cov5xParse(t, rr)
	if int(list["total"].(float64)) != count || int(list["count"].(float64)) != 2 {
		t.Fatalf("pagination: %v", list)
	}
	summary := list["summary"].(map[string]any)
	if int(summary["job_count"].(float64)) != count || summary["fill_rate"].(float64) != 0 {
		t.Fatalf("summary: %v", summary)
	}
	first := list["jobs"].([]any)[0].(map[string]any)["id"].(string)

	rr = cov5xDo(router, "POST", "/volunteer/jobs/"+first+"/signups", "parent", `{"volunteer_name":"Pat   Parent"}`)
	cov5xWant(t, "parent signup", rr, http.StatusCreated, "Pat Parent")

	rr = cov5xDo(router, "GET", "/volunteer/jobs?upcoming=true", "admin", "")
	cov5xWant(t, "upcoming", rr, http.StatusOK, "")
	if sum := cov5xParse(t, rr)["summary"].(map[string]any); sum["fill_rate"].(float64) <= 0 {
		t.Fatalf("one filled spot must raise the fill rate: %v", sum)
	}

	rr = cov5xDo(router, "GET", "/volunteer/signups?job_id="+first, "admin", "")
	cov5xWant(t, "signups", rr, http.StatusOK, `"total":1`)
	signupID := cov5xParse(t, rr)["signups"].([]any)[0].(map[string]any)["id"].(string)

	rr = cov5xDo(router, "PATCH", "/volunteer/jobs/"+first, "coach", `{"title":"Renamed","capacity":5,"starts_at":"2031-05-01T09:00:00Z"}`)
	cov5xWant(t, "update", rr, http.StatusOK, "Renamed")

	rr = cov5xDo(router, "POST", "/volunteer/signups/"+signupID+"/check-in", "coach", `{"status":"worked","hours":2}`)
	cov5xWant(t, "check-in", rr, http.StatusOK, "worked")
	cov5xWant(t, "delete worked job", cov5xDo(router, "DELETE", "/volunteer/jobs/"+first, "admin", ""), http.StatusConflict, "job_has_worked_signups")
	cov5xWant(t, "cancel worked", cov5xDo(router, "POST", "/volunteer/signups/"+signupID+"/cancel", "parent", ``), http.StatusConflict, "")

	cov5xWant(t, "settings", cov5xDo(router, "GET", "/volunteer/settings", "coach", ""), http.StatusOK, "settings")
	cov5xWant(t, "list requirements", cov5xDo(router, "GET", "/volunteer/requirements", "admin", ""), http.StatusOK, "requirements")
	cov5xWant(t, "list credits", cov5xDo(router, "GET", "/volunteer/credits", "admin", ""), http.StatusOK, "credits")
	cov5xWant(t, "list buyouts", cov5xDo(router, "GET", "/volunteer/buyouts", "admin", ""), http.StatusOK, "buyouts")
	cov5xWant(t, "household ledger", cov5xDo(router, "GET", "/volunteer/households/"+cov5xHH+"/ledger", "parent", ""), http.StatusOK, "ledger")
}

func cov5xParse(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v: %s", err, rr.Body.String())
	}
	return out
}

func TestCov5xVolunteerJobAndRequirementLifecycle(t *testing.T) {
	t.Parallel()
	router := cov5xVolunteerRouter(volunteer.NewService())

	rr := cov5xDo(router, "POST", "/volunteer/jobs", "coach",
		`{"event_name":"Fundraiser","role":"timer","title":"Snack bar","starts_at":"2031-05-01","ends_at":"2031-05-01T14:00:00Z","capacity":2}`)
	cov5xWant(t, "create", rr, http.StatusCreated, "Snack bar")
	job := cov5xParse(t, rr)["job"].(map[string]any)
	jobID := job["id"].(string)

	rr = cov5xDo(router, "GET", "/volunteer/jobs/"+jobID, "parent", "")
	cov5xWant(t, "get", rr, http.StatusOK, jobID)
	if got := cov5xParse(t, rr)["job"].(map[string]any); got["title"] != "Snack bar" || got["capacity"].(float64) != 2 {
		t.Fatalf("job: %v", got)
	}

	rr = cov5xDo(router, "DELETE", "/volunteer/jobs/"+jobID, "coach", "")
	cov5xWant(t, "delete", rr, http.StatusOK, `"deleted":true`)
	cov5xWant(t, "get after delete", cov5xDo(router, "GET", "/volunteer/jobs/"+jobID, "admin", ""), http.StatusNotFound, "Volunteer job not found")

	season := uuid.NewString()
	rr = cov5xDo(router, "PUT", "/volunteer/requirements", "admin",
		`{"season_id":"`+season+`","required_hours":6,"buyout_cents_per_hour":2500,"notes":"Standard"}`)
	cov5xWant(t, "upsert requirement", rr, http.StatusOK, `"required_hours":6`)
	reqID := cov5xParse(t, rr)["requirement"].(map[string]any)["id"].(string)
	cov5xWant(t, "list requirements", cov5xDo(router, "GET", "/volunteer/requirements?season_id="+season, "admin", ""), http.StatusOK, reqID)

	cov5xWant(t, "delete requirement", cov5xDo(router, "DELETE", "/volunteer/requirements/"+reqID, "admin", ""), http.StatusOK, `"deleted":true`)
	cov5xWant(t, "delete requirement again", cov5xDo(router, "DELETE", "/volunteer/requirements/"+reqID, "admin", ""), http.StatusNotFound, "Requirement not found")

	rr = cov5xDo(router, "POST", "/volunteer/credits", "admin",
		`{"season_id":"`+season+`","household_id":"`+cov5xHH+`","hours":1.5,"reason":"Setup crew"}`)
	cov5xWant(t, "add credit", rr, http.StatusCreated, "Setup crew")
	cov5xWant(t, "list credits", cov5xDo(router, "GET", "/volunteer/credits?season_id="+season+"&household_id="+cov5xHH, "admin", ""), http.StatusOK, "Setup crew")
	cov5xWant(t, "season ledger", cov5xDo(router, "GET", "/volunteer/ledger?season_id="+season, "admin", ""), http.StatusOK, `"buyout_due_cents"`)
}

func TestCov5xVolunteerSettingsAndCancellation(t *testing.T) {
	t.Parallel()
	router := cov5xVolunteerRouter(volunteer.NewService())

	rr := cov5xDo(router, "PUT", "/volunteer/settings", "admin",
		`{"default_required_hours":8,"default_buyout_cents_per_hour":1500,"cancellation_cutoff_hours":6,"reminders_enabled":false,"timezone":"America/Denver"}`)
	cov5xWant(t, "update settings", rr, http.StatusOK, "America/Denver")
	st := cov5xParse(t, rr)["settings"].(map[string]any)
	if st["default_required_hours"].(float64) != 8 || st["cancellation_cutoff_hours"].(float64) != 6 || st["reminders_enabled"] != false {
		t.Fatalf("settings not applied: %v", st)
	}
	cov5xWant(t, "bad timezone", cov5xDo(router, "PUT", "/volunteer/settings", "admin", `{"timezone":"Mars/Olympus"}`), http.StatusUnprocessableEntity, "validation_error")

	rr = cov5xDo(router, "POST", "/volunteer/jobs", "admin",
		`{"event_name":"Meet","role":"timer","title":"Timer","starts_at":"2031-06-01T09:00:00Z","ends_at":"2031-06-01T12:00:00Z","capacity":1}`)
	jobID := cov5xParse(t, rr)["job"].(map[string]any)["id"].(string)
	rr = cov5xDo(router, "POST", "/volunteer/jobs/"+jobID+"/signups", "parent", `{"volunteer_name":"Pat Parent"}`)
	cov5xWant(t, "signup", rr, http.StatusCreated, "Pat Parent")
	signupID := cov5xParse(t, rr)["signup"].(map[string]any)["id"].(string)

	cov5xWant(t, "job now full", cov5xDo(router, "POST", "/volunteer/jobs/"+jobID+"/signups", "admin",
		`{"volunteer_name":"Sam","household_id":"`+cov5xHH+`"}`), http.StatusConflict, "")

	rr = cov5xDo(router, "POST", "/volunteer/signups/"+signupID+"/cancel", "parent", `{"reason":"  family trip  "}`)
	cov5xWant(t, "cancel", rr, http.StatusOK, "cancelled")
	cov5xWant(t, "cancel again", cov5xDo(router, "POST", "/volunteer/signups/"+signupID+"/cancel", "parent", ``), http.StatusConflict, "signup_not_active")

	// The freed spot can be taken again.
	cov5xWant(t, "re-signup", cov5xDo(router, "POST", "/volunteer/jobs/"+jobID+"/signups", "parent", `{"volunteer_name":"Pat Parent"}`), http.StatusCreated, "Pat Parent")
}
