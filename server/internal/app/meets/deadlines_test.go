package meets

import (
	"errors"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestCreateMeetDeadlineFormats(t *testing.T) {
	t.Parallel()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	la := func(string) string { return "America/Los_Angeles" }
	cases := []struct{ in, want string }{
		{"", ""},
		{"2026-11-20", "2026-11-20"},
		// Zone-less browser values are club time (Pacific, UTC-8 in November).
		{"2026-11-20T17:00", "2026-11-21T01:00:00Z"},
		{"2026-11-20T17:00:30", "2026-11-21T01:00:30Z"},
		// Explicit offsets are honoured and stored in UTC.
		{"2026-11-20T17:00:00-08:00", "2026-11-21T01:00:00Z"},
		{"2026-11-21T01:00:00.123Z", "2026-11-21T01:00:00Z"},
		// A UTC-midnight instant round-trips through the DB as its date.
		{"2026-11-20T00:00:00Z", "2026-11-20"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			svc := NewService()
			svc.SetOrgTimezoneLookup(la)
			meet, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
				Name: "Deadline formats", EntryDeadline: tc.in, CommitmentDeadline: tc.in,
			})
			if err != nil {
				t.Fatalf("CreateMeet() error = %v", err)
			}
			if meet.EntryDeadline != tc.want || meet.CommitmentDeadline != tc.want {
				t.Fatalf("deadlines: entry=%q commitment=%q, want %q", meet.EntryDeadline, meet.CommitmentDeadline, tc.want)
			}
		})
	}
	t.Run("no club zone means UTC", func(t *testing.T) {
		meet, _, err := NewService().CreateMeet(admin, "org-a", CreateMeetInput{Name: "UTC", CommitmentDeadline: "2026-11-20T17:00"})
		if err != nil || meet.CommitmentDeadline != "2026-11-20T17:00:00Z" {
			t.Fatalf("got %q, %v", meet.CommitmentDeadline, err)
		}
	})
	t.Run("age-up date", func(t *testing.T) {
		meet, _, err := NewService().CreateMeet(admin, "org-a", CreateMeetInput{Name: "Age", StartDate: "2026-12-05", AgeUpDate: "2026-12-31"})
		if err != nil || meet.AgeUpDate != "2026-12-31" || meet.AgeReferenceDate() != "2026-12-31" {
			t.Fatalf("got %+v, %v", meet, err)
		}
		if _, _, err := NewService().CreateMeet(admin, "org-a", CreateMeetInput{Name: "Age", AgeUpDate: "soon"}); !errors.Is(err, ErrValidation) {
			t.Fatalf("bad age-up date error = %v, want ErrValidation", err)
		}
		if got := (Meet{StartDate: "2026-12-05"}).AgeReferenceDate(); got != "2026-12-05" {
			t.Fatalf("AgeReferenceDate fallback = %q", got)
		}
	})
	for _, deadline := range []string{"tomorrow", "2026-02-30T17:00", "2026-11-20T25:00", "2026-11-20T17:00junk"} {
		for _, field := range []string{"entry", "commitment"} {
			t.Run(field+"/"+deadline, func(t *testing.T) {
				input := CreateMeetInput{Name: "Invalid deadline"}
				if field == "entry" {
					input.EntryDeadline = deadline
				} else {
					input.CommitmentDeadline = deadline
				}
				if _, _, err := NewService().CreateMeet(admin, "org-a", input); !errors.Is(err, ErrValidation) {
					t.Fatalf("CreateMeet() error = %v, want ErrValidation", err)
				}
			})
		}
	}
}
