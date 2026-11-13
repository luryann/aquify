package meets

import (
	"errors"
	"strings"
	"testing"

	"github.com/luryann/aquify/meetfiles"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func eventFiles(t *testing.T, names ...string) ParseMeetFileInput {
	t.Helper()
	var in ParseMeetFileInput
	for _, name := range names {
		in.Files = append(in.Files, meetfiles.SourceFile{Name: name, Content: cov1xFixture(t, name)})
	}
	return in
}

// A host publishes its meet as a .hyv and a .ev3; uploading both must give
// the same draft as uploading the .ev3 alone, with no disagreement warnings.
func TestParseMeetFile_HostEventFiles(t *testing.T) {
	svc := NewService()
	for name, input := range map[string]ParseMeetFileInput{
		"both": eventFiles(t, "bca-fall-festival.hyv", "bca-fall-festival.ev3"),
		"ev3":  eventFiles(t, "bca-fall-festival.ev3"),
		"zip": {Files: []meetfiles.SourceFile{{Name: "Meet Events.zip", Content: cov1xZip(t,
			[2]string{"Meet Events/a.hyv", string(cov1xFixture(t, "bca-fall-festival.hyv"))},
			[2]string{"Meet Events/a.ev3", string(cov1xFixture(t, "bca-fall-festival.ev3"))},
		)}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := svc.ParseMeetFile(cov1xAdmin, "org-1", input)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.ParseWarnings) != 0 {
				t.Fatalf("warnings: %v", got.ParseWarnings)
			}
			if got.FileType != "ev3" || got.MeetName != "2026 BCA Fall Festival" || got.Location != "George Mannon Aquatic Center" || got.Course != "SCY" {
				t.Fatalf("header: %+v", got)
			}
			if got.StartDate != "2026-11-14" || got.EndDate != "2026-11-15" || got.AgeUpDate != "2026-11-14" {
				t.Fatalf("dates: %s to %s, age-up %s", got.StartDate, got.EndDate, got.AgeUpDate)
			}
			if len(got.Sessions) != 6 || got.Sessions[2].SessionDate != "2026-11-14" || got.Sessions[2].StartTime != "14:30" || got.Sessions[3].SessionDate != "2026-11-15" {
				t.Fatalf("sessions: %+v", got.Sessions)
			}
			if len(got.Events) != 79 {
				t.Fatalf("events = %d, want 79", len(got.Events))
			}
			first := got.Events[0]
			if first.EventNumber != "1" || first.Stroke != "freestyle" || first.Distance != 200 || first.Gender != "X" ||
				*first.MinAge != 7 || *first.MaxAge != 10 || first.SessionNumber != 1 || first.EntryFeeCents != 600 {
				t.Fatalf("event 1: %+v", first)
			}
			last := got.Events[78]
			if last.EventNumber != "76" || last.Distance != 500 || last.Gender != "M" || last.MaxAge != nil || last.SessionNumber != 6 {
				t.Fatalf("event 76: %+v", last)
			}
		})
	}
}

func TestParseMeetFile_HYVAloneDerivesDailySessions(t *testing.T) {
	got, err := NewService().ParseMeetFile(cov1xAdmin, "org-1", eventFiles(t, "bca-fall-festival.hyv"))
	if err != nil {
		t.Fatal(err)
	}
	if got.FileType != "hyv" || len(got.Events) != 79 || len(got.Sessions) != 2 || got.Events[0].EntryFeeCents != 600 {
		t.Fatalf("hyv draft: type=%s events=%d sessions=%d", got.FileType, len(got.Events), len(got.Sessions))
	}
}

func TestParseMeetFile_WarnsWhenFormatsDisagree(t *testing.T) {
	hyv := strings.Replace(string(cov1xFixture(t, "bca-fall-festival.hyv")), "George Mannon Aquatic Center", "Another Pool", 1)
	got, err := NewService().ParseMeetFile(cov1xAdmin, "org-1", ParseMeetFileInput{Files: []meetfiles.SourceFile{
		{Name: "a.hyv", Content: []byte(hyv)},
		{Name: "a.ev3", Content: cov1xFixture(t, "bca-fall-festival.ev3")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ParseWarnings) != 1 || !strings.Contains(got.ParseWarnings[0], "location differs") {
		t.Fatalf("warnings: %v", got.ParseWarnings)
	}
}

func TestParseMeetFile_Rejections(t *testing.T) {
	svc := NewService()
	ev3 := meetfiles.SourceFile{Name: "a.ev3", Content: cov1xFixture(t, "bca-fall-festival.ev3")}
	if _, err := svc.ParseMeetFile(&appmiddleware.Principal{UserID: "parent-1", Roles: []string{"parent"}}, "org-1", ParseMeetFileInput{Files: []meetfiles.SourceFile{ev3}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent: %v", err)
	}
	for name, input := range map[string]ParseMeetFileInput{
		"no files":     {},
		"blank name":   {Files: []meetfiles.SourceFile{{Name: " ", Content: ev3.Content}}},
		"empty file":   {Files: []meetfiles.SourceFile{{Name: "a.ev3"}}},
		"entry file":   {Files: []meetfiles.SourceFile{{Name: "entries.sd3", Content: []byte("A01V3")}}},
		"old cl2":      {Files: []meetfiles.SourceFile{{Name: "meet.cl2", Content: []byte("A01V3")}}},
		"header only":  {Files: []meetfiles.SourceFile{{Name: "a.hyv", Content: []byte("Meet;11/14/2026;11/15/2026;11/14/2026;Y;Pool\r\n")}}},
		"bad header":   {Files: []meetfiles.SourceFile{{Name: "a.ev3", Content: []byte("not an event file\r\n")}}},
		"corrupt zip":  {Files: []meetfiles.SourceFile{{Name: "a.zip", Content: []byte("PK\x03\x04 nope")}}},
		"zip no event": {Files: []meetfiles.SourceFile{{Name: "a.zip", Content: cov1xZip(t, [2]string{"readme.txt", "hi"})}}},
	} {
		if _, err := svc.ParseMeetFile(cov1xAdmin, "org-1", input); !errors.Is(err, ErrInvalidMeetFile) {
			t.Errorf("%s: err = %v, want ErrInvalidMeetFile", name, err)
		}
	}
	if _, err := svc.ParseMeetFile(cov1xAdmin, " ", ParseMeetFileInput{Files: []meetfiles.SourceFile{ev3}}); !errors.Is(err, ErrInvalidMeetFile) {
		t.Fatalf("blank org: %v", err)
	}
}

func TestParseResultsFile_SD3AndZip(t *testing.T) {
	sd3 := cov1xFixture(t, "valid.sd3")
	direct, _, err := ParseResultsFile("results.sd3", sd3)
	if err != nil || len(direct.IndividualResults) == 0 {
		t.Fatalf("sd3: err=%v results=%v", err, direct)
	}
	zipped, warnings, err := ParseResultsFile("results.zip", cov1xZip(t,
		[2]string{"b.sd3", string(sd3)}, [2]string{"a.sd3", string(sd3)}, [2]string{"heat-sheet.pdf", "%PDF"}))
	if err != nil || len(zipped.IndividualResults) != len(direct.IndividualResults) {
		t.Fatalf("zip: err=%v", err)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "imported a.sd3") {
		t.Fatalf("zip warnings: %v", warnings)
	}
}

func TestParseResultsFile_Rejections(t *testing.T) {
	sd3 := cov1xFixture(t, "valid.sd3")
	for name, tc := range map[string]struct {
		filename string
		data     []byte
	}{
		"blank name":     {" ", sd3},
		"empty":          {"results.sd3", nil},
		"event file":     {"meet.ev3", cov1xFixture(t, "bca-fall-festival.ev3")},
		"hy3 entry file": {"entries.hy3", []byte("A102Meet Entries")},
		"cl2":            {"results.cl2", []byte("not sdif")},
		"junk sd3":       {"results.sd3", []byte(cov1xJunk())},
		"corrupt zip":    {"results.zip", []byte("PK\x03\x04 nope")},
		"zip without":    {"results.zip", cov1xZip(t, [2]string{"a.ev3", "x"})},
	} {
		if _, _, err := ParseResultsFile(tc.filename, tc.data); !errors.Is(err, ErrInvalidMeetFile) {
			t.Errorf("%s: err = %v, want ErrInvalidMeetFile", name, err)
		}
	}
}
