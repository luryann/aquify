package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/performance"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov3xPerfWorld wires real in-memory members, groups, meets, documents and
// performance services around two households (A: Ada, B: Bo).
type cov3xPerfWorld struct {
	members *members.Service
	groups  *groups.Service
	meets   *meets.Service
	docs    *documents.Service
	perf    *performance.Service
	audit   *audit.Service
	h       *PerformanceHandlers
	r       http.Handler
	admin   *appmiddleware.Principal
	hhA     string
	hhB     string
	swA     string
	swB     string
}

func newCov3xPerfWorld(t *testing.T) *cov3xPerfWorld {
	t.Helper()
	w := &cov3xPerfWorld{
		members: members.NewService(),
		groups:  groups.NewService(),
		meets:   meets.NewService(),
		docs:    documents.NewService(),
		perf:    performance.NewService(),
		audit:   audit.NewService(),
		admin:   &appmiddleware.Principal{UserID: cov3xAdminID, OrgID: cov3xOrg, Roles: []string{"admin"}},
	}
	mk := func(name, first string) (string, string) {
		hh, err := w.members.CreateHousehold(w.admin, members.CreateHouseholdInput{OrgID: cov3xOrg, HouseholdName: name, PrimaryEmail: strings.ToLower(first) + "@example.test"})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := w.members.CreateSwimmer(w.admin, cov3xOrg, members.CreateSwimmerInput{
			HouseholdID: hh.ID, FirstName: first, LastName: name, DateOfBirth: "2012-04-05", Gender: "female", RegistrationStatus: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		w.groups.SeedSwimmer(cov3xOrg, sw.ID, hh.ID, sw.DateOfBirth, sw.Gender, sw.RegistrationStatus)
		w.meets.SeedSwimmer(cov3xOrg, sw.ID, hh.ID)
		return hh.ID, sw.ID
	}
	w.hhA, w.swA = mk("Alpha", "Ada")
	w.hhB, w.swB = mk("Beta", "Bo")
	w.perf.SetHouseholdChecker(w.members)
	w.h = NewPerformanceHandlersWithAuditAndContext(w.perf, w.members, w.groups, w.meets, w.docs, w.audit)
	w.r = cov3xRouterWith(w.tokens(), w.h.RegisterRoutes)
	return w
}

func (w *cov3xPerfWorld) tokens() tokenResolver {
	tk := cov3xTokens()
	tk["parentA"] = &appmiddleware.Principal{UserID: cov3xParentID, OrgID: cov3xOrg, HouseholdID: w.hhA, Roles: []string{"parent"}}
	tk["parentB"] = &appmiddleware.Principal{UserID: cov3xParent2ID, OrgID: cov3xOrg, HouseholdID: w.hhB, Roles: []string{"parent"}}
	tk["swimA"] = &appmiddleware.Principal{UserID: cov3xSwimmerUID, OrgID: cov3xOrg, HouseholdID: w.hhA, SwimmerID: w.swA, Roles: []string{"swimmer"}}
	return tk
}

func (w *cov3xPerfWorld) do(method, path, token, body string) *httptest.ResponseRecorder {
	return cov3xDo(w.r, method, path, token, body)
}

func (w *cov3xPerfWorld) postResult(t *testing.T, swimmer, stroke string, dist int, fin string, extra string) map[string]any {
	t.Helper()
	body := fmt.Sprintf(`{"swimmer_id":%q,"distance":%d,"stroke":%q,"course":"SCY","gender":"F"%s%s}`, swimmer, dist, stroke, fin, extra)
	rr := w.do(http.MethodPost, "/results", "coach", body)
	cov3xWant(t, rr, http.StatusCreated, "result")
	return cov3xJSON(t, rr)
}

func TestCov3xPerformanceScopeGuards(t *testing.T) {
	w := newCov3xPerfWorld(t)
	cov3xAssertScopeGuards(t, w.h.RegisterRoutes, []cov3xRoute{
		{http.MethodPost, "/results", `{}`},
		{http.MethodGet, "/performance/best-times", ""},
		{http.MethodGet, "/performance/progression", ""},
		{http.MethodGet, "/swimmers/x/best-times", ""},
		{http.MethodPost, "/swimmers/x/best-times/override", `{}`},
		{http.MethodGet, "/swimmers/x/progression", ""},
		{http.MethodGet, "/swimmers/x/activity", ""},
		{http.MethodGet, "/swimmers/x/cut-qualification", ""},
		{http.MethodGet, "/time-standards", ""},
		{http.MethodPost, "/time-standards", `{}`},
		{http.MethodGet, "/time-standards/x", ""},
		{http.MethodPut, "/time-standards/x", `{}`},
		{http.MethodDelete, "/time-standards/x", ""},
	})
}

func TestCov3xPerformanceConstructors(t *testing.T) {
	svc := performance.NewService()
	plain := NewPerformanceHandlers(svc)
	if plain.service != svc || plain.auditService != nil || plain.validate == nil || plain.membersService != nil {
		t.Fatalf("unexpected plain handler wiring: %+v", plain)
	}
	r := cov3xRouter(plain.RegisterRoutes)
	rr := cov3xDo(r, http.MethodGet, "/performance/best-times", "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	if cov3xJSON(t, rr)["count"] != float64(0) {
		t.Fatalf("fresh service should have no best times: %s", rr.Body.String())
	}

	auditSvc := audit.NewService()
	withAudit := NewPerformanceHandlersWithAudit(svc, auditSvc)
	if withAudit.auditService != auditSvc || withAudit.documentsService != nil {
		t.Fatalf("unexpected audited handler wiring: %+v", withAudit)
	}
	r = cov3xRouter(withAudit.RegisterRoutes)
	rr = cov3xDo(r, http.MethodPost, "/swimmers/"+cov3xSw1+"/best-times/override", "admin",
		`{"event_distance":50,"stroke":"free","course":"SCY","best_time_ms":28000,"reason":"paper timing"}`)
	cov3xWant(t, rr, http.StatusOK, "best_time")
	events, _ := auditSvc.Query(audit.Filter{OrgID: cov3xOrg, Action: audit.ActionPerformanceBestTimeOverride})
	if len(events) != 1 {
		t.Fatalf("override must be audited exactly once, got %d", len(events))
	}

	// Activity needs the extra services; the bare constructors refuse politely.
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/swimmers/"+cov3xSw1+"/activity", "admin", ""), http.StatusInternalServerError, "activity_unavailable")
}

func TestCov3xResultsBestTimesAndProgression(t *testing.T) {
	w := newCov3xPerfWorld(t)

	// POST /results
	cov3xWant(t, w.do(http.MethodPost, "/results", "parentA", `{}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, w.do(http.MethodPost, "/results", "coach", `{bad`), http.StatusBadRequest, "invalid_request_body")
	forbidden := fmt.Sprintf(`{"swimmer_id":%q,"distance":100,"stroke":"free","course":"SCY","gender":"F","fin_time_ms":61500,"is_official":true}`, w.swA)
	cov3xWant(t, w.do(http.MethodPost, "/results", "parentA", forbidden), http.StatusForbidden, "Insufficient role")

	first := w.postResult(t, w.swA, "free", 100, `,"fin_time_ms":61500,"is_official":true`, `,"splits":[{"split_number":1,"split_distance":50,"split_time_ms":29900,"cumulative_time_ms":29900}]`)
	if first["new_team_records"] == nil {
		t.Fatalf("new_team_records must always be present: %v", first)
	}
	w.postResult(t, w.swA, "free", 100, `,"fin_time_ms":59900,"is_official":true`, "")
	w.postResult(t, w.swA, "back", 50, `,"fin_time_ms":33100,"is_official":true`, "")
	w.postResult(t, w.swB, "free", 100, `,"fin_time_ms":70000,"is_official":true`, "")

	// best times: per swimmer
	cov3xWant(t, w.do(http.MethodGet, "/swimmers/"+w.swA+"/best-times", "stranger", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, w.do(http.MethodGet, "/swimmers/"+w.swA+"/best-times", "parentB", ""), http.StatusForbidden, "Insufficient role")
	rr := w.do(http.MethodGet, "/swimmers/"+w.swA+"/best-times?course=SCY&stroke=free", "parentA", "")
	cov3xWant(t, rr, http.StatusOK, "")
	bts := cov3xJSON(t, rr)
	list := bts["best_times"].([]any)
	if bts["count"] != float64(1) || bts["swimmer_id"] != w.swA || list[0].(map[string]any)["best_time_ms"] != float64(59900) {
		t.Fatalf("best time should be the faster swim: %v", bts)
	}

	// best times: org-wide, household scoped for parents
	cov3xWant(t, w.do(http.MethodGet, "/performance/best-times", "stranger", ""), http.StatusForbidden, "Insufficient role")
	rr = w.do(http.MethodGet, "/performance/best-times", "coach", "")
	if cov3xJSON(t, rr)["count"] != float64(3) {
		t.Fatalf("staff see every swimmer: %s", rr.Body.String())
	}
	rr = w.do(http.MethodGet, "/performance/best-times", "parentB", "")
	if got := cov3xJSON(t, rr); got["count"] != float64(1) {
		t.Fatalf("parent B must only see household B: %v", got)
	}
	rr = w.do(http.MethodGet, "/performance/best-times?swimmer_id="+w.swA+"&stroke=back&course=SCY", "admin", "")
	if got := cov3xJSON(t, rr); got["count"] != float64(1) {
		t.Fatalf("filters ignored: %v", got)
	}

	// override
	ob := `{"event_distance":100,"stroke":"free","course":"SCY","best_time_ms":58000,"reason":"hand timed","recorded_at":"2026-01-02"}`
	op := "/swimmers/" + w.swA + "/best-times/override"
	cov3xWant(t, w.do(http.MethodPost, op, "coach", ob), http.StatusForbidden, "Only admins")
	cov3xWant(t, w.do(http.MethodPost, op, "admin", `{}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, w.do(http.MethodPost, op, "admin", `{"event_distance":100,"stroke":"free","course":"SCY","reason":"   "}`), http.StatusUnprocessableEntity, "override_requires_reason")
	rr = w.do(http.MethodPost, op, "admin", ob)
	cov3xWant(t, rr, http.StatusOK, "audit_event")
	out := cov3xJSON(t, rr)
	if bt := out["best_time"].(map[string]any); bt["best_time_ms"] != float64(58000) || bt["is_manual_override"] != true || bt["override_reason"] != "hand timed" {
		t.Fatalf("override not applied: %v", bt)
	}
	if ev := out["audit_event"].(map[string]any); ev["action"] != "best_time.manual_override" {
		t.Fatalf("unexpected audit event: %v", ev)
	}
	if got, _ := w.audit.Query(audit.Filter{OrgID: cov3xOrg, Action: audit.ActionPerformanceBestTimeOverride}); len(got) != 1 {
		t.Fatalf("durable audit row missing, got %d", len(got))
	}

	// progression
	pp := "/swimmers/" + w.swA + "/progression"
	cov3xWant(t, w.do(http.MethodGet, pp, "stranger", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, w.do(http.MethodGet, pp+"?distance=abc", "admin", ""), http.StatusBadRequest, "invalid_query_parameter")
	rr = w.do(http.MethodGet, pp+"?distance=100&stroke=free&course=SCY", "parentA", "")
	if got := cov3xJSON(t, rr); got["count"] != float64(2) || got["swimmer_id"] != w.swA {
		t.Fatalf("want 2 free 100 results: %v", got)
	}
	cov3xWant(t, w.do(http.MethodGet, pp, "parentB", ""), http.StatusForbidden, "Insufficient role")

	// org-wide progression
	cov3xWant(t, w.do(http.MethodGet, "/performance/progression", "stranger", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, w.do(http.MethodGet, "/performance/progression?distance=-1", "admin", ""), http.StatusBadRequest, "invalid_query_parameter")
	rr = w.do(http.MethodGet, "/performance/progression", "coach", "")
	if cov3xJSON(t, rr)["count"] != float64(4) {
		t.Fatalf("staff see every result: %s", rr.Body.String())
	}
	rr = w.do(http.MethodGet, "/performance/progression?swimmer_id="+w.swB+"&distance=100&stroke=free&course=SCY", "parentB", "")
	if got := cov3xJSON(t, rr); got["count"] != float64(1) {
		t.Fatalf("filtered org progression: %v", got)
	}
}

func TestCov3xTimeStandardsAndCutQualification(t *testing.T) {
	w := newCov3xPerfWorld(t)
	std := func(extra string) string {
		return `{"name":"AAA 11-12","season_year":2026,"standard_code":"AAA","event_distance":100,"stroke":"free","course":"SCY","gender":"F","time_ms":60000,"is_active":true` + extra + `}`
	}

	cov3xWant(t, w.do(http.MethodPost, "/time-standards", "coach", std("")), http.StatusForbidden, "Only admins")
	cov3xWant(t, w.do(http.MethodPost, "/time-standards", "admin", `{}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, w.do(http.MethodPost, "/time-standards", "admin", std(`,"age_min":14,"age_max":10`)), http.StatusBadRequest, "validation_error")
	rr := w.do(http.MethodPost, "/time-standards", "admin", std(`,"age_min":10,"age_max":12,"description":"state cut"`))
	cov3xWant(t, rr, http.StatusCreated, "time_standard")
	id := cov3xJSON(t, rr)["time_standard"].(map[string]any)["id"].(string)
	// A second, inactive standard from another season.
	rr = w.do(http.MethodPost, "/time-standards", "admin", `{"name":"Old","season_year":2025,"standard_code":"B","event_distance":50,"stroke":"back","course":"SCM","gender":"M","time_ms":40000}`)
	cov3xWant(t, rr, http.StatusCreated, "")

	// list + filters
	count := func(q string) float64 {
		rr := w.do(http.MethodGet, "/time-standards"+q, "parentA", "")
		cov3xWant(t, rr, http.StatusOK, "")
		return cov3xJSON(t, rr)["count"].(float64)
	}
	if count("") != 2 || count("?season_year=2026") != 1 || count("?active_only=true") != 1 ||
		count("?standard_code=B&stroke=back&course=SCM&gender=M&event_distance=50") != 1 || count("?stroke=fly") != 0 {
		t.Fatal("time standard filters misbehave")
	}
	for _, bad := range []string{"?season_year=x", "?event_distance=x", "?active_only=maybe"} {
		cov3xWant(t, w.do(http.MethodGet, "/time-standards"+bad, "admin", ""), http.StatusBadRequest, "invalid_query_parameter")
	}

	// get
	cov3xWant(t, w.do(http.MethodGet, "/time-standards/missing", "admin", ""), http.StatusNotFound, "standard_not_found")
	cov3xWant(t, w.do(http.MethodGet, "/time-standards/"+id, "parentA", ""), http.StatusOK, "AAA 11-12")

	// update
	up := "/time-standards/" + id
	cov3xWant(t, w.do(http.MethodPut, up, "coach", `{"name":"x"}`), http.StatusForbidden, "Only admins")
	cov3xWant(t, w.do(http.MethodPut, up, "admin", `{bad`), http.StatusBadRequest, "invalid_request_body")
	cov3xWant(t, w.do(http.MethodPut, "/time-standards/missing", "admin", `{"name":"x"}`), http.StatusNotFound, "standard_not_found")
	cov3xWant(t, w.do(http.MethodPut, up, "admin", `{"time_ms":0}`), http.StatusBadRequest, "invalid_time_standard")
	cov3xWant(t, w.do(http.MethodPut, up, "admin", `{"name":"  "}`), http.StatusBadRequest, "invalid_time_standard")
	rr = w.do(http.MethodPut, up, "admin", `{"name":"AAA Updated","description":"d","time_ms":59000,"is_active":false}`)
	cov3xWant(t, rr, http.StatusOK, "")
	if ts := cov3xJSON(t, rr)["time_standard"].(map[string]any); ts["name"] != "AAA Updated" || ts["time_ms"] != float64(59000) || ts["is_active"] != false {
		t.Fatalf("update not applied: %v", ts)
	}
	cov3xWant(t, w.do(http.MethodPut, up, "admin", `{"is_active":true,"time_ms":60000}`), http.StatusOK, "")

	// cut qualification
	w.postResult(t, w.swA, "free", 100, `,"fin_time_ms":59000,"is_official":true`, "")
	cq := "/swimmers/" + w.swA + "/cut-qualification"
	cov3xWant(t, w.do(http.MethodGet, cq, "admin", ""), http.StatusBadRequest, "missing_standard_id")
	cov3xWant(t, w.do(http.MethodGet, cq+"?standard_id=%20", "admin", ""), http.StatusBadRequest, "missing_standard_id")
	cov3xWant(t, w.do(http.MethodGet, cq+"?standard_id="+id, "stranger", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, w.do(http.MethodGet, cq+"?standard_id="+id, "parentB", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, w.do(http.MethodGet, cq+"?standard_id=missing", "parentA", ""), http.StatusNotFound, "standard_not_found")
	rr = w.do(http.MethodGet, cq+"?standard_id="+id, "parentA", "")
	cov3xWant(t, rr, http.StatusOK, "")
	q := cov3xJSON(t, rr)["qualification"].(map[string]any)
	if q["qualified"] != true || q["best_time_ms"] != float64(59000) || q["margin_ms"] != float64(1000) || q["cut_time_ms"] != float64(60000) {
		t.Fatalf("unexpected qualification: %v", q)
	}

	// delete
	dp := "/time-standards/" + id
	cov3xWant(t, w.do(http.MethodDelete, dp, "coach", ""), http.StatusForbidden, "Only admins")
	cov3xWant(t, w.do(http.MethodDelete, "/time-standards/missing", "admin", ""), http.StatusNotFound, "standard_not_found")
	rr = w.do(http.MethodDelete, dp, "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	if got := cov3xJSON(t, rr); got["deleted"] != true || got["id"] != id {
		t.Fatalf("unexpected delete response: %v", got)
	}
	cov3xWant(t, w.do(http.MethodGet, dp, "admin", ""), http.StatusNotFound, "standard_not_found")
}

func TestCov3xSwimmerActivityFeed(t *testing.T) {
	w := newCov3xPerfWorld(t)

	// Source data for swimmer A across every domain.
	w.postResult(t, w.swA, "free", 100, `,"fin_time_ms":61500,"is_official":true`, "")
	w.postResult(t, w.swA, "back", 50, `,"is_official":false,"dq":true`, "")
	w.postResult(t, w.swA, "fly", 50, `,"is_official":false`, "")
	w.postResult(t, w.swA, "breast", 100, `,"fin_time_ms":9990`, "")
	w.postResult(t, w.swA, "im", 200, `,"fin_time_ms":180000`, "")
	w.postResult(t, w.swA, "free_relay", 200, `,"fin_time_ms":120000`, "")
	w.postResult(t, w.swB, "free", 100, `,"fin_time_ms":70000,"is_official":true`, "")

	grp, _, err := w.groups.CreateGroup(w.admin, cov3xOrg, groups.CreateGroupInput{Code: "SENIOR", Name: "Senior"})
	if err != nil {
		t.Fatal(err)
	}
	regA, err := w.groups.CreateRegistration(w.admin, cov3xOrg, groups.CreateRegistrationInput{HouseholdID: w.hhA, SwimmerID: w.swA, GroupID: grp.ID, Season: "2026"})
	if err != nil {
		t.Fatal(err)
	}
	regB, err := w.groups.CreateRegistration(w.admin, cov3xOrg, groups.CreateRegistrationInput{HouseholdID: w.hhB, SwimmerID: w.swB, GroupID: grp.ID, Season: "2026"})
	if err != nil {
		t.Fatal(err)
	}
	for _, reg := range []string{regA.ID, regB.ID} {
		if _, err := w.groups.AcceptWaiver(w.admin, cov3xOrg, reg, groups.AcceptWaiverInput{WaiverType: "Liability"}); err != nil {
			t.Fatal(err)
		}
	}

	dt, err := w.docs.CreateDocumentType(cov3xOrg, "Medical Form", "medical", "", "swimmer", true, true, nil, cov3xAdminID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := w.docs.CreateSubmission(cov3xOrg, dt.ID, w.hhA, w.swA, cov3xAdminID)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := w.docs.CreateSubmission(cov3xOrg, dt.ID, w.hhA, w.swA, cov3xAdminID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.docs.AttachFile(cov3xOrg, verified.ID, "documents/k", "m.pdf", "application/pdf", 3, cov3xAdminID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.docs.VerifySubmission(cov3xOrg, verified.ID, cov3xAdminID, "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.docs.CreateSubmission(cov3xOrg, dt.ID, w.hhA, "", cov3xAdminID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.docs.CreateSubmission(cov3xOrg, dt.ID, w.hhA, w.swB, cov3xAdminID); err != nil {
		t.Fatal(err)
	}

	meet, _, err := w.meets.CreateMeet(w.admin, cov3xOrg, meets.CreateMeetInput{Name: "Fall Classic", MeetType: "invitational", Course: "SCY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.meets.TransitionMeetStatus(w.admin, cov3xOrg, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := w.meets.PublishCommitmentRequest(w.admin, cov3xOrg, meet.ID); err != nil {
		t.Fatal(err)
	}

	path := "/swimmers/" + w.swA + "/activity"
	rr := w.do(http.MethodGet, path, "parentA", "")
	cov3xWant(t, rr, http.StatusOK, "")
	feed := cov3xJSON(t, rr)
	acts := feed["activities"].([]any)
	if feed["swimmer_id"] != w.swA || feed["count"] != float64(len(acts)) {
		t.Fatalf("count/swimmer mismatch: %v", feed["count"])
	}
	byPrefix := map[string][]map[string]any{}
	var prev time.Time
	for i, raw := range acts {
		a := raw.(map[string]any)
		id := a["id"].(string)
		byPrefix[strings.SplitN(id, ":", 2)[0]] = append(byPrefix[strings.SplitN(id, ":", 2)[0]], a)
		at, err := time.Parse(time.RFC3339Nano, a["occurred_at"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && at.After(prev) {
			t.Fatalf("feed must be newest first: %v after %v", at, prev)
		}
		prev = at
	}
	if len(byPrefix["result"]) != 6 || len(byPrefix["registration"]) != 1 || len(byPrefix["waiver"]) != 1 ||
		len(byPrefix["document"]) != 2 || len(byPrefix["commitment"]) != 1 {
		t.Fatalf("unexpected feed composition: result=%d registration=%d waiver=%d document=%d commitment=%d",
			len(byPrefix["result"]), len(byPrefix["registration"]), len(byPrefix["waiver"]), len(byPrefix["document"]), len(byPrefix["commitment"]))
	}
	titles := map[string]string{}
	details := map[string]string{}
	for _, a := range byPrefix["result"] {
		titles[a["title"].(string)] = a["status"].(string)
		details[a["title"].(string)], _ = a["detail"].(string)
	}
	if details["100 SCY Freestyle"] != "Finished in 1:01.50." || details["50 SCY Backstroke"] != "Marked as disqualified." ||
		details["50 SCY Butterfly"] != "Recorded progression result." || details["100 SCY Breaststroke"] != "Finished in 09.99." ||
		details["200 SCY IM"] != "Finished in 3:00.00." || details["200 SCY free_relay"] != "Finished in 2:00.00." {
		t.Fatalf("unexpected result details: %v", details)
	}
	if reg := byPrefix["registration"][0]; reg["title"] != "Registration pending" || reg["detail"] != "Season 2026 registration recorded." {
		t.Fatalf("unexpected registration activity: %v", reg)
	}
	if wv := byPrefix["waiver"][0]; wv["detail"] != "liability waiver accepted for 2026." || wv["status"] != "accepted" {
		t.Fatalf("unexpected waiver activity: %v", wv)
	}
	docTitles := map[string]string{}
	for _, d := range byPrefix["document"] {
		docTitles[d["source_id"].(string)] = d["title"].(string) + "|" + d["detail"].(string)
	}
	if docTitles[pending.ID] != "Document pending|Medical Form document pending." || docTitles[verified.ID] != "Document verified|Medical Form document verified." {
		t.Fatalf("unexpected document activity: %v", docTitles)
	}
	if c := byPrefix["commitment"][0]; c["title"] != "Meet commitment invited" || c["detail"] != "Response recorded for Fall Classic." || c["meet_id"] != meet.ID {
		t.Fatalf("unexpected commitment activity: %v", c)
	}

	// Access control and lookups.
	cov3xWant(t, w.do(http.MethodGet, path, "stranger", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, w.do(http.MethodGet, path, "parentB", ""), http.StatusNotFound, "swimmer_not_found")
	cov3xWant(t, w.do(http.MethodGet, "/swimmers/missing/activity", "admin", ""), http.StatusNotFound, "swimmer_not_found")
	cov3xWant(t, w.do(http.MethodGet, path, "swimA", ""), http.StatusOK, "activities")

	// A performance service that cannot prove household ownership fails closed.
	noChecker := NewPerformanceHandlersWithAuditAndContext(performance.NewService(), w.members, w.groups, w.meets, w.docs, w.audit)
	r := cov3xRouterWith(w.tokens(), noChecker.RegisterRoutes)
	cov3xWant(t, cov3xDo(r, http.MethodGet, path, "parentA", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, cov3xDo(r, http.MethodGet, path, "admin", ""), http.StatusOK, "activities")
}

func TestCov3xActivityHelpers(t *testing.T) {
	for in, want := range map[string]string{
		"free": "Freestyle", "Freestyle": "Freestyle", " back ": "Backstroke", "backstroke": "Backstroke",
		"breast": "Breaststroke", "breaststroke": "Breaststroke", "fly": "Butterfly", "BUTTERFLY": "Butterfly",
		"im": "IM", "individual_medley": "IM", "medley_relay": "medley_relay",
	} {
		if got := strokeActivityLabel(in); got != want {
			t.Errorf("strokeActivityLabel(%q) = %q, want %q", in, got, want)
		}
	}
	for ms, want := range map[int]string{0: "00.00", 9990: "09.99", 61500: "1:01.50", 125040: "2:05.04", 3599990: "59:59.99"} {
		if got := formatActivityTime(ms); got != want {
			t.Errorf("formatActivityTime(%d) = %q, want %q", ms, got, want)
		}
	}
	if got := humanizeStatus("coach_reviewed"); got != "coach reviewed" {
		t.Errorf("humanizeStatus = %q", got)
	}
}

func TestCov3xPerformanceDatabaseOutageReturns500(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := performance.NewServiceWithPool(pool)
	h := NewPerformanceHandlersWithAudit(svc, audit.NewService())
	r := cov3xRouter(h.RegisterRoutes)
	pool.Close()

	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/results", fmt.Sprintf(`{"swimmer_id":%q,"distance":50,"stroke":"free","course":"SCY","gender":"F","fin_time_ms":30000,"is_official":true}`, cov3xSw1)},
		{http.MethodGet, "/swimmers/" + cov3xSw1 + "/best-times", ""},
		{http.MethodGet, "/performance/best-times", ""},
		{http.MethodPost, "/swimmers/" + cov3xSw1 + "/best-times/override", `{"event_distance":50,"stroke":"free","course":"SCY","reason":"r"}`},
		{http.MethodGet, "/swimmers/" + cov3xSw1 + "/progression", ""},
		{http.MethodGet, "/performance/progression", ""},
		{http.MethodGet, "/swimmers/" + cov3xSw1 + "/cut-qualification?standard_id=s", ""},
		{http.MethodGet, "/time-standards", ""},
		{http.MethodPost, "/time-standards", `{"name":"n","season_year":2026,"standard_code":"c","event_distance":50,"stroke":"free","course":"SCY","gender":"F","time_ms":30000}`},
		{http.MethodGet, "/time-standards/s", ""},
		{http.MethodPut, "/time-standards/s", `{"name":"n"}`},
		{http.MethodDelete, "/time-standards/s", ""},
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

func TestCov3xTimeStandardWriteFailureReturns500(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	cov3xInsertUser(t, pool, cov3xOrg, cov3xAdminID)
	svc := performance.NewServiceWithPool(pool)
	h := NewPerformanceHandlers(svc)
	r := cov3xRouter(h.RegisterRoutes)

	create := `{"name":"AAA","season_year":2026,"standard_code":"AAA","event_distance":50,"stroke":"free","course":"SCY","gender":"F","time_ms":30000,"is_active":true}`
	rr := cov3xDo(r, http.MethodPost, "/time-standards", "admin", create)
	cov3xWant(t, rr, http.StatusCreated, "")
	id := cov3xJSON(t, rr)["time_standard"].(map[string]any)["id"].(string)

	cov3xBlockWrites(t, pool, "time_standards")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/time-standards", "admin", strings.Replace(create, `"AAA"`, `"BBB"`, 2)), http.StatusInternalServerError, "internal_error")
	cov3xWant(t, cov3xDo(r, http.MethodPut, "/time-standards/"+id, "admin", `{"name":"Renamed"}`), http.StatusInternalServerError, "internal_error")
	cov3xWant(t, cov3xDo(r, http.MethodDelete, "/time-standards/"+id, "admin", ""), http.StatusInternalServerError, "internal_error")
	// The failed writes left the original untouched.
	rr = cov3xDo(r, http.MethodGet, "/time-standards/"+id, "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	if ts := cov3xJSON(t, rr)["time_standard"].(map[string]any); ts["name"] != "AAA" {
		t.Fatalf("failed update must not change the standard: %v", ts)
	}
}

// swimmerProfileDirectory is a minimal performance.Directory for DB tests.
type cov3xDirectory struct{ profiles []performance.SwimmerProfile }

func (d cov3xDirectory) SwimmerProfiles(string) []performance.SwimmerProfile { return d.profiles }
func (d cov3xDirectory) Meets(string) []performance.MeetInfo                 { return nil }

func TestCov3xResultSavedButRecordDetectionFails(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	cov3xInsertUser(t, pool, cov3xOrg, cov3xAdminID)
	admin := &appmiddleware.Principal{UserID: cov3xAdminID, OrgID: cov3xOrg, Roles: []string{"admin"}}
	mem := members.NewServiceWithPool(pool)
	hh, err := mem.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov3xOrg, HouseholdName: "Rec", PrimaryEmail: "rec@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := mem.CreateSwimmer(admin, cov3xOrg, members.CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Rae", LastName: "Rec", DateOfBirth: "2012-04-05", Gender: "female", RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := performance.NewServiceWithPool(pool)
	svc.SetDirectory(cov3xDirectory{profiles: []performance.SwimmerProfile{{
		ID: sw.ID, HouseholdID: hh.ID, FirstName: "Rae", LastName: "Rec", DateOfBirth: "2012-04-05", Gender: "female",
	}}})
	h := NewPerformanceHandlers(svc)
	r := cov3xRouter(h.RegisterRoutes)
	body := func(fin int) string {
		return fmt.Sprintf(`{"swimmer_id":%q,"distance":100,"stroke":"free","course":"SCY","gender":"F","fin_time_ms":%d,"is_official":true}`, sw.ID, fin)
	}

	rr := cov3xDo(r, http.MethodPost, "/results", "coach", body(61000))
	cov3xWant(t, rr, http.StatusCreated, "new_team_records")
	if n := len(cov3xJSON(t, rr)["new_team_records"].([]any)); n == 0 {
		t.Fatalf("first official swim should set records: %s", rr.Body.String())
	}

	cov3xBlockWrites(t, pool, "team_record_entries")
	rr = cov3xDo(r, http.MethodPost, "/results", "coach", body(59000))
	cov3xWant(t, rr, http.StatusInternalServerError, "record_detection_incomplete")
	if strings.Contains(rr.Body.String(), "blocked write") {
		t.Fatalf("driver text leaked: %s", rr.Body.String())
	}
	// The swim itself was stored.
	rr = cov3xDo(r, http.MethodGet, "/swimmers/"+sw.ID+"/progression", "admin", "")
	if cov3xJSON(t, rr)["count"] != float64(2) {
		t.Fatalf("result must be saved despite the detection failure: %s", rr.Body.String())
	}
	_ = context.Background()
}

func (w *cov3xPerfWorld) routerWith(perf *performance.Service, mem *members.Service, mt *meets.Service, docs *documents.Service) http.Handler {
	h := NewPerformanceHandlersWithAuditAndContext(perf, mem, w.groups, mt, docs, w.audit)
	return cov3xRouterWith(w.tokens(), h.RegisterRoutes)
}

func TestCov3xActivityFeedDependencyFailures(t *testing.T) {
	w := newCov3xPerfWorld(t)
	path := "/swimmers/" + w.swA + "/activity"

	t.Run("members database down", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		down := members.NewServiceWithPool(pool)
		pool.Close()
		r := w.routerWith(w.perf, down, w.meets, w.docs)
		cov3xWant(t, cov3xDo(r, http.MethodGet, path, "admin", ""), http.StatusInternalServerError, "internal_error")
	})
	t.Run("performance database down", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		down := performance.NewServiceWithPool(pool)
		pool.Close()
		r := w.routerWith(down, w.members, w.meets, w.docs)
		cov3xWant(t, cov3xDo(r, http.MethodGet, path, "admin", ""), http.StatusInternalServerError, "internal_error")
	})
	t.Run("documents database down", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		down := documents.NewServiceWithPool(pool)
		pool.Close()
		r := w.routerWith(w.perf, w.members, w.meets, down)
		cov3xWant(t, cov3xDo(r, http.MethodGet, path, "admin", ""), http.StatusInternalServerError, "internal_error")
	})
	t.Run("meets database down after a meet was cached", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		cov3xInsertUser(t, pool, cov3xOrg, cov3xAdminID)
		mt := meets.NewServiceWithPool(pool)
		if _, _, err := mt.CreateMeet(w.admin, cov3xOrg, meets.CreateMeetInput{Name: "Cached", MeetType: "invitational", Course: "SCY"}); err != nil {
			t.Fatal(err)
		}
		r := w.routerWith(w.perf, w.members, mt, w.docs)
		cov3xWant(t, cov3xDo(r, http.MethodGet, path, "admin", ""), http.StatusOK, "activities")
		pool.Close()
		cov3xWant(t, cov3xDo(r, http.MethodGet, path, "admin", ""), http.StatusInternalServerError, "internal_error")
	})
	t.Run("a meet deleted behind the cache is skipped", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		cov3xInsertUser(t, pool, cov3xOrg, cov3xAdminID)
		mt := meets.NewServiceWithPool(pool)
		if _, _, err := mt.CreateMeet(w.admin, cov3xOrg, meets.CreateMeetInput{Name: "Doomed", MeetType: "invitational", Course: "SCY"}); err != nil {
			t.Fatal(err)
		}
		// Snapshot readers still see the cached meet, but the reload that
		// ListCommitments performs no longer finds it.
		if _, err := pool.Exec(context.Background(), `DELETE FROM meets`); err != nil {
			t.Fatal(err)
		}
		r := w.routerWith(w.perf, w.members, mt, w.docs)
		rr := cov3xDo(r, http.MethodGet, path, "admin", "")
		cov3xWant(t, rr, http.StatusOK, "activities")
		if cov3xJSON(t, rr)["count"] != float64(0) {
			t.Fatalf("deleted meet must not produce activity: %s", rr.Body.String())
		}
	})
}

func TestCov3xActivityFeedOrdersTiedTimestampsByID(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	cov3xInsertUser(t, pool, cov3xOrg, cov3xAdminID)
	admin := &appmiddleware.Principal{UserID: cov3xAdminID, OrgID: cov3xOrg, Roles: []string{"admin"}}
	mem := members.NewServiceWithPool(pool)
	hh, err := mem.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov3xOrg, HouseholdName: "Tie", PrimaryEmail: "tie@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := mem.CreateSwimmer(admin, cov3xOrg, members.CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Tia", LastName: "Tie", DateOfBirth: "2012-04-05", Gender: "female", RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	writer := performance.NewServiceWithPool(pool)
	writeH := NewPerformanceHandlers(writer)
	wr := cov3xRouter(writeH.RegisterRoutes)
	for _, fin := range []int{61000, 62000, 63000} {
		body := fmt.Sprintf(`{"swimmer_id":%q,"distance":100,"stroke":"free","course":"SCY","gender":"F","fin_time_ms":%d}`, sw.ID, fin)
		cov3xWant(t, cov3xDo(wr, http.MethodPost, "/results", "admin", body), http.StatusCreated, "")
	}
	if _, err := pool.Exec(context.Background(), `UPDATE results SET updated_at = '2026-02-03T04:05:06Z'`); err != nil {
		t.Fatal(err)
	}

	reader := performance.NewServiceWithPool(pool)
	reader.SetHouseholdChecker(mem)
	h := NewPerformanceHandlersWithAuditAndContext(reader, mem, groups.NewService(), meets.NewService(), documents.NewService(), audit.NewService())
	r := cov3xRouter(h.RegisterRoutes)
	rr := cov3xDo(r, http.MethodGet, "/swimmers/"+sw.ID+"/activity", "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	acts := cov3xJSON(t, rr)["activities"].([]any)
	if len(acts) != 3 {
		t.Fatalf("want 3 results, got %d", len(acts))
	}
	for i := 1; i < len(acts); i++ {
		a, b := acts[i-1].(map[string]any), acts[i].(map[string]any)
		if a["occurred_at"] != b["occurred_at"] {
			t.Fatalf("timestamps should tie: %v vs %v", a["occurred_at"], b["occurred_at"])
		}
		if a["id"].(string) <= b["id"].(string) {
			t.Fatalf("tied entries must sort by descending id: %v then %v", a["id"], b["id"])
		}
	}
}
