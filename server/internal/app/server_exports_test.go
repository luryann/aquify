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
		ID         string `json:"id"`
		ExportType string `json:"export_type"`
		Status     string `json:"status"`
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

// minimalExportBody returns a valid generate-export request body as a JSON string.
func minimalExportBody(exportType string) string {
	return `{
		"export_type": "` + exportType + `",
		"meet_name": "Spring Invitational",
		"meet_start_date": "03152026",
		"meet_end_date": "03162026",
		"course": "SCY",
		"host_team_code": "DARE",
		"host_team_name": "DARE Aquatics",
		"entries": [
			{
				"swimmer": {
					"last_name": "Smith",
					"first_name": "John",
					"uss_number": "123456789012",
					"dob": "02151999",
					"gender": "M",
					"team_code": "DARE"
				},
				"event_number": "1",
				"distance": 100,
				"stroke": "free",
				"event_gender": "M",
				"age_group": "1824",
				"course": "SCY",
				"seed_time_ms": 59100
			}
		]
	}`
}

func doGenerateExport(t *testing.T, router http.Handler, orgID, token, meetID, exportType string) exportEnvelope {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/exports",
		strings.NewReader(minimalExportBody(exportType)))
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

func TestGenerateExport_StandardSD3(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-std"
	token := issueSessionToken(t, router, orgID, []string{"coach"}, false)

	env := doGenerateExport(t, router, orgID, token, "meet-001", "standard_sd3")

	if env.Export.ExportType != "standard_sd3" {
		t.Errorf("export_type = %q; want standard_sd3", env.Export.ExportType)
	}
	if env.Export.Status != "ready" {
		t.Errorf("status = %q; want ready", env.Export.Status)
	}
	if env.Export.SizeBytes <= 0 {
		t.Errorf("size_bytes = %d; want > 0", env.Export.SizeBytes)
	}
	if !strings.HasSuffix(env.Export.Filename, ".sd3") {
		t.Errorf("filename = %q; want .sd3 suffix", env.Export.Filename)
	}
	if env.AuditEvent.Action != "export.requested" {
		t.Errorf("audit action = %q; want export.requested", env.AuditEvent.Action)
	}
}

func TestGenerateExport_ExtendedSD3(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-ext"
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	env := doGenerateExport(t, router, orgID, token, "meet-002", "extended_sd3")

	if env.Export.ExportType != "extended_sd3" {
		t.Errorf("export_type = %q; want extended_sd3", env.Export.ExportType)
	}
	if env.Export.Status != "ready" {
		t.Errorf("status = %q; want ready", env.Export.Status)
	}
}

func TestGenerateExport_SDIFRoster(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-roster"
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	env := doGenerateExport(t, router, orgID, token, "meet-003", "sdif_roster")

	if env.Export.ExportType != "sdif_roster" {
		t.Errorf("export_type = %q; want sdif_roster", env.Export.ExportType)
	}
	if env.Export.Status != "ready" {
		t.Errorf("status = %q; want ready", env.Export.Status)
	}
}

func TestGenerateExport_ParentForbidden(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-perm"
	parentToken := issueSessionToken(t, router, orgID, []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/m1/exports",
		strings.NewReader(minimalExportBody("standard_sd3")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for parent role, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGenerateExport_InvalidExportType(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-invalid"
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/m1/exports",
		strings.NewReader(`{
			"export_type": "hy3_format",
			"meet_name": "Test Meet"
		}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid export type, got %d", rr.Code)
	}
}

func TestListMeetExports(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-list"
	token := issueSessionToken(t, router, orgID, []string{"coach"}, false)
	meetID := "meet-list-001"

	// Generate two exports
	doGenerateExport(t, router, orgID, token, meetID, "standard_sd3")
	doGenerateExport(t, router, orgID, token, meetID, "extended_sd3")

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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-get"
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	env := doGenerateExport(t, router, orgID, token, "meet-get-001", "standard_sd3")
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
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-dl"
	token := issueSessionToken(t, router, orgID, []string{"coach"}, false)

	env := doGenerateExport(t, router, orgID, token, "meet-dl-001", "extended_sd3")
	exportID := env.Export.ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+exportID+"/download", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if len(body) == 0 {
		t.Fatal("expected non-empty file content in download response")
	}
	// Verify the response contains valid SD3 markers
	if !strings.Contains(body, "A0") {
		t.Error("expected A0 record in downloaded SD3 file")
	}
	if !strings.Contains(body, "B1") {
		t.Error("expected B1 meet record in downloaded SD3 file")
	}
	if !strings.Contains(body, "Z0") {
		t.Error("expected Z0 terminator in downloaded SD3 file")
	}

	// Check Content-Disposition header
	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q; want attachment", cd)
	}
	if !strings.Contains(cd, ".sd3") {
		t.Errorf("Content-Disposition = %q; want .sd3 filename", cd)
	}
}

func TestDownloadExport_OrgIsolation(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil, nil)

	orgA := "org-export-iso-a"
	tokenA := issueSessionToken(t, router, orgA, []string{"admin"}, false)

	orgB := "org-export-iso-b"
	tokenB := issueSessionToken(t, router, orgB, []string{"admin"}, false)

	// Org A generates an export
	envA := doGenerateExport(t, router, orgA, tokenA, "meet-iso-001", "standard_sd3")
	exportID := envA.Export.ID

	// Org B cannot access org A's export
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+exportID+"/download", nil)
	req.Header.Set("X-Org-Id", orgB)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for cross-org access, got %d", rr.Code)
	}
}

func TestGenerateExport_UnauthenticatedRejected(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/m1/exports",
		strings.NewReader(minimalExportBody("standard_sd3")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-any")
	// No Authorization header
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated request, got %d", rr.Code)
	}
}

func TestDownloadExport_ExtendedSD3_ContainsSeedTime(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil, nil)

	orgID := "org-export-seed"
	token := issueSessionToken(t, router, orgID, []string{"coach"}, false)

	env := doGenerateExport(t, router, orgID, token, "meet-seed-001", "extended_sd3")
	exportID := env.Export.ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+exportID+"/download", nil)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	// The seed time 59100ms = 59.10s → should appear as "  59.10 " in D0 record at col 89-96
	body := rr.Body.String()
	if !strings.Contains(body, "D0") {
		t.Error("expected D0 record in extended SD3 output")
	}
	// Extended export should have a seed time (not all blank or NT in col 89-96)
	// Loosely check: the string "59.10" should appear in the output
	if !strings.Contains(body, "59.10") {
		snippet := body
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		t.Errorf("expected seed time '59.10' in extended SD3 output; body snippet: %q", snippet)
	}
}
