package testutil

import (
	"errors"
	"testing"
	"time"
)

func TestParentPrincipalUsesExpectedDefaults(t *testing.T) {
	t.Parallel()

	principal := ParentPrincipal("parent-1", "household-1")
	if principal.UserID != "parent-1" {
		t.Fatalf("expected user id parent-1, got %q", principal.UserID)
	}
	if principal.OrgID != DefaultOrgID {
		t.Fatalf("expected default org id %q, got %q", DefaultOrgID, principal.OrgID)
	}
	if principal.HouseholdID != "household-1" {
		t.Fatalf("expected household id household-1, got %q", principal.HouseholdID)
	}
	if len(principal.Roles) != 1 || principal.Roles[0] != "parent" {
		t.Fatalf("expected parent role, got %#v", principal.Roles)
	}
}

func TestRequireErrorIsAcceptsWrappedError(t *testing.T) {
	t.Parallel()

	target := errors.New("boom")
	RequireErrorIs(t, errors.Join(target, errors.New("extra")), target)
}

func TestRequireRecentTimeAcceptsFreshTimestamp(t *testing.T) {
	t.Parallel()

	RequireRecentTime(t, time.Now().UTC(), time.Second)
}
