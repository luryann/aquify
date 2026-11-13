package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/testutil"
)

type cov3xMeetFixture struct {
	svc   *meets.Service
	audit *audit.Service
	h     *MeetsHandlers
	r     http.Handler
}

func newCov3xMeetFixture(t *testing.T) cov3xMeetFixture {
	t.Helper()
	svc := meets.NewService()
	svc.SeedSwimmer(cov3xOrg, cov3xSw1, cov3xHH1)
	svc.SeedSwimmer(cov3xOrg, cov3xSw2, cov3xHH2)
	svc.SeedParentHousehold(cov3xParentID, cov3xHH1)
	svc.SeedParentHousehold(cov3xParent2ID, cov3xHH2)
	svc.SetSwimmerEligibilityLookup(func(orgID, swimmerID string) (meets.SwimmerEligibilityView, bool) {
		switch swimmerID {
		case cov3xSw1:
			return meets.SwimmerEligibilityView{Gender: "male", DateOfBirth: "2012-05-01"}, true
		case cov3xSw2:
			return meets.SwimmerEligibilityView{Gender: "female", DateOfBirth: "2012-05-01"}, true
		}
		return meets.SwimmerEligibilityView{}, false
	})
	auditSvc := audit.NewService()
	h := NewMeetsHandlersWithAudit(svc, auditSvc)
	return cov3xMeetFixture{svc: svc, audit: auditSvc, h: h, r: cov3xRouter(h.RegisterRoutes)}
}

func (f cov3xMeetFixture) do(method, path, token, body string) *httptest.ResponseRecorder {
	return cov3xDo(f.r, method, path, token, body)
}

// meetWithEvents creates a meet with one open event (E1, 50 free) and one
// girls-only event (E2, 100 back) and returns its id and the two event ids.
func (f cov3xMeetFixture) meetWithEvents(t *testing.T, extra string) (meetID, e1, e2 string) {
	t.Helper()
	body := `{"name":"Spring Open","meet_type":"invitational","course":"SCY","start_date":"2026-12-05","end_date":"2026-12-06",
		"events":[{"event_number":"1","stroke":"freestyle","distance":50,"gender":"O"},
		          {"event_number":"2","stroke":"backstroke","distance":100,"gender":"F"}]` + extra + `}`
	rr := f.do(http.MethodPost, "/meets", "admin", body)
	cov3xWant(t, rr, http.StatusCreated, "")
	meetID = cov3xJSON(t, rr)["meet"].(map[string]any)["id"].(string)
	rr = f.do(http.MethodGet, "/meets/"+meetID+"/events", "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	evs := cov3xJSON(t, rr)["events"].([]any)
	if len(evs) != 2 {
		t.Fatalf("want 2 events, got %d", len(evs))
	}
	for _, raw := range evs {
		ev := raw.(map[string]any)
		switch ev["event_number"] {
		case "1":
			e1 = ev["id"].(string)
		case "2":
			e2 = ev["id"].(string)
		}
	}
	return meetID, e1, e2
}

func (f cov3xMeetFixture) transition(t *testing.T, meetID, status string) {
	t.Helper()
	cov3xWant(t, f.do(http.MethodPost, "/meets/"+meetID+"/transition", "admin", `{"status":"`+status+`"}`), http.StatusOK, status)
}

// openCommitments publishes the meet and returns commitment ids by swimmer.
func (f cov3xMeetFixture) openCommitments(t *testing.T, meetID string) map[string]string {
	t.Helper()
	f.transition(t, meetID, "published")
	cov3xWant(t, f.do(http.MethodPost, "/meets/"+meetID+"/commitment-request", "admin", ""), http.StatusOK, "commitment_request_published")
	rr := f.do(http.MethodGet, "/meets/"+meetID+"/commitments", "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	out := map[string]string{}
	for _, raw := range cov3xJSON(t, rr)["commitments"].([]any) {
		c := raw.(map[string]any)
		out[c["swimmer_id"].(string)] = c["id"].(string)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 commitments, got %v", out)
	}
	return out
}

func cov3xMultipartRequest(t *testing.T, field, filename string, data []byte, token string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/meets/parse-entry-file", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", cov3xOrg)
	return req
}

func TestCov3xMeetsScopeGuards(t *testing.T) {
	f := newCov3xMeetFixture(t)
	cov3xAssertScopeGuards(t, f.h.RegisterRoutes, []cov3xRoute{
		{http.MethodPost, "/meets", `{}`},
		{http.MethodPost, "/meets/parse-entry-file", ""},
		{http.MethodGet, "/meets", ""},
		{http.MethodGet, "/meets/m1", ""},
		{http.MethodPost, "/meets/m1/sessions", `{}`},
		{http.MethodGet, "/meets/m1/sessions", ""},
		{http.MethodPost, "/meets/m1/events", `{}`},
		{http.MethodGet, "/meets/m1/events", ""},
		{http.MethodPost, "/meets/m1/transition", `{}`},
		{http.MethodPost, "/meets/m1/commitment-request", ""},
		{http.MethodGet, "/meets/m1/commitments", ""},
		{http.MethodPost, "/meets/m1/commitments/c1/respond", `{}`},
		{http.MethodPost, "/meets/m1/commitments/c1/event-selections", `{}`},
		{http.MethodGet, "/meets/m1/commitments/c1/event-selections", ""},
		{http.MethodDelete, "/meets/m1/commitments/c1/event-selections/s1", ""},
		{http.MethodPost, "/meets/m1/lock-commitments", ""},
		{http.MethodGet, "/meets/m1/entries", ""},
		{http.MethodPost, "/meets/m1/entries", `{}`},
		{http.MethodPost, "/meets/m1/entries/e1/override", `{}`},
		{http.MethodPost, "/meets/m1/entries/e1/approve", ""},
		{http.MethodDelete, "/meets/m1/entries/e1", ""},
	})
}

func TestCov3xNewMeetsHandlersWithoutAudit(t *testing.T) {
	svc := meets.NewService()
	h := NewMeetsHandlers(svc)
	if h.auditService != nil || h.validate == nil || h.service != svc {
		t.Fatalf("unexpected handler wiring: %+v", h)
	}
	// Mutations still succeed when no audit service is wired.
	r := cov3xRouter(h.RegisterRoutes)
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/meets", "admin", `{"name":"No Audit"}`), http.StatusCreated, "No Audit")
}

func TestCov3xCreateGetListMeets(t *testing.T) {
	f := newCov3xMeetFixture(t)

	cov3xWant(t, f.do(http.MethodPost, "/meets", "parent", `{"name":"x"}`), http.StatusForbidden, "not allowed to create meets")
	cov3xWant(t, f.do(http.MethodPost, "/meets", "admin", `{}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, "/meets", "admin", `{"name":"x","course":"XXX"}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, "/meets", "admin", `{"name":"x","start_date":"not-a-date"}`), http.StatusBadRequest, "invalid_meet_request")

	rr := f.do(http.MethodPost, "/meets", "coach", `{"name":"Full Meet","meet_type":"championship","course":"LCM","start_date":"2026-12-05","end_date":"2026-12-06",
		"entry_deadline":"2026-11-30","commitment_deadline":"2026-11-20","host_org":"Hosts","location":"Aquatic Center",
		"sessions":[{"session_number":1,"name":"Prelims","session_date":"2026-12-05","warmup_time":"07:00","start_time":"08:00","notes":"n"}],
		"events":[{"session_number":1,"event_number":"1","stroke":"butterfly","distance":100,"course":"LCM","gender":"M","min_age":10,"max_age":12,"entry_fee_cents":500,"is_relay":false}],
		"signup_settings":{"self_signup_enabled":true,"max_entries_per_swimmer":3}}`)
	cov3xWant(t, rr, http.StatusCreated, "created")
	body := cov3xJSON(t, rr)
	meet := body["meet"].(map[string]any)
	if meet["name"] != "Full Meet" || meet["course"] != "LCM" || meet["host_org"] != "Hosts" || meet["status"] != "draft" || body["audit_event"] == nil {
		t.Fatalf("unexpected meet: %v", meet)
	}
	id := meet["id"].(string)
	rr = f.do(http.MethodGet, "/meets/"+id+"/sessions", "coach", "")
	cov3xWant(t, rr, http.StatusOK, "Prelims")
	rr = f.do(http.MethodGet, "/meets/"+id+"/events", "coach", "")
	evs := cov3xJSON(t, rr)["events"].([]any)
	if len(evs) != 1 || evs[0].(map[string]any)["entry_fee_cents"] != float64(500) || evs[0].(map[string]any)["min_age"] != float64(10) {
		t.Fatalf("event draft not mapped: %v", evs)
	}
	if got, _ := f.audit.Query(audit.Filter{OrgID: cov3xOrg}); len(got) != 1 {
		t.Fatalf("meet creation must be audited, got %d events", len(got))
	}

	// list
	cov3xWant(t, f.do(http.MethodGet, "/meets", "stranger", ""), http.StatusForbidden, "not allowed to list meets")
	rr = f.do(http.MethodGet, "/meets?limit=1", "coach", "")
	cov3xWant(t, rr, http.StatusOK, "")
	lst := cov3xJSON(t, rr)
	if lst["total"] != float64(1) || lst["limit"] != float64(1) || len(lst["meets"].([]any)) != 1 {
		t.Fatalf("unexpected list: %v", lst)
	}

	// get
	cov3xWant(t, f.do(http.MethodGet, "/meets/"+id, "stranger", ""), http.StatusForbidden, "not allowed to view this meet")
	cov3xWant(t, f.do(http.MethodGet, "/meets/missing", "admin", ""), http.StatusNotFound, "meet_not_found")
	cov3xWant(t, f.do(http.MethodGet, "/meets/%20", "admin", ""), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodGet, "/meets/"+id, "coach", ""), http.StatusOK, "Full Meet")
}

func TestCov3xParseEntryFile(t *testing.T) {
	f := newCov3xMeetFixture(t)
	serve := func(req *http.Request) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		f.r.ServeHTTP(rr, req)
		return rr
	}
	events, err := os.ReadFile("../../../../internal/meetfiles/testdata/fixtures/bca-fall-festival.hyv")
	if err != nil {
		t.Fatal(err)
	}

	rr := serve(cov3xMultipartRequest(t, "file", "Meet Events.hyv", events, "coach"))
	cov3xWant(t, rr, http.StatusOK, "")
	parsed := cov3xJSON(t, rr)["parsed"].(map[string]any)
	if parsed["meet_name"] != "2026 BCA Fall Festival" || parsed["start_date"] != "2026-11-14" || parsed["course"] != "SCY" || len(parsed["events"].([]any)) != 79 {
		t.Fatalf("unexpected parse: %v", parsed)
	}
	// Entry and results formats are not meet event files.
	cov3xWant(t, serve(cov3xMultipartRequest(t, "file", "entries.sd3", []byte("A01V3"), "admin")), http.StatusBadRequest, "invalid_meet_file")

	cov3xWant(t, serve(cov3xMultipartRequest(t, "file", "meet.hyv", events, "parent")), http.StatusForbidden, "not allowed to parse")
	cov3xWant(t, serve(cov3xMultipartRequest(t, "other", "meet.hyv", events, "admin")), http.StatusBadRequest, "missing_file")
	cov3xWant(t, serve(cov3xMultipartRequest(t, "file", "meet.zip", []byte("this is not a zip archive"), "admin")), http.StatusBadRequest, "invalid_meet_file")
	cov3xWant(t, serve(cov3xMultipartRequest(t, "file", "meet.hyv", nil, "admin")), http.StatusBadRequest, "meet.hyv is empty")

	// A plain JSON body is not multipart at all.
	rr = f.do(http.MethodPost, "/meets/parse-entry-file", "admin", `{"file":"x"}`)
	cov3xWant(t, rr, http.StatusBadRequest, "missing_file")

	// Exactly one byte over the cap, still below the request ceiling.
	big := bytes.Repeat([]byte("A"), 25<<20+1)
	cov3xWant(t, serve(cov3xMultipartRequest(t, "file", "meet.hyv", big, "admin")), http.StatusRequestEntityTooLarge, "file_too_large")
	// Far above the request ceiling.
	huge := bytes.Repeat([]byte("A"), 27<<20)
	cov3xWant(t, serve(cov3xMultipartRequest(t, "file", "meet.hyv", huge, "admin")), http.StatusRequestEntityTooLarge, "file_too_large")
}

func TestCov3xMeetTransitionSessionsAndEvents(t *testing.T) {
	f := newCov3xMeetFixture(t)
	rr := f.do(http.MethodPost, "/meets", "admin", `{"name":"Lifecycle"}`)
	id := cov3xJSON(t, rr)["meet"].(map[string]any)["id"].(string)

	// transition
	tp := "/meets/" + id + "/transition"
	cov3xWant(t, f.do(http.MethodPost, tp, "stranger", `{"status":"published"}`), http.StatusForbidden, "not allowed to transition")
	cov3xWant(t, f.do(http.MethodPost, tp, "admin", `{}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/transition", "admin", `{"status":"published"}`), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodPost, "/meets/missing/transition", "admin", `{"status":"published"}`), http.StatusNotFound, "meet_not_found")
	cov3xWant(t, f.do(http.MethodPost, tp, "admin", `{"status":"completed"}`), http.StatusUnprocessableEntity, "invalid_transition")
	cov3xWant(t, f.do(http.MethodPost, tp, "admin", `{"status":"bogus"}`), http.StatusUnprocessableEntity, "invalid_transition")
	cov3xWant(t, f.do(http.MethodPost, tp, "admin", `{"status":"commitment_open"}`), http.StatusUnprocessableEntity, "commitment-request")

	// sessions
	sp := "/meets/" + id + "/sessions"
	cov3xWant(t, f.do(http.MethodPost, sp, "parent", `{"session_number":1,"name":"S1"}`), http.StatusForbidden, "not allowed to add sessions")
	cov3xWant(t, f.do(http.MethodPost, sp, "admin", `{"name":"S1"}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/sessions", "admin", `{"session_number":1,"name":"S1"}`), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodPost, "/meets/missing/sessions", "admin", `{"session_number":1,"name":"S1"}`), http.StatusNotFound, "meet_not_found")
	cov3xWant(t, f.do(http.MethodPost, sp, "admin", `{"session_number":1,"name":"S1","session_date":"nope"}`), http.StatusBadRequest, "invalid_session_request")
	rr = f.do(http.MethodPost, sp, "coach", `{"session_number":1,"name":"S1","session_date":"2026-12-05"}`)
	cov3xWant(t, rr, http.StatusCreated, "created")
	sessionID := cov3xJSON(t, rr)["session"].(map[string]any)["id"].(string)
	cov3xWant(t, f.do(http.MethodGet, sp, "stranger", ""), http.StatusForbidden, "not allowed to list sessions")
	cov3xWant(t, f.do(http.MethodGet, "/meets/missing/sessions", "admin", ""), http.StatusNotFound, "meet_not_found")
	cov3xWant(t, f.do(http.MethodGet, "/meets/%20/sessions", "admin", ""), http.StatusBadRequest, "invalid_id")

	// events
	ep := "/meets/" + id + "/events"
	ev := func(extra string) string {
		return `{"event_number":"7","stroke":"freestyle","distance":50` + extra + `}`
	}
	cov3xWant(t, f.do(http.MethodPost, ep, "parent", ev("")), http.StatusForbidden, "not allowed to add events")
	cov3xWant(t, f.do(http.MethodPost, ep, "admin", `{"event_number":"7"}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/events", "admin", ev("")), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodPost, "/meets/missing/events", "admin", ev("")), http.StatusNotFound, "meet_not_found")
	cov3xWant(t, f.do(http.MethodPost, ep, "admin", ev(`,"session_id":"nope"`)), http.StatusNotFound, "session_not_found")
	cov3xWant(t, f.do(http.MethodPost, ep, "admin", ev(`,"min_age":15,"max_age":10`)), http.StatusBadRequest, "invalid_event_request")
	rr = f.do(http.MethodPost, ep, "admin", ev(`,"session_id":"`+sessionID+`","gender":"M"`))
	cov3xWant(t, rr, http.StatusCreated, "created")
	if got := cov3xJSON(t, rr)["event"].(map[string]any); got["session_id"] != sessionID || got["event_number"] != "7" {
		t.Fatalf("unexpected event: %v", got)
	}
	cov3xWant(t, f.do(http.MethodPost, ep, "admin", ev("")), http.StatusConflict, "event_number_conflict")
	cov3xWant(t, f.do(http.MethodGet, ep, "stranger", ""), http.StatusForbidden, "not allowed to list events")

	// Pagination metadata on a child list.
	rr = f.do(http.MethodGet, ep+"?limit=1&offset=5", "admin", "")
	if page := cov3xJSON(t, rr); page["count"] != float64(0) || page["total"] != float64(1) || page["offset"] != float64(5) {
		t.Fatalf("unexpected page: %v", page)
	}

	// Once published, sessions and events can no longer be added.
	f.openCommitments(t, id)
	cov3xWant(t, f.do(http.MethodPost, "/meets/"+id+"/lock-commitments", "admin", ""), http.StatusOK, "commitments_locked")
	cov3xWant(t, f.do(http.MethodPost, sp, "admin", `{"session_number":2,"name":"S2"}`), http.StatusBadRequest, "invalid_session_request")
	cov3xWant(t, f.do(http.MethodPost, ep, "admin", ev(`,"event_number":"8"`)), http.StatusBadRequest, "invalid_event_request")
}

func TestCov3xCommitmentsAndSelections(t *testing.T) {
	f := newCov3xMeetFixture(t)
	meetID, e1, e2 := f.meetWithEvents(t, "")
	base := "/meets/" + meetID

	// Before publishing there is nothing to commit to.
	cov3xWant(t, f.do(http.MethodPost, base+"/commitment-request", "parent", ""), http.StatusForbidden, "not allowed to publish")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/commitment-request", "admin", ""), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodPost, "/meets/missing/commitment-request", "admin", ""), http.StatusNotFound, "meet_not_found")
	cov3xWant(t, f.do(http.MethodPost, base+"/commitment-request", "admin", ""), http.StatusUnprocessableEntity, "invalid_transition")

	cm := f.openCommitments(t, meetID)
	c1, c2 := cm[cov3xSw1], cm[cov3xSw2]
	cp := func(id string) string { return base + "/commitments/" + id }

	// A freshly invited swimmer cannot pick events yet.
	cov3xWant(t, f.do(http.MethodPost, cp(c1)+"/event-selections", "parent", `{"meet_event_id":"`+e1+`"}`), http.StatusUnprocessableEntity, "not_committed_yes")

	// respond
	cov3xWant(t, f.do(http.MethodPost, cp(c1)+"/respond", "parent", `{"response":"maybe"}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/commitments/x/respond", "parent", `{"response":"committed_yes"}`), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodPost, cp("missing")+"/respond", "parent", `{"response":"committed_yes"}`), http.StatusNotFound, "commitment_not_found")
	cov3xWant(t, f.do(http.MethodPost, cp(c1)+"/respond", "coach", `{"response":"committed_yes"}`), http.StatusForbidden, "forbidden")
	cov3xWant(t, f.do(http.MethodPost, cp(c1)+"/respond", "parent2", `{"response":"committed_yes"}`), http.StatusForbidden, "forbidden")
	rr := f.do(http.MethodPost, cp(c1)+"/respond", "parent", `{"response":"committed_yes","notes":"  see you there "}`)
	cov3xWant(t, rr, http.StatusOK, "updated")
	if c := cov3xJSON(t, rr)["commitment"].(map[string]any); c["status"] != "committed_yes" || c["notes"] != "see you there" {
		t.Fatalf("unexpected commitment: %v", c)
	}
	cov3xWant(t, f.do(http.MethodPost, cp(c2)+"/respond", "parent2", `{"response":"committed_no"}`), http.StatusOK, "committed_no")

	// select
	sel := cp(c1) + "/event-selections"
	cov3xWant(t, f.do(http.MethodPost, sel, "parent", `{}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, sel, "parent", `{"meet_event_id":"`+e1+`","seed_time_ms":0}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/commitments/x/event-selections", "parent", `{"meet_event_id":"`+e1+`"}`), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodPost, sel, "swimmer", `{"meet_event_id":"`+e1+`"}`), http.StatusForbidden, "forbidden")
	cov3xWant(t, f.do(http.MethodPost, sel, "parent2", `{"meet_event_id":"`+e1+`"}`), http.StatusForbidden, "forbidden")
	cov3xWant(t, f.do(http.MethodPost, cp("missing")+"/event-selections", "parent", `{"meet_event_id":"`+e1+`"}`), http.StatusNotFound, "commitment_not_found")
	cov3xWant(t, f.do(http.MethodPost, sel, "parent", `{"meet_event_id":"missing"}`), http.StatusNotFound, "event_not_found")
	cov3xWant(t, f.do(http.MethodPost, sel, "parent", `{"meet_event_id":"`+e2+`"}`), http.StatusBadRequest, "selection_failed")
	rr = f.do(http.MethodPost, sel, "parent", `{"meet_event_id":"`+e1+`","seed_time_ms":31250}`)
	cov3xWant(t, rr, http.StatusCreated, "selected")
	selID := cov3xJSON(t, rr)["selection"].(map[string]any)["id"].(string)
	cov3xWant(t, f.do(http.MethodPost, sel, "parent", `{"meet_event_id":"`+e1+`"}`), http.StatusConflict, "duplicate_selection")

	// list selections
	cov3xWant(t, f.do(http.MethodGet, sel, "stranger", ""), http.StatusForbidden, "forbidden")
	cov3xWant(t, f.do(http.MethodGet, sel, "parent2", ""), http.StatusForbidden, "forbidden")
	cov3xWant(t, f.do(http.MethodGet, cp("missing")+"/event-selections", "admin", ""), http.StatusNotFound, "commitment_not_found")
	cov3xWant(t, f.do(http.MethodGet, "/meets/%20/commitments/x/event-selections", "admin", ""), http.StatusBadRequest, "invalid_id")
	rr = f.do(http.MethodGet, sel, "swimmer", "")
	cov3xWant(t, rr, http.StatusOK, selID)
	if l := cov3xJSON(t, rr); l["total"] != float64(1) {
		t.Fatalf("unexpected selections page: %v", l)
	}

	// remove
	del := sel + "/"
	cov3xWant(t, f.do(http.MethodDelete, "/meets/%20/commitments/x/event-selections/y", "parent", ""), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodDelete, del+selID, "stranger", ""), http.StatusForbidden, "forbidden")
	cov3xWant(t, f.do(http.MethodDelete, del+selID, "parent2", ""), http.StatusForbidden, "forbidden")
	cov3xWant(t, f.do(http.MethodDelete, del+"missing", "parent", ""), http.StatusNotFound, "selection_not_found")
	cov3xWant(t, f.do(http.MethodDelete, del+selID, "parent", ""), http.StatusOK, "removed")
	rr = f.do(http.MethodGet, sel, "admin", "")
	if l := cov3xJSON(t, rr); l["total"] != float64(0) {
		t.Fatalf("selection should be gone: %v", l)
	}
	cov3xWant(t, f.do(http.MethodPost, sel, "parent", `{"meet_event_id":"`+e1+`"}`), http.StatusCreated, "selected")

	// Locking closes the window: selections can no longer change.
	cov3xWant(t, f.do(http.MethodPost, base+"/lock-commitments", "parent", ""), http.StatusForbidden, "not allowed to lock")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/lock-commitments", "admin", ""), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodPost, "/meets/missing/lock-commitments", "admin", ""), http.StatusNotFound, "meet_not_found")
	rr = f.do(http.MethodPost, base+"/lock-commitments", "coach", "")
	cov3xWant(t, rr, http.StatusOK, "commitments_locked")
	if m := cov3xJSON(t, rr)["meet"].(map[string]any); m["status"] != "commitment_closed" {
		t.Fatalf("meet should be closed for commitments: %v", m)
	}
	cov3xWant(t, f.do(http.MethodPost, base+"/lock-commitments", "admin", ""), http.StatusUnprocessableEntity, "invalid_transition")
	cov3xWant(t, f.do(http.MethodPost, cp(c1)+"/respond", "parent", `{"response":"committed_no"}`), http.StatusUnprocessableEntity, "invalid_transition")
	cov3xWant(t, f.do(http.MethodPost, sel, "parent", `{"meet_event_id":"`+e1+`"}`), http.StatusUnprocessableEntity, "commitment_locked")
	cov3xWant(t, f.do(http.MethodDelete, del+"whatever", "parent", ""), http.StatusUnprocessableEntity, "commitment_locked")
	if got, _ := f.audit.Query(audit.Filter{OrgID: cov3xOrg, Action: audit.ActionMeetCommitmentsLock}); len(got) != 1 {
		t.Fatalf("lock must be audited once, got %d", len(got))
	}
}

func TestCov3xEntriesLifecycle(t *testing.T) {
	f := newCov3xMeetFixture(t)
	meetID, e1, e2 := f.meetWithEvents(t, "")
	base := "/meets/" + meetID
	f.openCommitments(t, meetID)

	// Entries cannot be proposed while commitments are still open.
	prop := func(tok, swimmer, event, extra string) *httptest.ResponseRecorder {
		return f.do(http.MethodPost, base+"/entries", tok, fmt.Sprintf(`{"swimmer_id":%q,"meet_event_id":%q%s}`, swimmer, event, extra))
	}
	cov3xWant(t, prop("admin", cov3xSw1, e1, ""), http.StatusUnprocessableEntity, "invalid_state")
	cov3xWant(t, f.do(http.MethodPost, base+"/lock-commitments", "admin", ""), http.StatusOK, "commitments_locked")

	cov3xWant(t, f.do(http.MethodPost, base+"/entries", "admin", `{}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, f.do(http.MethodPost, base+"/entries", "admin", `{"swimmer_id":"s","meet_event_id":"e","seed_time_ms":-4}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, prop("parent", cov3xSw1, e1, ""), http.StatusForbidden, "not allowed to propose")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/entries", "admin", fmt.Sprintf(`{"swimmer_id":%q,"meet_event_id":%q}`, cov3xSw1, e1)), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, f.do(http.MethodPost, "/meets/missing/entries", "admin", fmt.Sprintf(`{"swimmer_id":%q,"meet_event_id":%q}`, cov3xSw1, e1)), http.StatusNotFound, "meet_not_found")
	cov3xWant(t, prop("admin", cov3xSw1, "missing", ""), http.StatusNotFound, "event_not_found")
	cov3xWant(t, prop("admin", cov3xSw1, e2, ""), http.StatusUnprocessableEntity, "ineligible_entry")
	cov3xWant(t, prop("admin", cov3xSw1, e2, `,"override_eligibility":true`), http.StatusUnprocessableEntity, "ineligible_entry")

	rr := prop("coach", cov3xSw1, e1, `,"seed_time_ms":30500`)
	cov3xWant(t, rr, http.StatusCreated, "proposed")
	ent1 := cov3xJSON(t, rr)["entry"].(map[string]any)
	if ent1["status"] != "submitted" || ent1["seed_time_ms"] != float64(30500) {
		t.Fatalf("unexpected entry: %v", ent1)
	}
	ent1ID := ent1["id"].(string)
	cov3xWant(t, prop("admin", cov3xSw1, e1, ""), http.StatusConflict, "duplicate_entry")

	// Eligibility override with a reason is accepted and audited.
	cov3xWant(t, prop("admin", cov3xSw1, e2, `,"override_eligibility":true,"override_reason":"coach discretion"`), http.StatusCreated, "proposed")
	if got, _ := f.audit.Query(audit.Filter{OrgID: cov3xOrg, Action: audit.ActionMeetEntryPropose}); len(got) != 2 {
		t.Fatalf("each accepted proposal must be audited, got %d", len(got))
	}

	// sw2 may take the girls event and the open event.
	rr = prop("admin", cov3xSw2, e2, "")
	cov3xWant(t, rr, http.StatusCreated, "proposed")
	ent2ID := cov3xJSON(t, rr)["entry"].(map[string]any)["id"].(string)
	cov3xWant(t, prop("admin", cov3xSw2, e1, ""), http.StatusCreated, "proposed")

	// list
	cov3xWant(t, f.do(http.MethodGet, base+"/entries", "swimmer", ""), http.StatusForbidden, "not allowed to list entries")
	rr = f.do(http.MethodGet, base+"/entries", "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	if l := cov3xJSON(t, rr); l["total"] != float64(4) {
		t.Fatalf("want 4 entries, got %v", l)
	}
	rr = f.do(http.MethodGet, base+"/entries", "parent", "")
	cov3xWant(t, rr, http.StatusOK, "")
	if l := cov3xJSON(t, rr); l["total"] != float64(2) {
		t.Fatalf("parent must only see own household entries, got %v", l)
	}

	// override
	ov := func(tok, entry, body string) *httptest.ResponseRecorder {
		return f.do(http.MethodPost, base+"/entries/"+entry+"/override", tok, body)
	}
	cov3xWant(t, ov("admin", ent1ID, `{}`), http.StatusBadRequest, "validation_failed")
	cov3xWant(t, ov("parent", ent1ID, `{"new_meet_event_id":"`+e1+`","reason":"r"}`), http.StatusForbidden, "not allowed to override")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/entries/x/override", "admin", `{"new_meet_event_id":"`+e1+`","reason":"r"}`), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, ov("admin", "missing", `{"new_meet_event_id":"`+e1+`","reason":"r"}`), http.StatusNotFound, "entry_not_found")
	cov3xWant(t, ov("admin", ent1ID, `{"new_meet_event_id":"missing","reason":"r"}`), http.StatusNotFound, "event_not_found")
	cov3xWant(t, ov("admin", ent1ID, `{"new_meet_event_id":"`+e2+`","reason":"r"}`), http.StatusUnprocessableEntity, "ineligible_entry")
	cov3xWant(t, ov("admin", ent2ID, `{"new_meet_event_id":"`+e2+`","reason":"same"}`), http.StatusOK, "overridden")
	rr = ov("coach", ent2ID, `{"new_meet_event_id":"`+e2+`","reason":"fix","seed_time_ms":65000}`)
	cov3xWant(t, rr, http.StatusOK, "overridden")
	if e := cov3xJSON(t, rr)["entry"].(map[string]any); e["seed_time_ms"] != float64(65000) || e["override_reason"] != "fix" {
		t.Fatalf("override not applied: %v", e)
	}
	// Moving sw2 onto an event they already hold is a duplicate.
	cov3xWant(t, ov("admin", ent2ID, `{"new_meet_event_id":"`+e1+`","reason":"dup"}`), http.StatusConflict, "duplicate_entry")

	// approve
	ap := func(tok, entry string) *httptest.ResponseRecorder {
		return f.do(http.MethodPost, base+"/entries/"+entry+"/approve", tok, "")
	}
	cov3xWant(t, ap("parent", ent1ID), http.StatusForbidden, "not allowed to approve")
	cov3xWant(t, f.do(http.MethodPost, "/meets/%20/entries/x/approve", "admin", ""), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, ap("admin", "missing"), http.StatusNotFound, "entry_not_found")
	rr = ap("coach", ent1ID)
	cov3xWant(t, rr, http.StatusOK, "approved")
	if e := cov3xJSON(t, rr)["entry"].(map[string]any); e["status"] != "approved" {
		t.Fatalf("not approved: %v", e)
	}
	cov3xWant(t, ap("admin", ent1ID), http.StatusUnprocessableEntity, "entry_not_editable")
	cov3xWant(t, ov("admin", ent1ID, `{"new_meet_event_id":"`+e1+`","reason":"r"}`), http.StatusUnprocessableEntity, "entry_not_editable")

	// scratch
	sc := func(tok, entry, q string) *httptest.ResponseRecorder {
		return f.do(http.MethodDelete, base+"/entries/"+entry+q, tok, "")
	}
	cov3xWant(t, sc("parent", ent1ID, ""), http.StatusForbidden, "not allowed to scratch")
	cov3xWant(t, f.do(http.MethodDelete, "/meets/%20/entries/x", "admin", ""), http.StatusBadRequest, "invalid_id")
	cov3xWant(t, sc("admin", "missing", ""), http.StatusNotFound, "entry_not_found")
	rr = sc("admin", ent1ID, "?reason=%20withdrew%20")
	cov3xWant(t, rr, http.StatusOK, "scratched")
	if e := cov3xJSON(t, rr)["entry"].(map[string]any); e["status"] != "scratched" || e["scratch_reason"] != "withdrew" {
		t.Fatalf("not scratched: %v", e)
	}
	cov3xWant(t, sc("admin", ent1ID, ""), http.StatusConflict, "already_scratched")
	if got, _ := f.audit.Query(audit.Filter{OrgID: cov3xOrg, Action: audit.ActionMeetEntryScratch}); len(got) != 1 {
		t.Fatalf("scratch must be audited once, got %d", len(got))
	}

	// Exported entries are immutable.
	f.svc.SetEntryStatusForTest(ent2ID, "exported")
	cov3xWant(t, sc("admin", ent2ID, ""), http.StatusUnprocessableEntity, "entry_not_editable")

	// After finalization entries are frozen for every mutation.
	f.transition(t, meetID, "entry_finalized")
	cov3xWant(t, sc("admin", ent2ID, ""), http.StatusConflict, "invalid_meet_state")
	cov3xWant(t, ov("admin", ent2ID, `{"new_meet_event_id":"`+e1+`","reason":"late"}`), http.StatusConflict, "invalid_meet_state")
	f.transition(t, meetID, "completed")
	cov3xWant(t, ap("admin", ent2ID), http.StatusConflict, "invalid_meet_state")
}

func TestCov3xMaxEntriesPerSwimmerIsEnforced(t *testing.T) {
	f := newCov3xMeetFixture(t)
	meetID, e1, e2 := f.meetWithEvents(t, `,"signup_settings":{"max_entries_per_swimmer":1}`)
	cm := f.openCommitments(t, meetID)
	base := "/meets/" + meetID
	cov3xWant(t, f.do(http.MethodPost, base+"/commitments/"+cm[cov3xSw2]+"/respond", "parent2", `{"response":"committed_yes"}`), http.StatusOK, "")
	sel := base + "/commitments/" + cm[cov3xSw2] + "/event-selections"
	cov3xWant(t, f.do(http.MethodPost, sel, "parent2", `{"meet_event_id":"`+e1+`"}`), http.StatusCreated, "selected")
	cov3xWant(t, f.do(http.MethodPost, sel, "parent2", `{"meet_event_id":"`+e2+`"}`), http.StatusBadRequest, "selection_failed")
	cov3xWant(t, f.do(http.MethodPost, base+"/lock-commitments", "admin", ""), http.StatusOK, "")
	body := func(ev string) string { return fmt.Sprintf(`{"swimmer_id":%q,"meet_event_id":%q}`, cov3xSw2, ev) }
	// The locked selection already produced the one allowed entry.
	cov3xWant(t, f.do(http.MethodPost, base+"/entries", "admin", body(e1)), http.StatusConflict, "duplicate_entry")
	cov3xWant(t, f.do(http.MethodPost, base+"/entries", "admin", body(e2)), http.StatusBadRequest, "entry_proposal_failed")
}

func TestCov3xWriteMeetsBadRequest(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	writeMeetsBadRequest(rr, req, "lock_failed", fmt.Errorf("%w: pq: connection reset by 10.0.0.5", meets.ErrPersistence))
	cov3xWant(t, rr, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rr.Body.String(), "10.0.0.5") || strings.Contains(rr.Body.String(), "pq:") {
		t.Fatalf("persistence details leaked: %s", rr.Body.String())
	}
	rr = httptest.NewRecorder()
	writeMeetsBadRequest(rr, req, "lock_failed", errors.New("name is required"))
	cov3xWant(t, rr, http.StatusBadRequest, "name is required")
}

func TestCov3xMeetsDatabaseOutageReturns500(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := meets.NewServiceWithPool(pool)
	h := NewMeetsHandlersWithAudit(svc, audit.NewService())
	r := cov3xRouter(h.RegisterRoutes)
	pool.Close()
	assertOutage(t, r)
}

func assertOutage(t *testing.T, r http.Handler) {
	t.Helper()
	const m, c, e, s = "m1", "c1", "e1", "s1"
	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/meets", `{"name":"x"}`},
		{http.MethodGet, "/meets", ""},
		{http.MethodGet, "/meets/" + m, ""},
		{http.MethodPost, "/meets/" + m + "/transition", `{"status":"published"}`},
		{http.MethodPost, "/meets/" + m + "/sessions", `{"session_number":1,"name":"S"}`},
		{http.MethodGet, "/meets/" + m + "/sessions", ""},
		{http.MethodPost, "/meets/" + m + "/events", `{"event_number":"1","stroke":"freestyle","distance":50}`},
		{http.MethodGet, "/meets/" + m + "/events", ""},
		{http.MethodPost, "/meets/" + m + "/commitment-request", ""},
		{http.MethodGet, "/meets/" + m + "/commitments", ""},
		{http.MethodPost, "/meets/" + m + "/commitments/" + c + "/respond", `{"response":"committed_yes"}`},
		{http.MethodPost, "/meets/" + m + "/commitments/" + c + "/event-selections", `{"meet_event_id":"e"}`},
		{http.MethodGet, "/meets/" + m + "/commitments/" + c + "/event-selections", ""},
		{http.MethodDelete, "/meets/" + m + "/commitments/" + c + "/event-selections/" + s, ""},
		{http.MethodPost, "/meets/" + m + "/lock-commitments", ""},
		{http.MethodGet, "/meets/" + m + "/entries", ""},
		{http.MethodPost, "/meets/" + m + "/entries", `{"swimmer_id":"s","meet_event_id":"e"}`},
		{http.MethodPost, "/meets/" + m + "/entries/" + e + "/override", `{"new_meet_event_id":"e","reason":"r"}`},
		{http.MethodPost, "/meets/" + m + "/entries/" + e + "/approve", ""},
		{http.MethodDelete, "/meets/" + m + "/entries/" + e, ""},
	}
	for _, tc := range cases {
		rr := cov3xDo(r, tc.method, tc.path, "admin", tc.body)
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "internal_error") {
			t.Errorf("%s %s: want 500 internal_error, got %d %s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), "closed pool") || strings.Contains(rr.Body.String(), "persist ") {
			t.Errorf("%s %s leaked driver text: %s", tc.method, tc.path, rr.Body.String())
		}
	}
}

var _ = (*pgxpool.Pool)(nil)
