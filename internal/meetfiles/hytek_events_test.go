package meetfiles_test

import (
	"testing"

	"github.com/dareaquatics/aquify/meetfiles"
)

func TestParseHYVFixture(t *testing.T) {
	result, err := meetfiles.ParseHYV("sample.hyv", readFixture(t, "sample.hyv"))
	if err != nil {
		t.Fatalf("ParseHYV error: %v", err)
	}

	if result.FileType != meetfiles.FileTypeHYV {
		t.Fatalf("FileType = %q, want %q", result.FileType, meetfiles.FileTypeHYV)
	}
	if result.MeetName != "Armada Lc April Meet -April 11-12-2026" {
		t.Fatalf("MeetName = %q", result.MeetName)
	}
	if result.Location != "La Mirada Splash! Aquatic Complex" {
		t.Fatalf("Location = %q", result.Location)
	}
	if result.Course != meetfiles.CourseLongCourseMeters {
		t.Fatalf("Course = %q, want %q", result.Course, meetfiles.CourseLongCourseMeters)
	}
	if len(result.Events) != 106 {
		t.Fatalf("len(Events) = %d, want 106", len(result.Events))
	}
	if len(result.Sessions) != 2 {
		t.Fatalf("len(Sessions) = %d, want 2", len(result.Sessions))
	}
	first := result.Events[0]
	if first.EventNumber != "1" || first.Distance != 200 || first.Gender != "F" || first.Stroke != meetfiles.StrokeIM {
		t.Fatalf("unexpected first HYV event: %+v", first)
	}
	if first.SessionNumber != 0 {
		t.Fatalf("HYV SessionNumber = %d, want 0", first.SessionNumber)
	}
}

func TestParseEV3Fixture(t *testing.T) {
	result, err := meetfiles.ParseEV3("sample.ev3", readFixture(t, "sample.ev3"))
	if err != nil {
		t.Fatalf("ParseEV3 error: %v", err)
	}

	if result.FileType != meetfiles.FileTypeEV3 {
		t.Fatalf("FileType = %q, want %q", result.FileType, meetfiles.FileTypeEV3)
	}
	if result.MeetName != "Armada Lc April Meet -April 11-12-2026" {
		t.Fatalf("MeetName = %q", result.MeetName)
	}
	if result.Location != "La Mirada Splash! Aquatic Complex" {
		t.Fatalf("Location = %q", result.Location)
	}
	if result.Course != meetfiles.CourseShortCourseYards {
		t.Fatalf("Course = %q, want %q", result.Course, meetfiles.CourseShortCourseYards)
	}
	if len(result.Events) != 106 {
		t.Fatalf("len(Events) = %d, want 106", len(result.Events))
	}
	if len(result.Sessions) != 4 {
		t.Fatalf("len(Sessions) = %d, want 4", len(result.Sessions))
	}
	if result.Sessions[0].StartTime != "09:00" {
		t.Fatalf("first session start = %q, want 09:00", result.Sessions[0].StartTime)
	}
	if result.Sessions[1].StartTime != "13:00" {
		t.Fatalf("second session start = %q, want 13:00", result.Sessions[1].StartTime)
	}
	first := result.Events[0]
	if first.EventNumber != "1" || first.Distance != 200 || first.Gender != "F" || first.Stroke != meetfiles.StrokeIM || first.SessionNumber != 1 {
		t.Fatalf("unexpected first EV3 event: %+v", first)
	}
}

func TestReadMeetZip_PrefersMeetSetupFormats(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"setup/sample.ev3":  readFixture(t, "sample.ev3"),
		"setup/sample.hyv":  readFixture(t, "sample.hyv"),
		"setup/results.sd3": []byte("A0 sample"),
		"notes.txt":         []byte("ignored"),
	})

	entries, err := meetfiles.ReadMeetZip(archive)
	if err != nil {
		t.Fatalf("ReadMeetZip error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
}

// A garbled header spanning thousands of years must not expand into one
// session per day.
func TestParseHYV_CapsDerivedSessionsForHugeDateRange(t *testing.T) {
	data := []byte("Meet;01/01/0001;12/31/9999;x;L;Pool\nIgnored;1;F;I;0;0;200;5\n")
	result, err := meetfiles.ParseHYV("huge.hyv", data)
	if err != nil {
		t.Fatalf("ParseHYV error: %v", err)
	}
	if n := len(result.Sessions); n == 0 || n > 366 {
		t.Fatalf("len(Sessions) = %d, want 1..366", n)
	}
}

func TestParseEV3_ExtremeDayIndexIsClamped(t *testing.T) {
	data := "Meet;Pool;04/11/2026;04/12/2026;04/11/2026;YLS;0;15.5;0;x*>\n" +
		"1;1;F;1;I;G;11;12;200;E;0;;;N;6;;;;;;;1;1;999999999;09:00AM;L;0;0;0;0*>\n"
	res, err := meetfiles.ParseEV3("x.ev3", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sessions) == 0 || res.Sessions[0].SessionDate == nil {
		t.Fatalf("no dated session: %+v", res.Sessions)
	}
	if got := res.Sessions[0].SessionDate.Year(); got != 2026 {
		t.Fatalf("session date year = %d, want 2026", got)
	}
}
