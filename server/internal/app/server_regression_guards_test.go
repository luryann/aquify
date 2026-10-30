package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

func TestUpdateTimeStandardRejectsInvalidValuesWith400(t *testing.T) {
	t.Parallel()
	router := newPerfRouter(t, performance.NewService())
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	created := do(http.MethodPost, "/api/v1/time-standards", minimalTimeStandardBody("AA"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", created.Code, created.Body.String())
	}
	var env timeStandardEnvelope
	if err := json.NewDecoder(created.Body).Decode(&env); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	for _, body := range []string{`{"time_ms":0}`, `{"time_ms":-5}`, `{"name":"   "}`} {
		rr := do(http.MethodPut, "/api/v1/time-standards/"+env.TimeStandard.ID, body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: expected 400, got %d: %s", body, rr.Code, rr.Body.String())
		}
	}
}

// The cap exists to bound memory/temp-disk use during multipart parsing; the
// observable contract is that an oversized upload is refused with 413.
func TestParseMeetFileRejectsOversizedUpload(t *testing.T) {
	t.Parallel()
	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	token := issueSessionToken(t, router, "org-parse", []string{"admin"}, false)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", "huge.sd3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(make([]byte, 27<<20)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/parse-entry-file", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Org-Id", "org-parse")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
}
