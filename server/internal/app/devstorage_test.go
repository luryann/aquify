package app

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/go-chi/chi/v5"
)

func TestDevStorageHandlerServesPresignedStubURLs(t *testing.T) {
	t.Parallel()

	store := storage.NewStubStorage(devStoragePath)
	router := chi.NewRouter()
	router.Handle(devStoragePath+"/*", devStorageHandler(store))

	putURL, _ := store.PresignedPutURL("docs/org/a b.pdf", time.Minute)
	if !strings.HasPrefix(putURL, devStoragePath+"/") {
		t.Fatalf("presigned URL should target the dev route, got %q", putURL)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, putURL, bytes.NewBufferString("%PDF"))
	req.Header.Set("Content-Type", "application/pdf")
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rr.Code, rr.Body.String())
	}
	if ok, _ := store.Exists(req.Context(), "docs/org/a b.pdf"); !ok {
		t.Fatal("object not stored under the original key")
	}

	// A PUT-signed URL cannot be used to read.
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, putURL, nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("GET with PUT signature: want 403, got %d", rr.Code)
	}

	getURL, _ := store.PresignedGetURL("docs/org/a b.pdf", time.Minute)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, getURL, nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "%PDF" || rr.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("GET: %d %q %q", rr.Code, rr.Body.String(), rr.Header().Get("Content-Type"))
	}

	expired, _ := store.PresignedGetURL("docs/org/a b.pdf", -time.Minute)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, expired, nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expired GET: want 403, got %d", rr.Code)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestDevStorageDistinguishesReadErrorFromTooLarge(t *testing.T) {
	store := storage.NewStubStorage(devStoragePath)
	router := chi.NewRouter()
	router.Handle(devStoragePath+"/*", devStorageHandler(store))
	putURL, _ := store.PresignedPutURL("docs/x.bin", time.Minute)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, putURL, failingReader{}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("read error: want 400, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, putURL, bytes.NewReader(make([]byte, maxDevStorageObjectBytes+1))))
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize: want 413, got %d", rr.Code)
	}
}
