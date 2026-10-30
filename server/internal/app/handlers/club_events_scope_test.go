package handlers

import (
	"net/http"
	"testing"
)

// A staff RSVP on behalf of a household must reject household IDs outside the
// caller's org rather than storing (and possibly billing) a foreign household.
func TestStaffRSVPRejectsHouseholdOutsideOrg(t *testing.T) {
	f := newEngagementFixture(t)
	id := createClubEvent(t, f, "")
	path := "/api/v1/club-events/" + id + "/rsvps"
	expectStatus(t, f.do(t, http.MethodPut, path, "admin", `{"household_id":"hh-other-org","response":"yes"}`), http.StatusUnprocessableEntity, "foreign household")
	expectStatus(t, f.do(t, http.MethodPut, path, "admin", `{"household_id":"hh-a","response":"yes"}`), http.StatusOK, "own-org household")
}
