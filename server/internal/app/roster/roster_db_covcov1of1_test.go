package roster

import (
	"testing"

	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	"github.com/luryann/aquify/server/internal/testutil"
)

const cov1xDBOrg = "5b1c7a3e-0d3e-4c61-9f0a-1c0a5e6f7a01"

func TestCov1xDBErrorsPropagate(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ms := members.NewServiceWithPool(pool)
	dir := NewMembersDirectory(ms, groups.NewService())

	// Healthy DB-backed read works (empty club).
	if out, err := dir.Swimmers(cov1xDBOrg); err != nil || len(out) != 0 {
		t.Fatalf("healthy read: %v %v", out, err)
	}

	// Close the pool after the swimmer list reload, so the group-link reload
	// is the call that fails.
	ms.SetSyncHookForTest(func() { pool.Close() })
	out, err := dir.Swimmers(cov1xDBOrg)
	if err == nil || out != nil {
		t.Fatalf("expected group-link load error, got %v %v", out, err)
	}

	// With the pool closed, every first read fails.
	ms.SetSyncHookForTest(nil)
	if out, err := dir.Swimmers(cov1xDBOrg); err == nil || out != nil {
		t.Errorf("Swimmers on closed pool: %v %v", out, err)
	}
	if names, err := dir.HouseholdNames(cov1xDBOrg); err == nil || names != nil {
		t.Errorf("HouseholdNames on closed pool: %v %v", names, err)
	}
}
