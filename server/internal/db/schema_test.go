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
		"billing_charges.released_at",
		"billing_settings.late_fee_grace_days",
		"calendar_feed_tokens.token_hash",
		"club_event_rsvps.billed_quantity",
		"club_events.fee_cents",
		"club_events.rsvp_mode",
		"data_deletion_requests.deleted_at",
		"data_deletion_requests.expires_at",
		"data_export_requests.deleted_at",
		"email_campaigns.deleted_at",
		"entries.entry_time_ms",
		"entries.notes",
		"entries.scratch_reason",
		"file_exports.expires_at",
		"file_exports.last_error",
		"ledger_events.household_id",
		"meet_sessions.warm_up_time",
		"meets.max_entries_per_swimmer",
		"meets.self_signup_enabled",
		"meets.venue",
		"messages.edited_at",
		"org_settings.email_sender_address",
		"org_settings.email_sender_name",
		"org_settings.lsc_code",
		"org_settings.team_code",
		"org_settings.team_name",
		"payments.household_id",
		"practices.workout_id",
		"relay_lineups.relay_type",
		"relay_lineups.teams",
		"swimmer_goals.baseline_time_ms",
		"swimmer_goals.deadline",
		"swimmer_goals.target_time_ms",
		"swimmers.usa_swimming_id",
		"team_record_entries.age_group",
		"team_record_entries.locked",
		"team_record_entries.status",
		"team_record_entries.swim_date",
		"team_record_entries.verified",
		"training_plan_phases.weekly_yardage_target",
		"training_workouts.is_template",
		"training_workouts.sections",
		"volunteer_buyouts.invoice_id",
		"volunteer_buyouts.status",
		"volunteer_jobs.capacity",
		"volunteer_jobs.hour_credit",
		"volunteer_jobs.requires_certification",
		"volunteer_settings.cancellation_cutoff_hours",
		"volunteer_settings.timezone",
		"volunteer_signups.hours_credited",
		"volunteer_signups.volunteer_name",
		"waiver_acceptances.accepted_by",
		"wellness_checkins.sleep_quality",
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
		"meets":                  {"venue", "self_signup_enabled", "max_entries_per_swimmer"},
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
