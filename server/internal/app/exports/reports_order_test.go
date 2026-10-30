package exports

import (
	"reflect"
	"testing"
)

// Session IDs are opaque UUIDs, so session groups must follow meet order (event
// numbers), not ID order.
func TestOrderReportGroupsSessionsFollowEventOrder(t *testing.T) {
	groups := map[string][]ReportEntry{
		"aaa\x00Day 2": {{EventNumber: "30"}, {EventNumber: "25"}},
		"zzz\x00Day 1": {{EventNumber: "10"}, {EventNumber: "2"}},
		"mmm\x00Day 3": {{EventNumber: "41A"}},
	}
	order := []string{"aaa\x00Day 2", "zzz\x00Day 1", "mmm\x00Day 3"}
	orderReportGroups(order, groups, true)
	want := []string{"zzz\x00Day 1", "aaa\x00Day 2", "mmm\x00Day 3"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("session order = %q, want %q", order, want)
	}

	byCode := []string{"SENIOR", "AGE", "\xff"}
	orderReportGroups(byCode, nil, false)
	if want := []string{"AGE", "SENIOR", "\xff"}; !reflect.DeepEqual(byCode, want) {
		t.Fatalf("group order = %q, want %q", byCode, want)
	}
}
