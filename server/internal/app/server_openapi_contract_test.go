package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func loadOpenAPISpec(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile("../../../docs/api/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI spec: %v", err)
	}
	return string(raw)
}

func TestOpenAPIExampleContractsForCommonErrorResponses(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t)
	for _, snippet := range []string{
		"code: validation_failed",
		"message: One or more request fields are invalid.",
		"code: unauthorized",
		"message: Authentication context is missing.",
		"code: forbidden",
		"message: You are not allowed to perform this action.",
	} {
		if !strings.Contains(spec, snippet) {
			t.Fatalf("expected OpenAPI spec to contain example snippet %q", snippet)
		}
	}

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	t.Run("unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
		req.Header.Set("X-Org-Id", "org-contract")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
		var env apiErrorEnvelope
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode unauthorized response: %v", err)
		}
		if env.Error.Code != "unauthorized" {
			t.Fatalf("expected unauthorized code, got %q", env.Error.Code)
		}
	})

	t.Run("validation_failed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode validation response: %v", err)
		}
		if env.Error.Code != "validation_failed" || env.Error.Message != "One or more request fields are invalid." {
			t.Fatalf("validation response drifted from OpenAPI example: %#v", env.Error)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		parentToken := issueSessionToken(t, router, "org-contract", []string{"parent"}, false)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/upload-url", encodeJSONBody(t, map[string]any{
			"filename":   "contract.sd3",
			"file_type":  "sd3",
			"size_bytes": 1024,
			"mime_type":  "application/octet-stream",
		}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-contract")
		req.Header.Set("Authorization", "Bearer "+parentToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
		}
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode forbidden response: %v", err)
		}
		if env.Error.Code != "forbidden" || env.Error.Message != "You are not allowed to perform this action." {
			t.Fatalf("forbidden response drifted from OpenAPI example: %#v", env.Error)
		}
	})
}
