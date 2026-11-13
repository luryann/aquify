package handlers

import (
	"testing"
	"time"

	"github.com/luryann/aquify/meetfiles"
	"github.com/luryann/aquify/server/internal/app/members"
)

func TestResultImportHelpersAndSwimmerMatching(t *testing.T) {
	t.Parallel()
	if resultCourse(meetfiles.Course(""), "lcm", "SCY") != "LCM" || resultCourse("", "") != "" {
		t.Fatal("course resolution should take the first valid course")
	}
	if resultGender("", "female") != "F" || resultGender("x", "male") != "X" || resultGender("", "undisclosed") != "X" {
		t.Fatal("gender resolution")
	}
	if normalizeUSS(" 0402-12 casjone ") != "040212CASJONE" {
		t.Fatalf("normalizeUSS: %q", normalizeUSS(" 0402-12 casjone "))
	}

	swimmers := []members.Swimmer{
		{ID: "a", FirstName: "Casey", LastName: "Jones", DateOfBirth: "2012-04-02", Gender: "female", USASwimmingID: "040212CASJONE"},
		{ID: "b", FirstName: "Alex", LastName: "Kim", DateOfBirth: "2011-01-01", Gender: "male"},
		{ID: "c", FirstName: "Alex", LastName: "Kim", DateOfBirth: "2011-01-01", Gender: "female"},
		{ID: "d", FirstName: "Robert", PreferredName: "Bobby", LastName: "Lee", DateOfBirth: "2013-05-05", Gender: "male"},
	}
	m := newResultSwimmerMatcher(swimmers)
	dob := func(s string) *time.Time { t, _ := time.Parse("2006-01-02", s); return &t }

	if sw, _ := m.match(meetfiles.ParsedSwimmer{USSNumber: "040212-CASJONE", FirstName: "C", LastName: "Jones"}); sw == nil || sw.ID != "a" {
		t.Fatal("USA Swimming ID should match first")
	}
	if sw, _ := m.match(meetfiles.ParsedSwimmer{FirstName: "Alex", LastName: "Kim", DOB: dob("2011-01-01"), Gender: "F"}); sw == nil || sw.ID != "c" {
		t.Fatal("gender should break a name+DOB tie")
	}
	if sw, reason := m.match(meetfiles.ParsedSwimmer{FirstName: "Alex", LastName: "Kim", DOB: dob("2011-01-01")}); sw != nil || reason == "" {
		t.Fatal("unresolvable tie must be reported, not guessed")
	}
	if sw, _ := m.match(meetfiles.ParsedSwimmer{FirstName: "Bobby", LastName: "Lee", DOB: dob("2013-05-05")}); sw == nil || sw.ID != "d" {
		t.Fatal("preferred name should match")
	}
	if sw, reason := m.match(meetfiles.ParsedSwimmer{FirstName: "Casey", LastName: "Jones", DOB: dob("2010-01-01")}); sw != nil || reason != "name matches but date of birth does not" {
		t.Fatalf("DOB mismatch should not match: %v %q", sw, reason)
	}
	if sw, _ := m.match(meetfiles.ParsedSwimmer{FirstName: "Nobody", LastName: "Here"}); sw != nil {
		t.Fatal("unknown swimmer should not match")
	}
}
