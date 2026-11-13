package performance

import "testing"

func TestCov2xProgressReportSameNameSwimmersOrderedByID(t *testing.T) {
	s := NewService()
	s.SetDirectory(&fakeDirectory{
		swimmers: []SwimmerProfile{
			{ID: "zz-2", HouseholdID: "hh-1", FirstName: "Sam", LastName: "Lee", DateOfBirth: "2012-01-01", Gender: "female"},
			{ID: "aa-1", HouseholdID: "hh-2", FirstName: "Sam", LastName: "Lee", DateOfBirth: "2012-01-01", Gender: "female"},
		},
		meets: []MeetInfo{
			{ID: "mA", Name: "Fall", StartDate: "2025-01-10", Course: "SCY"},
			{ID: "mB", Name: "Winter", StartDate: "2026-01-10", Course: "SCY"},
		},
	})
	for _, id := range []string{"zz-2", "aa-1"} {
		cov2xAdd(t, s, coachP, "org", cov2xSwim{id, "mA", "SCY", "free", 100, 60000})
		cov2xAdd(t, s, coachP, "org", cov2xSwim{id, "mB", "SCY", "free", 100, 57000})
	}
	r, err := s.ProgressReport("org", coachP, cov2xIn())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Swimmers) != 2 || r.Swimmers[0].SwimmerID != "aa-1" || r.Swimmers[1].SwimmerID != "zz-2" {
		t.Fatalf("identical names must fall back to swimmer id order: %+v", r.Swimmers)
	}
	for _, sw := range r.Swimmers {
		if sw.EventsCompared != 1 || sw.AvgPercent != 5 {
			t.Fatalf("progress for %s: %+v", sw.SwimmerID, sw)
		}
	}
}
