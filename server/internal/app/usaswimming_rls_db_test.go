package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/usaswimming"
	"github.com/luryann/aquify/server/internal/testutil"
)

// each club's USA Swimming credentials are isolated by the database.
func TestUSASwimmingSettingsRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	cipher := usaswimming.NewCipher("test-secret-for-rls")
	svc := usaswimming.NewServiceWithPool(appPool, cipher, nil)
	for _, org := range []string{orgA, orgB} {
		url, key, on := "https://swims.example/"+org[:6], "key-"+org, true
		if _, err := svc.Update(org, usaswimming.Updates{BaseURL: &url, APIKey: &key, Enabled: &on}, ""); err != nil {
			t.Fatal(err)
		}
	}
	restarted := usaswimming.NewServiceWithPool(appPool, cipher, nil)
	for _, org := range []string{orgA, orgB} {
		v, err := restarted.Get(org)
		if err != nil || v == nil || v.BaseURL != "https://swims.example/"+org[:6] {
			t.Fatalf("club %s reads only its own settings after a restart: %+v %v", org, v, err)
		}
	}
	var n int
	if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM usa_swimming_integrations`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unbound session must see nothing: %d %v", n, err)
	}
}
