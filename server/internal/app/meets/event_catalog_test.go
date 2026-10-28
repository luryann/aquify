package meets

import (
	"testing"

	"github.com/dareaquatics/aquify/meetfiles"
)

func TestDeriveMeetEventsFromParseResult_DedupesAndParsesAgeGroups(t *testing.T) {
	result := &meetfiles.ParseResult{
		IndividualResults: []meetfiles.ParsedIndividualResult{
			{
				Event: meetfiles.ParsedEvent{
					EventNumber: "3", Distance: 100, Stroke: meetfiles.StrokeFreestyle,
					EventGender: "F", AgeGroup: "1517", Course: meetfiles.CourseShortCourseYards,
				},
			},
			{
				// Same event number, different swimmer — must collapse to one event.
				Event: meetfiles.ParsedEvent{
					EventNumber: "3", Distance: 100, Stroke: meetfiles.StrokeFreestyle,
					EventGender: "F", AgeGroup: "1517", Course: meetfiles.CourseShortCourseYards,
				},
			},
			{
				Event: meetfiles.ParsedEvent{
					EventNumber: "1", Distance: 200, Stroke: meetfiles.StrokeIM,
					EventGender: "M", AgeGroup: "OPEN", Course: meetfiles.CourseShortCourseYards,
				},
			},
		},
		RelayResults: []meetfiles.ParsedRelayResult{
			{
				Event: meetfiles.ParsedEvent{
					EventNumber: "10", Distance: 400, Stroke: meetfiles.StrokeFreestyleRelay,
					EventGender: "X", AgeGroup: "0018", Course: meetfiles.CourseShortCourseYards,
				},
			},
		},
	}

	events := deriveMeetEventsFromParseResult(result)
	if len(events) != 3 {
		t.Fatalf("expected 3 deduplicated events, got %d: %+v", len(events), events)
	}

	// Sorted numerically by event number: 1, 3, 10.
	if events[0].EventNumber != "1" || events[1].EventNumber != "3" || events[2].EventNumber != "10" {
		t.Fatalf("expected numeric event-number order [1 3 10], got %v", []string{events[0].EventNumber, events[1].EventNumber, events[2].EventNumber})
	}

	openEvent := events[0]
	if openEvent.MinAge != nil || openEvent.MaxAge != nil {
		t.Fatalf("expected OPEN age group to have no bounds, got min=%v max=%v", openEvent.MinAge, openEvent.MaxAge)
	}

	boundedEvent := events[1]
	if boundedEvent.MinAge == nil || *boundedEvent.MinAge != 15 || boundedEvent.MaxAge == nil || *boundedEvent.MaxAge != 17 {
		t.Fatalf("expected 15-17 age bounds for event 3, got min=%v max=%v", boundedEvent.MinAge, boundedEvent.MaxAge)
	}
	if boundedEvent.IsRelay {
		t.Fatalf("expected event 3 to not be a relay")
	}

	relayEvent := events[2]
	if !relayEvent.IsRelay {
		t.Fatalf("expected event 10 to be a relay")
	}
	if relayEvent.MinAge != nil {
		t.Fatalf("expected age group 0018 to have no minimum, got %v", *relayEvent.MinAge)
	}
	if relayEvent.MaxAge == nil || *relayEvent.MaxAge != 18 {
		t.Fatalf("expected age group 0018 to have max 18, got %v", relayEvent.MaxAge)
	}
}

func TestDeriveMeetEventsFromParseResult_NilAndEmpty(t *testing.T) {
	if got := deriveMeetEventsFromParseResult(nil); got != nil {
		t.Fatalf("expected nil for nil result, got %v", got)
	}
	if got := deriveMeetEventsFromParseResult(&meetfiles.ParseResult{}); got != nil {
		t.Fatalf("expected nil for result with no entries, got %v", got)
	}
}

func TestSdifAgeGroupBounds(t *testing.T) {
	cases := []struct {
		input   string
		wantMin *int
		wantMax *int
	}{
		{"OPEN", nil, nil},
		{"", nil, nil},
		{"1824", intPtr(18), intPtr(24)},
		{"0018", nil, intPtr(18)},
		{"1899", intPtr(18), nil},
		{"abcd", nil, nil},
	}
	for _, tc := range cases {
		min, max := sdifAgeGroupBounds(tc.input)
		if !intPtrEqual(min, tc.wantMin) || !intPtrEqual(max, tc.wantMax) {
			t.Errorf("sdifAgeGroupBounds(%q) = (%v, %v), want (%v, %v)", tc.input, min, max, tc.wantMin, tc.wantMax)
		}
	}
}

func intPtr(v int) *int { return &v }

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

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
