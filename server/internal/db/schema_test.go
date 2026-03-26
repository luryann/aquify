package db

import (
	"slices"
	"testing"
)

func TestMissingExpectedColumnsReturnsSortedMissingList(t *testing.T) {
	actual := map[string]map[string]bool{
		"eligibility_rules": {
			"rule_value": true,
		},
		"file_exports": {
			"filename":   true,
			"size_bytes": true,
		},
		"payments": {
			"payment_method": true,
		},
	}

	got := MissingExpectedColumns(actual)

	want := []string{
		"announcements.deleted_at",
		"data_deletion_requests.deleted_at",
		"data_deletion_requests.expires_at",
		"data_export_requests.deleted_at",
		"email_campaigns.deleted_at",
		"entries.entry_time_ms",
		"entries.notes",
		"file_exports.expires_at",
		"file_exports.last_error",
		"meet_sessions.warm_up_time",
		"meets.venue",
		"payments.household_id",
		"waiver_acceptances.accepted_by",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("MissingExpectedColumns() = %#v, want %#v", got, want)
	}
}

func TestMissingExpectedColumnsReturnsEmptyWhenSchemaMatches(t *testing.T) {
	actual := make(map[string]map[string]bool, len(expectedColumns))
	for table, columns := range expectedColumns {
		actual[table] = make(map[string]bool, len(columns))
		for _, column := range columns {
			actual[table][column] = true
		}
	}

	got := MissingExpectedColumns(actual)
	if len(got) != 0 {
		t.Fatalf("MissingExpectedColumns() = %#v, want empty", got)
	}
}

func TestExpectedColumnsCoversKnownDriftFixes(t *testing.T) {
	t.Parallel()

	required := map[string][]string{
		"announcements":          {"deleted_at"},
		"data_deletion_requests": {"deleted_at", "expires_at"},
		"data_export_requests":   {"deleted_at"},
		"email_campaigns":        {"deleted_at"},
		"entries":                {"entry_time_ms", "notes"},
		"eligibility_rules":      {"rule_value"},
		"file_exports":           {"expires_at", "filename", "last_error", "size_bytes"},
		"meets":                  {"venue"},
		"meet_sessions":          {"warm_up_time"},
		"payments":               {"household_id", "payment_method"},
		"waiver_acceptances":     {"accepted_by"},
	}

	for table, columns := range required {
		gotColumns := expectedColumns[table]
		for _, column := range columns {
			if !slices.Contains(gotColumns, column) {
				t.Fatalf("expectedColumns[%q] missing %q", table, column)
			}
		}
	}
}
