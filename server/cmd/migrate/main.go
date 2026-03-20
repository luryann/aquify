package main

import (
	"context"
	"flag"
	"fmt"
	"os"

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
	orgID := fs.String("org-id", "org-migration", "Org ID to associate with imported records")
	actorID := fs.String("actor-id", "migration-cli", "Actor ID to stamp on imported records")
	fs.Parse(args)

	if *inputDir == "" {
		fmt.Fprintln(os.Stderr, "dry-run requires -input-dir")
		os.Exit(1)
	}

	runner := migration.NewRunner(migration.Config{
		OrgID:   *orgID,
		ActorID: *actorID,
	})

	report, err := runner.DryRun(context.Background(), *inputDir)
	if err != nil {
		if writeErr := runner.WriteReport(*reportPath, report); writeErr != nil {
			fmt.Fprintf(os.Stderr, "dry-run error: %v\nreport write error: %v\n", err, writeErr)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "dry-run completed with errors; report written to %s\n", *reportPath)
		os.Exit(1)
	}

	if err := runner.WriteReport(*reportPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("dry-run completed successfully; report written to %s\n", *reportPath)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s <plan|dry-run> [flags]\n", os.Args[0])
}
