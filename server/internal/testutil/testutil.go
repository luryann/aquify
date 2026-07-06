package testutil

import (
	"errors"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

const (
	DefaultOrgID       = "org-test"
	DefaultHouseholdID = "household-test"
)

type PrincipalOptions struct {
	UserID           string
	OrgID            string
	HouseholdID      string
	Roles            []string
	MFAAuthenticated bool
}

func Principal(opts PrincipalOptions) *appmiddleware.Principal {
	userID := opts.UserID
	if userID == "" {
		userID = "user-test"
	}

	orgID := opts.OrgID
	if orgID == "" {
		orgID = DefaultOrgID
	}

	roles := append([]string{}, opts.Roles...)
	if len(roles) == 0 {
		roles = []string{"swimmer"}
	}

	return &appmiddleware.Principal{
		UserID:           userID,
		OrgID:            orgID,
		HouseholdID:      opts.HouseholdID,
		Roles:            roles,
		MFAAuthenticated: opts.MFAAuthenticated,
	}
}

func AdminPrincipal() *appmiddleware.Principal {
	return Principal(PrincipalOptions{
		UserID: "admin-user",
		Roles:  []string{"admin"},
	})
}

func CoachPrincipal() *appmiddleware.Principal {
	return Principal(PrincipalOptions{
		UserID: "coach-user",
		Roles:  []string{"coach"},
	})
}

func ParentPrincipal(userID, householdID string) *appmiddleware.Principal {
	return Principal(PrincipalOptions{
		UserID:      userID,
		HouseholdID: householdID,
		Roles:       []string{"parent"},
	})
}

func SwimmerPrincipal(userID, householdID string) *appmiddleware.Principal {
	return Principal(PrincipalOptions{
		UserID:      userID,
		HouseholdID: householdID,
		Roles:       []string{"swimmer"},
	})
}

func StringPtr(v string) *string {
	return &v
}

func StringsPtr(v ...string) *[]string {
	out := append([]string{}, v...)
	return &out
}

func RequireErrorIs(t *testing.T, err, target error) {
	t.Helper()

	if !errors.Is(err, target) {
		t.Fatalf("expected error %v, got %v", target, err)
	}
}

func RequireRecentTime(t *testing.T, got time.Time, maxAge time.Duration) {
	t.Helper()

	if got.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
	delta := time.Since(got)
	if delta < 0 {
		delta = -delta
	}
	if delta > maxAge {
		t.Fatalf("expected timestamp within %s, got age %s", maxAge, delta)
	}
}
