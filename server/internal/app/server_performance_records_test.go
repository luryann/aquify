package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

// TestPerformanceRecordsGoalsRelaysRoutes exercises the team records, top
// times, goals, relay builder and progress report routes end to end with
// allow and deny cases for every role.
func TestPerformanceRecordsGoalsRelaysRoutes(t *testing.T) {
	t.Parallel()
	const org = "org-records"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	membersSvc := members.NewService()
	meetsSvc := meets.NewService()
	perfSvc := performance.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, membersSvc, nil, meetsSvc, nil, perfSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	admin := issueSessionTokenForUser(t, router, "admin-user", org, []string{"admin"}, true)
	coach := issueSessionTokenForUser(t, router, "coach-user", org, []string{"coach"}, false)

	do := func(token, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("X-Org-Id", org)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	expect := func(rr *httptest.ResponseRecorder, code int, what string) {
		t.Helper()
		if rr.Code != code {
			t.Fatalf("%s: want %d, got %d body=%s", what, code, rr.Code, rr.Body.String())
		}
	}
	decode := func(rr *httptest.ResponseRecorder, dst any) {
		t.Helper()
		if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
			t.Fatalf("decode: %v (%s)", err, rr.Body.String())
		}
	}

	var hh struct {
		Household struct{ ID string } `json:"household"`
	}
	decode(do(admin, http.MethodPost, "/api/v1/members/households", map[string]any{"org_id": org, "household_name": "Ames"}), &hh)
	var hh2 struct {
		Household struct{ ID string } `json:"household"`
	}
	decode(do(admin, http.MethodPost, "/api/v1/members/households", map[string]any{"org_id": org, "household_name": "Bell"}), &hh2)
	newSwimmer := func(household, first, dob, gender string) string {
		var sw struct {
			Swimmer struct{ ID string } `json:"swimmer"`
		}
		decode(do(admin, http.MethodPost, "/api/v1/members/swimmers", map[string]any{
			"org_id": org, "household_id": household, "first_name": first, "last_name": "Test", "date_of_birth": dob, "gender": gender,
		}), &sw)
		if sw.Swimmer.ID == "" {
			t.Fatalf("create swimmer %s failed", first)
		}
		return sw.Swimmer.ID
	}
	amy := newSwimmer(hh.Household.ID, "Amy", "2014-06-01", "female")
	bea := newSwimmer(hh2.Household.ID, "Bea", "2014-02-01", "female")
	parent := issueSessionTokenWithHousehold(t, router, "parent-user", org, hh.Household.ID, []string{"parent"}, false)
	swimmerTok := issueSessionTokenWithHousehold(t, router, "swimmer-user", org, hh.Household.ID, []string{"swimmer"}, false)
	meetID := createMeet(t, router, org, admin, "Records Meet")

	// Results flag new team records.
	postResult := func(swimmerID string, ms int) []map[string]any {
		var env struct {
			NewTeamRecords []map[string]any `json:"new_team_records"`
		}
		rr := do(coach, http.MethodPost, "/api/v1/results", map[string]any{
			"swimmer_id": swimmerID, "meet_id": meetID, "distance": 50, "stroke": "free", "course": "SCY", "gender": "F", "fin_time_ms": ms, "is_official": true,
		})
		expect(rr, http.StatusCreated, "post result")
		decode(rr, &env)
		return env.NewTeamRecords
	}
	if recs := postResult(amy, 31000); len(recs) != 2 {
		t.Fatalf("first swim should set age-group + Open records, got %+v", recs)
	}
	if recs := postResult(bea, 30500); len(recs) != 2 || recs[0]["previous_holder"] != "Amy Test" {
		t.Fatalf("Bea should break both records held by Amy, got %+v", recs)
	}
	for _, tok := range []string{parent, swimmerTok, coach} {
		rr := do(tok, http.MethodGet, "/api/v1/performance/records?course=SCY", nil)
		expect(rr, http.StatusOK, "read records")
		var env struct {
			Records []struct {
				Record struct {
					SwimmerName string `json:"swimmer_name"`
				} `json:"record"`
				Previous *struct {
					SwimmerName string `json:"swimmer_name"`
				} `json:"previous"`
			} `json:"records"`
		}
		decode(rr, &env)
		if len(env.Records) != 2 || env.Records[0].Record.SwimmerName != "Bea Test" || env.Records[0].Previous == nil {
			t.Fatalf("record board: %s", rr.Body.String())
		}
	}
	expect(do(parent, http.MethodGet, "/api/v1/performance/records/history?course=SCY&gender=F&age_group=11-12&distance=50&stroke=free", nil), http.StatusOK, "parent history")
	expect(do(parent, http.MethodGet, "/api/v1/performance/records/history?course=SCY&gender=F&age_group=nope&distance=50&stroke=free", nil), http.StatusUnprocessableEntity, "bad history key")
	expect(do(parent, http.MethodGet, "/api/v1/performance/records/pending", nil), http.StatusForbidden, "parent pending")
	expect(do(coach, http.MethodGet, "/api/v1/performance/records/pending", nil), http.StatusOK, "coach pending")

	manual := map[string]any{"course": "SCY", "gender": "F", "age_group": "OPEN", "distance": 50, "stroke": "free", "time_ms": 29000, "swimmer_name": "Hall of Famer", "swim_date": "2001-03-10", "locked": true}
	expect(do(coach, http.MethodPost, "/api/v1/performance/records", manual), http.StatusForbidden, "coach manual record")
	expect(do(parent, http.MethodPost, "/api/v1/performance/records", manual), http.StatusForbidden, "parent manual record")
	rr := do(admin, http.MethodPost, "/api/v1/performance/records", manual)
	expect(rr, http.StatusCreated, "admin manual record")
	var created struct {
		Entry struct{ ID string } `json:"entry"`
	}
	decode(rr, &created)
	expect(do(parent, http.MethodPatch, "/api/v1/performance/records/"+created.Entry.ID, map[string]any{"verified": true}), http.StatusForbidden, "parent verify")
	expect(do(admin, http.MethodPatch, "/api/v1/performance/records/"+created.Entry.ID, map[string]any{"verified": true}), http.StatusOK, "admin verify")
	expect(do(admin, http.MethodPatch, "/api/v1/performance/records/"+created.Entry.ID, map[string]any{"bogus": 1}), http.StatusBadRequest, "unknown field")

	// Faster swim against a locked record is pending.
	recs := postResult(amy, 28500)
	var pendingID string
	for _, r := range recs {
		if r["age_group"] == "OPEN" {
			if r["status"] != "pending" {
				t.Fatalf("open break should be pending: %+v", r)
			}
			pendingID, _ = r["entry_id"].(string)
		}
	}
	expect(do(coach, http.MethodPost, "/api/v1/performance/records/"+pendingID+"/approve", nil), http.StatusForbidden, "coach approve")
	expect(do(admin, http.MethodPost, "/api/v1/performance/records/"+pendingID+"/approve", map[string]any{"notes": "Checked"}), http.StatusOK, "admin approve")
	expect(do(admin, http.MethodPost, "/api/v1/performance/records/"+pendingID+"/approve", nil), http.StatusConflict, "approve twice")
	expect(do(parent, http.MethodPost, "/api/v1/performance/records/"+pendingID+"/announcement", nil), http.StatusForbidden, "parent announcement")
	rr = do(coach, http.MethodPost, "/api/v1/performance/records/"+pendingID+"/announcement", nil)
	expect(rr, http.StatusCreated, "coach announcement draft")
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"status":"draft"`)) {
		t.Fatalf("announcement should be a draft: %s", rr.Body.String())
	}
	expect(do(coach, http.MethodPost, "/api/v1/performance/records/rebuild", nil), http.StatusForbidden, "coach rebuild")
	expect(do(admin, http.MethodPost, "/api/v1/performance/records/rebuild", nil), http.StatusOK, "admin rebuild")
	expect(do(admin, http.MethodDelete, "/api/v1/performance/records/"+created.Entry.ID, nil), http.StatusOK, "delete manual")
	expect(do(admin, http.MethodDelete, "/api/v1/performance/records/"+pendingID, nil), http.StatusUnprocessableEntity, "delete result entry")

	// Top times: readable by families.
	rr = do(swimmerTok, http.MethodGet, "/api/v1/performance/top-times?course=SCY&gender=F&distance=50&stroke=free&limit=10", nil)
	expect(rr, http.StatusOK, "swimmer top times")
	var top struct {
		Lists []struct {
			Entries []struct {
				SwimmerID string `json:"swimmer_id"`
				TimeMs    int    `json:"time_ms"`
			} `json:"entries"`
		} `json:"lists"`
	}
	decode(rr, &top)
	if len(top.Lists) != 1 || len(top.Lists[0].Entries) != 2 || top.Lists[0].Entries[0].TimeMs != 28500 {
		t.Fatalf("top times should list one best swim per swimmer: %s", rr.Body.String())
	}
	expect(do(parent, http.MethodGet, "/api/v1/performance/top-times?from=yesterday", nil), http.StatusUnprocessableEntity, "bad top-times date")

	// Goals: own household only for families; coach anyone.
	goal := map[string]any{"distance": 50, "stroke": "free", "course": "SCY", "target_time_ms": 28000, "deadline": "2027-03-01"}
	rr = do(parent, http.MethodPost, "/api/v1/swimmers/"+amy+"/goals", goal)
	expect(rr, http.StatusCreated, "parent own goal")
	var g struct {
		Goal struct {
			ID         string `json:"id"`
			BestTimeMs *int   `json:"best_time_ms"`
		} `json:"goal"`
	}
	decode(rr, &g)
	if g.Goal.BestTimeMs == nil || *g.Goal.BestTimeMs != 28500 {
		t.Fatalf("goal should carry current best: %s", rr.Body.String())
	}
	expect(do(parent, http.MethodPost, "/api/v1/swimmers/"+bea+"/goals", goal), http.StatusForbidden, "parent foreign goal")
	expect(do(swimmerTok, http.MethodPost, "/api/v1/swimmers/"+amy+"/goals", goal), http.StatusCreated, "swimmer own goal")
	expect(do(coach, http.MethodPost, "/api/v1/swimmers/"+bea+"/goals", goal), http.StatusCreated, "coach any goal")
	expect(do(parent, http.MethodGet, "/api/v1/swimmers/"+bea+"/goals", nil), http.StatusForbidden, "parent read foreign goals")
	expect(do(parent, http.MethodPatch, "/api/v1/swimmers/"+amy+"/goals/"+g.Goal.ID, map[string]any{"target_time_ms": 27900}), http.StatusOK, "parent update goal")
	expect(do(parent, http.MethodPost, "/api/v1/swimmers/"+amy+"/goals", map[string]any{"distance": 50, "stroke": "free", "course": "SCY", "target_time_ms": 1, "deadline": "nope"}), http.StatusUnprocessableEntity, "bad deadline")
	rr = do(parent, http.MethodGet, "/api/v1/performance/goals", nil)
	expect(rr, http.StatusOK, "parent org goals")
	var orgGoals struct {
		Goals []struct {
			SwimmerID string `json:"swimmer_id"`
		} `json:"goals"`
	}
	decode(rr, &orgGoals)
	for _, og := range orgGoals.Goals {
		if og.SwimmerID != amy {
			t.Fatalf("parent org goals leaked a foreign swimmer: %s", rr.Body.String())
		}
	}
	expect(do(parent, http.MethodDelete, "/api/v1/swimmers/"+amy+"/goals/"+g.Goal.ID, nil), http.StatusOK, "parent delete goal")

	// Relay builder + lineups: staff only.
	relay := map[string]any{"meet_id": meetID, "gender": "F", "age_group": "OPEN", "relay_type": "free", "distance": 200, "teams": 1}
	expect(do(parent, http.MethodPost, "/api/v1/performance/relays/optimize", relay), http.StatusForbidden, "parent relay builder")
	rr = do(coach, http.MethodPost, "/api/v1/performance/relays/optimize", relay)
	expect(rr, http.StatusOK, "coach relay builder")
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"warnings":["Not enough`)) {
		t.Fatalf("two swimmers cannot fill a relay: %s", rr.Body.String())
	}
	expect(do(coach, http.MethodPost, "/api/v1/performance/relays/optimize", map[string]any{"gender": "F", "relay_type": "medley", "distance": 800, "course": "SCY"}), http.StatusUnprocessableEntity, "medley 800")
	lineup := map[string]any{"meet_id": meetID, "course": "SCY", "gender": "F", "age_group": "OPEN", "relay_type": "free", "distance": 200, "teams": []map[string]any{{
		"label": "A", "legs": []map[string]any{
			{"leg": 1, "swimmer_id": amy, "time_ms": 28500}, {"leg": 2, "swimmer_id": bea, "time_ms": 30500},
			{"leg": 3, "swimmer_id": amy, "time_ms": 1}, {"leg": 4, "swimmer_id": bea, "time_ms": 1},
		},
	}}}
	expect(do(coach, http.MethodPost, "/api/v1/performance/relay-lineups", lineup), http.StatusUnprocessableEntity, "duplicate swimmer lineup")
	expect(do(parent, http.MethodPost, "/api/v1/performance/relay-lineups", lineup), http.StatusForbidden, "parent lineup")
	expect(do(parent, http.MethodGet, "/api/v1/performance/relay-lineups?meet_id="+meetID, nil), http.StatusForbidden, "parent list lineups")
	expect(do(coach, http.MethodGet, "/api/v1/performance/relay-lineups?meet_id="+meetID, nil), http.StatusOK, "coach list lineups")

	// Progress report: families see only their household.
	rr = do(parent, http.MethodGet, "/api/v1/performance/progress-report?from_a=2000-01-01&to_a=2000-12-31&from_b=2001-01-01&to_b=2099-12-31", nil)
	expect(rr, http.StatusOK, "parent progress report")
	if bytes.Contains(rr.Body.Bytes(), []byte(bea)) {
		t.Fatalf("parent progress report leaked a foreign swimmer: %s", rr.Body.String())
	}
	expect(do(coach, http.MethodGet, "/api/v1/performance/progress-report?from_a=2000-01-01", nil), http.StatusUnprocessableEntity, "incomplete ranges")
}

// TestResultsImportReportsNewTeamRecords checks that the results-file import
// response includes the team records the file broke.
func TestResultsImportReportsNewTeamRecords(t *testing.T) {
	t.Parallel()
	const org = "org-import-records"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, members.NewService(), nil, meets.NewService(), nil, performance.NewService(), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	admin := issueSessionTokenForUser(t, router, "admin-user", org, []string{"admin"}, true)
	post := func(path string, body map[string]any, dst any) {
		req := httptest.NewRequest(http.MethodPost, path, encodeJSONBody(t, body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", org)
		req.Header.Set("Authorization", "Bearer "+admin)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		_ = json.Unmarshal(rr.Body.Bytes(), dst)
	}
	var hh struct {
		Household struct{ ID string } `json:"household"`
	}
	post("/api/v1/members/households", map[string]any{"org_id": org, "household_name": "Smith"}, &hh)
	var sw struct {
		Swimmer struct{ ID string } `json:"swimmer"`
	}
	post("/api/v1/members/swimmers", map[string]any{"org_id": org, "household_id": hh.Household.ID, "first_name": "John", "last_name": "Smith", "date_of_birth": "1999-02-15", "gender": "male"}, &sw)
	meetID := createMeet(t, router, org, admin, "Import Records Meet")
	fixture, err := os.ReadFile("../../../internal/meetfiles/testdata/fixtures/valid.sd3")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "results.sd3")
	_, _ = fw.Write(fixture)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/results/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Org-Id", org)
	req.Header.Set("Authorization", "Bearer "+admin)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rr.Code, rr.Body.String())
	}
	var env struct {
		NewTeamRecords []struct {
			AgeGroup    string `json:"age_group"`
			SwimmerName string `json:"swimmer_name"`
			TimeMs      int    `json:"time_ms"`
		} `json:"new_team_records"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &env)
	if len(env.NewTeamRecords) == 0 || env.NewTeamRecords[len(env.NewTeamRecords)-1].AgeGroup != "OPEN" || env.NewTeamRecords[0].TimeMs != 58120 {
		t.Fatalf("expected an Open team record from the import: %s", rr.Body.String())
	}
}
