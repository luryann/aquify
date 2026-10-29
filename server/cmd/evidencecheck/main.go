package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type migrationReport struct {
	Errors      []string `json:"errors"`
	Comparisons []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"comparisons"`
}

type reconciliationEnvelope struct {
	Reconciliation struct {
		StripeReportedCents *int  `json:"stripe_reported_cents"`
		AlertTriggered      *bool `json:"alert_triggered"`
	} `json:"reconciliation"`
}

type providerMetadata struct {
	ExpectStripeWebhookEventID string `json:"expect_stripe_webhook_event_id"`
}

type webhookEventsEnvelope struct {
	Count  int `json:"count"`
	Events []struct {
		EventID string `json:"event_id"`
	} `json:"events"`
}

type deliveriesEnvelope struct {
	Count      int `json:"count"`
	Deliveries []struct {
		Status            string `json:"status"`
		ProviderMessageID string `json:"provider_message_id"`
	} `json:"deliveries"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("evidencecheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "Launch evidence bundle directory to verify")
	requireProvider := fs.Bool("require-provider", true, "Require provider validation evidence")
	requireParallel := fs.Bool("require-parallel-meets", true, "Require two parallel meet-cycle evidence")
	requireStability := fs.Bool("require-stability", true, "Require post-launch stability report evidence")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*dir) == "" {
		return errors.New("-dir is required")
	}

	checker := bundleChecker{root: *dir}
	checker.requireFile("metadata.json")
	checker.requireFile("migration-dry-run.log")
	checker.requireFile("migration-report.json")
	checker.requireFile("remediation-log.md")
	checker.requireFile("manifest.txt")
	checker.rejectFile("provider-validation-required.md", "provider validation was skipped")
	checker.checkMigrationReport("migration-report.json")
	checker.checkSmokeLog("cutover-smoke.log")
	checker.requireFile("cutover-billing-check.log")

	if *requireProvider {
		checker.checkProvider("provider-validation")
	}
	if *requireParallel {
		checker.requireFile(filepath.Join("parallel-meet-cycles", "cycle-validation-summary.md"))
		checker.requireText(filepath.Join("parallel-meet-cycles", "cycle-validation-summary.md"), "Result: PASS")
	}
	if *requireStability {
		checker.requireFile(filepath.Join("stability-report", "stability-report.md"))
		checker.requireText(filepath.Join("stability-report", "stability-report.md"), "Result: PASS")
	}

	if len(checker.failures) > 0 {
		return fmt.Errorf("evidence bundle failed validation:\n- %s", strings.Join(checker.failures, "\n- "))
	}
	fmt.Printf("evidence bundle validated: %s\n", *dir)
	return nil
}

type bundleChecker struct {
	root     string
	failures []string
}

func (c *bundleChecker) full(rel string) string {
	return filepath.Join(c.root, rel)
}

func (c *bundleChecker) requireFile(rel string) {
	info, err := os.Stat(c.full(rel))
	if err != nil {
		c.failures = append(c.failures, fmt.Sprintf("missing %s", rel))
		return
	}
	if info.IsDir() {
		c.failures = append(c.failures, fmt.Sprintf("%s is a directory, expected file", rel))
	}
}

func (c *bundleChecker) rejectFile(rel, reason string) {
	if _, err := os.Stat(c.full(rel)); err == nil {
		c.failures = append(c.failures, fmt.Sprintf("%s is present: %s", rel, reason))
	}
}

// readFile returns the file's contents, recording a failure when it cannot be read.
func (c *bundleChecker) readFile(rel string) ([]byte, bool) {
	payload, err := os.ReadFile(c.full(rel))
	if err != nil {
		c.failures = append(c.failures, fmt.Sprintf("read %s: %v", rel, err))
		return nil, false
	}
	return payload, true
}

// readJSON decodes a JSON file into v, recording a failure that names what the
// file should contain (for example "reconciliation JSON") when it is unreadable or invalid.
func (c *bundleChecker) readJSON(rel, what string, v any) bool {
	payload, ok := c.readFile(rel)
	if !ok {
		return false
	}
	if err := json.Unmarshal(payload, v); err != nil {
		c.failures = append(c.failures, fmt.Sprintf("%s is invalid %s: %v", rel, what, err))
		return false
	}
	return true
}

func (c *bundleChecker) requireText(rel, needle string) {
	payload, ok := c.readFile(rel)
	if !ok {
		return
	}
	if !strings.Contains(string(payload), needle) {
		c.failures = append(c.failures, fmt.Sprintf("%s does not contain %q", rel, needle))
	}
}

func (c *bundleChecker) checkSmokeLog(rel string) {
	payload, ok := c.readFile(rel)
	if !ok {
		return
	}
	text := strings.ToLower(string(payload))
	for _, bad := range []string{"fail ", "failed", "error"} {
		if strings.Contains(text, bad) {
			c.failures = append(c.failures, fmt.Sprintf("%s contains %q", rel, bad))
			return
		}
	}
}

// requiredComparisons are the source-vs-migrated comparisons every migration report must include.
var requiredComparisons = []string{
	"billing_outstanding_total_cents",
	"row_count_households",
	"row_count_swimmers",
	"row_count_groups",
	"row_count_historical_best_times",
	"row_count_open_invoices",
	"row_count_payment_history",
}

func (c *bundleChecker) checkMigrationReport(rel string) {
	var report migrationReport
	if !c.readJSON(rel, "JSON", &report) {
		return
	}
	if len(report.Errors) > 0 {
		c.failures = append(c.failures, fmt.Sprintf("%s contains migration errors", rel))
	}
	if len(report.Comparisons) == 0 {
		c.failures = append(c.failures, fmt.Sprintf("%s has no source comparisons", rel))
		return
	}
	seen := map[string]bool{}
	for _, comparison := range report.Comparisons {
		if comparison.Status != "match" {
			c.failures = append(c.failures, fmt.Sprintf("%s comparison %s status is %s", rel, comparison.Name, comparison.Status))
		}
		seen[comparison.Name] = true
	}
	for _, name := range requiredComparisons {
		if !seen[name] {
			c.failures = append(c.failures, fmt.Sprintf("%s is missing %s comparison", rel, name))
		}
	}
}

func (c *bundleChecker) checkProvider(dir string) {
	c.requireFile(filepath.Join(dir, "metadata.json"))
	c.requireFile(filepath.Join(dir, "health-ready.json"))
	c.requireFile(filepath.Join(dir, "stripe-reconciliation.json"))
	c.requireFile(filepath.Join(dir, "stripe-webhook-events.json"))
	c.requireFile(filepath.Join(dir, "stripe-refund.json"))
	c.requireFile(filepath.Join(dir, "postmark-send.json"))
	c.requireFile(filepath.Join(dir, "postmark-deliveries.json"))
	c.requireFile(filepath.Join(dir, "validation-summary.md"))
	c.requireText(filepath.Join(dir, "validation-summary.md"), "Expected Stripe webhook event: found")
	c.requireText(filepath.Join(dir, "validation-summary.md"), "Stripe refund validation: executed")
	c.requireText(filepath.Join(dir, "validation-summary.md"), "Postmark delivery validation: send_executed")
	for _, name := range []string{
		"health-ready", "stripe-reconciliation", "stripe-webhook-events", "stripe-refund", "postmark-send", "postmark-deliveries",
	} {
		c.checkHTTPStatus(filepath.Join(dir, name+".json.status"))
	}
	c.checkReconciliation(filepath.Join(dir, "stripe-reconciliation.json"))
	c.checkStripeWebhookEvent(filepath.Join(dir, "metadata.json"), filepath.Join(dir, "stripe-webhook-events.json"))
	c.checkPostmarkDeliveries(filepath.Join(dir, "postmark-deliveries.json"))
}

func (c *bundleChecker) checkHTTPStatus(rel string) {
	payload, ok := c.readFile(rel)
	if !ok {
		return
	}
	status := strings.TrimSpace(string(payload))
	if !strings.HasPrefix(status, "2") {
		c.failures = append(c.failures, fmt.Sprintf("%s is HTTP %s", rel, status))
	}
}

func (c *bundleChecker) checkReconciliation(rel string) {
	var envelope reconciliationEnvelope
	if !c.readJSON(rel, "reconciliation JSON", &envelope) {
		return
	}
	if envelope.Reconciliation.StripeReportedCents == nil {
		c.failures = append(c.failures, fmt.Sprintf("%s is missing reconciliation.stripe_reported_cents", rel))
	}
	if envelope.Reconciliation.AlertTriggered == nil {
		c.failures = append(c.failures, fmt.Sprintf("%s is missing reconciliation.alert_triggered", rel))
		return
	}
	if *envelope.Reconciliation.AlertTriggered {
		c.failures = append(c.failures, fmt.Sprintf("%s has alert_triggered=true", rel))
	}
}

func (c *bundleChecker) checkStripeWebhookEvent(metadataRel, eventsRel string) {
	var metadata providerMetadata
	if !c.readJSON(metadataRel, "provider metadata JSON", &metadata) {
		return
	}
	expected := strings.TrimSpace(metadata.ExpectStripeWebhookEventID)
	if expected == "" {
		c.failures = append(c.failures, fmt.Sprintf("%s is missing expect_stripe_webhook_event_id", metadataRel))
		return
	}

	var envelope webhookEventsEnvelope
	if !c.readJSON(eventsRel, "webhook events JSON", &envelope) {
		return
	}
	for _, event := range envelope.Events {
		if event.EventID == expected {
			return
		}
	}
	c.failures = append(c.failures, fmt.Sprintf("%s does not contain expected Stripe event_id %q", eventsRel, expected))
}

func (c *bundleChecker) checkPostmarkDeliveries(rel string) {
	var envelope deliveriesEnvelope
	if !c.readJSON(rel, "Postmark deliveries JSON", &envelope) {
		return
	}
	for _, delivery := range envelope.Deliveries {
		switch delivery.Status {
		case "sent", "delivered":
			if strings.TrimSpace(delivery.ProviderMessageID) != "" {
				return
			}
		}
	}
	c.failures = append(c.failures, fmt.Sprintf("%s has no sent/delivered row with provider_message_id", rel))
}
