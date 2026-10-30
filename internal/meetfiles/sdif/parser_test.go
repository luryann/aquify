package sdif_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/meetfiles/sdif"
)

// ─── Fixture helpers ──────────────────────────────────────────────────────────

// pad right-pads s with spaces to exactly n characters, truncating if longer.
func pad(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

// sdifLine builds a 160-char SDIF data line from a variadic list of (field,
// width) pairs.  The result is not terminated with CRLF.
func sdifLine(parts ...any) string {
	var sb strings.Builder
	for i := 0; i+1 < len(parts); i += 2 {
		s := parts[i].(string)
		n := parts[i+1].(int)
		sb.WriteString(pad(s, n))
	}
	result := sb.String()
	// Pad or truncate to exactly 160 chars.
	if len(result) < 160 {
		result += strings.Repeat(" ", 160-len(result))
	}
	return result[:160]
}

// crlf appends CRLF to a line string.
func crlf(line string) string { return line + "\r\n" }

// ─── Minimal fixture builders ─────────────────────────────────────────────────

func makeA0() string {
	return sdifLine(
		"A0", 2, "1", 1, "", 8,
		"02", 2, // file type: results
		"", 30, // future use
		"TestSoftware", 20, "v1.0", 10,
		"Test Contact", 20, "000-000-0000", 12,
		"03152026", 8, // creation date
		"", 42, "AZ", 2, "", 3,
	)
}

func makeB1() string {
	return sdifLine(
		"B1", 2, "1", 1, "", 8,
		"Spring Invitational", 30, // meet name
		"100 Pool Lane", 22, "", 22, // addr1 addr2
		"Springfield", 20, "AZ", 2, "85001", 10,
		"USA", 3, "1", 1, // country + meet type
		"03152026", 8, "03162026", 8, // start + end
		"0000", 4, "", 8, // altitude + future
		"2", 1, // course SCY
		"", 10,
	)
}

func makeC1() string {
	return sdifLine(
		"C1", 2, "1", 1, "", 8,
		"TESTA", 6, // team code
		"Test Aquatics Club", 30,
		"Test Aquatics", 16,
		"100 Club Rd", 22, "", 22,
		"Springfield", 20, "AZ", 2, "85001", 10,
		"USA", 3, "0", 1, "", 6, " ", 1, "", 10,
	)
}

// makeD0 builds a D0 individual result record.
func makeD0(name, ussNum, dob, ageClass, swimGender, evtGender,
	dist, stroke, evtNum, ageGrp, dateSwum,
	seedTime, seedCourse, finTime, finCourse,
	finHeat, finLane, finPlace string) string {

	return sdifLine(
		"D0", 2, "1", 1, "", 8,
		name, 28, // swimmer name
		ussNum, 12, // USS#
		"A", 1, // attach
		"   ", 3, // citizen
		dob, 8, // DOB
		ageClass, 2,
		swimGender, 1, // swimmer gender
		evtGender, 1, // event gender
		dist, 4, // distance
		stroke, 1, // stroke code
		evtNum, 4, // event number
		ageGrp, 4, // age group
		dateSwum, 8, // date of swim
		seedTime, 8, seedCourse, 1, // seed time + course
		"", 8, " ", 1, // prelim time + course
		"", 8, " ", 1, // swimoff time + course
		finTime, 8, finCourse, 1, // finals time + course
		"  ", 2, "  ", 2, // prelim heat + lane
		finHeat, 2, finLane, 2, // finals heat + lane
		"   ", 3, // prelim place
		finPlace, 3, // finals place
		"    ", 4, "  ", 2, " ", 1, // points, time class, flight
		"", 15, // future use
	)
}

// makeE0 builds an E0 relay event record.
func makeE0(teamCode, evtGender, dist, stroke, evtNum, ageGrp, finTime string) string {
	return sdifLine(
		"E0", 2, "1", 1, "", 8,
		"A", 1, // relay team char
		teamCode, 6,
		"04", 2, // number of F0 records
		evtGender, 1, dist, 4, stroke, 1,
		evtNum, 4, ageGrp, 4,
		"   ", 3, // total age
		"03152026", 8, // date of swim
		"NT", 8, "2", 1, // seed time + course
		"", 8, "2", 1, // prelim time + course
		"", 8, "2", 1, // swimoff time + course
		finTime, 8, "2", 1, // finals time + course
		"  ", 2, "  ", 2, " 1", 2, " 2", 2, // heats + lanes
		"   ", 3, "  1", 3, // places
		"    ", 4, "  ", 2, // points + time class
		"", 59,
	)
}

// makeF0 builds an F0 relay leg record.
func makeF0(teamCode, name, ussNum, dob, gender, legOrder, legTime string) string {
	return sdifLine(
		"F0", 2, "1", 1, "", 12,
		teamCode, 6, "A", 1, // team + relay char
		name, 28, // name
		ussNum, 12, // USS#
		"   ", 3, // citizen
		dob, 8, "  ", 2, // DOB + age
		gender, 1, // gender
		" ", 1, " ", 1, // prelim + swimoff order
		legOrder, 1, // finals order
		legTime, 8, "2", 1, // leg time + course
		"    ", 4, // auto takeof
		"", 14, // USS# new
		"", 15, // preferred first name
		"", 39, // future use
	)
}

func makeZ0() string {
	return sdifLine(
		"Z0", 2, "1", 1, "", 8,
		"02", 2, // file type
		"test file", 30,
		"1", 3, "1", 3, "1", 4, "1", 4, // B+meet+C+team counts
		"2", 6, "2", 6, // D + swimmer counts
		"1", 5, "2", 6, "0", 6, // E + F + G counts
		"0", 5, "0", 3, "0", 3, "0", 3, "0", 3, // batch + member counts
		"", 57,
	)
}

// ─── Valid SD3 test ───────────────────────────────────────────────────────────

func TestParse_ValidSD3(t *testing.T) {
	d0smith := makeD0(
		"Smith, John", "", "02151999", "25", "M", "M",
		"0100", "1", "0001", "1824", "03152026",
		"  59.10 ", "2", "  58.12 ", "2",
		" 1", " 4", "  1",
	)
	d0doe := makeD0(
		"Doe, Jane", "", "05102008", "17", "F", "F",
		"0200", "2", "0002", "1517", "03152026",
		"2:15.00 ", "2", "2:12.45 ", "2",
		" 2", " 3", "  1",
	)
	e0 := makeE0("TESTA", "X", "0200", "7", "0010", "OPEN", "1:55.34 ")
	f0a := makeF0("TESTA", "Smith, John", "", "02151999", "M", "1", "  58.12 ")
	f0b := makeF0("TESTA", "Doe, Jane", "", "05102008", "F", "2", "  32.01 ")

	content := crlf(makeA0()) + crlf(makeB1()) + crlf(makeC1()) +
		crlf(d0smith) + crlf(d0doe) +
		crlf(e0) + crlf(f0a) + crlf(f0b) +
		crlf(makeZ0())

	result, err := sdif.Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}

	// ── Meet metadata ──────────────────────────────────────────────────────
	if result.MeetName != "Spring Invitational" {
		t.Errorf("MeetName = %q, want %q", result.MeetName, "Spring Invitational")
	}
	if result.Course != meetfiles.CourseShortCourseYards {
		t.Errorf("Course = %v, want SCY", result.Course)
	}

	// ── Individual results ────────────────────────────────────────────────
	if len(result.IndividualResults) != 2 {
		t.Fatalf("len(IndividualResults) = %d, want 2", len(result.IndividualResults))
	}

	// First result: John Smith
	smith := result.IndividualResults[0]
	if smith.Swimmer.LastName != "Smith" {
		t.Errorf("smith.LastName = %q, want %q", smith.Swimmer.LastName, "Smith")
	}
	if smith.Swimmer.FirstName != "John" {
		t.Errorf("smith.FirstName = %q, want %q", smith.Swimmer.FirstName, "John")
	}
	if smith.Swimmer.Gender != "M" {
		t.Errorf("smith.Gender = %q, want M", smith.Swimmer.Gender)
	}
	if smith.Event.Distance != 100 {
		t.Errorf("smith.Event.Distance = %d, want 100", smith.Event.Distance)
	}
	if smith.Event.Stroke != meetfiles.StrokeFreestyle {
		t.Errorf("smith.Event.Stroke = %v, want free", smith.Event.Stroke)
	}
	if !smith.FinalsTime.Valid {
		t.Errorf("smith.FinalsTime.Valid = false, want true")
	}
	wantSecs := 58.12
	if diff := smith.FinalsTime.Seconds - wantSecs; diff > 0.001 || diff < -0.001 {
		t.Errorf("smith.FinalsTime.Seconds = %.4f, want %.4f", smith.FinalsTime.Seconds, wantSecs)
	}
	if smith.Heat != 1 || smith.Lane != 4 || smith.Place != 1 {
		t.Errorf("smith heat/lane/place = %d/%d/%d, want 1/4/1", smith.Heat, smith.Lane, smith.Place)
	}

	// Second result: Jane Doe
	doe := result.IndividualResults[1]
	if doe.Swimmer.LastName != "Doe" {
		t.Errorf("doe.LastName = %q, want %q", doe.Swimmer.LastName, "Doe")
	}
	if doe.Event.Distance != 200 {
		t.Errorf("doe.Event.Distance = %d, want 200", doe.Event.Distance)
	}
	if doe.Event.Stroke != meetfiles.StrokeBackstroke {
		t.Errorf("doe.Event.Stroke = %v, want back", doe.Event.Stroke)
	}
	if !doe.FinalsTime.Valid {
		t.Errorf("doe.FinalsTime.Valid = false, want true")
	}
	wantDoeSecs := 2*60 + 12.45
	if diff := doe.FinalsTime.Seconds - wantDoeSecs; diff > 0.001 || diff < -0.001 {
		t.Errorf("doe.FinalsTime.Seconds = %.4f, want %.4f", doe.FinalsTime.Seconds, wantDoeSecs)
	}

	// ── Relay result ──────────────────────────────────────────────────────
	if len(result.RelayResults) != 1 {
		t.Fatalf("len(RelayResults) = %d, want 1", len(result.RelayResults))
	}
	relay := result.RelayResults[0]
	if relay.Event.Stroke != meetfiles.StrokeMedleyRelay {
		t.Errorf("relay.Stroke = %v, want medley_relay", relay.Event.Stroke)
	}
	if relay.Event.Distance != 200 {
		t.Errorf("relay.Distance = %d, want 200", relay.Event.Distance)
	}
	if len(relay.Legs) != 2 {
		t.Errorf("len(relay.Legs) = %d, want 2", len(relay.Legs))
	}

	// ── No parse errors ───────────────────────────────────────────────────
	if len(result.ParseErrors) != 0 {
		t.Errorf("unexpected ParseErrors: %+v", result.ParseErrors)
	}
}

// ─── Malformed SD3 test ───────────────────────────────────────────────────────

func TestParse_MalformedSD3(t *testing.T) {
	// A known-good A0 header, then an unknown record type, then a truncated D0,
	// then a proper Z0 terminator.
	unknownRecord := pad("XX this is an unknown record type", 160)
	truncatedD0 := "D0  1            Smith, Jo" // only 26 chars — too short

	content := crlf(makeA0()) +
		crlf(unknownRecord) +
		truncatedD0 + "\r\n" +
		crlf(makeZ0())

	result, err := sdif.Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}

	// Should have exactly 2 parse errors: unknown record ID + truncated D0.
	if len(result.ParseErrors) < 2 {
		t.Errorf("len(ParseErrors) = %d, want >= 2; errors: %+v", len(result.ParseErrors), result.ParseErrors)
	}

	codes := make(map[meetfiles.ErrorCode]int)
	for _, pe := range result.ParseErrors {
		codes[pe.Code]++
	}
	if codes[meetfiles.ErrCodeMalformedRecord] < 2 {
		t.Errorf("expected >= 2 malformed_record errors, got %d; all: %+v", codes[meetfiles.ErrCodeMalformedRecord], result.ParseErrors)
	}

	// No individual results should have been parsed.
	if len(result.IndividualResults) != 0 {
		t.Errorf("expected 0 IndividualResults from malformed file, got %d", len(result.IndividualResults))
	}
}

// ─── Special time codes ───────────────────────────────────────────────────────

func TestParse_SpecialTimeCodes(t *testing.T) {
	d0nt := makeD0(
		"Johnson, Amy", "", "06201995", "30", "F", "F",
		"0050", "1", "0003", "OPEN", "03152026",
		"NT      ", "2", "NT      ", "2",
		"  ", "  ", "   ",
	)

	content := crlf(makeA0()) + crlf(makeB1()) + crlf(d0nt) + crlf(makeZ0())

	result, err := sdif.Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result.IndividualResults) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.IndividualResults))
	}

	r := result.IndividualResults[0]
	if r.SeedTime.Valid {
		t.Errorf("NT seed time should not be valid, got Valid=true")
	}
	if r.SeedTime.Raw != "NT" {
		t.Errorf("SeedTime.Raw = %q, want %q", r.SeedTime.Raw, "NT")
	}
	if r.FinalsTime.Valid {
		t.Errorf("NT finals time should not be valid")
	}
}

// ─── LF-only line endings ─────────────────────────────────────────────────────

func TestParse_LFLineEndings(t *testing.T) {
	d0 := makeD0(
		"Brown, Alice", "", "09012000", "25", "F", "F",
		"0100", "4", "0005", "OPEN", "03152026",
		"1:02.34 ", "2", "1:01.11 ", "2",
		" 3", " 5", "  2",
	)

	// Use LF only — no CR.
	lf := func(line string) string { return line + "\n" }
	content := lf(makeA0()) + lf(makeB1()) + lf(d0) + lf(makeZ0())

	result, err := sdif.Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result.IndividualResults) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.IndividualResults))
	}
	if result.IndividualResults[0].Swimmer.LastName != "Brown" {
		t.Errorf("LastName = %q, want Brown", result.IndividualResults[0].Swimmer.LastName)
	}
}

// ─── F0 without E0 ───────────────────────────────────────────────────────────

func TestParse_F0WithoutE0(t *testing.T) {
	f0orphan := makeF0("TESTA", "Orphan, Kid", "", "01012010", "M", "1", "  32.00 ")

	content := crlf(makeA0()) + crlf(f0orphan) + crlf(makeZ0())

	result, err := sdif.Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Should record an error for orphaned F0.
	if len(result.ParseErrors) == 0 {
		t.Error("expected at least 1 ParseError for orphaned F0 record")
	}
	found := false
	for _, pe := range result.ParseErrors {
		if pe.Code == meetfiles.ErrCodeMalformedRecord && strings.Contains(pe.Message, "F0") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected malformed_record error mentioning F0; got: %+v", result.ParseErrors)
	}
}

// ─── Empty file ───────────────────────────────────────────────────────────────

func TestParse_EmptyFile(t *testing.T) {
	result, err := sdif.Parse([]byte{})
	if err != nil {
		t.Fatalf("Parse error on empty input: %v", err)
	}
	if len(result.IndividualResults) != 0 {
		t.Errorf("expected 0 results for empty file")
	}
	if len(result.ParseErrors) != 0 {
		t.Errorf("expected 0 errors for empty file, got %+v", result.ParseErrors)
	}
}

// ─── Non-numeric time fields ──────────────────────────────────────────────────

func TestParse_NonNumericTimesAreInvalid(t *testing.T) {
	for _, raw := range []string{"NaN     ", "Inf     ", "-5.00   ", "1e2     ", "1:xx.00 ", "-1:00.00"} {
		d0 := makeD0("Poe, Ed", "", "01012005", "20", "M", "M", "0100", "1", "0001", "OPEN",
			"03152026", raw, "2", raw, "2", " 1", " 1", "  1")
		result, err := sdif.Parse([]byte(crlf(makeA0()) + crlf(d0)))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.IndividualResults) != 1 {
			t.Fatalf("%q: got %d results", raw, len(result.IndividualResults))
		}
		if ft := result.IndividualResults[0].FinalsTime; ft.Valid {
			t.Errorf("%q: FinalsTime.Valid = true (%v s); want false", raw, ft.Seconds)
		}
	}
}

// ─── Review-pass regressions ─────────────────────────────────────────────────

func TestParse_RelayEventCourse(t *testing.T) {
	data := makeA0() + "\r\n" + makeB1() + "\r\n" + makeC1() + "\r\n" +
		makeE0("TESTA", "X", "0200", "7", "0010", "OPEN", "1:55.34 ") + "\r\n" + makeZ0() + "\r\n"
	res, err := sdif.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RelayResults) != 1 {
		t.Fatalf("relays = %d", len(res.RelayResults))
	}
	if got := res.RelayResults[0].Event.Course; got != meetfiles.CourseShortCourseYards {
		t.Fatalf("relay course = %q, want SCY", got)
	}
}

func TestParse_CROnlyAndLongLines(t *testing.T) {
	data := makeA0() + "\r" + makeB1() + "\r" +
		makeD0("Doe, Jane", "123456789ABC", "01012010", "16", "F", "F", "0100", "1", "0005", "1517", "03152026", "NT", "2", "55.00", "2", "1", "1", "1") + "\r" + makeZ0() + "\r"
	res, err := sdif.Parse([]byte(data))
	if err != nil {
		t.Fatalf("CR-only file errored: %v", err)
	}
	if len(res.IndividualResults) != 1 {
		t.Fatalf("results = %d, want 1", len(res.IndividualResults))
	}
	// A >64KB junk line must not abort the parse.
	long := strings.Repeat("X", 200_000)
	if _, err := sdif.Parse([]byte(makeA0() + "\n" + long + "\n" + makeZ0() + "\n")); err != nil {
		t.Fatalf("long line errored: %v", err)
	}
}

func TestParse_NonASCIINameNotSplit(t *testing.T) {
	// 28-byte name field; the last rune straddles the field boundary.
	name := strings.Repeat("A", 27) + "é" + ", X"
	line := makeD0(name, "123456789ABC", "01012010", "16", "F", "F", "0100", "1", "0005", "1517", "03152026", "NT", "2", "55.00", "2", "1", "1", "1")
	res, err := sdif.Parse([]byte(line + "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res.IndividualResults {
		if !utf8.ValidString(r.Swimmer.LastName) || !utf8.ValidString(r.Swimmer.FirstName) {
			t.Fatalf("invalid UTF-8 name: %q / %q", r.Swimmer.LastName, r.Swimmer.FirstName)
		}
	}
}
