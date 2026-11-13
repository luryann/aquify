package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cov1xRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
}

func TestCov1xRunFlagAndDirErrors(t *testing.T) {
	if err := run([]string{"-bogus"}); err == nil {
		t.Fatal("expected flag parse error for unknown flag")
	}
	for _, args := range [][]string{{}, {"-dir", "   "}} {
		err := run(args)
		if err == nil || err.Error() != "-dir is required" {
			t.Fatalf("args %v: expected -dir required error, got %v", args, err)
		}
	}
}

func TestCov1xMainSuccessPath(t *testing.T) {
	dir := makeBundle(t)
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"evidencecheck", "-dir", dir}
	// A valid bundle must return from main without exiting the test process.
	main()
}

func TestCov1xRunOptionalSectionsCanBeDisabled(t *testing.T) {
	dir := makeBundle(t)
	cov1xRemove(t, filepath.Join(dir, "provider-validation", "validation-summary.md"))
	cov1xRemove(t, filepath.Join(dir, "parallel-meet-cycles", "cycle-validation-summary.md"))
	cov1xRemove(t, filepath.Join(dir, "stability-report", "stability-report.md"))

	if err := run([]string{"-dir", dir}); err == nil {
		t.Fatal("expected failure when required sections are incomplete")
	}
	err := run([]string{"-dir", dir, "-require-provider=false", "-require-parallel-meets=false", "-require-stability=false"})
	if err != nil {
		t.Fatalf("disabled sections must not be checked: %v", err)
	}
}

func TestCov1xRunReportsEachFailure(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   string
	}{
		{"missing top-level file", func(t *testing.T, d string) { cov1xRemove(t, filepath.Join(d, "manifest.txt")) }, "missing manifest.txt"},
		{"file is directory", func(t *testing.T, d string) {
			cov1xRemove(t, filepath.Join(d, "manifest.txt"))
			mkdir(t, filepath.Join(d, "manifest.txt"))
		}, "manifest.txt is a directory, expected file"},
		{"invalid migration json", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "migration-report.json"), `{not json`)
		}, "migration-report.json is invalid JSON"},
		{"migration errors", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "migration-report.json"), `{"errors":["boom"],"comparisons":[{"name":"x","status":"match"}]}`)
		}, "contains migration errors"},
		{"no comparisons", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "migration-report.json"), `{"errors":[],"comparisons":[]}`)
		}, "has no source comparisons"},
		{"missing required comparison", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "migration-report.json"), `{"comparisons":[{"name":"row_count_households","status":"match"}]}`)
		}, "is missing row_count_swimmers comparison"},
		{"smoke log unreadable", func(t *testing.T, d string) { cov1xRemove(t, filepath.Join(d, "cutover-smoke.log")) }, "read cutover-smoke.log"},
		{"smoke log failure", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "cutover-smoke.log"), "PASS a\nFAIL b: nope")
		}, `reports a failed check: "FAIL b: nope"`},
		{"http status non-2xx", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "stripe-refund.json.status"), "500\n")
		}, "stripe-refund.json.status is HTTP 500"},
		{"http status missing", func(t *testing.T, d string) {
			cov1xRemove(t, filepath.Join(d, "provider-validation", "postmark-send.json.status"))
		}, "read provider-validation/postmark-send.json.status"},
		{"summary lacks text", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "validation-summary.md"), "nothing")
		}, `does not contain "Stripe refund validation: executed"`},
		{"parallel summary not pass", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "parallel-meet-cycles", "cycle-validation-summary.md"), "Result: FAIL")
		}, `does not contain "Result: PASS"`},
		{"stability report not pass", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "stability-report", "stability-report.md"), "Result: FAIL")
		}, "stability-report.md does not contain"},
		{"reconciliation invalid json", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "stripe-reconciliation.json"), `nope`)
		}, "is invalid reconciliation JSON"},
		{"reconciliation missing alert flag", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "stripe-reconciliation.json"), `{"reconciliation":{"stripe_reported_cents":1}}`)
		}, "missing reconciliation.alert_triggered"},
		{"metadata invalid json", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "metadata.json"), `[`)
		}, "is invalid provider metadata JSON"},
		{"metadata without expected event", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "metadata.json"), `{"expect_stripe_webhook_event_id":"  "}`)
		}, "missing expect_stripe_webhook_event_id"},
		{"webhook events invalid json", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "stripe-webhook-events.json"), `{`)
		}, "is invalid webhook events JSON"},
		{"postmark invalid json", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "postmark-deliveries.json"), `{`)
		}, "is invalid Postmark deliveries JSON"},
		{"postmark failed status only", func(t *testing.T, d string) {
			write(t, filepath.Join(d, "provider-validation", "postmark-deliveries.json"), `{"deliveries":[{"status":"bounced","provider_message_id":"m1"}]}`)
		}, "has no sent/delivered row with provider_message_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := makeBundle(t)
			tc.mutate(t, dir)
			err := run([]string{"-dir", dir})
			if err == nil {
				t.Fatalf("expected failure containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), "evidence bundle failed validation") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestCov1xSmokeLogEdgeCases(t *testing.T) {
	cases := []struct {
		name, log, wantFailure string
	}{
		{"unterminated bracket line is ignored", "[no closing\nPASS ok", ""},
		{"lowercase pass with timestamp", "[t] pass: health", ""},
		{"blank lines ignored", "\n\nPASS ok\n\n", ""},
		{"error token", "PASS a\nERROR: db down", "ERROR: db down"},
		{"failed token", "FAILED check", "FAILED check"},
	}
	for _, tc := range cases {
		c := &bundleChecker{root: t.TempDir()}
		write(t, filepath.Join(c.root, "smoke.log"), tc.log)
		c.checkSmokeLog("smoke.log")
		if tc.wantFailure == "" {
			if len(c.failures) != 0 {
				t.Errorf("%s: unexpected failures %v", tc.name, c.failures)
			}
			continue
		}
		if len(c.failures) != 1 || !strings.Contains(c.failures[0], tc.wantFailure) {
			t.Errorf("%s: failures=%v, want one containing %q", tc.name, c.failures, tc.wantFailure)
		}
	}
}

func TestCov1xCheckerHelpers(t *testing.T) {
	c := &bundleChecker{root: t.TempDir()}
	write(t, filepath.Join(c.root, "ok.txt"), "hello world")
	c.requireText("ok.txt", "hello")
	c.requireFile("ok.txt")
	c.rejectFile("absent.txt", "never")
	if len(c.failures) != 0 {
		t.Fatalf("unexpected failures: %v", c.failures)
	}
	c.rejectFile("ok.txt", "should not exist")
	if len(c.failures) != 1 || !strings.Contains(c.failures[0], "ok.txt is present: should not exist") {
		t.Fatalf("rejectFile failures: %v", c.failures)
	}
	if _, ok := c.readFile("absent.txt"); ok {
		t.Fatal("readFile of absent file must report not ok")
	}
	var v struct{}
	if c.readJSON("absent.txt", "thing", &v) {
		t.Fatal("readJSON of absent file must report false")
	}
}
