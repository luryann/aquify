package members

import (
	"os"
	"strings"
	"testing"
)

func TestMembershipMigrationsEnforceOrgScopedForeignKeys(t *testing.T) {
	t.Parallel()

	assertFileContains(t, "../../../../db/migrations/20260315001000_create_contacts.sql", "FOREIGN KEY (org_id, household_id) REFERENCES households (org_id, id)")
	assertFileContains(t, "../../../../db/migrations/20260315001100_create_swimmers.sql", "FOREIGN KEY (org_id, household_id) REFERENCES households (org_id, id)")
	assertFileContains(t, "../../../../db/migrations/20260315001200_create_swimmer_groups.sql", "FOREIGN KEY (org_id, swimmer_id) REFERENCES swimmers (org_id, id)")
}

func TestSwimmerExternalIDMigrationUsesCaseInsensitiveUniqueness(t *testing.T) {
	t.Parallel()

	assertFileContains(t, "../../../../db/migrations/20260315001300_add_swimmer_uniqueness_constraints.sql", "ON swimmers (org_id, lower(swimmer_external_id))")
}

func assertFileContains(t *testing.T, path, expected string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	if !strings.Contains(string(raw), expected) {
		t.Fatalf("expected %s to contain %q", path, expected)
	}
}
