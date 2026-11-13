package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/migration"
)

type cov1xExit struct{ code int }

// cov1xRun runs fn with os.Args set, exitFunc faked (panics to halt flow),
// and stdout/stderr captured. It returns exit code (-1 if none), stdout, stderr.
func cov1xRun(t *testing.T, args []string, fn func()) (code int, stdout, stderr string) {
	t.Helper()
	oldArgs, oldExit, oldOut, oldErr := os.Args, exitFunc, os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Args, os.Stdout, os.Stderr = args, outW, errW
	exitFunc = func(c int) { panic(cov1xExit{c}) }
	code = -1
	func() {
		defer func() {
			os.Args, exitFunc, os.Stdout, os.Stderr = oldArgs, oldExit, oldOut, oldErr
			if r := recover(); r != nil {
				e, ok := r.(cov1xExit)
				if !ok {
					panic(r)
				}
				code = e.code
			}
		}()
		fn()
	}()
	outW.Close()
	errW.Close()
	ob, _ := io.ReadAll(outR)
	eb, _ := io.ReadAll(errR)
	outR.Close()
	errR.Close()
	return code, string(ob), string(eb)
}

func cov1xFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	hh := "legacy_household_id,household_name,primary_email,status\nhh-1,Smith Family,smith@example.com,active\n"
	files := map[string]string{
		"households.csv":            hh,
		"swimmers.csv":              "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status,usa_swimming_id\nsw-1,hh-1,Ava,Smith,2012-05-01,F,active,0501AVASMIT\n",
		"groups.csv":                "legacy_group_code,name,type\nsenior,Senior,competitive\n",
		"historical_best_times.csv": "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,recorded_at\nbt-1,sw-1,100,free,SCY,65000,2025-02-01\n",
		"open_invoices.csv":         "legacy_invoice_id,invoice_number,legacy_household_id,total_amount_cents,outstanding_amount_cents,due_date\ninv-1,INV-100,hh-1,12000,2000,2026-03-01\n",
		"payment_history.csv":       "legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at\npay-1,inv-1,10000,check,2026-02-15\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCov1xMigrateMainNoArgsPrintsUsage(t *testing.T) {
	code, _, stderr := cov1xRun(t, []string{"migrate"}, main)
	if code != 1 || !strings.Contains(stderr, "usage: migrate <plan|dry-run>") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestCov1xMigrateMainUnknownCommand(t *testing.T) {
	code, _, stderr := cov1xRun(t, []string{"migrate", "bogus"}, main)
	if code != 1 || !strings.Contains(stderr, "usage:") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestCov1xMigrateMainPlan(t *testing.T) {
	code, stdout, _ := cov1xRun(t, []string{"migrate", "plan"}, main)
	want := newCov1xRunner().Plan() + "\n"
	if code != -1 || stdout != want {
		t.Fatalf("code=%d stdout=%q want=%q", code, stdout, want)
	}
}

func TestCov1xMigrateDryRunRequiresInputDir(t *testing.T) {
	code, _, stderr := cov1xRun(t, []string{"migrate", "dry-run"}, main)
	if code != 1 || !strings.Contains(stderr, "dry-run requires -input-dir") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestCov1xMigrateDryRunSuccessWithOptions(t *testing.T) {
	dir := cov1xFixtures(t)
	out := t.TempDir()
	report := filepath.Join(out, "r.json")
	remed := filepath.Join(out, "r.md")
	counts := filepath.Join(out, "counts.json")
	if err := os.WriteFile(counts, []byte(`{"households":1,"swimmers":1,"groups":1,"historical_best_times":1,"open_invoices":1,"payment_history":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := cov1xRun(t, []string{"migrate", "dry-run", "-input-dir", dir, "-report", report,
		"-remediation-log", remed, "-org-id", "org-x", "-actor-id", "actor-x", "-source-counts", counts}, main)
	if code != -1 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "dry-run completed successfully; report written to "+report) {
		t.Fatalf("stdout=%q", stdout)
	}
	var got migration.Report
	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.OrgID != "org-x" || got.ActorID != "actor-x" || got.InputDir != dir {
		t.Fatalf("report identity wrong: %+v", got)
	}
	if len(got.Steps) != 6 || len(got.Comparisons) != 6 {
		t.Fatalf("steps=%d comparisons=%d", len(got.Steps), len(got.Comparisons))
	}
	for _, c := range got.Comparisons {
		if c.Status != "match" {
			t.Fatalf("comparison %+v should match", c)
		}
	}
	if _, err := os.Stat(remed); err != nil {
		t.Fatalf("remediation log not written: %v", err)
	}
}

func TestCov1xMigrateDryRunWithSourceInputsParses(t *testing.T) {
	dir := cov1xFixtures(t)
	out := t.TempDir()
	counts := filepath.Join(out, "counts.json")
	if err := os.WriteFile(counts, []byte(`{"households":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(out, "r.json")
	// Source billing total of 2000 matches the outstanding invoice amount.
	code, _, stderr := cov1xRun(t, []string{"migrate", "dry-run", "-input-dir", dir, "-report", report,
		"-source-counts", counts, "-source-billing-total-cents", "2000"}, main)
	if code != -1 {
		t.Fatalf("unexpected code=%d stderr=%q", code, stderr)
	}
	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("report not written: %v", err)
	}
	var got migration.Report
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	found := map[string]migration.ComparisonReport{}
	for _, c := range got.Comparisons {
		found[c.Name] = c
	}
	bill, ok := found["billing_outstanding_total_cents"]
	if !ok || bill.SourceValue != 2000 || bill.Status != "match" {
		t.Fatalf("billing comparison wrong: %+v (all=%+v)", bill, got.Comparisons)
	}
	if len(got.Comparisons) != 2 {
		t.Fatalf("want households + billing comparisons, got %+v", got.Comparisons)
	}
}

func TestCov1xMigrateDryRunValidationErrorsStillWriteReport(t *testing.T) {
	dir := cov1xFixtures(t)
	// A swimmer referencing an unknown household is a hard validation error.
	orphan := "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender,registration_status\nsw-1,missing,Ava,Smith,2012-05-01,F,active\n"
	if err := os.WriteFile(filepath.Join(dir, "swimmers.csv"), []byte(orphan), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "r.json")
	code, _, stderr := cov1xRun(t, []string{"migrate", "dry-run", "-input-dir", dir, "-report", report}, main)
	if code != 1 || !strings.Contains(stderr, "dry-run completed with errors; report written to "+report) {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(report); err != nil {
		t.Fatalf("report not written: %v", err)
	}
}

func TestCov1xMigrateDryRunRunAndWriteErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	report := filepath.Join(t.TempDir(), "no-dir", "r.json")
	code, _, stderr := cov1xRun(t, []string{"migrate", "dry-run", "-input-dir", missing, "-report", report}, main)
	if code != 1 || !strings.Contains(stderr, "dry-run error: required migration input files are missing") ||
		!strings.Contains(stderr, "write report:") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestCov1xMigrateDryRunWriteErrorOnly(t *testing.T) {
	dir := cov1xFixtures(t)
	report := filepath.Join(t.TempDir(), "no-dir", "r.json")
	code, _, stderr := cov1xRun(t, []string{"migrate", "dry-run", "-input-dir", dir, "-report", report}, main)
	if code != 1 || !strings.Contains(stderr, "write report:") || strings.Contains(stderr, "dry-run error") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestCov1xMigrateDryRunBadSourceCounts(t *testing.T) {
	code, _, stderr := cov1xRun(t, []string{"migrate", "dry-run", "-input-dir", "x",
		"-source-counts", filepath.Join(t.TempDir(), "missing.json")}, main)
	if code != 1 || !strings.Contains(stderr, "load source counts:") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestCov1xMigrateDryRunBadBillingTotal(t *testing.T) {
	code, _, stderr := cov1xRun(t, []string{"migrate", "dry-run", "-input-dir", "x",
		"-source-billing-total-cents", "abc"}, main)
	if code != 1 || !strings.Contains(stderr, "parse source billing total:") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestCov1xWriteDryRunOutputsRemediationLogError(t *testing.T) {
	dir := t.TempDir()
	err := writeDryRunOutputs(newCov1xRunner(), cov1xEmptyReport(), filepath.Join(dir, "r.json"), filepath.Join(dir, "no-dir", "r.md"))
	if err == nil || !strings.Contains(err.Error(), "write remediation log:") {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "r.json")); statErr != nil {
		t.Fatalf("report should still be written: %v", statErr)
	}
}

func TestCov1xLoadSourceCounts(t *testing.T) {
	if c, err := loadSourceCounts(""); c != nil || err != nil {
		t.Fatalf("empty path: %v %v", c, err)
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "g.json")
	bad := filepath.Join(dir, "b.json")
	if err := os.WriteFile(good, []byte(`{"a":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(`{"a":`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := loadSourceCounts(good)
	if err != nil || c["a"] != 3 {
		t.Fatalf("good: %v %v", c, err)
	}
	if _, err := loadSourceCounts(bad); err == nil {
		t.Fatal("expected JSON error")
	}
	if _, err := loadSourceCounts(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("expected read error")
	}
}

func TestCov1xParseOptionalCents(t *testing.T) {
	if v, err := parseOptionalCents(""); v != nil || err != nil {
		t.Fatalf("empty: %v %v", v, err)
	}
	if v, err := parseOptionalCents("250"); err != nil || v == nil || *v != 250 {
		t.Fatalf("250: %v %v", v, err)
	}
	if _, err := parseOptionalCents("x"); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := parseOptionalCents("-1"); err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("negative: %v", err)
	}
}

func newCov1xRunner() *migration.Runner {
	return migration.NewRunner(migration.Config{OrgID: "org-t", ActorID: "a"})
}

func cov1xEmptyReport() migration.Report { return migration.Report{OrgID: "org-t"} }
