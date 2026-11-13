package meetfiles_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/luryann/aquify/meetfiles"
)

// The bca-fall-festival fixtures are a real host's event files: the same
// meet published as .hyv and as .ev3.

func TestParseMeetEvents_RealHYVAndEV3Agree(t *testing.T) {
	hyv := meetfiles.SourceFile{Name: "Meet Events.hyv", Content: readFixture(t, "bca-fall-festival.hyv")}
	ev3 := meetfiles.SourceFile{Name: "Meet Events.ev3", Content: readFixture(t, "bca-fall-festival.ev3")}

	a, err := meetfiles.ParseHYV(hyv.Name, hyv.Content)
	if err != nil {
		t.Fatal(err)
	}
	b, err := meetfiles.ParseEV3(ev3.Name, ev3.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.ParseErrors)+len(b.ParseErrors) != 0 {
		t.Fatalf("parse errors: hyv %v, ev3 %v", a.ParseErrors, b.ParseErrors)
	}
	if diffs := meetfiles.CompareMeetDefinitions(b, a); len(diffs) != 0 {
		t.Fatalf("the two formats must describe the same meet:\n%s", strings.Join(diffs, "\n"))
	}

	def, warnings, err := meetfiles.ParseMeetEvents([]meetfiles.SourceFile{hyv, ev3})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if def.FileType != meetfiles.FileTypeEV3 {
		t.Fatalf("combined parse should build on the .ev3, got %q", def.FileType)
	}
}

func TestParseMeetEvents_RealFileContents(t *testing.T) {
	for _, name := range []string{"bca-fall-festival.hyv", "bca-fall-festival.ev3"} {
		t.Run(name, func(t *testing.T) {
			def, warnings, err := meetfiles.ParseMeetEvents([]meetfiles.SourceFile{{Name: name, Content: readFixture(t, name)}})
			if err != nil || len(warnings) != 0 {
				t.Fatalf("err=%v warnings=%v", err, warnings)
			}
			if def.MeetName != "2026 BCA Fall Festival" || def.Location != "George Mannon Aquatic Center" {
				t.Fatalf("header = %q at %q", def.MeetName, def.Location)
			}
			if def.Course != meetfiles.CourseShortCourseYards {
				t.Fatalf("course = %q", def.Course)
			}
			for label, got := range map[string]string{
				"start":  def.MeetStartDate.Format("2006-01-02"),
				"end":    def.MeetEndDate.Format("2006-01-02"),
				"age-up": def.AgeUpDate.Format("2006-01-02"),
			} {
				want := map[string]string{"start": "2026-11-14", "end": "2026-11-15", "age-up": "2026-11-14"}[label]
				if got != want {
					t.Errorf("%s date = %s, want %s", label, got, want)
				}
			}
			if len(def.Events) != 79 {
				t.Fatalf("events = %d, want 79", len(def.Events))
			}
			byNumber := map[string]meetfiles.ParsedMeetEvent{}
			for _, ev := range def.Events {
				if ev.IsRelay || ev.EntryFeeCents != 600 || ev.Course != meetfiles.CourseShortCourseYards {
					t.Fatalf("event %s: relay=%v fee=%d course=%q", ev.EventNumber, ev.IsRelay, ev.EntryFeeCents, ev.Course)
				}
				byNumber[ev.EventNumber] = ev
			}

			first := byNumber["1"]
			if first.Distance != 200 || first.Stroke != meetfiles.StrokeFreestyle || first.Gender != "X" || *first.MinAge != 7 || *first.MaxAge != 10 {
				t.Fatalf("event 1 = %+v", first)
			}
			if first.TimeStandards[meetfiles.CourseShortCourseYards] != 202790 || first.TimeStandards[meetfiles.CourseLongCourseMeters] != 230990 {
				t.Fatalf("event 1 standards = %v", first.TimeStandards)
			}
			if ev := byNumber["2"]; ev.TimeStandards != nil {
				t.Fatalf("event 2 has no standard, got %v", ev.TimeStandards)
			}
			// Lettered events split one swim into gender/age flights.
			split := byNumber["22B"]
			if split.Distance != 500 || split.Gender != "M" || *split.MinAge != 11 || *split.MaxAge != 12 {
				t.Fatalf("event 22B = %+v", split)
			}
			// "0" (.hyv) and "109" (.ev3) both mean no upper age limit.
			open := byNumber["37"]
			if open.Distance != 1650 || open.Gender != "F" || *open.MinAge != 11 || open.MaxAge != nil {
				t.Fatalf("event 37 = %+v", open)
			}
			if open.TimeStandards[meetfiles.CourseShortCourseYards] != 1292090 {
				t.Fatalf("event 37 standard = %v", open.TimeStandards)
			}
			if im := byNumber["11"]; im.Stroke != meetfiles.StrokeIM {
				t.Fatalf("event 11 stroke = %q", im.Stroke)
			}

			if def.FileType == meetfiles.FileTypeEV3 {
				if len(def.Sessions) != 6 {
					t.Fatalf("sessions = %d, want 6", len(def.Sessions))
				}
				want := []struct{ date, start string }{
					{"2026-11-14", "08:30"}, {"2026-11-14", "11:30"}, {"2026-11-14", "14:30"},
					{"2026-11-15", "08:30"}, {"2026-11-15", "11:30"}, {"2026-11-15", "14:30"},
				}
				for i, s := range def.Sessions {
					if s.SessionNumber != i+1 || s.SessionDate.Format("2006-01-02") != want[i].date || s.StartTime != want[i].start {
						t.Errorf("session %d = %+v", i+1, s)
					}
				}
				if byNumber["22D"].SessionNumber != 2 || byNumber["76"].SessionNumber != 6 {
					t.Fatalf("session assignment: 22D=%d 76=%d", byNumber["22D"].SessionNumber, byNumber["76"].SessionNumber)
				}
			} else if len(def.Sessions) != 2 {
				// The .hyv has no sessions; one per meet day is derived.
				t.Fatalf("derived sessions = %d, want 2", len(def.Sessions))
			}
		})
	}
}

func TestParseMeetEvents_ZipOfBothFiles(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"Meet Events-002/Meet Events.hyv": readFixture(t, "bca-fall-festival.hyv"),
		"Meet Events-002/Meet Events.ev3": readFixture(t, "bca-fall-festival.ev3"),
		"__MACOSX/._Meet Events.ev3":      []byte("junk"),
	})
	def, warnings, err := meetfiles.ParseMeetEvents([]meetfiles.SourceFile{{Name: "Meet Events-002.zip", Content: archive}})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
	if len(def.Events) != 79 || len(def.Sessions) != 6 {
		t.Fatalf("events=%d sessions=%d", len(def.Events), len(def.Sessions))
	}
}

func TestParseMeetEvents_ReportsDisagreement(t *testing.T) {
	hyv := strings.Replace(string(readFixture(t, "bca-fall-festival.hyv")), "2;F;X;I;5;8;50;1;", "2;F;X;I;5;8;100;1;", 1)
	hyv = strings.Replace(hyv, "76;F;M;I;11;0;500;1;;5:59.99;;6;;5:26.69;;;;\r\n", "", 1)
	_, warnings, err := meetfiles.ParseMeetEvents([]meetfiles.SourceFile{
		{Name: "a.hyv", Content: []byte(hyv)},
		{Name: "a.ev3", Content: readFixture(t, "bca-fall-festival.ev3")},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warnings, "\n")
	if len(warnings) != 2 || !strings.Contains(joined, "event 2 differs") || !strings.Contains(joined, "event 76 is in the .ev3 file but not the .hyv file") {
		t.Fatalf("warnings = %q", warnings)
	}
}

func TestParseMeetEvents_RejectsOtherFiles(t *testing.T) {
	_, _, err := meetfiles.ParseMeetEvents([]meetfiles.SourceFile{{Name: "entries.sd3", Content: []byte("A01V3")}})
	if !errors.Is(err, meetfiles.ErrNoMeetEvents) {
		t.Fatalf("err = %v, want ErrNoMeetEvents", err)
	}
	_, warnings, err := meetfiles.ParseMeetEvents([]meetfiles.SourceFile{
		{Name: "entries.hy3", Content: []byte("A102")},
		{Name: "a.hyv", Content: readFixture(t, "bca-fall-festival.hyv")},
	})
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "entries.hy3") {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
}

func TestParseSwimTime(t *testing.T) {
	for in, want := range map[string]int{"47.79": 47790, "3:50.99": 230990, "22:22.99": 1342990, "": 0, "NT": 0, "1:75.00": 0, "x:10.00": 0} {
		if got := meetfiles.ParseSwimTime(in); got != want {
			t.Errorf("ParseSwimTime(%q) = %d, want %d", in, got, want)
		}
	}
}
