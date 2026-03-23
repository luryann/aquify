package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/documents"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func newDocRouter(t *testing.T, svc *documents.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		svc,
		nil,
	)
}

func issueDocToken(t *testing.T, router http.Handler, userID, orgID, role, householdID string) string {
	t.Helper()
	payload := map[string]any{
		"user_id": userID,
		"org_id":  orgID,
		"roles":   []string{role},
	}
	if householdID != "" {
		payload["household_id"] = householdID
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issueDocToken: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var env struct {
		Session struct {
			AccessToken string `json:"access_token"`
		} `json:"session"`
	}
	json.NewDecoder(rr.Body).Decode(&env)
	return env.Session.AccessToken
}

func doDocReq(router http.Handler, method, path, token, orgID, body string) *httptest.ResponseRecorder {
	var b *bytes.Reader
	if body != "" {
		b = bytes.NewReader([]byte(body))
	} else {
		b = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, b)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestDocumentTypeCRUD verifies that admins can create, list, update, and
// delete document types.
func TestDocumentTypeCRUD(t *testing.T) {
	svc := documents.NewService()
	router := newDocRouter(t, svc)
	orgID := "org-docs-test"

	adminToken := issueDocToken(t, router, "admin-user", orgID, "admin", "")

	// Create
	rr := doDocReq(router, http.MethodPost, "/api/v1/document-types", adminToken, orgID, `{
		"name": "Season Liability Waiver",
		"category": "waiver",
		"description": "Annual liability release",
		"is_required": true,
		"applies_to": "swimmer",
		"expiry_months": 12,
		"is_active": true
	}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create doc type: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var createResp struct {
		DocumentType struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Category string `json:"category"`
		} `json:"document_type"`
	}
	json.NewDecoder(rr.Body).Decode(&createResp)
	typeID := createResp.DocumentType.ID
	if typeID == "" {
		t.Fatal("expected document_type.id in response")
	}
	if createResp.DocumentType.Name != "Season Liability Waiver" {
		t.Errorf("name mismatch: %q", createResp.DocumentType.Name)
	}
	if createResp.DocumentType.Category != "waiver" {
		t.Errorf("category mismatch: %q", createResp.DocumentType.Category)
	}

	// List
	rr = doDocReq(router, http.MethodGet, "/api/v1/document-types?active_only=true", adminToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list doc types: want 200, got %d", rr.Code)
	}
	var listResp struct {
		DocumentTypes []struct {
			ID string `json:"id"`
		} `json:"document_types"`
	}
	json.NewDecoder(rr.Body).Decode(&listResp)
	if len(listResp.DocumentTypes) != 1 {
		t.Errorf("expected 1 doc type, got %d", len(listResp.DocumentTypes))
	}

	// Update (make inactive)
	rr = doDocReq(router, http.MethodPut, "/api/v1/document-types/"+typeID, adminToken, orgID, `{"is_active": false}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("update doc type: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var updateResp struct {
		DocumentType struct {
			IsActive bool `json:"is_active"`
		} `json:"document_type"`
	}
	json.NewDecoder(rr.Body).Decode(&updateResp)
	if updateResp.DocumentType.IsActive {
		t.Error("expected is_active=false after update")
	}

	// Delete
	rr = doDocReq(router, http.MethodDelete, "/api/v1/document-types/"+typeID, adminToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("delete doc type: want 200, got %d", rr.Code)
	}
}

// TestDocumentTypeForbiddenForNonAdmin verifies that non-admin roles cannot
// create or delete document types.
func TestDocumentTypeForbiddenForNonAdmin(t *testing.T) {
	svc := documents.NewService()
	router := newDocRouter(t, svc)
	orgID := "org-docs-rbac"

	parentToken := issueDocToken(t, router, "parent-user", orgID, "parent", "hh-001")

	// Parent should NOT be able to create types
	rr := doDocReq(router, http.MethodPost, "/api/v1/document-types", parentToken, orgID, `{
		"name": "Waiver", "category": "waiver", "is_required": true, "is_active": true
	}`)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for parent creating doc type, got %d", rr.Code)
	}
}

// TestDocumentSubmissionWorkflow verifies the full submission lifecycle:
// create → attach file → verify.
func TestDocumentSubmissionWorkflow(t *testing.T) {
	svc := documents.NewService()
	router := newDocRouter(t, svc)
	orgID := "org-docs-workflow"
	householdID := "hh-workflow-001"

	adminToken := issueDocToken(t, router, "admin-user", orgID, "admin", "")
	parentToken := issueDocToken(t, router, "parent-user", orgID, "parent", householdID)

	// Admin creates a document type
	rr := doDocReq(router, http.MethodPost, "/api/v1/document-types", adminToken, orgID, `{
		"name": "Medical Authorization",
		"category": "medical",
		"is_required": true,
		"applies_to": "swimmer",
		"is_active": true
	}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create doc type: %d %s", rr.Code, rr.Body.String())
	}
	var typeResp struct {
		DocumentType struct{ ID string `json:"id"` } `json:"document_type"`
	}
	json.NewDecoder(rr.Body).Decode(&typeResp)
	typeID := typeResp.DocumentType.ID

	// Parent creates a submission
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents", parentToken, orgID, `{
		"document_type_id": "`+typeID+`",
		"household_id": "`+householdID+`"
	}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create submission: %d %s", rr.Code, rr.Body.String())
	}
	var subResp struct {
		Document struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"document"`
	}
	json.NewDecoder(rr.Body).Decode(&subResp)
	subID := subResp.Document.ID
	if subResp.Document.Status != "pending" {
		t.Errorf("expected status=pending, got %q", subResp.Document.Status)
	}

	// Parent attaches a file
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents/"+subID+"/attach", parentToken, orgID, `{
		"file_object_key": "uploads/doc.pdf",
		"file_name": "medical_auth.pdf",
		"file_mime": "application/pdf",
		"file_size_bytes": 12345
	}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("attach file: %d %s", rr.Code, rr.Body.String())
	}
	json.NewDecoder(rr.Body).Decode(&subResp)
	if subResp.Document.Status != "submitted" {
		t.Errorf("expected status=submitted after attach, got %q", subResp.Document.Status)
	}

	// Admin verifies
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents/"+subID+"/verify", adminToken, orgID, `{"review_notes": "Looks good!"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify: %d %s", rr.Code, rr.Body.String())
	}
	json.NewDecoder(rr.Body).Decode(&subResp)
	if subResp.Document.Status != "verified" {
		t.Errorf("expected status=verified, got %q", subResp.Document.Status)
	}
}

// TestDocumentRejectAndResubmit verifies that rejected submissions can be
// resubmitted by attaching a new file.
func TestDocumentRejectAndResubmit(t *testing.T) {
	svc := documents.NewService()
	router := newDocRouter(t, svc)
	orgID := "org-docs-reject"
	householdID := "hh-reject-001"

	adminToken := issueDocToken(t, router, "admin-user", orgID, "admin", "")
	parentToken := issueDocToken(t, router, "parent-user", orgID, "parent", householdID)

	// Create type
	rr := doDocReq(router, http.MethodPost, "/api/v1/document-types", adminToken, orgID, `{
		"name": "Insurance Card", "category": "insurance", "is_required": true, "is_active": true
	}`)
	var tr struct {
		DocumentType struct{ ID string `json:"id"` } `json:"document_type"`
	}
	json.NewDecoder(rr.Body).Decode(&tr)

	// Parent submits
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents", parentToken, orgID, `{
		"document_type_id": "`+tr.DocumentType.ID+`",
		"household_id": "`+householdID+`"
	}`)
	var sr struct {
		Document struct{ ID string `json:"id"` } `json:"document"`
	}
	json.NewDecoder(rr.Body).Decode(&sr)
	subID := sr.Document.ID

	// Attach file
	doDocReq(router, http.MethodPost, "/api/v1/documents/"+subID+"/attach", parentToken, orgID, `{
		"file_object_key": "uploads/ins.pdf", "file_name": "ins.pdf"
	}`)

	// Admin rejects
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents/"+subID+"/reject", adminToken, orgID, `{"review_notes": "Image is too blurry"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rr.Code, rr.Body.String())
	}
	var subResp struct {
		Document struct{ Status string `json:"status"` } `json:"document"`
	}
	json.NewDecoder(rr.Body).Decode(&subResp)
	if subResp.Document.Status != "rejected" {
		t.Errorf("expected status=rejected, got %q", subResp.Document.Status)
	}

	// Parent re-attaches
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents/"+subID+"/attach", parentToken, orgID, `{
		"file_object_key": "uploads/ins_v2.pdf", "file_name": "ins_v2.pdf"
	}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("re-attach after reject: %d %s", rr.Code, rr.Body.String())
	}
	json.NewDecoder(rr.Body).Decode(&subResp)
	if subResp.Document.Status != "submitted" {
		t.Errorf("expected status=submitted after re-attach, got %q", subResp.Document.Status)
	}
}

// TestDocumentHouseholdScopeEnforcement verifies that a parent can only see
// submissions for their own household.
func TestDocumentHouseholdScopeEnforcement(t *testing.T) {
	svc := documents.NewService()
	router := newDocRouter(t, svc)
	orgID := "org-docs-scope"

	adminToken := issueDocToken(t, router, "admin-user", orgID, "admin", "")
	parent1Token := issueDocToken(t, router, "parent-1", orgID, "parent", "hh-001")
	parent2Token := issueDocToken(t, router, "parent-2", orgID, "parent", "hh-002")

	// Create type
	rr := doDocReq(router, http.MethodPost, "/api/v1/document-types", adminToken, orgID, `{
		"name": "Photo Release", "category": "release", "is_required": false, "is_active": true
	}`)
	var tr struct {
		DocumentType struct{ ID string `json:"id"` } `json:"document_type"`
	}
	json.NewDecoder(rr.Body).Decode(&tr)
	typeID := tr.DocumentType.ID

	// Parent 1 creates a submission
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents", parent1Token, orgID, `{
		"document_type_id": "`+typeID+`", "household_id": "hh-001"
	}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("parent1 create submission: %d %s", rr.Code, rr.Body.String())
	}
	var sr struct {
		Document struct{ ID string `json:"id"` } `json:"document"`
	}
	json.NewDecoder(rr.Body).Decode(&sr)
	subID := sr.Document.ID

	// Parent 2 tries to view parent 1's submission — should get 404
	rr = doDocReq(router, http.MethodGet, "/api/v1/documents/"+subID, parent2Token, orgID, "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("parent2 accessing parent1 submission: want 404, got %d", rr.Code)
	}

	// Parent 2 tries to list with parent 1's household_id — must be rejected with 403
	rr = doDocReq(router, http.MethodGet, "/api/v1/documents?household_id=hh-001", parent2Token, orgID, "")
	if rr.Code != http.StatusForbidden {
		t.Errorf("parent2 listing parent1 docs: expected 403, got %d", rr.Code)
	}

	// Parent 1 lists their own — should see 1
	rr = doDocReq(router, http.MethodGet, "/api/v1/documents", parent1Token, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("parent1 list own docs: want 200, got %d", rr.Code)
	}
	var listResp struct {
		Documents []struct{ ID string `json:"id"` } `json:"documents"`
	}
	json.NewDecoder(rr.Body).Decode(&listResp)
	if len(listResp.Documents) != 1 {
		t.Errorf("parent1 should see 1 doc, got %d", len(listResp.Documents))
	}
}

// TestGetSubmissionRoleEnforcement verifies that swimmer and read_only roles
// cannot access GET /api/v1/documents/{id} (CRITICAL-1).
func TestGetSubmissionRoleEnforcement(t *testing.T) {
	svc := documents.NewService()
	router := newDocRouter(t, svc)
	orgID := "org-docs-role-get"
	householdID := "hh-role-get"

	adminToken := issueDocToken(t, router, "admin-user", orgID, "admin", "")
	parentToken := issueDocToken(t, router, "parent-user", orgID, "parent", householdID)
	swimmerToken := issueDocToken(t, router, "swimmer-user", orgID, "swimmer", "")
	readOnlyToken := issueDocToken(t, router, "readonly-user", orgID, "read_only", "")

	// Admin creates a type
	rr := doDocReq(router, http.MethodPost, "/api/v1/document-types", adminToken, orgID, `{
		"name": "Role Check Waiver", "category": "waiver", "is_required": true, "is_active": true
	}`)
	var tr struct {
		DocumentType struct{ ID string `json:"id"` } `json:"document_type"`
	}
	json.NewDecoder(rr.Body).Decode(&tr)

	// Parent creates a submission
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents", parentToken, orgID, `{
		"document_type_id": "`+tr.DocumentType.ID+`", "household_id": "`+householdID+`"
	}`)
	var sr struct {
		Document struct{ ID string `json:"id"` } `json:"document"`
	}
	json.NewDecoder(rr.Body).Decode(&sr)
	subID := sr.Document.ID

	// Swimmer role must get 403
	rr = doDocReq(router, http.MethodGet, "/api/v1/documents/"+subID, swimmerToken, orgID, "")
	if rr.Code != http.StatusForbidden {
		t.Errorf("swimmer GET document: want 403, got %d", rr.Code)
	}

	// read_only role must get 403
	rr = doDocReq(router, http.MethodGet, "/api/v1/documents/"+subID, readOnlyToken, orgID, "")
	if rr.Code != http.StatusForbidden {
		t.Errorf("read_only GET document: want 403, got %d", rr.Code)
	}

	// Admin must still succeed
	rr = doDocReq(router, http.MethodGet, "/api/v1/documents/"+subID, adminToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Errorf("admin GET document: want 200, got %d", rr.Code)
	}
}

// TestDocumentVerifyRequiresAdmin verifies that parents cannot verify or
// reject submissions.
func TestDocumentVerifyRequiresAdmin(t *testing.T) {
	svc := documents.NewService()
	router := newDocRouter(t, svc)
	orgID := "org-docs-admin-only"
	householdID := "hh-admin-only"

	adminToken := issueDocToken(t, router, "admin-user", orgID, "admin", "")
	parentToken := issueDocToken(t, router, "parent-user", orgID, "parent", householdID)

	// Create type + submission
	rr := doDocReq(router, http.MethodPost, "/api/v1/document-types", adminToken, orgID, `{
		"name": "USS Registration", "category": "registration", "is_required": true, "is_active": true
	}`)
	var tr struct {
		DocumentType struct{ ID string `json:"id"` } `json:"document_type"`
	}
	json.NewDecoder(rr.Body).Decode(&tr)

	rr = doDocReq(router, http.MethodPost, "/api/v1/documents", parentToken, orgID, `{
		"document_type_id": "`+tr.DocumentType.ID+`", "household_id": "`+householdID+`"
	}`)
	var sr struct {
		Document struct{ ID string `json:"id"` } `json:"document"`
	}
	json.NewDecoder(rr.Body).Decode(&sr)
	subID := sr.Document.ID

	doDocReq(router, http.MethodPost, "/api/v1/documents/"+subID+"/attach", parentToken, orgID, `{
		"file_object_key": "uploads/uss.pdf", "file_name": "uss.pdf"
	}`)

	// Parent tries to verify — must be forbidden
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents/"+subID+"/verify", parentToken, orgID, `{}`)
	if rr.Code != http.StatusForbidden {
		t.Errorf("parent verify: want 403, got %d", rr.Code)
	}

	// Parent tries to reject — must be forbidden
	rr = doDocReq(router, http.MethodPost, "/api/v1/documents/"+subID+"/reject", parentToken, orgID, `{}`)
	if rr.Code != http.StatusForbidden {
		t.Errorf("parent reject: want 403, got %d", rr.Code)
	}
}
