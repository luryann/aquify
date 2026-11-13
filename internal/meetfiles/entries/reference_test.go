package entries

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/meetfiles"
)

// The reference bundle holds real swimmers' details, so it lives outside the
// module (repo root, meetfilesClaude/) and these tests skip when it is absent.
// They rebuild the bundle from the data in the reference files and require
// the SD3, HY3 and log to match byte for byte and the PDF reports to place
// every piece of text and every rule at the same coordinates.

func referenceDir(t *testing.T) string {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join("..", "..", "..", "meetfilesClaude", "DARE-Entries-*OUTPUT"))
	if len(matches) == 0 {
		t.Skip("reference entry bundle not present")
	}
	return matches[0]
}

func readReference(t *testing.T, dir, suffix string) []byte {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(dir, "*"+suffix))
	if len(matches) != 1 {
		t.Fatalf("want one reference file ending %q, got %d", suffix, len(matches))
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type pdfOp struct {
	page int
	kind string // "text", "line", "rect"
	font string
	size float64
	x, y float64
	w, h float64
	text string
}

func (o pdfOp) String() string {
	return fmt.Sprintf("p%d %s %s %.1f (%.3f,%.3f) %.1fx%.1f %q", o.page, o.kind, o.font, o.size, o.x, o.y, o.w, o.h, o.text)
}

var (
	reStream = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	reTf     = regexp.MustCompile(`^/(F\d+) ([\d.]+) Tf$`)
	reTd     = regexp.MustCompile(`^([\d.]+) ([\d.]+) Td$`)
	reTj     = regexp.MustCompile(`^\((.*)\) Tj$`)
	reRe     = regexp.MustCompile(`^([\d.]+) ([\d.]+) ([\d.]+) (-?[\d.]+) re$`)
	reMove   = regexp.MustCompile(`^([\d.]+) ([\d.]+) m$`)
	reLineTo = regexp.MustCompile(`^([\d.]+) ([\d.]+) l$`)
)

func f(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }

// pdfOps extracts drawing operations from an uncompressed PDF.
func pdfOps(data []byte) []pdfOp {
	var ops []pdfOp
	page := 0
	for _, m := range reStream.FindAllSubmatch(data, -1) {
		body := string(m[1])
		if !strings.Contains(body, "BT\n") {
			continue
		}
		page++
		var font string
		var size, x, y, mx, my float64
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimRight(line, "\r")
			if g := reTf.FindStringSubmatch(line); g != nil {
				font, size = g[1], f(g[2])
			} else if g := reTd.FindStringSubmatch(line); g != nil {
				x, y = f(g[1]), f(g[2])
			} else if g := reTj.FindStringSubmatch(line); g != nil {
				ops = append(ops, pdfOp{page: page, kind: "text", font: font, size: size, x: x, y: y, text: g[1]})
			} else if g := reRe.FindStringSubmatch(line); g != nil {
				ops = append(ops, pdfOp{page: page, kind: "rect", x: f(g[1]), y: f(g[2]), w: f(g[3]), h: f(g[4])})
			} else if g := reMove.FindStringSubmatch(line); g != nil {
				mx, my = f(g[1]), f(g[2])
			} else if g := reLineTo.FindStringSubmatch(line); g != nil {
				ops = append(ops, pdfOp{page: page, kind: "line", x: mx, y: my, w: f(g[1]) - mx, h: f(g[2]) - my})
			}
		}
	}
	return ops
}

// referenceInput rebuilds the export input from the reference HY3 (athletes),
// and the by-event report (entry order within each event and seed sources).
func referenceInput(t *testing.T, dir string) Input {
	t.Helper()
	hy3 := strings.Split(string(readReference(t, dir, ".hy3")), "\r\n")
	loc := time.FixedZone("PST", -8*3600)
	in := Input{
		GeneratedAt: time.Date(2026, 11, 9, 15, 38, 41, 0, loc),
		GeneratedBy: "Ryan Lu",
	}
	date := func(s string) time.Time {
		d, err := time.Parse("01022006", s)
		if err != nil {
			t.Fatalf("bad date %q", s)
		}
		return d
	}
	trim := strings.TrimSpace
	byDisplay := map[string]Swimmer{}
	type key struct{ swimmer, event string }
	events := map[string]Event{}
	seeds := map[key]int{}
	var current Swimmer
	for _, l := range hy3 {
		if len(l) < 130 {
			continue
		}
		switch l[:2] {
		case "B1":
			in.Meet = Meet{Name: trim(l[2:47]), Facility: trim(l[47:92]), Start: date(l[92:100]), End: date(l[100:108]), AgeUpDate: date(l[108:116]), Course: meetfiles.CourseShortCourseYards}
		case "C1":
			in.Club.Code, in.Club.Name, in.Club.LSC = trim(l[2:7]), trim(l[7:37]), trim(l[53:55])
		case "C2":
			in.Club.Address1, in.Club.City, in.Club.State, in.Club.PostalCode, in.Club.Country = trim(l[2:32]), trim(l[62:92]), trim(l[92:94]), trim(l[94:104]), trim(l[104:107])
		case "D1":
			current = Swimmer{ID: trim(l[3:8]), Gender: l[2:3], LastName: trim(l[8:28]), FirstName: trim(l[28:48]), PreferredName: trim(l[48:68]), MiddleInitial: trim(l[68:69]), USSID: trim(l[69:83]), DOB: date(l[88:96])}
			if _, dup := byDisplay[current.DisplayName()]; dup {
				t.Fatalf("duplicate display name in reference")
			}
			byDisplay[current.DisplayName()] = current
		case "E1":
			number := trim(l[38:42])
			ev := Event{Number: number, Gender: l[14:15], Distance: int(f(trim(l[15:21]))), Stroke: []meetfiles.Stroke{meetfiles.StrokeFreestyle, meetfiles.StrokeBackstroke, meetfiles.StrokeBreaststroke, meetfiles.StrokeButterfly, meetfiles.StrokeIM}[l[21]-'A']}
			if lo := int(f(trim(l[22:25]))); lo > 0 {
				ev.MinAge = &lo
			}
			if hi := int(f(trim(l[25:28]))); hi < 99 {
				ev.MaxAge = &hi
			}
			events[number] = ev
			seeds[key{current.ID, number}] = int(math.Round(f(trim(l[42:50])) * 1000))
		}
	}
	// 132 athletes at $35.00 make the reference summary's $4620.00.
	in.Meet.AthleteFeeCents = 3500

	var eventNumber string
	var pending *Entry
	flush := func() {
		if pending != nil {
			in.Entries = append(in.Entries, *pending)
			pending = nil
		}
	}
	reSource := regexp.MustCompile(`^(.*) - (\d\d/\d\d/\d{4})$`)
	for _, op := range pdfOps(readReference(t, dir, "-EntryReport-ByEvent.pdf")) {
		if op.kind != "text" || op.y < 60 {
			continue
		}
		switch {
		case op.font == "F2" && op.size == 10:
			flush()
			eventNumber = strings.TrimPrefix(strings.Fields(op.text)[0], "#")
		case op.font == "F1" && op.size == 9 && (op.x == 48 || op.x == 326) && eventNumber != "":
			flush()
			sw, ok := byDisplay[op.text]
			if !ok {
				t.Fatalf("report lists an athlete missing from the HY3")
			}
			pending = &Entry{Swimmer: sw, Event: events[eventNumber], SeedTimeMs: seeds[key{sw.ID, eventNumber}]}
		case op.font == "F3" && pending != nil:
			if g := reSource.FindStringSubmatch(op.text); g != nil {
				pending.SeedMeet = g[1]
				pending.SeedDate, _ = time.Parse("01/02/2006", g[2])
			}
		case op.text == "Meet Summary":
			flush()
			eventNumber = ""
		}
	}
	flush()
	if len(in.Entries) != len(seeds) {
		t.Fatalf("rebuilt %d entries, reference HY3 has %d", len(in.Entries), len(seeds))
	}
	return in
}

func firstDiff(t *testing.T, name string, got, want []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	gl, wl := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Fatalf("%s differs at line %d (got %d lines, want %d)\n got: %q\nwant: %q", name, i+1, len(gl), len(wl), g, w)
		}
	}
}

func TestReferenceBundleSD3HY3AndLog(t *testing.T) {
	dir := referenceDir(t)
	in := referenceInput(t, dir)

	// The reference was produced by other software; only its name differs.
	in.SoftwareName, in.SoftwareVersion = "Commit Swimming", "1.0"
	sd3, err := SD3(in)
	if err != nil {
		t.Fatal(err)
	}
	firstDiff(t, "sd3", sd3, readReference(t, dir, "1538.sd3"))

	in.SoftwareName = "CommitSwim"
	hy3, err := HY3(in)
	if err != nil {
		t.Fatal(err)
	}
	firstDiff(t, "hy3", hy3, readReference(t, dir, ".hy3"))

	bundle, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	wantBase := strings.TrimSuffix(filepath.Base(dir), "OUTPUT")
	if bundle.BaseName != wantBase {
		t.Fatalf("base name = %q, want %q", bundle.BaseName, wantBase)
	}
	firstDiff(t, "log", bundle.File("-EntryLog.txt").Content, readReference(t, dir, "-EntryLog.txt"))
	if bundle.Athletes != 132 || bundle.IndividualEntries != 421 {
		t.Fatalf("totals = %d athletes / %d entries", bundle.Athletes, bundle.IndividualEntries)
	}
}

func TestReferenceBundleReports(t *testing.T) {
	dir := referenceDir(t)
	bundle, err := Build(referenceInput(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-EntryReport-ByEvent.pdf", "-EntryReport-BySwimmer.pdf"} {
		want := pdfOps(readReference(t, dir, suffix))
		var got []pdfOp
		for _, op := range pdfOps(bundle.File(suffix).Content) {
			// aquify's own footer mark replaces the reference's logos.
			if op.kind == "text" && op.text == DefaultSoftwareName {
				continue
			}
			got = append(got, op)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: %d drawing operations, want %d", suffix, len(got), len(want))
		}
		// The reference draws each page's footer after its body, as we do, so
		// operations line up one to one within a page.
		index := func(ops []pdfOp) map[string]int {
			m := map[string]int{}
			for _, op := range ops {
				// The reference title is 0.7pt off-centre; ours is centred.
				if op.kind == "text" && op.size == 14 {
					op.x = 0
				}
				op.x, op.y = math.Round(op.x*100)/100, math.Round(op.y*100)/100
				m[op.String()]++
			}
			return m
		}
		gotIdx, wantIdx := index(got), index(want)
		for k, n := range wantIdx {
			if gotIdx[k] != n {
				t.Errorf("%s: missing or misplaced: %s (got %d, want %d)", suffix, k, gotIdx[k], n)
				if t.Failed() && len(k) > 0 {
					break
				}
			}
		}
		if t.Failed() {
			for k := range gotIdx {
				if wantIdx[k] == 0 {
					t.Logf("%s: unexpected: %s", suffix, k)
					break
				}
			}
		}
	}
}
