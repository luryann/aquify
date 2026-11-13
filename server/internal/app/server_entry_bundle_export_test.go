package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/app/exports"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
)

func TestMeetEntryBundleExportFlow(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsSvc := meets.NewService()
	membersSvc := members.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Members: membersSvc,
		Meets:   meetsSvc,
		Exports: exports.NewService(),
	})
	adminToken := issueSessionTokenForUser(t, router, "admin-user", "org-bundle", []string{"admin"}, true)
	coachToken := issueSessionTokenForUser(t, router, "coach-user", "org-bundle", []string{"coach"}, false)
	parentToken := issueSessionTokenForUser(t, router, "parent-user", "org-bundle", []string{"parent"}, false)

	do := func(token, method, path string, body map[string]any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("X-Org-Id", "org-bundle")
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
	decode(do(adminToken, http.MethodPost, "/api/v1/members/households", map[string]any{"org_id": "org-bundle", "household_name": "Jones"}), &hh)
	var sw struct {
		Swimmer struct{ ID string } `json:"swimmer"`
	}
	decode(do(adminToken, http.MethodPost, "/api/v1/members/swimmers", map[string]any{
		"org_id": "org-bundle", "household_id": hh.Household.ID, "first_name": "Casey", "last_name": "Jones",
		"date_of_birth": "2012-04-02", "gender": "female", "usa_swimming_id": "040212CASJONE",
	}), &sw)

	var created meetEnvelope
	decode(do(adminToken, http.MethodPost, "/api/v1/meets", map[string]any{
		"name": "Bundle Invitational", "meet_type": "invitational", "course": "SCY",
		"start_date": "2027-01-16", "end_date": "2027-01-17", "location": "Example Aquatic Center",
	}), &created)
	meetID := created.Meet.ID
	eventID := createMeetEvent(t, router, "org-bundle", adminToken, meetID, "7")
	publishMeet(t, router, "org-bundle", adminToken, meetID)

	// Exporting before entries are finalized is rejected.
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/export", nil); rr.Code != http.StatusUnprocessableEntity {
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

	if rr := do(parentToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/export", nil); rr.Code != http.StatusForbidden {
		t.Fatalf("parent export: want 403, got %d", rr.Code)
	}
	// Without a team code the host cannot import the file.
	if rr := do(coachToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/export", nil); rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "team_code_required") {
		t.Fatalf("export without team code: want 422 team_code_required, got %d %s", rr.Code, rr.Body.String())
	}
	if rr := do(adminToken, http.MethodPut, "/api/v1/org-settings", map[string]any{"team_code": "aquify", "team_name": "aquify Aquatics"}); rr.Code != http.StatusOK {
		t.Fatalf("set team code: %d %s", rr.Code, rr.Body.String())
	}

	rr := do(coachToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/export", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rr.Code, rr.Body.String())
	}
	cd := rr.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, `attachment; filename="AQUIFY-Entries-Bundle Invitational-`) || !strings.HasSuffix(cd, `.zip"`) {
		t.Fatalf("unexpected content-disposition %q", cd)
	}
	base := strings.TrimSuffix(strings.TrimPrefix(cd, `attachment; filename="`), `.zip"`)
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("export is not a zip: %v", err)
	}
	files := map[string]string{}
	var names []string
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = string(data)
		names = append(names, f.Name)
	}
	want := []string{base + ".sd3", base + ".hy3", base + "-EntryReport-ByEvent.pdf", base + "-EntryReport-BySwimmer.pdf", base + "-EntryLog.txt"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("bundle files = %q", names)
	}
	sd3, hy3 := files[base+".sd3"], files[base+".hy3"]
	if !strings.HasPrefix(sd3, "A01V3") || !strings.Contains(sd3, "aquify Aquatics") || !strings.Contains(sd3, "D01        Jones, Casey") || !strings.Contains(sd3, "040212CASJON") {
		t.Fatalf("SD3 missing expected records:\n%s", sd3)
	}
	if !strings.HasPrefix(hy3, "A102Meet Entries") || !strings.Contains(hy3, "D1F    1Jones               Casey") || !strings.Contains(hy3, "\r\nE1F    1Jones") {
		t.Fatalf("HY3 missing expected records:\n%s", hy3)
	}
	if log := files[base+"-EntryLog.txt"]; !strings.Contains(log, "Athletes:         1") || !strings.Contains(log, "Individual Entries: 1") {
		t.Fatalf("entry log:\n%s", log)
	}
	if !strings.Contains(files[base+"-EntryReport-BySwimmer.pdf"], "(Casey Jones) Tj") {
		t.Fatal("by-swimmer report does not list the swimmer")
	}

	decode(do(adminToken, http.MethodGet, "/api/v1/meets/"+meetID+"/entries", nil), &entries)
	if entries.Entries[0].Status != "exported" {
		t.Fatalf("want entry exported, got %q", entries.Entries[0].Status)
	}
	// Re-download keeps working once entries are exported.
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/entries/export", nil); rr.Code != http.StatusOK {
		t.Fatalf("re-export: %d %s", rr.Code, rr.Body.String())
	}
	// Exported entries still appear in PDF reports.
	if rr := do(adminToken, http.MethodPost, "/api/v1/meets/"+meetID+"/exports", map[string]any{"report_type": "event_report"}); rr.Code != http.StatusCreated && rr.Code != http.StatusAccepted && rr.Code != http.StatusOK {
		t.Fatalf("report after export: %d %s", rr.Code, rr.Body.String())
	}
}
