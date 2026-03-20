package meetfiles_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/meetfiles/cl2"
	"github.com/dareaquatics/aquify/meetfiles/sdif"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", "fixtures", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return raw
}

func TestParserFixtureDirectory_SD3ValidFixture(t *testing.T) {
	t.Parallel()

	result, err := sdif.Parse(readFixture(t, "valid.sd3"))
	if err != nil {
		t.Fatalf("Parse valid fixture: %v", err)
	}
	if result.MeetName != "Spring Invitational" {
		t.Fatalf("expected meet name from fixture, got %q", result.MeetName)
	}
	if len(result.IndividualResults) != 2 {
		t.Fatalf("expected 2 individual results, got %d", len(result.IndividualResults))
	}
}

func TestParserFixtureDirectory_MalformedFixture(t *testing.T) {
	t.Parallel()

	result, err := sdif.Parse(readFixture(t, "malformed.sd3"))
	if err != nil {
		t.Fatalf("Parse malformed fixture returned error: %v", err)
	}
	if len(result.ParseErrors) == 0 {
		t.Fatal("expected malformed fixture to produce parse errors")
	}
}

func TestParserFixtureDirectory_CL2Fixture(t *testing.T) {
	t.Parallel()

	result, err := cl2.Parse(readFixture(t, "sample.cl2"))
	if err != nil {
		t.Fatalf("Parse CL2 fixture: %v", err)
	}
	if result.FileType != meetfiles.FileTypeCL2 {
		t.Fatalf("expected CL2 fixture file type, got %q", result.FileType)
	}
	if len(result.ParseErrors) == 0 {
		t.Fatal("expected CL2 scaffold fixture to retain unrecognised-line coverage")
	}
}

func TestLargeImportFixtureParsesWithinBudget(t *testing.T) {
	t.Parallel()

	raw := string(readFixture(t, "valid.sd3"))
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n")), "\n")
	if len(lines) < 5 {
		t.Fatal("valid fixture does not contain enough lines to construct a large input")
	}

	header := lines[:3]
	var d0Lines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "D0") {
			d0Lines = append(d0Lines, line)
		}
	}
	if len(d0Lines) == 0 {
		t.Fatal("valid fixture does not contain D0 result rows")
	}

	var builder strings.Builder
	for _, line := range header {
		builder.WriteString(line)
		builder.WriteString("\r\n")
	}
	for i := 0; i < 500; i++ {
		for _, line := range d0Lines {
			builder.WriteString(line)
			builder.WriteString("\r\n")
		}
	}
	builder.WriteString(lines[len(lines)-1])
	builder.WriteString("\r\n")

	start := time.Now()
	result, err := sdif.Parse([]byte(builder.String()))
	if err != nil {
		t.Fatalf("Parse large fixture: %v", err)
	}
	if len(result.IndividualResults) != 1000 {
		t.Fatalf("expected 1000 individual results, got %d", len(result.IndividualResults))
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("expected large fixture parse within 3s, took %s", elapsed)
	}
}
