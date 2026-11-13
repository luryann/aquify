package testutil

import (
	"errors"
	"testing"
	"time"
)

// cov1xRunFake runs fn against a detached *testing.T in its own goroutine so
// the helper's Fatal/Skip paths (which call runtime.Goexit) can be observed
// without failing the real test.
func cov1xRunFake(fn func(t *testing.T)) *testing.T {
	fake := &testing.T{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(fake)
	}()
	<-done
	return fake
}

func TestCov1xPrincipalDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	p := Principal(PrincipalOptions{})
	if p.UserID != "user-test" || p.OrgID != DefaultOrgID || p.HouseholdID != "" {
		t.Fatalf("unexpected defaults: %#v", p)
	}
	if len(p.Roles) != 1 || p.Roles[0] != "swimmer" || p.MFAAuthenticated {
		t.Fatalf("expected default swimmer role without MFA, got %#v", p)
	}

	roles := []string{"admin", "coach"}
	p = Principal(PrincipalOptions{UserID: "u1", OrgID: "o1", HouseholdID: "h1", Roles: roles, MFAAuthenticated: true})
	if p.UserID != "u1" || p.OrgID != "o1" || p.HouseholdID != "h1" || !p.MFAAuthenticated {
		t.Fatalf("overrides not applied: %#v", p)
	}
	roles[0] = "mutated"
	if p.Roles[0] != "admin" || len(p.Roles) != 2 {
		t.Fatalf("roles must be copied, got %#v", p.Roles)
	}
}

func TestCov1xRolePrincipals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		got       []string
		user      string
		household string
		role      string
	}{
		{"admin", AdminPrincipal().Roles, AdminPrincipal().UserID, AdminPrincipal().HouseholdID, "admin"},
		{"coach", CoachPrincipal().Roles, CoachPrincipal().UserID, CoachPrincipal().HouseholdID, "coach"},
		{"swimmer", SwimmerPrincipal("s1", "h9").Roles, SwimmerPrincipal("s1", "h9").UserID, SwimmerPrincipal("s1", "h9").HouseholdID, "swimmer"},
	}
	wantUser := map[string]string{"admin": "admin-user", "coach": "coach-user", "swimmer": "s1"}
	wantHH := map[string]string{"admin": "", "coach": "", "swimmer": "h9"}
	for _, tc := range cases {
		if len(tc.got) != 1 || tc.got[0] != tc.role {
			t.Errorf("%s: roles = %#v", tc.name, tc.got)
		}
		if tc.user != wantUser[tc.name] || tc.household != wantHH[tc.name] {
			t.Errorf("%s: user=%q household=%q", tc.name, tc.user, tc.household)
		}
	}
	if CoachPrincipal().OrgID != DefaultOrgID || SwimmerPrincipal("s", "h").OrgID != DefaultOrgID {
		t.Fatal("expected default org id")
	}
}

func TestCov1xStringPointers(t *testing.T) {
	t.Parallel()

	if p := StringPtr("abc"); p == nil || *p != "abc" {
		t.Fatalf("StringPtr returned %v", p)
	}
	src := []string{"a", "b"}
	ptr := StringsPtr(src...)
	src[0] = "z"
	if ptr == nil || len(*ptr) != 2 || (*ptr)[0] != "a" || (*ptr)[1] != "b" {
		t.Fatalf("StringsPtr must copy its input, got %v", ptr)
	}
	if empty := StringsPtr(); empty == nil || len(*empty) != 0 {
		t.Fatalf("StringsPtr() should be a non-nil empty slice, got %v", empty)
	}
}

func TestCov1xRequireErrorIsFailsOnMismatch(t *testing.T) {
	t.Parallel()

	target := errors.New("target")
	fake := cov1xRunFake(func(ft *testing.T) { RequireErrorIs(ft, errors.New("other"), target) })
	if !fake.Failed() {
		t.Fatal("expected RequireErrorIs to fail for a non-matching error")
	}
	fake = cov1xRunFake(func(ft *testing.T) { RequireErrorIs(ft, nil, target) })
	if !fake.Failed() {
		t.Fatal("expected RequireErrorIs to fail for a nil error")
	}
}

func TestCov1xRequireRecentTime(t *testing.T) {
	t.Parallel()

	if f := cov1xRunFake(func(ft *testing.T) { RequireRecentTime(ft, time.Time{}, time.Hour) }); !f.Failed() {
		t.Fatal("zero time must fail")
	}
	if f := cov1xRunFake(func(ft *testing.T) { RequireRecentTime(ft, time.Now().Add(-time.Hour), time.Minute) }); !f.Failed() {
		t.Fatal("old time must fail")
	}
	// A timestamp in the future is measured by absolute distance.
	if f := cov1xRunFake(func(ft *testing.T) { RequireRecentTime(ft, time.Now().Add(time.Hour), time.Minute) }); !f.Failed() {
		t.Fatal("far-future time must fail")
	}
	if f := cov1xRunFake(func(ft *testing.T) { RequireRecentTime(ft, time.Now().Add(5*time.Second), time.Minute) }); f.Failed() {
		t.Fatal("near-future time within tolerance must pass")
	}
}

func TestCov1xRequireDBHarnessSkipsWithoutDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "  ")
	fake := cov1xRunFake(func(ft *testing.T) { RequireDBHarness(ft) })
	if !fake.Skipped() || fake.Failed() {
		t.Fatalf("expected skip, skipped=%v failed=%v", fake.Skipped(), fake.Failed())
	}
}

func TestCov1xRequireDBHarnessFailsWithoutPsql(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/db")
	t.Setenv("PATH", t.TempDir())
	fake := cov1xRunFake(func(ft *testing.T) { RequireDBHarness(ft) })
	if !fake.Failed() || fake.Skipped() {
		t.Fatalf("expected failure when psql is missing, failed=%v skipped=%v", fake.Failed(), fake.Skipped())
	}
}

func TestCov1xExecFailsOnBadSQL(t *testing.T) {
	h := RequireDBHarness(t)
	fake := cov1xRunFake(func(ft *testing.T) { h.Exec(ft, "select * from cov1x_table_that_does_not_exist") })
	if !fake.Failed() {
		t.Fatal("expected Exec to fail for invalid SQL")
	}
	if got := h.Exec(t, "select 40 + 2"); got != "42" {
		t.Fatalf("expected 42, got %q", got)
	}
}
