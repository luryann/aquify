package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/imports"
)

type malwareScannerFunc func(storageKey string, metadata imports.FileUploadMetadata) imports.MalwareScanResult

func (fn malwareScannerFunc) Scan(storageKey string, metadata imports.FileUploadMetadata) imports.MalwareScanResult {
	return fn(storageKey, metadata)
}

// ─── Envelope helpers ─────────────────────────────────────────────────────────

type importEnvelope struct {
	Import struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		FileType  string `json:"file_type"`
		UploadURL string `json:"upload_url"`
	} `json:"import"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type importListEnvelope struct {
	Count   int `json:"count"`
	Imports []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"imports"`
}

type conflictListEnvelope struct {
	Count     int `json:"count"`
	Conflicts []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"conflicts"`
}

type conflictEnvelope struct {
	Conflict struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"conflict"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type importReportEnvelope struct {
	Import struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"import"`
	Report *struct {
		TotalRows    int `json:"total_rows"`
		MatchedRows  int `json:"matched_rows"`
		ErrorRows    int `json:"error_rows"`
		ConflictRows int `json:"conflict_rows"`
	} `json:"report"`
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func requestUploadURL(t *testing.T, router http.Handler, orgID, token, filename, fileType string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/upload-url",
		encodeJSONBody(t, map[string]any{
			"filename":   filename,
			"file_type":  fileType,
			"size_bytes": 1024,
			"mime_type":  importMIMEFor(filename, fileType),
		}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected upload-url status 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	var env importEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode upload-url response: %v", err)
	}
	if env.Import.ID == "" {
		t.Fatal("expected import ID in upload-url response")
	}
	return env.Import.ID
}

func importMIMEFor(filename, fileType string) string {
	switch fileType {
	case "zip", "hy3":
		return "application/zip"
	case "cl2", "sd3":
		return "application/octet-stream"
	}
	switch {
	case len(filename) >= 4 && filename[len(filename)-4:] == ".zip":
		return "application/zip"
	case len(filename) >= 4 && filename[len(filename)-4:] == ".hy3":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestRequestUploadURL(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-import-test", []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/upload-url",
		encodeJSONBody(t, map[string]any{
			"filename":   "results.sd3",
			"file_type":  "sd3",
			"size_bytes": 1024,
			"mime_type":  "application/octet-stream",
		}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-import-test")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	var env importEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Import.ID == "" {
		t.Fatal("expected import ID in response")
	}
	if env.Import.Status != "uploaded" {
		t.Fatalf("expected status 'uploaded', got %q", env.Import.Status)
	}
	if env.Import.UploadURL == "" {
		t.Fatal("expected upload_url in response")
	}
	if env.Import.FileType != "sd3" {
		t.Fatalf("expected file_type 'sd3', got %q", env.Import.FileType)
	}
	if env.AuditEvent.Action != "import.upload_url_requested" {
		t.Fatalf("expected audit action import.upload_url_requested, got %q", env.AuditEvent.Action)
	}
}

func TestRequestUploadURL_FileTypeInferredFromFilename(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-x", []string{"admin"}, false)

	// No file_type field — inferred from filename.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/upload-url",
		encodeJSONBody(t, map[string]any{
			"filename":   "meet_results.zip",
			"size_bytes": 2048,
			"mime_type":  "application/zip",
		}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-x")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	var env importEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Import.FileType != "zip" {
		t.Fatalf("expected inferred file_type 'zip', got %q", env.Import.FileType)
	}
}

func TestRequestUploadURL_Forbidden_Parent(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	parentToken := issueSessionToken(t, router, "org-x", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/upload-url",
		encodeJSONBody(t, map[string]any{
			"filename":   "results.sd3",
			"file_type":  "sd3",
			"size_bytes": 1024,
			"mime_type":  "application/octet-stream",
		}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-x")
	req.Header.Set("Authorization", "Bearer "+parentToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestFinalizeImport(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-import-test", []string{"admin"}, false)

	importID := requestUploadURL(t, router, "org-import-test", adminToken, "results.cl2", "cl2")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	req.Header.Set("X-Org-Id", "org-import-test")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var env importEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Import.Status != "conflicts_pending" {
		t.Fatalf("expected status 'conflicts_pending', got %q", env.Import.Status)
	}
	if env.AuditEvent.Action != "import.finalized" {
		t.Fatalf("expected audit action import.finalized, got %q", env.AuditEvent.Action)
	}
}

func TestFinalizeImport_AlreadyFinalized(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-x", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-x", adminToken, "data.sd3", "sd3")

	// Finalize once.
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	req1.Header.Set("X-Org-Id", "org-x")
	req1.Header.Set("Authorization", "Bearer "+adminToken)
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first finalize: expected 200, got %d", rr1.Code)
	}

	// Finalize again — should conflict.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	req2.Header.Set("X-Org-Id", "org-x")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("second finalize: expected 409, got %d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestListImports(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-list", []string{"admin"}, false)

	// Initially empty.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/imports", nil)
	req.Header.Set("X-Org-Id", "org-list")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var env importListEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 0 {
		t.Fatalf("expected 0 imports, got %d", env.Count)
	}

	// Create one import.
	requestUploadURL(t, router, "org-list", adminToken, "data.zip", "zip")

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/imports", nil)
	req2.Header.Set("X-Org-Id", "org-list")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr2.Code, rr2.Body.String())
	}
	var env2 importListEnvelope
	if err := json.Unmarshal(rr2.Body.Bytes(), &env2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env2.Count != 1 {
		t.Fatalf("expected 1 import, got %d", env2.Count)
	}
}

func TestGetImport(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-get", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-get", adminToken, "results.sd3", "sd3")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+importID, nil)
	req.Header.Set("X-Org-Id", "org-get")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var env importEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Import.ID != importID {
		t.Fatalf("expected import ID %q, got %q", importID, env.Import.ID)
	}
}

func TestListConflicts(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-conflicts", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-conflicts", adminToken, "data.cl2", "cl2")

	// Before finalize: no conflicts.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+importID+"/conflicts", nil)
	req.Header.Set("X-Org-Id", "org-conflicts")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var env conflictListEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 0 {
		t.Fatalf("expected 0 conflicts before finalize, got %d", env.Count)
	}

	// Finalize — stub seeds one conflict.
	finalReq := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	finalReq.Header.Set("X-Org-Id", "org-conflicts")
	finalReq.Header.Set("Authorization", "Bearer "+adminToken)
	finalRR := httptest.NewRecorder()
	router.ServeHTTP(finalRR, finalReq)
	if finalRR.Code != http.StatusOK {
		t.Fatalf("finalize: expected 200, got %d", finalRR.Code)
	}

	// Now one conflict should appear.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+importID+"/conflicts", nil)
	req2.Header.Set("X-Org-Id", "org-conflicts")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}
	var env2 conflictListEnvelope
	if err := json.Unmarshal(rr2.Body.Bytes(), &env2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env2.Count != 1 {
		t.Fatalf("expected 1 conflict after finalize, got %d", env2.Count)
	}
	if env2.Conflicts[0].Status != "open" {
		t.Fatalf("expected conflict status 'open', got %q", env2.Conflicts[0].Status)
	}
}

func TestResolveConflict(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-resolve", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-resolve", adminToken, "data.zip", "zip")

	// Finalize to get a conflict.
	finalReq := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	finalReq.Header.Set("X-Org-Id", "org-resolve")
	finalReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), finalReq)

	// Get conflict ID.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+importID+"/conflicts", nil)
	listReq.Header.Set("X-Org-Id", "org-resolve")
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	var listEnv conflictListEnvelope
	if err := json.Unmarshal(listRR.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listEnv.Conflicts) == 0 {
		t.Fatal("expected at least one conflict")
	}
	conflictID := listEnv.Conflicts[0].ID

	// Resolve the conflict.
	resolveReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/imports/"+importID+"/conflicts/"+conflictID+"/resolve",
		encodeJSONBody(t, map[string]any{
			"resolved_swimmer_id": "swm-abc-123",
			"resolution_note":     "Manual match confirmed by coach",
		}))
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveReq.Header.Set("X-Org-Id", "org-resolve")
	resolveReq.Header.Set("Authorization", "Bearer "+adminToken)
	resolveRR := httptest.NewRecorder()
	router.ServeHTTP(resolveRR, resolveReq)

	if resolveRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resolveRR.Code, resolveRR.Body.String())
	}
	var env conflictEnvelope
	if err := json.Unmarshal(resolveRR.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Conflict.Status != "resolved_manual" {
		t.Fatalf("expected status 'resolved_manual', got %q", env.Conflict.Status)
	}
	if env.AuditEvent.Action != "import.conflict_resolved" {
		t.Fatalf("expected audit action import.conflict_resolved, got %q", env.AuditEvent.Action)
	}
}

func TestResolveConflict_Forbidden_Coach(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-x", []string{"admin"}, false)
	coachToken := issueSessionToken(t, router, "org-x", []string{"coach"}, false)

	importID := requestUploadURL(t, router, "org-x", adminToken, "data.sd3", "sd3")

	// Finalize.
	finalReq := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	finalReq.Header.Set("X-Org-Id", "org-x")
	finalReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), finalReq)

	// Try to resolve as coach — should be forbidden.
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/imports/"+importID+"/conflicts/fake-conflict-id/resolve",
		encodeJSONBody(t, map[string]any{
			"resolved_swimmer_id": "swm-abc",
		}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-x")
	req.Header.Set("Authorization", "Bearer "+coachToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestGetImportReport(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-report", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-report", adminToken, "data.cl2", "cl2")

	// Finalize so report is populated.
	finalReq := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	finalReq.Header.Set("X-Org-Id", "org-report")
	finalReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), finalReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+importID+"/report", nil)
	req.Header.Set("X-Org-Id", "org-report")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var env importReportEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Report == nil {
		t.Fatal("expected report to be populated after finalize")
	}
	if env.Report.TotalRows != 10 {
		t.Fatalf("expected total_rows 10, got %d", env.Report.TotalRows)
	}
}

func TestGetImport_NotFound(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-x", []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/nonexistent-id", nil)
	req.Header.Set("X-Org-Id", "org-x")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestRequestUploadURL_UnsupportedType(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-x", []string{"admin"}, false)

	// filename has no recognized extension and no file_type given.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/upload-url",
		encodeJSONBody(t, map[string]any{
			"filename":   "something.csv",
			"size_bytes": 512,
			"mime_type":  "text/csv",
		}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-x")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported type, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFinalizeImportRejectsRequiredModeWithoutScanner(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	importsService.ConfigureSecurity(imports.SecurityConfig{RequireMalwareScanHook: true})
	importsService.SetMalwareScanner(nil)
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-required", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-required", adminToken, "required.sd3", "sd3")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	req.Header.Set("X-Org-Id", "org-required")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFinalizeImportRejectsRequiredModeWithStubScanner(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	importsService.ConfigureSecurity(imports.SecurityConfig{RequireMalwareScanHook: true})
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-stub", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-stub", adminToken, "stub.sd3", "sd3")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	req.Header.Set("X-Org-Id", "org-stub")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFinalizeImportRejectsInfectedUpload(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	importsService.SetMalwareScanner(malwareScannerFunc(func(_ string, _ imports.FileUploadMetadata) imports.MalwareScanResult {
		return imports.MalwareScanResult{Status: "infected", Provider: "clamav"}
	}))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-infected", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-infected", adminToken, "infected.zip", "zip")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	req.Header.Set("X-Org-Id", "org-infected")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFinalizeImportAcceptsCleanUploadFromRealScanner(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importsService := imports.NewService()
	importsService.ConfigureSecurity(imports.SecurityConfig{RequireMalwareScanHook: true})
	importsService.SetMalwareScanner(malwareScannerFunc(func(_ string, metadata imports.FileUploadMetadata) imports.MalwareScanResult {
		if metadata.Filename != "clean.sd3" {
			t.Fatalf("expected filename clean.sd3, got %q", metadata.Filename)
		}
		return imports.MalwareScanResult{Status: "clean", Provider: "clamav"}
	}))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-clean", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-clean", adminToken, "clean.sd3", "sd3")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	req.Header.Set("X-Org-Id", "org-clean")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var env importEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Import.Status != "conflicts_pending" {
		t.Fatalf("expected status conflicts_pending, got %q", env.Import.Status)
	}
}
