package comms

import "testing"

func TestAnnouncementPublicVisibilityRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a    Announcement
		want bool
	}{
		{"public published all", Announcement{Public: true, Status: "published", Audience: "all"}, true},
		{"not public", Announcement{Public: false, Status: "published", Audience: "all"}, false},
		{"draft", Announcement{Public: true, Status: "draft", Audience: "all"}, false},
		{"archived", Announcement{Public: true, Status: "archived", Audience: "all"}, false},
		{"group audience", Announcement{Public: true, Status: "published", Audience: "group"}, false},
	}
	for _, tc := range cases {
		if got := tc.a.IsPubliclyVisible(); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
