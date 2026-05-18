package meetfiles_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dareaquatics/aquify/meetfiles"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestParseHYVFixture(t *testing.T) {
	result, err := meetfiles.ParseHYV("sample.hyv", loadFixture(t, "sample.hyv"))
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
	result, err := meetfiles.ParseEV3("sample.ev3", loadFixture(t, "sample.ev3"))
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
		"setup/sample.ev3":  loadFixture(t, "sample.ev3"),
		"setup/sample.hyv":  loadFixture(t, "sample.hyv"),
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
