package performance

import (
	"fmt"
	"slices"
	"testing"
)

// Standards are listed newest season first, then by standard code. The service keeps them in a
// map, so the order they reach the sort is random: many seasons, listed repeatedly, make sure
// the comparator is exercised in both directions and the result never depends on map order.
func TestListTimeStandardsOrdersNewestSeasonFirstThenCode(t *testing.T) {
	t.Parallel()
	s := NewService()
	inserted := []struct {
		season int
		code   string
	}{
		{2023, "B"}, {2026, "A"}, {2024, "C"}, {2026, "B"}, {2023, "A"},
		{2025, "A"}, {2022, "Z"}, {2025, "B"}, {2024, "A"}, {2027, "A"},
	}
	for i, in := range inserted {
		_, err := s.CreateTimeStandard("org", "admin", adminP, CreateTimeStandardInput{
			Name: fmt.Sprintf("standard %d", i), SeasonYear: in.season, StandardCode: in.code,
			EventDistance: 50 + i, Stroke: "free", Course: "SCY", Gender: "F", TimeMs: 60000, IsActive: true,
		})
		if err != nil {
			t.Fatalf("create %d/%s: %v", in.season, in.code, err)
		}
	}

	want := []string{"2027/A", "2026/A", "2026/B", "2025/A", "2025/B", "2024/A", "2024/C", "2023/A", "2023/B", "2022/Z"}
	for round := 0; round < 25; round++ {
		list, err := s.ListTimeStandards("org", ListTimeStandardsFilters{})
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, len(list))
		for i, ts := range list {
			got[i] = fmt.Sprintf("%d/%s", ts.SeasonYear, ts.StandardCode)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("round %d: order = %v, want %v", round, got, want)
		}
	}
}
