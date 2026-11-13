package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/training"
	"github.com/luryann/aquify/server/internal/testutil"
)

type cov3xTrainWorld struct {
	svc      *training.Service
	members  *members.Service
	groups   *groups.Service
	audit    *audit.Service
	h        *TrainingHandlers
	r        http.Handler
	admin    *appmiddleware.Principal
	hhA, hhB string
	swA, swB string
	senior   groups.Group
	age      groups.Group
}

func newCov3xTrainWorld(t *testing.T) *cov3xTrainWorld {
	t.Helper()
	w := &cov3xTrainWorld{
		svc:     training.NewService(),
		members: members.NewService(),
		groups:  groups.NewService(),
		audit:   audit.NewService(),
		admin:   &appmiddleware.Principal{UserID: cov3xAdminID, OrgID: cov3xOrg, Roles: []string{"admin"}},
	}
	var err error
	if w.senior, _, err = w.groups.CreateGroup(w.admin, cov3xOrg, groups.CreateGroupInput{Code: "SENIOR", Name: "Senior"}); err != nil {
		t.Fatal(err)
	}
	if w.age, _, err = w.groups.CreateGroup(w.admin, cov3xOrg, groups.CreateGroupInput{Code: "AGE", Name: "Age Group"}); err != nil {
		t.Fatal(err)
	}
	mk := func(name, first, preferred string) (string, string) {
		hh, err := w.members.CreateHousehold(w.admin, members.CreateHouseholdInput{OrgID: cov3xOrg, HouseholdName: name, PrimaryEmail: strings.ToLower(first) + "@example.test"})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := w.members.CreateSwimmer(w.admin, cov3xOrg, members.CreateSwimmerInput{
			HouseholdID: hh.ID, FirstName: first, LastName: name, PreferredName: preferred, DateOfBirth: "2012-04-05", Gender: "female", RegistrationStatus: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return hh.ID, sw.ID
	}
	w.hhA, w.swA = mk("Alpha", "Ada", "Addie")
	w.hhB, w.swB = mk("Beta", "Bo", "")
	assign := func(sw, code string) {
		if _, err := w.members.AssignSwimmerToGroup(w.admin, cov3xOrg, sw, members.AssignSwimmerGroupInput{GroupCode: code, IsPrimary: false}); err != nil {
			t.Fatal(err)
		}
	}
	assign(w.swA, "SENIOR")
	assign(w.swA, "AGE") // Ada is in two groups: team-wide rosters must list her once.
	assign(w.swB, "AGE")
	w.h = NewTrainingHandlersWithMembers(w.svc, w.members, w.groups, w.audit)
	w.r = cov3xRouterWith(w.tokens(), w.h.RegisterRoutes)
	return w
}

func (w *cov3xTrainWorld) tokens() tokenResolver {
	tk := cov3xTokens()
	tk["parentA"] = &appmiddleware.Principal{UserID: cov3xParentID, OrgID: cov3xOrg, HouseholdID: w.hhA, Roles: []string{"parent"}}
	tk["parentB"] = &appmiddleware.Principal{UserID: cov3xParent2ID, OrgID: cov3xOrg, HouseholdID: w.hhB, Roles: []string{"parent"}}
	return tk
}

func (w *cov3xTrainWorld) do(method, path, token, body string) *httptest.ResponseRecorder {
	return cov3xDo(w.r, method, path, token, body)
}

func (w *cov3xTrainWorld) create(t *testing.T, group, notes string) string {
	t.Helper()
	body := fmt.Sprintf(`{"date":"2026-03-02","group_id":%q,"type":"water","description":"Aerobic set","yardage_target":4500,"notes":%q}`, group, notes)
	rr := w.do(http.MethodPost, "/practices", "coach", body)
	cov3xWant(t, rr, http.StatusCreated, "practice")
	return cov3xJSON(t, rr)["practice"].(map[string]any)["id"].(string)
}

func TestCov3xTrainingScopeGuards(t *testing.T) {
	w := newCov3xTrainWorld(t)
	cov3xAssertScopeGuards(t, w.h.RegisterRoutes, []cov3xRoute{
		{http.MethodGet, "/practices", ""},
		{http.MethodPost, "/practices", `{}`},
		{http.MethodGet, "/practices/x", ""},
		{http.MethodPut, "/practices/x", `{}`},
		{http.MethodDelete, "/practices/x", ""},
		{http.MethodGet, "/practices/x/attendance", ""},
		{http.MethodPut, "/practices/x/attendance/y", `{}`},
	})
}

func TestCov3xTrainingConstructors(t *testing.T) {
	svc := training.NewService()
	a := audit.NewService()
	plain := NewTrainingHandlers(svc, a)
	if plain.service != svc || plain.auditService != a || plain.membersService != nil || plain.groupsService != nil || plain.validate == nil {
		t.Fatalf("unexpected plain wiring: %+v", plain)
	}
	// Without a members service the attendance roster is simply empty.
	r := cov3xRouter(plain.RegisterRoutes)
	rr := cov3xDo(r, http.MethodPost, "/practices", "admin", `{"date":"2026-03-02","type":"water"}`)
	cov3xWant(t, rr, http.StatusCreated, "")
	id := cov3xJSON(t, rr)["practice"].(map[string]any)["id"].(string)
	rr = cov3xDo(r, http.MethodGet, "/practices/"+id+"/attendance", "admin", "")
	cov3xWant(t, rr, http.StatusOK, `"roster":[]`)
	// ...and attendance can be marked for any swimmer id.
	rr = cov3xDo(r, http.MethodPut, "/practices/"+id+"/attendance/"+cov3xSw1, "admin", `{"status":"excused"}`)
	cov3xWant(t, rr, http.StatusOK, "excused")

	m, g := members.NewService(), groups.NewService()
	full := NewTrainingHandlersWithMembers(svc, m, g, a)
	if full.membersService != m || full.groupsService != g || full.auditService != a {
		t.Fatalf("unexpected full wiring: %+v", full)
	}
}

func TestCov3xPracticeCRUDAndValidation(t *testing.T) {
	w := newCov3xTrainWorld(t)

	cov3xWant(t, w.do(http.MethodPost, "/practices", "parentA", `{"date":"2026-03-02","type":"water"}`), http.StatusForbidden, "Only admin/coach may create")
	cov3xWant(t, w.do(http.MethodPost, "/practices", "coach", `{}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, w.do(http.MethodPost, "/practices", "coach", `{bad`), http.StatusBadRequest, "bad_request")
	cov3xWant(t, w.do(http.MethodPost, "/practices", "coach", `{"date":"2026-13-45","type":"water"}`), http.StatusBadRequest, "YYYY-MM-DD")
	cov3xWant(t, w.do(http.MethodPost, "/practices", "coach", `{"date":"2026-03-02","type":"swim"}`), http.StatusBadRequest, "Invalid practice type")
	cov3xWant(t, w.do(http.MethodPost, "/practices", "coach", `{"date":"2026-03-02","type":"water","yardage_target":999999999}`), http.StatusBadRequest, "between 0 and 200000")

	rr := w.do(http.MethodPost, "/practices", "coach",
		`{"date":"2026-03-02","group_id":"SENIOR","type":"water","description":"Threshold","yardage_target":5000,"attendance":12,"strokes":{"free":3000,"back":500,"breast":500,"fly":500,"im":500},"notes":"coach only"}`)
	cov3xWant(t, rr, http.StatusCreated, "")
	p := cov3xJSON(t, rr)["practice"].(map[string]any)
	id := p["id"].(string)
	st := p["strokes"].(map[string]any)
	if p["yardage_target"] != float64(5000) || p["attendance"] != float64(12) || st["free"] != float64(3000) || st["im"] != float64(500) || p["notes"] != "coach only" {
		t.Fatalf("unexpected practice: %v", p)
	}
	if got, _ := w.audit.Query(audit.Filter{OrgID: cov3xOrg, Action: "practice.create"}); len(got) != 1 {
		t.Fatalf("create must be audited, got %d", len(got))
	}

	// update
	up := "/practices/" + id
	cov3xWant(t, w.do(http.MethodPut, up, "parentA", `{}`), http.StatusForbidden, "Only admin/coach may update")
	cov3xWant(t, w.do(http.MethodPut, up, "coach", `{bad`), http.StatusBadRequest, "bad_request")
	cov3xWant(t, w.do(http.MethodPut, "/practices/missing", "coach", `{"notes":"n"}`), http.StatusNotFound, "Practice not found")
	cov3xWant(t, w.do(http.MethodPut, up, "coach", `{"date":"soon"}`), http.StatusBadRequest, "YYYY-MM-DD")
	cov3xWant(t, w.do(http.MethodPut, up, "coach", `{"type":"swim"}`), http.StatusBadRequest, "Invalid practice type")
	cov3xWant(t, w.do(http.MethodPut, up, "coach", `{"yardage_target":-5}`), http.StatusBadRequest, "between 0 and 200000")
	rr = w.do(http.MethodPut, up, "admin",
		`{"date":"2026-03-09","group_id":"AGE","type":"dryland","description":"Core","yardage_target":0,"attendance":9,"strokes":{"free":100},"notes":"updated"}`)
	cov3xWant(t, rr, http.StatusOK, "")
	p = cov3xJSON(t, rr)["practice"].(map[string]any)
	if p["date"] != "2026-03-09" || p["group_id"] != "AGE" || p["type"] != "dryland" || p["description"] != "Core" ||
		p["notes"] != "updated" || p["strokes"].(map[string]any)["free"] != float64(100) {
		t.Fatalf("update not applied: %v", p)
	}
	// An empty body leaves the practice unchanged.
	rr = w.do(http.MethodPut, up, "coach", ``)
	cov3xWant(t, rr, http.StatusOK, "updated")

	// delete
	cov3xWant(t, w.do(http.MethodDelete, up, "parentA", ""), http.StatusForbidden, "Only admin/coach may delete")
	cov3xWant(t, w.do(http.MethodDelete, "/practices/missing", "coach", ""), http.StatusNotFound, "Practice not found")
	cov3xWant(t, w.do(http.MethodDelete, up, "coach", ""), http.StatusOK, `"deleted":true`)
	cov3xWant(t, w.do(http.MethodGet, up, "admin", ""), http.StatusNotFound, "not_found")
	for _, a := range []string{"practice.create", "practice.update", "practice.delete"} {
		if got, _ := w.audit.Query(audit.Filter{OrgID: cov3xOrg, Action: a}); len(got) == 0 {
			t.Errorf("missing audit action %s", a)
		}
	}
}

func TestCov3xPracticeVisibilityByHouseholdGroups(t *testing.T) {
	w := newCov3xTrainWorld(t)
	team := w.create(t, "", "team secret")
	senior := w.create(t, w.senior.ID, "senior secret")
	age := w.create(t, "age", "age secret") // group referenced by lower-case code

	ids := func(token string) map[string]bool {
		rr := w.do(http.MethodGet, "/practices", token, "")
		cov3xWant(t, rr, http.StatusOK, "")
		out := map[string]bool{}
		for _, raw := range cov3xJSON(t, rr)["practices"].([]any) {
			p := raw.(map[string]any)
			out[p["id"].(string)] = true
			if token == "parentA" || token == "parentB" {
				if _, has := p["notes"]; has {
					t.Fatalf("notes must be redacted for %s: %v", token, p)
				}
			}
		}
		return out
	}
	cov3xWant(t, w.do(http.MethodGet, "/practices", "stranger", ""), http.StatusForbidden, "Insufficient role")
	if got := ids("coach"); len(got) != 3 {
		t.Fatalf("staff see everything: %v", got)
	}
	// Ada is in both groups; Bo only in AGE.
	if got := ids("parentA"); !got[team] || !got[senior] || !got[age] {
		t.Fatalf("parent A should see all three: %v", got)
	}
	if got := ids("parentB"); !got[team] || got[senior] || !got[age] {
		t.Fatalf("parent B must not see SENIOR practice: %v", got)
	}

	cov3xWant(t, w.do(http.MethodGet, "/practices/"+senior, "stranger", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, w.do(http.MethodGet, "/practices/"+senior, "parentB", ""), http.StatusNotFound, "not_found")
	cov3xWant(t, w.do(http.MethodGet, "/practices/missing", "parentB", ""), http.StatusNotFound, "not_found")
	rr := w.do(http.MethodGet, "/practices/"+senior, "parentA", "")
	cov3xWant(t, rr, http.StatusOK, "")
	if p := cov3xJSON(t, rr)["practice"].(map[string]any); p["id"] != senior {
		t.Fatalf("unexpected practice: %v", p)
	} else if _, has := p["notes"]; has {
		t.Fatalf("parent must not see notes: %v", p)
	}
	rr = w.do(http.MethodGet, "/practices/"+senior, "coach", "")
	if p := cov3xJSON(t, rr)["practice"].(map[string]any); p["notes"] != "senior secret" {
		t.Fatalf("coach must see notes: %v", p)
	}
}

func TestCov3xAttendanceRosterAndMarking(t *testing.T) {
	w := newCov3xTrainWorld(t)
	team := w.create(t, "", "")
	byID := w.create(t, w.senior.ID, "")
	byCode := w.create(t, "SENIOR", "")
	ghostGroup := w.create(t, "NOSUCH", "")

	roster := func(practice string) []map[string]any {
		rr := w.do(http.MethodGet, "/practices/"+practice+"/attendance", "admin", "")
		cov3xWant(t, rr, http.StatusOK, "")
		var out []map[string]any
		for _, raw := range cov3xJSON(t, rr)["roster"].([]any) {
			out = append(out, raw.(map[string]any))
		}
		return out
	}

	cov3xWant(t, w.do(http.MethodGet, "/practices/"+team+"/attendance", "parentA", ""), http.StatusForbidden, "Only admin/coach may view attendance")
	cov3xWant(t, w.do(http.MethodGet, "/practices/missing/attendance", "coach", ""), http.StatusNotFound, "not_found")

	// Team-wide: both swimmers, Ada listed once despite two group links; the
	// preferred name wins over the legal first name.
	r := roster(team)
	if len(r) != 2 {
		t.Fatalf("team practice roster: %v", r)
	}
	names := map[string]string{}
	for _, e := range r {
		names[e["swimmer_id"].(string)] = e["swimmer_name"].(string)
	}
	if names[w.swA] != "Addie" || names[w.swB] != "Bo Beta" {
		t.Fatalf("unexpected names: %v", names)
	}
	// A group referenced by id or by code resolves to the same roster.
	for _, p := range []string{byID, byCode} {
		if r := roster(p); len(r) != 1 || r[0]["swimmer_id"] != w.swA {
			t.Fatalf("SENIOR roster should be Ada only: %v", r)
		}
	}
	if r := roster(ghostGroup); len(r) != 0 {
		t.Fatalf("unknown group has no roster: %v", r)
	}

	// Mark attendance.
	mark := func(token, practice, swimmer, body string) *httptest.ResponseRecorder {
		return w.do(http.MethodPut, "/practices/"+practice+"/attendance/"+swimmer, token, body)
	}
	cov3xWant(t, mark("parentA", byID, w.swA, `{"status":"present"}`), http.StatusForbidden, "Only admin/coach may mark attendance")
	cov3xWant(t, mark("coach", byID, w.swA, `{"status":"late"}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, mark("coach", byID, w.swA, `{bad`), http.StatusBadRequest, "bad_request")
	cov3xWant(t, mark("coach", byID, "ghost", `{"status":"present"}`), http.StatusNotFound, "swimmer_not_found")
	cov3xWant(t, mark("coach", "missing", w.swA, `{"status":"present"}`), http.StatusNotFound, "Practice not found")
	rr := mark("coach", byID, w.swA, `{"status":"present"}`)
	cov3xWant(t, rr, http.StatusOK, "")
	rec := cov3xJSON(t, rr)["attendance"].(map[string]any)
	if rec["status"] != "present" || rec["swimmer_id"] != w.swA || rec["marked_by"] != cov3xCoachID {
		t.Fatalf("unexpected attendance record: %v", rec)
	}
	// Re-marking updates the same row.
	cov3xWant(t, mark("coach", byID, w.swA, `{"status":"absent"}`), http.StatusOK, "absent")

	r = roster(byID)
	if len(r) != 1 || r[0]["status"] != "absent" || r[0]["marked_by"] != cov3xCoachID || r[0]["marked_at"] == "" {
		t.Fatalf("roster should carry the mark: %v", r)
	}
	// Unmarked swimmers carry no status.
	for _, e := range roster(team) {
		if e["swimmer_id"] == w.swB {
			if _, has := e["status"]; has {
				t.Fatalf("unmarked swimmer must have no status: %v", e)
			}
		}
	}
	if got, _ := w.audit.Query(audit.Filter{OrgID: cov3xOrg, Action: "practice.attendance.update"}); len(got) != 2 {
		t.Fatalf("each mark must be audited, got %d", len(got))
	}
}

func TestCov3xTrainingDatabaseOutageReturns500(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := training.NewServiceWithPool(pool)
	h := NewTrainingHandlers(svc, audit.NewService())
	r := cov3xRouter(h.RegisterRoutes)
	pool.Close()

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/practices", ""},
		{http.MethodPost, "/practices", `{"date":"2026-03-02","type":"water"}`},
		{http.MethodPut, "/practices/p1", `{"notes":"n"}`},
		{http.MethodDelete, "/practices/p1", ""},
		{http.MethodGet, "/practices/p1/attendance", ""},
		{http.MethodPut, "/practices/p1/attendance/" + cov3xSw1, `{"status":"present"}`},
	}
	for _, tc := range cases {
		rr := cov3xDo(r, tc.method, tc.path, "admin", tc.body)
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "internal_error") {
			t.Errorf("%s %s: want 500 internal_error, got %d %s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), "closed pool") {
			t.Errorf("%s %s leaked driver text: %s", tc.method, tc.path, rr.Body.String())
		}
	}
}

func TestCov3xPracticeListScopeFailureReturns500(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	down := members.NewServiceWithPool(pool)
	pool.Close()
	h := NewTrainingHandlersWithMembers(training.NewService(), down, groups.NewService(), audit.NewService())
	r := cov3xRouter(h.RegisterRoutes)
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/practices", "parent", ""), http.StatusInternalServerError, "Failed to resolve practice scope")
	// Staff never need the roster lookup, so they are unaffected.
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/practices", "coach", ""), http.StatusOK, "practices")
}

func TestCov3xAttendanceRosterDependencyFailures(t *testing.T) {
	setup := func(t *testing.T) (trainingPool, membersPool interface{ Close() }, run func(hook func()) *httptest.ResponseRecorder, hookable *members.Service) {
		t.Helper()
		tp := testutil.IsolatedSchemaPool(t)
		cov3xInsertUser(t, tp, cov3xOrg, cov3xAdminID)
		mp := testutil.IsolatedSchemaPool(t)
		tsvc := training.NewServiceWithPool(tp)
		practice, err := tsvc.Create(cov3xOrg, "2026-03-02", "", "water", "", 0, nil, training.PracticeStrokes{}, "", cov3xAdminID)
		if err != nil {
			t.Fatal(err)
		}
		msvc := members.NewServiceWithPool(mp)
		h := NewTrainingHandlersWithMembers(tsvc, msvc, nil, audit.NewService())
		r := cov3xRouter(h.RegisterRoutes)
		run = func(hook func()) *httptest.ResponseRecorder {
			if hook != nil {
				msvc.SetSyncHookForTest(hook)
			}
			return cov3xDo(r, http.MethodGet, "/practices/"+practice.ID+"/attendance", "admin", "")
		}
		return tp, mp, run, msvc
	}

	t.Run("practice lookup fails", func(t *testing.T) {
		tp, _, run, _ := setup(t)
		tp.Close()
		cov3xWant(t, run(nil), http.StatusInternalServerError, "Failed to get practice")
	})
	t.Run("group links fail to load", func(t *testing.T) {
		_, mp, run, _ := setup(t)
		mp.Close()
		cov3xWant(t, run(nil), http.StatusInternalServerError, "Failed to load group roster")
	})
	t.Run("swimmers fail to load", func(t *testing.T) {
		_, mp, run, _ := setup(t)
		var calls atomic.Int32
		cov3xWant(t, run(func() {
			if calls.Add(1) == 1 {
				mp.Close() // the links read succeeded; the swimmers read cannot
			}
		}), http.StatusInternalServerError, "Failed to load swimmers")
	})
	t.Run("attendance marks fail to load", func(t *testing.T) {
		tp, _, run, _ := setup(t)
		var calls atomic.Int32
		cov3xWant(t, run(func() {
			if calls.Add(1) == 2 {
				tp.Close() // roster is complete; only the marks lookup is left
			}
		}), http.StatusInternalServerError, "Failed to load attendance")
	})
}

func TestCov3xAttendanceRosterSkipsSwimmerRemovedMidRequest(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	cov3xInsertUser(t, pool, cov3xOrg, cov3xAdminID)
	admin := &appmiddleware.Principal{UserID: cov3xAdminID, OrgID: cov3xOrg, Roles: []string{"admin"}}
	msvc := members.NewServiceWithPool(pool)
	hh, err := msvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov3xOrg, HouseholdName: "Gone", PrimaryEmail: "gone@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, name := range []string{"Gia", "Gus"} {
		sw, _, err := msvc.CreateSwimmer(admin, cov3xOrg, members.CreateSwimmerInput{
			HouseholdID: hh.ID, FirstName: name, LastName: "Gone", DateOfBirth: "2012-04-05", Gender: "female", RegistrationStatus: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := msvc.AssignSwimmerToGroup(admin, cov3xOrg, sw.ID, members.AssignSwimmerGroupInput{GroupCode: "SENIOR"}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sw.ID)
	}
	tsvc := training.NewServiceWithPool(pool)
	practice, err := tsvc.Create(cov3xOrg, "2026-03-02", "SENIOR", "water", "", 0, nil, training.PracticeStrokes{}, "", cov3xAdminID)
	if err != nil {
		t.Fatal(err)
	}
	h := NewTrainingHandlersWithMembers(tsvc, msvc, nil, audit.NewService())
	r := cov3xRouter(h.RegisterRoutes)

	// Another instance deletes Gus after the group links were read but before
	// the swimmer list is.
	var calls atomic.Int32
	msvc.SetSyncHookForTest(func() {
		if calls.Add(1) == 1 {
			if _, err := pool.Exec(context.Background(), `DELETE FROM swimmers WHERE id = $1::uuid`, ids[1]); err != nil {
				t.Errorf("delete swimmer: %v", err)
			}
		}
	})
	rr := cov3xDo(r, http.MethodGet, "/practices/"+practice.ID+"/attendance", "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	roster := cov3xJSON(t, rr)["roster"].([]any)
	if len(roster) != 1 || roster[0].(map[string]any)["swimmer_id"] != ids[0] {
		t.Fatalf("the removed swimmer must be skipped, got %v", roster)
	}
}
