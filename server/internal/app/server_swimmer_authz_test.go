package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Swimmer role: no access to financial data ────────────────────────────────

// TestSwimmerRoleCannotAccessHouseholdFinancialData verifies that a user with
// only the "swimmer" role is denied access to all billing endpoints.
// The billing service's requireBillingReadAccess helper permits only
// admin and parent; the swimmer role must never appear in that list.
func TestSwimmerRoleCannotAccessHouseholdFinancialData(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	swimmerToken := issueSessionToken(t, router, "org-demo", []string{"swimmer"}, false)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "household invoices list",
			method: http.MethodGet,
			path:   "/api/v1/households/hsh_any/invoices",
		},
		{
			name:   "ledger events list",
			method: http.MethodGet,
			path:   "/api/v1/ledger-events",
		},
		{
			name:   "invoice get by id",
			method: http.MethodGet,
			path:   "/api/v1/invoices/inv_any",
		},
		{
			name:   "invoice items list",
			method: http.MethodGet,
			path:   "/api/v1/invoices/inv_any/items",
		},
		{
			name:   "payments list",
			method: http.MethodGet,
			path:   "/api/v1/invoices/inv_any/payments",
		},
		{
			name:   "refunds list",
			method: http.MethodGet,
			path:   "/api/v1/invoices/inv_any/refunds",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("X-Org-Id", "org-demo")
			req.Header.Set("Authorization", "Bearer "+swimmerToken)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("%s %s: expected status %d (swimmer must not access billing), got %d (body: %s)",
					tc.method, tc.path, http.StatusForbidden, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestSwimmerRoleCannotAccessLedger confirms the ledger endpoint requires at
// admin — a swimmer-only token must receive 403.
func TestSwimmerRoleCannotAccessLedger(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	swimmerToken := issueSessionToken(t, router, "org-demo", []string{"swimmer"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ledger-events", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+swimmerToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d for swimmer accessing ledger, got %d body=%s",
			http.StatusForbidden, rr.Code, rr.Body.String())
	}
}

// ─── Parent role: no cross-household data access ──────────────────────────────

// TestParentCannotAccessCrossHouseholdMemberData verifies that a parent can
// only read/mutate data for households they own.  A parent who creates Household
// A must be denied when attempting to:
//   - update Household B
//   - add a contact to Household B
//   - create a swimmer in Household B
//   - list swimmers scoped to Household B
func TestParentCannotAccessCrossHouseholdMemberData(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Parent A and an admin (to create Household B).
	parentAToken := issueSessionTokenForUser(t, router, "parent-user-a", "org-demo", []string{"parent"}, false)
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// --- Household A (owned by Parent A) ---
	createHHA := httptest.NewRequest(http.MethodPost, "/api/v1/members/households",
		encodeJSONBody(t, map[string]any{
			"org_id":         "org-demo",
			"household_name": "Family A",
		}))
	createHHA.Header.Set("Content-Type", "application/json")
	createHHA.Header.Set("X-Org-Id", "org-demo")
	createHHA.Header.Set("Authorization", "Bearer "+parentAToken)
	rrA := httptest.NewRecorder()
	router.ServeHTTP(rrA, createHHA)
	if rrA.Code != http.StatusCreated {
		t.Fatalf("expected household A create status %d, got %d body=%s", http.StatusCreated, rrA.Code, rrA.Body.String())
	}

	// --- Household B (owned by admin — no association with Parent A) ---
	createHHB := httptest.NewRequest(http.MethodPost, "/api/v1/members/households",
		encodeJSONBody(t, map[string]any{
			"org_id":         "org-demo",
			"household_name": "Family B",
		}))
	createHHB.Header.Set("Content-Type", "application/json")
	createHHB.Header.Set("X-Org-Id", "org-demo")
	createHHB.Header.Set("Authorization", "Bearer "+adminToken)
	rrB := httptest.NewRecorder()
	router.ServeHTTP(rrB, createHHB)
	if rrB.Code != http.StatusCreated {
		t.Fatalf("expected household B create status %d, got %d body=%s", http.StatusCreated, rrB.Code, rrB.Body.String())
	}
	var hhBEnv householdEnvelope
	if err := json.Unmarshal(rrB.Body.Bytes(), &hhBEnv); err != nil {
		t.Fatalf("failed to decode household B response: %v", err)
	}
	householdBID := hhBEnv.Household.ID
	if householdBID == "" {
		t.Fatal("expected household B id")
	}

	// --- Parent A tries to UPDATE Household B → 403 ---
	t.Run("parent cannot update foreign household", func(t *testing.T) {
		t.Parallel()
		householdName := "Hijacked Name"
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/members/households/"+householdBID,
			encodeJSONBody(t, map[string]any{"household_name": &householdName}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+parentAToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d when parent A updates household B, got %d body=%s",
				http.StatusForbidden, rr.Code, rr.Body.String())
		}
	})

	// --- Parent A tries to ADD A CONTACT to Household B → 403 ---
	t.Run("parent cannot add contact to foreign household", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/members/households/"+householdBID+"/contacts",
			encodeJSONBody(t, map[string]any{
				"first_name":   "Intruder",
				"last_name":    "Contact",
				"relationship": "guardian",
			}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+parentAToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d when parent A adds contact to household B, got %d body=%s",
				http.StatusForbidden, rr.Code, rr.Body.String())
		}
	})

	// --- Parent A tries to CREATE A SWIMMER in Household B → 403 ---
	t.Run("parent cannot create swimmer in foreign household", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers",
			encodeJSONBody(t, map[string]any{
				"org_id":        "org-demo",
				"household_id":  householdBID,
				"first_name":    "Hostile",
				"last_name":     "Swimmer",
				"date_of_birth": "2012-01-01",
				"gender":        "male",
			}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+parentAToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d when parent A creates swimmer in household B, got %d body=%s",
				http.StatusForbidden, rr.Code, rr.Body.String())
		}
	})

	// --- Parent A tries to LIST SWIMMERS scoped to Household B → 403 ---
	t.Run("parent cannot list swimmers scoped to foreign household", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmers?household_id="+householdBID, nil)
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+parentAToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d when parent A lists swimmers in household B, got %d body=%s",
				http.StatusForbidden, rr.Code, rr.Body.String())
		}
	})
}

func TestSwimmerRoleCanListUpcomingMeets(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	swimmerToken := issueSessionToken(t, router, "org-demo", []string{"swimmer"}, false)

	createMeetReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets",
		encodeJSONBody(t, map[string]any{
			"name":      "Spring Invite",
			"meet_type": "invitational",
			"course":    "SCY",
		}))
	createMeetReq.Header.Set("Content-Type", "application/json")
	createMeetReq.Header.Set("X-Org-Id", "org-demo")
	createMeetReq.Header.Set("Authorization", "Bearer "+adminToken)
	createMeetRes := httptest.NewRecorder()
	router.ServeHTTP(createMeetRes, createMeetReq)
	if createMeetRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d creating meet, got %d body=%s", http.StatusCreated, createMeetRes.Code, createMeetRes.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/meets", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+swimmerToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for swimmer listing meets, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
}

func TestSwimmerRoleListSwimmersIsScopedToOwnHousehold(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	createHousehold := func(name string) string {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/members/households",
			encodeJSONBody(t, map[string]any{
				"org_id":         "org-demo",
				"household_name": name,
			}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected status %d creating household, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
		}

		var env householdEnvelope
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode household response: %v", err)
		}
		if env.Household.ID == "" {
			t.Fatal("expected household id in response")
		}
		return env.Household.ID
	}

	createSwimmer := func(householdID, firstName string) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers",
			encodeJSONBody(t, map[string]any{
				"org_id":        "org-demo",
				"household_id":  householdID,
				"first_name":    firstName,
				"last_name":     "Test",
				"date_of_birth": "2013-02-14",
				"gender":        "female",
			}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected status %d creating swimmer, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
		}
	}

	householdA := createHousehold("Family A")
	householdB := createHousehold("Family B")
	createSwimmer(householdA, "Alice")
	createSwimmer(householdB, "Bri")

	swimmerToken := issueSessionTokenWithHousehold(t, router, "swimmer-a", "org-demo", householdA, []string{"swimmer"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmers", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+swimmerToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for swimmer listing swimmers, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var envelope struct {
		Swimmers []struct {
			ID          string `json:"id"`
			HouseholdID string `json:"household_id"`
		} `json:"swimmers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode swimmers response: %v", err)
	}
	if len(envelope.Swimmers) != 1 {
		t.Fatalf("expected exactly 1 swimmer for swimmer household scope, got %d", len(envelope.Swimmers))
	}
	if envelope.Swimmers[0].HouseholdID != householdA {
		t.Fatalf("expected swimmer household_id %q, got %q", householdA, envelope.Swimmers[0].HouseholdID)
	}
}
