package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
)

// ─── Envelope types ───────────────────────────────────────────────────────────

type exportEnvelope struct {
	Export struct {
		ID         string `json:"id"`
		MeetID     string `json:"meet_id"`
		ExportType string `json:"export_type"`
		Status     string `json:"status"`
		Filename   string `json:"filename"`
		SizeBytes  int    `json:"size_bytes"`
	} `json:"export"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type exportListEnvelope struct {
	Count   int `json:"count"`
	Exports []struct {
		ID          string `json:"id"`
		ExportType  string `json:"export_type"`
		Status      string `json:"status"`
		DownloadURL string `json:"download_url"`
	} `json:"exports"`
}

type exportGetEnvelope struct {
	Export struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		SizeBytes int    `json:"size_bytes"`
	} `json:"export"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// reportRequestBody returns a valid generate-report request body. All report
// data is assembled server-side from the meet, so the client sends only the
// report type (and optional grouping for entry reports).
func reportRequestBody(reportType string) string {
	return `{"report_type": "` + reportType + `"}`
}

// newExportsReportRouter wires the services the report endpoint needs
// (meets + members + exports) and seeds one meet, returning the router, a token
// for the given roles, and the seeded meet ID.
func newExportsReportRouter(t *testing.T, orgID string, roles []string) (http.Handler, string, string) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Members: members.NewService(),
		Meets:   meets.NewService(),
		Exports: exports.NewService(),
	})
	token := issueSessionToken(t, router, orgID, roles, false)
	meetID := createMeet(t, router, orgID, token, "Spring Invitational")
	return router, token, meetID
}

func doGenerateExport(t *testing.T, router http.Handler, orgID, token, meetID, reportType string) exportEnvelope {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/exports",
		strings.NewReader(reportRequestBody(reportType)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("generateExport: expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	var env exportEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode generateExport response: %v", err)
	}
	if env.Export.ID == "" {
		t.Fatal("expected export ID in response")
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+env.Export.ID, nil)
		req.Header.Set("X-Org-Id", orgID)
		req.Header.Set("Authorization", "Bearer "+token)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code == http.StatusOK {
			var getEnv exportGetEnvelope
			if err := json.Unmarshal(rr.Body.Bytes(), &getEnv); err == nil && getEnv.Export.Status == "ready" {
				env.Export.Status = getEnv.Export.Status
				env.Export.SizeBytes = getEnv.Export.SizeBytes
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return env
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestGenerateReport_EventReport(t *testing.T) {
	t.Parallel()

	orgID := "org-export-event"
	router, token, meetID := newExportsReportRouter(t, orgID, []string{"coach"})

	env := doGenerateExport(t, router, orgID, token, meetID, "event_report")

	if env.Export.ExportType != "event_report" {
		t.Errorf("export_type = %q; want event_report", env.Export.ExportType)
	}
	if env.Export.Status != "ready" {
		t.Errorf("status = %q; want ready", env.Export.Status)
	}
	if env.Export.SizeBytes <= 0 {
		t.Errorf("size_bytes = %d; want > 0", env.Export.SizeBytes)
	}
	if !strings.HasSuffix(env.Export.Filename, ".pdf") {
		t.Errorf("filename = %q; want .pdf suffix", env.Export.Filename)
	}
	if env.AuditEvent.Action != "report.generated" {
		t.Errorf("audit action = %q; want report.generated", env.AuditEvent.Action)
	}
}

func TestGenerateReport_EntryReport(t *testing.T) {
	t.Parallel()

	orgID := "org-export-entry"
	router, token, meetID := newExportsReportRouter(t, orgID, []string{"admin"})

	env := doGenerateExport(t, router, orgID, token, meetID, "entry_report")

	if env.Export.ExportType != "entry_report" {
		t.Errorf("export_type = %q; want entry_report", env.Export.ExportType)
	}
	if env.Export.Status != "ready" {
		t.Errorf("status = %q; want ready", env.Export.Status)
	}
}

func TestGenerateReport_EligibilityReport(t *testing.T) {
	t.Parallel()

	orgID := "org-export-elig"
	router, token, meetID := newExportsReportRouter(t, orgID, []string{"admin"})

	env := doGenerateExport(t, router, orgID, token, meetID, "eligibility_report")

	if env.Export.ExportType != "eligibility_report" {
		t.Errorf("export_type = %q; want eligibility_report", env.Export.ExportType)
	}
	if env.Export.Status != "ready" {
		t.Errorf("status = %q; want ready", env.Export.Status)
	}
}

func TestGenerateReport_EntryReportGrouping(t *testing.T) {
	t.Parallel()

	orgID := "org-export-grouping"
	router, token, meetID := newExportsReportRouter(t, orgID, []string{"coach"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/exports",
		strings.NewReader(`{"report_type": "entry_report", "grouping": "session"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for grouped entry report, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGenerateReport_ParentForbidden(t *testing.T) {
	t.Parallel()

	orgID := "org-export-perm"
	router, _, meetID := newExportsReportRouter(t, orgID, []string{"admin"})
	parentToken := issueSessionToken(t, router, orgID, []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/exports",
		strings.NewReader(reportRequestBody("event_report")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for parent role, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGenerateReport_InvalidReportType(t *testing.T) {
	t.Parallel()

	orgID := "org-export-invalid"
	router, token, meetID := newExportsReportRouter(t, orgID, []string{"admin"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/exports",
		strings.NewReader(`{"report_type": "hy3_format"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid report type, got %d", rr.Code)
	}
}

func TestListMeetExports(t *testing.T) {
	t.Parallel()

	orgID := "org-export-list"
	router, token, meetID := newExportsReportRouter(t, orgID, []string{"coach"})

	// Generate two reports
	doGenerateExport(t, router, orgID, token, meetID, "event_report")
	doGenerateExport(t, router, orgID, token, meetID, "entry_report")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/exports", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var env exportListEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if env.Count != 2 {
		t.Errorf("count = %d; want 2", env.Count)
	}
}

func TestGetExport(t *testing.T) {
	t.Parallel()

	orgID := "org-export-get"
	router, token, meetID := newExportsReportRouter(t, orgID, []string{"admin"})

	env := doGenerateExport(t, router, orgID, token, meetID, "event_report")
	exportID := env.Export.ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+exportID, nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var getEnv exportGetEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &getEnv); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getEnv.Export.ID != exportID {
		t.Errorf("id = %q; want %q", getEnv.Export.ID, exportID)
	}
	if getEnv.Export.Status != "ready" {
		t.Errorf("status = %q; want ready", getEnv.Export.Status)
	}
}

func TestGetExport_NotFound(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, RouterDeps{})

	orgID := "org-export-404"
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/export_doesnotexist", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDownloadExport(t *testing.T) {
	t.Parallel()

	orgID := "org-export-dl"
	router, token, meetID := newExportsReportRouter(t, orgID, []string{"coach"})

	env := doGenerateExport(t, router, orgID, token, meetID, "event_report")
	exportID := env.Export.ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+exportID+"/download", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	body := rr.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("expected non-empty file content in download response")
	}
	// Downloaded artifact must be a PDF.
	if !strings.HasPrefix(string(body), "%PDF-") {
		t.Errorf("expected PDF content, got prefix %q", string(body[:min(8, len(body))]))
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q; want application/pdf", ct)
	}
	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q; want attachment", cd)
	}
	if !strings.Contains(cd, ".pdf") {
		t.Errorf("Content-Disposition = %q; want .pdf filename", cd)
	}
}

func TestDownloadExport_OrgIsolation(t *testing.T) {
	t.Parallel()

	orgA := "org-export-iso-a"
	router, tokenA, meetID := newExportsReportRouter(t, orgA, []string{"admin"})

	orgB := "org-export-iso-b"
	tokenB := issueSessionToken(t, router, orgB, []string{"admin"}, false)

	// Org A generates a report
	envA := doGenerateExport(t, router, orgA, tokenA, meetID, "event_report")
	exportID := envA.Export.ID

	// Org B cannot access org A's report
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+exportID+"/download", nil)
	req.Header.Set("X-Org-Id", orgB)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for cross-org access, got %d", rr.Code)
	}
}

func TestGenerateReport_UnauthenticatedRejected(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, RouterDeps{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/m1/exports",
		strings.NewReader(reportRequestBody("event_report")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-any")
	// No Authorization header
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated request, got %d", rr.Code)
	}
}

func TestListMeetExports_ParentHouseholdCanAccessMeetFiles(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsSvc := meets.NewService()
	exportsSvc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets:   meetsSvc,
		Exports: exportsSvc,
	})

	orgID := "org-export-parent-files"
	adminToken := issueSessionToken(t, router, orgID, []string{"admin"}, false)
	parentToken := issueSessionTokenWithHousehold(t, router, "parent-files", orgID, "household-a", []string{"parent"}, false)

	meetsSvc.SeedSwimmer(orgID, "swimmer-a", "household-a")
	meetsSvc.SeedParentHousehold("parent-files", "household-a")

	meetID := createMeet(t, router, orgID, adminToken, "Family Files Meet")
	publishMeet(t, router, orgID, adminToken, meetID)

	publishCommitmentsReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	publishCommitmentsReq.Header.Set("X-Org-Id", orgID)
	publishCommitmentsReq.Header.Set("Authorization", "Bearer "+adminToken)
	publishCommitmentsRes := httptest.NewRecorder()
	router.ServeHTTP(publishCommitmentsRes, publishCommitmentsReq)
	if publishCommitmentsRes.Code != http.StatusOK {
		t.Fatalf("publish commitment request: expected 200, got %d body=%s", publishCommitmentsRes.Code, publishCommitmentsRes.Body.String())
	}

	doGenerateExport(t, router, orgID, adminToken, meetID, "event_report")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/exports", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for parent file listing, got %d body=%s", rr.Code, rr.Body.String())
	}

	var env exportListEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if env.Count != 1 || len(env.Exports) != 1 {
		t.Fatalf("expected 1 meet file, got count=%d len=%d", env.Count, len(env.Exports))
	}
	if env.Exports[0].DownloadURL == "" {
		t.Fatal("expected parent-scoped meet file listing to include a download URL")
	}
}

func TestListMeetExports_ParentCrossHouseholdForbidden(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsSvc := meets.NewService()
	exportsSvc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets:   meetsSvc,
		Exports: exportsSvc,
	})

	orgID := "org-export-parent-forbidden"
	adminToken := issueSessionToken(t, router, orgID, []string{"admin"}, false)
	parentToken := issueSessionTokenWithHousehold(t, router, "parent-other", orgID, "household-b", []string{"parent"}, false)

	meetsSvc.SeedSwimmer(orgID, "swimmer-a", "household-a")
	meetID := createMeet(t, router, orgID, adminToken, "Restricted Meet")
	publishMeet(t, router, orgID, adminToken, meetID)

	publishCommitmentsReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	publishCommitmentsReq.Header.Set("X-Org-Id", orgID)
	publishCommitmentsReq.Header.Set("Authorization", "Bearer "+adminToken)
	publishCommitmentsRes := httptest.NewRecorder()
	router.ServeHTTP(publishCommitmentsRes, publishCommitmentsReq)
	if publishCommitmentsRes.Code != http.StatusOK {
		t.Fatalf("publish commitment request: expected 200, got %d body=%s", publishCommitmentsRes.Code, publishCommitmentsRes.Body.String())
	}

	doGenerateExport(t, router, orgID, adminToken, meetID, "event_report")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/exports", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-household parent file listing, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestListMeetExports_SwimmerHouseholdCanAccessMeetFiles(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsSvc := meets.NewService()
	exportsSvc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets:   meetsSvc,
		Exports: exportsSvc,
	})

	orgID := "org-export-swimmer-files"
	adminToken := issueSessionToken(t, router, orgID, []string{"admin"}, false)
	swimmerToken := issueSessionTokenWithHousehold(t, router, "swimmer-files", orgID, "household-a", []string{"swimmer"}, false)

	meetsSvc.SeedSwimmer(orgID, "swimmer-a", "household-a")
	meetID := createMeet(t, router, orgID, adminToken, "Swimmer Files Meet")
	publishMeet(t, router, orgID, adminToken, meetID)

	publishCommitmentsReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	publishCommitmentsReq.Header.Set("X-Org-Id", orgID)
	publishCommitmentsReq.Header.Set("Authorization", "Bearer "+adminToken)
	publishCommitmentsRes := httptest.NewRecorder()
	router.ServeHTTP(publishCommitmentsRes, publishCommitmentsReq)
	if publishCommitmentsRes.Code != http.StatusOK {
		t.Fatalf("publish commitment request: expected 200, got %d body=%s", publishCommitmentsRes.Code, publishCommitmentsRes.Body.String())
	}

	doGenerateExport(t, router, orgID, adminToken, meetID, "event_report")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/exports", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+swimmerToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for swimmer file listing, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestListMeetExports_SwimmerCrossHouseholdForbidden(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsSvc := meets.NewService()
	exportsSvc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets:   meetsSvc,
		Exports: exportsSvc,
	})

	orgID := "org-export-swimmer-forbidden"
	adminToken := issueSessionToken(t, router, orgID, []string{"admin"}, false)
	swimmerToken := issueSessionTokenWithHousehold(t, router, "swimmer-other", orgID, "household-b", []string{"swimmer"}, false)

	meetsSvc.SeedSwimmer(orgID, "swimmer-a", "household-a")
	meetID := createMeet(t, router, orgID, adminToken, "Restricted Swimmer Meet")
	publishMeet(t, router, orgID, adminToken, meetID)

	publishCommitmentsReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	publishCommitmentsReq.Header.Set("X-Org-Id", orgID)
	publishCommitmentsReq.Header.Set("Authorization", "Bearer "+adminToken)
	publishCommitmentsRes := httptest.NewRecorder()
	router.ServeHTTP(publishCommitmentsRes, publishCommitmentsReq)
	if publishCommitmentsRes.Code != http.StatusOK {
		t.Fatalf("publish commitment request: expected 200, got %d body=%s", publishCommitmentsRes.Code, publishCommitmentsRes.Body.String())
	}

	doGenerateExport(t, router, orgID, adminToken, meetID, "event_report")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/exports", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+swimmerToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-household swimmer file listing, got %d body=%s", rr.Code, rr.Body.String())
	}
}
