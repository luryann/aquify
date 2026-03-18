package handlers

import (
	"net/http"
	"os"
)

// OpenAPIStub serves the OpenAPI 3.1 spec.
//
// The spec path defaults to "docs/api/openapi.yaml" (relative to the process
// working directory) and can be overridden by setting OPENAPI_SPEC_PATH to an
// absolute path. In production deployments, set OPENAPI_SPEC_PATH or run the
// binary from the repository root.
func OpenAPIStub(w http.ResponseWriter, r *http.Request) {
	specPath := os.Getenv("OPENAPI_SPEC_PATH")
	if specPath == "" {
		specPath = "docs/api/openapi.yaml"
	}
	data, err := os.ReadFile(specPath) //nolint:gosec // path comes from trusted env var or fixed default
	if err != nil {
		http.Error(w, "OpenAPI spec not found — set OPENAPI_SPEC_PATH or run from repo root", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
