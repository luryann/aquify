package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type apiErrorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

type issueSessionEnvelope struct {
	Session struct {
		AccessToken string `json:"access_token"`
	} `json:"session"`
}

type roleAssignmentEnvelope struct {
	Status     string `json:"status"`
	Assignment struct {
		OrgID       string `json:"org_id"`
		ActorUserID string `json:"actor_user_id"`
		Role        string `json:"role"`
	} `json:"assignment"`
}

type roleAssignmentsListEnvelope struct {
	Assignments []struct {
		OrgID        string `json:"org_id"`
		TargetUserID string `json:"target_user_id"`
		Role         string `json:"role"`
		Reason       string `json:"reason"`
	} `json:"assignments"`
	Count int `json:"count"`
}

func TestProtectedRoutesDenyByDefaultWithoutAuthorization(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestRoleAssignmentEndpointRejectsUnauthorizedRequests(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := map[string]any{
		"org_id":         "org-demo",
		"target_user_id": "user-123",
		"role":           "coach",
		"reason":         "Bootstrap assignment",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/roles/assign", encodeJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestRoleAssignmentRejectsScopeViolation(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminMFAToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)
	body := map[string]any{
		"org_id":         "org-other",
		"target_user_id": "user-123",
		"role":           "coach",
		"reason":         "Bootstrap assignment",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/roles/assign", encodeJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+adminMFAToken)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}

	var envelope apiErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if envelope.Error.Code != "scope_violation" {
		t.Fatalf("expected error code scope_violation, got %q", envelope.Error.Code)
	}
}

func TestRoleAssignmentRequiresMFAForPrivilegedAction(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	body := map[string]any{
		"org_id":         "org-demo",
		"target_user_id": "user-123",
		"role":           "coach",
		"reason":         "Bootstrap assignment",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/roles/assign", encodeJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}

	var envelope apiErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if envelope.Error.Code != "mfa_required" {
		t.Fatalf("expected error code mfa_required, got %q", envelope.Error.Code)
	}
}

func TestRoleAssignmentAuthorizationMatrix(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	crossOrgAdminMFAToken := issueSessionToken(t, router, "org-other", []string{"admin"}, true)
	inScopeAdminMFAToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	cases := []struct {
		name             string
		authHeader       string
		headerOrgID      string
		bodyOrgID        string
		expectedStatus   int
		expectedErrCode  string
		expectAssignment bool
	}{
		{
			name:            "unauthenticated request is denied by default",
			headerOrgID:     "org-demo",
			bodyOrgID:       "org-demo",
			expectedStatus:  http.StatusUnauthorized,
			expectedErrCode: "unauthorized",
		},
		{
			name:            "privileged role requires mfa",
			authHeader:      "Bearer " + issueSessionToken(t, router, "org-demo", []string{"admin"}, false),
			headerOrgID:     "org-demo",
			bodyOrgID:       "org-demo",
			expectedStatus:  http.StatusForbidden,
			expectedErrCode: "mfa_required",
		},
		{
			name:            "body org must match header org",
			authHeader:      "Bearer " + issueSessionToken(t, router, "org-demo", []string{"admin"}, true),
			headerOrgID:     "org-demo",
			bodyOrgID:       "org-other",
			expectedStatus:  http.StatusForbidden,
			expectedErrCode: "scope_violation",
		},
		{
			name:            "principal org must match header org",
			authHeader:      "Bearer " + crossOrgAdminMFAToken,
			headerOrgID:     "org-demo",
			bodyOrgID:       "org-demo",
			expectedStatus:  http.StatusForbidden,
			expectedErrCode: "scope_violation",
		},
		{
			name:             "assignment succeeds when role mfa and org scope align",
			authHeader:       "Bearer " + inScopeAdminMFAToken,
			headerOrgID:      "org-demo",
			bodyOrgID:        "org-demo",
			expectedStatus:   http.StatusCreated,
			expectAssignment: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := map[string]any{
				"org_id":         tc.bodyOrgID,
				"target_user_id": "user-123",
				"role":           "coach",
				"reason":         "Bootstrap assignment",
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/roles/assign", encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Org-Id", tc.headerOrgID)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			if tc.expectAssignment {
				var envelope roleAssignmentEnvelope
				if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("failed to decode assignment response: %v", err)
				}
				if envelope.Status != "assigned" {
					t.Fatalf("expected assigned status, got %q", envelope.Status)
				}
				if envelope.Assignment.OrgID != tc.bodyOrgID {
					t.Fatalf("expected assignment org_id %q, got %q", tc.bodyOrgID, envelope.Assignment.OrgID)
				}
				if envelope.Assignment.Role != "coach" {
					t.Fatalf("expected assignment role coach, got %q", envelope.Assignment.Role)
				}
				if envelope.Assignment.ActorUserID == "" {
					t.Fatal("expected actor_user_id to be set")
				}
				return
			}

			var envelope apiErrorEnvelope
			if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if envelope.Error.Code != tc.expectedErrCode {
				t.Fatalf("expected error code %q, got %q", tc.expectedErrCode, envelope.Error.Code)
			}
		})
	}
}

func TestRoleAssignmentListReturnsLiveAssignments(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminMFAToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	body := map[string]any{
		"org_id":         "org-demo",
		"target_user_id": "user-live",
		"role":           "coach",
		"reason":         "Live assignment check",
	}
	assignReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/roles/assign", encodeJSONBody(t, body))
	assignReq.Header.Set("Content-Type", "application/json")
	assignReq.Header.Set("X-Org-Id", "org-demo")
	assignReq.Header.Set("Authorization", "Bearer "+adminMFAToken)
	assignRR := httptest.NewRecorder()
	router.ServeHTTP(assignRR, assignReq)
	if assignRR.Code != http.StatusCreated {
		t.Fatalf("expected assignment creation status %d, got %d: %s", http.StatusCreated, assignRR.Code, assignRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/roles/assignments", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+adminMFAToken)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d: %s", http.StatusOK, listRR.Code, listRR.Body.String())
	}

	var env roleAssignmentsListEnvelope
	if err := json.Unmarshal(listRR.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode assignments list response: %v", err)
	}
	if env.Count != 1 || len(env.Assignments) != 1 {
		t.Fatalf("expected exactly one assignment, got count=%d len=%d", env.Count, len(env.Assignments))
	}
	if env.Assignments[0].TargetUserID != "user-live" {
		t.Fatalf("expected target_user_id user-live, got %q", env.Assignments[0].TargetUserID)
	}
	if env.Assignments[0].Role != "coach" {
		t.Fatalf("expected role coach, got %q", env.Assignments[0].Role)
	}
}

func TestMFAEnrollContractAcceptsOnlyImplementedFactors(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	cases := []struct {
		name           string
		factorType     string
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "totp is accepted",
			factorType:     "totp",
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "backup_code is accepted",
			factorType:     "backup_code",
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "webauthn is rejected until implemented",
			factorType:     "webauthn",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "validation_failed",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := map[string]any{
				"factor_type": tc.factorType,
				"label":       "Primary factor",
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/enroll", encodeJSONBody(t, body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Org-Id", "org-demo")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			if tc.expectedCode == "" {
				return
			}

			var envelope apiErrorEnvelope
			if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if envelope.Error.Code != tc.expectedCode {
				t.Fatalf("expected error code %q, got %q", tc.expectedCode, envelope.Error.Code)
			}
		})
	}
}

func issueSessionToken(t *testing.T, router http.Handler, orgID string, roles []string, mfaAuthenticated bool) string {
	t.Helper()

	payload := map[string]any{
		"user_id":           "test-admin-user",
		"org_id":            orgID,
		"roles":             roles,
		"mfa_authenticated": mfaAuthenticated,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", encodeJSONBody(t, payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected session issue status %d, got %d", http.StatusCreated, rr.Code)
	}

	var envelope issueSessionEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode session issue response: %v", err)
	}
	if envelope.Session.AccessToken == "" {
		t.Fatal("expected access_token in session issue response")
	}
	return envelope.Session.AccessToken
}

func encodeJSONBody(t *testing.T, payload any) *bytes.Reader {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal request payload: %v", err)
	}
	return bytes.NewReader(raw)
}
