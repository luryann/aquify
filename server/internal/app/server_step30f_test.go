package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func issueStep30FToken(t *testing.T, router http.Handler, userID, orgID string, roles []string) string {
	t.Helper()
	return issueSession(t, router, sessionSpec{UserID: userID, OrgID: orgID, Roles: roles})
}

// ─── POST /api/v1/register (public self-registration) ─────────────────────────

func TestSelfRegister_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	router := newStep30BRouter(t, kc)

	body := `{"first_name":"Jane","last_name":"Doe","email":"jane@example.com","org_id":"org-1","requested_role":"coach","club_code":"dare2026","note":"Parent volunteer"}`
	rr := doRequest(router, http.MethodPost, "/api/v1/register", "", body, "")

	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "pending_approval" {
		t.Errorf("want status=pending_approval, got %v", resp["status"])
	}
	if resp["user_id"] == "" {
		t.Error("want non-empty user_id")
	}

	// Verify registration_status attribute was set.
	userID := resp["user_id"].(string)
	u, err := kc.GetUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Attributes == nil || len(u.Attributes["registration_status"]) == 0 || u.Attributes["registration_status"][0] != "pending_approval" {
		t.Errorf("expected registration_status=pending_approval, got attrs=%v", u.Attributes)
	}
	if got := u.Attributes["requested_role"][0]; got != "coach" {
		t.Errorf("expected requested_role=coach, got %q", got)
	}
	if got := u.Attributes["registration_club_code"][0]; got != "DARE2026" {
		t.Errorf("expected registration_club_code=DARE2026, got %q", got)
	}
	if got := u.Attributes["registration_note"][0]; got != "Parent volunteer" {
		t.Errorf("expected registration_note to persist, got %q", got)
	}
	if got := u.Attributes["registration_submitted_at"][0]; got == "" {
		t.Error("expected registration_submitted_at to be set")
	}
}

func TestSelfRegister_DuplicateEmail_Returns409(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID:    "existing-user",
		Email: "jane@example.com",
		Attributes: map[string][]string{
			"org_id": {"org-1"},
		},
	})
	router := newStep30BRouter(t, kc)

	body := `{"first_name":"Jane","last_name":"Doe","email":"jane@example.com","org_id":"org-1"}`
	rr := doRequest(router, http.MethodPost, "/api/v1/register", "", body, "")

	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSelfRegister_MissingEmail_Returns400(t *testing.T) {
	kc := keycloak.NewStubClient()
	router := newStep30BRouter(t, kc)

	body := `{"first_name":"Jane","org_id":"org-1"}`
	rr := doRequest(router, http.MethodPost, "/api/v1/register", "", body, "")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSelfRegister_MissingOrgID_Returns400(t *testing.T) {
	kc := keycloak.NewStubClient()
	router := newStep30BRouter(t, kc)

	body := `{"first_name":"Jane","last_name":"Doe","email":"jane@example.com"}`
	rr := doRequest(router, http.MethodPost, "/api/v1/register", "", body, "")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── GET /api/v1/users?status=pending_approval ────────────────────────────────

func TestListUsers_PendingApprovalFilter(t *testing.T) {
	kc := keycloak.NewStubClient()
	// Add a pending user.
	kc.AddUser(keycloak.User{
		ID:      "pending-1",
		Email:   "pending@example.com",
		Enabled: true,
		Attributes: map[string][]string{
			"org_id":              {"org-1"},
			"registration_status": {"pending_approval"},
		},
	})
	// Add an active user (should NOT appear).
	kc.AddUser(keycloak.User{
		ID:      "active-1",
		Email:   "active@example.com",
		Enabled: true,
		Attributes: map[string][]string{
			"org_id": {"org-1"},
		},
	})

	router := newStep30BRouter(t, kc)
	token := issueStep30FToken(t, router, "admin-1", "org-1", []string{"admin"})

	rr := doRequest(router, http.MethodGet, "/api/v1/users?status=pending_approval", token, "", "org-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Users []struct {
			ID string `json:"id"`
		} `json:"users"`
		Count int `json:"count"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("want count=1, got %d", resp.Count)
	}
	if len(resp.Users) == 0 || resp.Users[0].ID != "pending-1" {
		t.Errorf("expected pending-1 in results, got %+v", resp.Users)
	}
}

// ─── POST /users/{id}/approve ─────────────────────────────────────────────────

func TestApproveUser_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID:      "pending-user",
		Email:   "pending@example.com",
		Enabled: false,
		Attributes: map[string][]string{
			"org_id":              {"org-1"},
			"registration_status": {"pending_approval"},
			"requested_role":      {"coach"},
			"registration_note":   {"Volunteer family"},
		},
	})

	router := newStep30BRouter(t, kc)
	token := issueStep30FToken(t, router, "admin-1", "org-1", []string{"admin"})

	rr := doRequest(router, http.MethodPost, "/api/v1/users/pending-user/approve", token, "", "org-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "approved" {
		t.Errorf("want status=approved, got %v", resp["status"])
	}

	// Verify user is now enabled.
	u, _ := kc.GetUser(context.Background(), "pending-user")
	if !u.Enabled {
		t.Error("expected user to be enabled after approval")
	}
	// Verify registration_status updated.
	if u.Attributes["registration_status"][0] != "active" {
		t.Errorf("expected registration_status=active, got %v", u.Attributes["registration_status"])
	}
	if got := u.Attributes["registration_note"][0]; got != "Volunteer family" {
		t.Errorf("expected registration metadata to be preserved, got %q", got)
	}
	roles, err := kc.GetUserRealmRoles(context.Background(), "pending-user")
	if err != nil {
		t.Fatalf("GetUserRealmRoles: %v", err)
	}
	if len(roles) != 1 || roles[0].Name != "coach" {
		t.Errorf("expected requested role coach to be assigned, got %+v", roles)
	}
}

func TestApproveUser_NonAdmin_Returns403(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID:    "pending-user",
		Email: "pending@example.com",
		Attributes: map[string][]string{
			"org_id": {"org-1"},
		},
	})

	router := newStep30BRouter(t, kc)
	token := issueStep30FToken(t, router, "coach-1", "org-1", []string{"coach"})

	rr := doRequest(router, http.MethodPost, "/api/v1/users/pending-user/approve", token, "", "org-1")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── POST /users/{id}/reject ──────────────────────────────────────────────────

func TestRejectUser_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID:      "pending-user",
		Email:   "pending@example.com",
		Enabled: true,
		Attributes: map[string][]string{
			"org_id":              {"org-1"},
			"registration_status": {"pending_approval"},
		},
	})

	router := newStep30BRouter(t, kc)
	token := issueStep30FToken(t, router, "admin-1", "org-1", []string{"admin"})

	body := `{"reason":"Not a club member"}`
	rr := doRequest(router, http.MethodPost, "/api/v1/users/pending-user/reject", token, body, "org-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "rejected" {
		t.Errorf("want status=rejected, got %v", resp["status"])
	}

	// Verify user is disabled.
	u, _ := kc.GetUser(context.Background(), "pending-user")
	if u.Enabled {
		t.Error("expected user to be disabled after rejection")
	}
	// Verify registration_status updated.
	if u.Attributes["registration_status"][0] != "rejected" {
		t.Errorf("expected registration_status=rejected, got %v", u.Attributes["registration_status"])
	}
}

func TestRejectUser_WithoutReason_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID:      "pending-user2",
		Email:   "pending2@example.com",
		Enabled: true,
		Attributes: map[string][]string{
			"org_id":              {"org-1"},
			"registration_status": {"pending_approval"},
		},
	})

	router := newStep30BRouter(t, kc)
	token := issueStep30FToken(t, router, "admin-1", "org-1", []string{"admin"})

	rr := doRequest(router, http.MethodPost, "/api/v1/users/pending-user2/reject", token, "", "org-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRejectUser_NonAdmin_Returns403(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID:    "pending-user",
		Email: "pending@example.com",
		Attributes: map[string][]string{
			"org_id": {"org-1"},
		},
	})

	router := newStep30BRouter(t, kc)
	token := issueStep30FToken(t, router, "coach-1", "org-1", []string{"coach"})

	body := `{"reason":"no"}`
	rr := doRequest(router, http.MethodPost, "/api/v1/users/pending-user/reject", token, body, "org-1")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Approve/reject emit audit events ─────────────────────────────────────────

func TestApproveUser_EmitsAuditEvent(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID:      "pending-audit",
		Email:   "audit@example.com",
		Enabled: false,
		Attributes: map[string][]string{
			"org_id":              {"org-1"},
			"registration_status": {"pending_approval"},
		},
	})

	auditSvc := audit.NewService()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger, RouterDeps{
		Audit:    auditSvc,
		Keycloak: kc,
	})

	token := issueStep30FToken(t, router, "admin-1", "org-1", []string{"admin"})
	doRequest(router, http.MethodPost, "/api/v1/users/pending-audit/approve", token, "", "org-1")

	events, _ := auditSvc.Query(audit.Filter{OrgID: "org-1", Action: "auth.user.approve"})
	if len(events) == 0 {
		t.Error("expected audit event for approve action, got none")
	}
}

func TestRejectUser_EmitsAuditEvent(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID:      "pending-reject-audit",
		Email:   "reject-audit@example.com",
		Enabled: true,
		Attributes: map[string][]string{
			"org_id":              {"org-1"},
			"registration_status": {"pending_approval"},
		},
	})

	auditSvc := audit.NewService()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger, RouterDeps{
		Audit:    auditSvc,
		Keycloak: kc,
	})

	token := issueStep30FToken(t, router, "admin-1", "org-1", []string{"admin"})
	doRequest(router, http.MethodPost, "/api/v1/users/pending-reject-audit/reject", token, `{"reason":"test"}`, "org-1")

	events, _ := auditSvc.Query(audit.Filter{OrgID: "org-1", Action: "auth.user.reject"})
	if len(events) == 0 {
		t.Error("expected audit event for reject action, got none")
	}
}
