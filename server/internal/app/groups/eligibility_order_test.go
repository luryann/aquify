package groups

import (
	"fmt"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestEvaluateSwimmerEligibilityRuleOrderIsDeterministic(t *testing.T) {
	svc := NewService()
	svc.groups["g1"] = Group{ID: "g1", OrgID: "org-1"}
	svc.swimmers["sw1"] = swimmerRef{ID: "sw1", OrgID: "org-1", DateOfBirth: "2012-01-01", Gender: "F"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("r%02d", i)
		svc.rules[id] = EligibilityRule{ID: id, OrgID: "org-1", GroupID: "g1", RuleType: "gender", IsActive: true, CreatedAt: base}
	}
	admin := &appmiddleware.Principal{UserID: "u", Roles: []string{"admin"}}
	for n := 0; n < 20; n++ {
		res, err := svc.EvaluateSwimmerEligibility(admin, "org-1", "g1", "sw1")
		if err != nil {
			t.Fatal(err)
		}
		for i, rr := range res.RuleResults {
			if want := fmt.Sprintf("r%02d", i); rr.RuleID != want {
				t.Fatalf("run %d: result %d = %s, want %s", n, i, rr.RuleID, want)
			}
		}
	}
}
