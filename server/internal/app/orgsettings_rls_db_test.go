package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/testutil"
)

// org settings are isolated by the database; the one pre-auth lookup
// (public slug) finds exactly the club whose slug is presented and nothing else.
func TestOrgSettingsRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := orgsettings.NewServiceWithPool(appPool)
	slugA, slugB := "club-a-"+orgA[:6], "club-b-"+orgB[:6]
	for org, slug := range map[string]string{orgA: slugA, orgB: slugB} {
		name := "Club " + slug
		if _, err := svc.Update(org, orgsettings.Updates{TeamName: &name, PublicSlug: &slug}, ""); err != nil {
			t.Fatal(err)
		}
	}
	restarted := orgsettings.NewServiceWithPool(appPool)
	got, err := restarted.Get(orgA)
	if err != nil || got.PublicSlug != slugA {
		t.Fatalf("club A reads its own settings: %+v %v", got, err)
	}
	if found, err := restarted.FindBySlug(slugB); err != nil || found == nil || found.OrgID != orgB {
		t.Fatalf("the slug lookup finds exactly club B: %+v %v", found, err)
	}
	if found, err := restarted.FindBySlug("no-such-club"); err != nil || found != nil {
		t.Fatalf("an unknown slug finds nothing: %+v %v", found, err)
	}
	// Taking another club's slug is still refused (the uniqueness check sees across clubs).
	if _, err := restarted.Update(orgA, orgsettings.Updates{PublicSlug: &slugB}, ""); err == nil {
		t.Fatal("club A must not be able to take club B's slug")
	}
	var n int
	if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM org_settings`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unbound session must see nothing: %d %v", n, err)
	}
}
