package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func newSeasonsRouter(t *testing.T) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestSeasonCRUD verifies that admins can create, list, get, update, and
// delete season records.
func TestSeasonCRUD(t *testing.T) {
	router := newSeasonsRouter(t)
	orgID := "org-seasons-test"

	adminToken := issueTrainingToken(t, router, "admin-1", orgID, "admin")

	// Create
	body := `{
		"name": "2025–2026 Season",
		"start_date": "2025-09-01",
		"end_date": "2026-06-30",
		"status": "active",
		"goals": "Qualify 10+ swimmers for sectionals"
	}`
	rr := doOrgRequest(router, http.MethodPost, "/api/v1/seasons", adminToken, orgID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create season: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var created struct {
		Season struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Goals  string `json:"goals"`
		} `json:"season"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	seasonID := created.Season.ID
	if seasonID == "" || created.Season.Status != "active" {
		t.Fatalf("unexpected created season: %+v", created.Season)
	}

	// List
	rr = doOrgRequest(router, http.MethodGet, "/api/v1/seasons", adminToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list seasons: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var listed struct {
		Seasons []struct {
			ID string `json:"id"`
		} `json:"seasons"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Seasons) != 1 || listed.Seasons[0].ID != seasonID {
		t.Fatalf("expected one listed season %q, got %+v", seasonID, listed.Seasons)
	}

	// Get
	rr = doOrgRequest(router, http.MethodGet, "/api/v1/seasons/"+seasonID, adminToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("get season: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Update
	rr = doOrgRequest(router, http.MethodPut, "/api/v1/seasons/"+seasonID, adminToken, orgID, `{"status":"completed","goals":"Wrapped"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("update season: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var updated struct {
		Season struct {
			Status string `json:"status"`
			Goals  string `json:"goals"`
		} `json:"season"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Season.Status != "completed" || updated.Season.Goals != "Wrapped" {
		t.Fatalf("unexpected updated season: %+v", updated.Season)
	}

	// Invalid status is rejected
	rr = doOrgRequest(router, http.MethodPut, "/api/v1/seasons/"+seasonID, adminToken, orgID, `{"status":"paused"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid status update: want 400, got %d: %s", rr.Code, rr.Body.String())
	}

	// Delete
	rr = doOrgRequest(router, http.MethodDelete, "/api/v1/seasons/"+seasonID, adminToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("delete season: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	rr = doOrgRequest(router, http.MethodGet, "/api/v1/seasons/"+seasonID, adminToken, orgID, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get deleted season: want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestSeasonRBAC verifies write access is limited to admin, read access
// includes coach, and parent/swimmer are denied.
func TestSeasonRBAC(t *testing.T) {
	router := newSeasonsRouter(t)
	orgID := "org-seasons-rbac"

	adminToken := issueTrainingToken(t, router, "admin-1", orgID, "admin")
	coachToken := issueTrainingToken(t, router, "coach-1", orgID, "coach")
	parentToken := issueTrainingToken(t, router, "parent-1", orgID, "parent")
	swimToken := issueTrainingToken(t, router, "swim-1", orgID, "swimmer")

	body := `{"name":"2026–2027 Season","status":"upcoming"}`
	rr := doOrgRequest(router, http.MethodPost, "/api/v1/seasons", adminToken, orgID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("admin create season: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var created struct {
		Season struct {
			ID string `json:"id"`
		} `json:"season"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id := created.Season.ID

	// Coach can list.
	rr = doOrgRequest(router, http.MethodGet, "/api/v1/seasons", coachToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("coach list seasons: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Parent and swimmer are denied reads.
	for name, token := range map[string]string{"parent": parentToken, "swimmer": swimToken} {
		rr = doOrgRequest(router, http.MethodGet, "/api/v1/seasons", token, orgID, "")
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s list seasons: want 403, got %d: %s", name, rr.Code, rr.Body.String())
		}
	}

	// Coach cannot create, update, or delete.
	rr = doOrgRequest(router, http.MethodPost, "/api/v1/seasons", coachToken, orgID, body)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("coach create season: want 403, got %d: %s", rr.Code, rr.Body.String())
	}
	rr = doOrgRequest(router, http.MethodPut, "/api/v1/seasons/"+id, coachToken, orgID, `{"goals":"hacked"}`)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("coach update season: want 403, got %d: %s", rr.Code, rr.Body.String())
	}
	rr = doOrgRequest(router, http.MethodDelete, "/api/v1/seasons/"+id, coachToken, orgID, "")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("coach delete season: want 403, got %d: %s", rr.Code, rr.Body.String())
	}

	// Foreign-org access is denied by org scoping.
	otherToken := issueTrainingToken(t, router, "admin-2", "org-other", "admin")
	rr = doOrgRequest(router, http.MethodGet, "/api/v1/seasons/"+id, otherToken, "org-other", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("foreign org get season: want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
