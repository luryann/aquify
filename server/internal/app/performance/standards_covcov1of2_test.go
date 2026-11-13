package performance

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func cov1xParseOne(t *testing.T, header, row string) StandardsImportRow {
	t.Helper()
	rows, err := ParseTimeStandardsCSV([]byte(header + "\n" + row + "\n"))
	if err != nil {
		t.Fatalf("parse %q: %v", row, err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	return rows[0]
}

func TestCov1xParseStandardsRowErrors(t *testing.T) {
	const hdr = "standard_name,standard_code,season_year,course,gender,age_group,event_distance,stroke,cut_time"
	cases := []struct {
		name string
		row  string
		want string
	}{
		{"missing name", ",A,2026,SCY,F,11-12,100,free,58.12", "standard_name is required"},
		{"bad season", "S,A,someday,SCY,F,11-12,100,free,58.12", `season_year "someday" must be`},
		{"bad course", "S,A,2026,XYZ,F,11-12,100,free,58.12", `course "XYZ" must be SCY, SCM or LCM`},
		{"bad gender", "S,A,2026,SCY,Q,11-12,100,free,58.12", `gender "Q" must be M, F or X`},
		{"bad age group", "S,A,2026,SCY,F,eleven,100,free,58.12", `age_group "eleven" must look like`},
		{"reversed age group", "S,A,2026,SCY,F,14-11,100,free,58.12", "has min greater than max"},
		{"bad distance", "S,A,2026,SCY,F,11-12,abc,free,58.12", `event distance "abc" must be a positive whole number`},
		{"huge distance", "S,A,2026,SCY,F,11-12,5001,free,58.12", "must be a positive whole number"},
		{"bad stroke", "S,A,2026,SCY,F,11-12,100,sidestroke,58.12", `stroke "sidestroke" must be`},
		{"bad time", "S,A,2026,SCY,F,11-12,100,free,fast", "cut_time:"},
	}
	for _, tc := range cases {
		row := cov1xParseOne(t, hdr, tc.row)
		if len(row.Errors) != 1 || !strings.Contains(row.Errors[0], tc.want) {
			t.Errorf("%s: errors = %v, want one containing %q", tc.name, row.Errors, tc.want)
		}
	}
}

func TestCov1xParseStandardsAgeColumnsAndEventColumn(t *testing.T) {
	const hdr = "name,season,course,gender,age_min,age_max,event,time"
	row := cov1xParseOne(t, hdr, "S,2025-26,scm,boys,10,12,100 Backstroke,1:02.45")
	if len(row.Errors) != 0 {
		t.Fatalf("errors = %v", row.Errors)
	}
	if row.StandardCode != "S" || row.SeasonYear != 2026 || row.Course != "SCM" || row.Gender != "M" ||
		row.AgeMin == nil || *row.AgeMin != 10 || row.AgeMax == nil || *row.AgeMax != 12 ||
		row.EventDistance != 100 || row.Stroke != "back" || row.TimeMs != 62450 {
		t.Fatalf("row = %+v", row)
	}
	// min-only and max-only columns; blank cells are skipped.
	if r := cov1xParseOne(t, hdr, "S,2026,LCM,women,15,,50 fly,25.00"); r.AgeMin == nil || r.AgeMax != nil || len(r.Errors) != 0 {
		t.Fatalf("min only = %+v", r)
	}
	if r := cov1xParseOne(t, hdr, "S,2026,LCM,mixed,,9,200 IM,2:30.00"); r.AgeMin != nil || r.AgeMax == nil || r.Gender != "X" || r.Stroke != "im" || len(r.Errors) != 0 {
		t.Fatalf("max only = %+v", r)
	}
	for name, tc := range map[string]struct{ row, want string }{
		"min not a number": {"S,2026,SCY,F,x,12,100 free,58.12", `age_min "x" is not a valid age`},
		"max too large":    {"S,2026,SCY,F,10,121,100 free,58.12", `age_max "121" is not a valid age`},
		"min above max":    {"S,2026,SCY,F,13,12,100 free,58.12", "age_min (13) is greater than age_max (12)"},
		"event one word":   {"S,2026,SCY,F,10,12,freestyle,58.12", `event "freestyle" must look like "100 Free"`},
	} {
		got := cov1xParseOne(t, hdr, tc.row)
		if len(got.Errors) == 0 || !strings.Contains(strings.Join(got.Errors, ";"), tc.want) {
			t.Errorf("%s: errors = %v, want containing %q", name, got.Errors, tc.want)
		}
	}
}

func TestCov1xParseSeasonYearAndNormalizers(t *testing.T) {
	for raw, want := range map[string]int{"2026": 2026, "2025-26": 2026, "2025/2026": 2026, "2099-00": 2100, " 2025 - 2026 ": 2026} {
		got, err := parseSeasonYear(raw)
		if err != nil || got != want {
			t.Errorf("parseSeasonYear(%q) = %d, %v; want %d", raw, got, err, want)
		}
	}
	for raw, want := range map[string]string{"soon": "must be a 4-digit year", "1999": "out of range", "2201": "out of range", "1999-2000": "out of range"} {
		if _, err := parseSeasonYear(raw); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parseSeasonYear(%q) = %v, want containing %q", raw, err, want)
		}
	}
	for raw, want := range map[string]string{"m": "M", "Boys": "M", "women": "F", "GIRL": "F", "open": "X", "Coed": "X"} {
		if got, ok := normalizeStandardGender(raw); !ok || got != want {
			t.Errorf("normalizeStandardGender(%q) = %q %v", raw, got, ok)
		}
	}
	if _, ok := normalizeStandardGender("robot"); ok {
		t.Error("unknown gender must not normalize")
	}
	for raw, want := range map[string]string{"Free": "free", "bk": "back", "Backstroke": "back", "BR": "breast", "butterfly": "fly", "medley": "im", "Individual Medley": "im"} {
		if got, ok := normalizeStroke(raw); !ok || got != want {
			t.Errorf("normalizeStroke(%q) = %q %v", raw, got, ok)
		}
	}
	if _, ok := normalizeStroke("dog paddle"); ok {
		t.Error("unknown stroke must not normalize")
	}
	if _, _, err := parseAgeGroup("tall"); err == nil || !strings.Contains(err.Error(), `age_group "tall" must look like`) {
		t.Errorf("parseAgeGroup(tall) = %v", err)
	}
}

func TestCov1xParseTimeStandardsCSVStructureAndLimits(t *testing.T) {
	// blank rows are skipped; the rest are numbered by file line
	rows, err := ParseTimeStandardsCSV([]byte(stdHeader + ",,,,,,,,\n\nS,A,2026,SCY,F,11-12,100,free,58.12\n"))
	if err != nil || len(rows) != 1 || rows[0].Line != 4 {
		t.Fatalf("blank rows: %+v %v", rows, err)
	}
	if rows[0].Errors == nil || len(rows[0].Errors) != 0 {
		t.Fatalf("valid rows carry an empty (non-nil) error list for JSON: %#v", rows[0].Errors)
	}

	var b strings.Builder
	b.WriteString(stdHeader)
	for i := 0; i <= MaxStandardsImportRows; i++ {
		fmt.Fprintf(&b, "S%d,A,2026,SCY,F,11-12,100,free,58.12\n", i)
	}
	if _, err := ParseTimeStandardsCSV([]byte(b.String())); err == nil || !strings.Contains(err.Error(), "more than 5000 rows") {
		t.Fatalf("row cap = %v", err)
	}
	if _, err := ParseTimeStandardsCSV(nil); err == nil || err.Error() != "file is empty" {
		t.Fatalf("empty = %v", err)
	}
}

func TestCov1xStandardsImportPlanCommitEdgeCases(t *testing.T) {
	s := NewService()
	parse := func(body string) []StandardsImportRow {
		t.Helper()
		rows, err := ParseTimeStandardsCSV([]byte(stdHeader + body))
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	rows := parse("S,A,2026,SCY,F,11-12,100,free,58.12\n")
	if _, err := s.CommitTimeStandardsImport("org", "admin", adminP, rows); err != nil {
		t.Fatal(err)
	}
	// An import that carries a description updates it; one without leaves it alone.
	withDesc, err := ParseTimeStandardsCSV([]byte("standard_name,standard_code,season_year,course,gender,age_group,event_distance,stroke,cut_time,description\nS,A,2026,SCY,F,11-12,100,free,57.00,Regional cut\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitTimeStandardsImport("org", "admin", adminP, withDesc); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListTimeStandards("org", ListTimeStandardsFilters{})
	if len(list) != 1 || list[0].TimeMs != 57000 || list[0].Description != "Regional cut" {
		t.Fatalf("after description import = %+v", list)
	}
	again := parse("S,A,2026,SCY,F,11-12,100,free,56.00\n")
	if _, err := s.CommitTimeStandardsImport("org", "admin", adminP, again); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListTimeStandards("org", ListTimeStandardsFilters{})
	if list[0].TimeMs != 56000 || list[0].Description != "Regional cut" {
		t.Fatalf("an import without a description keeps the existing one: %+v", list[0])
	}

	// Other clubs' and deleted standards never match; a deleted standard is recreated.
	planned, err := s.PlanTimeStandardsImport("other", adminP, rows)
	if err != nil || planned[0].Action != ImportActionCreate {
		t.Fatalf("other club plan = %+v %v", planned, err)
	}
	if err := s.DeleteTimeStandard("org", "admin", list[0].ID, adminP); err != nil {
		t.Fatal(err)
	}
	if planned, _ = s.PlanTimeStandardsImport("org", adminP, rows); planned[0].Action != ImportActionCreate {
		t.Fatalf("deleted standard must plan as create: %+v", planned)
	}
	if _, err := s.PlanTimeStandardsImport("org", cov1xGuest, rows); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guest plan = %v", err)
	}

	// Failures: load error and persistence error (nothing is cached).
	un := cov1xUnloaded(t)
	if _, err := un.PlanTimeStandardsImport("org", adminP, rows); err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("plan load failure = %v", err)
	}
	if _, err := un.CommitTimeStandardsImport("org", "a", adminP, rows); err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("commit load failure = %v", err)
	}
	cov1xBreakPersistence(t, s, "org")
	if _, err := s.CommitTimeStandardsImport("org", "admin", adminP, rows); err == nil || !strings.Contains(err.Error(), "persist time standards import") {
		t.Fatalf("commit persist failure = %v", err)
	}
	if got, _ := s.ListTimeStandards("org", ListTimeStandardsFilters{}); len(got) != 0 {
		t.Fatalf("failed import must not change the cache: %+v", got)
	}
}
