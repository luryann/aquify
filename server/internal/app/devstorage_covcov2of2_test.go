package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/storage"
)

type cov2xFailingReader struct{}

func (cov2xFailingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func cov2xDevStorageServe(store *storage.StubStorage, wildcard, target, method string, body *strings.Reader) *httptest.ResponseRecorder {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("*", wildcard)
	req := httptest.NewRequest(method, target, body)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	devStorageHandler(store).ServeHTTP(rr, req)
	return rr
}

func TestCov2xDevStorageRejectsBadKeys(t *testing.T) {
	t.Parallel()
	store := storage.NewStubStorage(devStoragePath)
	for name, key := range map[string]string{"bad escape": "%zz", "blank": "   ", "empty": ""} {
		rr := cov2xDevStorageServe(store, key, devStoragePath+"/x", http.MethodGet, strings.NewReader(""))
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invalid_key") {
			t.Errorf("%s: want 400 invalid_key, got %d %s", name, rr.Code, rr.Body.String())
		}
	}
}

func TestCov2xDevStorageExpiredAndUnsigned(t *testing.T) {
	t.Parallel()
	store := storage.NewStubStorage(devStoragePath)
	past := time.Now().Add(-time.Minute).Unix()
	for name, target := range map[string]string{
		"missing expiry": devStoragePath + "/k?X-Stub-Method=GET",
		"bad expiry":     devStoragePath + "/k?X-Stub-Method=GET&X-Stub-Expires=soon",
		"expired":        devStoragePath + "/k?X-Stub-Method=GET&X-Stub-Expires=" + strconv.FormatInt(past, 10),
	} {
		rr := cov2xDevStorageServe(store, "k", target, http.MethodGet, strings.NewReader(""))
		if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "url_expired") {
			t.Errorf("%s: want 403 url_expired, got %d %s", name, rr.Code, rr.Body.String())
		}
	}
}

func TestCov2xDevStorageReadFailureAndUnsupportedMethod(t *testing.T) {
	t.Parallel()
	store := storage.NewStubStorage(devStoragePath)
	exp := time.Now().Add(time.Minute).Unix()
	putURL, err := store.PresignedPutURL("docs/fail.bin", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("*", "docs/fail.bin")
	req := httptest.NewRequest(http.MethodPut, putURL, cov2xFailingReader{})
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	devStorageHandler(store).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "read_failed") {
		t.Fatalf("want 400 read_failed, got %d %s", rr.Code, rr.Body.String())
	}
	if ok, _ := store.Exists(context.Background(), "docs/fail.bin"); ok {
		t.Fatal("a failed upload must not leave an object behind")
	}

	// A method that is "signed" but not supported is rejected with 405.
	target := devStoragePath + "/docs/x?X-Stub-Method=DELETE&X-Stub-Expires=" + strconv.FormatInt(exp, 10)
	rr = cov2xDevStorageServe(store, "docs/x", target, http.MethodDelete, strings.NewReader(""))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rr.Code)
	}
}

func TestCov2xDevStorageGetMissingObject(t *testing.T) {
	t.Parallel()
	store := storage.NewStubStorage(devStoragePath)
	getURL, _ := store.PresignedGetURL("nope/missing.txt", time.Minute)
	rr := cov2xDevStorageServe(store, "nope/missing.txt", getURL, http.MethodGet, strings.NewReader(""))
	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "object_not_found") {
		t.Fatalf("want 404, got %d %s", rr.Code, rr.Body.String())
	}
}
