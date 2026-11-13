package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cov1xFakeGo is a stand-in `go` binary (true external boundary) whose
// behaviour is chosen by the package argument.
const cov1xFakeGo = `#!/bin/sh
if [ "$1" = "test" ] && [ "$2" = "-c" ]; then
  out=""; prev=""; pkg=""
  for a in "$@"; do
    if [ "$prev" = "-o" ]; then out="$a"; fi
    prev="$a"; pkg="$a"
  done
  case "$pkg" in
    *buildfail*) echo "build exploded"; exit 1;;
    *runfail*) printf '#!/bin/sh\necho "binary crashed"\nexit 1\n' > "$out";;
    *nocov*) printf '#!/bin/sh\necho "PASS"\n' > "$out";;
    *) printf '#!/bin/sh\necho "coverage: 61.5%% of statements"\n' > "$out";;
  esac
  chmod +x "$out"
  exit 0
fi
pkg="$2"
case "$pkg" in
  *blocked*) echo "Application Control policy has blocked this file"; exit 1;;
  *fail*) echo "FAIL $pkg"; exit 1;;
  *low*) echo "ok $pkg 0.1s coverage: 3.0% of statements"; exit 0;;
  *nocov*) echo "ok $pkg no test files"; exit 0;;
  *) echo "ok $pkg 0.1s coverage: 55.5% of statements"; exit 0;;
esac
`

func cov1xSetup(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(cov1xFakeGo), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	work := t.TempDir()
	t.Chdir(work)
	return work
}

func TestCov1xParseCoverage(t *testing.T) {
	got, err := parseCoverage("ok x 0.1s coverage: 42.5% of statements")
	if err != nil || got != 42.5 {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := parseCoverage("no numbers here"); err == nil || !strings.Contains(err.Error(), "coverage percentage not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestCov1xRunPackageCoverage(t *testing.T) {
	cov1xSetup(t)
	cov, out, err := runPackageCoverage("./ok")
	if err != nil || cov != 55.5 || !strings.Contains(out, "ok ./ok") {
		t.Fatalf("ok: %v %v %q", cov, err, out)
	}
	if _, out, err := runPackageCoverage("./fail"); err == nil || !strings.Contains(out, "FAIL ./fail") {
		t.Fatalf("fail: %v %q", err, out)
	}
	if _, _, err := runPackageCoverage("./nocov"); err == nil || !strings.Contains(err.Error(), "coverage percentage not found") {
		t.Fatalf("nocov: %v", err)
	}
}

func TestCov1xRunPackageCoverageBlockedFallsBackToBinary(t *testing.T) {
	work := cov1xSetup(t)
	cov, _, err := runPackageCoverage("./internal/blocked")
	if err != nil || cov != 61.5 {
		t.Fatalf("got %v, %v", cov, err)
	}
	// temp exe/profile are cleaned up
	entries, _ := os.ReadDir(filepath.Join(work, ".coverage-bin"))
	if len(entries) != 0 {
		t.Fatalf("expected cleanup, found %v", entries)
	}
}

func TestCov1xRunPackageCoverageBinaryErrors(t *testing.T) {
	t.Run("mkdir fails", func(t *testing.T) {
		work := cov1xSetup(t)
		if err := os.WriteFile(filepath.Join(work, ".coverage-bin"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, out, err := runPackageCoverageBinary("./a", "orig")
		if err == nil || out != "orig" {
			t.Fatalf("got %v %q", err, out)
		}
	})
	t.Run("build fails", func(t *testing.T) {
		cov1xSetup(t)
		_, out, err := runPackageCoverageBinary("./buildfail", "orig;")
		if err == nil || !strings.Contains(out, "orig;") || !strings.Contains(out, "build exploded") {
			t.Fatalf("got %v %q", err, out)
		}
	})
	t.Run("run fails", func(t *testing.T) {
		cov1xSetup(t)
		_, out, err := runPackageCoverageBinary("./runfail", "orig;")
		if err == nil || !strings.Contains(out, "binary crashed") {
			t.Fatalf("got %v %q", err, out)
		}
	})
	t.Run("no coverage in output", func(t *testing.T) {
		cov1xSetup(t)
		_, _, err := runPackageCoverageBinary("./nocov", "")
		if err == nil || !strings.Contains(err.Error(), "coverage percentage not found") {
			t.Fatalf("got %v", err)
		}
	})
}

func cov1xWithThresholds(t *testing.T, ts []threshold) (code *int) {
	t.Helper()
	oldT, oldExit := criticalThresholds, exit
	criticalThresholds = ts
	c := -1
	code = &c
	exit = func(n int) { c = n }
	t.Cleanup(func() { criticalThresholds, exit = oldT, oldExit })
	return code
}

func TestCov1xMainPasses(t *testing.T) {
	cov1xSetup(t)
	code := cov1xWithThresholds(t, []threshold{{pkg: "./ok", min: 50}})
	main()
	if *code != -1 {
		t.Fatalf("exit called with %d", *code)
	}
}

func TestCov1xMainReportsFailures(t *testing.T) {
	cov1xSetup(t)
	code := cov1xWithThresholds(t, []threshold{
		{pkg: "./ok", min: 50},
		{pkg: "./low", min: 10},
		{pkg: "./fail", min: 1},
	})
	// capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })
	main()
	os.Stderr = old
	w.Close()
	data, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	msg := string(data)
	if strings.Contains(msg, "./ok") {
		t.Errorf("passing package must not be reported: %s", msg)
	}
	if *code != 1 {
		t.Fatalf("expected exit 1, got %d", *code)
	}
	for _, want := range []string{"critical coverage gate failed", "./low: coverage 3.0% is below 10.0%", "./fail: go test failed"} {
		if !strings.Contains(msg, want) {
			t.Errorf("stderr missing %q: %s", want, msg)
		}
	}
}

func TestCov1xParseCoverageOutOfRange(t *testing.T) {
	huge := strings.Repeat("9", 400)
	_, err := parseCoverage("coverage: " + huge + "% of statements")
	if err == nil || !strings.Contains(err.Error(), "parse coverage") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
