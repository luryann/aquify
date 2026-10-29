package handlers

import (
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestPrincipalMayActForHouseholdHonoursSessionAndLinkedHouseholds(t *testing.T) {
	parent := &appmiddleware.Principal{UserID: "p", Roles: []string{"parent"}, HouseholdID: " hh-1 "}
	linked := []string{"hh-1", "hh-2"}
	cases := []struct {
		name   string
		p      *appmiddleware.Principal
		hh     string
		linked []string
		want   bool
	}{
		{"session household (trimmed, shared rule)", parent, "hh-1", nil, true},
		{"second linked household of a multi-household parent", parent, " hh-2 ", linked, true},
		{"foreign household", parent, "hh-9", linked, false},
		{"empty household never qualifies", parent, "", linked, false},
		{"nil principal", nil, "hh-1", linked, false},
		{"no session household but linked", &appmiddleware.Principal{UserID: "p", Roles: []string{"parent"}}, "hh-2", linked, true},
	}
	for _, tc := range cases {
		if got := principalMayActForHousehold(tc.p, tc.hh, tc.linked); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
