package handlers

import (
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/usaswimming"
)

func TestCompareRoster(t *testing.T) {
	swims := []usaswimming.RosterMember{
		{MemberID: "AAA111", FirstName: "Ann", LastName: "Lee", DateOfBirth: "2012-04-05"},
		{MemberID: "BBB222", FirstName: "Bo", LastName: "Kim", DateOfBirth: "2011-01-01"},
		{MemberID: "CCC333", FirstName: "Cy", LastName: "Ng"},
	}
	local := []members.Swimmer{
		{ID: "s1", FirstName: "Ann", LastName: "Lee", DateOfBirth: "2012-04-05", USASwimmingID: "aaa-111"},
		{ID: "s2", FirstName: "Bo", LastName: "Park", DateOfBirth: "2011-01-01", USASwimmingID: "BBB222"},
		{ID: "s3", FirstName: "Di", LastName: "Wu", USASwimmingID: "DDD444"},
		{ID: "s4", FirstName: "Ed", LastName: "Fox"},
	}
	matched, onlySwims, onlyLocal, changed := compareRoster(swims, local)
	if matched != 2 || len(onlySwims) != 1 || onlySwims[0].MemberID != "CCC333" {
		t.Fatalf("matched=%d onlySwims=%+v", matched, onlySwims)
	}
	if len(onlyLocal) != 1 || onlyLocal[0].SwimmerID != "s3" {
		t.Fatalf("onlyLocal=%+v", onlyLocal)
	}
	if len(changed) != 1 || changed[0].SwimmerID != "s2" {
		t.Fatalf("changed=%+v", changed)
	}
}
