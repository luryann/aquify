package entries

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/meetfiles"
)

func intPtr(v int) *int { return &v }

// sampleInput is a small fictional meet exercising the layout branches the
// reference bundle does not: a metres seed, an age-ranged girls event, a
// lettered event number, a middle initial, a preferred name, a missing member
// ID and a no-time entry.
func sampleInput() Input {
	loc := time.FixedZone("EST", -5*3600)
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	ana := Swimmer{ID: "s1", LastName: "Okafor", FirstName: "Anastasia", MiddleInitial: "R", PreferredName: "Ana", USSID: "0A1B2C3D4E5F6A", DOB: day(2014, 12, 1), Gender: "F"}
	ben := Swimmer{ID: "s2", LastName: "de la Cruz", FirstName: "Benjamin", USSID: "", DOB: day(2010, 3, 9), Gender: "M"}
	free50 := Event{Number: "3", Distance: 50, Stroke: meetfiles.StrokeFreestyle, Gender: "X", FeeCents: 600}
	im200 := Event{Number: "12a", Distance: 200, Stroke: meetfiles.StrokeIM, Gender: "F", MinAge: intPtr(11), MaxAge: intPtr(12), FeeCents: 750}
	fly100 := Event{Number: "20", Distance: 100, Stroke: meetfiles.StrokeButterfly, Gender: "M", MinAge: intPtr(13), FeeCents: 600}
	return Input{
		Club: Club{Name: "Harbor Tide Swim Club", Code: "htsc", LSC: "ne", Address1: "1 Pier Road", City: "Portsmouth", State: "NH", PostalCode: "03801"},
		Meet: Meet{Name: "Winter Lights Invitational: 2026", Facility: "Granite Aquatic Center", Start: day(2026, 12, 5), End: day(2026, 12, 6), Course: meetfiles.CourseShortCourseYards, AthleteFeeCents: 1000},
		Entries: []Entry{
			{Swimmer: ben, Event: fly100, SeedTimeMs: 61230, SeedCourse: meetfiles.CourseLongCourseMeters, SeedMeet: "Summer Long Course Championship Finals", SeedDate: day(2026, 7, 19)},
			{Swimmer: ana, Event: im200, SeedTimeMs: 165459, SeedMeet: "Fall Classic", SeedDate: day(2026, 10, 3)},
			{Swimmer: ben, Event: free50},
			{Swimmer: ana, Event: free50, SeedTimeMs: 31070},
		},
		GeneratedAt: time.Date(2026, 11, 22, 9, 5, 7, 0, loc),
		GeneratedBy: "Coach Example",
	}
}

func TestSD3Layout(t *testing.T) {
	out, err := SD3(sampleInput())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"A01V3      01Meet Entries                  aquify              1.0       Coach Example                   11222026",
		"B11        Winter Lights Invitational: 20Granite Aquatic Center                                                          1205202612062026   0        Y",
		"C11        NEHTSCHarbor Tide Swim Club                         1 Pier Road                                 Portsmouth          NH03801",
		// Meet age is as of the first day: 12 (birthday 12/1) and 16.
		"D01        Okafor, Anastasia R         0A1B2C3D4E5FA   1201201412FX  501   3UNOV           31.07Y",
		"D30A1B2C3D4E5F6AAna              FFFFFFFFF",
		"D01        Okafor, Anastasia R         0A1B2C3D4E5FA   1201201412FF 2005 12A1112         2:45.45Y",
		"D01        de la Cruz, Benjamin                    A   0309201016MX  501   3UNOV              NTY",
		"D3              Benjamin         FFFFFFFFF",
		"D01        de la Cruz, Benjamin                    A   0309201016MM 1004  2013OV         1:01.23L",
		"Z01        01                                1  1   1   1     6     3    0     0     0    1  0  0  0  0",
	}
	lines := strings.Split(string(out), "\r\n")
	if lines[len(lines)-1] != "" || len(lines)-1 != len(want) {
		t.Fatalf("got %d lines, want %d, CRLF-terminated:\n%s", len(lines)-1, len(want), out)
	}
	for i, w := range want {
		if len(lines[i]) != sd3RecordWidth {
			t.Errorf("line %d is %d columns, want %d", i+1, len(lines[i]), sd3RecordWidth)
		}
		if got := strings.TrimRight(lines[i], " "); got != w {
			t.Errorf("line %d:\n got %q\nwant %q", i+1, got, w)
		}
	}
}

func TestHY3Layout(t *testing.T) {
	out, err := HY3(sampleInput())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"A102Meet Entries             Hy-Tek, Ltd    aquify 1.0    11222026  9:05 AMHarbor Tide Swim Club",
		"B1Winter Lights Invitational: 2026             Granite Aquatic Center                       120520261206202612052026   0",
		"B2                                                                                                Y   0.00",
		"C1HTSC Harbor Tide Swim Club         Harbor Tide SwimNE",
		"C21 Pier Road                                                 Portsmouth                    NH03801     USA",
		"C3",
		// Athletes are numbered as they first appear walking events 3, 12A, 20.
		"D1M    1de la Cruz          Benjamin                                                    03092010 16     0                   N",
		"E1M    1de laMX    50A  0109  0S  6.00  3     0.00Y    0.00Y    0.00    0.00   NN               N",
		"E1M    1de laMM   100D 13109  0S  6.00 20    61.23L   61.23L    0.00    0.00   NN               N",
		"D1F    2Okafor              Anastasia           Ana                 R0A1B2C3D4E5F6A     12012014 12     0                   N",
		"E1F    2OkafoFX    50A  0109  0S  6.00  3    31.07Y   31.07Y    0.00    0.00   NN               N",
		"E1F    2OkafoFF   200E 11 12  0S  7.50 12A  165.45Y  165.45Y    0.00    0.00   NN               N",
	}
	lines := strings.Split(string(out), "\r\n")
	if lines[len(lines)-1] != "" || len(lines)-1 != len(want) {
		t.Fatalf("got %d lines, want %d, CRLF-terminated:\n%s", len(lines)-1, len(want), out)
	}
	for i, w := range want {
		if len(lines[i]) != hy3DataWidth+2 {
			t.Fatalf("line %d is %d columns, want %d", i+1, len(lines[i]), hy3DataWidth+2)
		}
		data, sum := lines[i][:hy3DataWidth], lines[i][hy3DataWidth:]
		if got := strings.TrimRight(data, " "); got != w {
			t.Errorf("line %d:\n got %q\nwant %q", i+1, got, w)
		}
		if sum != HY3Checksum(data) {
			t.Errorf("line %d checksum %q, want %q", i+1, sum, HY3Checksum(data))
		}
	}
}

func TestHY3ChecksumKnownLines(t *testing.T) {
	// Lines from a real Hy-Tek-format entry file, with their checksums.
	for line, want := range map[string]string{
		"C3": "10",
		"B2                                                                                                Y   0.00": "80",
	} {
		if got := HY3Checksum(padRight(line, hy3DataWidth)); got != want {
			t.Errorf("checksum(%q) = %s, want %s", line, got, want)
		}
	}
}

func TestBundleNamesLogAndZip(t *testing.T) {
	bundle, err := Build(sampleInput())
	if err != nil {
		t.Fatal(err)
	}
	// The colon cannot appear in a file name.
	const base = "HTSC-Entries-Winter Lights Invitational 2026-22Nov2026-0905"
	names := []string{base + ".sd3", base + ".hy3", base + "-EntryReport-ByEvent.pdf", base + "-EntryReport-BySwimmer.pdf", base + "-EntryLog.txt"}
	if bundle.BaseName != base || len(bundle.Files) != len(names) {
		t.Fatalf("base %q with %d files", bundle.BaseName, len(bundle.Files))
	}
	wantLog := strings.Join([]string{
		"ENTRY FILE LOG",
		"========================================",
		"Date:             November 22, 2026",
		"Time:             9:05:07 AM",
		"Generated By:     Coach Example",
		"",
		"CLUB",
		"----------------------------------------",
		"Club Name:        Harbor Tide Swim Club",
		"Club Code:        HTSC",
		"LSC Code:         NE",
		"Code (LSC-Club):  NE-HTSC",
		"Address:          1 Pier Road, Portsmouth, NH, 03801",
		"",
		"MEET",
		"----------------------------------------",
		"Meet Name:        Winter Lights Invitational: 2026",
		"Start Date:       December 5, 2026",
		"End Date:         December 6, 2026",
		"",
		"ENTRIES",
		"----------------------------------------",
		"Athletes:         2",
		"Individual Entries: 4",
		"Relay Teams:      0",
		"",
		"FILES",
		"----------------------------------------",
	}, "\n") + "\n" + strings.Join(names, "\n")
	if got := string(bundle.File("-EntryLog.txt").Content); got != wantLog {
		t.Fatalf("log:\n%s\nwant:\n%s", got, wantLog)
	}

	archive, err := bundle.Zip()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range zr.File {
		if f.Name != names[i] {
			t.Errorf("zip entry %d = %q, want %q", i, f.Name, names[i])
		}
	}
	if len(zr.File) != len(names) {
		t.Fatalf("zip has %d files", len(zr.File))
	}
}

func textOps(t *testing.T, pdf []byte) map[string]pdfOp {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.3\n")) || !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Fatal("not a complete PDF")
	}
	out := map[string]pdfOp{}
	for _, op := range pdfOps(pdf) {
		if op.kind == "text" {
			out[op.text] = op
		}
	}
	return out
}

func TestReportsContent(t *testing.T) {
	bundle, err := Build(sampleInput())
	if err != nil {
		t.Fatal(err)
	}
	byEvent := textOps(t, bundle.File("-ByEvent.pdf").Content)
	for _, want := range []string{
		"Winter Lights Invitational: 2026",
		"12/05/2026 \x96 12/06/2026  |  Harbor Tide Swim Club \\(NE-HTSC\\)",
		"Generated: Nov 22, 2026",
		"#3 Mixed Open 50 Free", "#12A Female 11-12 200 IM", "#20 Male 13 & over 100 Fly",
		"Ana Okafor", "Benjamin de la Cruz",
		"31.07Y", "2:45.45Y", "1:01.23L", "NT",
		"Manually added", "Fall Classic - 10/03/2026",
		// Seed meets are cut to 30 characters.
		"Summer Long Course Championshi - 07/19/2026",
		"Meet Summary", "Individual Entries", "Individual Athletes", "Relay Entries",
		// 2x$6.00 + $7.50 + $6.00 in entries, 2 athletes at $10.00.
		"$25.50", "$20.00", "$45.50",
		"Generated: Nov 22nd, 2026 9:05 AM", "By: Coach Example", "Page 1 of 2", "Page 2 of 2",
	} {
		if _, ok := byEvent[want]; !ok {
			t.Errorf("by-event report is missing %q", want)
		}
	}
	// Within an event, entries keep the order they were supplied in.
	if ben, ana := byEvent["NT"], byEvent["31.07Y"]; !(ben.y > ana.y) {
		t.Errorf("event 3 order: Benjamin at y=%v should be above Ana at y=%v", ben.y, ana.y)
	}

	bySwimmer := textOps(t, bundle.File("-BySwimmer.pdf").Content)
	// Sorted case-insensitively by "Last, First": "de la Cruz" before "Okafor".
	if ben, ana := bySwimmer["Benjamin de la Cruz"], bySwimmer["Ana Okafor"]; ben.font != "F2" || !(ben.y > ana.y) {
		t.Errorf("by-swimmer order/heading wrong: %v then %v", ben, ana)
	}
	if _, ok := bySwimmer["#12A Female 11-12 200 IM"]; !ok {
		t.Error("by-swimmer report is missing the event row")
	}
}

func TestReportFlowsAcrossColumnsAndPages(t *testing.T) {
	in := sampleInput()
	in.Entries = nil
	ev := Event{Number: "1", Distance: 50, Stroke: meetfiles.StrokeFreestyle, Gender: "X"}
	for i := 0; i < 60; i++ {
		id := string(rune('A'+i/26)) + string(rune('a'+i%26))
		in.Entries = append(in.Entries, Entry{Swimmer: Swimmer{ID: id, LastName: "Swimmer" + id, FirstName: "Test", DOB: time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC), Gender: "F"}, Event: ev})
	}
	bundle, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	pages, columns := map[int]bool{}, map[float64]bool{}
	for _, op := range pdfOps(bundle.File("-ByEvent.pdf").Content) {
		if op.kind == "text" && strings.HasPrefix(op.text, "Test Swimmer") {
			pages[op.page], columns[op.x] = true, true
			if op.y < 792-flowBottom {
				t.Fatalf("entry drawn below the flow area at y=%v", op.y)
			}
		}
	}
	// 23 rows fit in each first-page column and 60 need a third column.
	if len(pages) != 2 || len(columns) != 2 {
		t.Fatalf("60 entries used %d pages and %d columns", len(pages), len(columns))
	}
}

func TestBuildValidation(t *testing.T) {
	cases := map[string]func(*Input){
		"club code":      func(in *Input) { in.Club.Code = " " },
		"meet name":      func(in *Input) { in.Meet.Name = "" },
		"start date":     func(in *Input) { in.Meet.Start = time.Time{} },
		"generated-at":   func(in *Input) { in.GeneratedAt = time.Time{} },
		"at least one":   func(in *Input) { in.Entries = nil },
		"swimmer id":     func(in *Input) { in.Entries[0].Swimmer.ID = "" },
		"swimmer name":   func(in *Input) { in.Entries[0].Swimmer.LastName = "" },
		"gender must":    func(in *Input) { in.Entries[0].Swimmer.Gender = "X" },
		"date of birth":  func(in *Input) { in.Entries[0].Swimmer.DOB = time.Time{} },
		"event number":   func(in *Input) { in.Entries[0].Event.Number = "A1" },
		"distance":       func(in *Input) { in.Entries[0].Event.Distance = 0 },
		"individual":     func(in *Input) { in.Entries[0].Event.Stroke = meetfiles.StrokeMedleyRelay },
		"event gender":   func(in *Input) { in.Entries[0].Event.Gender = "O" },
		"negative":       func(in *Input) { in.Entries[0].SeedTimeMs = -1 },
		"more than once": func(in *Input) { in.Entries = append(in.Entries, in.Entries[0]) },
	}
	for want, mutate := range cases {
		in := sampleInput()
		mutate(&in)
		_, err := Build(in)
		if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", want, err)
		}
	}
}

func TestFixedWidthFieldsStayASCII(t *testing.T) {
	in := sampleInput()
	in.Entries[0].Swimmer.LastName = "Muñoz-Ångström"
	in.Entries[0].Swimmer.FirstName = "Zoë 李"
	in.Entries[2].Swimmer = in.Entries[0].Swimmer
	for name, render := range map[string]func(Input) ([]byte, error){"sd3": SD3, "hy3": HY3} {
		out, err := render(in)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range out {
			if b > 0x7e {
				t.Fatalf("%s contains a non-ASCII byte", name)
			}
		}
		if !bytes.Contains(out, []byte("Munoz-Angstrom")) || !bytes.Contains(out, []byte("Zoe ?")) {
			t.Fatalf("%s did not fold accented names:\n%s", name, out)
		}
	}
}
