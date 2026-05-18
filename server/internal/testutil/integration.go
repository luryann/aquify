package testutil

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// DBHarness executes real SQL against DATABASE_URL through psql so integration
// tests can exercise an actual database without coupling the app packages to a
// concrete driver yet.
type DBHarness struct {
	DatabaseURL string
}

// RequireDBHarness returns a DATABASE_URL-backed harness or skips when the
// environment is not provisioned for DB integration tests.
func RequireDBHarness(t *testing.T) *DBHarness {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set; skipping DB integration harness test")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql is not installed; skipping DB integration harness test")
	}

	return &DBHarness{DatabaseURL: databaseURL}
}

// Exec runs a SQL statement through psql and returns trimmed stdout.
func (h *DBHarness) Exec(t *testing.T, sql string) string {
	t.Helper()

	cmd := exec.Command("psql", h.DatabaseURL, "-v", "ON_ERROR_STOP=1", "-At", "-c", sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("psql exec failed: %v\nsql: %s\noutput:\n%s", err, sql, string(out))
	}
	return strings.TrimSpace(string(out))
}
