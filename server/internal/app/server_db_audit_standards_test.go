package app

import (
	"context"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/testutil"
)

func TestDBAuditQueryReadsFromPostgres(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)
	const org = "audit-db-test-org"
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `ALTER TABLE audit_events DISABLE TRIGGER USER`)
	_, _ = pool.Exec(ctx, `DELETE FROM audit_events WHERE org_id = $1`, org)
	_, _ = pool.Exec(ctx, `ALTER TABLE audit_events ENABLE TRIGGER USER`)

	svc1 := audit.NewService()
	svc1.SetPool(pool)
	svc2 := audit.NewService()
	svc2.SetPool(pool)

	for _, action := range []string{"a.one", "a.two", "b.three"} {
		if _, err := svc1.Append(audit.AppendRequest{OrgID: org, ActorUserID: "u1", Action: action, ResourceType: "thing", ResourceID: "r1"}); err != nil {
			t.Fatalf("append %s: %v", action, err)
		}
	}

	// A second instance sees events written by the first without a restart.
	all, err := svc2.Query(audit.Filter{OrgID: org})
	if err != nil || len(all) != 3 {
		t.Fatalf("cross-instance query = %d events, err %v; want 3", len(all), err)
	}
	if all[0].Action != "b.three" {
		t.Fatalf("newest first: got %q", all[0].Action)
	}

	page, err := svc2.Query(audit.Filter{OrgID: org, Limit: 1, Cursor: all[0].ID})
	if err != nil || len(page) != 1 || page[0].ID != all[1].ID {
		t.Fatalf("cursor page = %+v err %v; want event %s", page, err, all[1].ID)
	}
	none, err := svc2.Query(audit.Filter{OrgID: org, Cursor: "missing"})
	if err != nil || len(none) != 0 {
		t.Fatalf("unknown cursor = %d events, err %v; want 0", len(none), err)
	}
	byAction, err := svc2.Query(audit.Filter{OrgID: org, Action: "A.TWO"})
	if err != nil || len(byAction) != 1 {
		t.Fatalf("action filter = %d, err %v; want 1", len(byAction), err)
	}
}

func TestDBTimeStandardDeactivateAndDeleteSurviveRestart(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)
	admin := dbAdminPrincipal(t, pool)
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM time_standards WHERE org_id = $1::uuid`, testOrgID)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM time_standards WHERE org_id = $1::uuid`, testOrgID) })

	svc := performance.NewServiceWithPool(pool)
	mk := func(code string) string {
		ts, err := svc.CreateTimeStandard(testOrgID, admin.UserID, admin, performance.CreateTimeStandardInput{
			Name: "Zones " + code, SeasonYear: 2026, StandardCode: code, Description: "desc " + code,
			EventDistance: 100, Stroke: "free", Course: "SCY", Gender: "F", TimeMs: 60000, IsActive: true,
		})
		if err != nil {
			t.Fatalf("create %s: %v", code, err)
		}
		return ts.ID
	}
	keep, drop := mk("A"), mk("B")
	off := false
	if _, err := svc.UpdateTimeStandard(testOrgID, admin.UserID, keep, admin, performance.UpdateTimeStandardInput{IsActive: &off}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if err := svc.DeleteTimeStandard(testOrgID, admin.UserID, drop, admin); err != nil {
		t.Fatalf("delete: %v", err)
	}

	restarted := performance.NewServiceWithPool(pool)
	got, err := restarted.GetTimeStandard(testOrgID, keep)
	if err != nil {
		t.Fatalf("get kept standard after restart: %v", err)
	}
	if got.IsActive {
		t.Error("deactivated standard came back active after restart")
	}
	if got.Description != "desc A" {
		t.Errorf("description = %q after restart, want %q", got.Description, "desc A")
	}
	if _, err := restarted.GetTimeStandard(testOrgID, drop); err == nil {
		t.Error("deleted standard reappeared after restart")
	}
}
