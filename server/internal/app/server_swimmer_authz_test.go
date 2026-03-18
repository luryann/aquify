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
// admin/owner/billing/parent; the swimmer role must never appear in that list.
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
// least admin/owner/billing — a swimmer-only token must receive 403.
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
