package handlers

import (
	"net/http"

	appopenapi "github.com/dareaquatics/aquify/server/internal/openapi"
)

// OpenAPIStub serves the packaged OpenAPI 3.1 spec.
func OpenAPIStub(w http.ResponseWriter, r *http.Request) {
	if err := appopenapi.Validate(); err != nil {
		http.Error(w, "OpenAPI spec not available", http.StatusServiceUnavailable)
		return
	}
	data := appopenapi.Bytes()

	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
