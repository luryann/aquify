package performance

import (
	"errors"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestBestTimeOverrideAndCutQualification(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	parent := &appmiddleware.Principal{UserID: "parent-1", Roles: []string{"parent"}}
	best := 58_000

	if _, _, err := svc.OverrideBestTime("org-a", "admin-1", parent, OverrideBestTimeInput{
		SwimmerID: "swimmer-1", EventDistance: 100, Stroke: "free", Course: "SCY", BestTimeMs: &best, Reason: "seed",
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("OverrideBestTime() parent error = %v, want %v", err, ErrForbidden)
	}
	if _, _, err := svc.OverrideBestTime("org-a", "admin-1", admin, OverrideBestTimeInput{
		SwimmerID: "swimmer-1", EventDistance: 100, Stroke: "free", Course: "SCY", BestTimeMs: &best,
	}); !errors.Is(err, ErrOverrideRequiresReason) {
		t.Fatalf("OverrideBestTime() missing reason error = %v, want %v", err, ErrOverrideRequiresReason)
	}

	bt, audit, err := svc.OverrideBestTime("org-a", "admin-1", admin, OverrideBestTimeInput{
		SwimmerID: "swimmer-1", EventDistance: 100, Stroke: "free", Course: "SCY", BestTimeMs: &best, Reason: "verified import",
	})
	if err != nil {
		t.Fatalf("OverrideBestTime() error = %v", err)
	}
	if bt.BestTimeMs == nil || *bt.BestTimeMs != best || !bt.IsManualOverride {
		t.Fatalf("OverrideBestTime() = %+v, want manual override", bt)
	}
	if audit == nil || audit.Action != "best_time.manual_override" {
		t.Fatalf("OverrideBestTime() audit = %+v, want manual override audit", audit)
	}

	standard, err := svc.CreateTimeStandard("org-a", "admin-1", admin, CreateTimeStandardInput{
		Name: "AAA 100 Free", SeasonYear: 2026, StandardCode: "AAA", EventDistance: 100, Stroke: "free", Course: "SCY", Gender: "F", TimeMs: 60_000, IsActive: true,
	})
	if err != nil {
		t.Fatalf("CreateTimeStandard() error = %v", err)
	}
	qualification, err := svc.EvaluateCutQualification("org-a", admin, "swimmer-1", standard.ID)
	if err != nil {
		t.Fatalf("EvaluateCutQualification() error = %v", err)
	}
	if !qualification.Qualified || qualification.MarginMs == nil || *qualification.MarginMs != 2_000 {
		t.Fatalf("EvaluateCutQualification() = %+v, want qualified by 2000ms", qualification)
	}
}

func TestUpdateTimeStandardRejectsNonPositiveTimeAndBlankName(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	std, err := svc.CreateTimeStandard("org-a", "admin-1", admin, CreateTimeStandardInput{
		Name: "AAA", SeasonYear: 2026, StandardCode: "AAA", EventDistance: 100, Stroke: "free", Course: "SCY", Gender: "F", TimeMs: 60_000, IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	zero, blank := 0, "  "
	if _, err := svc.UpdateTimeStandard("org-a", "admin-1", std.ID, admin, UpdateTimeStandardInput{TimeMs: &zero}); !errors.Is(err, ErrStandardInvalid) {
		t.Fatalf("zero time: %v", err)
	}
	if _, err := svc.UpdateTimeStandard("org-a", "admin-1", std.ID, admin, UpdateTimeStandardInput{Name: &blank}); !errors.Is(err, ErrStandardInvalid) {
		t.Fatalf("blank name: %v", err)
	}
	if got, _ := svc.GetTimeStandard("org-a", std.ID); got.TimeMs != 60_000 || got.Name != "AAA" {
		t.Fatalf("rejected update must not change the standard: %+v", got)
	}
}
