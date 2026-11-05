package performance

import (
	"context"
	"testing"

	"github.com/google/uuid"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

func overrideFixture() (*Service, *appmiddleware.Principal) {
	return NewService(), &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
}

func ovSwim(t *testing.T, svc *Service, p *appmiddleware.Principal, org, sw string, ms int) *Result {
	t.Helper()
	// In-memory fixtures use the readable meet id "m"; the meet_id column is a
	// UUID, so database-backed fixtures (UUID org ids) record the swim meet-less.
	meet := "m"
	if _, err := uuid.Parse(org); err == nil {
		meet = ""
	}
	r, err := svc.UpsertResult(org, "admin-1", p, UpsertResultInput{
		SwimmerID: sw, MeetID: meet, Distance: 50, Stroke: "free", Course: "SCY", Gender: "F",
		FinTimeMs: intPtr(ms), IsOfficial: true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return r
}

func currentBT(svc *Service, org, sw string) *BestTime {
	return svc.btByKey[btKey(org, sw, 50, "free", "SCY")]
}

func TestSlowerOverrideSurvivesLaterSlowerSwim(t *testing.T) {
	svc, admin := overrideFixture()
	ovSwim(t, svc, admin, "org-a", "sw-1", 25_000)
	slow := 27_000
	if _, _, err := svc.OverrideBestTime("org-a", "admin-1", admin, OverrideBestTimeInput{
		SwimmerID: "sw-1", EventDistance: 50, Stroke: "free", Course: "SCY", BestTimeMs: &slow, Reason: "bad import",
	}); err != nil {
		t.Fatal(err)
	}
	ovSwim(t, svc, admin, "org-a", "sw-1", 28_100)
	if bt := currentBT(svc, "org-a", "sw-1"); bt.BestTimeMs == nil || *bt.BestTimeMs != 27_000 || !bt.IsManualOverride {
		t.Fatalf("override must stand: %+v", bt)
	}
	ovSwim(t, svc, admin, "org-a", "sw-1", 26_000)
	if bt := currentBT(svc, "org-a", "sw-1"); *bt.BestTimeMs != 26_000 || bt.IsManualOverride || bt.OverrideReason != "" {
		t.Fatalf("faster later swim must supersede: %+v", bt)
	}
}

func TestNTOverrideSupersededByLaterSwim(t *testing.T) {
	svc, admin := overrideFixture()
	ovSwim(t, svc, admin, "org-a", "sw-1", 25_000)
	if _, _, err := svc.OverrideBestTime("org-a", "admin-1", admin, OverrideBestTimeInput{
		SwimmerID: "sw-1", EventDistance: 50, Stroke: "free", Course: "SCY", BestTimeMs: nil, Reason: "clear",
	}); err != nil {
		t.Fatal(err)
	}
	ovSwim(t, svc, admin, "org-a", "sw-1", 30_000)
	if bt := currentBT(svc, "org-a", "sw-1"); bt.BestTimeMs == nil || *bt.BestTimeMs != 30_000 {
		t.Fatalf("NT override should be replaced by the new swim: %+v", bt)
	}
}

func TestEqualTimeBestTimeKeepsEarliestSwim(t *testing.T) {
	svc, admin := overrideFixture()
	first := ovSwim(t, svc, admin, "org-a", "sw-1", 25_000)
	ovSwim(t, svc, admin, "org-a", "sw-1", 25_000)
	for i := 0; i < 30; i++ {
		if err := func() error {
			svc.mu.Lock()
			defer svc.mu.Unlock()
			return svc.recomputeBestTimeLocked("org-a", "admin-1", "sw-1", 50, "free", "SCY")
		}(); err != nil {
			t.Fatal(err)
		}
		if bt := currentBT(svc, "org-a", "sw-1"); bt.ResultID != first.ID {
			t.Fatalf("iteration %d: result_id = %s, want earliest %s", i, bt.ResultID, first.ID)
		}
	}
}

func TestOverridePersistsAcrossReload(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), Roles: []string{"admin"}}
	org, sw, household := uuid.NewString(), uuid.NewString(), uuid.NewString()
	// results.swimmer_id is a foreign key, so the swimmer must exist.
	if _, err := pool.Exec(context.Background(), `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Override test')`, household, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender) VALUES ($1, $2, $3, 'Test', 'Swimmer', '2013-01-01', 'female')`, sw, org, household); err != nil {
		t.Fatal(err)
	}
	ovSwim(t, svc, admin, org, sw, 25_000)
	slow := 27_000
	if _, _, err := svc.OverrideBestTime(org, admin.UserID, admin, OverrideBestTimeInput{
		SwimmerID: sw, EventDistance: 50, Stroke: "free", Course: "SCY", BestTimeMs: &slow, Reason: "bad import",
	}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.repo.LoadBestTimes(context.Background(), org)
	if err != nil || len(list) != 1 {
		t.Fatalf("load: %v %d", err, len(list))
	}
	if !list[0].IsManualOverride || list[0].OverrideReason != "bad import" {
		t.Fatalf("override metadata lost on reload: %+v", list[0])
	}
	svc2 := NewServiceWithPool(pool)
	if err := svc2.ensureOrg(org); err != nil {
		t.Fatal(err)
	}
	ovSwim(t, svc2, admin, org, sw, 28_100)
	if bt := svc2.btByKey[btKey(org, sw, 50, "free", "SCY")]; bt == nil || *bt.BestTimeMs != 27_000 || !bt.IsManualOverride {
		t.Fatalf("override must survive reload: %+v", bt)
	}
}
