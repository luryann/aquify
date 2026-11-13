package wellness

import "testing"

func TestCov1xListOrdersSameSwimmerByDate(t *testing.T) {
	s := fixedService("2026-10-20")
	for _, d := range []string{"2026-10-20", "2026-10-18", "2026-10-19"} {
		if _, _, err := s.Upsert("org", Input{SwimmerID: "s1", Date: d, Metrics: good()}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.List("org", nil, "2026-10-01", "2026-10-31")
	if err != nil || len(list) != 3 {
		t.Fatalf("list: %v %+v", err, list)
	}
	for i, want := range []string{"2026-10-18", "2026-10-19", "2026-10-20"} {
		if list[i].Date != want {
			t.Fatalf("row %d date %s want %s", i, list[i].Date, want)
		}
	}
}

func TestCov1xFlagsAndAverageEmpty(t *testing.T) {
	flags := Flags(nil, "2026-10-20")
	if flags == nil || len(flags) != 0 {
		t.Fatalf("empty flags should be a non-nil empty slice, got %#v", flags)
	}
	if a := Average(nil); a != (Averages{}) {
		t.Fatalf("empty average should be zero-valued, got %+v", a)
	}
}
