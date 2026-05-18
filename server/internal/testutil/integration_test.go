package testutil

import "testing"

func TestDBHarnessExecutesAgainstConfiguredDatabase(t *testing.T) {
	t.Parallel()

	harness := RequireDBHarness(t)

	if got := harness.Exec(t, "select current_database();"); got == "" {
		t.Fatal("expected current_database() to return a non-empty value")
	}

	// Temporary tables keep the harness side-effect free while still proving
	// that setup, write, and read paths work against a real database.
	got := harness.Exec(t, `
		create temporary table if not exists codex_step23_integration_test (value integer);
		truncate codex_step23_integration_test;
		insert into codex_step23_integration_test(value) values (1), (2), (3);
		select count(*) from codex_step23_integration_test;
	`)
	if got != "3" {
		t.Fatalf("expected temp table round-trip count 3, got %q", got)
	}
}
