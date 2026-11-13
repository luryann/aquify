package sdif

import (
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/meetfiles"
)

func TestCov1xFieldBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		start, length int
		want          string
	}{
		{"start past end", "ab", 5, 3, ""},
		{"length clipped to line", "abcdef", 4, 10, "def"},
		{"begin inside multibyte rune", "aéb", 3, 2, "b"},
		{"end inside multibyte rune", "aéb", 1, 2, "a"},
		{"trimmed", "  x  ", 1, 5, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := field(tt.line, tt.start, tt.length); got != tt.want {
				t.Fatalf("field = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCov1xDecoders(t *testing.T) {
	courses := map[string]meetfiles.Course{
		"1": meetfiles.CourseShortCourseMeters, "s": meetfiles.CourseShortCourseMeters,
		"2": meetfiles.CourseShortCourseYards, " Y ": meetfiles.CourseShortCourseYards,
		"3": meetfiles.CourseLongCourseMeters, "L": meetfiles.CourseLongCourseMeters,
		"": meetfiles.CourseUnknown, "9": meetfiles.CourseUnknown,
	}
	for in, want := range courses {
		if got := decodeCourse(in); got != want {
			t.Errorf("decodeCourse(%q) = %q, want %q", in, got, want)
		}
	}
	strokes := map[string]meetfiles.Stroke{
		"1": meetfiles.StrokeFreestyle, "2": meetfiles.StrokeBackstroke,
		"3": meetfiles.StrokeBreaststroke, "4": meetfiles.StrokeButterfly,
		"5": meetfiles.StrokeIM, "6": meetfiles.StrokeFreestyleRelay,
		"7": meetfiles.StrokeMedleyRelay, "8": meetfiles.StrokeUnknown, "": meetfiles.StrokeUnknown,
	}
	for in, want := range strokes {
		if got := decodeStroke(in); got != want {
			t.Errorf("decodeStroke(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov1xParseHelpersRejectBadInput(t *testing.T) {
	if parseDate("13322026") != nil {
		t.Error("impossible date should parse to nil")
	}
	if d := parseDate("02152026"); d == nil || !d.Equal(time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("valid date = %v", d)
	}
	if _, err := parseTimeString("1.2.3"); err == nil || !strings.Contains(err.Error(), "invalid seconds") {
		t.Errorf("1.2.3 err = %v", err)
	}
	if parseInt("abc") != 0 || parseInt("") != 0 || parseInt(" 42 ") != 42 {
		t.Error("parseInt mishandled input")
	}
}

func TestCov1xParseTooManyErrorsOnD0AndOrphanF0(t *testing.T) {
	for name, rec := range map[string]string{"short D0": "D0 short", "orphan F0": "F0 orphan"} {
		t.Run(name, func(t *testing.T) {
			data := strings.Repeat(rec+"\n", maxTotalParseErrors+1)
			res, err := Parse([]byte(data))
			if err == nil || !strings.Contains(err.Error(), "too many malformed records") || res != nil {
				t.Fatalf("res=%v err=%v", res, err)
			}
		})
	}
}

func TestCov1xSplitRecordLinesTrailingPartial(t *testing.T) {
	got := splitRecordLines([]byte("A0one\r\nB1two\rC1three"))
	if len(got) != 3 || got[0] != "A0one" || got[1] != "B1two" || got[2] != "C1three" {
		t.Fatalf("lines = %q", got)
	}
}
