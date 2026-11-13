package volunteer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

type cov1xRepoRig struct {
	pool   *pgxpool.Pool
	repo   *Repository
	org    string
	season string
}

func cov1xNewRepoRig(t *testing.T) *cov1xRepoRig {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	season := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO seasons (id, org_id, name, status) VALUES ($1::uuid, $2, 'Cov Season', 'active')`, season, org); err != nil {
		t.Fatalf("seed season: %v", err)
	}
	return &cov1xRepoRig{pool: pool, repo: newRepository(pool), org: org, season: season}
}

func (r *cov1xRepoRig) job(title string, at time.Time, capacity int) *Job {
	return &Job{ID: uuid.NewString(), OrgID: r.org, EventName: "Evt", Role: "timer", Title: title,
		StartsAt: at, EndsAt: at.Add(2 * time.Hour), Capacity: capacity, HourCredit: 2,
		CreatedAt: at, UpdatedAt: at, CreatedBy: "u1", UpdatedBy: "u1"}
}

func (r *cov1xRepoRig) signup(jobID, hh, name string, at time.Time) *Signup {
	return &Signup{ID: uuid.NewString(), OrgID: r.org, JobID: jobID, HouseholdID: hh, VolunteerName: name,
		Status: StatusConfirmed, SignedUpBy: "u1", CreatedAt: at, UpdatedAt: at}
}

func TestCov1xRepositoryHelpers(t *testing.T) {
	if !isUUID(" "+uuid.NewString()+" ") || isUUID("nope") {
		t.Fatal("isUUID should trim and validate")
	}
	if nullable("  ") != nil || nullable("x") != "x" {
		t.Fatal("nullable should map blank to NULL")
	}
	if minAgeArg(0) != nil || minAgeArg(-3) != nil || minAgeArg(9) != 9 {
		t.Fatal("minAgeArg should map <=0 to NULL")
	}
	if isUniqueViolation(errors.New("plain")) || isUniqueViolation(nil) {
		t.Fatal("non-pg errors are not unique violations")
	}
}

func TestCov1xRepositorySettings(t *testing.T) {
	rig := cov1xNewRepoRig(t)
	ctx := context.Background()
	if _, found, err := rig.repo.getSettings(ctx, rig.org); err != nil || found {
		t.Fatalf("no row yet: %v %v", found, err)
	}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	st := &Settings{OrgID: rig.org, DefaultRequiredHours: 6.5, DefaultBuyoutCentsPerHour: 1500,
		CancellationCutoffHours: 12, RemindersEnabled: true, Timezone: "America/Chicago", UpdatedAt: &when, UpdatedBy: "u1"}
	if err := rig.repo.upsertSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	st.DefaultRequiredHours, st.RemindersEnabled, st.UpdatedAt = 3, false, nil
	if err := rig.repo.upsertSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	got, found, err := rig.repo.getSettings(ctx, rig.org)
	if err != nil || !found {
		t.Fatalf("get: %v %v", found, err)
	}
	if got.DefaultRequiredHours != 3 || got.RemindersEnabled || got.Timezone != "America/Chicago" ||
		got.DefaultBuyoutCentsPerHour != 1500 || got.CancellationCutoffHours != 12 || got.UpdatedBy != "u1" || got.UpdatedAt == nil {
		t.Fatalf("upsert should overwrite in place: %+v", got)
	}

	// Errors surface with context (a cancelled context fails every statement).
	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := rig.repo.getSettings(dead, rig.org); err == nil || !strings.Contains(err.Error(), "get settings") {
		t.Fatalf("get settings error: %v", err)
	}
	if err := rig.repo.upsertSettings(dead, st); err == nil || !strings.Contains(err.Error(), "upsert settings") {
		t.Fatalf("upsert settings error: %v", err)
	}
}

func TestCov1xRepositoryJobsLifecycle(t *testing.T) {
	rig := cov1xNewRepoRig(t)
	ctx := context.Background()
	at := time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC)

	if err := rig.repo.insertJobs(ctx, nil); err != nil {
		t.Fatalf("empty batch is a no-op: %v", err)
	}
	if _, err := rig.repo.getJob(ctx, rig.org, "not-a-uuid"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := rig.repo.getJob(ctx, rig.org, uuid.NewString()); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing id: %v", err)
	}

	a := rig.job("B-late", at.Add(time.Hour), 2)
	a.MeetID, a.SeasonID, a.MinAge, a.RequiresCertification = "meet-1", rig.season, 14, true
	b := rig.job("A-early", at, 1)
	c := rig.job("C-other-meet", at.Add(2*time.Hour), 1)
	c.MeetID = "meet-2"
	if err := rig.repo.insertJobs(ctx, []*Job{a, b, c}); err != nil {
		t.Fatal(err)
	}

	// Mixed orgs are rejected before touching the database.
	foreign := rig.job("x", at, 1)
	foreign.OrgID = cov1xOtherOrg
	if err := rig.repo.insertJobs(ctx, []*Job{b, foreign}); err == nil || !strings.Contains(err.Error(), "mixed orgs") {
		t.Fatalf("mixed orgs: %v", err)
	}
	// A failing row rolls back the whole batch (duplicate primary key).
	fresh := rig.job("fresh", at, 1)
	if err := rig.repo.insertJobs(ctx, []*Job{fresh, a}); err == nil || !strings.Contains(err.Error(), "insert job") {
		t.Fatalf("duplicate key: %v", err)
	}
	if _, err := rig.repo.getJob(ctx, rig.org, fresh.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("failed batch must roll back: %v", err)
	}

	all, err := rig.repo.listJobs(ctx, rig.org, "")
	if err != nil || len(all) != 3 || all[0].Title != "A-early" || all[2].Title != "C-other-meet" {
		t.Fatalf("ordered by start: %+v %v", all, err)
	}
	byMeet, _ := rig.repo.listJobs(ctx, rig.org, "meet-1")
	if len(byMeet) != 1 || byMeet[0].ID != a.ID {
		t.Fatalf("meet filter: %+v", byMeet)
	}
	if other, _ := rig.repo.listJobs(ctx, cov1xOtherOrg, ""); len(other) != 0 {
		t.Fatalf("org isolation: %+v", other)
	}
	got, err := rig.repo.getJob(ctx, rig.org, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MeetID != "meet-1" || got.SeasonID != rig.season || got.MinAge != 14 || !got.RequiresCertification ||
		!got.StartsAt.Equal(a.StartsAt) || got.StartsAt.Location() != time.UTC || got.HourCredit != 2 {
		t.Fatalf("round trip: %+v", got)
	}
	if plain, _ := rig.repo.getJob(ctx, rig.org, b.ID); plain.MeetID != "" || plain.SeasonID != "" || plain.MinAge != 0 {
		t.Fatalf("NULLs read back as zero values: %+v", plain)
	}

	// updateJob.
	if err := rig.repo.updateJob(ctx, &Job{ID: "bad", OrgID: rig.org}); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("bad id: %v", err)
	}
	ghost := rig.job("ghost", at, 1)
	if err := rig.repo.updateJob(ctx, ghost); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing: %v", err)
	}
	su1, su2 := rig.signup(a.ID, "hh1", "One", at), rig.signup(a.ID, "hh2", "Two", at)
	for _, su := range []*Signup{su1, su2} {
		if err := rig.repo.insertSignup(ctx, su); err != nil {
			t.Fatal(err)
		}
	}
	shrink := *a
	shrink.Capacity = 1
	if err := rig.repo.updateJob(ctx, &shrink); !errors.Is(err, ErrCapacityBelowFilled) {
		t.Fatalf("capacity guard: %v", err)
	}
	edit := *a
	edit.Title, edit.Capacity, edit.MinAge, edit.SeasonID, edit.UpdatedBy = "Renamed", 5, 0, "", "u2"
	if err := rig.repo.updateJob(ctx, &edit); err != nil {
		t.Fatal(err)
	}
	if upd, _ := rig.repo.getJob(ctx, rig.org, a.ID); upd.Title != "Renamed" || upd.Capacity != 5 || upd.MinAge != 0 || upd.SeasonID != "" || upd.UpdatedBy != "u2" {
		t.Fatalf("update not applied: %+v", upd)
	}
	badSeason := edit
	badSeason.Role = "not-a-role"
	if err := rig.repo.updateJob(ctx, &badSeason); err == nil || !strings.Contains(err.Error(), "update job") {
		t.Fatalf("role check constraint: %v", err)
	}
	if upd, _ := rig.repo.getJob(ctx, rig.org, a.ID); upd.Title != "Renamed" {
		t.Fatal("failed update must not change the row")
	}

	// deleteJob.
	if err := rig.repo.deleteJob(ctx, rig.org, "bad"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("bad id: %v", err)
	}
	if err := rig.repo.deleteJob(ctx, rig.org, uuid.NewString()); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing: %v", err)
	}
	worked := *su1
	worked.Status = StatusWorked
	if err := rig.repo.updateSignup(ctx, &worked, StatusConfirmed); err != nil {
		t.Fatal(err)
	}
	if err := rig.repo.deleteJob(ctx, rig.org, a.ID); !errors.Is(err, ErrJobHasWorkedSignups) {
		t.Fatalf("worked guard: %v", err)
	}
	if err := rig.repo.deleteJob(ctx, rig.org, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.repo.getJob(ctx, rig.org, b.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	// Deleting a job with only live (unworked) signups cascades them.
	if err := rig.repo.insertJobs(ctx, []*Job{rig.job("tmp", at, 2)}); err != nil {
		t.Fatal(err)
	}
	tmp, _ := rig.repo.listJobs(ctx, rig.org, "")
	var tmpID string
	for _, j := range tmp {
		if j.Title == "tmp" {
			tmpID = j.ID
		}
	}
	if err := rig.repo.insertSignup(ctx, rig.signup(tmpID, "hh9", "Nine", at)); err != nil {
		t.Fatal(err)
	}
	if err := rig.repo.deleteJob(ctx, rig.org, tmpID); err != nil {
		t.Fatal(err)
	}
	if left, _ := rig.repo.listSignups(ctx, rig.org, SignupFilter{JobID: tmpID}); len(left) != 0 {
		t.Fatalf("signups cascade: %+v", left)
	}

	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := rig.repo.listJobs(dead, rig.org, ""); err == nil || !strings.Contains(err.Error(), "list jobs") {
		t.Fatalf("list: %v", err)
	}
	if _, err := rig.repo.getJob(dead, rig.org, a.ID); err == nil || !strings.Contains(err.Error(), "get job") {
		t.Fatalf("get: %v", err)
	}
	if err := rig.repo.insertJobs(dead, []*Job{fresh}); err == nil || !strings.Contains(err.Error(), "begin insert jobs") {
		t.Fatalf("insert begin: %v", err)
	}
	if err := rig.repo.updateJob(dead, &edit); err == nil || !strings.Contains(err.Error(), "begin job update") {
		t.Fatalf("update begin: %v", err)
	}
	if err := rig.repo.deleteJob(dead, rig.org, a.ID); err == nil || !strings.Contains(err.Error(), "begin job delete") {
		t.Fatalf("delete begin: %v", err)
	}
}

func TestCov1xRepositorySignups(t *testing.T) {
	rig := cov1xNewRepoRig(t)
	ctx := context.Background()
	at := time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC)
	job := rig.job("Timer", at, 2)
	if err := rig.repo.insertJobs(ctx, []*Job{job}); err != nil {
		t.Fatal(err)
	}

	if _, err := rig.repo.getSignup(ctx, rig.org, "bad"); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := rig.repo.getSignup(ctx, rig.org, uuid.NewString()); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if list, err := rig.repo.listSignups(ctx, rig.org, SignupFilter{JobID: "bad"}); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("bad job filter yields an empty non-nil list: %v %v", list, err)
	}

	// Unknown job.
	if err := rig.repo.insertSignup(ctx, rig.signup(uuid.NewString(), "hh1", "X", at)); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("unknown job: %v", err)
	}
	s1 := rig.signup(job.ID, "hh1", "Ann", at)
	s1.SwimmerID = "sw-1"
	if err := rig.repo.insertSignup(ctx, s1); err != nil {
		t.Fatal(err)
	}
	if err := rig.repo.insertSignup(ctx, rig.signup(job.ID, "hh1", "ANN", at)); !errors.Is(err, ErrDuplicateSignup) {
		t.Fatalf("case-insensitive duplicate: %v", err)
	}
	// Re-using an existing primary key surfaces as a duplicate (unique violation).
	clash := rig.signup(job.ID, "hh2", "Zed", at)
	clash.ID = s1.ID
	if err := rig.repo.insertSignup(ctx, clash); !errors.Is(err, ErrDuplicateSignup) {
		t.Fatalf("pk clash: %v", err)
	}
	s2 := rig.signup(job.ID, "hh2", "Bea", at.Add(time.Minute))
	if err := rig.repo.insertSignup(ctx, s2); err != nil {
		t.Fatal(err)
	}
	if err := rig.repo.insertSignup(ctx, rig.signup(job.ID, "hh3", "Cy", at)); !errors.Is(err, ErrJobFull) {
		t.Fatalf("full: %v", err)
	}

	got, err := rig.repo.getSignup(ctx, rig.org, s1.ID)
	if err != nil || got.SwimmerID != "sw-1" || got.VolunteerName != "Ann" || got.Status != StatusConfirmed || got.HoursCredited != nil {
		t.Fatalf("round trip: %+v %v", got, err)
	}

	ids := func(f SignupFilter) []string {
		list, err := rig.repo.listSignups(ctx, rig.org, f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, su := range list {
			out = append(out, su.ID)
		}
		return out
	}
	if got := ids(SignupFilter{}); len(got) != 2 || got[0] != s1.ID || got[1] != s2.ID {
		t.Fatalf("ordered by created_at: %v", got)
	}
	if got := ids(SignupFilter{JobID: job.ID, HouseholdID: "hh2", Status: StatusConfirmed}); len(got) != 1 || got[0] != s2.ID {
		t.Fatalf("combined filter: %v", got)
	}
	if got := ids(SignupFilter{Status: StatusWorked}); len(got) != 0 {
		t.Fatalf("status filter: %v", got)
	}

	// updateSignup compare-and-swap.
	hours, now := 1.5, at.Add(time.Hour)
	done := *s1
	done.Status, done.HoursCredited, done.CheckedInAt, done.CheckedInBy, done.Notes, done.UpdatedAt = StatusWorked, &hours, &now, "coach", "great", now
	if err := rig.repo.updateSignup(ctx, &done, StatusConfirmed); err != nil {
		t.Fatal(err)
	}
	after, _ := rig.repo.getSignup(ctx, rig.org, s1.ID)
	if after.Status != StatusWorked || after.HoursCredited == nil || *after.HoursCredited != 1.5 || after.CheckedInBy != "coach" || after.Notes != "great" {
		t.Fatalf("update not applied: %+v", after)
	}
	if err := rig.repo.updateSignup(ctx, &done, StatusConfirmed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("stale expected status: %v", err)
	}
	gone := done
	gone.ID = uuid.NewString()
	if err := rig.repo.updateSignup(ctx, &gone, StatusConfirmed); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("missing row: %v", err)
	}

	// erase household.
	n, err := rig.repo.eraseHousehold(ctx, rig.org, "hh2")
	if err != nil || n != 1 {
		t.Fatalf("erase: %d %v", n, err)
	}
	if n, _ := rig.repo.eraseHousehold(ctx, rig.org, "hh2"); n != 0 {
		t.Fatalf("second erase: %d", n)
	}
	if _, err := rig.repo.getSignup(ctx, rig.org, s2.ID); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("erased: %v", err)
	}

	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := rig.repo.listSignups(dead, rig.org, SignupFilter{}); err == nil || !strings.Contains(err.Error(), "list signups") {
		t.Fatalf("list: %v", err)
	}
	if _, err := rig.repo.getSignup(dead, rig.org, s1.ID); err == nil || !strings.Contains(err.Error(), "get signup") {
		t.Fatalf("get: %v", err)
	}
	if err := rig.repo.insertSignup(dead, s1); err == nil || !strings.Contains(err.Error(), "begin signup") {
		t.Fatalf("insert: %v", err)
	}
	if err := rig.repo.updateSignup(dead, &done, StatusWorked); err == nil || !strings.Contains(err.Error(), "update signup") {
		t.Fatalf("update: %v", err)
	}
	if _, err := rig.repo.eraseHousehold(dead, rig.org, "hh1"); err == nil || !strings.Contains(err.Error(), "erase household") {
		t.Fatalf("erase: %v", err)
	}
}

func TestCov1xRepositoryRequirementsCreditsBuyouts(t *testing.T) {
	rig := cov1xNewRepoRig(t)
	ctx := context.Background()
	at := time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC)

	// Requirements: season default (NULL household) sorts before overrides; upsert updates in place.
	if _, err := rig.repo.upsertRequirement(ctx, &Requirement{ID: uuid.NewString(), OrgID: rig.org, SeasonID: "bad"}); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("bad season: %v", err)
	}
	if list, err := rig.repo.listRequirements(ctx, rig.org, "bad"); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("bad season filter: %v %v", list, err)
	}
	def, err := rig.repo.upsertRequirement(ctx, &Requirement{ID: uuid.NewString(), OrgID: rig.org, SeasonID: rig.season,
		RequiredHours: 8, BuyoutCentsPerHour: 2000, Notes: "default", UpdatedAt: at, UpdatedBy: "u1"})
	if err != nil || def.HouseholdID != "" {
		t.Fatalf("default requirement: %+v %v", def, err)
	}
	over, err := rig.repo.upsertRequirement(ctx, &Requirement{ID: uuid.NewString(), OrgID: rig.org, SeasonID: rig.season,
		HouseholdID: "hh1", RequiredHours: 2, BuyoutCentsPerHour: 1000, UpdatedAt: at})
	if err != nil || over.HouseholdID != "hh1" {
		t.Fatalf("override: %+v %v", over, err)
	}
	again, err := rig.repo.upsertRequirement(ctx, &Requirement{ID: uuid.NewString(), OrgID: rig.org, SeasonID: rig.season,
		RequiredHours: 10, BuyoutCentsPerHour: 2500, Notes: "changed", UpdatedAt: at.Add(time.Hour), UpdatedBy: "u2"})
	if err != nil || again.ID != def.ID || again.RequiredHours != 10 || again.Notes != "changed" || again.UpdatedBy != "u2" {
		t.Fatalf("upsert keeps the original row: %+v %v", again, err)
	}
	reqs, _ := rig.repo.listRequirements(ctx, rig.org, rig.season)
	if len(reqs) != 2 || reqs[0].HouseholdID != "" || reqs[1].HouseholdID != "hh1" {
		t.Fatalf("season default first: %+v", reqs)
	}
	if all, _ := rig.repo.listRequirements(ctx, rig.org, ""); len(all) != 2 {
		t.Fatalf("unfiltered: %+v", all)
	}
	if _, err := rig.repo.upsertRequirement(ctx, &Requirement{ID: uuid.NewString(), OrgID: rig.org, SeasonID: uuid.NewString()}); err == nil || !strings.Contains(err.Error(), "upsert requirement") {
		t.Fatalf("unknown season FK: %v", err)
	}
	if err := rig.repo.deleteRequirement(ctx, rig.org, "bad"); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("bad id: %v", err)
	}
	if err := rig.repo.deleteRequirement(ctx, rig.org, uuid.NewString()); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := rig.repo.deleteRequirement(ctx, rig.org, over.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := rig.repo.listRequirements(ctx, rig.org, ""); len(left) != 1 {
		t.Fatalf("after delete: %+v", left)
	}

	// Credits.
	if err := rig.repo.insertCredit(ctx, &Credit{ID: uuid.NewString(), OrgID: rig.org, SeasonID: "bad"}); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("bad season: %v", err)
	}
	if err := rig.repo.insertCredit(ctx, &Credit{ID: uuid.NewString(), OrgID: rig.org, SeasonID: uuid.NewString(), HouseholdID: "hh1", Hours: 1}); err == nil || !strings.Contains(err.Error(), "insert credit") {
		t.Fatalf("unknown season FK: %v", err)
	}
	c1 := &Credit{ID: uuid.NewString(), OrgID: rig.org, SeasonID: rig.season, HouseholdID: "hh1", Hours: 1.5, Reason: "cleanup", CreatedBy: "u1", CreatedAt: at}
	c2 := &Credit{ID: uuid.NewString(), OrgID: rig.org, SeasonID: rig.season, HouseholdID: "hh2", Hours: -1, Reason: "fix", CreatedBy: "u1", CreatedAt: at.Add(time.Minute)}
	for _, c := range []*Credit{c1, c2} {
		if err := rig.repo.insertCredit(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	credits, err := rig.repo.listCredits(ctx, rig.org, rig.season, "")
	if err != nil || len(credits) != 2 || credits[0].ID != c1.ID || credits[0].Hours != 1.5 || credits[0].Reason != "cleanup" || credits[1].Hours != -1 {
		t.Fatalf("credits: %+v %v", credits, err)
	}
	if one, _ := rig.repo.listCredits(ctx, rig.org, "", "hh2"); len(one) != 1 || one[0].ID != c2.ID {
		t.Fatalf("household filter: %+v", one)
	}
	if none, err := rig.repo.listCredits(ctx, rig.org, "bad", ""); err != nil || none == nil || len(none) != 0 {
		t.Fatalf("bad season filter: %v %v", none, err)
	}

	// Buyouts.
	mkb := func(hh string, created time.Time) *Buyout {
		return &Buyout{ID: uuid.NewString(), OrgID: rig.org, SeasonID: rig.season, HouseholdID: hh, Status: "pending",
			HoursShort: 2.5, RateCentsPerHour: 2000, AmountCents: 5000, CreatedBy: "u1", CreatedAt: created, UpdatedAt: created}
	}
	if err := rig.repo.reserveBuyout(ctx, &Buyout{ID: uuid.NewString(), OrgID: rig.org, SeasonID: "bad"}); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("bad season: %v", err)
	}
	b1, b2 := mkb("hh1", at), mkb("hh2", at.Add(time.Minute))
	if err := rig.repo.reserveBuyout(ctx, b2); err != nil {
		t.Fatal(err)
	}
	if err := rig.repo.reserveBuyout(ctx, b1); err != nil {
		t.Fatal(err)
	}
	if err := rig.repo.reserveBuyout(ctx, mkb("hh1", at)); !errors.Is(err, ErrBuyoutExists) {
		t.Fatalf("one buyout per household per season: %v", err)
	}
	invalid := mkb("hh7", at)
	invalid.AmountCents = 0
	if err := rig.repo.reserveBuyout(ctx, invalid); err == nil || !strings.Contains(err.Error(), "reserve buyout") {
		t.Fatalf("check constraint: %v", err)
	}
	buyouts, err := rig.repo.listBuyouts(ctx, rig.org, rig.season)
	if err != nil || len(buyouts) != 2 || buyouts[0].ID != b1.ID || buyouts[0].HoursShort != 2.5 || buyouts[0].AmountCents != 5000 {
		t.Fatalf("buyouts: %+v %v", buyouts, err)
	}
	if all, _ := rig.repo.listBuyouts(ctx, rig.org, ""); len(all) != 2 {
		t.Fatalf("unfiltered: %+v", all)
	}
	if none, err := rig.repo.listBuyouts(ctx, rig.org, "bad"); err != nil || none == nil || len(none) != 0 {
		t.Fatalf("bad season filter: %v %v", none, err)
	}
	billed := *b1
	billed.Status, billed.InvoiceID, billed.InvoiceItemID, billed.UpdatedAt = "billed", "inv-9", "item-9", at.Add(time.Hour)
	if err := rig.repo.updateBuyout(ctx, &billed); err != nil {
		t.Fatal(err)
	}
	after, _ := rig.repo.listBuyouts(ctx, rig.org, rig.season)
	if after[0].Status != "billed" || after[0].InvoiceID != "inv-9" || after[0].InvoiceItemID != "item-9" {
		t.Fatalf("update not applied: %+v", after[0])
	}
	ghost := billed
	ghost.ID = uuid.NewString()
	if err := rig.repo.updateBuyout(ctx, &ghost); !errors.Is(err, ErrBuyoutNotFound) {
		t.Fatalf("missing: %v", err)
	}
	broken := billed
	broken.Status = "bogus"
	if err := rig.repo.updateBuyout(ctx, &broken); err == nil || !strings.Contains(err.Error(), "update buyout") {
		t.Fatalf("check constraint: %v", err)
	}
	if err := rig.repo.deleteBuyout(ctx, rig.org, b1.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := rig.repo.listBuyouts(ctx, rig.org, ""); len(left) != 1 || left[0].ID != b2.ID {
		t.Fatalf("after delete: %+v", left)
	}
	// Another org cannot delete this org's reservation.
	if err := rig.repo.deleteBuyout(ctx, cov1xOtherOrg, b2.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := rig.repo.listBuyouts(ctx, rig.org, ""); len(left) != 1 {
		t.Fatalf("cross-org delete must be a no-op: %+v", left)
	}

	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := rig.repo.listRequirements(dead, rig.org, ""); err == nil || !strings.Contains(err.Error(), "list requirements") {
		t.Fatalf("list requirements: %v", err)
	}
	if err := rig.repo.deleteRequirement(dead, rig.org, def.ID); err == nil || !strings.Contains(err.Error(), "delete requirement") {
		t.Fatalf("delete requirement: %v", err)
	}
	if _, err := rig.repo.listCredits(dead, rig.org, "", ""); err == nil || !strings.Contains(err.Error(), "list credits") {
		t.Fatalf("list credits: %v", err)
	}
	if _, err := rig.repo.listBuyouts(dead, rig.org, ""); err == nil || !strings.Contains(err.Error(), "list buyouts") {
		t.Fatalf("list buyouts: %v", err)
	}
	if err := rig.repo.updateBuyout(dead, &billed); err == nil || !strings.Contains(err.Error(), "update buyout") {
		t.Fatalf("update buyout: %v", err)
	}
	if err := rig.repo.deleteBuyout(dead, rig.org, b2.ID); err == nil || !strings.Contains(err.Error(), "delete buyout") {
		t.Fatalf("delete buyout: %v", err)
	}
}

// The service layered over the real repository behaves like the in-memory one.
func TestCov1xServiceOverPostgres(t *testing.T) {
	s := NewServiceWithPool(cov1xNewRepoRig(t).pool)
	s.SetClock(func() time.Time { return baseNow })
	org := uuid.NewString()
	p := admin()
	p.OrgID = org
	ctx := context.Background()
	job, err := s.CreateJob(ctx, p, org, JobInput{EventName: "Evt", Role: "timer", Title: "Timer", StartsAt: baseNow.Add(72 * time.Hour),
		EndsAt: baseNow.Add(75 * time.Hour), Capacity: 1})
	if err != nil || job.HourCredit != 3 {
		t.Fatalf("create: %+v %v", job, err)
	}
	par := parent(householdA)
	par.OrgID = org
	if _, err := s.SignUp(ctx, par, org, job.ID, SignupInput{VolunteerName: "Pat"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignUp(ctx, par, org, job.ID, SignupInput{VolunteerName: "Quinn"}); !errors.Is(err, ErrJobFull) {
		t.Fatalf("capacity through the service: %v", err)
	}
}

const cov1xOtherOrg = "4f0c5f0e-0000-4000-8000-00000000000b"

func cov1xExec(t *testing.T, pool *pgxpool.Pool, stmts ...string) {
	t.Helper()
	for _, q := range stmts {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// The transactional write paths report failures from each step. The faults are
// injected as DDL in the test's private throwaway schema (a deferred trigger
// that fails at COMMIT, a renamed table or column, a failing delete trigger).
func TestCov1xRepositoryTransactionFaults(t *testing.T) {
	rig := cov1xNewRepoRig(t)
	ctx := context.Background()
	at := time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC)
	job := rig.job("Timer", at, 3)
	other := rig.job("Other", at.Add(time.Hour), 3)
	if err := rig.repo.insertJobs(ctx, []*Job{job, other}); err != nil {
		t.Fatal(err)
	}
	edit := *job
	edit.Title = "Edited"
	cov1xExec(t, rig.pool, `CREATE FUNCTION cov1x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x injected failure'; END $$`)

	// Failure at COMMIT.
	cov1xExec(t, rig.pool,
		`CREATE CONSTRAINT TRIGGER cov1x_jobs_commit AFTER INSERT OR UPDATE OR DELETE ON volunteer_jobs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cov1x_fail()`,
		`CREATE CONSTRAINT TRIGGER cov1x_signups_commit AFTER INSERT ON volunteer_signups DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cov1x_fail()`)
	if err := rig.repo.insertJobs(ctx, []*Job{rig.job("New", at, 1)}); err == nil || !strings.Contains(err.Error(), "commit jobs") {
		t.Fatalf("commit jobs: %v", err)
	}
	if err := rig.repo.updateJob(ctx, &edit); err == nil || !strings.Contains(err.Error(), "commit job update") {
		t.Fatalf("commit job update: %v", err)
	}
	if err := rig.repo.deleteJob(ctx, rig.org, other.ID); err == nil || !strings.Contains(err.Error(), "commit job delete") {
		t.Fatalf("commit job delete: %v", err)
	}
	if err := rig.repo.insertSignup(ctx, rig.signup(job.ID, "hh1", "Ann", at)); err == nil || !strings.Contains(err.Error(), "commit signup") {
		t.Fatalf("commit signup: %v", err)
	}
	cov1xExec(t, rig.pool, `DROP TRIGGER cov1x_jobs_commit ON volunteer_jobs`, `DROP TRIGGER cov1x_signups_commit ON volunteer_signups`)
	if got, _ := rig.repo.getJob(ctx, rig.org, job.ID); got.Title != "Timer" {
		t.Fatalf("failed commit must not persist the edit: %+v", got)
	}
	if got, _ := rig.repo.listSignups(ctx, rig.org, SignupFilter{}); len(got) != 0 {
		t.Fatalf("failed commit must not persist the signup: %+v", got)
	}

	// The DELETE statement itself fails.
	cov1xExec(t, rig.pool, `CREATE TRIGGER cov1x_jobs_delete BEFORE DELETE ON volunteer_jobs FOR EACH ROW EXECUTE FUNCTION cov1x_fail()`)
	if err := rig.repo.deleteJob(ctx, rig.org, other.ID); err == nil || !strings.Contains(err.Error(), "delete job") {
		t.Fatalf("delete statement: %v", err)
	}
	cov1xExec(t, rig.pool, `DROP TRIGGER cov1x_jobs_delete ON volunteer_jobs`)

	// The INSERT statement itself fails (not a unique violation).
	cov1xExec(t, rig.pool, `CREATE TRIGGER cov1x_signups_insert BEFORE INSERT ON volunteer_signups FOR EACH ROW EXECUTE FUNCTION cov1x_fail()`)
	if err := rig.repo.insertSignup(ctx, rig.signup(job.ID, "hh1", "Ann", at)); err == nil || !strings.Contains(err.Error(), "insert signup") || errors.Is(err, ErrDuplicateSignup) {
		t.Fatalf("insert statement: %v", err)
	}
	cov1xExec(t, rig.pool, `DROP TRIGGER cov1x_signups_insert ON volunteer_signups`)

	// The signup job column is unavailable after the job row was locked.
	cov1xExec(t, rig.pool, `ALTER TABLE volunteer_signups RENAME COLUMN job_id TO job_id_moved`)
	if err := rig.repo.updateJob(ctx, &edit); err == nil || !strings.Contains(err.Error(), "count signups") {
		t.Fatalf("update count: %v", err)
	}
	if err := rig.repo.insertSignup(ctx, rig.signup(job.ID, "hh1", "Ann", at)); err == nil || !strings.Contains(err.Error(), "count signups") {
		t.Fatalf("insert count: %v", err)
	}
	if err := rig.repo.deleteJob(ctx, rig.org, other.ID); err == nil || !strings.Contains(err.Error(), "lock signups for delete") {
		t.Fatalf("delete lock signups: %v", err)
	}
	cov1xExec(t, rig.pool, `ALTER TABLE volunteer_signups RENAME COLUMN job_id_moved TO job_id`)

	// The job row cannot be locked.
	cov1xExec(t, rig.pool, `ALTER TABLE volunteer_jobs RENAME COLUMN org_id TO org_id_moved`)
	if err := rig.repo.updateJob(ctx, &edit); err == nil || !strings.Contains(err.Error(), "lock job") {
		t.Fatalf("update lock: %v", err)
	}
	if err := rig.repo.deleteJob(ctx, rig.org, other.ID); err == nil || !strings.Contains(err.Error(), "lock job for delete") {
		t.Fatalf("delete lock: %v", err)
	}
	if err := rig.repo.insertSignup(ctx, rig.signup(job.ID, "hh1", "Ann", at)); err == nil || !strings.Contains(err.Error(), "lock job") {
		t.Fatalf("insert lock: %v", err)
	}
}

func TestCov1xNewServiceDefaultClockIsUTCNow(t *testing.T) {
	s := NewService()
	before := time.Now()
	got := s.now()
	if got.Location() != time.UTC || got.Before(before.Add(-time.Second)) || got.After(time.Now().Add(time.Second)) {
		t.Fatalf("default clock should be current UTC time, got %v", got)
	}
}
