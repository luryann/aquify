package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/comms"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/performance"
)

// cov6xPerfDirectory is a static performance.Directory (the true cross-domain
// boundary of the performance service).
type cov6xPerfDirectory struct {
	swimmers []performance.SwimmerProfile
	meets    []performance.MeetInfo
}

func (d cov6xPerfDirectory) SwimmerProfiles(string) []performance.SwimmerProfile { return d.swimmers }
func (d cov6xPerfDirectory) Meets(string) []performance.MeetInfo                 { return d.meets }

type cov6xPerfFixture struct {
	svc    *performance.Service
	comms  *comms.Service
	audit  *audit.Service
	router http.Handler
	// recordEntryID is an accepted team record held by the long-named swimmer.
	recordEntryID string
}

func cov6xNewPerf(t *testing.T, withComms bool) cov6xPerfFixture {
	t.Helper()
	svc := performance.NewService()
	svc.SetDirectory(cov6xPerfDirectory{
		swimmers: []performance.SwimmerProfile{
			{ID: "sw-long", FirstName: "Ada", LastName: strings.Repeat("é", 300), DateOfBirth: "2012-05-01", Gender: "female"},
		},
		meets: []performance.MeetInfo{{ID: "meet-1", Name: "Fall Invite", StartDate: "2026-10-10", Course: "SCY"}},
	})
	f := cov6xPerfFixture{svc: svc, audit: audit.NewService()}
	h := NewPerformanceHandlersWithAudit(svc, f.audit)
	if withComms {
		f.comms = comms.NewService()
		h.SetCommsService(f.comms)
	}
	f.router = cov6xRouter(h.registerExtensionRoutes)

	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: cov6xOrg, Roles: []string{"admin"}, MFAAuthenticated: true}
	fin := 31000
	_, breaks, err := svc.UpsertResultWithRecords(cov6xOrg, admin.UserID, admin, performance.UpsertResultInput{
		SwimmerID: "sw-long", MeetID: "meet-1", Distance: 50, Stroke: "free", Course: "SCY", Gender: "F",
		FinTimeMs: &fin, IsOfficial: true,
	})
	if err != nil || len(breaks) == 0 {
		t.Fatalf("seed result: breaks=%v err=%v", breaks, err)
	}
	f.recordEntryID = breaks[0].EntryID
	return f
}

func TestCov6xWritePerfExtErrorMapsEverySentinel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{performance.ErrForbidden, http.StatusForbidden, "forbidden"},
		{performance.ErrRecordNotFound, http.StatusNotFound, "record_not_found"},
		{performance.ErrRecordNotPending, http.StatusConflict, "record_not_pending"},
		{performance.ErrGoalNotFound, http.StatusNotFound, "goal_not_found"},
		{performance.ErrRelayLineupNotFound, http.StatusNotFound, "relay_lineup_not_found"},
		{performance.ErrRecordInvalid, http.StatusUnprocessableEntity, "validation_error"},
		{performance.ErrGoalInvalid, http.StatusUnprocessableEntity, "validation_error"},
		{performance.ErrRelayInvalid, http.StatusUnprocessableEntity, "validation_error"},
		{errors.New("disk on fire"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		writePerfExtError(rr, httptest.NewRequest(http.MethodGet, "/x", nil), tc.err)
		cov6xWant(t, rr, tc.status, `"`+tc.code+`"`)
		if tc.status == http.StatusInternalServerError && strings.Contains(rr.Body.String(), "disk on fire") {
			t.Errorf("internal error text leaked to client: %s", rr.Body.String())
		}
	}
}

func TestCov6xPerformanceExtensionQueryAndBodyValidation(t *testing.T) {
	t.Parallel()
	f := cov6xNewPerf(t, false)

	// Malformed numeric/boolean query parameters are 400s, never silent filters.
	for _, path := range []string{
		"/performance/records?distance=abc",
		"/performance/records/history?distance=-5",
		"/performance/top-times?distance=x",
		"/performance/top-times?limit=-1",
		"/performance/progress-report?limit=nope",
		"/swimmers/sw-long/goals?include_archived=maybe",
	} {
		rr := cov6xDo(f.router, http.MethodGet, path, "admin", "")
		cov6xWant(t, rr, http.StatusBadRequest, "invalid_query_parameter")
	}

	// A principal with no recognised role is refused by the service layer.
	for _, path := range []string{
		"/performance/records",
		"/performance/records/history?course=SCY&gender=F&age_group=OPEN&distance=50&stroke=free",
		"/performance/top-times?course=SCY",
		"/performance/goals",
		"/performance/progress-report",
	} {
		rr := cov6xDo(f.router, http.MethodGet, path, "stranger", "")
		cov6xWant(t, rr, http.StatusForbidden, `"forbidden"`)
	}

	// Request bodies are validated before reaching the service.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/performance/records", `{"course":"BAD"}`},
		{http.MethodPatch, "/performance/records/x", `{"time_ms":"slow"}`},
		{http.MethodPost, "/performance/records/x/approve", `{"notes":`},
		{http.MethodPost, "/performance/records/x/reject", `{"notes":`},
		{http.MethodPatch, "/swimmers/sw-long/goals/g1", `{"target_time_ms":"fast"}`},
		{http.MethodPost, "/performance/relays/optimize", `{"gender":"Z"}`},
	} {
		rr := cov6xDo(f.router, tc.method, tc.path, "admin", tc.body)
		if rr.Code != http.StatusBadRequest && rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s %s: want 400/422, got %d %s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
	}

	// Unknown record entries surface as 404 through the shared error mapper.
	rr := cov6xDo(f.router, http.MethodPost, "/performance/records/missing/approve", "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "record_not_found")
	// Rejecting an accepted record works once; a second decision is a conflict.
	rr = cov6xDo(f.router, http.MethodPost, "/performance/records/"+f.recordEntryID+"/reject", "admin", `{"notes":"duplicate swim"}`)
	cov6xWant(t, rr, http.StatusOK, `"status":"rejected"`)
	rr = cov6xDo(f.router, http.MethodPost, "/performance/records/"+f.recordEntryID+"/reject", "admin", `{"notes":"again"}`)
	cov6xWant(t, rr, http.StatusConflict, "record_not_pending")
}

func TestCov6xDraftRecordAnnouncement(t *testing.T) {
	t.Parallel()

	t.Run("not configured", func(t *testing.T) {
		t.Parallel()
		f := cov6xNewPerf(t, false)
		rr := cov6xDo(f.router, http.MethodPost, "/performance/records/"+f.recordEntryID+"/announcement", "admin", "")
		cov6xWant(t, rr, http.StatusServiceUnavailable, "service_unavailable")
	})

	t.Run("unknown and forbidden", func(t *testing.T) {
		t.Parallel()
		f := cov6xNewPerf(t, true)
		rr := cov6xDo(f.router, http.MethodPost, "/performance/records/missing/announcement", "admin", "")
		cov6xWant(t, rr, http.StatusNotFound, "record_not_found")
		rr = cov6xDo(f.router, http.MethodPost, "/performance/records/"+f.recordEntryID+"/announcement", "parent", "")
		cov6xWant(t, rr, http.StatusForbidden, "forbidden")
	})

	t.Run("very long swimmer name is truncated by rune", func(t *testing.T) {
		t.Parallel()
		f := cov6xNewPerf(t, true)
		rr := cov6xDo(f.router, http.MethodPost, "/performance/records/"+f.recordEntryID+"/announcement", "coach", "")
		cov6xWant(t, rr, http.StatusCreated, "announcement")
		body := cov6xDecode(t, rr)
		ann := body["announcement"].(map[string]any)
		title := ann["Title"]
		if title == nil {
			title = ann["title"]
		}
		got, _ := title.(string)
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 255 {
			t.Fatalf("title should be cut to 255 whole runes, got %d runes (valid=%v)", utf8.RuneCountInString(got), utf8.ValidString(got))
		}
		if !strings.HasPrefix(got, "New team record: Ada ") {
			t.Fatalf("unexpected title prefix: %q", got[:40])
		}
		events := cov6xAuditActions(t, f.audit)["performance.team_record.announcement_drafted"]
		if len(events) != 1 || events[0].Metadata["record_entry_id"] != f.recordEntryID {
			t.Fatalf("expected one announcement audit event, got %+v", events)
		}
	})
}

func TestCov6xRelayLineupLifecycle(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	a := audit.NewService()
	h := NewPerformanceHandlersWithAudit(svc, a)
	router := cov6xRouter(h.registerExtensionRoutes)

	lineup := func(name string) string {
		legs := func(ids ...string) string {
			parts := make([]string, 0, 4)
			for i, id := range ids {
				parts = append(parts, `{"leg":`+string(rune('1'+i))+`,"swimmer_id":"`+id+`"}`)
			}
			return `[` + strings.Join(parts, ",") + `]`
		}
		return `{"meet_id":"meet-1","name":"` + name + `","course":"SCY","gender":"F","age_group":"OPEN","relay_type":"free","distance":200,` +
			`"teams":[{"legs":` + legs("a", "b", "c", "d") + `}]}`
	}

	rr := cov6xDo(router, http.MethodPost, "/performance/relay-lineups", "parent", lineup("A"))
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")
	rr = cov6xDo(router, http.MethodPost, "/performance/relay-lineups", "admin", `{"meet_id":"meet-1"}`)
	cov6xWant(t, rr, http.StatusUnprocessableEntity, "")

	rr = cov6xDo(router, http.MethodPost, "/performance/relay-lineups", "coach", lineup("Girls A"))
	cov6xWant(t, rr, http.StatusCreated, "Girls A")
	id := cov6xDecode(t, rr)["lineup"].(map[string]any)["id"].(string)

	rr = cov6xDo(router, http.MethodPut, "/performance/relay-lineups/"+id, "coach", lineup("Girls B"))
	cov6xWant(t, rr, http.StatusOK, "Girls B")
	if got := cov6xDecode(t, rr)["lineup"].(map[string]any)["id"]; got != id {
		t.Fatalf("replace must keep the lineup id, got %v want %s", got, id)
	}
	rr = cov6xDo(router, http.MethodPut, "/performance/relay-lineups/missing", "coach", lineup("Z"))
	cov6xWant(t, rr, http.StatusNotFound, "relay_lineup_not_found")

	rr = cov6xDo(router, http.MethodGet, "/performance/relay-lineups?meet_id=meet-1", "coach", "")
	cov6xWant(t, rr, http.StatusOK, `"total":1`)
	rr = cov6xDo(router, http.MethodGet, "/performance/relay-lineups", "parent", "")
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")

	rr = cov6xDo(router, http.MethodDelete, "/performance/relay-lineups/"+id, "parent", "")
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")
	rr = cov6xDo(router, http.MethodDelete, "/performance/relay-lineups/"+id, "coach", "")
	cov6xWant(t, rr, http.StatusOK, `"deleted":true`)
	rr = cov6xDo(router, http.MethodDelete, "/performance/relay-lineups/"+id, "coach", "")
	cov6xWant(t, rr, http.StatusNotFound, "relay_lineup_not_found")

	actions := cov6xAuditActions(t, a)
	if len(actions["performance.relay_lineup.saved"]) != 2 || len(actions["performance.relay_lineup.deleted"]) != 1 {
		t.Fatalf("expected 2 saved + 1 deleted audit events, got %d/%d",
			len(actions["performance.relay_lineup.saved"]), len(actions["performance.relay_lineup.deleted"]))
	}
}

func TestCov6xOptimizeRelayForwardsLocks(t *testing.T) {
	t.Parallel()
	f := cov6xNewPerf(t, false)

	// A lock naming a swimmer who is not in the pool must be reported back by
	// the optimizer, proving the handler forwarded the lock list.
	body := `{"course":"SCY","gender":"F","relay_type":"free","distance":200,"teams":1,"as_of":"2026-10-10",` +
		`"locks":[{"team":0,"leg":1,"swimmer_id":"ghost"}]}`
	rr := cov6xDo(f.router, http.MethodPost, "/performance/relays/optimize", "coach", body)
	cov6xWant(t, rr, http.StatusUnprocessableEntity, "locked swimmer ghost is not eligible")
	rr = cov6xDo(f.router, http.MethodPost, "/performance/relays/optimize", "parent", body)
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")
}
