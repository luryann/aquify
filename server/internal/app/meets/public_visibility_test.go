package meets

import "testing"

func TestMeetPublicListingRule(t *testing.T) {
	t.Parallel()
	today := "2026-10-29"
	cases := []struct {
		name string
		m    Meet
		want bool
	}{
		{"upcoming", Meet{OrgID: "o", Status: "published", StartDate: "2026-11-01", EndDate: "2026-11-02"}, true},
		{"ends today", Meet{OrgID: "o", Status: "commitment_open", StartDate: "2026-10-29"}, true},
		{"draft", Meet{OrgID: "o", Status: "draft", StartDate: "2026-11-01"}, false},
		{"archived", Meet{OrgID: "o", Status: "archived", StartDate: "2026-11-01"}, false},
		{"past", Meet{OrgID: "o", Status: "completed", StartDate: "2026-10-01", EndDate: "2026-10-02"}, false},
		{"no dates", Meet{OrgID: "o", Status: "published"}, false},
		{"other org", Meet{OrgID: "x", Status: "published", StartDate: "2026-11-01"}, false},
	}
	for _, tc := range cases {
		if got := tc.m.IsPubliclyListed("o", today); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
