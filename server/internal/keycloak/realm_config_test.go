package keycloak

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Tenancy tracker item 2.4: the realm that ships with the repo must keep the
// tenant attribute out of users' hands. A user who could edit org_id (or the
// registration metadata the approval queue trusts) could move themselves to
// another club or approve themselves.
func TestRealmKeepsTenantAttributesAdminOnly(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "keycloak", "realm-aquify-local.json"))
	if err != nil {
		t.Fatal(err)
	}
	var realm struct {
		Components map[string][]struct {
			Config map[string][]string `json:"config"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &realm); err != nil {
		t.Fatal(err)
	}
	cfgs := realm.Components["org.keycloak.userprofile.UserProfileProvider"]
	if len(cfgs) == 0 || len(cfgs[0].Config["kc.user.profile.config"]) == 0 {
		t.Fatal("realm has no user profile configuration")
	}
	var profile struct {
		UnmanagedAttributePolicy string `json:"unmanagedAttributePolicy"`
		Attributes               []struct {
			Name        string `json:"name"`
			Permissions struct {
				Edit []string `json:"edit"`
			} `json:"permissions"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(cfgs[0].Config["kc.user.profile.config"][0]), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.UnmanagedAttributePolicy != "ADMIN_EDIT" {
		t.Fatalf("unmanagedAttributePolicy = %q; users must not be able to edit undeclared attributes (registration_status, requested_role, ...): want ADMIN_EDIT", profile.UnmanagedAttributePolicy)
	}
	found := false
	for _, a := range profile.Attributes {
		if a.Name != "org_id" {
			continue
		}
		found = true
		for _, who := range a.Permissions.Edit {
			if who != "admin" {
				t.Fatalf("org_id is editable by %q; only admin may edit it", who)
			}
		}
		if len(a.Permissions.Edit) == 0 {
			t.Fatal("org_id has no edit permission at all; admins must be able to set it")
		}
	}
	if !found {
		t.Fatal("org_id must be a declared (managed) user attribute")
	}
}
