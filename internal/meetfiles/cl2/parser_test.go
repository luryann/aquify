package cl2_test

import (
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/meetfiles/cl2"
)

// sampleCL2 is a minimal CL2 scaffold fixture that exercises the known-prefix
// paths and the unrecognised-line fallback.
const sampleCL2 = `CL2 SCAFFOLD SAMPLE FILE - proprietary format
EVENT 001 100 FREE SCY
HEAT  1  LANE  1  SMITH     JOHN    58.12
HEAT  1  LANE  2  DOE       JANE    32.01
SOME UNRECOGNISED LINE TYPE XYZ
`

func TestParse_CL2Scaffold(t *testing.T) {
	result, err := cl2.Parse([]byte(sampleCL2))
	if err != nil {
		t.Fatalf("cl2.Parse returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("cl2.Parse returned nil result")
	}

	// File type should be CL2.
	if result.FileType != meetfiles.FileTypeCL2 {
		t.Errorf("FileType = %v, want %v", result.FileType, meetfiles.FileTypeCL2)
	}

	// CL2 is a scaffold: individual results will be empty until the real
	// column layout is implemented.
	if len(result.IndividualResults) != 0 {
		t.Errorf("expected 0 IndividualResults from CL2 scaffold, got %d", len(result.IndividualResults))
	}

	// At least the unrecognised line should produce a parse error.
	if len(result.ParseErrors) == 0 {
		t.Error("expected at least 1 ParseError for unrecognised CL2 line")
	}

	// Verify the error code is correct.
	for _, pe := range result.ParseErrors {
		if pe.Code != meetfiles.ErrCodeMalformedRecord {
			t.Errorf("unexpected error code %q; want malformed_record", pe.Code)
		}
	}
}

func TestParse_CL2EmptyFile(t *testing.T) {
	result, err := cl2.Parse([]byte{})
	if err != nil {
		t.Fatalf("cl2.Parse error on empty input: %v", err)
	}
	if len(result.IndividualResults) != 0 {
		t.Errorf("expected 0 results for empty CL2 file")
	}
	if len(result.ParseErrors) != 0 {
		t.Errorf("expected 0 errors for empty CL2 file, got %+v", result.ParseErrors)
	}
}

func TestParse_CL2OnlyHeaderAndEvents(t *testing.T) {
	// A file that only has CL2 header and EVENT lines (all recognised prefixes).
	content := "CL2 SAMPLE\r\nEVENT 001 50 FREE SCY\r\nEVENT 002 100 BACK SCY\r\n"
	result, err := cl2.Parse([]byte(content))
	if err != nil {
		t.Fatalf("cl2.Parse error: %v", err)
	}
	// No unrecognised lines → no parse errors expected.
	if len(result.ParseErrors) != 0 {
		t.Errorf("expected 0 parse errors, got %+v", result.ParseErrors)
	}
}

func TestParse_CL2MalformedLines(t *testing.T) {
	// All lines are unrecognised.
	lines := strings.Join([]string{
		"GARBAGE LINE 1",
		"GARBAGE LINE 2",
		"GARBAGE LINE 3",
	}, "\r\n") + "\r\n"

	result, err := cl2.Parse([]byte(lines))
	if err != nil {
		t.Fatalf("cl2.Parse error: %v", err)
	}
	if len(result.ParseErrors) != 3 {
		t.Errorf("expected 3 parse errors, got %d: %+v", len(result.ParseErrors), result.ParseErrors)
	}
}
