package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type threshold struct {
	pkg string
	min float64
}

var criticalThresholds = []threshold{
	{pkg: "./internal/app/handlers", min: 1.0},
	{pkg: "./internal/app/compliance", min: 50.0},
	{pkg: "./internal/app/documents", min: 30.0},
	{pkg: "./internal/app/imports", min: 15.0},
	{pkg: "./internal/app/meets", min: 20.0},
	{pkg: "./internal/app/performance", min: 14.0},
	{pkg: "./internal/app/reports", min: 2.0},
	{pkg: "./internal/app/training", min: 35.0},
	{pkg: "./internal/workers", min: 5.0},
	{pkg: "./internal/workers/jobhandlers", min: 17.0},
}

var coveragePattern = regexp.MustCompile(`coverage:\s+([0-9]+(?:\.[0-9]+)?)%\s+of statements`)

func main() {
	var failures []string
	for _, item := range criticalThresholds {
		coverage, output, err := runPackageCoverage(item.pkg)
		if err != nil {
			fmt.Fprint(os.Stderr, output)
			failures = append(failures, fmt.Sprintf("%s: go test failed: %v", item.pkg, err))
			continue
		}
		fmt.Printf("%s coverage %.1f%% (minimum %.1f%%)\n", item.pkg, coverage, item.min)
		if coverage < item.min {
			failures = append(failures, fmt.Sprintf("%s: coverage %.1f%% is below %.1f%%", item.pkg, coverage, item.min))
		}
	}

	if len(failures) > 0 {
		fmt.Fprintln(os.Stderr, "\ncritical coverage gate failed:")
		for _, failure := range failures {
			fmt.Fprintf(os.Stderr, "- %s\n", failure)
		}
		os.Exit(1)
	}
}

func runPackageCoverage(pkg string) (float64, string, error) {
	cmd := exec.Command("go", "test", pkg, "-cover")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	output := out.String()
	if err != nil {
		if strings.Contains(output, "Application Control policy has blocked this file") {
			return runPackageCoverageBinary(pkg, output)
		}
		return 0, output, err
	}

	matches := coveragePattern.FindStringSubmatch(output)
	if len(matches) != 2 {
		return 0, output, fmt.Errorf("coverage percentage not found in output: %s", strings.TrimSpace(output))
	}
	coverage, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, output, fmt.Errorf("parse coverage %q: %w", matches[1], err)
	}
	return coverage, output, nil
}

func runPackageCoverageBinary(pkg, originalOutput string) (float64, string, error) {
	binDir := filepath.Join(".", ".coverage-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return 0, originalOutput, err
	}

	name := strings.NewReplacer("/", "_", "\\", "_", ".", "_").Replace(strings.TrimPrefix(pkg, "./"))
	exe := filepath.Join(binDir, name+".test.exe")
	profile := filepath.Join(binDir, name+".cover.out")
	defer os.Remove(exe)
	defer os.Remove(profile)

	build := exec.Command("go", "test", "-c", "-cover", "-o", exe, pkg)
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		return 0, originalOutput + buildOut.String(), err
	}

	run := exec.Command(exe, "-test.v", "-test.coverprofile="+profile)
	var runOut bytes.Buffer
	run.Stdout = &runOut
	run.Stderr = &runOut
	if err := run.Run(); err != nil {
		return 0, originalOutput + runOut.String(), err
	}

	output := runOut.String()
	matches := coveragePattern.FindStringSubmatch(output)
	if len(matches) != 2 {
		return 0, output, fmt.Errorf("coverage percentage not found in output: %s", strings.TrimSpace(output))
	}
	coverage, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, output, fmt.Errorf("parse coverage %q: %w", matches[1], err)
	}
	return coverage, output, nil
}
