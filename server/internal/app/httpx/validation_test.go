package httpx

import "testing"

type validationSample struct {
	SleepQuality int `json:"sleep_quality" validate:"required,gte=1,lte=5"`
	SleepHours   int `json:"sleep_hours" validate:"gte=0,lte=24"`
}

func TestFormatValidationErrorsUsesJSONNamesAndParams(t *testing.T) {
	err := NewValidator().Struct(validationSample{SleepQuality: 9, SleepHours: 30})
	issues := FormatValidationErrors(err)
	got := map[string]ValidationIssue{}
	for _, issue := range issues {
		got[issue.Field] = issue
	}
	if q, ok := got["sleep_quality"]; !ok || q.Message != "Must be at most 5." {
		t.Fatalf("sleep_quality issue = %+v (all %+v)", q, issues)
	}
	if h, ok := got["sleep_hours"]; !ok || h.Message != "Must be at most 24." {
		t.Fatalf("sleep_hours issue = %+v", h)
	}
}
