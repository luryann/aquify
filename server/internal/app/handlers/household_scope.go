package handlers

import (
	"slices"
	"strings"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// principalOwnsHousehold is the single household-scope rule for parent and
// swimmer principals in handlers that scope by the session's household: the
// principal must carry a non-empty household that equals householdID (both
// trimmed). Handlers must not re-implement this comparison.
func principalOwnsHousehold(principal *appmiddleware.Principal, householdID string) bool {
	if principal == nil {
		return false
	}
	own := strings.TrimSpace(principal.HouseholdID)
	return own != "" && own == strings.TrimSpace(householdID)
}

// parentOutsideHousehold reports whether a parent-only caller is acting on a
// household that is not their own. Admins are never restricted.
func parentOutsideHousehold(principal *appmiddleware.Principal, householdID string) bool {
	return hasAnyRole(principal, "parent") && !hasAnyRole(principal, "admin") &&
		!principalOwnsHousehold(principal, householdID)
}

// principalMayActForHousehold extends principalOwnsHousehold to parents linked
// to several households: the session household always qualifies (shared rule),
// and so does any household in linked (the directory's resolved set for the
// principal). An empty householdID never qualifies.
func principalMayActForHousehold(principal *appmiddleware.Principal, householdID string, linked []string) bool {
	if principalOwnsHousehold(principal, householdID) {
		return true
	}
	id := strings.TrimSpace(householdID)
	return id != "" && principal != nil && slices.Contains(linked, id)
}
