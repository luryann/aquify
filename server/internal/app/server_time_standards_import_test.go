package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

func TestTimeStandardsImportRolesPreviewAndCommit(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, RouterDeps{Performance: performance.NewService()})
	org := "org-tsi"
	admin := issueSessionTokenForUser(t, router, "admin-user", org, []string{"admin"}, true)
	coach := issueSessionTokenForUser(t, router, "coach-user", org, []string{"coach"}, false)
	parent := issueSessionTokenForUser(t, router, "parent-user", org, []string{"parent"}, false)

	csv := "standard_name,standard_code,season_year,course,gender,age_group,event_distance,stroke,cut_time\n" +
		"2026 Cuts,A,2026,SCY,F,11-12,100,free,58.12\n2026 Cuts,A,2026,SCY,M,11-12,100,free,57.00\n"
	bad := csv + "2026 Cuts,A,2026,NOPE,M,11-12,50,free,27.00\n"

	post := func(token, path, filename, body string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", filename)
		_, _ = fw.Write([]byte(body))
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, path, &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("X-Org-Id", org)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	count := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/time-standards", nil)
		req.Header.Set("X-Org-Id", org)
		req.Header.Set("Authorization", "Bearer "+admin)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		var out struct{ Count int }
		_ = json.Unmarshal(rr.Body.Bytes(), &out)
		return out.Count
	}

	for _, tok := range []string{coach, parent} {
		if rr := post(tok, "/api/v1/time-standards/import?preview=true", "s.csv", csv); rr.Code != http.StatusForbidden {
			t.Fatalf("non-admin preview = %d", rr.Code)
		}
		if rr := post(tok, "/api/v1/time-standards/import", "s.csv", csv); rr.Code != http.StatusForbidden {
			t.Fatalf("non-admin commit = %d", rr.Code)
		}
	}
	if rr := post(admin, "/api/v1/time-standards/import", "s.xlsx", csv); rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("xlsx = %d", rr.Code)
	}

	rr := post(admin, "/api/v1/time-standards/import?preview=true", "s.csv", bad)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rr.Code, rr.Body)
	}
	var pv struct {
		Preview bool
		Counts  performance.StandardsImportCounts
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &pv)
	if !pv.Preview || pv.Counts.Total != 3 || pv.Counts.Errors != 1 || pv.Counts.Create != 2 {
		t.Fatalf("preview counts = %+v", pv)
	}
	if count() != 0 {
		t.Fatal("preview wrote data")
	}

	if rr := post(admin, "/api/v1/time-standards/import", "s.csv", bad); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad commit = %d %s", rr.Code, rr.Body)
	}
	if count() != 0 {
		t.Fatal("rejected commit wrote data")
	}

	rr = post(admin, "/api/v1/time-standards/import", "s.csv", csv)
	if rr.Code != http.StatusOK {
		t.Fatalf("commit = %d %s", rr.Code, rr.Body)
	}
	if count() != 2 {
		t.Fatalf("count = %d", count())
	}
	// Re-commit is idempotent.
	if rr = post(admin, "/api/v1/time-standards/import", "s.csv", csv); rr.Code != http.StatusOK || count() != 2 {
		t.Fatalf("recommit = %d count %d", rr.Code, count())
	}
}
