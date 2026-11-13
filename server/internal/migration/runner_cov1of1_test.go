package migration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xCountdownCtx reports context.Canceled after a fixed number of Err calls.
type cov1xCountdownCtx struct {
	context.Context
	remaining int
}

func (c *cov1xCountdownCtx) Err() error {
	if c.remaining <= 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func cov1xWriteAll(t *testing.T, dir string, over map[string]string) {
	t.Helper()
	files := map[string]string{
		"households.csv":            "legacy_household_id,household_name\n",
		"swimmers.csv":              "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth\n",
		"groups.csv":                "legacy_group_code,name\n",
		"historical_best_times.csv": "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms\n",
		"open_invoices.csv":         "legacy_invoice_id,invoice_number,legacy_household_id,due_date,outstanding_amount_cents\n",
		"payment_history.csv":       "legacy_payment_id,legacy_invoice_id,amount_cents,method\n",
	}
	for k, v := range over {
		files[k] = v
	}
	for k, v := range files {
		writeCSVFixture(t, dir, k, v)
	}
}

func cov1xRunner(t *testing.T) (*Runner, string) {
	t.Helper()
	return NewRunner(Config{OrgID: testutil.DefaultOrgID}), t.TempDir()
}

func cov1xHasErr(t *testing.T, step StepReport, want string) {
	t.Helper()
	for _, e := range step.Errors {
		if strings.Contains(e, want) {
			return
		}
	}
	t.Fatalf("errors %v do not contain %q", step.Errors, want)
}

func TestCov1xNewRunnerDefaults(t *testing.T) {
	r := NewRunner(Config{})
	if r.cfg.OrgID != "org-migration" || r.cfg.ActorID != "migration-cli" {
		t.Fatalf("unexpected defaults: %+v", r.cfg)
	}
	if r.cfg.SourceCounts != nil {
		t.Fatalf("expected nil source counts, got %v", r.cfg.SourceCounts)
	}
	if r.MembersService() == nil || r.GroupsService() == nil || r.PerformanceService() == nil || r.BillingService() == nil {
		t.Fatal("services must be initialised")
	}
}

func TestCov1xPlanListsInputFiles(t *testing.T) {
	plan := NewRunner(Config{}).Plan()
	for _, f := range RequiredFiles {
		if !strings.Contains(plan, "- "+f) {
			t.Errorf("plan missing %s", f)
		}
	}
}

func TestCov1xDryRunMissingFiles(t *testing.T) {
	r, dir := cov1xRunner(t)
	report, err := r.DryRun(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing files error, got %v", err)
	}
	if len(report.Errors) != len(RequiredFiles) {
		t.Fatalf("expected %d errors, got %v", len(RequiredFiles), report.Errors)
	}
}

func TestCov1xDryRunCanceledContext(t *testing.T) {
	r, dir := cov1xRunner(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.DryRun(ctx, dir)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCov1xDryRunCanceledBetweenSteps(t *testing.T) {
	r, dir := cov1xRunner(t)
	cov1xWriteAll(t, dir, nil)
	// Err calls: 1 at start, 1 before step one, 0 rows, then cancelled before step two.
	ctx := &cov1xCountdownCtx{Context: context.Background(), remaining: 2}
	report, err := r.DryRun(ctx, dir)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if len(report.Steps) != 1 || len(report.Errors) != 1 || report.Errors[0] != context.Canceled.Error() {
		t.Fatalf("unexpected report: steps=%d errors=%v", len(report.Steps), report.Errors)
	}
}

func TestCov1xDryRunCollectsWarnings(t *testing.T) {
	r, dir := cov1xRunner(t)
	cov1xWriteAll(t, dir, map[string]string{
		"households.csv":    "legacy_household_id,household_name\nhh-1,Smith\n",
		"open_invoices.csv": "legacy_invoice_id,invoice_number,legacy_household_id,due_date,outstanding_amount_cents,total_amount_cents\ninv-1,I-1,hh-1,2026-03-01,500,1000\n",
	})
	report, err := r.DryRun(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 1 || !strings.HasPrefix(report.Warnings[0], "open_invoices: invoice inv-1 imported with total=1000 and outstanding=500") {
		t.Fatalf("unexpected warnings %v", report.Warnings)
	}
}

func TestCov1xWriteReport(t *testing.T) {
	r, dir := cov1xRunner(t)
	report := Report{OrgID: "o1", Errors: []string{"boom"}}
	path := filepath.Join(dir, "report.json")
	if err := r.WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"boom"`) || !strings.Contains(string(b), "o1") {
		t.Fatalf("report content unexpected: %s", b)
	}
	if err := r.WriteReport(filepath.Join(dir, "missing", "r.json"), report); err == nil {
		t.Fatal("expected write error for missing directory")
	}
}

func TestCov1xWriteRemediationLog(t *testing.T) {
	r, dir := cov1xRunner(t)

	empty := filepath.Join(dir, "empty.md")
	if err := r.WriteRemediationLog(empty, Report{}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(empty)
	if !strings.Contains(string(b), "No manual remediation items") {
		t.Fatalf("expected empty log message: %s", b)
	}

	full := filepath.Join(dir, "full.md")
	report := Report{
		Steps: []StepReport{{Name: "groups", Warnings: []string{"w1"}, Errors: []string{"e1"}}},
		Comparisons: []ComparisonReport{
			{Name: "ok", SourceValue: 1, TargetValue: 1, Status: "match"},
			{Name: "bad", SourceValue: 2, TargetValue: 1, Delta: -1, Status: "mismatch"},
		},
	}
	if err := r.WriteRemediationLog(full, report); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(full)
	got := string(b)
	for _, want := range []string{"`groups` warning: w1", "`groups` error: e1", "`bad` source parity mismatch: source=2 target=1 delta=-1", "| ok | 1 | 1 | 0 | match |"} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`ok` source parity mismatch") || strings.Contains(got, "No manual remediation items") {
		t.Fatalf("unexpected content:\n%s", got)
	}

	if err := r.WriteRemediationLog(filepath.Join(dir, "nope", "x.md"), report); err == nil {
		t.Fatal("expected write error")
	}
}

func TestCov1xLoadRowsErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadRows(filepath.Join(dir, "none.csv"), nil, func(m map[string]string) int { return 0 }); err == nil {
		t.Fatal("expected open error")
	}
	writeCSVFixture(t, dir, "empty.csv", "")
	if _, err := loadRows(filepath.Join(dir, "empty.csv"), nil, func(m map[string]string) int { return 0 }); err == nil {
		t.Fatal("expected EOF error for empty file")
	}
	writeCSVFixture(t, dir, "bad.csv", "a,b\n\"unterminated,x\n")
	if _, err := loadRows(filepath.Join(dir, "bad.csv"), nil, func(m map[string]string) int { return 0 }); err == nil {
		t.Fatal("expected csv parse error")
	}
	writeCSVFixture(t, dir, "short.csv", "A Col,b-col\n1\n")
	rows, err := loadRows(filepath.Join(dir, "short.csv"), []string{"a col"}, func(m map[string]string) map[string]string { return m })
	if err != nil || len(rows) != 1 || rows[0]["a_col"] != "1" {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if _, ok := rows[0]["b_col"]; ok {
		t.Fatal("short record must not produce a value for missing column")
	}
	if got := missingHeaders([]string{"a"}, nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestCov1xStepsFailOnUnreadableFile(t *testing.T) {
	r, dir := cov1xRunner(t)
	ctx := context.Background()
	missing := func(n string) string { return filepath.Join(dir, n) }
	for name, step := range map[string]StepReport{
		"households.csv":            r.importHouseholds(ctx, missing("households.csv")),
		"swimmers.csv":              r.importSwimmers(ctx, missing("swimmers.csv")),
		"groups.csv":                r.importGroups(ctx, missing("groups.csv")),
		"historical_best_times.csv": r.importHistoricalBestTimes(ctx, missing("historical_best_times.csv")),
		"open_invoices.csv":         r.importOpenInvoices(ctx, missing("open_invoices.csv")),
		"payment_history.csv":       r.importPaymentHistory(ctx, missing("payment_history.csv")),
	} {
		if len(step.Errors) != 1 || step.Imported != 0 || step.SourceFile != name || step.CompletedAt == "" {
			t.Errorf("%s: unexpected step %+v", name, step)
		}
	}
}

func TestCov1xStepsStopOnCanceledContext(t *testing.T) {
	r, dir := cov1xRunner(t)
	cov1xWriteAll(t, dir, map[string]string{
		"households.csv":            "legacy_household_id,household_name\nhh-1,A\n",
		"swimmers.csv":              "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth\ns,hh-1,A,B,2012-01-01\n",
		"groups.csv":                "legacy_group_code,name\ng,G\n",
		"historical_best_times.csv": "legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms\nb,s,100,free,SCY,1\n",
		"open_invoices.csv":         "legacy_invoice_id,invoice_number,legacy_household_id,due_date,outstanding_amount_cents\ni,N,hh-1,2026-01-01,1\n",
		"payment_history.csv":       "legacy_payment_id,legacy_invoice_id,amount_cents,method\np,i,1,cash\n",
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := func(n string) string { return filepath.Join(dir, n) }
	for name, step := range map[string]StepReport{
		"h": r.importHouseholds(ctx, p("households.csv")),
		"s": r.importSwimmers(ctx, p("swimmers.csv")),
		"g": r.importGroups(ctx, p("groups.csv")),
		"b": r.importHistoricalBestTimes(ctx, p("historical_best_times.csv")),
		"i": r.importOpenInvoices(ctx, p("open_invoices.csv")),
		"p": r.importPaymentHistory(ctx, p("payment_history.csv")),
	} {
		if step.Imported != 0 || step.Processed != 0 || len(step.Errors) != 1 || step.Errors[0] != context.Canceled.Error() {
			t.Errorf("%s: unexpected step %+v", name, step)
		}
	}
}

func TestCov1xHouseholdRowErrors(t *testing.T) {
	r, dir := cov1xRunner(t)
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name,primary_phone\n,NoKey,\nhh-2,,\nhh-3,Phone Fam,+15555550100\n")
	step := r.importHouseholds(context.Background(), filepath.Join(dir, "households.csv"))
	cov1xHasErr(t, step, "row missing legacy_household_id")
	cov1xHasErr(t, step, "household hh-2:")
	if step.Processed != 3 || step.Imported != 1 || len(step.Errors) != 2 {
		t.Fatalf("unexpected step %+v", step)
	}
	hs := r.MembersService().SnapshotHouseholds(testutil.DefaultOrgID)
	if len(hs) != 1 || hs[0].PrimaryPhoneE164 != "+15555550100" {
		t.Fatalf("unexpected households %+v", hs)
	}
}

func TestCov1xSwimmerRowErrorsAndDuplicates(t *testing.T) {
	r, dir := cov1xRunner(t)
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name\nhh-1,Fam\n")
	if s := r.importHouseholds(context.Background(), filepath.Join(dir, "households.csv")); len(s.Errors) != 0 {
		t.Fatal(s.Errors)
	}
	writeCSVFixture(t, dir, "swimmers.csv", strings.Join([]string{
		"legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth,gender",
		",hh-1,No,Key,2012-01-01,m",
		"sw-1,hh-1,Ann,Lee,2012-01-01,female",
		"sw-1,hh-1,Ann,Lee,2012-01-01,female",
		"sw-2,hh-1,,Lee,2012-01-01,x",
		"sw-3,hh-1,Bo,Lee,2012-01-01,weird",
		"sw-4,hh-1,Cy,Lee,2012-01-01,boy",
		"",
	}, "\n"))
	step := r.importSwimmers(context.Background(), filepath.Join(dir, "swimmers.csv"))
	cov1xHasErr(t, step, "row missing legacy_swimmer_id")
	cov1xHasErr(t, step, "swimmer sw-2:")
	if step.Imported != 3 || step.Duplicates != 1 || step.Skipped != 1 || len(step.Errors) != 2 {
		t.Fatalf("unexpected step %+v", step)
	}
	genders := map[string]string{}
	for _, s := range r.MembersService().SnapshotSwimmers(testutil.DefaultOrgID) {
		genders[s.FirstName] = s.Gender
	}
	if genders["Ann"] != "female" || genders["Bo"] != "undisclosed" || genders["Cy"] != "male" {
		t.Fatalf("unexpected genders %v", genders)
	}
}

func TestCov1xGroupRowErrorsAndDuplicates(t *testing.T) {
	r, dir := cov1xRunner(t)
	writeCSVFixture(t, dir, "groups.csv", strings.Join([]string{
		"legacy_group_code,name,type,min_age,max_age",
		",NoKey,,,",
		"SR,Senior,competitive,13,18",
		"SR,Senior,competitive,13,18",
		"BADMIN,Bad,competitive,abc,18",
		"BADMAX,Bad,competitive,10,xyz",
		"NONAME,,competitive,,",
		"",
	}, "\n"))
	step := r.importGroups(context.Background(), filepath.Join(dir, "groups.csv"))
	cov1xHasErr(t, step, "row missing legacy_group_code")
	cov1xHasErr(t, step, "group BADMIN: invalid min_age:")
	cov1xHasErr(t, step, "group BADMAX: invalid max_age:")
	cov1xHasErr(t, step, "group NONAME:")
	if step.Imported != 1 || step.Duplicates != 1 || step.Skipped != 1 || len(step.Errors) != 4 {
		t.Fatalf("unexpected step %+v", step)
	}
	gs := r.GroupsService().SnapshotGroups(testutil.DefaultOrgID)
	if len(gs) != 1 || gs[0].MinAge == nil || *gs[0].MinAge != 13 || *gs[0].MaxAge != 18 {
		t.Fatalf("unexpected groups %+v", gs)
	}
}

func TestCov1xBestTimeRowErrorsAndDuplicates(t *testing.T) {
	r, dir := cov1xRunner(t)
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name\nhh-1,Fam\n")
	writeCSVFixture(t, dir, "swimmers.csv", "legacy_swimmer_id,legacy_household_id,first_name,last_name,date_of_birth\nsw-1,hh-1,Ann,Lee,2012-01-01\n")
	ctx := context.Background()
	r.importHouseholds(ctx, filepath.Join(dir, "households.csv"))
	if s := r.importSwimmers(ctx, filepath.Join(dir, "swimmers.csv")); s.Imported != 1 {
		t.Fatalf("setup failed: %+v", s)
	}
	writeCSVFixture(t, dir, "historical_best_times.csv",
		"legacy_best_time_key,legacy_swimmer_id,distance,stroke,course,best_time_ms,best_time_millis,recorded_at\n"+
			",sw-1,50,free,scy,,30000,\n"+
			"k1,sw-1,50,Freestyle,scy,,30000,2025/01/02\n"+
			"k1,sw-1,50,Freestyle,scy,,30000,2025/01/02\n"+
			"k2,nope,50,free,scy,30000,,\n"+
			"k3,sw-1,zero,free,scy,30000,,\n"+
			"k4,sw-1,50,free,scy,-5,,\n"+
			"k5,sw-1,50,free,scy,fast,,\n"+
			"k6,sw-1,50,free,scy,30000,,junk\n"+
			"k7,sw-1,50,notastroke,scy,30000,,\n"+
			"k8,sw-1,100,backstroke,lcm,70000,,\n"+
			"k9,sw-1,100,breast,lcm,80000,,\n"+
			"k10,sw-1,100,butterfly,lcm,75000,,\n"+
			"k11,sw-1,200,individual medley,lcm,150000,,\n")
	step := r.importHistoricalBestTimes(ctx, filepath.Join(dir, "historical_best_times.csv"))
	cov1xHasErr(t, step, "row missing legacy_best_time_key")
	cov1xHasErr(t, step, `best_time k2: unknown legacy_swimmer_id "nope"`)
	cov1xHasErr(t, step, `best_time k3: invalid distance "zero"`)
	cov1xHasErr(t, step, `best_time k4: invalid best_time_ms "-5"`)
	cov1xHasErr(t, step, `best_time k5: invalid best_time_ms "fast"`)
	cov1xHasErr(t, step, `best_time k6: invalid recorded_at "junk"`)
	if len(step.Errors) != 6 {
		t.Fatalf("expected 6 errors, got %v", step.Errors)
	}
	if step.Duplicates != 1 || step.Skipped != 1 {
		t.Fatalf("unexpected duplicate counts %+v", step)
	}
	if step.Imported != 6 {
		t.Fatalf("expected 6 imported best times, got %+v", step)
	}
}

func TestCov1xInvoiceRowErrorsAndDuplicates(t *testing.T) {
	r, dir := cov1xRunner(t)
	ctx := context.Background()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name\nhh-1,Fam\n")
	r.importHouseholds(ctx, filepath.Join(dir, "households.csv"))
	writeCSVFixture(t, dir, "open_invoices.csv", strings.Join([]string{
		"legacy_invoice_id,invoice_number,legacy_household_id,due_date,outstanding_amount_cents,invoice_total_cents",
		",N0,hh-1,2026-01-01,100,",
		"i1,N1,hh-1,2026-01-01,100,",
		"i1,N1,hh-1,2026-01-01,100,",
		"i2,N2,nope,2026-01-01,100,",
		"i3,N3,hh-1,2026-01-01,100,abc",
		"i4,N4,hh-1,2026-01-01,100,-1",
		"i5,N5,hh-1,2026-01-01,xx,",
		"i6,N6,hh-1,2026-01-01,-3,",
		"i7,N7,hh-1,2026-01-01,500,200",
		"i8,N8,hh-1,,100,",
		"i9,N9,hh-1,2026-01-01,100,",
		"i10,N9,hh-1,2026-01-01,100,",
		"",
	}, "\n"))
	step := r.importOpenInvoices(ctx, filepath.Join(dir, "open_invoices.csv"))
	for _, want := range []string{
		"row missing legacy_invoice_id",
		`invoice i2: unknown legacy_household_id "nope"`,
		`invoice i3: invalid total_amount_cents "abc"`,
		`invoice i4: invalid total_amount_cents "-1"`,
		`invoice i5: invalid outstanding_amount_cents "xx"`,
		`invoice i6: invalid outstanding_amount_cents "-3"`,
		"invoice i7: total_amount_cents must be >= outstanding_amount_cents",
		`invoice i8: invalid due_date ""`,
	} {
		cov1xHasErr(t, step, want)
	}
	if step.Duplicates != 1 || step.Skipped != 1 {
		t.Fatalf("unexpected duplicate counts %+v", step)
	}
	cov1xHasErr(t, step, "invoice i10: invoice number already exists")
	if step.Imported != 2 || len(step.Errors) != 9 {
		t.Fatalf("expected i1 and i9 imported with 9 errors, got %+v", step)
	}
	numbers := map[string]bool{}
	for _, inv := range r.BillingService().SnapshotInvoices(testutil.DefaultOrgID) {
		if inv.Status != "voided" {
			numbers[inv.InvoiceNumber] = true
		}
	}
	if !numbers["N1"] || !numbers["N9"] || len(numbers) != 2 {
		t.Fatalf("unexpected live invoices %v", numbers)
	}
}

func TestCov1xPaymentRowErrorsAndDuplicates(t *testing.T) {
	r, dir := cov1xRunner(t)
	ctx := context.Background()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name\nhh-1,Fam\n")
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,due_date,outstanding_amount_cents\ni1,N1,hh-1,2026-01-01,1000\n")
	r.importHouseholds(ctx, filepath.Join(dir, "households.csv"))
	if s := r.importOpenInvoices(ctx, filepath.Join(dir, "open_invoices.csv")); s.Imported != 1 {
		t.Fatalf("setup failed: %+v", s)
	}
	writeCSVFixture(t, dir, "payment_history.csv", strings.Join([]string{
		"legacy_payment_id,legacy_invoice_id,amount_cents,method,paid_at,external_ref,notes",
		",i1,100,cash,,,",
		"p1,i1,100,card,2026-01-05,ext-1, note ",
		"p1,i1,100,card,2026-01-05,ext-1,",
		"p2,nope,100,cash,,,",
		"p3,i1,zero,cash,,,",
		"p4,i1,0,cash,,,",
		"p5,i1,100,cash,bogus,,",
		"p6,i1,999999,ach,,,",
		"p7,i1,50,wire,,,",
		"p8,i1,50,cheque,,,",
		"",
	}, "\n"))
	step := r.importPaymentHistory(ctx, filepath.Join(dir, "payment_history.csv"))
	for _, want := range []string{
		"row missing legacy_payment_id",
		`payment p2: unknown legacy_invoice_id "nope"`,
		`payment p3: invalid amount_cents "zero"`,
		`payment p4: invalid amount_cents "0"`,
		`payment p5: invalid paid_at "bogus"`,
		"payment p6:",
	} {
		cov1xHasErr(t, step, want)
	}
	if step.Imported != 3 || step.Duplicates != 1 || step.Skipped != 1 {
		t.Fatalf("unexpected step %+v", step)
	}
	methods := map[string]string{}
	for _, p := range r.BillingService().SnapshotPayments(testutil.DefaultOrgID) {
		methods[p.ExternalRef] = string(p.Method)
		if p.ExternalRef == "ext-1" && p.Notes != "note" {
			t.Fatalf("notes not trimmed: %q", p.Notes)
		}
	}
	if methods["ext-1"] != "stripe" || methods["p7"] != "other" || methods["p8"] != "check" {
		t.Fatalf("unexpected methods %v", methods)
	}
}

func TestCov1xNormalizers(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"F": "female", "Woman": "female", "girl": "female", "M": "male", "man": "male", "Male": "male", "x": "non_binary", "non-binary": "non_binary", "nonbinary": "non_binary", "non_binary": "non_binary", "": "undisclosed", "zzz": "undisclosed"} {
		if got := normalizeGender(in); got != want {
			t.Errorf("normalizeGender(%q)=%q want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"free": "free", "Freestyle": "free", "back": "back", "Backstroke": "back", "breast": "breast", "breaststroke": "breast", "fly": "fly", "butterfly": "fly", "im": "im", "individual_medley": "im", "Individual Medley": "im", " Odd ": "odd"} {
		if got := normalizeBestTimeStroke(in); got != want {
			t.Errorf("normalizeBestTimeStroke(%q)=%q want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"stripe": "stripe", "Card": "stripe", "credit_card": "stripe", "payment_intent": "stripe", "check": "check", "cheque": "check", "cash": "cash", "ach": "ach", "bank": "ach", "bank_transfer": "ach", "": "other", "venmo": "other"} {
		if got := normalizePaymentMethod(in); got != want {
			t.Errorf("normalizePaymentMethod(%q)=%q want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"recreational": "recreational", "precompetitive": "developmental", "lessons": "recreational", "learn-to-swim": "recreational", "learn_to_swim": "recreational", "developmental": "developmental", "other": "other"} {
		if got := normalizeGroupType(in); got != want {
			t.Errorf("normalizeGroupType(%q)=%q want %q", in, got, want)
		}
	}
	if got := normalizeCourse(" scy "); got != "SCY" {
		t.Errorf("normalizeCourse=%q", got)
	}
	if got := normalizeStatus(" ACTIVE ", "x"); got != "active" {
		t.Errorf("normalizeStatus=%q", got)
	}
}

func TestCov1xDates(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":                     "",
		"2026-03-04":           "2026-03-04",
		"2026-03-04T10:00:00Z": "2026-03-04",
		"2026/03/04":           "2026-03-04",
		"03/04/2026":           "2026-03-04",
		"3/4/2026":             "2026-03-04",
		"garbage":              "garbage",
	} {
		if got := normalizeDate(in); got != want {
			t.Errorf("normalizeDate(%q)=%q want %q", in, got, want)
		}
	}
	if got, err := normalizeOptionalDate("  "); got != "" || err != nil {
		t.Errorf("blank optional date: %q %v", got, err)
	}
	if _, err := normalizeOptionalDate("garbage"); err == nil {
		t.Error("expected error for garbage date")
	}
	if got, err := normalizeRequiredDate("3/4/2026"); got != "2026-03-04" || err != nil {
		t.Errorf("required date: %q %v", got, err)
	}
	if _, err := normalizeRequiredDate(""); err == nil || err.Error() != "date is required" {
		t.Errorf("expected required error, got %v", err)
	}
	if _, err := normalizeRequiredDate("garbage"); err == nil {
		t.Error("expected parse error")
	}
}

func TestCov1xParseOptionalInt(t *testing.T) {
	t.Parallel()
	if v, err := parseOptionalInt(" "); v != nil || err != nil {
		t.Fatalf("blank: %v %v", v, err)
	}
	if v, err := parseOptionalInt(" 12 "); err != nil || v == nil || *v != 12 {
		t.Fatalf("12: %v %v", v, err)
	}
	if _, err := parseOptionalInt("x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCov1xCopySourceCountsNormalizesKeys(t *testing.T) {
	t.Parallel()
	if copySourceCounts(nil) != nil {
		t.Fatal("nil input must yield nil")
	}
	got := copySourceCounts(map[string]int{" Open-Invoices.CSV ": 3, "best times": 2})
	if got["open_invoices"] != 3 || got["best_times"] != 2 || len(got) != 2 {
		t.Fatalf("unexpected %v", got)
	}
}

func TestCov1xInvoiceItemRejectionDiscardsDraft(t *testing.T) {
	r, dir := cov1xRunner(t)
	ctx := context.Background()
	writeCSVFixture(t, dir, "households.csv", "legacy_household_id,household_name\nhh-1,Fam\n")
	r.importHouseholds(ctx, filepath.Join(dir, "households.csv"))
	// Amount exceeds the maximum a single line item may carry.
	writeCSVFixture(t, dir, "open_invoices.csv", "legacy_invoice_id,invoice_number,legacy_household_id,due_date,outstanding_amount_cents\ni1,N1,hh-1,2026-01-01,3000000000\n")
	step := r.importOpenInvoices(ctx, filepath.Join(dir, "open_invoices.csv"))
	cov1xHasErr(t, step, "invoice i1:")
	if step.Imported != 0 || len(step.Warnings) != 0 {
		t.Fatalf("unexpected step %+v", step)
	}
	for _, inv := range r.BillingService().SnapshotInvoices(testutil.DefaultOrgID) {
		if inv.Status != "voided" {
			t.Fatalf("expected draft to be voided, got %+v", inv)
		}
	}
}
