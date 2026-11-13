package meetfiles_test

import (
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/meetfiles"
)

func cov1xHYVRow(num, gender, kind, minAge, maxAge, dist, stroke string) string {
	return strings.Join([]string{num, "", gender, kind, minAge, maxAge, dist, stroke}, ";")
}

func cov1xEV3Row(num, kind, gender, minAge, maxAge, dist, stroke, session, day, clock string) string {
	f := make([]string, 25)
	f[1], f[4], f[5], f[6], f[7], f[8], f[9] = num, kind, gender, minAge, maxAge, dist, stroke
	f[21], f[23], f[24] = session, day, clock
	return strings.Join(f, ";")
}

func TestCov1xParseHYVHeaderErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"empty", " \r\n\n", "hyv: file is empty"},
		{"short header", "Meet;01/01/2026;01/02/2026", "header has 3 fields"},
		{"bad start", "Meet;13/45/2026;01/02/2026;;Y;Pool", "hyv: parse start date"},
		{"bad end", "Meet;01/01/2026;nope;;Y;Pool", "hyv: parse end date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := meetfiles.ParseHYV("x.hyv", []byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q (res=%v)", err, tt.want, res)
			}
			if res != nil {
				t.Fatalf("expected nil result on error")
			}
		})
	}
}

func TestCov1xParseHYVEventsAndErrors(t *testing.T) {
	lines := []string{
		"Test Meet*>;01/05/2026;01/06/2026;;M;Pool A",
		cov1xHYVRow("1", "X", "I", "10", "12", "50", "4"),
		cov1xHYVRow("2", "Q", "I", "", "", "100", "9"),
		cov1xHYVRow("3", "f", "R", "13", "99", "200", "5"),
		cov1xHYVRow("", "F", "I", "", "", "50", "1"),
		"short;1",
		cov1xHYVRow("5", "F", "I", "", "", "abc", "1"),
		cov1xHYVRow("6", "F", "I", "", "", "50", "x"),
		cov1xHYVRow("7", "M", "I", "xx", "0", "50", "1"),
	}
	res, err := meetfiles.ParseHYV("meet.hyv", []byte(strings.Join(lines, "\r\n")))
	if err != nil {
		t.Fatalf("ParseHYV: %v", err)
	}
	if res.Course != meetfiles.CourseShortCourseMeters || res.MeetName != "Test Meet" {
		t.Fatalf("course/name = %q/%q", res.Course, res.MeetName)
	}
	if len(res.Events) != 4 {
		t.Fatalf("events = %d, want 4: %+v", len(res.Events), res.Events)
	}
	e := res.Events
	if e[0].Gender != "X" || e[0].Stroke != meetfiles.StrokeButterfly || *e[0].MinAge != 10 || *e[0].MaxAge != 12 {
		t.Fatalf("event 1 = %+v", e[0])
	}
	if e[1].Gender != "O" || e[1].Stroke != meetfiles.StrokeUnknown || e[1].MinAge != nil || e[1].MaxAge != nil {
		t.Fatalf("event 2 = %+v", e[1])
	}
	if !e[2].IsRelay || e[2].Stroke != meetfiles.StrokeMedleyRelay || e[2].Gender != "F" || e[2].MaxAge != nil || *e[2].MinAge != 13 {
		t.Fatalf("event 3 = %+v", e[2])
	}
	if e[3].EventNumber != "7" || e[3].MinAge != nil || e[3].MaxAge != nil {
		t.Fatalf("event 7 = %+v", e[3])
	}
	if len(res.ParseErrors) != 3 {
		t.Fatalf("parse errors = %+v", res.ParseErrors)
	}
	wantMsgs := []string{"expected at least 8 fields, got 2", `invalid distance "abc"`, `invalid stroke code "x"`}
	for i, pe := range res.ParseErrors {
		if !strings.Contains(pe.Message, wantMsgs[i]) || pe.Code != meetfiles.ErrCodeMalformedRecord {
			t.Fatalf("parse error %d = %+v", i, pe)
		}
	}
	if res.ParseErrors[0].LineNumber != 6 || res.ParseErrors[1].LineNumber != 7 || res.ParseErrors[2].LineNumber != 8 {
		t.Fatalf("line numbers = %+v", res.ParseErrors)
	}
	if len(res.Sessions) != 2 || res.Sessions[1].Name != "Session 2" {
		t.Fatalf("sessions = %+v", res.Sessions)
	}
}

func TestCov1xHytekCourseCodesAndSessionDates(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }
	tests := []struct {
		name     string
		header   string
		course   meetfiles.Course
		sessions []time.Time // zero value => nil SessionDate
	}{
		{"yards", "M;01/05/2026;01/05/2026;;y;L", meetfiles.CourseShortCourseYards, []time.Time{d(2026, 1, 5)}},
		{"long", "M;01/05/2026;01/06/2026;;LCM;L", meetfiles.CourseLongCourseMeters, []time.Time{d(2026, 1, 5), d(2026, 1, 6)}},
		{"unknown letter", "M;01/05/2026;;;Z;L", meetfiles.CourseUnknown, []time.Time{d(2026, 1, 5)}},
		{"empty course, start missing", "M;;01/07/2026;;;L", meetfiles.CourseUnknown, []time.Time{d(2026, 1, 7)}},
		{"end before start", "M;01/09/2026;01/07/2026;;S;L", meetfiles.CourseShortCourseMeters, []time.Time{d(2026, 1, 9)}},
		{"no dates", "M;;;;Y;L", meetfiles.CourseShortCourseYards, []time.Time{{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := meetfiles.ParseHYV("x.hyv", []byte(tt.header))
			if err != nil {
				t.Fatal(err)
			}
			if res.Course != tt.course {
				t.Fatalf("course = %q, want %q", res.Course, tt.course)
			}
			if len(res.Sessions) != len(tt.sessions) {
				t.Fatalf("sessions = %+v", res.Sessions)
			}
			for i, want := range tt.sessions {
				got := res.Sessions[i]
				if want.IsZero() {
					if got.SessionDate != nil || got.Name != "Session 1" || got.Section != "Day 1" {
						t.Fatalf("session = %+v", got)
					}
					continue
				}
				if got.SessionDate == nil || !got.SessionDate.Equal(want) || got.SessionNumber != i+1 {
					t.Fatalf("session %d = %+v, want %v", i, got, want)
				}
			}
		})
	}
}

func TestCov1xParseEV3HeaderErrors(t *testing.T) {
	tests := []struct {
		name, data, want string
	}{
		{"empty", "", "ev3: file is empty"},
		{"short header", "Meet;Loc;01/01/2026", "header has 3 fields"},
		{"bad start", "Meet;Loc;zz;01/02/2026;;L", "ev3: parse start date"},
		{"bad end", "Meet;Loc;01/01/2026;zz;;L", "ev3: parse end date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := meetfiles.ParseEV3("x.ev3", []byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) || res != nil {
				t.Fatalf("res=%v err=%v, want %q", res, err, tt.want)
			}
		})
	}
}

func TestCov1xParseEV3SessionsEventsAndErrors(t *testing.T) {
	lines := []string{
		"EV3 Meet;Big Pool;01/10/2026;01/12/2026;;L",
		cov1xEV3Row("1", "I", "G", "9", "10", "50", "a", "1", "1", "09:00AM"),
		cov1xEV3Row("2", "I", "b", "", "", "100", "B", "1", "1", "10:00AM"),
		cov1xEV3Row("3", "I", "X", "", "99", "200", "C", "2", "2", ""),
		cov1xEV3Row("4", "R", "M", "", "", "400", "E", "2", "2", "02:30PM"),
		cov1xEV3Row("5", "R", "F", "", "", "400", "A", "3", "x", "13:99XX"),
		cov1xEV3Row("6", "I", "?", "", "", "100", "D", "3", "0", ""),
		cov1xEV3Row("7", "I", "F", "", "", "100", "Z", "4", "9999", ""),
		cov1xEV3Row("", "I", "F", "", "", "100", "A", "1", "1", ""),
		"too;short",
		cov1xEV3Row("8", "I", "F", "", "", "zero", "A", "1", "1", ""),
		cov1xEV3Row("9", "I", "F", "", "", "50", "A", "0", "1", ""),
	}
	res, err := meetfiles.ParseEV3("m.ev3", []byte(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if res.Course != meetfiles.CourseLongCourseMeters || res.Location != "Big Pool" {
		t.Fatalf("header = %+v", res)
	}
	if len(res.Events) != 7 {
		t.Fatalf("events = %d", len(res.Events))
	}
	e := res.Events
	if e[0].Gender != "F" || e[0].Stroke != meetfiles.StrokeFreestyle || e[0].SessionNumber != 1 {
		t.Fatalf("e0 = %+v", e[0])
	}
	if e[1].Gender != "M" || e[1].Stroke != meetfiles.StrokeBackstroke {
		t.Fatalf("e1 = %+v", e[1])
	}
	if e[2].Gender != "X" || e[2].Stroke != meetfiles.StrokeBreaststroke || e[2].MaxAge != nil {
		t.Fatalf("e2 = %+v", e[2])
	}
	if !e[3].IsRelay || e[3].Stroke != meetfiles.StrokeMedleyRelay {
		t.Fatalf("e3 = %+v", e[3])
	}
	if e[4].Stroke != meetfiles.StrokeFreestyleRelay {
		t.Fatalf("e4 = %+v", e[4])
	}
	if e[5].Gender != "O" || e[5].Stroke != meetfiles.StrokeButterfly {
		t.Fatalf("e5 = %+v", e[5])
	}
	if e[6].Stroke != meetfiles.StrokeUnknown {
		t.Fatalf("e6 = %+v", e[6])
	}
	if len(res.ParseErrors) != 3 {
		t.Fatalf("parse errors = %+v", res.ParseErrors)
	}
	for i, want := range []string{"expected at least 25 fields, got 2", `invalid distance "zero"`, `invalid session number "0"`} {
		if !strings.Contains(res.ParseErrors[i].Message, want) {
			t.Fatalf("error %d = %q, want %q", i, res.ParseErrors[i].Message, want)
		}
	}
	if len(res.Sessions) != 4 {
		t.Fatalf("sessions = %+v", res.Sessions)
	}
	wantTimes := []string{"09:00", "14:30", "", ""}
	wantDays := []int{10, 11, 10, 10}
	for i, s := range res.Sessions {
		if s.SessionNumber != i+1 || s.StartTime != wantTimes[i] {
			t.Fatalf("session %d = %+v", i, s)
		}
		if s.SessionDate == nil || s.SessionDate.Day() != wantDays[i] {
			t.Fatalf("session %d date = %v", i, s.SessionDate)
		}
	}
}

func TestCov1xParseEV3NoEventsFallsBackToDailySessions(t *testing.T) {
	res, err := meetfiles.ParseEV3("m.ev3", []byte("Meet;Loc;01/10/2026;01/11/2026;;S"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 0 || len(res.Sessions) != 2 || res.Sessions[0].Section != "Day 1" {
		t.Fatalf("res = %+v", res)
	}
	if res.Course != meetfiles.CourseShortCourseMeters {
		t.Fatalf("course = %q", res.Course)
	}
}

func TestCov1xParseEV3NoStartDateSessionsHaveNoDate(t *testing.T) {
	data := "Meet;Loc;;;;Y\n" + cov1xEV3Row("1", "I", "F", "", "", "50", "A", "1", "1", "")
	res, err := meetfiles.ParseEV3("m.ev3", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sessions) != 1 || res.Sessions[0].SessionDate != nil {
		t.Fatalf("sessions = %+v", res.Sessions)
	}
}
