package events

import "testing"

func TestEventPublicVisibilityRule(t *testing.T) {
	t.Parallel()
	ok := &Event{OrgID: "o", Public: true, Audience: "all", Status: "scheduled"}
	if !ok.IsPubliclyVisible("o") {
		t.Fatal("public scheduled all-audience event must be visible")
	}
	for name, e := range map[string]*Event{
		"other org":  {OrgID: "x", Public: true, Audience: "all", Status: "scheduled"},
		"not public": {OrgID: "o", Public: false, Audience: "all", Status: "scheduled"},
		"groups":     {OrgID: "o", Public: true, Audience: "groups", Status: "scheduled"},
		"cancelled":  {OrgID: "o", Public: true, Audience: "all", Status: "cancelled"},
		"nil":        nil,
	} {
		if e.IsPubliclyVisible("o") {
			t.Errorf("%s must not be public", name)
		}
	}
}
