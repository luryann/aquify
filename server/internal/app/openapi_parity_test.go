package app

import (
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/openapi"
	"github.com/go-chi/chi/v5"
)

var (
	specPathLine   = regexp.MustCompile(`^  (/\S*):\s*$`)
	specMethodLine = regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	specParamNames = regexp.MustCompile(`\{[^}]+\}`)
)

// normalizeOperation collapses path parameter names, which the spec and the
// router name differently (`{id}` vs `{practiceID}`), into `{}`.
func normalizeOperation(method, path string) string {
	path = strings.TrimSuffix(path, "/*")
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}
	return strings.ToUpper(method) + " " + specParamNames.ReplaceAllString(path, "{}")
}

// TestOpenAPIMatchesRegisteredRoutes keeps docs/api/openapi.yaml honest: every
// /api/v1 route the router serves must be documented, and nothing may be
// documented that the router does not serve.
func TestOpenAPIMatchesRegisteredRoutes(t *testing.T) {
	t.Parallel()

	documented := map[string]bool{}
	inPaths := false
	current := ""
	for _, line := range strings.Split(openapi.String(), "\n") {
		if line == "paths:" {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		if line != "" && line[0] != ' ' && line[0] != '#' {
			break
		}
		if m := specPathLine.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if m := specMethodLine.FindStringSubmatch(line); m != nil && current != "" {
			documented[normalizeOperation(m[1], current)] = true
		}
	}
	if len(documented) == 0 {
		t.Fatal("found no operations in the packaged OpenAPI spec")
	}

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	served := map[string]bool{}
	err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if rest, ok := strings.CutPrefix(route, "/api/v1"); ok {
			served[normalizeOperation(method, rest)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}

	var undocumented, stale []string
	for op := range served {
		if !documented[op] {
			undocumented = append(undocumented, op)
		}
	}
	for op := range documented {
		if !served[op] {
			stale = append(stale, op)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(stale)
	if len(undocumented) > 0 {
		t.Errorf("routes missing from docs/api/openapi.yaml:\n  %s", strings.Join(undocumented, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("operations documented but not served:\n  %s", strings.Join(stale, "\n  "))
	}
}
