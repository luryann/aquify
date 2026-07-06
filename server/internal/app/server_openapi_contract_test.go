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

	appopenapi "github.com/dareaquatics/aquify/server/internal/openapi"
	"go.yaml.in/yaml/v2"
)

func loadOpenAPISpec(t *testing.T) string {
	t.Helper()

	if err := appopenapi.Validate(); err != nil {
		t.Fatalf("validate packaged OpenAPI spec: %v", err)
	}
	return appopenapi.String()
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
		"message: You are not allowed to access this resource.",
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
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
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
		if env.Error.Code != "forbidden" || env.Error.Message != "You are not allowed to access this resource." {
			t.Fatalf("forbidden response drifted from OpenAPI example: %#v", env.Error)
		}
	})
}

func TestOpenAPIEndpointServesPackagedSpecOutsideRepoLayout(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWD); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	})

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != appopenapi.String() {
		t.Fatal("expected /api/v1/openapi to serve the packaged OpenAPI spec")
	}
}

func TestOpenAPIAuthSessionResponsesOmitTokenSecrets(t *testing.T) {
	t.Parallel()

	var spec map[any]any
	if err := yaml.Unmarshal([]byte(loadOpenAPISpec(t)), &spec); err != nil {
		t.Fatalf("decode packaged OpenAPI spec: %v", err)
	}

	for _, schemaName := range []string{"OIDCCallbackResponse", "SessionResponse"} {
		if schemaContainsProperty(t, spec, "#/components/schemas/"+schemaName, "access_token", nil) {
			t.Fatalf("%s response schema exposes access_token", schemaName)
		}
		if schemaContainsProperty(t, spec, "#/components/schemas/"+schemaName, "refresh_token", nil) {
			t.Fatalf("%s response schema exposes refresh_token", schemaName)
		}
	}
}

func schemaContainsProperty(t *testing.T, spec map[any]any, ref string, propertyName string, seen map[string]bool) bool {
	t.Helper()

	if seen == nil {
		seen = make(map[string]bool)
	}
	if seen[ref] {
		return false
	}
	seen[ref] = true

	schema, ok := resolveOpenAPIRef(spec, ref).(map[any]any)
	if !ok {
		t.Fatalf("expected %s to resolve to an object schema", ref)
	}
	return schemaNodeContainsProperty(t, spec, schema, propertyName, seen)
}

func schemaNodeContainsProperty(t *testing.T, spec map[any]any, node any, propertyName string, seen map[string]bool) bool {
	t.Helper()

	switch typed := node.(type) {
	case map[any]any:
		if ref, ok := typed["$ref"].(string); ok {
			return schemaContainsProperty(t, spec, ref, propertyName, seen)
		}
		if properties, ok := typed["properties"].(map[any]any); ok {
			if _, ok := properties[propertyName]; ok {
				return true
			}
			for _, child := range properties {
				if schemaNodeContainsProperty(t, spec, child, propertyName, seen) {
					return true
				}
			}
		}
		for _, key := range []string{"items", "additionalProperties"} {
			if child, ok := typed[key]; ok && schemaNodeContainsProperty(t, spec, child, propertyName, seen) {
				return true
			}
		}
		for _, key := range []string{"allOf", "anyOf", "oneOf"} {
			if children, ok := typed[key].([]any); ok {
				for _, child := range children {
					if schemaNodeContainsProperty(t, spec, child, propertyName, seen) {
						return true
					}
				}
			}
		}
	case []any:
		for _, child := range typed {
			if schemaNodeContainsProperty(t, spec, child, propertyName, seen) {
				return true
			}
		}
	}
	return false
}

func resolveOpenAPIRef(spec map[any]any, ref string) any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}

	var current any = spec
	for _, segment := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		next, ok := current.(map[any]any)
		if !ok {
			return nil
		}
		current = next[segment]
	}
	return current
}
