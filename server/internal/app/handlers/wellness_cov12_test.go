package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/attendance"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/roster"
	"github.com/luryann/aquify/server/internal/app/wellness"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov12gWellDir wraps a static roster and lets a test make individual
// directory reads fail, standing in for an unavailable roster backend.
type cov12gWellDir struct {
	*roster.StaticDirectory
	swimmersErr   error
	householdsErr error
}

func (d cov12gWellDir) Swimmers(orgID string) ([]roster.Swimmer, error) {
	if d.swimmersErr != nil {
		return nil, d.swimmersErr
	}
	return d.StaticDirectory.Swimmers(orgID)
}

func (d cov12gWellDir) HouseholdIDsForPrincipal(p *appmiddleware.Principal, orgID string) ([]string, error) {
	if d.householdsErr != nil {
		return nil, d.householdsErr
	}
	return d.StaticDirectory.HouseholdIDsForPrincipal(p, orgID)
}

func cov12gStaticDir() *roster.StaticDirectory {
	return &roster.StaticDirectory{
		SwimmerList: []roster.Swimmer{
			{ID: "sw-a", OrgID: engOrg, HouseholdID: "hh-a", FirstName: "Ada", LastName: "A", DisplayName: "Ada A", GroupCodes: []string{"SENIOR"}},
			{ID: "sw-z", OrgID: engOrg, HouseholdID: "hh-z", FirstName: "Zed", LastName: "Z", DisplayName: "Zed Z", GroupCodes: []string{"SENIOR"}},
		},
		Households: map[string]string{"hh-a": "A Family", "hh-z": "Z Family"},
	}
}

var cov12gTokens = tokenResolver{
	"admin":   {UserID: "u-admin", OrgID: engOrg, Roles: []string{"admin"}},
	"coach":   {UserID: "u-coach", OrgID: engOrg, Roles: []string{"coach"}},
	"guest":   {UserID: "u-guest", OrgID: engOrg, Roles: []string{"guest"}},
	"parentA": {UserID: "u-pa", OrgID: engOrg, HouseholdID: "hh-a", Roles: []string{"parent"}},
}

type cov12gSource struct {
	practices []attendance.Practice
	err       error
}

func (s cov12gSource) Practices(string) ([]attendance.Practice, error) { return s.practices, s.err }
func (s cov12gSource) Attendance(string, string) ([]attendance.Record, error) {
	return []attendance.Record{{SwimmerID: "sw-a", Status: "present"}}, nil
}

func cov12gWellRouter(t *testing.T, svc *wellness.Service, dir roster.Directory, src attendance.PracticeSource) http.Handler {
	t.Helper()
	return g2Router(t, cov12gTokens, func(r chi.Router) {
		NewWellnessHandlers(svc, dir).RegisterRoutes(r)
		NewAttendanceSummaryHandlers(src, dir, func() string { return "2026-10-20" }).RegisterRoutes(r)
	})
}

func cov12gBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return out
}

func cov12gWant(t *testing.T, rr *httptest.ResponseRecorder, status int, contains string) {
	t.Helper()
	if rr.Code != status || !strings.Contains(rr.Body.String(), contains) {
		t.Fatalf("want %d containing %q, got %d: %s", status, contains, rr.Code, rr.Body.String())
	}
}

const cov12gCheckIn = `{"sleep_hours":8,"sleep_quality":4,"soreness":2,"energy":4,"stress":2,"mood":4`

func TestCov12gClampDaysAndNextISODate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw        string
		def, max   int
		want       int
		descripton string
	}{
		{"", 30, 90, 30, "default when absent"},
		{"7", 30, 90, 7, "within range"},
		{"90", 30, 90, 90, "exactly the max"},
		{"91", 30, 90, 90, "clamped to the max"},
		{"100000", 14, 28, 28, "huge value clamped"},
	} {
		if got := clampDays(tc.raw, tc.def, tc.max); got != tc.want {
			t.Errorf("%s: clampDays(%q) = %d, want %d", tc.descripton, tc.raw, got, tc.want)
		}
	}
	if got := nextISODate("2026-02-28"); got != "2026-03-01" {
		t.Errorf("nextISODate rolls over month end: %q", got)
	}
	if got := nextISODate("not-a-date"); got != "not-a-date" {
		t.Errorf("nextISODate returns unparseable input unchanged: %q", got)
	}
}

func TestCov12gWellnessWindowsAreClamped(t *testing.T) {
	t.Parallel()
	r := cov12gWellRouter(t, wellness.NewService(), cov12gStaticDir(), cov12gSource{})

	rr := g2Do(r, http.MethodGet, "/wellness/swimmers/sw-a?days=5000", "coach", "")
	cov12gWant(t, rr, http.StatusOK, "today_checkin")
	trend := cov12gBody(t, rr)
	from, _ := time.Parse("2006-01-02", trend["from"].(string))
	to, _ := time.Parse("2006-01-02", trend["to"].(string))
	if span := int(to.Sub(from).Hours()/24) + 1; span != wellnessMaxTrendDays {
		t.Fatalf("trend window spans %d days, want clamp to %d", span, wellnessMaxTrendDays)
	}

	rr = g2Do(r, http.MethodGet, "/wellness/heatmap?days=5000", "coach", "")
	cov12gWant(t, rr, http.StatusOK, "roster_size")
	if days := cov12gBody(t, rr)["days"].([]any); len(days) != wellnessMaxHeatmapDays {
		t.Fatalf("heatmap has %d days, want clamp to %d", len(days), wellnessMaxHeatmapDays)
	}
}

func TestCov12gWellnessRoleAndInputValidation(t *testing.T) {
	t.Parallel()
	r := cov12gWellRouter(t, wellness.NewService(), cov12gStaticDir(), cov12gSource{})

	cov12gWant(t, g2Do(r, http.MethodGet, "/wellness/swimmers/sw-a", "guest", ""), http.StatusForbidden, "Insufficient role.")
	cov12gWant(t, g2Do(r, http.MethodGet, "/wellness/heatmap", "guest", ""), http.StatusForbidden, "heat-map")
	cov12gWant(t, g2Do(r, http.MethodPut, "/wellness/checkins", "coach", cov12gCheckIn+`}`), http.StatusForbidden, "Check-ins are submitted")

	// A parent must say which child the check-in is for.
	rr := g2Do(r, http.MethodPut, "/wellness/checkins", "parentA", cov12gCheckIn+`}`)
	cov12gWant(t, rr, http.StatusUnprocessableEntity, "swimmer_id is required.")

	// A malformed date is rejected by the wellness service and surfaced field-by-field.
	rr = g2Do(r, http.MethodPut, "/wellness/checkins", "parentA", cov12gCheckIn+`,"swimmer_id":"sw-a","date":"yesterday"}`)
	cov12gWant(t, rr, http.StatusUnprocessableEntity, "must be YYYY-MM-DD")
	if !strings.Contains(rr.Body.String(), `"field":"date"`) {
		t.Fatalf("validation issue should name the date field: %s", rr.Body.String())
	}

	// A valid check-in for the same child still goes through (and is attributed to the parent).
	rr = g2Do(r, http.MethodPut, "/wellness/checkins", "parentA", cov12gCheckIn+`,"swimmer_id":"sw-a"}`)
	cov12gWant(t, rr, http.StatusCreated, `"submitted_role":"parent"`)
}

func TestCov12gWellnessDirectoryFailuresReturn500(t *testing.T) {
	t.Parallel()
	boom := errors.New("roster backend down")
	healthy := cov12gStaticDir()

	t.Run("roster unavailable", func(t *testing.T) {
		r := cov12gWellRouter(t, wellness.NewService(), cov12gWellDir{StaticDirectory: healthy, swimmersErr: boom}, cov12gSource{})
		cov12gWant(t, g2Do(r, http.MethodPut, "/wellness/checkins", "parentA", cov12gCheckIn+`,"swimmer_id":"sw-a"}`), http.StatusInternalServerError, "Failed to resolve household scope.")
		cov12gWant(t, g2Do(r, http.MethodGet, "/wellness/swimmers/sw-a", "coach", ""), http.StatusInternalServerError, "Failed to resolve swimmer.")
		cov12gWant(t, g2Do(r, http.MethodGet, "/wellness/heatmap", "coach", ""), http.StatusInternalServerError, "Failed to load roster.")
		cov12gWant(t, g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary", "coach", ""), http.StatusInternalServerError, "Failed to resolve swimmer.")
		if strings.Contains(g2Do(r, http.MethodGet, "/wellness/heatmap", "coach", "").Body.String(), boom.Error()) {
			t.Fatal("the underlying error must not leak to clients")
		}
	})
	t.Run("household lookup unavailable", func(t *testing.T) {
		r := cov12gWellRouter(t, wellness.NewService(), cov12gWellDir{StaticDirectory: healthy, householdsErr: boom}, cov12gSource{})
		cov12gWant(t, g2Do(r, http.MethodPut, "/wellness/checkins", "parentA", cov12gCheckIn+`,"swimmer_id":"sw-a"}`), http.StatusInternalServerError, "Failed to resolve household scope.")
		cov12gWant(t, g2Do(r, http.MethodGet, "/wellness/swimmers/sw-a", "parentA", ""), http.StatusInternalServerError, "Failed to resolve swimmer.")
		// Staff do not need the household lookup, so they are unaffected.
		cov12gWant(t, g2Do(r, http.MethodGet, "/wellness/swimmers/sw-a", "coach", ""), http.StatusOK, "today_checkin")
	})
}

func TestCov12gWellnessStorageFailuresReturn500(t *testing.T) {
	t.Parallel()
	pool := testutil.IsolatedSchemaPool(t)
	svc := wellness.NewServiceWithPool(pool)
	pool.Close()
	r := cov12gWellRouter(t, svc, cov12gStaticDir(), cov12gSource{})

	cov12gWant(t, g2Do(r, http.MethodPut, "/wellness/checkins", "parentA", cov12gCheckIn+`,"swimmer_id":"sw-a"}`), http.StatusInternalServerError, "Failed to save check-in.")
	cov12gWant(t, g2Do(r, http.MethodGet, "/wellness/swimmers/sw-a", "coach", ""), http.StatusInternalServerError, "Failed to load check-ins.")
	cov12gWant(t, g2Do(r, http.MethodGet, "/wellness/heatmap", "coach", ""), http.StatusInternalServerError, "Failed to load check-ins.")
}

func TestCov12gHeatmapListsFlaggedSwimmersFirst(t *testing.T) {
	t.Parallel()
	svc := wellness.NewService()
	good := wellness.Metrics{SleepHours: 8, SleepQuality: 4, Soreness: 2, Energy: 4, Stress: 2, Mood: 4}
	distressed := good
	distressed.Mood = 1 // triggers check_in_with_athlete
	for id, m := range map[string]wellness.Metrics{"sw-a": good, "sw-z": distressed} {
		if _, _, err := svc.Upsert(engOrg, wellness.Input{SwimmerID: id, HouseholdID: "hh", Metrics: m, SubmittedBy: "u", SubmittedRole: "parent"}); err != nil {
			t.Fatal(err)
		}
	}
	r := cov12gWellRouter(t, svc, cov12gStaticDir(), cov12gSource{})

	rr := g2Do(r, http.MethodGet, "/wellness/heatmap?group=senior", "coach", "")
	cov12gWant(t, rr, http.StatusOK, "flagged_count")
	out := cov12gBody(t, rr)
	if out["flagged_count"] != float64(1) || out["roster_size"] != float64(2) || out["checked_in_today"] != float64(2) {
		t.Fatalf("unexpected counts: %v", out)
	}
	rows := out["swimmers"].([]any)
	first, second := rows[0].(map[string]any), rows[1].(map[string]any)
	if first["swimmer_id"] != "sw-z" || second["swimmer_id"] != "sw-a" {
		t.Fatalf("flagged swimmer must sort ahead of the alphabetically earlier one: %v then %v", first["swimmer_id"], second["swimmer_id"])
	}
	flags := first["flags"].([]any)
	if len(flags) != 1 || flags[0].(map[string]any)["code"] != "check_in_with_athlete" {
		t.Fatalf("unexpected flags %v", flags)
	}
	if second["flags"] == nil || len(second["flags"].([]any)) != 0 {
		t.Fatalf("unflagged swimmer should have no flags: %v", second["flags"])
	}
	if first["latest"] == nil {
		t.Fatal("swimmer with check-ins should expose their latest one")
	}
}

func TestCov12gAttendanceSummaryWindowAndErrors(t *testing.T) {
	t.Parallel()
	src := cov12gSource{practices: []attendance.Practice{
		{ID: "p1", Date: "2026-10-18", GroupID: "SENIOR", Type: "water"},
		{ID: "p2", Date: "2026-09-01", GroupID: "SENIOR", Type: "water"},
	}}
	r := cov12gWellRouter(t, wellness.NewService(), cov12gStaticDir(), src)

	cov12gWant(t, g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary", "guest", ""), http.StatusForbidden, "Insufficient role.")

	// The default 90-day window sees both practices.
	summary := cov12gBody(t, g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary", "coach", ""))["summary"].(map[string]any)
	if summary["eligible_practices"] != float64(2) || summary["to"] != "2026-10-20" {
		t.Fatalf("default window: %v", summary)
	}

	// An explicit from narrows the window to the recent practice only.
	rr := g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary?from=2026-10-01", "coach", "")
	summary = cov12gBody(t, rr)["summary"].(map[string]any)
	if summary["from"] != "2026-10-01" || summary["eligible_practices"] != float64(1) || summary["present"] != float64(1) {
		t.Fatalf("explicit from: %v", summary)
	}

	// An explicit to excludes later practices.
	rr = g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary?from=2026-08-01&to=2026-09-30", "coach", "")
	summary = cov12gBody(t, rr)["summary"].(map[string]any)
	if summary["to"] != "2026-09-30" || summary["eligible_practices"] != float64(1) {
		t.Fatalf("explicit to: %v", summary)
	}

	// Windows wider than a year are refused before any aggregation.
	cov12gWant(t, g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary?from=2024-01-01&to=2026-01-01", "coach", ""), http.StatusBadRequest, "at most 365 days")

	// Reversed or malformed bounds are an invalid range.
	cov12gWant(t, g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary?from=2026-10-10&to=2026-10-01", "coach", ""), http.StatusBadRequest, "from and to must be YYYY-MM-DD")
	cov12gWant(t, g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary?from=garbage", "coach", ""), http.StatusBadRequest, "from and to must be YYYY-MM-DD")
}

func TestCov12gAttendanceSummarySourceFailureReturns500(t *testing.T) {
	t.Parallel()
	boom := errors.New("training store offline")
	r := cov12gWellRouter(t, wellness.NewService(), cov12gStaticDir(), cov12gSource{err: boom})
	rr := g2Do(r, http.MethodGet, "/swimmers/sw-a/attendance-summary", "coach", "")
	cov12gWant(t, rr, http.StatusInternalServerError, "Failed to aggregate attendance.")
	if strings.Contains(rr.Body.String(), boom.Error()) {
		t.Fatal("the underlying error must not leak to clients")
	}
}
