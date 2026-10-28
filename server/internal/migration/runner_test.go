package migration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/testutil"
)

func TestRunnerDryRunImportsPrimaryStep24Domains(t *testing.T) {
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_email,status\nhh-1,Smith Family,smith@example.com,active\n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\nsw-1,hh-1,Ava,Smith,2012-05-01,F,active\n")
	writeCSVFixture(t, dir, "groups.csv", "legacy_group_code,name,type\nsenior,Senior,competitive\n")
	writeCSVFixture(t, dir, "historical_best_times.csv", "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,recorded_at\nbt-1,sw-1,100,free,SCY,65000,2025-02-01\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\ninv-1,INV-100,hh-1,12000,2000,2026-03-01\n")
	writeCSVFixture(t, dir, "payment_history.csv", "legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at\npay-1,inv-1,10000,check,2026-02-15\n")

	runner := NewRunner(Config{OrgID: testutil.DefaultOrgID, ActorID: "migration-test"})
	report, err := runner.DryRun(context.Background(), dir)
	if err != nil {
		t.Fatalf("DryRun returned error: %v", err)
	}

	if len(report.Steps) != 6 {
		t.Fatalf("expected 6 step reports, got %d", len(report.Steps))
	}

	households := runner.MembersService().SnapshotHouseholds(testutil.DefaultOrgID)
	if len(households) != 1 {
		t.Fatalf("expected 1 household, got %d", len(households))
	}

	swimmers := runner.MembersService().SnapshotSwimmers(testutil.DefaultOrgID)
	if len(swimmers) != 1 {
		t.Fatalf("expected 1 swimmer, got %d", len(swimmers))
	}

	groupRows := runner.GroupsService().SnapshotGroups(testutil.DefaultOrgID)
	if len(groupRows) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groupRows))
	}

	bestTimes, err := runner.PerformanceService().ListBestTimes(testutil.DefaultOrgID, testutil.AdminPrincipal(), swimmers[0].ID, performance.ListBestTimesFilters{})
	if err != nil {
		t.Fatalf("ListBestTimes returned error: %v", err)
	}
	if len(bestTimes) != 1 {
		t.Fatalf("expected 1 best time, got %d", len(bestTimes))
	}

	invoices := runner.BillingService().SnapshotInvoices(testutil.DefaultOrgID)
	if len(invoices) != 1 {
		t.Fatalf("expected 1 invoice, got %d", len(invoices))
	}
	if invoices[0].Status != "partially_paid" {
		t.Fatalf("expected partially_paid invoice, got %s", invoices[0].Status)
	}

	payments := runner.BillingService().SnapshotPayments(testutil.DefaultOrgID)
	if len(payments) != 1 {
		t.Fatalf("expected 1 payment, got %d", len(payments))
	}
	if payments[0].PaidAt == nil || payments[0].PaidAt.Format("2006-01-02") != "2026-02-15" {
		t.Fatalf("expected migrated payment paid_at to be preserved, got %+v", payments[0].PaidAt)
	}
	if payments[0].Notes != "" {
		t.Fatalf("expected payment notes to remain empty when no notes are provided, got %q", payments[0].Notes)
	}
}

func TestRunnerDryRunSkipsDuplicateIdempotencyKeys(t *testing.T) {
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_email,status\nhh-1,Smith Family,smith@example.com,active\nhh-1,Smith Family,smith@example.com,active\n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\n")
	writeCSVFixture(t, dir, "groups.csv", "legacy_group_code,name,type\n")
	writeCSVFixture(t, dir, "historical_best_times.csv", "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,recorded_at\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\n")
	writeCSVFixture(t, dir, "payment_history.csv", "legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at\n")

	report, err := NewRunner(Config{OrgID: testutil.DefaultOrgID}).DryRun(context.Background(), dir)
	if err != nil {
		t.Fatalf("DryRun returned error: %v", err)
	}

	var householdStep StepReport
	for _, step := range report.Steps {
		if step.Name == StepHouseholds {
			householdStep = step
			break
		}
	}

	if householdStep.Imported != 1 {
		t.Fatalf("expected 1 imported household, got %d", householdStep.Imported)
	}
	if householdStep.Duplicates != 1 {
		t.Fatalf("expected 1 duplicate household, got %d", householdStep.Duplicates)
	}
	if householdStep.Skipped != 1 {
		t.Fatalf("expected 1 skipped household, got %d", householdStep.Skipped)
	}
}

func TestRunnerDryRunReportsUnknownHouseholdReference(t *testing.T) {
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_email,status\nhh-1,Smith Family,smith@example.com,active\n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\nsw-1,missing,Ava,Smith,2012-05-01,F,active\n")
	writeCSVFixture(t, dir, "groups.csv", "legacy_group_code,name,type\n")
	writeCSVFixture(t, dir, "historical_best_times.csv", "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,recorded_at\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\n")
	writeCSVFixture(t, dir, "payment_history.csv", "legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at\n")

	report, err := NewRunner(Config{OrgID: testutil.DefaultOrgID}).DryRun(context.Background(), dir)
	if err == nil {
		t.Fatal("expected DryRun to fail when swimmer import references an unknown household")
	}

	var swimmerStep StepReport
	for _, step := range report.Steps {
		if step.Name == StepSwimmers {
			swimmerStep = step
			break
		}
	}

	if len(swimmerStep.Errors) == 0 {
		t.Fatal("expected swimmer step to report unknown household error")
	}
}

func TestRunnerDryRunRejectsMissingRequiredHeaders(t *testing.T) {
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_email,status\nhh-1,Smith Family,smith@example.com,active\n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\n")
	writeCSVFixture(t, dir, "groups.csv", "legacy_group_code,name,type\n")
	writeCSVFixture(t, dir, "historical_best_times.csv", "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\n")
	writeCSVFixture(t, dir, "payment_history.csv", "legacy_payment_id,legacy_invoice_id,amount_cents,paid_at\n")

	report, err := NewRunner(Config{OrgID: testutil.DefaultOrgID}).DryRun(context.Background(), dir)
	if err == nil {
		t.Fatal("expected DryRun to fail when a required header is missing")
	}

	var paymentStep StepReport
	for _, step := range report.Steps {
		if step.Name == StepPaymentHistory {
			paymentStep = step
			break
		}
	}
	if len(paymentStep.Errors) != 1 {
		t.Fatalf("expected one payment_history error, got %v", paymentStep.Errors)
	}
	if got := paymentStep.Errors[0]; got != "payment_history.csv missing required headers: method" {
		t.Fatalf("unexpected payment_history error %q", got)
	}
}

func TestRunnerDryRunIgnoresBlankRows(t *testing.T) {
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_email,status\n\nhh-1,Smith Family,smith@example.com,active\n  , , , \n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\n")
	writeCSVFixture(t, dir, "groups.csv", "legacy_group_code,name,type\n")
	writeCSVFixture(t, dir, "historical_best_times.csv", "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,recorded_at\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\n")
	writeCSVFixture(t, dir, "payment_history.csv", "legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at\n")

	report, err := NewRunner(Config{OrgID: testutil.DefaultOrgID}).DryRun(context.Background(), dir)
	if err != nil {
		t.Fatalf("DryRun returned error: %v", err)
	}

	var householdStep StepReport
	for _, step := range report.Steps {
		if step.Name == StepHouseholds {
			householdStep = step
			break
		}
	}
	if householdStep.Processed != 1 {
		t.Fatalf("expected blank rows to be ignored, processed=%d", householdStep.Processed)
	}
	if len(householdStep.Errors) != 0 {
		t.Fatalf("expected no household errors for blank spacer rows, got %v", householdStep.Errors)
	}
}

func TestRunnerDryRunComparesSourceCountsAndBillingTotals(t *testing.T) {
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_email,status\nhh-1,Smith Family,smith@example.com,active\n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\n")
	writeCSVFixture(t, dir, "groups.csv", "legacy_group_code,name,type\n")
	writeCSVFixture(t, dir, "historical_best_times.csv", "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,recorded_at\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\ninv-1,INV-100,hh-1,12000,12000,2026-03-01\n")
	writeCSVFixture(t, dir, "payment_history.csv", "legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at\n")

	billingTotal := 12000
	report, err := NewRunner(Config{
		OrgID:                   testutil.DefaultOrgID,
		SourceCounts:            map[string]int{"households.csv": 1, "open_invoices": 1},
		SourceBillingTotalCents: &billingTotal,
	}).DryRun(context.Background(), dir)
	if err != nil {
		t.Fatalf("DryRun returned error: %v", err)
	}

	assertComparison(t, report, "row_count_households", 1, 1, "match")
	assertComparison(t, report, "row_count_open_invoices", 1, 1, "match")
	assertComparison(t, report, "billing_outstanding_total_cents", 12000, 12000, "match")
}

func TestRunnerDryRunFailsOnSourceComparisonMismatchAndWritesRemediationLog(t *testing.T) {
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_email,status\nhh-1,Smith Family,smith@example.com,active\n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\n")
	writeCSVFixture(t, dir, "groups.csv", "legacy_group_code,name,type\n")
	writeCSVFixture(t, dir, "historical_best_times.csv", "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,recorded_at\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\ninv-1,INV-100,hh-1,12000,12000,2026-03-01\n")
	writeCSVFixture(t, dir, "payment_history.csv", "legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at\n")

	billingTotal := 13000
	runner := NewRunner(Config{
		OrgID:                   testutil.DefaultOrgID,
		SourceCounts:            map[string]int{"households": 2},
		SourceBillingTotalCents: &billingTotal,
	})
	report, err := runner.DryRun(context.Background(), dir)
	if err == nil {
		t.Fatal("expected DryRun to fail on comparison mismatches")
	}

	assertComparison(t, report, "row_count_households", 2, 1, "mismatch")
	assertComparison(t, report, "billing_outstanding_total_cents", 13000, 12000, "mismatch")

	logPath := filepath.Join(dir, "remediation.md")
	if err := runner.WriteRemediationLog(logPath, report); err != nil {
		t.Fatalf("WriteRemediationLog: %v", err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(remediation): %v", err)
	}
	if got := string(content); !strings.Contains(got, "row_count_households") || !strings.Contains(got, "billing_outstanding_total_cents") {
		t.Fatalf("remediation log missing comparison items:\n%s", got)
	}
}

func TestRunnerDryRunRejectsInvalidDates(t *testing.T) {
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_email,status\nhh-1,Smith Family,smith@example.com,active\n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\nsw-1,hh-1,Ava,Smith,2012-05-01,F,active\n")
	writeCSVFixture(t, dir, "groups.csv", "legacy_group_code,name,type\n")
	writeCSVFixture(t, dir, "historical_best_times.csv", "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,recorded_at\nbt-1,sw-1,100,free,SCY,65000,not-a-date\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\ninv-1,INV-100,hh-1,12000,2000,bad-date\n")
	writeCSVFixture(t, dir, "payment_history.csv", "legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at\npay-1,inv-1,10000,check,also-bad\n")

	report, err := NewRunner(Config{OrgID: testutil.DefaultOrgID}).DryRun(context.Background(), dir)
	if err == nil {
		t.Fatal("expected DryRun to return an error for invalid date fields")
	}

	assertStepHasError(t, report, StepHistoricalBestTimes, `best_time bt-1: invalid recorded_at "not-a-date"`)
	assertStepHasError(t, report, StepOpenInvoices, `invoice inv-1: invalid due_date "bad-date"`)
	assertStepHasError(t, report, StepPaymentHistory, `payment pay-1: unknown legacy_invoice_id "inv-1"`)
}

func assertStepHasError(t *testing.T, report Report, stepName, want string) {
	t.Helper()

	for _, step := range report.Steps {
		if step.Name != stepName {
			continue
		}
		for _, got := range step.Errors {
			if got == want {
				return
			}
		}
		t.Fatalf("step %s errors %v did not include %q", stepName, step.Errors, want)
	}

	t.Fatalf("step %s not found in report", stepName)
}

func assertComparison(t *testing.T, report Report, name string, source, target int, status string) {
	t.Helper()
	for _, comparison := range report.Comparisons {
		if comparison.Name != name {
			continue
		}
		if comparison.SourceValue != source || comparison.TargetValue != target || comparison.Status != status {
			t.Fatalf("comparison %s = %+v, want source=%d target=%d status=%s", name, comparison, source, target, status)
		}
		return
	}
	t.Fatalf("comparison %s not found in %+v", name, report.Comparisons)
}

func writeCSVFixture(t *testing.T, dir, name, contents string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func TestOpenInvoicesAllowsOmittedOptionalTotal(t *testing.T) {
	runner := NewRunner(Config{OrgID: testutil.DefaultOrgID})
	dir := t.TempDir()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name\nhh-1,Smith Family\n")
	if step := runner.importHouseholds(context.Background(), filepath.Join(dir, "households.csv")); len(step.Errors) != 0 {
		t.Fatal(step.Errors)
	}
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,outstanding_amount_cents,due_date\ninv-1,INV-100,hh-1,2000,2026-03-01\n")
	step := runner.importOpenInvoices(context.Background(), filepath.Join(dir, "open_invoices.csv"))
	if len(step.Errors) != 0 || step.Imported != 1 {
		t.Fatalf("unexpected import: %+v", step)
	}
	invoices := runner.BillingService().SnapshotInvoices(testutil.DefaultOrgID)
	if len(invoices) != 1 || invoices[0].AmountDueCents != 2000 {
		t.Fatalf("unexpected invoices: %+v", invoices)
	}
}
