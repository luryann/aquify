// Package deploycheck holds repository-level checks on deployment files that
// run without Docker: they catch config mistakes a strict parser or a reviewer
// would otherwise only find on the target host.
package deploycheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	// internal/deploycheck -> repo root
	return filepath.Join("..", "..", "..", filepath.FromSlash(rel))
}

func loadYAML(t *testing.T, rel string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(repoFile(t, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	var doc map[string]any
	// yaml.v3 rejects duplicate mapping keys, including repeated `<<` merge keys,
	// which looser parsers accept and Docker Compose may not.
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%s is not valid strict YAML: %v", rel, err)
	}
	return doc
}

func TestComposeFilesParseStrictly(t *testing.T) {
	for _, rel := range []string{
		"docker-compose.yml",
		"deploy/docker-compose.production.yml",
		"deploy/docker-compose.staging.yml",
	} {
		t.Run(rel, func(t *testing.T) {
			doc := loadYAML(t, rel)
			if _, ok := doc["services"].(map[string]any); !ok {
				t.Fatalf("%s has no services map", rel)
			}
		})
	}
}

func TestProductionServicesInheritRestartAndLogging(t *testing.T) {
	doc := loadYAML(t, "deploy/docker-compose.production.yml")
	services := doc["services"].(map[string]any)
	for _, name := range []string{"aquify-api", "aquify-admin", "aquify-worker", "keycloak"} {
		svc := services[name].(map[string]any)
		if svc["restart"] != "always" {
			t.Errorf("%s: restart = %v, want always (merge key lost)", name, svc["restart"])
		}
		if _, ok := svc["logging"]; !ok {
			t.Errorf("%s: logging config missing (merge key lost)", name)
		}
	}
}

func TestWorkflowFilesParseStrictly(t *testing.T) {
	for _, rel := range []string{
		".github/workflows/ci.yml",
		".github/workflows/deploy-staging.yml",
		".github/workflows/deploy-production.yml",
	} {
		t.Run(rel, func(t *testing.T) { loadYAML(t, rel) })
	}
}

// Workflow inputs must never be interpolated into script text.
func TestProductionWorkflowDoesNotInterpolateInputsIntoShell(t *testing.T) {
	b, err := os.ReadFile(repoFile(t, ".github/workflows/deploy-production.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "github.event.inputs.") && !strings.HasPrefix(trimmed, "IMAGE_TAG_INPUT:") &&
			!strings.HasPrefix(trimmed, "CONFIRM_INPUT:") && !strings.Contains(line, `"value": "${{ github.event.inputs.image_tag }}"`) {
			t.Errorf("line %d interpolates a workflow input: %s", i+1, trimmed)
		}
	}
}
