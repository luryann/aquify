package volunteer

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCov1xEffectiveHours(t *testing.T) {
	job := &Job{HourCredit: 2.5}
	if got := effectiveHours(&Signup{Status: StatusConfirmed}, job); got != 0 {
		t.Fatalf("unworked signup earns nothing: %v", got)
	}
	if got := effectiveHours(&Signup{Status: StatusWorked}, job); got != 2.5 {
		t.Fatalf("worked without override earns the job credit: %v", got)
	}
	override := 1.25
	if got := effectiveHours(&Signup{Status: StatusWorked, HoursCredited: &override}, job); got != 1.25 {
		t.Fatalf("override wins: %v", got)
	}
}

func TestCov1xMemoryStoreJobs(t *testing.T) {
	ctx := context.Background()
	m := newMemoryStore()
	start := baseNow.Add(time.Hour)
	mk := func(id, org, title string, at time.Time) *Job {
		return &Job{ID: id, OrgID: org, Title: title, StartsAt: at, Capacity: 2}
	}
	// Same start and title: id breaks the tie; then title; then start.
	jobs := []*Job{
		mk("b", testOrg, "Timer", start),
		mk("a", testOrg, "Timer", start),
		mk("c", testOrg, "Judge", start),
		mk("d", testOrg, "Zed", start.Add(-time.Hour)),
		mk("x", "other-org", "Foreign", start),
	}
	if err := m.insertJobs(ctx, jobs); err != nil {
		t.Fatal(err)
	}
	got, err := m.listJobs(ctx, testOrg, "")
	if err != nil {
		t.Fatal(err)
	}
	var order string
	for _, j := range got {
		order += j.ID
	}
	if order != "dcab" {
		t.Fatalf("sort order by start, title, id: %q", order)
	}

	// updateJob: unknown id, wrong org, capacity guard, success.
	if err := m.updateJob(ctx, mk("zz", testOrg, "n", start)); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if err := m.updateJob(ctx, mk("a", "other-org", "n", start)); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("wrong org: %v", err)
	}
	su := &Signup{ID: "s1", OrgID: testOrg, JobID: "a", HouseholdID: householdA, VolunteerName: "V", Status: StatusConfirmed}
	if err := m.insertSignup(ctx, su); err != nil {
		t.Fatal(err)
	}
	cancelled := &Signup{ID: "s2", OrgID: testOrg, JobID: "a", HouseholdID: householdB, VolunteerName: "W", Status: StatusCancelled}
	if err := m.insertSignup(ctx, cancelled); err != nil {
		t.Fatal(err)
	}
	low := mk("a", testOrg, "Timer", start)
	low.Capacity = 0
	if err := m.updateJob(ctx, low); !errors.Is(err, ErrCapacityBelowFilled) {
		t.Fatalf("capacity guard: %v", err)
	}
	ok := mk("a", testOrg, "Renamed", start)
	ok.Capacity = 1
	if err := m.updateJob(ctx, ok); err != nil {
		t.Fatal(err)
	}
	if j, _ := m.getJob(ctx, testOrg, "a"); j.Title != "Renamed" || j.Capacity != 1 {
		t.Fatalf("update not applied: %+v", j)
	}

	// deleteJob: unknown, wrong org, worked guard, cascade.
	if err := m.deleteJob(ctx, testOrg, "zz"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("delete unknown: %v", err)
	}
	if err := m.deleteJob(ctx, "other-org", "a"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("delete wrong org: %v", err)
	}
	worked := &Signup{ID: "s3", OrgID: testOrg, JobID: "b", HouseholdID: householdA, VolunteerName: "X", Status: StatusWorked}
	if err := m.insertSignup(ctx, worked); err != nil {
		t.Fatal(err)
	}
	if err := m.deleteJob(ctx, testOrg, "b"); !errors.Is(err, ErrJobHasWorkedSignups) {
		t.Fatalf("worked guard: %v", err)
	}
	if err := m.deleteJob(ctx, testOrg, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.getSignup(ctx, testOrg, "s1"); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("signups cascade with the job: %v", err)
	}
	if _, err := m.getJob(ctx, "other-org", "b"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("getJob cross-org: %v", err)
	}
	if got, _ := m.listJobs(ctx, testOrg, "meet-none"); len(got) != 0 {
		t.Fatalf("meet filter: %+v", got)
	}
}

func TestCov1xMemoryStoreSignups(t *testing.T) {
	ctx := context.Background()
	m := newMemoryStore()
	if err := m.insertJobs(ctx, []*Job{{ID: "j", OrgID: testOrg, Capacity: 1}}); err != nil {
		t.Fatal(err)
	}
	mk := func(id, org, job, hh, name, status string, at time.Time) *Signup {
		return &Signup{ID: id, OrgID: org, JobID: job, HouseholdID: hh, VolunteerName: name, Status: status, CreatedAt: at}
	}
	// Job missing or in another org.
	if err := m.insertSignup(ctx, mk("s0", testOrg, "nope", householdA, "A", StatusConfirmed, baseNow)); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("unknown job: %v", err)
	}
	if err := m.insertSignup(ctx, mk("s0", "other-org", "j", householdA, "A", StatusConfirmed, baseNow)); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("foreign org job: %v", err)
	}
	if err := m.insertSignup(ctx, mk("s1", testOrg, "j", householdA, "Ann", StatusConfirmed, baseNow)); err != nil {
		t.Fatal(err)
	}
	if err := m.insertSignup(ctx, mk("s2", testOrg, "j", householdA, "ANN", StatusConfirmed, baseNow)); !errors.Is(err, ErrDuplicateSignup) {
		t.Fatalf("case-insensitive duplicate: %v", err)
	}
	if err := m.insertSignup(ctx, mk("s2", testOrg, "j", householdB, "Bob", StatusConfirmed, baseNow)); !errors.Is(err, ErrJobFull) {
		t.Fatalf("full: %v", err)
	}

	// updateSignup: unknown, wrong org.
	if err := m.updateSignup(ctx, mk("zz", testOrg, "j", householdA, "Ann", StatusWorked, baseNow), StatusConfirmed); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("update unknown: %v", err)
	}
	if err := m.updateSignup(ctx, mk("s1", "other-org", "j", householdA, "Ann", StatusWorked, baseNow), StatusConfirmed); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("update wrong org: %v", err)
	}

	// Listing filters and ordering (created_at, then id).
	m.signups["s9"] = mk("s9", testOrg, "j", householdB, "Zoe", StatusCancelled, baseNow)
	m.signups["s8"] = mk("s8", testOrg, "j", householdB, "Yan", StatusCancelled, baseNow.Add(-time.Hour))
	m.signups["f1"] = mk("f1", "other-org", "j", householdB, "Foreign", StatusCancelled, baseNow)
	ids := func(f SignupFilter) string {
		list, _ := m.listSignups(ctx, testOrg, f)
		s := ""
		for _, su := range list {
			s += su.ID + ","
		}
		return s
	}
	if got := ids(SignupFilter{}); got != "s8,s1,s9," {
		t.Fatalf("order: %q", got)
	}
	if got := ids(SignupFilter{JobID: "j", HouseholdID: householdB, Status: StatusCancelled}); got != "s8,s9," {
		t.Fatalf("combined filter: %q", got)
	}
	if got := ids(SignupFilter{JobID: "other"}); got != "" {
		t.Fatalf("job filter: %q", got)
	}
	if got := ids(SignupFilter{HouseholdID: householdA}); got != "s1," {
		t.Fatalf("household filter: %q", got)
	}
	if got := ids(SignupFilter{Status: StatusConfirmed}); got != "s1," {
		t.Fatalf("status filter: %q", got)
	}
	if _, err := m.getSignup(ctx, "other-org", "s1"); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}
}

func TestCov1xMemoryStoreRequirementsCreditsBuyouts(t *testing.T) {
	ctx := context.Background()
	m := newMemoryStore()

	if err := m.deleteRequirement(ctx, testOrg, "nope"); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("delete missing requirement: %v", err)
	}
	req := &Requirement{ID: "r1", OrgID: testOrg, SeasonID: testSeason, RequiredHours: 8}
	if _, err := m.upsertRequirement(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := m.deleteRequirement(ctx, "other-org", "r1"); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("cross-org delete: %v", err)
	}
	if err := m.deleteRequirement(ctx, testOrg, "r1"); err != nil {
		t.Fatal(err)
	}

	if err := m.insertCredit(ctx, &Credit{ID: "c1", OrgID: testOrg, SeasonID: testSeason, HouseholdID: householdA, Hours: 1}); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.listCredits(ctx, testOrg, "other-season", ""); len(got) != 0 {
		t.Fatalf("season filter: %+v", got)
	}
	if got, _ := m.listCredits(ctx, testOrg, "", householdB); len(got) != 0 {
		t.Fatalf("household filter: %+v", got)
	}
	if got, _ := m.listCredits(ctx, "other-org", "", ""); len(got) != 0 {
		t.Fatalf("org filter: %+v", got)
	}

	b := &Buyout{ID: "b1", OrgID: testOrg, SeasonID: testSeason, HouseholdID: householdA, Status: "pending", CreatedAt: baseNow}
	if err := m.reserveBuyout(ctx, b); err != nil {
		t.Fatal(err)
	}
	dup := *b
	dup.ID = "b2"
	if err := m.reserveBuyout(ctx, &dup); !errors.Is(err, ErrBuyoutExists) {
		t.Fatalf("duplicate reservation: %v", err)
	}
	// A different household and an earlier creation time sort first.
	earlier := &Buyout{ID: "b3", OrgID: testOrg, SeasonID: testSeason, HouseholdID: householdB, Status: "pending", CreatedAt: baseNow.Add(-time.Hour)}
	if err := m.reserveBuyout(ctx, earlier); err != nil {
		t.Fatal(err)
	}
	list, _ := m.listBuyouts(ctx, testOrg, testSeason)
	if len(list) != 2 || list[0].ID != "b3" {
		t.Fatalf("buyouts ordered by creation: %+v", list)
	}
	if got, _ := m.listBuyouts(ctx, testOrg, "other-season"); len(got) != 0 {
		t.Fatalf("season filter: %+v", got)
	}
	if got, _ := m.listBuyouts(ctx, "other-org", ""); len(got) != 0 {
		t.Fatalf("org filter: %+v", got)
	}
	billed := *b
	billed.Status = "billed"
	billed.InvoiceID = "inv"
	if err := m.updateBuyout(ctx, &billed); err != nil {
		t.Fatal(err)
	}
	wrongOrg := billed
	wrongOrg.OrgID = "other-org"
	if err := m.updateBuyout(ctx, &wrongOrg); !errors.Is(err, ErrBuyoutNotFound) {
		t.Fatalf("cross-org update: %v", err)
	}
	missing := billed
	missing.ID = "zz"
	if err := m.updateBuyout(ctx, &missing); !errors.Is(err, ErrBuyoutNotFound) {
		t.Fatalf("missing update: %v", err)
	}
	// Deleting in the wrong org or an unknown id is a silent no-op.
	_ = m.deleteBuyout(ctx, "other-org", "b1")
	_ = m.deleteBuyout(ctx, testOrg, "zz")
	if got, _ := m.listBuyouts(ctx, testOrg, ""); len(got) != 2 {
		t.Fatalf("no-op deletes removed rows: %+v", got)
	}
	_ = m.deleteBuyout(ctx, testOrg, "b1")
	if got, _ := m.listBuyouts(ctx, testOrg, ""); len(got) != 1 || got[0].ID != "b3" {
		t.Fatalf("delete: %+v", got)
	}
}
