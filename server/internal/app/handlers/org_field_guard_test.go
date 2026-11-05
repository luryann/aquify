package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tenancy tracker item 2.7. Request/payload structs may carry an org_id only
// where the handler pins it to the authenticated org. This guard counts the
// `json:"org_id"` fields per file; a new one fails until it is reviewed and the
// allowlist (with the reason it is safe) is updated.
var orgIDFieldAllowlist = map[string]struct {
	count  int
	reason string
}{
	"auth.go":               {3, "dev-only session issue (gated by AllowDevSessionIssue); OIDC callback ignores it; role assignment pins it via the scope check"},
	"members.go":            {2, "validateBodyOrgScope pins body org_id to the authenticated org"},
	"users.go":              {1, "public self-register body; the org is resolved from club_slug server-side and a raw org_id is dev/test only"},
	"compliance.go":         {1, "background job payload built server-side, never decoded from a request"},
	"comms_campaign_job.go": {1, "background job payload built server-side, never decoded from a request"},
}

func TestOrgIDRequestFieldsAreReviewed(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Count(string(src), `json:"org_id`)
		want := orgIDFieldAllowlist[f].count
		if got != want {
			t.Errorf("%s has %d org_id request fields, reviewed allowlist says %d: pin the new field to the authenticated org (validateBodyOrgScope) and update orgIDFieldAllowlist with the reason", f, got, want)
		}
	}
}
