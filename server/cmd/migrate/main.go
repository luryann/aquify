package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/dareaquatics/aquify/server/internal/migration"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "plan":
		runPlan()
	case "dry-run":
		runDryRun(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func runPlan() {
	runner := migration.NewRunner(migration.Config{})
	fmt.Println(runner.Plan())
}

func runDryRun(args []string) {
	fs := flag.NewFlagSet("dry-run", flag.ExitOnError)
	inputDir := fs.String("input-dir", "", "Directory containing migration CSV files")
	reportPath := fs.String("report", "migration-report.json", "Path to write the JSON validation report")
	sourceCountsPath := fs.String("source-counts", "", "Optional JSON file mapping source export names to expected row counts")
	sourceBillingTotalRaw := fs.String("source-billing-total-cents", "", "Optional source-system outstanding billing total in cents")
	remediationLogPath := fs.String("remediation-log", "", "Optional path to write a markdown remediation log")
	orgID := fs.String("org-id", "org-migration", "Org ID to associate with imported records")
	actorID := fs.String("actor-id", "migration-cli", "Actor ID to stamp on imported records")
	fs.Parse(args)

	if *inputDir == "" {
		fmt.Fprintln(os.Stderr, "dry-run requires -input-dir")
		os.Exit(1)
	}

	sourceCounts, err := loadSourceCounts(*sourceCountsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load source counts: %v\n", err)
		os.Exit(1)
	}
	sourceBillingTotal, err := parseOptionalCents(*sourceBillingTotalRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse source billing total: %v\n", err)
		os.Exit(1)
	}

	runner := migration.NewRunner(migration.Config{
		OrgID:                   *orgID,
		ActorID:                 *actorID,
		SourceCounts:            sourceCounts,
		SourceBillingTotalCents: sourceBillingTotal,
	})

	report, err := runner.DryRun(context.Background(), *inputDir)
	if err != nil {
		if writeErr := runner.WriteReport(*reportPath, report); writeErr != nil {
			fmt.Fprintf(os.Stderr, "dry-run error: %v\nreport write error: %v\n", err, writeErr)
			os.Exit(1)
		}
		if *remediationLogPath != "" {
			if writeErr := runner.WriteRemediationLog(*remediationLogPath, report); writeErr != nil {
				fmt.Fprintf(os.Stderr, "dry-run error: %v\nremediation log write error: %v\n", err, writeErr)
				os.Exit(1)
			}
		}
		fmt.Fprintf(os.Stderr, "dry-run completed with errors; report written to %s\n", *reportPath)
		os.Exit(1)
	}

	if err := runner.WriteReport(*reportPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	if *remediationLogPath != "" {
		if err := runner.WriteRemediationLog(*remediationLogPath, report); err != nil {
			fmt.Fprintf(os.Stderr, "write remediation log: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("dry-run completed successfully; report written to %s\n", *reportPath)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s <plan|dry-run> [flags]\n", os.Args[0])
}

func loadSourceCounts(path string) (map[string]int, error) {
	if path == "" {
		return nil, nil
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var counts map[string]int
	if err := json.Unmarshal(payload, &counts); err != nil {
		return nil, err
	}
	return counts, nil
}

func parseOptionalCents(raw string) (*int, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, err
	}
	if value < 0 {
		return nil, fmt.Errorf("value must be non-negative")
	}
	return &value, nil
}
