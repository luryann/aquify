package openapi

import (
	"os"
	"testing"
)

func TestValidatePackagedSpec(t *testing.T) {
	t.Parallel()

	if err := Validate(); err != nil {
		t.Fatalf("expected packaged OpenAPI spec to validate, got %v", err)
	}
}

func TestPackagedSpecMatchesRepositorySource(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../docs/api/openapi.yaml")
	if err != nil {
		t.Fatalf("read repository OpenAPI spec: %v", err)
	}

	if string(raw) != String() {
		t.Fatal("packaged OpenAPI spec drifted from docs/api/openapi.yaml")
	}
}
