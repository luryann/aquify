package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/training"
)

type trainingFixture struct {
	t            *testing.T
	router       http.Handler
	org          string
	admin, coach string
	parent       string
	swimmer      string
	stranger     string // parent of another household
	swimmerID    string
	seniorID     string
	ageID        string
}

func (f *trainingFixture) do(method, path, token, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	var reader io.Reader = strings.NewReader(body)
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", f.org)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	return rr
}

func (f *trainingFixture) must(rr *httptest.ResponseRecorder, status int, dst any) {
	f.t.Helper()
	if rr.Code != status {
		f.t.Fatalf("want %d, got %d: %s", status, rr.Code, rr.Body.String())
	}
	if dst != nil {
		if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
			f.t.Fatalf("decode: %v", err)
		}
	}
}

func newTrainingFixture(t *testing.T) *trainingFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	router := NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger,
		nil, memberSvc, groupSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, training.NewService(), nil, nil)
	f := &trainingFixture{t: t, router: router, org: "org-train"}
	f.admin = issueSessionTokenForUser(t, router, "admin-train", f.org, []string{"admin"}, true)
	f.coach = issueSessionTokenForUser(t, router, "coach-train", f.org, []string{"coach"}, false)
	f.parent = issueSessionTokenForUser(t, router, "parent-train", f.org, []string{"parent"}, false)
	f.stranger = issueSessionTokenForUser(t, router, "parent-other", f.org, []string{"parent"}, false)

	var hh householdEnvelope
	f.must(f.do(http.MethodPost, "/api/v1/members/households", f.parent, `{"org_id":"org-train","household_name":"Train Family"}`), http.StatusCreated, &hh)
	f.must(f.do(http.MethodPost, "/api/v1/members/households", f.stranger, `{"org_id":"org-train","household_name":"Other Family"}`), http.StatusCreated, nil)
	var sw swimmerEnvelope
	f.must(f.do(http.MethodPost, "/api/v1/members/swimmers", f.admin, `{"org_id":"org-train","household_id":"`+hh.Household.ID+`","first_name":"Ada","last_name":"Lap","date_of_birth":"2011-02-03","gender":"female"}`), http.StatusCreated, &sw)
	f.swimmerID = sw.Swimmer.ID
	f.swimmer = issueSessionTokenWithHousehold(t, router, "swimmer-train", f.org, hh.Household.ID, []string{"swimmer"}, false)

	var g struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}
	f.must(f.do(http.MethodPost, "/api/v1/groups", f.admin, `{"code":"SENIOR","name":"Senior"}`), http.StatusCreated, &g)
	f.seniorID = g.Group.ID
	f.must(f.do(http.MethodPost, "/api/v1/groups", f.admin, `{"code":"AGE","name":"Age Group"}`), http.StatusCreated, &g)
	f.ageID = g.Group.ID
	f.must(f.do(http.MethodPost, "/api/v1/members/swimmers/"+f.swimmerID+"/groups", f.admin, `{"group_code":"SENIOR","is_primary":true}`), http.StatusCreated, nil)
	return f
}

type workoutEnv struct {
	Workout  training.Workout  `json:"workout"`
	Practice training.Practice `json:"practice"`
}

func TestTrainingWorkoutLibraryAndAssignRoutes(t *testing.T) {
	t.Parallel()
	f := newTrainingFixture(t)

	// Quick entry parse → create template.
	var parsed struct {
		Sections []training.WorkoutSection `json:"sections"`
		Errors   []training.ParseError     `json:"errors"`
		Totals   training.WorkoutTotals    `json:"totals"`
	}
	f.must(f.do(http.MethodPost, "/api/v1/workouts/parse", f.coach, `{"text":"Warmup: 400 choice\nMain\n8x100 free @1:30 EN2\nnonsense","course":"SCY"}`), http.StatusOK, &parsed)
	if parsed.Totals.Distance != 1200 || len(parsed.Errors) != 1 || parsed.Errors[0].Line != 4 {
		t.Fatalf("parse = %+v", parsed)
	}
	sections, _ := json.Marshal(parsed.Sections)
	var created workoutEnv
	f.must(f.do(http.MethodPost, "/api/v1/workouts", f.coach, `{"title":"Threshold","tags":["EN2"],"sections":`+string(sections)+`}`), http.StatusCreated, &created)
	if !created.Workout.IsTemplate || created.Workout.Totals.ByZone["EN2"] != 800 {
		t.Fatalf("created = %+v", created.Workout)
	}
	id := created.Workout.ID

	// Validation errors are 422 with the field path.
	rr := f.do(http.MethodPost, "/api/v1/workouts", f.coach, `{"title":"Bad","sections":[{"kind":"main","sets":[{"repeats":4,"distance":100,"zone":"EN9"}]}]}`)
	if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "sections[0].sets[0].zone") {
		t.Fatalf("invalid workout: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.do(http.MethodPost, "/api/v1/workouts", f.coach, `{"title":"x","bogus":1}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", rr.Code)
	}

	var list struct {
		Workouts []training.Workout `json:"workouts"`
		Total    int                `json:"total"`
	}
	f.must(f.do(http.MethodGet, "/api/v1/workouts?q=thresh&tag=en2", f.admin, ""), http.StatusOK, &list)
	if list.Total != 1 {
		t.Fatalf("search total = %d", list.Total)
	}

	var dup workoutEnv
	f.must(f.do(http.MethodPost, "/api/v1/workouts/"+id+"/duplicate", f.coach, ""), http.StatusCreated, &dup)
	if dup.Workout.SourceWorkoutID != id || dup.Workout.Title != "Threshold (copy)" {
		t.Fatalf("duplicate = %+v", dup.Workout)
	}

	// Assign by group code → canonical group ID, derived yardage.
	var assigned workoutEnv
	f.must(f.do(http.MethodPost, "/api/v1/workouts/"+id+"/assign", f.coach, `{"date":"2026-11-02","group_id":"senior"}`), http.StatusCreated, &assigned)
	if assigned.Practice.GroupID != f.seniorID || assigned.Practice.YardageTarget != 1200 || assigned.Practice.WorkoutID != assigned.Workout.ID {
		t.Fatalf("assigned = %+v", assigned.Practice)
	}
	if rr := f.do(http.MethodPost, "/api/v1/workouts/"+id+"/assign", f.coach, `{"date":"2026-11-02","group_id":"NOPE"}`); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown group: %d", rr.Code)
	}
	if rr := f.do(http.MethodPost, "/api/v1/workouts/"+id+"/assign", f.coach, `{}`); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty assign: %d", rr.Code)
	}

	// Edit practice workout in place; then clear it.
	var set workoutEnv
	f.must(f.do(http.MethodPut, "/api/v1/practices/"+assigned.Practice.ID+"/workout", f.coach, `{"title":"Short","sections":[{"kind":"main","sets":[{"repeats":10,"distance":50}]}]}`), http.StatusOK, &set)
	if set.Workout.ID != assigned.Workout.ID || set.Practice.YardageTarget != 500 {
		t.Fatalf("set practice workout = %+v / %+v", set.Practice, set.Workout)
	}
	if rr := f.do(http.MethodPut, "/api/v1/practices/"+assigned.Practice.ID+"/workout", f.coach, `{"title":"T","is_template":true}`); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("template practice workout: %d", rr.Code)
	}

	// RBAC: parent/swimmer cannot touch the library or write anything.
	for _, tok := range []string{f.parent, f.swimmer} {
		for _, c := range []struct{ method, path, body string }{
			{http.MethodGet, "/api/v1/workouts", ""},
			{http.MethodGet, "/api/v1/workouts/" + id, ""},
			{http.MethodPost, "/api/v1/workouts", `{"title":"x"}`},
			{http.MethodPost, "/api/v1/workouts/parse", `{"text":"400"}`},
			{http.MethodPut, "/api/v1/workouts/" + id, `{"title":"x"}`},
			{http.MethodDelete, "/api/v1/workouts/" + id, ""},
			{http.MethodPost, "/api/v1/workouts/" + id + "/assign", `{"date":"2026-11-03"}`},
			{http.MethodPut, "/api/v1/practices/" + assigned.Practice.ID + "/workout", `{"title":"x"}`},
			{http.MethodDelete, "/api/v1/practices/" + assigned.Practice.ID + "/workout", ""},
			{http.MethodGet, "/api/v1/training/analytics", ""},
		} {
			if rr := f.do(c.method, c.path, tok, c.body); rr.Code != http.StatusForbidden {
				t.Fatalf("%s %s should be 403, got %d", c.method, c.path, rr.Code)
			}
		}
	}

	f.must(f.do(http.MethodDelete, "/api/v1/practices/"+assigned.Practice.ID+"/workout", f.coach, ""), http.StatusOK, nil)
	if rr := f.do(http.MethodGet, "/api/v1/practices/"+assigned.Practice.ID+"/workout", f.coach, ""); rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "no_workout") {
		t.Fatalf("cleared workout: %d %s", rr.Code, rr.Body.String())
	}
	f.must(f.do(http.MethodDelete, "/api/v1/workouts/"+dup.Workout.ID, f.admin, ""), http.StatusOK, nil)
	if rr := f.do(http.MethodGet, "/api/v1/workouts/"+dup.Workout.ID, f.admin, ""); rr.Code != http.StatusNotFound {
		t.Fatalf("deleted workout get: %d", rr.Code)
	}
}

func TestTrainingPortalScopingAndAnalytics(t *testing.T) {
	t.Parallel()
	f := newTrainingFixture(t)
	var tmpl workoutEnv
	f.must(f.do(http.MethodPost, "/api/v1/workouts", f.coach, `{"title":"Main","sections":[{"kind":"main","sets":[{"repeats":4,"distance":100,"zone":"EN1"}]}]}`), http.StatusCreated, &tmpl)
	var senior, age, team workoutEnv
	f.must(f.do(http.MethodPost, "/api/v1/workouts/"+tmpl.Workout.ID+"/assign", f.coach, `{"date":"2026-11-02","group_id":"`+f.seniorID+`"}`), http.StatusCreated, &senior)
	f.must(f.do(http.MethodPost, "/api/v1/workouts/"+tmpl.Workout.ID+"/assign", f.coach, `{"date":"2026-11-03","group_id":"`+f.ageID+`"}`), http.StatusCreated, &age)
	f.must(f.do(http.MethodPost, "/api/v1/workouts/"+tmpl.Workout.ID+"/assign", f.coach, `{"date":"2026-11-04"}`), http.StatusCreated, &team)
	f.must(f.do(http.MethodPut, "/api/v1/practices/"+senior.Practice.ID+"/attendance/"+f.swimmerID, f.coach, `{"status":"present"}`), http.StatusOK, nil)

	type sched struct {
		Items []struct {
			Practice training.Practice `json:"practice"`
			Workout  *training.Workout `json:"workout"`
		} `json:"items"`
	}
	ids := func(tok string) []string {
		var s sched
		f.must(f.do(http.MethodGet, "/api/v1/training/schedule?from=2026-11-01&to=2026-11-08", tok, ""), http.StatusOK, &s)
		out := []string{}
		for _, it := range s.Items {
			if it.Workout == nil {
				t.Fatalf("schedule item missing workout: %+v", it)
			}
			out = append(out, it.Practice.ID)
		}
		return out
	}
	if got := ids(f.coach); len(got) != 3 {
		t.Fatalf("coach schedule = %v", got)
	}
	for name, tok := range map[string]string{"parent": f.parent, "swimmer": f.swimmer} {
		got := ids(tok)
		if len(got) != 2 || got[0] != senior.Practice.ID || got[1] != team.Practice.ID {
			t.Fatalf("%s schedule = %v, want senior + team-wide only", name, got)
		}
		f.must(f.do(http.MethodGet, "/api/v1/practices/"+senior.Practice.ID+"/workout", tok, ""), http.StatusOK, nil)
		if rr := f.do(http.MethodGet, "/api/v1/practices/"+age.Practice.ID+"/workout", tok, ""); rr.Code != http.StatusNotFound {
			t.Fatalf("%s foreign group workout: %d", name, rr.Code)
		}
		if rr := f.do(http.MethodGet, "/api/v1/practices/"+age.Practice.ID, tok, ""); rr.Code != http.StatusNotFound {
			t.Fatalf("%s foreign group practice: %d", name, rr.Code)
		}
		var pl struct {
			Practices []training.Practice `json:"practices"`
		}
		f.must(f.do(http.MethodGet, "/api/v1/practices", tok, ""), http.StatusOK, &pl)
		if len(pl.Practices) != 2 {
			t.Fatalf("%s practice list = %d", name, len(pl.Practices))
		}
	}
	if got := ids(f.stranger); len(got) != 1 || got[0] != team.Practice.ID {
		t.Fatalf("stranger schedule = %v", got)
	}

	// Swimmer analytics: household-scoped.
	var sa training.SwimmerAnalytics
	for _, tok := range []string{f.coach, f.parent, f.swimmer} {
		f.must(f.do(http.MethodGet, "/api/v1/training/analytics/swimmers/"+f.swimmerID+"?from=2026-11-01&to=2026-11-08", tok, ""), http.StatusOK, &sa)
		if sa.Yards != 400 || sa.Present != 1 {
			t.Fatalf("swimmer analytics = %+v", sa)
		}
	}
	if rr := f.do(http.MethodGet, "/api/v1/training/analytics/swimmers/"+f.swimmerID, f.stranger, ""); rr.Code != http.StatusNotFound {
		t.Fatalf("stranger swimmer analytics: %d", rr.Code)
	}

	var ga training.TrainingAnalytics
	f.must(f.do(http.MethodGet, "/api/v1/training/analytics?from=2026-11-01&to=2026-11-08&group_id=SENIOR", f.coach, ""), http.StatusOK, &ga)
	if len(ga.Groups) != 1 || ga.Groups[0].GroupID != f.seniorID || ga.Groups[0].SwimmerYards != 400 || ga.ZoneYards["EN1"] != 400 {
		t.Fatalf("group analytics = %+v", ga)
	}
	if rr := f.do(http.MethodGet, "/api/v1/training/analytics?from=2026-11-08&to=2026-11-01", f.coach, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad range: %d", rr.Code)
	}
}

func TestTrainingSeasonPhaseRoutes(t *testing.T) {
	t.Parallel()
	f := newTrainingFixture(t)
	var season struct {
		Season struct {
			ID string `json:"id"`
		} `json:"season"`
	}
	f.must(f.do(http.MethodPost, "/api/v1/seasons", f.admin, `{"name":"Fall 2026","start_date":"2026-09-07","end_date":"2026-12-20","status":"active"}`), http.StatusCreated, &season)
	base := "/api/v1/seasons/" + season.Season.ID
	var phase struct {
		Phase training.PlanPhase `json:"phase"`
	}
	f.must(f.do(http.MethodPost, base+"/training-phases", f.coach, `{"group_id":"senior","phase":"base","start_date":"2026-09-07","end_date":"2026-10-04","weekly_yardage_target":20000}`), http.StatusCreated, &phase)
	if phase.Phase.GroupID != f.seniorID {
		t.Fatalf("phase group not canonicalised: %+v", phase.Phase)
	}
	if rr := f.do(http.MethodPost, base+"/training-phases", f.coach, `{"group_id":"SENIOR","phase":"build","start_date":"2026-10-01","end_date":"2026-10-20"}`); rr.Code != http.StatusConflict {
		t.Fatalf("overlap: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.do(http.MethodPost, base+"/training-phases", f.coach, `{"group_id":"SENIOR","phase":"build","start_date":"2027-01-01","end_date":"2027-01-20"}`); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("outside season: %d", rr.Code)
	}
	if rr := f.do(http.MethodPost, base+"/training-phases", f.coach, `{"group_id":"GHOST","phase":"build","start_date":"2026-10-05","end_date":"2026-10-20"}`); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown group: %d", rr.Code)
	}
	if rr := f.do(http.MethodPost, "/api/v1/seasons/missing/training-phases", f.coach, `{"group_id":"SENIOR","phase":"build","start_date":"2026-10-05","end_date":"2026-10-20"}`); rr.Code != http.StatusNotFound {
		t.Fatalf("missing season: %d", rr.Code)
	}
	f.must(f.do(http.MethodPut, "/api/v1/training-phases/"+phase.Phase.ID, f.coach, `{"group_id":"SENIOR","phase":"base","start_date":"2026-09-07","end_date":"2026-10-11","weekly_yardage_target":22000}`), http.StatusOK, nil)

	var plan struct {
		Plan training.SeasonPlan `json:"plan"`
	}
	f.must(f.do(http.MethodGet, base+"/training-plan?group_id=SENIOR", f.coach, ""), http.StatusOK, &plan)
	if len(plan.Plan.Weeks) != 15 || plan.Plan.Weeks[0].TargetYards != 22000 {
		t.Fatalf("plan = %+v", plan.Plan)
	}
	for _, tok := range []string{f.parent, f.swimmer} {
		for _, c := range []struct{ method, path, body string }{
			{http.MethodGet, base + "/training-phases", ""},
			{http.MethodGet, base + "/training-plan", ""},
			{http.MethodPost, base + "/training-phases", `{"group_id":"SENIOR","phase":"build","start_date":"2026-10-12","end_date":"2026-10-20"}`},
			{http.MethodDelete, "/api/v1/training-phases/" + phase.Phase.ID, ""},
		} {
			if rr := f.do(c.method, c.path, tok, c.body); rr.Code != http.StatusForbidden {
				t.Fatalf("%s %s should be 403, got %d", c.method, c.path, rr.Code)
			}
		}
	}
	f.must(f.do(http.MethodDelete, "/api/v1/training-phases/"+phase.Phase.ID, f.coach, ""), http.StatusOK, nil)
	if rr := f.do(http.MethodDelete, "/api/v1/training-phases/"+phase.Phase.ID, f.coach, ""); rr.Code != http.StatusNotFound {
		t.Fatalf("second delete: %d", rr.Code)
	}
}
