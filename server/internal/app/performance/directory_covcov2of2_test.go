package performance

import (
	"encoding/json"
	"testing"
)

func TestCov2xAgeGroupForAgeNegativeAndBoundaries(t *testing.T) {
	cases := map[int]string{-1: "", 0: "8U", 8: "8U", 9: "9-10", 10: "9-10", 11: "11-12", 14: "13-14", 15: "15-16", 17: "17-18", 18: "17-18", 19: ""}
	for age, want := range cases {
		if got := AgeGroupForAge(age); got != want {
			t.Errorf("AgeGroupForAge(%d) = %q, want %q", age, got, want)
		}
	}
}

func TestCov2xAgeGroupContains(t *testing.T) {
	twelve, twenty := 12, 20
	if !ageGroupContains(AgeGroupOpen, nil) {
		t.Error("open must accept swims with unknown age")
	}
	if ageGroupContains("11-12", nil) {
		t.Error("a bounded group must reject swims with unknown age")
	}
	if !ageGroupContains("11-12", &twelve) || ageGroupContains("13-14", &twelve) {
		t.Error("age 12 belongs to 11-12 only")
	}
	if ageGroupContains("17-18", &twenty) {
		t.Error("age 20 belongs to no bounded group")
	}
}

func TestCov2xAgeOnEdgeCases(t *testing.T) {
	if AgeOn("2014-06-01", "not-a-date") != nil {
		t.Error("malformed swim date must yield nil")
	}
	if AgeOn("2014-06-01", "") != nil {
		t.Error("empty swim date must yield nil")
	}
	if AgeOn("2026-01-01", "2025-01-01") != nil {
		t.Error("a swim before birth must yield nil, not a negative age")
	}
	// Timestamps are truncated to their date part and whitespace is trimmed.
	a := AgeOn(" 2014-06-01T00:00:00Z ", "2026-06-01T23:59:59Z")
	if a == nil || *a != 12 {
		t.Errorf("timestamp inputs: got %v want 12", a)
	}
}

func TestCov2xNormalizeGender(t *testing.T) {
	cases := []struct{ result, swimmer, want string }{
		{"M", "", "M"},
		{" f ", "male", "F"}, // the result's own gender wins
		{"X", "female", "F"},
		{"", "Female", "F"},
		{"", "f", "F"},
		{"", "male", "M"},
		{"", " m ", "M"},
		{"X", "non_binary", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := normalizeGender(c.result, c.swimmer); got != c.want {
			t.Errorf("normalizeGender(%q,%q) = %q, want %q", c.result, c.swimmer, got, c.want)
		}
	}
}

func TestCov2xRuntimeSnapshotRoundTripAndErrors(t *testing.T) {
	s := NewService()
	// Empty payload is a no-op and must not clobber state.
	if err := s.LoadRuntimeSnapshot(nil); err != nil {
		t.Fatalf("empty snapshot: %v", err)
	}
	if err := s.LoadRuntimeSnapshot([]byte("{not json")); err == nil {
		t.Fatal("malformed snapshot must error")
	}

	ms := 61000
	r, err := s.UpsertResult("org", "coach", coachP, UpsertResultInput{
		SwimmerID: "amy", Distance: 100, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: &ms, IsOfficial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	// A null best-time entry in the payload is skipped when rebuilding the index.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var bts map[string]json.RawMessage
	if err := json.Unmarshal(raw["best_times"], &bts); err != nil {
		t.Fatal(err)
	}
	bts["ghost"] = json.RawMessage("null")
	raw["best_times"], _ = json.Marshal(bts)
	data, _ = json.Marshal(raw)

	fresh := NewService()
	if err := fresh.LoadRuntimeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	if got := fresh.results[r.ID]; got == nil || got.SwimmerID != "amy" || *got.FinTimeMs != ms {
		t.Fatalf("result not restored: %+v", got)
	}
	bt := fresh.btByKey[btKey("org", "amy", 100, "free", "SCY")]
	if bt == nil || bt.BestTimeMs == nil || *bt.BestTimeMs != ms {
		t.Fatalf("best-time index not rebuilt: %+v", bt)
	}
	if len(fresh.btByKey) != 1 {
		t.Fatalf("null best time must not be indexed, got %d entries", len(fresh.btByKey))
	}
}
