package meets

import (
	"testing"
)

func intPtr(v int) *int { return &v }

func TestEventNumberLessIsStrictOrdering(t *testing.T) {
	t.Parallel()
	want := []string{"1", "9", "10", "10A", "10B", "X"}
	for i := range want {
		for j := range want {
			if got := eventNumberLess(want[i], want[j]); got != (i < j) {
				t.Fatalf("eventNumberLess(%q,%q)=%v, want %v", want[i], want[j], got, i < j)
			}
		}
	}
}
