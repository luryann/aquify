package performance

import (
	"strings"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

const stdHeader = "standard_name,standard_code,season_year,course,gender,age_group,event_distance,stroke,cut_time\n"

func TestParseSwimTime(t *testing.T) {
	for in, want := range map[string]int{"58.12": 58120, "1:02.45": 62450, "1:02:45": 62450, "5.5": 5500, "10:00.00": 600000} {
		got, err := ParseSwimTime(in)
		if err != nil || got != want {
			t.Errorf("%q = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "0", "1:75.00", "61:00.00", "1.2345", "-5", "1e3", "0x1p3", "Inf", "NaN", "5000", "3600"} {
		if _, err := ParseSwimTime(in); err == nil {
			t.Errorf("%q should fail", in)
		}
	}
}

func TestParseAgeGroup(t *testing.T) {
	cases := map[string][2]int{"11-12": {11, 12}, "10&U": {-1, 10}, "10 & under": {-1, 10}, "15&O": {15, -1}, "15+": {15, -1}, "13": {13, 13}, "OPEN": {-1, -1}, "": {-1, -1}}
	for in, want := range cases {
		lo, hi, err := parseAgeGroup(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		g := [2]int{-1, -1}
		if lo != nil {
			g[0] = *lo
		}
		if hi != nil {
			g[1] = *hi
		}
		if g != want {
			t.Errorf("%q = %v want %v", in, g, want)
		}
	}
	if _, _, err := parseAgeGroup("12-11"); err == nil {
		t.Error("reversed range should fail")
	}
}

func TestParseTimeStandardsCSV(t *testing.T) {
	csv := "\ufeff" + stdHeader +
		"2026 Senior,AA,2026,scy,Girls,11-12,100,Freestyle,58.12\n" +
		"2026 Senior,AA,2026,SCY,F,11-12,100,free,57.00\n" + // duplicate
		"2026 Senior,AA,2026,XXX,F,11-12,100,free,57.00\n" + // bad course
		"\n" +
		"2026 Senior,AA,2025-2026,LCM,M,OPEN,200,im,2:10.5\n"
	rows, err := ParseTimeStandardsCSV([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d", len(rows))
	}
	if len(rows[0].Errors) != 0 || rows[0].Gender != "F" || rows[0].Stroke != "free" || rows[0].TimeMs != 58120 || *rows[0].AgeMin != 11 {
		t.Errorf("row0 = %+v", rows[0])
	}
	if len(rows[1].Errors) != 1 || !strings.Contains(rows[1].Errors[0], "duplicate") {
		t.Errorf("row1 errors = %v", rows[1].Errors)
	}
	if len(rows[2].Errors) != 1 || !strings.Contains(rows[2].Errors[0], "course") {
		t.Errorf("row2 errors = %v", rows[2].Errors)
	}
	if len(rows[3].Errors) != 0 || rows[3].SeasonYear != 2026 || rows[3].TimeMs != 130500 || rows[3].AgeMin != nil {
		t.Errorf("row3 = %+v", rows[3])
	}
}

func TestParseTimeStandardsCSVStructuralErrors(t *testing.T) {
	for name, in := range map[string]string{
		"empty":       "",
		"no rows":     stdHeader,
		"missing col": "standard_name,course\nx,SCY\n",
	} {
		if _, err := ParseTimeStandardsCSV([]byte(in)); err == nil {
			t.Errorf("%s should fail", name)
		}
	}
	// "event" column alternative
	rows, err := ParseTimeStandardsCSV([]byte("name,season,course,gender,event,time\nS,2026,SCY,M,50 Fly,25.0\n"))
	if err != nil || len(rows[0].Errors) != 0 || rows[0].EventDistance != 50 || rows[0].Stroke != "fly" {
		t.Fatalf("event col: %v %+v", err, rows)
	}
}

func TestStandardsImportPlanAndCommit(t *testing.T) {
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "a", Roles: []string{"admin"}}
	coach := &appmiddleware.Principal{UserID: "c", Roles: []string{"coach"}}
	parse := func(body string) []StandardsImportRow {
		rows, err := ParseTimeStandardsCSV([]byte(stdHeader + body))
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	rows := parse("S,A,2026,SCY,F,11-12,100,free,58.12\nS,A,2026,SCY,F,11-12,50,free,27.00\n")

	if _, err := svc.PlanTimeStandardsImport("o", coach, rows); err != ErrForbidden {
		t.Fatalf("coach plan = %v", err)
	}
	if _, err := svc.CommitTimeStandardsImport("o", "c", coach, rows); err != ErrForbidden {
		t.Fatalf("coach commit = %v", err)
	}
	planned, _ := svc.PlanTimeStandardsImport("o", admin, rows)
	if c := CountStandardsImport(planned); c.Create != 2 {
		t.Fatalf("plan counts = %+v", c)
	}
	if l, _ := svc.ListTimeStandards("o", ListTimeStandardsFilters{}); len(l) != 0 {
		t.Fatal("preview must not write")
	}
	if _, err := svc.CommitTimeStandardsImport("o", "a", admin, rows); err != nil {
		t.Fatal(err)
	}
	// Re-import: one changed, one unchanged, one new.
	rows = parse("S2,A,2026,SCY,F,11-12,100,free,57.00\nS,A,2026,SCY,F,11-12,50,free,27.00\nS,A,2026,SCY,F,11-12,200,free,2:10.00\n")
	// first row has different name => update (same identity: code+event)
	planned, _ = svc.CommitTimeStandardsImport("o", "a", admin, rows)
	if c := CountStandardsImport(planned); c.Update != 1 || c.Unchanged != 1 || c.Create != 1 {
		t.Fatalf("recommit counts = %+v", c)
	}
	if l, _ := svc.ListTimeStandards("o", ListTimeStandardsFilters{}); len(l) != 3 {
		t.Fatalf("standards = %d", len(l))
	}
	// Other org untouched; an invalid row aborts everything.
	bad := parse("S,A,2026,SCY,F,11-12,400,free,4:00.00\nS,A,2026,BAD,F,11-12,500,free,5:00.00\n")
	if _, err := svc.CommitTimeStandardsImport("o", "a", admin, bad); err != ErrImportHasErrors {
		t.Fatalf("bad commit = %v", err)
	}
	if l, _ := svc.ListTimeStandards("o", ListTimeStandardsFilters{}); len(l) != 3 {
		t.Fatal("invalid import must write nothing")
	}
	if l, _ := svc.ListTimeStandards("other", ListTimeStandardsFilters{}); len(l) != 0 {
		t.Fatal("org leak")
	}
}
