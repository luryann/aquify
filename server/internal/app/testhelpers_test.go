package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
)

// doRequest sends a request through the router. Authorization, X-Org-Id and the
// JSON content type are set only when the matching argument is non-empty.
func doRequest(router http.Handler, method, path, token, body, orgID string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if orgID != "" {
		req.Header.Set("X-Org-Id", orgID)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// doOrgRequest sends a JSON request that always carries the bearer token and org header.
func doOrgRequest(router http.Handler, method, path, token, orgID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}
