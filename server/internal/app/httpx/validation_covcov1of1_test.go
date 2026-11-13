package httpx

import (
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
)

type cov1xValidationAll struct {
	Req   string `json:"req" validate:"required"`
	Mail  string `json:"mail" validate:"email"`
	MinS  string `json:"min_s" validate:"min=3"`
	MaxS  string `json:"max_s" validate:"max=2"`
	Gte   int    `json:"gte" validate:"gte=5"`
	Lte   int    `json:"lte" validate:"lte=5"`
	Gt    int    `json:"gt" validate:"gt=5"`
	Lt    int    `json:"lt" validate:"lt=5"`
	Len   string `json:"len" validate:"len=4"`
	One   string `json:"one" validate:"oneof=a b"`
	Other string `json:"other" validate:"alpha"`
	Skip  string `json:"-" validate:"required"`
}

func TestCov1xValidationMessagesPerRule(t *testing.T) {
	in := cov1xValidationAll{Mail: "nope", MinS: "a", MaxS: "abcd", Gte: 1, Lte: 9, Gt: 5, Lt: 5, Len: "ab", One: "z", Other: "123"}
	issues := FormatValidationErrors(NewValidator().Struct(in))
	got := map[string]ValidationIssue{}
	for _, i := range issues {
		got[i.Field] = i
	}
	want := map[string]ValidationIssue{
		"req":   {Field: "req", Message: "This field is required.", Rule: "required"},
		"mail":  {Field: "mail", Message: "Must be a valid email address.", Rule: "email"},
		"min_s": {Field: "min_s", Message: "Value is below the minimum allowed.", Rule: "min"},
		"max_s": {Field: "max_s", Message: "Value exceeds the maximum allowed.", Rule: "max"},
		"gte":   {Field: "gte", Message: "Must be at least 5.", Rule: "gte"},
		"lte":   {Field: "lte", Message: "Must be at most 5.", Rule: "lte"},
		"gt":    {Field: "gt", Message: "Must be greater than 5.", Rule: "gt"},
		"lt":    {Field: "lt", Message: "Must be less than 5.", Rule: "lt"},
		"len":   {Field: "len", Message: "Must have length 4.", Rule: "len"},
		"one":   {Field: "one", Message: "Must be one of: a b.", Rule: "oneof"},
		"other": {Field: "other", Message: "Invalid value.", Rule: "alpha"},
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: got %+v want %+v", k, got[k], w)
		}
	}
	// The json:"-" field falls back to its Go name (lowercased) and is still validated.
	if sk, ok := got["skip"]; !ok || sk.Rule != "required" {
		t.Errorf("json:\"-\" field issue = %+v (present=%v)", sk, ok)
	}
	if len(issues) != len(want)+1 {
		t.Errorf("issues = %d (%+v), want exactly %d", len(issues), issues, len(want)+1)
	}
}

func TestCov1xFormatValidationErrorsNilAndGenericError(t *testing.T) {
	if got := FormatValidationErrors(nil); got != nil {
		t.Fatalf("nil error should give nil issues, got %+v", got)
	}
	got := FormatValidationErrors(errors.New("boom"))
	if len(got) != 1 || got[0].Field != "request" || got[0].Message != "boom" || got[0].Rule != "" {
		t.Fatalf("generic error issues = %+v", got)
	}
}

func TestCov1xValidationFieldNameLowercasesUntaggedFields(t *testing.T) {
	type untagged struct {
		SleepHours int `validate:"required"`
	}
	err := validator.New().Struct(untagged{})
	issues := FormatValidationErrors(err)
	if len(issues) != 1 || issues[0].Field != "sleephours" || issues[0].Rule != "required" {
		t.Fatalf("issues = %+v", issues)
	}
}
