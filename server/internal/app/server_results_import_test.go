package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

func TestMeetResultsImportMatchesSwimmersAndIsIdempotent(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	membersSvc := members.NewService()
	meetsSvc := meets.NewService()
	perfSvc := performance.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Members:     membersSvc,
		Meets:       meetsSvc,
		Performance: perfSvc,
	})
	coachToken := issueSessionTokenForUser(t, router, "coach-user", "org-results", []string{"coach"}, false)
	adminToken := issueSessionTokenForUser(t, router, "admin-user", "org-results", []string{"admin"}, true)
	parentToken := issueSessionTokenForUser(t, router, "parent-user", "org-results", []string{"parent"}, false)

	doJSON := func(token, method, path string, body map[string]any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("X-Org-Id", "org-results")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	fixture, err := os.ReadFile("../../../internal/meetfiles/testdata/fixtures/valid.sd3")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	upload := func(token, meetID string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "results.sd3")
		_, _ = fw.Write(fixture)
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/results/import", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("X-Org-Id", "org-results")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	var hh struct {
		Household struct{ ID string } `json:"household"`
	}
	if err := json.Unmarshal(doJSON(adminToken, http.MethodPost, "/api/v1/members/households", map[string]any{"org_id": "org-results", "household_name": "Smith"}).Body.Bytes(), &hh); err != nil {
		t.Fatalf("household: %v", err)
	}
	var sw struct {
		Swimmer struct{ ID string } `json:"swimmer"`
	}
	if err := json.Unmarshal(doJSON(adminToken, http.MethodPost, "/api/v1/members/swimmers", map[string]any{
		"org_id": "org-results", "household_id": hh.Household.ID, "first_name": "John", "last_name": "Smith",
		"date_of_birth": "1999-02-15", "gender": "male",
	}).Body.Bytes(), &sw); err != nil || sw.Swimmer.ID == "" {
		t.Fatalf("swimmer: %v", err)
	}
	meetID := createMeet(t, router, "org-results", adminToken, "Results Meet")

	if rr := upload(parentToken, meetID); rr.Code != http.StatusForbidden {
		t.Fatalf("parent import: want 403, got %d", rr.Code)
	}

	type summary struct {
		Imported   int `json:"imported"`
		Duplicates int `json:"duplicates"`
		Unmatched  []struct {
			Name string `json:"name"`
		} `json:"unmatched"`
	}
	rr := upload(coachToken, meetID)
	if rr.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rr.Code, rr.Body.String())
	}
	var first summary
	_ = json.Unmarshal(rr.Body.Bytes(), &first)
	if first.Imported != 1 || len(first.Unmatched) != 1 || !strings.Contains(first.Unmatched[0].Name, "Jane") {
		t.Fatalf("unexpected first import summary: %s", rr.Body.String())
	}

	var second summary
	rr = upload(coachToken, meetID)
	_ = json.Unmarshal(rr.Body.Bytes(), &second)
	if second.Imported != 0 || second.Duplicates != 1 {
		t.Fatalf("re-import should only report duplicates: %s", rr.Body.String())
	}

	bt := doJSON(coachToken, http.MethodGet, "/api/v1/swimmers/"+sw.Swimmer.ID+"/best-times?course=SCY", nil)
	if bt.Code != http.StatusOK || !strings.Contains(bt.Body.String(), "58120") {
		t.Fatalf("expected 58.12 best time from import, got %d %s", bt.Code, bt.Body.String())
	}
}
