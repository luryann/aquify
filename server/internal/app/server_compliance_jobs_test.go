package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/compliance"
)

func TestComplianceRequestsAreProcessedByWorkers(t *testing.T) {
	compSvc := compliance.NewService()
	router := newStep16Router(t, nil, compSvc)

	issue := func(userID, role string, mfa bool) string {
		raw, _ := json.Marshal(map[string]any{"user_id": userID, "org_id": "org-demo", "roles": []string{role}, "mfa_authenticated": mfa})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return issuedAccessTokenFromResponse(t, rr)
	}
	adminMFA := issue("admin-mfa", "admin", true)
	adminNoMFA := issue("admin-plain", "admin", false)

	var hh struct {
		Household struct{ ID string } `json:"household"`
	}
	rr := doStep16Req(router, http.MethodPost, "/api/v1/members/households", adminMFA, step16Body(map[string]any{"org_id": "org-demo", "household_name": "Erase Me"}))
	if err := json.Unmarshal(rr.Body.Bytes(), &hh); err != nil || hh.Household.ID == "" {
		t.Fatalf("household: %d %s", rr.Code, rr.Body.String())
	}

	waitFor := func(desc string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if done() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", desc)
	}

	// Export: the worker builds the archive and marks the request ready.
	var exp struct {
		ID string `json:"id"`
	}
	rr = doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-export-requests", adminMFA, step16Body(map[string]any{"household_id": hh.Household.ID}))
	_ = json.Unmarshal(rr.Body.Bytes(), &exp)
	waitFor("export ready", func() bool {
		req, err := compSvc.GetDataExportRequest("org-demo", exp.ID)
		return err == nil && req.Status == "ready"
	})

	data, err := compSvc.CollectHouseholdData("org-demo", hh.Household.ID)
	if err != nil || data["household"] == nil {
		t.Fatalf("export archive should include the household record, got %v (%v)", data, err)
	}
	for _, key := range []string{"contacts", "swimmers", "registrations", "invoices", "payments"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("export archive missing %q section", key)
		}
	}

	// Deletion review: rejection needs notes; approval needs MFA.
	var del struct {
		ID string `json:"id"`
	}
	rr = doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests", adminMFA, step16Body(map[string]any{"household_id": hh.Household.ID, "reason": "Family left the club"}))
	_ = json.Unmarshal(rr.Body.Bytes(), &del)
	if del.ID == "" {
		t.Fatalf("create deletion: %d %s", rr.Code, rr.Body.String())
	}
	if rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests/"+del.ID+"/reject", adminMFA, "{}"); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reject without notes: want 422, got %d", rr.Code)
	}
	if rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests/"+del.ID+"/approve", adminNoMFA, "{}"); rr.Code != http.StatusForbidden {
		t.Fatalf("approve without MFA: want 403, got %d", rr.Code)
	}
	if rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests/"+del.ID+"/approve", adminMFA, "{}"); rr.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rr.Code, rr.Body.String())
	}
	waitFor("deletion completed", func() bool {
		req, err := compSvc.GetDataDeletionRequest("org-demo", del.ID)
		return err == nil && req.Status == "completed"
	})
	if rr := doStep16Req(router, http.MethodPost, "/api/v1/compliance/data-deletion-requests/"+del.ID+"/approve", adminMFA, "{}"); rr.Code != http.StatusConflict {
		t.Fatalf("re-approve completed request: want 409, got %d", rr.Code)
	}
}
