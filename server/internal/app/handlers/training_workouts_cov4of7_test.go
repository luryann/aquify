package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/luryann/aquify/server/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/seasons"
	"github.com/luryann/aquify/server/internal/app/training"
)

const cvg4TrainOrg = "org-cvg4-training"

// cvg4Router mounts routes behind the real Auth and org-scope middleware.
func cvg4Router(tokens tokenResolver, register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokens))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return r
}

const cvg4Sections = `[{"kind":"warmup","sets":[{"repeats":1,"distance":400,"stroke":"free"}]},` +
	`{"kind":"main","sets":[{"repeats":8,"distance":100,"stroke":"free","interval_seconds":90,"zone":"EN2"}]}]`

func cvg4WorkoutJSON(title string, extra string) string {
	if extra != "" {
		extra += ","
	}
	return fmt.Sprintf(`{%s"title":%q,"sections":%s}`, extra, title, cvg4Sections)
}

type cvg4TrainWorld struct {
	t       *testing.T
	svc     *training.Service
	members *members.Service
	groups  *groups.Service
	seasons *seasons.Service
	audit   *audit.Service
	h       *TrainingHandlers
	router  http.Handler
	bare    http.Handler
	admin   *appmiddleware.Principal
	hhA     string
	swA     string
	hhB     string
	senior  groups.Group
	age     groups.Group
	season  *seasons.Season
}

func cvg4TrainTokens(hhA, hhB string) tokenResolver {
	mk := func(id, hh string, roles ...string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: id, OrgID: cvg4TrainOrg, HouseholdID: hh, Roles: roles}
	}
	return tokenResolver{
		"admin":      mk("u-admin", "", "admin"),
		"coach":      mk("u-coach", "", "coach"),
		"parentA":    mk("u-parent-a", hhA, "parent"),
		"parentB":    mk("u-parent-b", hhB, "parent"),
		"swimmerA":   mk("u-swim-a", hhA, "swimmer"),
		"swimmerNoH": mk("u-swim-x", "", "swimmer"),
		"guest":      mk("u-guest", "", "guest"),
	}
}

func cvg4NewTrainWorld(t *testing.T) *cvg4TrainWorld {
	t.Helper()
	w := &cvg4TrainWorld{
		t:       t,
		svc:     training.NewService(),
		members: members.NewService(),
		groups:  groups.NewService(),
		seasons: seasons.NewService(),
		audit:   audit.NewService(),
		admin:   &appmiddleware.Principal{UserID: "u-admin", OrgID: cvg4TrainOrg, Roles: []string{"admin"}},
	}
	var err error
	if w.senior, _, err = w.groups.CreateGroup(w.admin, cvg4TrainOrg, groups.CreateGroupInput{Code: "SENIOR", Name: "Senior"}); err != nil {
		t.Fatal(err)
	}
	if w.age, _, err = w.groups.CreateGroup(w.admin, cvg4TrainOrg, groups.CreateGroupInput{Code: "AGE", Name: "Age Group"}); err != nil {
		t.Fatal(err)
	}
	mk := func(name, first string) (string, string) {
		hh, err := w.members.CreateHousehold(w.admin, members.CreateHouseholdInput{OrgID: cvg4TrainOrg, HouseholdName: name, PrimaryEmail: strings.ToLower(first) + "@example.test"})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := w.members.CreateSwimmer(w.admin, cvg4TrainOrg, members.CreateSwimmerInput{
			HouseholdID: hh.ID, FirstName: first, LastName: name, DateOfBirth: "2012-04-05", Gender: "female", RegistrationStatus: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return hh.ID, sw.ID
	}
	var swB string
	w.hhA, w.swA = mk("Alpha", "Ada")
	w.hhB, swB = mk("Beta", "Bo")
	for sw, code := range map[string]string{w.swA: "SENIOR", swB: "AGE"} {
		if _, err := w.members.AssignSwimmerToGroup(w.admin, cvg4TrainOrg, sw, members.AssignSwimmerGroupInput{GroupCode: code}); err != nil {
			t.Fatal(err)
		}
	}
	if w.season, err = w.seasons.Create(cvg4TrainOrg, "2026-27", "2026-09-01", "2027-08-31", "active", "", "u-admin"); err != nil {
		t.Fatal(err)
	}
	w.h = NewTrainingHandlersWithMembers(w.svc, w.members, w.groups, w.audit)
	w.h.SetSeasonsService(w.seasons)
	w.build()
	return w
}

func (w *cvg4TrainWorld) build() {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cvg4TrainTokens(w.hhA, w.hhB)))
		p.Use(appmiddleware.RequireOrgScope)
		w.h.RegisterRoutes(p)
	})
	w.router = r
	bare := chi.NewRouter()
	w.h.RegisterRoutes(bare)
	w.bare = bare
}

func (w *cvg4TrainWorld) do(method, path, token, body string) *httptest.ResponseRecorder {
	w.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cvg4TrainOrg)
	}
	rr := httptest.NewRecorder()
	w.router.ServeHTTP(rr, req)
	return rr
}

func cvg4JSON(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return out
}

func cvg4Want(t *testing.T, rr *httptest.ResponseRecorder, status int, code, label string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("%s: want %d, got %d %s", label, status, rr.Code, rr.Body.String())
	}
	if code != "" {
		if got := cvg4ErrCode(t, rr); got != code {
			t.Fatalf("%s: want code %q, got %q (%s)", label, code, got, rr.Body.String())
		}
	}
}

func (w *cvg4TrainWorld) practice(date, groupID, notes string) *training.Practice {
	w.t.Helper()
	p, err := w.svc.Create(cvg4TrainOrg, date, groupID, "water", "Aerobic", 3000, nil, training.PracticeStrokes{}, notes, "u-admin")
	if err != nil {
		w.t.Fatal(err)
	}
	return p
}

func (w *cvg4TrainWorld) auditActions() map[string]int {
	w.t.Helper()
	evs, err := w.audit.Query(audit.Filter{OrgID: cvg4TrainOrg, Limit: 500})
	if err != nil {
		w.t.Fatal(err)
	}
	out := map[string]int{}
	for _, e := range evs {
		out[e.Action]++
	}
	return out
}

func (w *cvg4TrainWorld) createWorkout(token, title, extra string) map[string]any {
	w.t.Helper()
	rr := w.do("POST", "/workouts", token, cvg4WorkoutJSON(title, extra))
	cvg4Want(w.t, rr, http.StatusCreated, "", "create workout")
	return cvg4JSON(w.t, rr)["workout"].(map[string]any)
}

// ─── Error mapping ────────────────────────────────────────────────────────────

func TestCvg4WriteTrainingErrorMapping(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"validation", &training.WorkoutValidationError{Field: "title", Message: "is required"}, 422, "validation_error"},
		{"invalid assignment", training.ErrInvalidAssignment, 422, "validation_error"},
		{"invalid date", training.ErrInvalidDate, 400, "bad_request"},
		{"invalid type", training.ErrInvalidType, 400, "bad_request"},
		{"invalid range", training.ErrInvalidRange, 400, "invalid_range"},
		{"workout missing", training.ErrWorkoutNotFound, 404, "not_found"},
		{"practice without workout", training.ErrPracticeNoWorkout, 404, "no_workout"},
		{"practice missing", training.ErrNotFound, 404, "not_found"},
		{"phase missing", training.ErrPhaseNotFound, 404, "not_found"},
		{"phase overlap", training.ErrPhaseOverlap, 409, "phase_overlap"},
		{"season missing", seasons.ErrNotFound, 404, "not_found"},
		{"wrapped season missing", fmt.Errorf("load: %w", seasons.ErrNotFound), 404, "not_found"},
		{"anything else", errors.New("pq: connection refused"), 500, "internal_error"},
	} {
		rr := httptest.NewRecorder()
		writeTrainingError(rr, httptest.NewRequest(http.MethodGet, "/x", nil), tc.err, "Fallback message.")
		cvg4Want(t, rr, tc.status, tc.code, tc.name)
		if tc.status == 500 {
			if !strings.Contains(rr.Body.String(), "Fallback message.") || strings.Contains(rr.Body.String(), "refused") {
				t.Errorf("500 must show the fallback and hide the cause: %s", rr.Body.String())
			}
		}
	}
	rr := httptest.NewRecorder()
	writeTrainingError(rr, httptest.NewRequest(http.MethodGet, "/x", nil), &training.WorkoutValidationError{Field: "sections", Message: "too many"}, "")
	if !strings.Contains(rr.Body.String(), `"field":"sections"`) || !strings.Contains(rr.Body.String(), "too many") {
		t.Fatalf("validation issue must name the field: %s", rr.Body.String())
	}
}

// ─── Auth context and role gates ──────────────────────────────────────────────

type cvg4Route struct{ method, path, body string }

func cvg4AllRoutes() []cvg4Route {
	return []cvg4Route{
		{"GET", "/workouts", ""},
		{"POST", "/workouts", `{}`},
		{"POST", "/workouts/parse", `{}`},
		{"POST", "/workouts/preview", `{}`},
		{"GET", "/workouts/x", ""},
		{"PUT", "/workouts/x", `{}`},
		{"DELETE", "/workouts/x", ""},
		{"POST", "/workouts/x/duplicate", `{}`},
		{"POST", "/workouts/x/assign", `{}`},
		{"GET", "/practices/x/workout", ""},
		{"PUT", "/practices/x/workout", `{}`},
		{"DELETE", "/practices/x/workout", ""},
		{"GET", "/training/schedule", ""},
		{"GET", "/training/analytics", ""},
		{"GET", "/training/analytics/swimmers/x", ""},
		{"GET", "/seasons/x/training-phases", ""},
		{"POST", "/seasons/x/training-phases", `{}`},
		{"GET", "/seasons/x/training-plan", ""},
		{"PUT", "/training-phases/x", `{}`},
		{"DELETE", "/training-phases/x", ""},
	}
}

func TestCvg4WorkoutRoutesRequireAuthContext(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	for _, rt := range cvg4AllRoutes() {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		rr := httptest.NewRecorder()
		w.bare.ServeHTTP(rr, req)
		cvg4Want(t, rr, http.StatusUnauthorized, "unauthorized", rt.method+" "+rt.path+" without a principal")
	}
}

func TestCvg4StaffOnlyWorkoutRoutesDenyOtherRoles(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	// Routes that parents and swimmers may read are exercised separately.
	readable := map[string]bool{
		"GET /practices/x/workout":           true,
		"GET /training/schedule":             true,
		"GET /training/analytics/swimmers/x": true,
	}
	for _, rt := range cvg4AllRoutes() {
		for _, token := range []string{"parentA", "swimmerA", "guest"} {
			if readable[rt.method+" "+rt.path] && token != "guest" {
				continue
			}
			rr := w.do(rt.method, rt.path, token, rt.body)
			cvg4Want(t, rr, http.StatusForbidden, "forbidden", token+" "+rt.method+" "+rt.path)
		}
	}
	if n := len(w.auditActions()); n != 0 {
		t.Fatalf("denied requests must not be audited, got %d actions", n)
	}
}

// ─── Workout library ──────────────────────────────────────────────────────────

func TestCvg4WorkoutLibraryLifecycle(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)

	tmpl := w.createWorkout("coach", "Threshold ladder", `"tags":["Aerobic"]`)
	if tmpl["is_template"] != true || tmpl["course"] != "SCY" {
		t.Fatalf("library workouts default to SCY templates: %v", tmpl)
	}
	if got := tmpl["totals"].(map[string]any)["distance"]; got != float64(1200) {
		t.Fatalf("server must compute totals (400 + 8x100), got %v", got)
	}
	instance := w.createWorkout("admin", "One-off practice", `"is_template":false`)
	if instance["is_template"] != false {
		t.Fatalf("an explicit is_template=false must be honoured: %v", instance)
	}

	// Invalid input.
	cvg4Want(t, w.do("POST", "/workouts", "coach", `{"title":`), 400, "invalid_request_body", "malformed JSON")
	cvg4Want(t, w.do("POST", "/workouts", "coach", `{"title":"x","bogus":1}`), 400, "invalid_request_body", "unknown field")
	cvg4Want(t, w.do("POST", "/workouts", "coach", `{"title":"","sections":[]}`), 422, "validation_error", "blank title")

	// Listing: templates by default, everything with scope=all, filters, paging.
	list := func(query string) map[string]any {
		rr := w.do("GET", "/workouts"+query, "coach", "")
		cvg4Want(t, rr, http.StatusOK, "", "list "+query)
		return cvg4JSON(t, rr)
	}
	if got := list("")["total"]; got != float64(1) {
		t.Fatalf("default list shows templates only, total=%v", got)
	}
	if got := list("?scope=ALL")["total"]; got != float64(2) {
		t.Fatalf("scope=all lists instances too, total=%v", got)
	}
	if got := list("?scope=templates&q=ladder")["total"]; got != float64(1) {
		t.Fatalf("query filter total=%v", got)
	}
	if got := list("?q=nomatch")["total"]; got != float64(0) {
		t.Fatalf("query filter should exclude, total=%v", got)
	}
	if got := list("?tag=aerobic")["total"]; got != float64(1) {
		t.Fatalf("tag filter total=%v", got)
	}
	paged := list("?scope=all&limit=1")
	if paged["count"] != float64(1) || paged["total"] != float64(2) || paged["limit"] != float64(1) {
		t.Fatalf("pagination metadata wrong: %v", paged)
	}
	cvg4Want(t, w.do("GET", "/workouts?scope=archived", "coach", ""), 400, "bad_request", "unknown scope")

	// Read.
	id := tmpl["id"].(string)
	rr := w.do("GET", "/workouts/"+id, "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "get")
	got := cvg4JSON(t, rr)
	if got["workout"].(map[string]any)["title"] != "Threshold ladder" || !strings.Contains(got["text"].(string), "8 x 100") && !strings.Contains(got["text"].(string), "8x100") {
		t.Fatalf("get must return the workout and its text form: %s", rr.Body.String())
	}
	cvg4Want(t, w.do("GET", "/workouts/nope", "coach", ""), 404, "not_found", "unknown workout")

	// Update keeps the template flag fixed at creation.
	cvg4Want(t, w.do("PUT", "/workouts/"+id, "coach", `{"title":`), 400, "invalid_request_body", "update malformed")
	cvg4Want(t, w.do("PUT", "/workouts/nope", "coach", cvg4WorkoutJSON("x", "")), 404, "not_found", "update unknown")
	cvg4Want(t, w.do("PUT", "/workouts/"+id, "coach", `{"title":"","sections":[]}`), 422, "validation_error", "update invalid")
	rr = w.do("PUT", "/workouts/"+id, "coach", cvg4WorkoutJSON("Renamed", `"is_template":false`))
	cvg4Want(t, rr, http.StatusOK, "", "update")
	updated := cvg4JSON(t, rr)["workout"].(map[string]any)
	if updated["title"] != "Renamed" || updated["is_template"] != true {
		t.Fatalf("title changes but is_template is fixed: %v", updated)
	}

	// Duplicate.
	cvg4Want(t, w.do("POST", "/workouts/"+id+"/duplicate", "coach", `{"title":"`+strings.Repeat("x", 161)+`"}`), 422, "validation_error", "title too long")
	cvg4Want(t, w.do("POST", "/workouts/"+id+"/duplicate", "coach", `{"nope":1}`), 400, "invalid_request_body", "duplicate unknown field")
	cvg4Want(t, w.do("POST", "/workouts/nope/duplicate", "coach", ``), 404, "not_found", "duplicate unknown")
	rr = w.do("POST", "/workouts/"+id+"/duplicate", "coach", ``)
	cvg4Want(t, rr, http.StatusCreated, "", "duplicate with empty body")
	dup := cvg4JSON(t, rr)["workout"].(map[string]any)
	if dup["title"] != "Renamed (copy)" || dup["is_template"] != true || dup["source_workout_id"] != id || dup["id"] == id {
		t.Fatalf("unexpected copy %v", dup)
	}
	rr = w.do("POST", "/workouts/"+id+"/duplicate", "coach", `{"title":"Saved instance","is_template":false}`)
	cvg4Want(t, rr, http.StatusCreated, "", "duplicate as instance")
	if d := cvg4JSON(t, rr)["workout"].(map[string]any); d["title"] != "Saved instance" || d["is_template"] != false {
		t.Fatalf("unexpected copy %v", d)
	}

	// Delete.
	cvg4Want(t, w.do("DELETE", "/workouts/nope", "coach", ""), 404, "not_found", "delete unknown")
	rr = w.do("DELETE", "/workouts/"+id, "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "delete")
	if cvg4JSON(t, rr)["deleted"] != true {
		t.Fatalf("delete body: %s", rr.Body.String())
	}
	cvg4Want(t, w.do("GET", "/workouts/"+id, "coach", ""), 404, "not_found", "deleted workout is gone")

	acts := w.auditActions()
	for action, want := range map[string]int{"workout.create": 2, "workout.update": 1, "workout.duplicate": 2, "workout.delete": 1} {
		if acts[action] != want {
			t.Errorf("audit %s = %d, want %d (all: %v)", action, acts[action], want, acts)
		}
	}
}

func TestCvg4ParseAndPreviewWorkouts(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)

	rr := w.do("POST", "/workouts/parse", "coach", `{"text":"Warmup:\n400 free\nMain\n8x100 free @1:30 EN2\nbogus ???","course":"scm"}`)
	cvg4Want(t, rr, http.StatusOK, "", "parse")
	out := cvg4JSON(t, rr)
	if out["course"] != "SCM" {
		t.Fatalf("course must be normalised: %v", out["course"])
	}
	if secs, _ := out["sections"].([]any); len(secs) != 2 {
		t.Fatalf("want 2 parsed sections, got %v", out["sections"])
	}
	if got := out["totals"].(map[string]any)["distance"]; got != float64(1200) || out["totals"].(map[string]any)["unit"] != "m" {
		t.Fatalf("totals for SCM = %v", out["totals"])
	}
	cvg4Want(t, w.do("POST", "/workouts/parse", "coach", `{"text":`), 400, "invalid_request_body", "parse malformed")
	cvg4Want(t, w.do("POST", "/workouts/parse", "coach", `{"text":"x","course":"XYZ"}`), 422, "validation_error", "parse bad course")
	cvg4Want(t, w.do("POST", "/workouts/parse", "coach", `{"text":"`+strings.Repeat("a", 20001)+`"}`), 422, "validation_error", "parse too long")

	rr = w.do("POST", "/workouts/preview", "admin", `{"course":"LCM","sections":`+cvg4Sections+`}`)
	cvg4Want(t, rr, http.StatusOK, "", "preview")
	prev := cvg4JSON(t, rr)
	if prev["course"] != "LCM" || prev["text"] == "" || prev["totals"].(map[string]any)["distance"] != float64(1200) {
		t.Fatalf("unexpected preview %s", rr.Body.String())
	}
	rr = w.do("GET", "/workouts?scope=all", "admin", "")
	if cvg4JSON(t, rr)["total"] != float64(0) {
		t.Fatalf("parse and preview must not persist anything: %s", rr.Body.String())
	}
	cvg4Want(t, w.do("POST", "/workouts/preview", "admin", `{"sections":`), 400, "invalid_request_body", "preview malformed")
	cvg4Want(t, w.do("POST", "/workouts/preview", "admin", `{"course":"XYZ","sections":[]}`), 422, "validation_error", "preview bad course")
	cvg4Want(t, w.do("POST", "/workouts/preview", "admin", `{"sections":[{"kind":"nonsense","sets":[]}]}`), 422, "validation_error", "preview bad section")
}

func TestCvg4AssignWorkout(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	src := w.createWorkout("coach", "Template", "")
	id := src["id"].(string)
	assign := func(body string) *httptest.ResponseRecorder {
		return w.do("POST", "/workouts/"+id+"/assign", "coach", body)
	}

	cvg4Want(t, assign(`{"date":`), 400, "invalid_request_body", "malformed JSON")
	cvg4Want(t, assign(`{"date":"2026-10-05","group_id":"NOPE"}`), 422, "validation_error", "unknown group")
	cvg4Want(t, assign(`{}`), 422, "validation_error", "neither practice nor date")
	cvg4Want(t, assign(`{"date":"05/10/2026"}`), 422, "validation_error", "bad date")
	cvg4Want(t, assign(`{"date":"2026-10-05","type":"party"}`), 400, "bad_request", "bad type")
	cvg4Want(t, w.do("POST", "/workouts/nope/assign", "coach", `{"date":"2026-10-05"}`), 404, "not_found", "unknown workout")
	cvg4Want(t, assign(`{"practice_id":"nope"}`), 404, "not_found", "unknown practice")

	// A group code in any case is stored as the group's ID.
	rr := assign(`{"date":"2026-10-05","group_id":" senior "}`)
	cvg4Want(t, rr, http.StatusCreated, "", "assign to a new practice")
	out := cvg4JSON(t, rr)
	practice, wk := out["practice"].(map[string]any), out["workout"].(map[string]any)
	if practice["group_id"] != w.senior.ID || practice["date"] != "2026-10-05" || practice["yardage_target"] != float64(1200) {
		t.Fatalf("unexpected practice %v", practice)
	}
	if wk["is_template"] != false || wk["source_workout_id"] != id || wk["id"] == id {
		t.Fatalf("assignment must copy the template into a practice-owned instance: %v", wk)
	}

	// Assigning to an existing practice replaces its instance.
	existing := w.practice("2026-10-06", "", "")
	rr = assign(fmt.Sprintf(`{"practice_id":%q,"group_id":"ignored"}`, existing.ID))
	cvg4Want(t, rr, http.StatusCreated, "", "assign to an existing practice")
	if p := cvg4JSON(t, rr)["practice"].(map[string]any); p["id"] != existing.ID || p["workout_id"] == "" {
		t.Fatalf("unexpected practice %v", p)
	}
	if got := w.auditActions()["workout.assign"]; got != 2 {
		t.Fatalf("audit workout.assign = %d, want 2", got)
	}
}

// Without a groups service any group reference is accepted verbatim.
func TestCvg4AssignWorkoutWithoutGroupDirectory(t *testing.T) {
	t.Parallel()
	svc := training.NewService()
	h := NewTrainingHandlers(svc, audit.NewService())
	r := cvg4Router(cvg4TrainTokens("hh", "hh2"), h.RegisterRoutes)
	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cvg4TrainOrg)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}
	rr := do("POST", "/workouts", "coach", cvg4WorkoutJSON("T", ""))
	cvg4Want(t, rr, 201, "", "create")
	id := cvg4JSON(t, rr)["workout"].(map[string]any)["id"].(string)
	rr = do("POST", "/workouts/"+id+"/assign", "coach", `{"date":"2026-10-05","group_id":"Any-Group"}`)
	cvg4Want(t, rr, 201, "", "assign")
	if p := cvg4JSON(t, rr)["practice"].(map[string]any); p["group_id"] != "Any-Group" {
		t.Fatalf("group reference must be kept as typed: %v", p)
	}

	// Without a members service a parent sees only team-wide practices.
	if _, err := svc.Create(cvg4TrainOrg, "2026-10-07", "", "water", "Team-wide", 1000, nil, training.PracticeStrokes{}, "", "u"); err != nil {
		t.Fatal(err)
	}
	rr = do("GET", "/training/schedule?from=2026-10-01&to=2026-10-31", "parentA", "")
	cvg4Want(t, rr, 200, "", "parent schedule")
	items := cvg4JSON(t, rr)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["practice"].(map[string]any)["description"] != "Team-wide" {
		t.Fatalf("parent must only see team-wide practices, got %v", items)
	}
	// And swimmer analytics is unavailable.
	rr = do("GET", "/training/analytics/swimmers/s1", "parentA", "")
	cvg4Want(t, rr, http.StatusServiceUnavailable, "unavailable", "swimmer analytics without members")
}

// ─── Practice workouts ────────────────────────────────────────────────────────

func TestCvg4PracticeWorkoutLifecycleAndScope(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	seniorPractice := w.practice("2026-10-05", w.senior.ID, "coach-only notes")
	agePractice := w.practice("2026-10-05", "age", "")
	teamPractice := w.practice("2026-10-06", "", "")

	cvg4Want(t, w.do("GET", "/practices/"+seniorPractice.ID+"/workout", "coach", ""), 404, "no_workout", "no workout yet")
	cvg4Want(t, w.do("GET", "/practices/nope/workout", "coach", ""), 404, "not_found", "unknown practice")
	cvg4Want(t, w.do("DELETE", "/practices/nope/workout", "coach", ""), 404, "not_found", "clear unknown practice")
	cvg4Want(t, w.do("DELETE", "/practices/"+seniorPractice.ID+"/workout", "coach", ""), 404, "no_workout", "clear without workout")

	set := func(id, body string) *httptest.ResponseRecorder {
		return w.do("PUT", "/practices/"+id+"/workout", "coach", body)
	}
	cvg4Want(t, set(seniorPractice.ID, `{"title":`), 400, "invalid_request_body", "malformed JSON")
	cvg4Want(t, set(seniorPractice.ID, cvg4WorkoutJSON("x", `"is_template":true`)), 422, "validation_error", "template flag refused")
	cvg4Want(t, set("nope", cvg4WorkoutJSON("x", "")), 404, "not_found", "unknown practice")
	cvg4Want(t, set(seniorPractice.ID, `{"sections":[{"kind":"main","sets":[{"repeats":0}]}]}`), 422, "validation_error", "invalid sections")
	rr := set(seniorPractice.ID, cvg4WorkoutJSON("Race pace", ""))
	cvg4Want(t, rr, http.StatusOK, "", "set")
	out := cvg4JSON(t, rr)
	if p := out["practice"].(map[string]any); p["yardage_target"] != float64(1200) || p["workout_id"] == "" {
		t.Fatalf("yardage must derive from the workout: %v", p)
	}
	if wk := out["workout"].(map[string]any); wk["is_template"] != false || wk["title"] != "Race pace" {
		t.Fatalf("unexpected workout %v", wk)
	}
	// A second PUT updates the same instance in place.
	firstID := out["workout"].(map[string]any)["id"]
	rr = set(seniorPractice.ID, cvg4WorkoutJSON("Race pace v2", ""))
	cvg4Want(t, rr, http.StatusOK, "", "set again")
	if again := cvg4JSON(t, rr)["workout"].(map[string]any); again["id"] != firstID || again["title"] != "Race pace v2" {
		t.Fatalf("second save must update the instance: %v", again)
	}

	// Staff read everything, including notes.
	rr = w.do("GET", "/practices/"+seniorPractice.ID+"/workout", "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "staff read")
	if p := cvg4JSON(t, rr)["practice"].(map[string]any); p["notes"] != "coach-only notes" {
		t.Fatalf("staff must see notes: %v", p)
	}
	// A parent of a senior swimmer sees it, minus the coach notes.
	rr = w.do("GET", "/practices/"+seniorPractice.ID+"/workout", "parentA", "")
	cvg4Want(t, rr, http.StatusOK, "", "parent in group")
	if p := cvg4JSON(t, rr)["practice"].(map[string]any); p["notes"] != nil {
		t.Fatalf("notes must be redacted for families: %v", p)
	}
	rr = w.do("GET", "/practices/"+seniorPractice.ID+"/workout", "swimmerA", "")
	cvg4Want(t, rr, http.StatusOK, "", "swimmer in group")
	// Other households cannot tell the practice exists.
	cvg4Want(t, w.do("GET", "/practices/"+seniorPractice.ID+"/workout", "parentB", ""), 404, "not_found", "parent outside the group")
	cvg4Want(t, w.do("GET", "/practices/"+agePractice.ID+"/workout", "parentA", ""), 404, "not_found", "other group's practice")
	cvg4Want(t, w.do("GET", "/practices/"+teamPractice.ID+"/workout", "parentA", ""), 404, "no_workout", "team-wide practice is visible")

	// Clearing removes the link.
	rr = w.do("DELETE", "/practices/"+seniorPractice.ID+"/workout", "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "clear")
	if p := cvg4JSON(t, rr)["practice"].(map[string]any); p["workout_id"] != nil {
		t.Fatalf("workout link must be removed: %v", p)
	}
	cvg4Want(t, w.do("GET", "/practices/"+seniorPractice.ID+"/workout", "coach", ""), 404, "no_workout", "after clear")

	acts := w.auditActions()
	if acts["practice.workout.set"] != 2 || acts["practice.workout.clear"] != 1 {
		t.Fatalf("unexpected audit trail %v", acts)
	}
}

// ─── Schedule and analytics ───────────────────────────────────────────────────

func TestCvg4TrainingSchedule(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	senior := w.practice("2026-10-05", w.senior.ID, "private")
	w.practice("2026-10-06", "AGE", "")
	team := w.practice("2026-10-07", "", "")
	w.practice("2026-12-25", "", "") // outside the queried window
	if _, _, err := w.svc.SetPracticeWorkout(cvg4TrainOrg, senior.ID, training.WorkoutInput{Title: "W", Sections: cvg4Parse(t, cvg4Sections)}, "u-admin"); err != nil {
		t.Fatal(err)
	}
	const window = "?from=2026-10-01&to=2026-10-31"

	rr := w.do("GET", "/training/schedule"+window, "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "staff schedule")
	items := cvg4JSON(t, rr)["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("staff see every practice in the window, got %d", len(items))
	}
	if first := items[0].(map[string]any); first["practice"].(map[string]any)["id"] != senior.ID || first["workout"] == nil {
		t.Fatalf("schedule reads oldest-first with workouts attached: %v", first)
	}
	if last := items[2].(map[string]any); last["practice"].(map[string]any)["id"] != team.ID || last["workout"] != nil {
		t.Fatalf("practice without workout has a null workout: %v", last)
	}

	// Group filter accepts a code in any case.
	rr = w.do("GET", "/training/schedule"+window+"&group_id=senior", "coach", "")
	if got := cvg4JSON(t, rr)["items"].([]any); len(got) != 1 || got[0].(map[string]any)["practice"].(map[string]any)["id"] != senior.ID {
		t.Fatalf("group filter result %v", got)
	}

	// Families see only team-wide and own-group practices, without notes.
	rr = w.do("GET", "/training/schedule"+window, "parentA", "")
	cvg4Want(t, rr, http.StatusOK, "", "parent schedule")
	pitems := cvg4JSON(t, rr)["items"].([]any)
	if len(pitems) != 2 {
		t.Fatalf("parent A should see 2 practices (senior + team-wide), got %d", len(pitems))
	}
	for _, it := range pitems {
		if it.(map[string]any)["practice"].(map[string]any)["notes"] != nil {
			t.Fatalf("notes must be redacted for families: %v", it)
		}
	}
	rr = w.do("GET", "/training/schedule"+window, "swimmerA", "")
	if got := cvg4JSON(t, rr)["items"].([]any); len(got) != 2 {
		t.Fatalf("swimmer in the senior group sees 2 practices, got %d", len(got))
	}

	// Default window is today through four weeks ahead.
	before := time.Now().UTC()
	rr = w.do("GET", "/training/schedule", "coach", "")
	after := time.Now().UTC()
	cvg4Want(t, rr, http.StatusOK, "", "default window")
	out := cvg4JSON(t, rr)
	okWindow := false
	for _, now := range []time.Time{before, after} { // tolerate a UTC midnight rollover mid-request
		if out["from"] == now.Format("2006-01-02") && out["to"] == now.AddDate(0, 0, 27).Format("2006-01-02") {
			okWindow = true
		}
	}
	if !okWindow {
		t.Fatalf("unexpected default window %v..%v", out["from"], out["to"])
	}
	cvg4Want(t, w.do("GET", "/training/schedule?from=2026-10-31&to=2026-10-01", "coach", ""), 400, "invalid_range", "inverted range")
	cvg4Want(t, w.do("GET", "/training/schedule?from=2026-01-01&to=2028-01-01", "coach", ""), 400, "invalid_range", "oversized range")
	cvg4Want(t, w.do("GET", "/training/schedule?from=garbage", "coach", ""), 400, "invalid_range", "garbage date")
}

func cvg4Parse(t *testing.T, raw string) []training.WorkoutSection {
	t.Helper()
	var s []training.WorkoutSection
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCvg4TrainingAnalytics(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	p := w.practice("2026-10-05", w.senior.ID, "")
	if _, err := w.svc.SetAttendance(cvg4TrainOrg, p.ID, w.swA, "present", "u-admin"); err != nil {
		t.Fatal(err)
	}
	const window = "?from=2026-10-01&to=2026-10-31"

	rr := w.do("GET", "/training/analytics"+window, "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "group analytics")
	if out := cvg4JSON(t, rr); out["from"] != "2026-10-01" || out["to"] != "2026-10-31" {
		t.Fatalf("unexpected analytics %s", rr.Body.String())
	}
	cvg4Want(t, w.do("GET", "/training/analytics"+window+"&group_id=senior", "admin", ""), 200, "", "group filter")
	cvg4Want(t, w.do("GET", "/training/analytics?from=2026-10-31&to=2026-10-01", "admin", ""), 400, "invalid_range", "inverted")

	// Per-swimmer analytics are household scoped.
	path := "/training/analytics/swimmers/" + w.swA
	rr = w.do("GET", path+window, "parentA", "")
	cvg4Want(t, rr, http.StatusOK, "", "own child")
	out := cvg4JSON(t, rr)
	if out["swimmer_id"] != w.swA || out["scheduled"] != float64(1) || out["present"] != float64(1) {
		t.Fatalf("unexpected swimmer analytics %s", rr.Body.String())
	}
	cvg4Want(t, w.do("GET", path+window, "coach", ""), 200, "", "coach")
	cvg4Want(t, w.do("GET", path+window, "swimmerA", ""), 200, "", "the swimmer")
	cvg4Want(t, w.do("GET", path+window, "parentB", ""), 404, "not_found", "another household's child")
	cvg4Want(t, w.do("GET", "/training/analytics/swimmers/nope"+window, "coach", ""), 404, "not_found", "unknown swimmer")
	cvg4Want(t, w.do("GET", path+"?from=bad", "parentA", ""), 400, "invalid_range", "bad range")
	// A swimmer account that is not linked to a household cannot see anyone.
	cvg4Want(t, w.do("GET", path+window, "swimmerNoH", ""), 404, "not_found", "unlinked swimmer account")
}

// ─── Season periodization ─────────────────────────────────────────────────────

func cvg4PhaseBody(group, phase, start, end string) string {
	return fmt.Sprintf(`{"group_id":%q,"phase":%q,"name":"Block","start_date":%q,"end_date":%q,"weekly_yardage_target":20000,"focus":"aerobic"}`, group, phase, start, end)
}

func TestCvg4TrainingPhasesLifecycle(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	base := "/seasons/" + w.season.ID

	// Unknown seasons.
	cvg4Want(t, w.do("GET", "/seasons/nope/training-phases", "coach", ""), 404, "not_found", "list unknown season")
	cvg4Want(t, w.do("POST", "/seasons/nope/training-phases", "coach", cvg4PhaseBody("senior", "base", "2026-09-01", "2026-10-31")), 404, "not_found", "create unknown season")
	cvg4Want(t, w.do("GET", "/seasons/nope/training-plan", "coach", ""), 404, "not_found", "plan unknown season")

	// Create.
	cvg4Want(t, w.do("POST", base+"/training-phases", "coach", `{"group_id":`), 400, "invalid_request_body", "malformed JSON")
	cvg4Want(t, w.do("POST", base+"/training-phases", "coach", `{"phase":"base"}`), 422, "validation_error", "missing fields")
	cvg4Want(t, w.do("POST", base+"/training-phases", "coach", cvg4PhaseBody("nope", "base", "2026-09-01", "2026-10-31")), 422, "validation_error", "unknown group")
	cvg4Want(t, w.do("POST", base+"/training-phases", "coach", cvg4PhaseBody("senior", "sprint", "2026-09-01", "2026-10-31")), 422, "validation_error", "unknown phase kind")
	cvg4Want(t, w.do("POST", base+"/training-phases", "coach", cvg4PhaseBody("senior", "base", "2025-01-01", "2025-02-01")), 422, "validation_error", "outside the season")
	rr := w.do("POST", base+"/training-phases", "coach", cvg4PhaseBody(" senior ", "BASE", "2026-09-01", "2026-10-31"))
	cvg4Want(t, rr, http.StatusCreated, "", "create")
	phase := cvg4JSON(t, rr)["phase"].(map[string]any)
	if phase["group_id"] != w.senior.ID || phase["phase"] != "base" || phase["season_id"] != w.season.ID {
		t.Fatalf("group codes are canonicalised to IDs and phase kinds lower-cased: %v", phase)
	}
	cvg4Want(t, w.do("POST", base+"/training-phases", "coach", cvg4PhaseBody("SENIOR", "build", "2026-10-15", "2026-11-30")), 409, "phase_overlap", "overlap")
	rr = w.do("POST", base+"/training-phases", "coach", cvg4PhaseBody("age", "base", "2026-10-15", "2026-11-30"))
	cvg4Want(t, rr, http.StatusCreated, "", "another group may overlap")
	phaseID := phase["id"].(string)

	// List and plan.
	rr = w.do("GET", base+"/training-phases", "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "list")
	if got := cvg4JSON(t, rr)["phases"].([]any); len(got) != 2 {
		t.Fatalf("want 2 phases, got %d", len(got))
	}
	rr = w.do("GET", base+"/training-plan", "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "plan")
	plan := cvg4JSON(t, rr)["plan"].(map[string]any)
	if plan["season_id"] != w.season.ID || len(plan["phases"].([]any)) != 2 || len(plan["weeks"].([]any)) == 0 {
		t.Fatalf("unexpected plan %v", plan)
	}
	rr = w.do("GET", base+"/training-plan?group_id=senior", "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "plan filtered")
	if got := cvg4JSON(t, rr)["plan"].(map[string]any)["phases"].([]any); len(got) != 1 {
		t.Fatalf("group filter should keep one phase, got %d", len(got))
	}

	// Update.
	cvg4Want(t, w.do("PUT", "/training-phases/"+phaseID, "coach", `{"group_id":`), 400, "invalid_request_body", "update malformed")
	cvg4Want(t, w.do("PUT", "/training-phases/nope", "coach", cvg4PhaseBody("senior", "build", "2026-09-01", "2026-10-31")), 404, "not_found", "update unknown phase")
	cvg4Want(t, w.do("PUT", "/training-phases/"+phaseID, "coach", cvg4PhaseBody("nope", "build", "2026-09-01", "2026-10-31")), 422, "validation_error", "update unknown group")
	cvg4Want(t, w.do("PUT", "/training-phases/"+phaseID, "coach", cvg4PhaseBody("senior", "build", "2025-01-01", "2025-02-01")), 422, "validation_error", "update outside season")
	rr = w.do("PUT", "/training-phases/"+phaseID, "coach", cvg4PhaseBody("senior", "build", "2026-09-01", "2026-10-20"))
	cvg4Want(t, rr, http.StatusOK, "", "update")
	if got := cvg4JSON(t, rr)["phase"].(map[string]any); got["phase"] != "build" || got["end_date"] != "2026-10-20" {
		t.Fatalf("unexpected phase %v", got)
	}

	// Delete.
	cvg4Want(t, w.do("DELETE", "/training-phases/nope", "coach", ""), 404, "not_found", "delete unknown")
	rr = w.do("DELETE", "/training-phases/"+phaseID, "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "delete")
	cvg4Want(t, w.do("DELETE", "/training-phases/"+phaseID, "coach", ""), 404, "not_found", "already deleted")

	// A phase whose season has since been removed cannot be edited.
	rr = w.do("GET", base+"/training-phases", "coach", "")
	other := cvg4JSON(t, rr)["phases"].([]any)[0].(map[string]any)["id"].(string)
	if err := w.seasons.Delete(cvg4TrainOrg, w.season.ID); err != nil {
		t.Fatal(err)
	}
	cvg4Want(t, w.do("PUT", "/training-phases/"+other, "coach", cvg4PhaseBody("age", "base", "2026-10-15", "2026-11-30")), 404, "not_found", "season removed")

	acts := w.auditActions()
	for action, want := range map[string]int{"training_phase.create": 2, "training_phase.update": 1, "training_phase.delete": 1} {
		if acts[action] != want {
			t.Errorf("audit %s = %d, want %d (all: %v)", action, acts[action], want, acts)
		}
	}
}

func TestCvg4PhasesWithoutSeasonsService(t *testing.T) {
	t.Parallel()
	h := NewTrainingHandlers(training.NewService(), audit.NewService())
	r := cvg4Router(cvg4TrainTokens("hh", "hh2"), h.RegisterRoutes)
	req := httptest.NewRequest("GET", "/seasons/s1/training-phases", nil)
	req.Header.Set("Authorization", "Bearer coach")
	req.Header.Set("X-Org-Id", cvg4TrainOrg)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	cvg4Want(t, rr, http.StatusServiceUnavailable, "unavailable", "seasons unavailable")
}

// ─── Infrastructure failures ──────────────────────────────────────────────────

// With the training database gone every read/write must answer 500 without
// leaking the cause, while still honouring authorization first.
func TestCvg4TrainingDatabaseFailureReturns500(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	broken := training.NewServiceWithPool(testutil.ClosedPool(t))
	w.h = NewTrainingHandlersWithMembers(broken, w.members, w.groups, w.audit)
	w.h.SetSeasonsService(w.seasons)
	w.build()

	const window = "?from=2026-10-01&to=2026-10-31"
	for _, rt := range []struct {
		cvg4Route
		token string
	}{
		{cvg4Route{"GET", "/workouts", ""}, "coach"},
		{cvg4Route{"GET", "/training/schedule" + window, ""}, "coach"},
		{cvg4Route{"GET", "/training/analytics" + window, ""}, "coach"},
		{cvg4Route{"GET", "/training/analytics/swimmers/" + w.swA + window, ""}, "parentA"},
		{cvg4Route{"GET", "/seasons/" + w.season.ID + "/training-phases", ""}, "coach"},
		{cvg4Route{"GET", "/seasons/" + w.season.ID + "/training-plan", ""}, "coach"},
		{cvg4Route{"GET", "/practices/x/workout", ""}, "coach"},
	} {
		rr := w.do(rt.method, rt.path, rt.token, rt.body)
		cvg4Want(t, rr, http.StatusInternalServerError, "internal_error", rt.method+" "+rt.path)
		if strings.Contains(rr.Body.String(), "127.0.0.1") || strings.Contains(rr.Body.String(), "closed pool") {
			t.Errorf("%s %s leaked the cause: %s", rt.method, rt.path, rr.Body.String())
		}
	}
}

// If the members directory fails, family-scoped reads fail closed (500) rather
// than widening access.
func TestCvg4FamilyReadsFailClosedWhenMembersUnavailable(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	p := w.practice("2026-10-05", w.senior.ID, "")
	w.h = NewTrainingHandlersWithMembers(w.svc, members.NewServiceWithPool(testutil.ClosedPool(t)), w.groups, w.audit)
	w.build()

	rr := w.do("GET", "/practices/"+p.ID+"/workout", "parentA", "")
	cvg4Want(t, rr, http.StatusInternalServerError, "internal_error", "practice workout")
	rr = w.do("GET", "/training/schedule?from=2026-10-01&to=2026-10-31", "parentA", "")
	cvg4Want(t, rr, http.StatusInternalServerError, "internal_error", "schedule")
	rr = w.do("GET", "/training/analytics/swimmers/"+w.swA+"?from=2026-10-01&to=2026-10-31", "parentA", "")
	cvg4Want(t, rr, http.StatusInternalServerError, "internal_error", "swimmer analytics")
	// Staff never consult the members directory for scoping.
	cvg4Want(t, w.do("GET", "/practices/"+p.ID+"/workout", "coach", ""), http.StatusNotFound, "no_workout", "staff unaffected")
}
