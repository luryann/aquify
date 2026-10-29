package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
)

func TestMeetEntriesSD3ExportFlow(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsSvc := meets.NewService()
	membersSvc := members.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Members: membersSvc,
		Meets:   meetsSvc,
		Exports: exports.NewService(),
	})
	adminToken := issueSessionTokenForUser(t, router, "admin-user", "org-sd3", []string{"admin"}, true)
	coachToken := issueSessionTokenForUser(t, router, "coach-user", "org-sd3", []string{"coach"}, false)
	parentToken := issueSessionTokenForUser(t, router, "parent-user", "org-sd3", []string{"parent"}, false)

	do := func(token, method, path string, body map[string]any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("X-Org-Id", "org-sd3")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	decode := func(rr *httptest.ResponseRecorder, dst any) {
		t.Helper()
		if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
			t.Fatalf("decode %s: %v", rr.Body.String(), err)
		}
	}

	var hh struct {
		Household struct{ ID string } `json:"household"`
	}
	decode(do(adminToken, http.MethodPost, "/api/v1/members/households", map[string]any{"org_id": "org-sd3", "household_name": "Jones"}), &hh)
	var sw struct {
		Swimmer struct{ ID string } `json:"swimmer"`
	}
	decode(do(adminToken, http.MethodPost, "/api/v1/members/swimmers", map[string]any{
		"org_id": "org-sd3", "household_id": hh.Household.ID, "first_name": "Casey", "last_name": "Jones",
		"date_of_birth": "2012-04-02", "gender": "female", "usa_swimming_id": "040212CASJONE",
	}), &sw)

	meetID := createMeet(t, router, "org-sd3", adminToken, "SD3 Invitational")
	eventID := createMeetEvent(t, router, "org-sd3", adminToken, meetID, "7")
	publishMeet(t, router, "org-sd3", adminToken, meetID)

	// Exporting before entries are finalized is rejected.
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/sd3", nil); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("early export: want 422, got %d %s", rr.Code, rr.Body.String())
	}

	var pub commitmentRequestEnvelope
	decode(do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil), &pub)
	if len(pub.Commitments) != 1 {
		t.Fatalf("want 1 commitment, got %d", len(pub.Commitments))
	}
	cmtID := pub.Commitments[0].ID
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/commitments/"+cmtID+"/respond", map[string]any{"response": "committed_yes"}); rr.Code != http.StatusOK {
		t.Fatalf("respond: %d %s", rr.Code, rr.Body.String())
	}
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/commitments/"+cmtID+"/event-selections", map[string]any{"meet_event_id": eventID}); rr.Code != http.StatusCreated {
		t.Fatalf("select: %d %s", rr.Code, rr.Body.String())
	}
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil); rr.Code != http.StatusOK {
		t.Fatalf("lock: %d %s", rr.Code, rr.Body.String())
	}
	var entries struct {
		Entries []struct{ ID, Status string } `json:"entries"`
	}
	decode(do(adminToken, http.MethodGet, "/api/v1/meets/"+meetID+"/entries", nil), &entries)
	if len(entries.Entries) != 1 {
		t.Fatalf("want 1 entry after lock, got %d", len(entries.Entries))
	}
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entries.Entries[0].ID+"/approve", nil); rr.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rr.Code, rr.Body.String())
	}
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/transition", map[string]any{"status": "entry_finalized"}); rr.Code != http.StatusOK {
		t.Fatalf("finalize: %d %s", rr.Code, rr.Body.String())
	}

	if rr := do(parentToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/sd3", nil); rr.Code != http.StatusForbidden {
		t.Fatalf("parent export: want 403, got %d", rr.Code)
	}
	// Without a team code the host cannot import the file.
	if rr := do(coachToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/sd3", nil); rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "team_code_required") {
		t.Fatalf("export without team code: want 422 team_code_required, got %d %s", rr.Code, rr.Body.String())
	}
	if rr := do(adminToken, http.MethodPut, "/api/v1/org-settings", map[string]any{"team_code": "DARE", "team_name": "DARE Aquatics"}); rr.Code != http.StatusOK {
		t.Fatalf("set team code: %d %s", rr.Code, rr.Body.String())
	}

	rr := do(coachToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/sd3", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rr.Code, rr.Body.String())
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "SD3-Invitational") || !strings.HasSuffix(strings.TrimSuffix(cd, `"`), ".sd3") {
		t.Fatalf("unexpected content-disposition %q", cd)
	}
	body := rr.Body.String()
	var d0 string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "D0") {
			d0 = line
		}
	}
	if !strings.HasPrefix(body, "A0") || !strings.Contains(body, "\nC1") || !strings.Contains(body, "DARE Aquatics") ||
		d0 == "" || !strings.Contains(d0, "Jones, Casey") || !strings.Contains(d0, "040212CASJON") {
		t.Fatalf("SD3 missing expected records:\n%s", body)
	}

	decode(do(adminToken, http.MethodGet, "/api/v1/meets/"+meetID+"/entries", nil), &entries)
	if entries.Entries[0].Status != "exported" {
		t.Fatalf("want entry exported, got %q", entries.Entries[0].Status)
	}
	// Re-download keeps working once entries are exported.
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/sd3?variant=standard", nil); rr.Code != http.StatusOK {
		t.Fatalf("re-export: %d %s", rr.Code, rr.Body.String())
	}
	// Exported entries still appear in PDF reports.
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/exports", map[string]any{"report_type": "event_report"}); rr.Code != http.StatusCreated && rr.Code != http.StatusAccepted && rr.Code != http.StatusOK {
		t.Fatalf("report after export: %d %s", rr.Code, rr.Body.String())
	}
}
