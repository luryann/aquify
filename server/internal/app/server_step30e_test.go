package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func newStep30ERouter(t *testing.T, kc keycloak.Client, membersSvc *members.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, membersSvc, nil, nil, nil, nil, nil, nil, nil, nil,
		audit.NewService(), nil, nil, nil, nil,
		kc,
	)
}

func issueStep30EToken(t *testing.T, router http.Handler, userID, orgID string, roles []string) string {
	t.Helper()
	payload := map[string]any{
		"user_id": userID,
		"org_id":  orgID,
		"roles":   roles,
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issueStep30EToken: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	return issuedAccessTokenFromResponse(t, rr)
}

func do30E(router http.Handler, method, path, token, body, orgID string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if orgID != "" {
		req.Header.Set("X-Org-Id", orgID)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// ─── Invite with create_household=true ────────────────────────────────────────

func TestStep30E_InviteCreatesHousehold(t *testing.T) {
	kc := keycloak.NewStubClient()
	membersSvc := members.NewService()
	router := newStep30ERouter(t, kc, membersSvc)

	ownerToken := issueStep30EToken(t, router, "owner-1", "org-1", []string{"owner"})

	body := `{"first_name":"Alice","last_name":"Smith","email":"alice@example.com","role":"parent","create_household":true}`
	rr := do30E(router, http.MethodPost, "/api/v1/users/invite", ownerToken, body, "org-1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["household_id"] == nil {
		t.Fatalf("expected household_id in response, got: %v", resp)
	}
	householdID := resp["household_id"].(string)
	if householdID == "" {
		t.Fatal("household_id should not be empty")
	}

	// Verify the household was created with keycloak_user_id set.
	userID := resp["user_id"].(string)
	hh, found := membersSvc.GetHouseholdByKeycloakUserID(userID)
	if !found {
		t.Fatal("expected household to be linked to created Keycloak user")
	}
	if hh.ID != householdID {
		t.Fatalf("household ID mismatch: want %s got %s", householdID, hh.ID)
	}
	if hh.PrimaryEmail != "alice@example.com" {
		t.Fatalf("expected primary email alice@example.com, got %s", hh.PrimaryEmail)
	}
}

func TestStep30E_InviteWithoutCreateHousehold(t *testing.T) {
	kc := keycloak.NewStubClient()
	membersSvc := members.NewService()
	router := newStep30ERouter(t, kc, membersSvc)

	ownerToken := issueStep30EToken(t, router, "owner-1", "org-1", []string{"owner"})

	body := `{"first_name":"Bob","last_name":"Jones","email":"bob@example.com","role":"parent","create_household":false}`
	rr := do30E(router, http.MethodPost, "/api/v1/users/invite", ownerToken, body, "org-1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["household_id"] != nil {
		t.Fatalf("expected no household_id in response, got: %v", resp["household_id"])
	}
}

// ─── GET /users/{id}/household ────────────────────────────────────────────────

func TestStep30E_GetUserHousehold_Linked(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u-parent-1", Email: "carol@example.com",
		FirstName: "Carol", LastName: "White", Enabled: true,
		Attributes: map[string][]string{"org_id": {"org-1"}},
	})

	membersSvc := members.NewService()
	// Pre-create a household linked to u-parent-1.
	hh, _ := membersSvc.CreateHousehold(
		&appmiddleware.Principal{UserID: "admin-1", Roles: []string{"owner"}, OrgID: "org-1"},
		members.CreateHouseholdInput{
			OrgID:          "org-1",
			HouseholdName:  "Carol White",
			PrimaryEmail:   "carol@example.com",
			KeycloakUserID: "u-parent-1",
		},
	)

	router := newStep30ERouter(t, kc, membersSvc)
	ownerToken := issueStep30EToken(t, router, "admin-1", "org-1", []string{"owner"})

	rr := do30E(router, http.MethodGet, "/api/v1/users/u-parent-1/household", ownerToken, "", "org-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	hhData, ok := resp["household"].(map[string]any)
	if !ok || hhData == nil {
		t.Fatalf("expected household in response, got: %v", resp)
	}
	if hhData["id"] != hh.ID {
		t.Fatalf("household id mismatch: want %s got %v", hh.ID, hhData["id"])
	}
}

func TestStep30E_GetUserHousehold_NotLinked(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u-2", Email: "dave@example.com",
		FirstName: "Dave", LastName: "Brown", Enabled: true,
		Attributes: map[string][]string{"org_id": {"org-1"}},
	})

	membersSvc := members.NewService()
	router := newStep30ERouter(t, kc, membersSvc)
	ownerToken := issueStep30EToken(t, router, "admin-1", "org-1", []string{"owner"})

	rr := do30E(router, http.MethodGet, "/api/v1/users/u-2/household", ownerToken, "", "org-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["household"] != nil {
		t.Fatalf("expected null household, got: %v", resp["household"])
	}
}

// ─── POST /users/{id}/household/link ─────────────────────────────────────────

func TestStep30E_LinkAndUnlink(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u-link-1", Email: "eve@example.com",
		FirstName: "Eve", LastName: "Green", Enabled: true,
		Attributes: map[string][]string{"org_id": {"org-1"}},
	})

	membersSvc := members.NewService()
	hh, _ := membersSvc.CreateHousehold(
		&appmiddleware.Principal{UserID: "admin-1", Roles: []string{"owner"}, OrgID: "org-1"},
		members.CreateHouseholdInput{
			OrgID:         "org-1",
			HouseholdName: "Eve Green",
			PrimaryEmail:  "eve@example.com",
		},
	)

	router := newStep30ERouter(t, kc, membersSvc)
	ownerToken := issueStep30EToken(t, router, "admin-1", "org-1", []string{"owner"})

	// Link household to user.
	linkBody, _ := json.Marshal(map[string]any{"household_id": hh.ID})
	rr := do30E(router, http.MethodPost, "/api/v1/users/u-link-1/household/link",
		ownerToken, string(linkBody), "org-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("link: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify the link is queryable.
	linked, found := membersSvc.GetHouseholdByKeycloakUserID("u-link-1")
	if !found || linked.ID != hh.ID {
		t.Fatal("household should be linked after link call")
	}

	// Unlink.
	unlinkBody, _ := json.Marshal(map[string]any{"household_id": hh.ID})
	rr = do30E(router, http.MethodPost, "/api/v1/users/u-link-1/household/unlink",
		ownerToken, string(unlinkBody), "org-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("unlink: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	_, found = membersSvc.GetHouseholdByKeycloakUserID("u-link-1")
	if found {
		t.Fatal("household should not be linked after unlink call")
	}
}

func TestStep30E_Link_NotFound(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u-link-2", Email: "frank@example.com",
		FirstName: "Frank", LastName: "Black", Enabled: true,
		Attributes: map[string][]string{"org_id": {"org-1"}},
	})

	membersSvc := members.NewService()
	router := newStep30ERouter(t, kc, membersSvc)
	ownerToken := issueStep30EToken(t, router, "admin-1", "org-1", []string{"owner"})

	linkBody, _ := json.Marshal(map[string]any{"household_id": "nonexistent-hh"})
	rr := do30E(router, http.MethodPost, "/api/v1/users/u-link-2/household/link",
		ownerToken, string(linkBody), "org-1")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStep30E_HouseholdLinkage_RBAC_AdminForbidden(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u-coach-1", Email: "coach@example.com",
		FirstName: "Coach", LastName: "X", Enabled: true,
		Attributes: map[string][]string{"org_id": {"org-1"}},
	})

	membersSvc := members.NewService()
	router := newStep30ERouter(t, kc, membersSvc)
	// Coach is NOT allowed to manage users/households.
	coachToken := issueStep30EToken(t, router, "coach-1", "org-1", []string{"coach"})

	rr := do30E(router, http.MethodGet, "/api/v1/users/u-coach-1/household", coachToken, "", "org-1")
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusUnauthorized {
		t.Fatalf("coach should be denied, got %d: %s", rr.Code, rr.Body.String())
	}
}
