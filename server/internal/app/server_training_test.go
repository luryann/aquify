package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/training"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func newTrainingRouter(t *testing.T, svc *training.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		svc,
		nil,
		nil,
	)
}

func issueTrainingToken(t *testing.T, router http.Handler, userID, orgID, role string) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"user_id": userID,
		"org_id":  orgID,
		"roles":   []string{role},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issueTrainingToken: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	return issuedAccessTokenFromResponse(t, rr)
}

func doTrainingReq(router http.Handler, method, path, token, orgID, body string) *httptest.ResponseRecorder {
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

// TestPracticeCRUD verifies that coaches can create, list, get, update, and
// delete practice records.
func TestPracticeCRUD(t *testing.T) {
	svc := training.NewService()
	router := newTrainingRouter(t, svc)
	orgID := "org-training-test"

	coachToken := issueTrainingToken(t, router, "coach-1", orgID, "coach")

	// Create
	body := `{
		"date": "2026-03-24",
		"group_id": "group-senior",
		"type": "water",
		"description": "Aerobic base — 10x200 on 3:00",
		"yardage_target": 5000,
		"attendance": 12,
		"strokes": {"free": 3000, "back": 600, "breast": 400, "fly": 600, "im": 400}
	}`
	rr := doTrainingReq(router, http.MethodPost, "/api/v1/practices", coachToken, orgID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create practice: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var createResp struct {
		Practice struct {
			ID          string `json:"id"`
			Date        string `json:"date"`
			Type        string `json:"type"`
			Description string `json:"description"`
			Strokes     struct {
				Free int `json:"free"`
				Back int `json:"back"`
			} `json:"strokes"`
		} `json:"practice"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Practice.ID == "" {
		t.Fatal("expected non-empty practice ID")
	}
	if createResp.Practice.Type != "water" {
		t.Errorf("type: want water, got %s", createResp.Practice.Type)
	}
	if createResp.Practice.Strokes.Free != 3000 {
		t.Errorf("strokes.free: want 3000, got %d", createResp.Practice.Strokes.Free)
	}
	practiceID := createResp.Practice.ID

	// List
	rr = doTrainingReq(router, http.MethodGet, "/api/v1/practices", coachToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list practices: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var listResp struct {
		Practices []struct {
			ID string `json:"id"`
		} `json:"practices"`
	}
	json.NewDecoder(rr.Body).Decode(&listResp)
	if len(listResp.Practices) != 1 {
		t.Errorf("list: want 1, got %d", len(listResp.Practices))
	}

	// Get
	rr = doTrainingReq(router, http.MethodGet, "/api/v1/practices/"+practiceID, coachToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("get practice: want 200, got %d", rr.Code)
	}

	// Update
	updateBody := `{"description": "Updated — sprint set 20x50"}`
	rr = doTrainingReq(router, http.MethodPut, "/api/v1/practices/"+practiceID, coachToken, orgID, updateBody)
	if rr.Code != http.StatusOK {
		t.Fatalf("update practice: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var updateResp struct {
		Practice struct {
			Description string `json:"description"`
		} `json:"practice"`
	}
	json.NewDecoder(rr.Body).Decode(&updateResp)
	if updateResp.Practice.Description != "Updated — sprint set 20x50" {
		t.Errorf("description after update: got %q", updateResp.Practice.Description)
	}

	// Delete
	rr = doTrainingReq(router, http.MethodDelete, "/api/v1/practices/"+practiceID, coachToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("delete practice: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify gone
	rr = doTrainingReq(router, http.MethodGet, "/api/v1/practices/"+practiceID, coachToken, orgID, "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("get after delete: want 404, got %d", rr.Code)
	}
}

// TestPracticeRoleEnforcement verifies that parents and swimmers can read
// practices but cannot create, update, or delete them.
func TestPracticeRoleEnforcement(t *testing.T) {
	svc := training.NewService()
	router := newTrainingRouter(t, svc)
	orgID := "org-training-roles"

	coachToken := issueTrainingToken(t, router, "coach-1", orgID, "coach")
	parentToken := issueTrainingToken(t, router, "parent-1", orgID, "parent")
	swimToken := issueTrainingToken(t, router, "swimmer-1", orgID, "swimmer")

	// Coach creates a practice
	body := `{"date":"2026-03-25","group_id":"grp1","type":"dryland","description":"Core circuit"}`
	rr := doTrainingReq(router, http.MethodPost, "/api/v1/practices", coachToken, orgID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("coach create: want 201, got %d", rr.Code)
	}
	var resp struct {
		Practice struct {
			ID string `json:"id"`
		} `json:"practice"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	id := resp.Practice.ID

	// Parent can list
	rr = doTrainingReq(router, http.MethodGet, "/api/v1/practices", parentToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Errorf("parent list: want 200, got %d", rr.Code)
	}

	// Swimmer can list
	rr = doTrainingReq(router, http.MethodGet, "/api/v1/practices", swimToken, orgID, "")
	if rr.Code != http.StatusOK {
		t.Errorf("swimmer list: want 200, got %d", rr.Code)
	}

	// Parent cannot create
	rr = doTrainingReq(router, http.MethodPost, "/api/v1/practices", parentToken, orgID, body)
	if rr.Code != http.StatusForbidden {
		t.Errorf("parent create: want 403, got %d", rr.Code)
	}

	// Swimmer cannot create
	rr = doTrainingReq(router, http.MethodPost, "/api/v1/practices", swimToken, orgID, body)
	if rr.Code != http.StatusForbidden {
		t.Errorf("swimmer create: want 403, got %d", rr.Code)
	}

	// Parent cannot update
	rr = doTrainingReq(router, http.MethodPut, "/api/v1/practices/"+id, parentToken, orgID, `{"description":"hacked"}`)
	if rr.Code != http.StatusForbidden {
		t.Errorf("parent update: want 403, got %d", rr.Code)
	}

	// Parent cannot delete
	rr = doTrainingReq(router, http.MethodDelete, "/api/v1/practices/"+id, parentToken, orgID, "")
	if rr.Code != http.StatusForbidden {
		t.Errorf("parent delete: want 403, got %d", rr.Code)
	}

}

// TestPracticeOrgIsolation verifies that practices from one org are not visible
// to another org's token.
func TestPracticeOrgIsolation(t *testing.T) {
	svc := training.NewService()
	router := newTrainingRouter(t, svc)
	orgA := "org-a-training"
	orgB := "org-b-training"

	tokenA := issueTrainingToken(t, router, "coach-a", orgA, "coach")
	tokenB := issueTrainingToken(t, router, "coach-b", orgB, "coach")

	// Org A creates practice
	body := `{"date":"2026-03-26","type":"video","description":"Team meeting"}`
	rr := doTrainingReq(router, http.MethodPost, "/api/v1/practices", tokenA, orgA, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("org A create: want 201, got %d", rr.Code)
	}

	// Org B sees empty list
	rr = doTrainingReq(router, http.MethodGet, "/api/v1/practices", tokenB, orgB, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("org B list: want 200, got %d", rr.Code)
	}
	var listResp struct {
		Practices []struct{ ID string } `json:"practices"`
	}
	json.NewDecoder(rr.Body).Decode(&listResp)
	if len(listResp.Practices) != 0 {
		t.Errorf("org B should see 0 practices, got %d", len(listResp.Practices))
	}
}

// TestPracticeInvalidType ensures that creating a practice with an unknown type
// returns a 400 Bad Request.
func TestPracticeInvalidType(t *testing.T) {
	svc := training.NewService()
	router := newTrainingRouter(t, svc)
	orgID := "org-training-invalid"

	token := issueTrainingToken(t, router, "coach-1", orgID, "coach")

	rr := doTrainingReq(router, http.MethodPost, "/api/v1/practices", token, orgID,
		`{"date":"2026-03-27","type":"swim","description":"bad type"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid type: want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
