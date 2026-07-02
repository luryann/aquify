//go:build ignore

// build_fixtures.go generates the testdata fixture files for meetfiles tests.
// Run once with: go run internal/meetfiles/testdata/build_fixtures.go
//
// The fixtures are checked in so that tests do not need to regenerate them.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)

	writeFile := func(name, content string) {
		path := filepath.Join(dir, "fixtures", name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", name, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", name)
	}

	// Each SDIF line is 160 data chars + CRLF.
	pad := func(s string, width int) string {
		if len(s) >= width {
			return s[:width]
		}
		return s + strings.Repeat(" ", width-len(s))
	}
	sdifLine := func(parts ...any) string {
		var sb strings.Builder
		for i := 0; i+1 < len(parts); i += 2 {
			sb.WriteString(pad(parts[i].(string), parts[i+1].(int)))
		}
		line := sb.String()
		if len(line) < 160 {
			line += strings.Repeat(" ", 160-len(line))
		}
		return line[:160]
	}

	// Build a minimal valid SD3 result file.
	// File type 02 = meet results.
	a0 := sdifLine(
		"A0", 2, "1", 1, "", 8,
		"02", 2,
		"", 30,
		"TestSoftware", 20, "v1.0", 10,
		"Test Contact", 20, "000-000-0000", 12,
		"03152026", 8,
		"", 42, "AZ", 2, "", 3,
	)

	b1 := sdifLine(
		"B1", 2, "1", 1, "", 8,
		"Spring Invitational", 30,
		"100 Pool Lane", 22, "", 22,
		"Springfield", 20, "AZ", 2, "85001", 10,
		"USA", 3, "1", 1,
		"03152026", 8, "03162026", 8,
		"0000", 4, "", 8,
		"2", 1,
		"", 10,
	)

	c1 := sdifLine(
		"C1", 2, "1", 1, "", 8,
		"TESTA", 6,
		"Test Aquatics Club", 30,
		"Test Aquatics", 16,
		"100 Club Rd", 22, "", 22,
		"Springfield", 20, "AZ", 2, "85001", 10,
		"USA", 3, "0", 1, "", 6, " ", 1, "", 10,
	)

	// D0 for John Smith, 100 Free SCY, finals 58.12
	//                         12345678901234567890123456789012345678901234567890
	// Name at col 12, len 28: "Smith, John               "
	// USS# at col 40, len 12: "            "  (blank)
	// Attach at 52: "A"
	// blank at 53-55 (3 chars)
	// DOB at 56, len 8: "02151999"
	// Age at 64, len 2: "25"
	// Sex at 66: "M"
	// Event Sex at 67: "M"
	// Distance at 68, len 4: "0100"
	// Stroke at 72: "1" (freestyle)
	// EventNum at 73, len 4: "0001"
	// AgeGroup at 77, len 4: "1824"
	// DateSwum at 81, len 8: "03152026"
	// SeedTime at 89, len 8: "  59.10 "
	// SeedCourse at 97: "2" (SCY)
	// PrelimTime at 98, len 8: "        " (blank)
	// SwimoffTime at 106... etc up to col 116
	// FinalsTime at 116, len 8: "  58.12 "
	// FinalsCourse at 124: "2"
	// PrelimHeat 125-126: "  "
	// PrelimLane 127-128: "  "
	// FinalsHeat 129-130: " 1"
	// FinalsLane 131-132: " 4"
	// PrelimPlace 133-135: "   "
	// FinalsPlace 136-138: "  1"
	d0smith := buildD0("Smith, John", "", "02151999", "25", "M", "M", "0100", "1", "0001", "1824", "03152026", "  59.10 ", "2", "  58.12 ", "2", " 1", " 4", "  1")
	// D0 for Jane Doe, 200 Back SCY, finals 2:12.45
	d0doe := buildD0("Doe, Jane", "", "05102008", "17", "F", "F", "0200", "2", "0002", "1517", "03152026", "2:15.00 ", "2", "2:12.45 ", "2", " 2", " 3", "  1")

	// E0 relay: mixed 200 medley relay, event 10
	e0 := sdifLine(
		"E0", 2, "1", 1, "", 8,
		"A", 1,
		"TESTA", 6,
		"04", 2,
		"X", 1, "0200", 4, "7", 1,
		"0010", 4, "OPEN", 4,
		"   ", 3,
		"03152026", 8,
		"NT", 8, "2", 1,
		"", 8, "2", 1,
		"", 8, "2", 1,
		"1:55.34 ", 8, "2", 1,
		"  ", 2, "  ", 2, " 1", 2, " 2", 2,
		"   ", 3, "  1", 3,
		"    ", 4, "  ", 2,
		"", 59,
	)

	// F0 leg 1: Smith
	f0a := buildF0("TESTA", "Smith, John", "", "02151999", "M", "1", "  58.12 ")
	// F0 leg 2: Doe
	f0b := buildF0("TESTA", "Doe, Jane", "", "05102008", "F", "2", "  32.01 ")

	z0 := sdifLine(
		"Z0", 2, "1", 1, "", 8,
		"02", 2,
		"test file", 30,
		"1", 3, "1", 3, "1", 4, "1", 4,
		"2", 6, "2", 6,
		"1", 5, "2", 6, "0", 6,
		"0", 5, "0", 3, "0", 3, "0", 3, "0", 3,
		"", 57,
	)

	crlf := "\r\n"
	validSD3 := a0 + crlf + b1 + crlf + c1 + crlf +
		d0smith + crlf + d0doe + crlf +
		e0 + crlf + f0a + crlf + f0b + crlf +
		z0 + crlf

	writeFile("valid.sd3", validSD3)

	// Malformed SD3: valid A0, then a line with unknown record ID, then a
	// truncated D0 line, then valid Z0.
	badRecord := pad("XX this is an unknown record type", 160)
	truncatedD0 := "D0  1            Smith, Jo" // only 26 chars — too short
	malformedSD3 := a0 + crlf + badRecord + crlf + truncatedD0 + crlf + z0 + crlf
	writeFile("malformed.sd3", malformedSD3)

	// Minimal CL2 scaffold fixture.
	// CL2 format is proprietary; this fixture exercises the scaffold.
	cl2Content := "CL2 SCAFFOLD SAMPLE FILE - proprietary format\r\n" +
		"EVENT 001 100 FREE SCY\r\n" +
		"HEAT  1  LANE  1  SMITH     JOHN    58.12\r\n" +
		"HEAT  1  LANE  2  DOE       JANE    32.01\r\n" +
		"MYSTERY vendor-specific payload\r\n"
	writeFile("sample.cl2", cl2Content)

	fmt.Println("all fixtures written")
}

// buildD0 constructs a D0 line from its key fields.
// Fields not supplied are padded with blanks to their required widths.
func buildD0(name, ussNum, dob, ageClass, swimmerGender, eventGender,
	distance, stroke, eventNum, ageGroup, dateSwum,
	seedTime, seedCourse, finalsTime, finalsCourse,
	finalsHeat, finalsLane, finalsPlace string) string {

	pad := func(s string, n int) string {
		if len(s) >= n {
			return s[:n]
		}
		return s + strings.Repeat(" ", n-len(s))
	}
	sdifLine := func(parts ...any) string {
		var sb strings.Builder
		for i := 0; i+1 < len(parts); i += 2 {
			sb.WriteString(pad(parts[i].(string), parts[i+1].(int)))
		}
		line := sb.String()
		if len(line) < 160 {
			line += strings.Repeat(" ", 160-len(line))
		}
		return line[:160]
	}

	// col 1-2: record ID
	// col 3: org
	// col 4-11: future use (8)
	// col 12-39: name (28)
	// col 40-51: uss# (12)
	// col 52: attach
	// col 53-55: citizen (3)
	// col 56-63: dob (8)
	// col 64-65: age/class (2)
	// col 66: swimmer gender
	// col 67: event gender
	// col 68-71: distance (4)
	// col 72: stroke
	// col 73-76: event number (4)
	// col 77-80: age group (4)
	// col 81-88: date of swim (8)
	// col 89-96: seed time (8)
	// col 97: seed course
	// col 98-105: prelim time (8)
	// col 106: prelim course
	// col 107-114: swimoff time (8)
	// col 115: swimoff course
	// col 116-123: finals time (8)
	// col 124: finals course
	// col 125-126: prelim heat (2)
	// col 127-128: prelim lane (2)
	// col 129-130: finals heat (2)
	// col 131-132: finals lane (2)
	// col 133-135: prelim place (3)
	// col 136-138: finals place (3)
	// col 139-142: points (4)
	// col 143-144: time class (2)
	// col 145: flight
	// col 146-160: future use (15)

	return sdifLine(
		"D0", 2, "1", 1, "", 8,
		name, 28,
		ussNum, 12,
		"A", 1,
		"   ", 3,
		dob, 8,
		ageClass, 2,
		swimmerGender, 1,
		eventGender, 1,
		distance, 4,
		stroke, 1,
		eventNum, 4,
		ageGroup, 4,
		dateSwum, 8,
		seedTime, 8, seedCourse, 1,
		"", 8, " ", 1,
		"", 8, " ", 1,
		finalsTime, 8, finalsCourse, 1,
		"  ", 2, "  ", 2,
		finalsHeat, 2, finalsLane, 2,
		"   ", 3,
		finalsPlace, 3,
		"    ", 4, "  ", 2, " ", 1,
		"", 15,
	)
}

// buildF0 constructs an F0 relay-leg line.
func buildF0(teamCode, name, ussNum, dob, gender, legOrder, legTime string) string {
	pad := func(s string, n int) string {
		if len(s) >= n {
			return s[:n]
		}
		return s + strings.Repeat(" ", n-len(s))
	}
	sdifLine := func(parts ...any) string {
		var sb strings.Builder
		for i := 0; i+1 < len(parts); i += 2 {
			sb.WriteString(pad(parts[i].(string), parts[i+1].(int)))
		}
		line := sb.String()
		if len(line) < 160 {
			line += strings.Repeat(" ", 160-len(line))
		}
		return line[:160]
	}

	// col 1-2: F0
	// col 3: org
	// col 4-15: future use (12)
	// col 16-21: team code (6)
	// col 22: relay team char (1)
	// col 23-50: name (28)
	// col 51-62: uss# (12)
	// col 63-65: citizen (3)
	// col 66-73: dob (8)
	// col 74-75: age/class (2)
	// col 76: gender
	// col 77: order prelim
	// col 78: order swimoff
	// col 79: order finals
	// col 80-87: leg time (8)
	// col 88: course
	// col 89-92: auto takeof (4)
	// col 93-106: uss# new (14)
	// col 107-121: preferred first name (15)
	// col 122-160: future use (39)

	return sdifLine(
		"F0", 2, "1", 1, "", 12,
		teamCode, 6, "A", 1,
		name, 28,
		ussNum, 12,
		"   ", 3,
		dob, 8, "  ", 2,
		gender, 1,
		" ", 1, " ", 1,
		legOrder, 1,
		legTime, 8, "2", 1,
		"    ", 4,
		"", 14,
		"", 15,
		"", 39,
	)
}
