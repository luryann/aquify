package performance

import (
	"errors"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func intPtr(v int) *int { return &v }

func TestResultsDriveBestTimesProgressionAndDedupe(t *testing.T) {
	t.Parallel()
	svc := NewService()
	coach := &appmiddleware.Principal{UserID: "coach-1", Roles: []string{"coach"}}
	parent := &appmiddleware.Principal{UserID: "parent-1", Roles: []string{"parent"}}

	if _, err := svc.UpsertResult("org-a", "parent-1", parent, UpsertResultInput{SwimmerID: "sw-1", Distance: 100, Stroke: "free", Course: "SCY", Gender: "F"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent upsert: want ErrForbidden, got %v", err)
	}

	swims := []struct {
		meet string
		ms   int
		dq   bool
	}{{"meet-1", 61_000, false}, {"meet-2", 59_500, false}, {"meet-3", 55_000, true}}
	for _, s := range swims {
		if _, err := svc.UpsertResult("org-a", "coach-1", coach, UpsertResultInput{
			SwimmerID: "sw-1", MeetID: s.meet, Distance: 100, Stroke: "free", Course: "SCY", Gender: "F",
			FinTimeMs: intPtr(s.ms), DQ: s.dq, IsOfficial: true, Place: intPtr(2),
			Splits: []SplitInput{{SplitNumber: 1, SplitDistance: 50, SplitTimeMs: s.ms / 2, CumulativeTimeMs: s.ms / 2}},
		}); err != nil {
			t.Fatalf("upsert %s: %v", s.meet, err)
		}
	}
	// Unofficial swims never set a best time.
	if _, err := svc.UpsertResult("org-a", "coach-1", coach, UpsertResultInput{SwimmerID: "sw-1", MeetID: "time-trial", Distance: 100, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: intPtr(50_000)}); err != nil {
		t.Fatalf("unofficial upsert: %v", err)
	}

	if !svc.HasMeetResult("org-a", "sw-1", "meet-2", 100, "free", "SCY") {
		t.Fatal("HasMeetResult should find the meet-2 swim")
	}
	if svc.HasMeetResult("org-a", "sw-1", "meet-2", 200, "free", "SCY") || svc.HasMeetResult("org-b", "sw-1", "meet-2", 100, "free", "SCY") {
		t.Fatal("HasMeetResult must match event and org exactly")
	}

	best, err := svc.ListBestTimes("org-a", coach, "sw-1", ListBestTimesFilters{Course: "SCY"})
	if err != nil || len(best) != 1 || best[0].BestTimeMs == nil || *best[0].BestTimeMs != 59_500 {
		t.Fatalf("best time should be the fastest official, non-DQ swim (59.50): %+v (%v)", best, err)
	}
	orgBest, err := svc.ListBestTimesForOrg("org-a", coach, ListBestTimesFilters{Stroke: "free"})
	if err != nil || len(orgBest) != 1 {
		t.Fatalf("org best times: %d (%v)", len(orgBest), err)
	}

	progression, err := svc.ListProgression("org-a", coach, "sw-1", ListResultsFilters{EventDistance: 100, Stroke: "free", Course: "SCY"})
	if err != nil || len(progression) != 4 {
		t.Fatalf("progression: want all 4 swims, got %d (%v)", len(progression), err)
	}
	orgProgression, err := svc.ListProgressionForOrg("org-a", coach, ListResultsFilters{SwimmerID: "sw-1"})
	if err != nil || len(orgProgression) != 4 {
		t.Fatalf("org progression: %d (%v)", len(orgProgression), err)
	}

	recomputed, err := svc.RecomputeBestTime("org-a", "coach-1", coach, "sw-1", 100, "free", "SCY")
	if err != nil || recomputed.BestTimeMs == nil || *recomputed.BestTimeMs != 59_500 {
		t.Fatalf("recompute: %+v (%v)", recomputed, err)
	}
}

func TestTimeStandardsLifecycle(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	coach := &appmiddleware.Principal{UserID: "coach-1", Roles: []string{"coach"}}

	std, err := svc.CreateTimeStandard("org-a", "admin-1", admin, CreateTimeStandardInput{
		Name: "JO 11-12 100 Free", SeasonYear: 2026, StandardCode: "JO", EventDistance: 100, Stroke: "free",
		Course: "SCY", Gender: "F", AgeMin: intPtr(11), AgeMax: intPtr(12), TimeMs: 62_000, IsActive: true,
	})
	if err != nil {
		t.Fatalf("CreateTimeStandard: %v", err)
	}
	if got, err := svc.GetTimeStandard("org-a", std.ID); err != nil || got.TimeMs != 62_000 {
		t.Fatalf("GetTimeStandard: %+v (%v)", got, err)
	}
	if _, err := svc.GetTimeStandard("org-b", std.ID); err == nil {
		t.Fatal("standards must be org-scoped")
	}

	list, err := svc.ListTimeStandards("org-a", ListTimeStandardsFilters{SeasonYear: 2026, StandardCode: "JO", EventDistance: 100, Stroke: "free", Course: "SCY", Gender: "F", ActiveOnly: true})
	if err != nil || len(list) != 1 {
		t.Fatalf("ListTimeStandards: %d (%v)", len(list), err)
	}

	newName, faster, inactive := "JO 11-12 100 Free (2027)", 61_500, false
	updated, err := svc.UpdateTimeStandard("org-a", "admin-1", std.ID, admin, UpdateTimeStandardInput{Name: &newName, TimeMs: &faster, IsActive: &inactive})
	if err != nil || updated.Name != newName || updated.TimeMs != 61_500 || updated.IsActive {
		t.Fatalf("UpdateTimeStandard: %+v (%v)", updated, err)
	}
	if active, _ := svc.ListTimeStandards("org-a", ListTimeStandardsFilters{ActiveOnly: true}); len(active) != 0 {
		t.Fatalf("inactive standard should be filtered out, got %d", len(active))
	}

	if err := svc.DeleteTimeStandard("org-a", "coach-1", std.ID, coach); err == nil {
		t.Fatal("coach should not be able to delete standards")
	}
	if err := svc.DeleteTimeStandard("org-a", "admin-1", std.ID, admin); err != nil {
		t.Fatalf("DeleteTimeStandard: %v", err)
	}
	if _, err := svc.GetTimeStandard("org-a", std.ID); err == nil {
		t.Fatal("deleted standard should be gone")
	}
}
