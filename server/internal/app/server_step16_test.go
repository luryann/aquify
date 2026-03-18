package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func newStep16Router(t *testing.T, auditSvc *audit.Service, compSvc *compliance.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		auditSvc, compSvc, nil,
	)
}

// issueStep16Token issues a session token via the API, supporting household_id
// for parent principals.
func issueStep16Token(t *testing.T, router http.Handler, userID, orgID, role, householdID string) string {
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
		t.Fatalf("issueStep16Token: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var env struct {
		Session struct {
			AccessToken string `json:"access_token"`
		} `json:"session"`
	}
	json.NewDecoder(rr.Body).Decode(&env)
	return env.Session.AccessToken
}

func doStep16Req(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("{}")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func step16Body(fields map[string]any) string {
	b, _ := json.Marshal(fields)
	return string(b)
}

// ─── Audit endpoint tests ─────────────────────────────────────────────────────

func TestAuditEvents_OwnerCanQuery(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	rr := doStep16Req(router, http.MethodGet, "/api/v1/audit-events", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	if _, ok := body["events"]; !ok {
		t.Errorf("response missing 'events' key: %v", body)
	}
}

func TestAuditEvents_AdminCanQuery(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "admin-user", "org-demo", "admin", "")

	rr := doStep16Req(router, http.MethodGet, "/api/v1/audit-events", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuditEvents_ParentForbidden(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	rr := doStep16Req(router, http.MethodGet, "/api/v1/audit-events", token, "")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuditEvents_CoachForbidden(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "coach-user", "org-demo", "coach", "")

	rr := doStep16Req(router, http.MethodGet, "/api/v1/audit-events", token, "")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuditEvents_WithFilters(t *testing.T) {
	auditSvc := audit.NewService()
	router := newStep16Router(t, auditSvc, nil)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	// Seed one event.
	auditSvc.Append(audit.AppendRequest{
		OrgID:        "org-demo",
		ActorUserID:  "owner-user",
		ActorRoles:   []string{"owner"},
		Action:       "billing.invoice.issue",
		ResourceType: "invoice",
		ResourceID:   "inv-001",
	})

	// Filter by actor_id — should return the seeded event.
	rr := doStep16Req(router, http.MethodGet, "/api/v1/audit-events?actor_id=owner-user", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Events []struct {
			Action string `json:"action"`
		} `json:"events"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Events) == 0 {
		t.Fatal("expected at least one event")
	}
	if resp.Events[0].Action != "billing.invoice.issue" {
		t.Errorf("unexpected action: %s", resp.Events[0].Action)
	}
}

func TestAuditEvents_InvalidFromTimestamp_Returns400(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	rr := doStep16Req(router, http.MethodGet, "/api/v1/audit-events?from=not-a-date", token, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad 'from', got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuditEvents_InvalidToTimestamp_Returns400(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	rr := doStep16Req(router, http.MethodGet, "/api/v1/audit-events?to=2026-01-01", token, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad 'to', got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuditEvents_ValidTimeRange(t *testing.T) {
	auditSvc := audit.NewService()
	router := newStep16Router(t, auditSvc, nil)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)

	rr := doStep16Req(router, http.MethodGet, "/api/v1/audit-events?from="+from+"&to="+to, token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Compliance export request tests ─────────────────────────────────────────

func TestComplianceExport_OwnerCanCreate(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-export-requests", token,
		step16Body(map[string]any{"household_id": "hsh-001"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID          string     `json:"id"`
		ExpiresAt   *time.Time `json:"expires_at"`
		HouseholdID string     `json:"household_id"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if resp.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set on new export request")
	}
	if resp.HouseholdID != "hsh-001" {
		t.Errorf("unexpected household_id: %s", resp.HouseholdID)
	}
}

func TestComplianceExport_ParentCanCreateForOwnHousehold(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-export-requests", token,
		step16Body(map[string]any{"household_id": "hsh-parent"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("parent with own household: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestComplianceExport_ParentCannotCreateForOtherHousehold(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	// Attempt to create an export request for a different household.
	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-export-requests", token,
		step16Body(map[string]any{"household_id": "hsh-other"}))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("parent with other household: want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestComplianceExport_CoachForbidden(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "coach-user", "org-demo", "coach", "")

	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-export-requests", token,
		step16Body(map[string]any{"household_id": "hsh-001"}))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("coach: want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestComplianceExport_GetByOwner(t *testing.T) {
	svc := compliance.NewServiceWithConfig(30, 30)
	router := newStep16Router(t, nil, svc)
	ownerToken := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	// Create a request.
	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-export-requests", ownerToken,
		step16Body(map[string]any{"household_id": "hsh-001"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", rr.Code)
	}
	var created struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&created)

	// Owner can fetch it.
	rr2 := doStep16Req(router, http.MethodGet, "/api/v1/compliance/data-export-requests/"+created.ID, ownerToken, "")
	if rr2.Code != http.StatusOK {
		t.Fatalf("get by owner: want 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestComplianceExport_GetByNonRequesterReturns404(t *testing.T) {
	svc := compliance.NewServiceWithConfig(30, 30)
	router := newStep16Router(t, nil, svc)
	ownerToken := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")
	parentToken := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	// Owner creates a request.
	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-export-requests", ownerToken,
		step16Body(map[string]any{"household_id": "hsh-001"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", rr.Code)
	}
	var created struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&created)

	// Different parent (non-requester) should get 404, not 403.
	rr2 := doStep16Req(router, http.MethodGet, "/api/v1/compliance/data-export-requests/"+created.ID, parentToken, "")
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("non-requester parent: want 404 (not 403), got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestComplianceExport_ListRequiresOwnerOrAdmin(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	parentToken := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	rr := doStep16Req(router, http.MethodGet, "/api/v1/compliance/data-export-requests", parentToken, "")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("parent list: want 403, got %d", rr.Code)
	}
}

func TestComplianceExport_ExpiresAtIsSet(t *testing.T) {
	svc := compliance.NewServiceWithConfig(14, 30) // 14-day TTL
	router := newStep16Router(t, nil, svc)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	before := time.Now().UTC()
	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-export-requests", token,
		step16Body(map[string]any{"household_id": "hsh-001"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ExpiresAt *time.Time `json:"expires_at"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.ExpiresAt == nil {
		t.Fatal("ExpiresAt must not be nil")
	}
	// Should be ~14 days from now.
	expected := before.AddDate(0, 0, 14)
	if resp.ExpiresAt.Before(expected.Add(-time.Minute)) || resp.ExpiresAt.After(expected.Add(time.Minute)) {
		t.Errorf("ExpiresAt %v not within 1m of expected %v", *resp.ExpiresAt, expected)
	}
}

// ─── Compliance deletion request tests ───────────────────────────────────────

func TestComplianceDeletion_OwnerCanCreate(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests", token,
		step16Body(map[string]any{"household_id": "hsh-001", "reason": "test"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID        string     `json:"id"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if resp.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set on new deletion request")
	}
}

func TestComplianceDeletion_ParentCanCreateForOwnHousehold(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests", token,
		step16Body(map[string]any{"household_id": "hsh-parent", "reason": "right to erasure"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("parent own household: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestComplianceDeletion_ParentCannotCreateForOtherHousehold(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests", token,
		step16Body(map[string]any{"household_id": "hsh-other", "reason": "test"}))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("parent other household: want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestComplianceDeletion_GetByNonRequesterReturns404(t *testing.T) {
	svc := compliance.NewServiceWithConfig(30, 30)
	router := newStep16Router(t, nil, svc)
	ownerToken := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")
	parentToken := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	// Owner creates a deletion request.
	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests", ownerToken,
		step16Body(map[string]any{"household_id": "hsh-001", "reason": "test"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", rr.Code)
	}
	var created struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&created)

	// Different parent (non-requester) must receive 404, not 403.
	rr2 := doStep16Req(router, http.MethodGet, "/api/v1/compliance/data-deletion-requests/"+created.ID, parentToken, "")
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("non-requester parent: want 404 (not 403), got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestComplianceDeletion_DeletionReviewDaysApplied(t *testing.T) {
	svc := compliance.NewServiceWithConfig(30, 7) // 7-day review window
	router := newStep16Router(t, nil, svc)
	token := issueStep16Token(t, router, "owner-user", "org-demo", "owner", "")

	before := time.Now().UTC()
	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests", token,
		step16Body(map[string]any{"household_id": "hsh-001", "reason": "test"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ExpiresAt *time.Time `json:"expires_at"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.ExpiresAt == nil {
		t.Fatal("ExpiresAt must not be nil")
	}
	expected := before.AddDate(0, 0, 7)
	if resp.ExpiresAt.Before(expected.Add(-time.Minute)) || resp.ExpiresAt.After(expected.Add(time.Minute)) {
		t.Errorf("ExpiresAt %v not within 1m of expected %v", *resp.ExpiresAt, expected)
	}
}

func TestComplianceDeletion_CoachForbidden(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	token := issueStep16Token(t, router, "coach-user", "org-demo", "coach", "")

	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests", token,
		step16Body(map[string]any{"household_id": "hsh-001", "reason": "test"}))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("coach: want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestComplianceDeletion_ListRequiresOwnerOrAdmin(t *testing.T) {
	router := newStep16Router(t, nil, nil)
	parentToken := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	rr := doStep16Req(router, http.MethodGet, "/api/v1/compliance/data-deletion-requests", parentToken, "")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("parent list: want 403, got %d", rr.Code)
	}
}

func TestComplianceDeletion_GetByRequesterSelf(t *testing.T) {
	svc := compliance.NewServiceWithConfig(30, 30)
	router := newStep16Router(t, nil, svc)
	parentToken := issueStep16Token(t, router, "parent-user", "org-demo", "parent", "hsh-parent")

	// Parent creates a deletion request for own household.
	rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests", parentToken,
		step16Body(map[string]any{"household_id": "hsh-parent", "reason": "gdpr"}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&created)

	// The same parent can fetch their own request.
	rr2 := doStep16Req(router, http.MethodGet, "/api/v1/compliance/data-deletion-requests/"+created.ID, parentToken, "")
	if rr2.Code != http.StatusOK {
		t.Fatalf("get own: want 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
}
