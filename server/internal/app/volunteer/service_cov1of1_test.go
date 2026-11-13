package volunteer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/workers"
)

const (
	cov1xSeason2 = "22222222-2222-2222-2222-222222222222"
	cov1xHHC     = "hh-c"
	cov1xHHD     = "hh-d"
	cov1xHHE     = "hh-e"
	cov1xHHF     = "hh-f"
)

var errCov1xBoom = errors.New("cov1x boom")

// cov1xFailStore delegates to a real store but fails any method whose name
// has been armed, so service error propagation can be asserted per call site.
type cov1xFailStore struct {
	store
	mu   sync.Mutex
	fail map[string]error
}

func cov1xWrap(s *Service) *cov1xFailStore {
	fs := &cov1xFailStore{store: s.store, fail: map[string]error{}}
	s.store = fs
	return fs
}

func (f *cov1xFailStore) arm(names ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range names {
		f.fail[n] = errCov1xBoom
	}
}

func (f *cov1xFailStore) armWith(name string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[name] = err
}

func (f *cov1xFailStore) disarm() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = map[string]error{}
}

func (f *cov1xFailStore) failing(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fail[name]
}

func (f *cov1xFailStore) getSettings(ctx context.Context, orgID string) (*Settings, bool, error) {
	if err := f.failing("getSettings"); err != nil {
		return nil, false, err
	}
	return f.store.getSettings(ctx, orgID)
}
func (f *cov1xFailStore) upsertSettings(ctx context.Context, st *Settings) error {
	if err := f.failing("upsertSettings"); err != nil {
		return err
	}
	return f.store.upsertSettings(ctx, st)
}
func (f *cov1xFailStore) listJobs(ctx context.Context, orgID, meetID string) ([]*Job, error) {
	if err := f.failing("listJobs"); err != nil {
		return nil, err
	}
	return f.store.listJobs(ctx, orgID, meetID)
}
func (f *cov1xFailStore) getJob(ctx context.Context, orgID, id string) (*Job, error) {
	if err := f.failing("getJob"); err != nil {
		return nil, err
	}
	return f.store.getJob(ctx, orgID, id)
}
func (f *cov1xFailStore) insertJobs(ctx context.Context, jobs []*Job) error {
	if err := f.failing("insertJobs"); err != nil {
		return err
	}
	return f.store.insertJobs(ctx, jobs)
}
func (f *cov1xFailStore) updateJob(ctx context.Context, job *Job) error {
	if err := f.failing("updateJob"); err != nil {
		return err
	}
	return f.store.updateJob(ctx, job)
}
func (f *cov1xFailStore) deleteJob(ctx context.Context, orgID, id string) error {
	if err := f.failing("deleteJob"); err != nil {
		return err
	}
	return f.store.deleteJob(ctx, orgID, id)
}
func (f *cov1xFailStore) listSignups(ctx context.Context, orgID string, fl SignupFilter) ([]*Signup, error) {
	if err := f.failing("listSignups"); err != nil {
		return nil, err
	}
	return f.store.listSignups(ctx, orgID, fl)
}
func (f *cov1xFailStore) getSignup(ctx context.Context, orgID, id string) (*Signup, error) {
	if err := f.failing("getSignup"); err != nil {
		return nil, err
	}
	return f.store.getSignup(ctx, orgID, id)
}
func (f *cov1xFailStore) insertSignup(ctx context.Context, su *Signup) error {
	if err := f.failing("insertSignup"); err != nil {
		return err
	}
	return f.store.insertSignup(ctx, su)
}
func (f *cov1xFailStore) updateSignup(ctx context.Context, su *Signup, expected string) error {
	if err := f.failing("updateSignup"); err != nil {
		return err
	}
	return f.store.updateSignup(ctx, su, expected)
}
func (f *cov1xFailStore) listRequirements(ctx context.Context, orgID, seasonID string) ([]*Requirement, error) {
	if err := f.failing("listRequirements"); err != nil {
		return nil, err
	}
	return f.store.listRequirements(ctx, orgID, seasonID)
}
func (f *cov1xFailStore) upsertRequirement(ctx context.Context, r *Requirement) (*Requirement, error) {
	if err := f.failing("upsertRequirement"); err != nil {
		return nil, err
	}
	return f.store.upsertRequirement(ctx, r)
}
func (f *cov1xFailStore) deleteRequirement(ctx context.Context, orgID, id string) error {
	if err := f.failing("deleteRequirement"); err != nil {
		return err
	}
	return f.store.deleteRequirement(ctx, orgID, id)
}
func (f *cov1xFailStore) listCredits(ctx context.Context, orgID, seasonID, householdID string) ([]*Credit, error) {
	if err := f.failing("listCredits"); err != nil {
		return nil, err
	}
	return f.store.listCredits(ctx, orgID, seasonID, householdID)
}
func (f *cov1xFailStore) insertCredit(ctx context.Context, c *Credit) error {
	if err := f.failing("insertCredit"); err != nil {
		return err
	}
	return f.store.insertCredit(ctx, c)
}
func (f *cov1xFailStore) listBuyouts(ctx context.Context, orgID, seasonID string) ([]*Buyout, error) {
	if err := f.failing("listBuyouts"); err != nil {
		return nil, err
	}
	return f.store.listBuyouts(ctx, orgID, seasonID)
}
func (f *cov1xFailStore) reserveBuyout(ctx context.Context, b *Buyout) error {
	if err := f.failing("reserveBuyout"); err != nil {
		return err
	}
	return f.store.reserveBuyout(ctx, b)
}
func (f *cov1xFailStore) updateBuyout(ctx context.Context, b *Buyout) error {
	if err := f.failing("updateBuyout"); err != nil {
		return err
	}
	return f.store.updateBuyout(ctx, b)
}
func (f *cov1xFailStore) deleteBuyout(ctx context.Context, orgID, id string) error {
	if err := f.failing("deleteBuyout"); err != nil {
		return err
	}
	return f.store.deleteBuyout(ctx, orgID, id)
}
func (f *cov1xFailStore) eraseHousehold(ctx context.Context, orgID, householdID string) (int, error) {
	if err := f.failing("eraseHousehold"); err != nil {
		return 0, err
	}
	return f.store.eraseHousehold(ctx, orgID, householdID)
}

// cov1xQueue records enqueued reminder tasks and can be told to fail.
type cov1xQueue struct {
	mu    sync.Mutex
	tasks []*workers.Task
	delay []time.Duration
	err   error
}

func (q *cov1xQueue) EnqueueIn(_ context.Context, d time.Duration, t *workers.Task, _ ...workers.Option) (*workers.TaskInfo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return nil, q.err
	}
	q.tasks = append(q.tasks, t)
	q.delay = append(q.delay, d)
	return &workers.TaskInfo{ID: fmt.Sprint(len(q.tasks))}, nil
}

// cov1xService builds the standard test service with a failure-injecting store,
// a failing-capable queue, a log buffer and a movable clock.
type cov1xRig struct {
	s      *Service
	fs     *cov1xFailStore
	q      *cov1xQueue
	biller *fakeBiller
	sent   *[]ReminderEmail
	logs   *bytes.Buffer
	now    *time.Time
}

func cov1xNewRig(t *testing.T) *cov1xRig {
	t.Helper()
	s, biller, _, sent := newTestService(t)
	now := baseNow
	s.SetClock(func() time.Time { return now })
	r := &cov1xRig{s: s, biller: biller, sent: sent, logs: &bytes.Buffer{}, now: &now}
	s.SetLogger(slog.New(slog.NewTextHandler(r.logs, nil)))
	r.q = &cov1xQueue{}
	s.SetReminderQueue(r.q)
	r.fs = cov1xWrap(s)
	return r
}

// cov1xBigDirectory installs two seasons and six households (one inactive).
func cov1xBigDirectory(s *Service) {
	s.SetSeasonLookup(func(string) ([]SeasonRef, error) {
		return []SeasonRef{
			{ID: cov1xSeason2, Name: "2024-25", Status: "completed", StartDate: "2024-09-01", EndDate: "2025-08-31"},
			{ID: testSeason, Name: "2025-26", Status: "active", StartDate: "2025-09-01", EndDate: "2026-08-31"},
		}, nil
	})
	s.SetHouseholdDirectory(func(string) ([]HouseholdRef, error) {
		return []HouseholdRef{
			{ID: householdA, Name: "Alpha", Email: "a@example.com", Status: "active"},
			{ID: householdB, Name: "Bravo", Email: "b@example.com", Status: "active"},
			{ID: cov1xHHC, Name: "Charlie", Email: "c@example.com", Status: "inactive"},
			{ID: cov1xHHD, Name: "Delta", Email: "  ", Status: ""},
			{ID: cov1xHHE, Name: "Echo", Email: "e@example.com", Status: "active"},
			{ID: cov1xHHF, Name: "Foxtrot", Email: "f@example.com", Status: "active"},
		}, nil
	})
}

func cov1xSignUp(t *testing.T, s *Service, hh, jobID, name string) *SignupView {
	t.Helper()
	v, err := s.SignUp(context.Background(), parent(hh), testOrg, jobID, SignupInput{VolunteerName: name})
	if err != nil {
		t.Fatalf("signup %s: %v", name, err)
	}
	return v
}

func cov1xIsValidation(err error, field string) bool {
	var ve *ValidationError
	return errors.As(err, &ve) && ve.Field == field
}

// ─── Role helpers and scope ───────────────────────────────────────────────────

func TestCov1xRoleHelpersAndScope(t *testing.T) {
	if hasRole(nil, "admin") {
		t.Fatal("nil principal must have no roles")
	}
	if !hasRole(&appmiddleware.Principal{Roles: []string{" Admin "}}, "admin") {
		t.Fatal("role match must ignore case and whitespace")
	}
	if hasRole(&appmiddleware.Principal{Roles: []string{"parent"}}, "admin", "coach") {
		t.Fatal("parent must not match staff roles")
	}
	if hh, err := householdScope(admin()); err != nil || hh != "" {
		t.Fatalf("staff scope: %q %v", hh, err)
	}
	if hh, err := householdScope(&appmiddleware.Principal{Roles: []string{"parent"}, HouseholdID: " hh-x "}); err != nil || hh != "hh-x" {
		t.Fatalf("parent scope: %q %v", hh, err)
	}
	if _, err := householdScope(&appmiddleware.Principal{Roles: []string{"parent"}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent without household: %v", err)
	}
	if _, err := householdScope(&appmiddleware.Principal{Roles: []string{"viewer"}, HouseholdID: "hh"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unknown role: %v", err)
	}
	if _, err := householdScope(nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil principal: %v", err)
	}

	s, _, _, _ := newTestService(t)
	ctx := context.Background()
	nobody := &appmiddleware.Principal{UserID: "x", Roles: []string{"viewer"}}
	if _, err := s.ListJobs(ctx, nobody, testOrg, JobFilter{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListJobs: %v", err)
	}
	if _, err := s.GetJob(ctx, nobody, testOrg, "x"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetJob: %v", err)
	}
	if _, err := s.ListSignups(ctx, nobody, testOrg, SignupFilter{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListSignups: %v", err)
	}
	if _, err := s.ListCredits(ctx, nobody, testOrg, "", ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListCredits: %v", err)
	}
	if _, err := s.HouseholdLedger(ctx, nobody, testOrg, householdA); !errors.Is(err, ErrForbidden) {
		t.Fatalf("HouseholdLedger: %v", err)
	}
	if _, err := s.GetSettings(ctx, parent(householdA), testOrg); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetSettings: %v", err)
	}
	if _, err := s.ListRequirements(ctx, parent(householdA), testOrg, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListRequirements: %v", err)
	}
	if err := s.DeleteRequirement(ctx, coach(), testOrg, "x"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("DeleteRequirement: %v", err)
	}
	if _, err := s.AddCredit(ctx, parent(householdA), testOrg, CreditInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("AddCredit: %v", err)
	}
	if _, err := s.SeasonLedger(ctx, parent(householdA), testOrg, testSeason); !errors.Is(err, ErrForbidden) {
		t.Fatalf("SeasonLedger: %v", err)
	}
	if _, err := s.ListBuyouts(ctx, coach(), testOrg, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListBuyouts: %v", err)
	}
	if _, _, err := s.UpdateJob(ctx, parent(householdA), testOrg, "x", JobUpdate{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("UpdateJob: %v", err)
	}
	if _, err := s.DeleteJob(ctx, parent(householdA), testOrg, "x"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("DeleteJob: %v", err)
	}
	if _, err := s.BulkCreateJobs(ctx, parent(householdA), testOrg, BulkInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("BulkCreateJobs: %v", err)
	}
}

func TestCov1xNewServiceWithNilPoolIsInMemory(t *testing.T) {
	s := NewServiceWithPool(nil)
	if _, ok := s.store.(*memoryStore); !ok {
		t.Fatalf("nil pool must keep the in-memory store, got %T", s.store)
	}
	var logs bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&logs, nil))
	s.SetLogger(custom)
	if s.logger != custom {
		t.Fatal("SetLogger must install a non-nil logger")
	}
	s.SetLogger(nil)
	if s.logger != custom {
		t.Fatal("SetLogger(nil) must keep the current logger")
	}
	if (&ValidationError{Field: "f", Message: "m"}).Error() != "f: m" {
		t.Fatal("ValidationError text")
	}
}

// ─── Settings ─────────────────────────────────────────────────────────────────

func TestCov1xSettingsUpdateValidationAndRoundTrip(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()

	def, err := s.GetSettings(ctx, coach(), testOrg)
	if err != nil || def.CancellationCutoffHours != 48 || !def.RemindersEnabled || def.Timezone != "UTC" || def.UpdatedAt != nil {
		t.Fatalf("defaults: %+v %v", def, err)
	}
	if _, err := s.UpdateSettings(ctx, coach(), testOrg, SettingsUpdate{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach update: %v", err)
	}

	neg, big, negC, bigC, bigCut, negCut := -1.0, 1000.5, -1, 100001, 721, -1
	blank, local, bogus := "  ", "Local", "Not/AZone"
	cases := []struct {
		name  string
		in    SettingsUpdate
		field string
	}{
		{"hours negative", SettingsUpdate{DefaultRequiredHours: &neg}, "default_required_hours"},
		{"hours too big", SettingsUpdate{DefaultRequiredHours: &big}, "default_required_hours"},
		{"cents negative", SettingsUpdate{DefaultBuyoutCentsPerHour: &negC}, "default_buyout_cents_per_hour"},
		{"cents too big", SettingsUpdate{DefaultBuyoutCentsPerHour: &bigC}, "default_buyout_cents_per_hour"},
		{"cutoff negative", SettingsUpdate{CancellationCutoffHours: &negCut}, "cancellation_cutoff_hours"},
		{"cutoff too big", SettingsUpdate{CancellationCutoffHours: &bigCut}, "cancellation_cutoff_hours"},
		{"tz blank", SettingsUpdate{Timezone: &blank}, "timezone"},
		{"tz Local", SettingsUpdate{Timezone: &local}, "timezone"},
		{"tz bogus", SettingsUpdate{Timezone: &bogus}, "timezone"},
	}
	for _, c := range cases {
		if _, err := s.UpdateSettings(ctx, admin(), testOrg, c.in); !cov1xIsValidation(err, c.field) {
			t.Fatalf("%s: want validation error on %s, got %v", c.name, c.field, err)
		}
	}
	// Nothing was persisted by the rejected updates.
	if got, _ := s.GetSettings(ctx, admin(), testOrg); got.UpdatedAt != nil {
		t.Fatalf("rejected updates must not persist: %+v", got)
	}

	hours, cents, cutoff, off, tz := 12.346, 2750, 72, false, " America/Los_Angeles "
	got, err := s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{
		DefaultRequiredHours: &hours, DefaultBuyoutCentsPerHour: &cents, CancellationCutoffHours: &cutoff,
		RemindersEnabled: &off, Timezone: &tz,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultRequiredHours != 12.35 || got.DefaultBuyoutCentsPerHour != 2750 || got.CancellationCutoffHours != 72 ||
		got.RemindersEnabled || got.Timezone != "America/Los_Angeles" || got.UpdatedBy != "u-admin" || got.UpdatedAt == nil || !got.UpdatedAt.Equal(baseNow) {
		t.Fatalf("updated settings: %+v", got)
	}
	// A partial update keeps the other fields.
	on := true
	again, err := s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{RemindersEnabled: &on})
	if err != nil || !again.RemindersEnabled || again.DefaultRequiredHours != 12.35 || again.Timezone != "America/Los_Angeles" {
		t.Fatalf("partial update: %+v %v", again, err)
	}
	reread, _ := s.GetSettings(ctx, admin(), testOrg)
	if *reread.UpdatedAt != *again.UpdatedAt || reread.CancellationCutoffHours != 72 {
		t.Fatalf("reread: %+v", reread)
	}

	r.fs.arm("getSettings")
	if _, err := s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("settings read failure: %v", err)
	}
	if _, err := s.GetSettings(ctx, admin(), testOrg); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("GetSettings read failure: %v", err)
	}
	r.fs.disarm()
	r.fs.arm("upsertSettings")
	if _, err := s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{RemindersEnabled: &off}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("settings write failure: %v", err)
	}
}

// ─── Job validation, create, update, delete ───────────────────────────────────

func TestCov1xCreateJobValidationTable(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	base := func() JobInput {
		return JobInput{EventName: "Clinic", Role: "timer", Title: "T", StartsAt: baseNow.Add(72 * time.Hour), EndsAt: baseNow.Add(75 * time.Hour), Capacity: 2}
	}
	neg, over := -0.5, 24.5
	cases := []struct {
		name   string
		mutate func(*JobInput)
		field  string
	}{
		{"no anchor", func(in *JobInput) { in.EventName = "  " }, "event_name"},
		{"event name too long", func(in *JobInput) { in.EventName = strings.Repeat("e", 161) }, "event_name"},
		{"bad role", func(in *JobInput) { in.Role = "janitor" }, "role"},
		{"empty title", func(in *JobInput) { in.Title = " " }, "title"},
		{"long title", func(in *JobInput) { in.Title = strings.Repeat("t", 121) }, "title"},
		{"long description", func(in *JobInput) { in.Description = strings.Repeat("d", 2001) }, "description"},
		{"long location", func(in *JobInput) { in.Location = strings.Repeat("l", 201) }, "location"},
		{"zero start", func(in *JobInput) { in.StartsAt = time.Time{} }, "starts_at"},
		{"zero end", func(in *JobInput) { in.EndsAt = time.Time{} }, "starts_at"},
		{"end not after start", func(in *JobInput) { in.EndsAt = in.StartsAt }, "ends_at"},
		{"shift over 24h", func(in *JobInput) { in.EndsAt = in.StartsAt.Add(25 * time.Hour) }, "ends_at"},
		{"capacity zero", func(in *JobInput) { in.Capacity = 0 }, "capacity"},
		{"capacity 501", func(in *JobInput) { in.Capacity = 501 }, "capacity"},
		{"credit negative", func(in *JobInput) { in.HourCredit = &neg }, "hour_credit"},
		{"credit over 24", func(in *JobInput) { in.HourCredit = &over }, "hour_credit"},
		{"min age negative", func(in *JobInput) { in.MinAge = -1 }, "min_age"},
		{"min age 100", func(in *JobInput) { in.MinAge = 100 }, "min_age"},
	}
	for _, c := range cases {
		in := base()
		c.mutate(&in)
		if _, err := s.CreateJob(ctx, coach(), testOrg, in); !cov1xIsValidation(err, c.field) {
			t.Fatalf("%s: want validation error on %s, got %v", c.name, c.field, err)
		}
	}
	if jobs, _ := r.fs.store.listJobs(ctx, testOrg, ""); len(jobs) != 0 {
		t.Fatalf("rejected jobs must not be stored: %d", len(jobs))
	}

	// Normalisation: role case/whitespace, trimmed text, derived credit.
	in := base()
	in.Role, in.Title, in.Description, in.Location = " TIMER ", "  Lane 1  ", " d ", " pool "
	job, err := s.CreateJob(ctx, admin(), testOrg, in)
	if err != nil {
		t.Fatal(err)
	}
	if job.Role != "timer" || job.Title != "Lane 1" || job.Description != "d" || job.Location != "pool" || job.HourCredit != 3 || job.CreatedBy != "u-admin" {
		t.Fatalf("normalised job: %+v", job)
	}

	// Meet and season references.
	in = base()
	in.MeetID = "meet-missing"
	if _, err := s.CreateJob(ctx, admin(), testOrg, in); !errors.Is(err, ErrMeetNotFound) {
		t.Fatalf("unknown meet: %v", err)
	}
	in.MeetID = "meet-1"
	in.SeasonID = "99999999-9999-9999-9999-999999999999"
	if _, err := s.CreateJob(ctx, admin(), testOrg, in); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("unknown season: %v", err)
	}
	in.SeasonID = " " + testSeason + " "
	if j, err := s.CreateJob(ctx, admin(), testOrg, in); err != nil || j.SeasonID != testSeason {
		t.Fatalf("known season: %+v %v", j, err)
	}
	s.SetSeasonLookup(func(string) ([]SeasonRef, error) { return nil, errCov1xBoom })
	if _, err := s.CreateJob(ctx, admin(), testOrg, in); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("season lookup failure must read as not found: %v", err)
	}
	s.SetSeasonLookup(nil)
	in.SeasonID = "anything"
	if j, err := s.CreateJob(ctx, admin(), testOrg, in); err != nil || j.SeasonID != "anything" {
		t.Fatalf("without a season directory any season id is accepted: %+v %v", j, err)
	}
	s.SetMeetLookup(nil)
	in.MeetID = "no-lookup-meet"
	if _, err := s.CreateJob(ctx, admin(), testOrg, in); err != nil {
		t.Fatalf("without a meet directory any meet id is accepted: %v", err)
	}

	r.fs.arm("insertJobs")
	if _, err := s.CreateJob(ctx, admin(), testOrg, base()); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("insert failure: %v", err)
	}
}

func TestCov1xUpdateJob(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	cov1xBigDirectory(s)

	job := mustJob(t, s, 2, 96*time.Hour) // 3h derived credit, meet-1
	a := cov1xSignUp(t, s, householdA, job.ID, "Ann")
	cov1xSignUp(t, s, householdB, job.ID, "Bea")
	if _, _, err := s.Cancel(ctx, coach(), testOrg, a.ID, "swap"); err != nil {
		t.Fatal(err)
	}
	cov1xSignUp(t, s, householdA, job.ID, "Cat") // a fresh confirmed spot in the freed seat
	baseline := len(r.q.tasks)

	if _, _, err := s.UpdateJob(ctx, coach(), testOrg, "missing", JobUpdate{}); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing job: %v", err)
	}

	// Extending the end re-derives a derived credit and does not reschedule reminders.
	newEnd := job.EndsAt.Add(time.Hour)
	before, after, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{EndsAt: &newEnd})
	if err != nil {
		t.Fatal(err)
	}
	if before.HourCredit != 3 || after.HourCredit != 4 || !after.EndsAt.Equal(newEnd) || before.EndsAt.Equal(after.EndsAt) {
		t.Fatalf("derived credit: before=%+v after=%+v", before, after)
	}
	if len(r.q.tasks) != baseline {
		t.Fatalf("an unchanged start must not reschedule reminders (%d -> %d)", baseline, len(r.q.tasks))
	}

	// Moving the start reschedules reminders for confirmed volunteers only.
	newStart := job.StartsAt.Add(2 * time.Hour)
	_, moved, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{StartsAt: &newStart})
	if err != nil {
		t.Fatal(err)
	}
	if moved.HourCredit != 2 {
		t.Fatalf("credit re-derived from the new window: %v", moved.HourCredit)
	}
	if len(r.q.tasks) != baseline+2 {
		t.Fatalf("want a reminder for each of the 2 confirmed signups, got %d new tasks", len(r.q.tasks)-baseline)
	}
	var p ShiftReminderPayload
	last := r.q.tasks[len(r.q.tasks)-1]
	if last.Type != TaskTypeShiftReminder {
		t.Fatalf("task type %q", last.Type)
	}
	if err := json.Unmarshal(last.Payload, &p); err != nil || p.StartsAtUnix != newStart.Unix() || p.OrgID != testOrg {
		t.Fatalf("payload: %+v %v", p, err)
	}

	// An explicit credit is kept when the window changes.
	credit := 1.5
	if _, upd, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{HourCredit: &credit}); err != nil || upd.HourCredit != 1.5 {
		t.Fatalf("explicit credit: %+v %v", upd, err)
	}
	later := newEnd.Add(time.Hour)
	if _, upd, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{EndsAt: &later}); err != nil || upd.HourCredit != 1.5 {
		t.Fatalf("explicit credit survives a window change: %+v %v", upd, err)
	}

	// Every writable field.
	ev, season, role, title, desc, loc := "Gala", testSeason, "setup", "Setup crew", "Arrive early", "Deck"
	cap5, minAge, cert := 5, 12, true
	_, full, err := s.UpdateJob(ctx, admin(), testOrg, job.ID, JobUpdate{
		EventName: &ev, SeasonID: &season, Role: &role, Title: &title, Description: &desc, Location: &loc,
		Capacity: &cap5, MinAge: &minAge, RequiresCertification: &cert,
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := r.fs.store.getJob(ctx, testOrg, job.ID)
	for _, got := range []*Job{full, stored} {
		if got.EventName != "Gala" || got.SeasonID != testSeason || got.Role != "setup" || got.Title != "Setup crew" ||
			got.Description != "Arrive early" || got.Location != "Deck" || got.Capacity != 5 || got.MinAge != 12 ||
			!got.RequiresCertification || got.MeetID != "meet-1" || got.UpdatedBy != "u-admin" {
			t.Fatalf("updated job: %+v", got)
		}
	}

	// Rejections leave the job untouched.
	empty := " "
	if _, _, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{Title: &empty}); !cov1xIsValidation(err, "title") {
		t.Fatalf("blank title: %v", err)
	}
	one := 1
	if _, _, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{Capacity: &one}); !errors.Is(err, ErrCapacityBelowFilled) {
		t.Fatalf("capacity below filled spots: %v", err)
	}
	if now, _ := r.fs.store.getJob(ctx, testOrg, job.ID); now.Title != "Setup crew" || now.Capacity != 5 {
		t.Fatalf("rejected update changed the job: %+v", now)
	}

	for _, name := range []string{"getJob", "listSignups", "updateJob"} {
		r.fs.disarm()
		r.fs.arm(name)
		if _, _, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{Title: &title}); !errors.Is(err, errCov1xBoom) {
			t.Fatalf("%s failure: %v", name, err)
		}
	}
}

func TestCov1xDeleteJob(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()

	if _, err := s.DeleteJob(ctx, coach(), testOrg, "missing"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing job: %v", err)
	}
	job := mustJob(t, s, 3, 96*time.Hour)
	su := cov1xSignUp(t, s, householdA, job.ID, "Ann")
	cov1xSignUp(t, s, householdB, job.ID, "Bea")
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, su.ID, StatusWorked, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteJob(ctx, coach(), testOrg, job.ID); !errors.Is(err, ErrJobHasWorkedSignups) {
		t.Fatalf("worked signups block deletion: %v", err)
	}
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, su.ID, StatusConfirmed, nil); err != nil {
		t.Fatal(err)
	}
	r.fs.arm("listSignups")
	if _, err := s.DeleteJob(ctx, coach(), testOrg, job.ID); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("listSignups failure: %v", err)
	}
	r.fs.disarm()
	r.fs.arm("deleteJob")
	if _, err := s.DeleteJob(ctx, coach(), testOrg, job.ID); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("deleteJob failure: %v", err)
	}
	r.fs.disarm()
	r.fs.arm("getJob")
	if _, err := s.DeleteJob(ctx, coach(), testOrg, job.ID); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("getJob failure: %v", err)
	}
	r.fs.disarm()

	gone, err := s.DeleteJob(ctx, coach(), testOrg, job.ID)
	if err != nil || gone.ID != job.ID {
		t.Fatalf("delete: %+v %v", gone, err)
	}
	if _, err := s.GetJob(ctx, coach(), testOrg, job.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("deleted job still readable: %v", err)
	}
	if left, _ := r.fs.store.listSignups(ctx, testOrg, SignupFilter{}); len(left) != 0 {
		t.Fatalf("signups of a deleted job must be removed: %d", len(left))
	}
}

// ─── Listing jobs ─────────────────────────────────────────────────────────────

func TestCov1xListJobsFiltersAndFillCounts(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	cov1xBigDirectory(s)
	var lookups int
	s.SetMeetLookup(func(_, meetID string) (string, bool) {
		lookups++
		return "Spring Invite", meetID == "meet-1"
	})

	mk := func(in JobInput) *Job {
		t.Helper()
		j, err := s.CreateJob(ctx, coach(), testOrg, in)
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	meetA := mk(JobInput{MeetID: "meet-1", Role: "timer", Title: "B timer", StartsAt: baseNow.Add(72 * time.Hour), EndsAt: baseNow.Add(75 * time.Hour), Capacity: 2})
	meetB := mk(JobInput{MeetID: "meet-1", Role: "timer", Title: "A timer", StartsAt: baseNow.Add(72 * time.Hour), EndsAt: baseNow.Add(75 * time.Hour), Capacity: 1})
	club := mk(JobInput{EventName: "Clinic", Role: "setup", Title: "Setup", StartsAt: baseNow.Add(24 * time.Hour), EndsAt: baseNow.Add(26 * time.Hour), Capacity: 3})
	past := mk(JobInput{EventName: "Old", Role: "other", Title: "Past", StartsAt: baseNow.Add(-5 * time.Hour), EndsAt: baseNow.Add(-2 * time.Hour), Capacity: 1})
	explicit := mk(JobInput{EventName: "Archive", SeasonID: cov1xSeason2, Role: "other", Title: "Explicit", StartsAt: baseNow.Add(200 * time.Hour), EndsAt: baseNow.Add(201 * time.Hour), Capacity: 1})

	cov1xSignUp(t, s, householdA, meetA.ID, "Ann")
	cov1xSignUp(t, s, householdB, meetA.ID, "Bea")
	cov1xSignUp(t, s, householdA, meetB.ID, "Cat")
	cancelled := cov1xSignUp(t, s, householdB, club.ID, "Dee")
	if _, _, err := s.Cancel(ctx, coach(), testOrg, cancelled.ID, ""); err != nil {
		t.Fatal(err)
	}

	titles := func(views []JobView) string {
		out := make([]string, len(views))
		for i, v := range views {
			out[i] = v.Title
		}
		return strings.Join(out, ",")
	}
	lookups = 0
	all, err := s.ListJobs(ctx, swimmer(householdA), testOrg, JobFilter{})
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by start, then title.
	if got := titles(all); got != "Past,Setup,A timer,B timer,Explicit" {
		t.Fatalf("order: %s", got)
	}
	byTitle := map[string]JobView{}
	for _, v := range all {
		byTitle[v.Title] = v
	}
	if v := byTitle["B timer"]; v.FilledCount != 2 || v.OpenSpots != 0 || v.MeetName != "Spring Invite" || v.EffectiveSeasonID != testSeason {
		t.Fatalf("B timer: %+v", v)
	}
	if v := byTitle["Setup"]; v.FilledCount != 0 || v.OpenSpots != 3 || v.MeetName != "" {
		t.Fatalf("a cancelled signup must not count as filled: %+v", v)
	}
	if v := byTitle["Explicit"]; v.EffectiveSeasonID != cov1xSeason2 {
		t.Fatalf("explicit season wins over dates: %+v", v)
	}
	if lookups != 1 {
		t.Fatalf("meet names must be cached per list call (two jobs, one meet); lookups=%d", lookups)
	}
	if len(byTitle["Past"].Signups) != 0 {
		t.Fatal("list views carry no rosters")
	}

	up, _ := s.ListJobs(ctx, parent(householdA), testOrg, JobFilter{Upcoming: true})
	if got := titles(up); got != "Setup,A timer,B timer,Explicit" {
		t.Fatalf("upcoming: %s", got)
	}
	from, to := baseNow.Add(24*time.Hour), baseNow.Add(72*time.Hour)
	win, _ := s.ListJobs(ctx, coach(), testOrg, JobFilter{From: from, To: to})
	if got := titles(win); got != "Setup" {
		t.Fatalf("from/to is [from,to): %s", got)
	}
	meet, _ := s.ListJobs(ctx, coach(), testOrg, JobFilter{MeetID: "meet-1"})
	if got := titles(meet); got != "A timer,B timer" {
		t.Fatalf("meet filter: %s", got)
	}
	s2, _ := s.ListJobs(ctx, coach(), testOrg, JobFilter{SeasonID: cov1xSeason2})
	if got := titles(s2); got != "Explicit" {
		t.Fatalf("effective season filter: %s", got)
	}
	s1, _ := s.ListJobs(ctx, coach(), testOrg, JobFilter{SeasonID: testSeason})
	if len(s1) != 4 || strings.Contains(titles(s1), "Explicit") {
		t.Fatalf("season 1 filter: %s", titles(s1))
	}
	_ = explicit
	_ = past

	r.fs.arm("listJobs")
	if _, err := s.ListJobs(ctx, coach(), testOrg, JobFilter{}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("listJobs failure: %v", err)
	}
	r.fs.disarm()
	r.fs.arm("listSignups")
	if _, err := s.ListJobs(ctx, coach(), testOrg, JobFilter{}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("listSignups failure: %v", err)
	}

	// A lookup failure degrades gracefully: jobs list without effective seasons.
	r.fs.disarm()
	s.SetSeasonLookup(func(string) ([]SeasonRef, error) { return nil, errCov1xBoom })
	deg, err := s.ListJobs(ctx, coach(), testOrg, JobFilter{})
	if err != nil || len(deg) != 5 || byTitleSeason(deg, "B timer") != "" {
		t.Fatalf("degraded list: %v %v", err, deg)
	}
	if !strings.Contains(r.logs.String(), "season lookup failed") {
		t.Fatalf("season lookup failure must be logged: %s", r.logs.String())
	}
}

func byTitleSeason(views []JobView, title string) string {
	for _, v := range views {
		if v.Title == title {
			return v.EffectiveSeasonID
		}
	}
	return "?"
}

func TestCov1xJobViewClampsOpenSpots(t *testing.T) {
	s, _, _, _ := newTestService(t)
	v := s.jobView(&Job{OrgID: testOrg, Capacity: 1}, "", 3, map[string]string{})
	if v.OpenSpots != 0 || v.FilledCount != 3 {
		t.Fatalf("over-filled job must clamp open spots to zero: %+v", v)
	}
	s.SetMeetLookup(nil)
	if v := s.jobView(&Job{OrgID: testOrg, MeetID: "m", Capacity: 2}, "", 0, map[string]string{}); v.MeetName != "" || v.OpenSpots != 2 {
		t.Fatalf("no meet directory: %+v", v)
	}
}

func TestCov1xGetJobRosterVisibility(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	job := mustJob(t, s, 3, 96*time.Hour)
	ann := cov1xSignUp(t, s, householdA, job.ID, "Ann")
	cov1xSignUp(t, s, householdB, job.ID, "Bea")
	if _, _, err := s.Cancel(ctx, coach(), testOrg, ann.ID, ""); err != nil {
		t.Fatal(err)
	}

	staff, err := s.GetJob(ctx, coach(), testOrg, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if staff.FilledCount != 1 || staff.OpenSpots != 2 || len(staff.Signups) != 2 || staff.MeetName != "Spring Invite" {
		t.Fatalf("staff view: %+v", staff)
	}
	names := map[string]string{}
	for _, su := range staff.Signups {
		names[su.VolunteerName] = su.HouseholdName
	}
	if names["Ann"] != "Alpha" || names["Bea"] != "Bravo" {
		t.Fatalf("household names on roster: %v", names)
	}
	par, err := s.GetJob(ctx, parent(householdA), testOrg, job.ID)
	if err != nil || len(par.Signups) != 0 || par.FilledCount != 1 {
		t.Fatalf("parents never see the roster: %+v %v", par, err)
	}
	if _, err := s.GetJob(ctx, coach(), testOrg, "missing"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing: %v", err)
	}
	for _, name := range []string{"getJob", "listSignups"} {
		r.fs.disarm()
		r.fs.arm(name)
		if _, err := s.GetJob(ctx, coach(), testOrg, job.ID); !errors.Is(err, errCov1xBoom) {
			t.Fatalf("%s failure: %v", name, err)
		}
	}
}

// ─── Templates and bulk create ────────────────────────────────────────────────

func TestCov1xBulkCreateJobs(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	shift := func(h int, label string) Shift {
		return Shift{StartsAt: baseNow.Add(time.Duration(h) * time.Hour), EndsAt: baseNow.Add(time.Duration(h+3) * time.Hour), Label: label}
	}

	jobs, err := s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{MeetID: "meet-1", Location: "Main pool", Shifts: []Shift{shift(72, "AM"), shift(100, " ")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 14 {
		t.Fatalf("standard_meet is 7 roles x 2 shifts, got %d", len(jobs))
	}
	if jobs[0].Title != "Timer · AM" || jobs[7].Title != "Timer" || jobs[0].Location != "Main pool" || jobs[0].MeetID != "meet-1" {
		t.Fatalf("titles/labels: %q %q", jobs[0].Title, jobs[7].Title)
	}
	stored, _ := r.fs.store.listJobs(ctx, testOrg, "")
	if len(stored) != 14 {
		t.Fatalf("stored %d jobs", len(stored))
	}

	club, err := s.BulkCreateJobs(ctx, admin(), testOrg, BulkInput{EventName: "Banquet", TemplateID: " club_event ", Shifts: []Shift{shift(50, "")}})
	if err != nil || len(club) != 4 {
		t.Fatalf("club_event: %d %v", len(club), err)
	}

	hc := 0.5
	custom, err := s.BulkCreateJobs(ctx, admin(), testOrg, BulkInput{EventName: "Custom", Roles: []TemplateRole{{Role: "timer", Title: "Solo", Capacity: 1, HourCredit: &hc, MinAge: 14, RequiresCertification: true}}, Shifts: []Shift{shift(60, "")}})
	if err != nil || len(custom) != 1 || custom[0].HourCredit != 0.5 || custom[0].MinAge != 14 || !custom[0].RequiresCertification {
		t.Fatalf("custom roles override the template: %+v %v", custom, err)
	}

	count := func() int { l, _ := r.fs.store.listJobs(ctx, testOrg, ""); return len(l) }
	before := count()
	if _, err := s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{EventName: "X", TemplateID: "nope", Shifts: []Shift{shift(10, "")}}); !cov1xIsValidation(err, "template") {
		t.Fatalf("unknown template: %v", err)
	}
	if _, err := s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{EventName: "X"}); !cov1xIsValidation(err, "shifts") {
		t.Fatalf("no shifts: %v", err)
	}
	many := make([]Shift, 41)
	for i := range many {
		many[i] = shift(10+i, "")
	}
	if _, err := s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{EventName: "X", Shifts: many}); !cov1xIsValidation(err, "shifts") {
		t.Fatalf("41 shifts: %v", err)
	}
	roles := make([]TemplateRole, 13)
	for i := range roles {
		roles[i] = TemplateRole{Role: "other", Title: fmt.Sprintf("R%d", i), Capacity: 1}
	}
	if _, err := s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{EventName: "X", Roles: roles, Shifts: many[:40]}); !cov1xIsValidation(err, "roles") {
		t.Fatalf("520 jobs: %v", err)
	}
	bad := shift(10, "")
	bad.EndsAt = bad.StartsAt
	_, err = s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{EventName: "X", TemplateID: "club_event", Shifts: []Shift{shift(10, ""), bad}})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "shifts[1].ends_at" {
		t.Fatalf("validation errors name the offending shift: %v", err)
	}
	_, err = s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{EventName: "X", Roles: []TemplateRole{{Role: "timer", Capacity: 1}}, Shifts: []Shift{shift(10, "Lbl")}})
	if !errors.As(err, &ve) || ve.Field != "shifts[0].title" {
		t.Fatalf("blank title keeps no label: %v", err)
	}
	if _, err := s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{MeetID: "meet-missing", Shifts: []Shift{shift(10, "")}}); !errors.Is(err, ErrMeetNotFound) {
		t.Fatalf("unknown meet is returned as is: %v", err)
	}
	r.fs.arm("insertJobs")
	if _, err := s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{EventName: "X", Shifts: []Shift{shift(10, "")}}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("insert failure: %v", err)
	}
	r.fs.disarm()
	if count() != before {
		t.Fatalf("failed bulk creates must be all-or-nothing: %d -> %d", before, count())
	}
}

// ─── Signups ──────────────────────────────────────────────────────────────────

func TestCov1xSignUpGuards(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	job := mustJob(t, s, 5, 96*time.Hour)

	noHH := &appmiddleware.Principal{UserID: "p", Roles: []string{"parent"}}
	if _, err := s.SignUp(ctx, noHH, testOrg, job.ID, SignupInput{VolunteerName: "X"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent without household: %v", err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "  "}); !cov1xIsValidation(err, "volunteer_name") {
		t.Fatalf("blank name: %v", err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: strings.Repeat("n", 121)}); !cov1xIsValidation(err, "volunteer_name") {
		t.Fatalf("long name: %v", err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Pat", Notes: strings.Repeat("n", 501)}); !cov1xIsValidation(err, "notes") {
		t.Fatalf("long notes: %v", err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, "missing", SignupInput{VolunteerName: "Pat"}); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing job: %v", err)
	}
	// A parent may name their own household explicitly.
	su, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Pat", HouseholdID: " " + householdA + " ", Notes: " bring pen "})
	if err != nil || su.Notes != "bring pen" || su.SignedUpBy != "u-parent-hh-a" || su.Status != StatusConfirmed {
		t.Fatalf("own household: %+v %v", su, err)
	}

	// Staff path: household lookup failure surfaces; no directory means no check.
	s.SetHouseholdDirectory(func(string) ([]HouseholdRef, error) { return nil, errCov1xBoom })
	if _, err := s.SignUp(ctx, coach(), testOrg, job.ID, SignupInput{VolunteerName: "Bo", HouseholdID: householdB}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("household directory failure: %v", err)
	}
	s.SetHouseholdDirectory(nil)
	if v, err := s.SignUp(ctx, coach(), testOrg, job.ID, SignupInput{VolunteerName: "Bo", HouseholdID: "hh-unlisted"}); err != nil || v.HouseholdName != "" {
		t.Fatalf("without a directory staff may use any household id: %+v %v", v, err)
	}

	// Swimmer volunteers.
	teen := SignupInput{VolunteerName: "Teen", SwimmerID: " " + teenSwimmer + " "}
	v, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, teen)
	if err != nil || v.SwimmerID != teenSwimmer {
		t.Fatalf("swimmer volunteer: %+v %v", v, err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Ghost", SwimmerID: "ghost"}); !cov1xIsValidation(err, "swimmer_id") {
		t.Fatalf("unknown swimmer: %v", err)
	}
	s.SetSwimmerLookup(nil)
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Teen2", SwimmerID: teenSwimmer}); !cov1xIsValidation(err, "swimmer_id") {
		t.Fatalf("no swimmer directory: %v", err)
	}
	s.SetSwimmerLookup(func(_, id string) (SwimmerRef, bool) {
		return SwimmerRef{HouseholdID: householdA, DateOfBirth: "garbage", Name: id}, true
	})
	minAge, err := s.CreateJob(ctx, admin(), testOrg, JobInput{EventName: "Clinic", Role: "official", Title: "Older", StartsAt: baseNow.Add(80 * time.Hour), EndsAt: baseNow.Add(82 * time.Hour), Capacity: 2, MinAge: 13})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, minAge.ID, SignupInput{VolunteerName: "Sw", SwimmerID: "sw"}); !errors.Is(err, ErrAgeRequirement) {
		t.Fatalf("an unreadable date of birth cannot satisfy a minimum age: %v", err)
	}
	// Without a minimum age the date of birth is never consulted.
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Sw", SwimmerID: "sw"}); err != nil {
		t.Fatalf("no minimum age: %v", err)
	}

	r.fs.arm("insertSignup")
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Zed"}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("insert failure: %v", err)
	}
	r.fs.disarm()
	r.fs.arm("getJob")
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Zed"}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("getJob failure: %v", err)
	}
}

func TestCov1xAgeOn(t *testing.T) {
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 9, 0, 0, 0, time.UTC) }
	cases := []struct {
		dob  string
		at   time.Time
		want int
		ok   bool
	}{
		{"2010-06-15", at(2026, 6, 14), 15, true},
		{"2010-06-15", at(2026, 6, 15), 16, true},
		{"2010-06-15", at(2026, 5, 1), 15, true},
		{"2010-06-15", at(2026, 7, 1), 16, true},
		{" 2010-06-15 ", at(2026, 7, 1), 16, true},
		{"06/15/2010", at(2026, 7, 1), 0, false},
		{"", at(2026, 7, 1), 0, false},
	}
	for _, c := range cases {
		got, ok := ageOn(c.dob, c.at)
		if got != c.want || ok != c.ok {
			t.Fatalf("ageOn(%q, %s) = %d,%v want %d,%v", c.dob, c.at.Format("2006-01-02"), got, ok, c.want, c.ok)
		}
	}
}

func TestCov1xCancelBranches(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	job := mustJob(t, s, 5, 96*time.Hour)

	su, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Pat", Notes: "front desk"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Cancel(ctx, parent(householdA), testOrg, "missing", ""); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("missing signup: %v", err)
	}
	noHH := &appmiddleware.Principal{UserID: "p", Roles: []string{"parent"}}
	if _, _, err := s.Cancel(ctx, noHH, testOrg, su.ID, ""); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("parent without household must not see signups: %v", err)
	}

	// Reasons are trimmed, appended to existing notes and capped at 500 runes.
	long := strings.Repeat("é", 600)
	before, after, err := s.Cancel(ctx, coach(), testOrg, su.ID, "  "+long+"  ")
	if err != nil {
		t.Fatal(err)
	}
	want := "front desk\nCancelled: " + strings.Repeat("é", 500)
	if before.Status != StatusConfirmed || after.Status != StatusCancelled || after.Notes != want || after.CancelledBy != "u-coach" || after.CancelledAt == nil {
		t.Fatalf("cancel: before=%+v after=%+v", before, after)
	}
	// A blank reason adds no note.
	b2 := cov1xSignUp(t, s, householdB, job.ID, "Bea")
	_, blank, err := s.Cancel(ctx, coach(), testOrg, b2.ID, "   ")
	if err != nil || blank.Notes != "" {
		t.Fatalf("blank reason: %+v %v", blank, err)
	}

	// Worked / no-show spots cannot be cancelled.
	w := cov1xSignUp(t, s, householdA, job.ID, "Worker")
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, w.ID, StatusWorked, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Cancel(ctx, coach(), testOrg, w.ID, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("worked: %v", err)
	}
	n := cov1xSignUp(t, s, householdA, job.ID, "Absent")
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, n.ID, StatusNoShow, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Cancel(ctx, parent(householdA), testOrg, n.ID, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("no-show: %v", err)
	}

	live := cov1xSignUp(t, s, householdA, job.ID, "Live")
	for _, name := range []string{"getSignup", "getJob", "getSettings", "updateSignup"} {
		r.fs.disarm()
		r.fs.arm(name)
		p := parent(householdA)
		if _, _, err := s.Cancel(ctx, p, testOrg, live.ID, ""); !errors.Is(err, errCov1xBoom) {
			t.Fatalf("%s failure: %v", name, err)
		}
	}
	r.fs.disarm()
	if got, _ := r.fs.store.getSignup(ctx, testOrg, live.ID); got.Status != StatusConfirmed {
		t.Fatalf("failed cancels must not change state: %s", got.Status)
	}
	if !s.canCancel(job, nil) {
		t.Fatal("with no settings the default 48h cutoff applies (job is 96h away)")
	}
}

func TestCov1xCheckInTransitions(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	job := mustJob(t, s, 5, 96*time.Hour)
	su := cov1xSignUp(t, s, householdA, job.ID, "Pat")

	if _, _, err := s.CheckIn(ctx, coach(), testOrg, su.ID, "later", nil); !cov1xIsValidation(err, "status") {
		t.Fatalf("bad status: %v", err)
	}
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, "missing", StatusWorked, nil); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("missing: %v", err)
	}
	h := 2.0
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, su.ID, StatusNoShow, &h); !cov1xIsValidation(err, "hours") {
		t.Fatalf("hours with no_show: %v", err)
	}
	for _, bad := range []float64{-1, 24.5} {
		bad := bad
		if _, _, err := s.CheckIn(ctx, coach(), testOrg, su.ID, StatusWorked, &bad); !cov1xIsValidation(err, "hours") {
			t.Fatalf("hours %v: %v", bad, err)
		}
	}
	// Plain worked: credit falls back to the job's hour credit.
	before, worked, err := s.CheckIn(ctx, coach(), testOrg, su.ID, " WORKED ", nil)
	if err != nil || before.Status != StatusConfirmed || worked.Status != StatusWorked || worked.HoursCredited != nil || worked.CheckedInBy != "u-coach" || worked.CheckedInAt == nil {
		t.Fatalf("worked: %+v %+v %v", before, worked, err)
	}
	// Override rounds to 2 decimals.
	odd := 2.126
	_, worked, err = s.CheckIn(ctx, coach(), testOrg, su.ID, StatusWorked, &odd)
	if err != nil || worked.HoursCredited == nil || *worked.HoursCredited != 2.13 {
		t.Fatalf("override: %+v %v", worked, err)
	}
	// no_show clears the override; confirmed clears the check-in stamp.
	_, ns, err := s.CheckIn(ctx, coach(), testOrg, su.ID, StatusNoShow, nil)
	if err != nil || ns.HoursCredited != nil || ns.CheckedInAt == nil {
		t.Fatalf("no_show: %+v %v", ns, err)
	}
	_, back, err := s.CheckIn(ctx, coach(), testOrg, su.ID, StatusConfirmed, nil)
	if err != nil || back.Status != StatusConfirmed || back.CheckedInAt != nil || back.CheckedInBy != "" {
		t.Fatalf("back to confirmed: %+v %v", back, err)
	}
	// Cancelled signups are final.
	if _, _, err := s.Cancel(ctx, coach(), testOrg, su.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, su.ID, StatusWorked, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("cancelled: %v", err)
	}
	live := cov1xSignUp(t, s, householdA, job.ID, "Live")
	r.fs.arm("updateSignup")
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, live.ID, StatusWorked, nil); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("updateSignup failure: %v", err)
	}
}

func TestCov1xListSignupsScopingFiltersAndErrors(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	meetJob := mustJob(t, s, 5, 96*time.Hour)
	clubJob, err := s.CreateJob(ctx, admin(), testOrg, JobInput{EventName: "Clinic", Role: "setup", Title: "Setup", StartsAt: baseNow.Add(30 * time.Hour), EndsAt: baseNow.Add(32 * time.Hour), Capacity: 5})
	if err != nil {
		t.Fatal(err)
	}
	a1 := cov1xSignUp(t, s, householdA, meetJob.ID, "Ann")
	cov1xSignUp(t, s, householdB, meetJob.ID, "Bea")
	a2 := cov1xSignUp(t, s, householdA, clubJob.ID, "Ann")
	if _, _, err := s.Cancel(ctx, coach(), testOrg, a2.ID, ""); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListSignups(ctx, parent(householdA), testOrg, SignupFilter{})
	if err != nil || len(got) != 2 {
		t.Fatalf("parent sees only own household: %d %v", len(got), err)
	}
	// Sorted by job start (the club job starts first).
	if got[0].JobID != clubJob.ID || got[1].JobID != meetJob.ID || got[0].Job == nil || got[0].Job.Title != "Setup" {
		t.Fatalf("order/job decoration: %+v", got)
	}
	if got[1].CanCancel != true || got[0].CanCancel != false {
		t.Fatalf("can_cancel: confirmed outside cutoff true, cancelled false: %v %v", got[1].CanCancel, got[0].CanCancel)
	}
	if _, err := s.ListSignups(ctx, parent(householdA), testOrg, SignupFilter{HouseholdID: householdB}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other household: %v", err)
	}
	own, err := s.ListSignups(ctx, swimmer(householdA), testOrg, SignupFilter{HouseholdID: householdA})
	if err != nil || len(own) != 2 {
		t.Fatalf("swimmer own household: %d %v", len(own), err)
	}

	byMeet, _ := s.ListSignups(ctx, coach(), testOrg, SignupFilter{MeetID: "meet-1"})
	if len(byMeet) != 2 {
		t.Fatalf("meet filter drops signups of non-meet jobs: %d", len(byMeet))
	}
	byJob, _ := s.ListSignups(ctx, coach(), testOrg, SignupFilter{JobID: clubJob.ID})
	byHH, _ := s.ListSignups(ctx, coach(), testOrg, SignupFilter{HouseholdID: householdB})
	byStatus, _ := s.ListSignups(ctx, coach(), testOrg, SignupFilter{Status: StatusCancelled})
	if len(byJob) != 1 || len(byHH) != 1 || len(byStatus) != 1 || byStatus[0].ID != a2.ID || a1.ID == a2.ID {
		t.Fatalf("filters: job=%d hh=%d status=%d", len(byJob), len(byHH), len(byStatus))
	}
	if byHH[0].HouseholdName != "Bravo" {
		t.Fatalf("household name: %+v", byHH[0])
	}
	for _, name := range []string{"listSignups", "listJobs", "getSettings"} {
		r.fs.disarm()
		r.fs.arm(name)
		if _, err := s.ListSignups(ctx, coach(), testOrg, SignupFilter{}); !errors.Is(err, errCov1xBoom) {
			t.Fatalf("%s failure: %v", name, err)
		}
	}
}

// ─── Requirements and credits ─────────────────────────────────────────────────

func TestCov1xRequirements(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	cov1xBigDirectory(s)

	bad := []struct {
		name  string
		in    RequirementInput
		check func(error) bool
	}{
		{"no season", RequirementInput{}, func(e error) bool { return cov1xIsValidation(e, "season_id") }},
		{"unknown season", RequirementInput{SeasonID: "nope"}, func(e error) bool { return errors.Is(e, ErrSeasonNotFound) }},
		{"unknown household", RequirementInput{SeasonID: testSeason, HouseholdID: "ghost"}, func(e error) bool { return errors.Is(e, ErrHouseholdNotFound) }},
		{"hours negative", RequirementInput{SeasonID: testSeason, RequiredHours: -1}, func(e error) bool { return cov1xIsValidation(e, "required_hours") }},
		{"hours too big", RequirementInput{SeasonID: testSeason, RequiredHours: 1001}, func(e error) bool { return cov1xIsValidation(e, "required_hours") }},
		{"rate negative", RequirementInput{SeasonID: testSeason, BuyoutCentsPerHour: -1}, func(e error) bool { return cov1xIsValidation(e, "buyout_cents_per_hour") }},
		{"rate too big", RequirementInput{SeasonID: testSeason, BuyoutCentsPerHour: 100001}, func(e error) bool { return cov1xIsValidation(e, "buyout_cents_per_hour") }},
		{"notes too long", RequirementInput{SeasonID: testSeason, Notes: strings.Repeat("n", 501)}, func(e error) bool { return cov1xIsValidation(e, "notes") }},
	}
	for _, c := range bad {
		if _, err := s.UpsertRequirement(ctx, admin(), testOrg, c.in); !c.check(err) {
			t.Fatalf("%s: got %v", c.name, err)
		}
	}
	if all, _ := s.ListRequirements(ctx, admin(), testOrg, ""); len(all) != 0 {
		t.Fatalf("rejected requirements stored: %d", len(all))
	}

	first, err := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: " " + testSeason + " ", RequiredHours: 10.004, BuyoutCentsPerHour: 2500, Notes: "  base  "})
	if err != nil || first.RequiredHours != 10 || first.Notes != "base" || first.HouseholdID != "" || first.UpdatedBy != "u-admin" {
		t.Fatalf("season default: %+v %v", first, err)
	}
	again, err := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: testSeason, RequiredHours: 8, BuyoutCentsPerHour: 3000})
	if err != nil || again.ID != first.ID || again.RequiredHours != 8 || again.BuyoutCentsPerHour != 3000 {
		t.Fatalf("a second upsert for the same scope replaces the first: %+v %v", again, err)
	}
	override, err := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: testSeason, HouseholdID: householdB, RequiredHours: 0})
	if err != nil || override.ID == first.ID || override.HouseholdID != householdB {
		t.Fatalf("household override: %+v %v", override, err)
	}
	other, _ := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: cov1xSeason2, RequiredHours: 4})
	list, _ := s.ListRequirements(ctx, coach(), testOrg, testSeason)
	if len(list) != 2 || list[0].HouseholdID != "" || list[1].HouseholdID != householdB {
		t.Fatalf("season filter and ordering: %+v", list)
	}
	all, _ := s.ListRequirements(ctx, coach(), testOrg, "")
	if len(all) != 3 || all[2].ID != other.ID {
		t.Fatalf("all requirements sorted by season id: %+v", all)
	}

	if err := s.DeleteRequirement(ctx, admin(), testOrg, "missing"); !errors.Is(err, ErrRequirementNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	if err := s.DeleteRequirement(ctx, admin(), testOrg, override.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.ListRequirements(ctx, coach(), testOrg, testSeason); len(left) != 1 {
		t.Fatalf("after delete: %d", len(left))
	}

	r.fs.arm("upsertRequirement")
	if _, err := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: testSeason, RequiredHours: 1}); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("upsert failure: %v", err)
	}
	r.fs.disarm()
	r.fs.arm("listRequirements")
	if _, err := s.ListRequirements(ctx, coach(), testOrg, ""); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("list failure: %v", err)
	}
	// A season directory failure reads as an unknown season.
	s.SetSeasonLookup(func(string) ([]SeasonRef, error) { return nil, errCov1xBoom })
	if _, err := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: testSeason}); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("season directory failure: %v", err)
	}
}

func TestCov1xCredits(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	cov1xBigDirectory(s)

	ok := CreditInput{SeasonID: testSeason, HouseholdID: householdA, Hours: 2, Reason: "Pool day"}
	mut := func(f func(*CreditInput)) CreditInput { in := ok; f(&in); return in }
	bad := []struct {
		name  string
		in    CreditInput
		check func(error) bool
	}{
		{"no season", mut(func(in *CreditInput) { in.SeasonID = "" }), func(e error) bool { return cov1xIsValidation(e, "season_id") }},
		{"unknown season", mut(func(in *CreditInput) { in.SeasonID = "nope" }), func(e error) bool { return errors.Is(e, ErrSeasonNotFound) }},
		{"no household", mut(func(in *CreditInput) { in.HouseholdID = " " }), func(e error) bool { return cov1xIsValidation(e, "household_id") }},
		{"unknown household", mut(func(in *CreditInput) { in.HouseholdID = "ghost" }), func(e error) bool { return errors.Is(e, ErrHouseholdNotFound) }},
		{"zero hours", mut(func(in *CreditInput) { in.Hours = 0 }), func(e error) bool { return cov1xIsValidation(e, "hours") }},
		{"hours round to zero", mut(func(in *CreditInput) { in.Hours = 0.001 }), func(e error) bool { return cov1xIsValidation(e, "hours") }},
		{"too many hours", mut(func(in *CreditInput) { in.Hours = 200.5 }), func(e error) bool { return cov1xIsValidation(e, "hours") }},
		{"too few hours", mut(func(in *CreditInput) { in.Hours = -201 }), func(e error) bool { return cov1xIsValidation(e, "hours") }},
		{"no reason", mut(func(in *CreditInput) { in.Reason = "  " }), func(e error) bool { return cov1xIsValidation(e, "reason") }},
		{"long reason", mut(func(in *CreditInput) { in.Reason = strings.Repeat("r", 501) }), func(e error) bool { return cov1xIsValidation(e, "reason") }},
	}
	for _, c := range bad {
		if _, err := s.AddCredit(ctx, coach(), testOrg, c.in); !c.check(err) {
			t.Fatalf("%s: got %v", c.name, err)
		}
	}
	pos, err := s.AddCredit(ctx, coach(), testOrg, mut(func(in *CreditInput) { in.Hours = 1.234; in.Reason = " Pool day " }))
	if err != nil || pos.Hours != 1.23 || pos.Reason != "Pool day" || pos.CreatedBy != "u-coach" || !pos.CreatedAt.Equal(baseNow) {
		t.Fatalf("credit: %+v %v", pos, err)
	}
	if _, err := s.AddCredit(ctx, admin(), testOrg, mut(func(in *CreditInput) { in.Hours = -2; in.HouseholdID = householdB; in.Reason = "Correction" })); err != nil {
		t.Fatalf("negative credit: %v", err)
	}
	r.fs.arm("insertCredit")
	if _, err := s.AddCredit(ctx, coach(), testOrg, ok); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("insert failure: %v", err)
	}
	r.fs.disarm()

	own, err := s.ListCredits(ctx, parent(householdA), testOrg, "", "")
	if err != nil || len(own) != 1 || own[0].HouseholdID != householdA {
		t.Fatalf("parent scope: %+v %v", own, err)
	}
	if _, err := s.ListCredits(ctx, parent(householdA), testOrg, "", householdB); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other household: %v", err)
	}
	if got, _ := s.ListCredits(ctx, swimmer(householdA), testOrg, testSeason, householdA); len(got) != 1 {
		t.Fatalf("swimmer own: %d", len(got))
	}
	if all, _ := s.ListCredits(ctx, coach(), testOrg, "", ""); len(all) != 2 {
		t.Fatalf("staff all: %d", len(all))
	}
	if none, _ := s.ListCredits(ctx, coach(), testOrg, cov1xSeason2, ""); len(none) != 0 {
		t.Fatalf("season filter: %d", len(none))
	}
	if b, _ := s.ListCredits(ctx, coach(), testOrg, "", householdB); len(b) != 1 || b[0].Hours != -2 {
		t.Fatalf("household filter: %+v", b)
	}
}

// ─── Season / household helpers ───────────────────────────────────────────────

func TestCov1xEffectiveSeason(t *testing.T) {
	at := func(d string) *Job {
		ts, _ := time.Parse("2006-01-02 15:04", d+" 23:30")
		return &Job{StartsAt: ts}
	}
	seasons := []SeasonRef{
		{ID: "undated", Name: "x"},
		{ID: "half", StartDate: "2026-01-01"},
		{ID: "upcoming", Status: "upcoming", StartDate: "2026-01-01", EndDate: "2026-12-31"},
		{ID: "active", Status: "active", StartDate: "2026-03-01", EndDate: "2026-06-30"},
		{ID: "past", Status: "completed", StartDate: "2025-01-01", EndDate: "2025-12-31"},
	}
	if got := effectiveSeason(&Job{SeasonID: "explicit", StartsAt: baseNow}, seasons); got != "explicit" {
		t.Fatalf("explicit: %s", got)
	}
	for date, want := range map[string]string{
		"2026-02-10": "upcoming", // only the non-active season contains it
		"2026-03-01": "active",   // active wins an overlap, boundary inclusive
		"2026-06-30": "active",
		"2026-07-01": "upcoming",
		"2025-06-01": "past",
		"2024-06-01": "",
	} {
		if got := effectiveSeason(at(date), seasons); got != want {
			t.Fatalf("%s: got %q want %q", date, got, want)
		}
	}
	if got := effectiveSeason(at("2026-03-02"), nil); got != "" {
		t.Fatalf("no seasons: %q", got)
	}
}

func TestCov1xDirectoryFailuresDegradeGracefully(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	s.SetHouseholdDirectory(func(string) ([]HouseholdRef, error) { return nil, errCov1xBoom })
	if names := s.householdNames(testOrg); len(names) != 0 {
		t.Fatalf("names: %v", names)
	}
	if !strings.Contains(r.logs.String(), "household lookup failed") {
		t.Fatalf("household lookup failure must be logged: %s", r.logs.String())
	}
	if _, err := s.HouseholdLedger(ctx, admin(), testOrg, householdA); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("household ledger with a failing directory: %v", err)
	}
	rows, err := s.SeasonLedger(ctx, admin(), testOrg, testSeason)
	if err != nil || len(rows) != 0 {
		t.Fatalf("season ledger with a failing directory: %v %v", rows, err)
	}
	s.SetHouseholdDirectory(nil)
	if s.householdList(testOrg) != nil || s.seasonListNil() {
		t.Fatal("unset directories yield no households")
	}
	// Without directories the household ledger still answers for the id given.
	s.SetSeasonLookup(nil)
	rows, err = s.HouseholdLedger(ctx, admin(), testOrg, "anyone")
	if err != nil || len(rows) != 0 {
		t.Fatalf("no seasons configured: %v %v", rows, err)
	}
	// With no season directory every season id is accepted, but an empty one is not.
	if _, err := s.SeasonLedger(ctx, admin(), testOrg, ""); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("blank season: %v", err)
	}
}

func (s *Service) seasonListNil() bool { return s.seasonList(testOrg) == nil }

// ─── Ledger and buyouts ───────────────────────────────────────────────────────

type cov1xLedgerWorld struct {
	r    *cov1xRig
	jobs map[string]*Job
}

func cov1xBuildLedgerWorld(t *testing.T) *cov1xLedgerWorld {
	t.Helper()
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	cov1xBigDirectory(s)
	w := &cov1xLedgerWorld{r: r, jobs: map[string]*Job{}}

	req := func(in RequirementInput) {
		t.Helper()
		if _, err := s.UpsertRequirement(ctx, admin(), testOrg, in); err != nil {
			t.Fatal(err)
		}
	}
	req(RequirementInput{SeasonID: testSeason, RequiredHours: 10, BuyoutCentsPerHour: 2500})
	req(RequirementInput{SeasonID: testSeason, HouseholdID: householdB, RequiredHours: 0})
	req(RequirementInput{SeasonID: testSeason, HouseholdID: cov1xHHD, RequiredHours: 4, BuyoutCentsPerHour: 1000})

	mk := func(key, title string, startsIn, length time.Duration, credit float64, season string) {
		t.Helper()
		hc := credit
		j, err := s.CreateJob(ctx, coach(), testOrg, JobInput{EventName: "Ledger", SeasonID: season, Role: "timer", Title: title,
			StartsAt: baseNow.Add(startsIn), EndsAt: baseNow.Add(startsIn + length), Capacity: 10, HourCredit: &hc})
		if err != nil {
			t.Fatal(err)
		}
		w.jobs[key] = j
	}
	mk("meet", "Meet shift", 72*time.Hour, 3*time.Hour, 3, "")
	mk("big", "Big shift", 80*time.Hour, 3*time.Hour, 5, "")
	mk("past", "Past shift", -10*time.Hour, 3*time.Hour, 3, "")
	mk("old", "Old season shift", 90*time.Hour, 3*time.Hour, 6, cov1xSeason2)

	// Alpha: worked 3.5 (override) + one scheduled 3h shift + an already-over confirmed shift + a shift in another season.
	pat := cov1xSignUp(t, s, householdA, w.jobs["meet"].ID, "Pat")
	cov1xSignUp(t, s, householdA, w.jobs["meet"].ID, "Chris")
	hours := 3.5
	if _, _, err := s.CheckIn(ctx, coach(), testOrg, pat.ID, StatusWorked, &hours); err != nil {
		t.Fatal(err)
	}
	if err := r.fs.store.insertSignup(ctx, &Signup{ID: "su-past-confirmed", OrgID: testOrg, JobID: w.jobs["past"].ID, HouseholdID: householdA,
		VolunteerName: "Ghost", Status: StatusConfirmed, CreatedAt: baseNow, UpdatedAt: baseNow}); err != nil {
		t.Fatal(err)
	}
	cov1xSignUp(t, s, householdA, w.jobs["old"].ID, "Old")
	if _, err := s.AddCredit(ctx, coach(), testOrg, CreditInput{SeasonID: testSeason, HouseholdID: householdA, Hours: 1.25, Reason: "Pool cleanup"}); err != nil {
		t.Fatal(err)
	}
	// Bravo is exempt but books a shift.
	cov1xSignUp(t, s, householdB, w.jobs["meet"].ID, "Bo")
	// Delta books a 5h shift (on track against 4h).
	cov1xSignUp(t, s, cov1xHHD, w.jobs["big"].ID, "Dan")
	// Echo meets the default through manual credit.
	if _, err := s.AddCredit(ctx, admin(), testOrg, CreditInput{SeasonID: testSeason, HouseholdID: cov1xHHE, Hours: 10, Reason: "Officiating"}); err != nil {
		t.Fatal(err)
	}
	// Foxtrot has a pending (unbilled) buyout reservation.
	if err := r.fs.store.reserveBuyout(ctx, &Buyout{ID: "bo-fox", OrgID: testOrg, SeasonID: testSeason, HouseholdID: cov1xHHF, Status: "pending",
		HoursShort: 10, RateCentsPerHour: 2500, AmountCents: 25000, CreatedAt: baseNow, UpdatedAt: baseNow}); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestCov1xSeasonLedgerStatuses(t *testing.T) {
	w := cov1xBuildLedgerWorld(t)
	s, ctx := w.r.s, context.Background()

	rows, err := s.SeasonLedger(ctx, coach(), testOrg, testSeason)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(rows))
	byID := map[string]LedgerRow{}
	for i, row := range rows {
		ids[i] = row.HouseholdID
		byID[row.HouseholdID] = row
	}
	// Inactive Charlie is omitted; sorted by remaining hours desc, then name.
	wantOrder := []string{cov1xHHF, householdA, cov1xHHD, householdB, cov1xHHE}
	if strings.Join(ids, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("order: %v want %v", ids, wantOrder)
	}

	a := byID[householdA]
	if a.WorkedHours != 3.5 || a.CreditedHours != 1.25 || a.EarnedHours != 4.75 || a.ScheduledHours != 3 || a.RemainingHours != 5.25 ||
		a.Status != ComplianceShort || a.BuyoutDueCents != 13125 || a.RequirementSource != "season" || a.RequiredHours != 10 || a.Buyout != nil ||
		a.SeasonName != "2025-26" || a.SeasonStartDate != "2025-09-01" || a.HouseholdName != "Alpha" {
		t.Fatalf("alpha: %+v", a)
	}
	b := byID[householdB]
	if b.Status != ComplianceExempt || b.RequirementSource != "household" || b.RequiredHours != 0 || b.BuyoutDueCents != 0 || b.ScheduledHours != 3 {
		t.Fatalf("bravo: %+v", b)
	}
	d := byID[cov1xHHD]
	if d.Status != ComplianceOnTrack || d.RemainingHours != 4 || d.BuyoutCentsPerHour != 1000 || d.BuyoutDueCents != 4000 || d.RequirementSource != "household" || d.ScheduledHours != 5 {
		t.Fatalf("delta: %+v", d)
	}
	e := byID[cov1xHHE]
	if e.Status != ComplianceMet || e.RemainingHours != 0 || e.BuyoutDueCents != 0 || e.EarnedHours != 10 {
		t.Fatalf("echo: %+v", e)
	}
	f := byID[cov1xHHF]
	if f.Status != ComplianceShort || f.Buyout == nil || f.Buyout.Status != "pending" || f.BuyoutDueCents != 0 || f.RemainingHours != 10 {
		t.Fatalf("a pending buyout is shown but does not zero the compliance status: %+v", f)
	}

	// Errors.
	if _, err := s.SeasonLedger(ctx, coach(), testOrg, "unknown"); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("unknown season: %v", err)
	}
	for _, name := range []string{"getSettings", "listJobs", "listSignups", "listCredits", "listRequirements", "listBuyouts"} {
		w.r.fs.disarm()
		w.r.fs.arm(name)
		if _, err := s.SeasonLedger(ctx, coach(), testOrg, testSeason); !errors.Is(err, errCov1xBoom) {
			t.Fatalf("%s failure: %v", name, err)
		}
		if _, err := s.HouseholdLedger(ctx, coach(), testOrg, householdA); !errors.Is(err, errCov1xBoom) {
			t.Fatalf("household ledger, %s failure: %v", name, err)
		}
	}
}

func TestCov1xHouseholdLedgerSeasonsAndAccess(t *testing.T) {
	w := cov1xBuildLedgerWorld(t)
	s, ctx := w.r.s, context.Background()
	hours, cents := 2.0, 500
	if _, err := s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{DefaultRequiredHours: &hours, DefaultBuyoutCentsPerHour: &cents}); err != nil {
		t.Fatal(err)
	}

	rows, err := s.HouseholdLedger(ctx, parent(householdA), testOrg, " "+householdA+" ")
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	if rows[0].SeasonID != testSeason || rows[1].SeasonID != cov1xSeason2 {
		t.Fatalf("seasons are listed newest first: %s, %s", rows[0].SeasonID, rows[1].SeasonID)
	}
	old := rows[1]
	if old.RequirementSource != "org_default" || old.RequiredHours != 2 || old.BuyoutCentsPerHour != 500 || old.ScheduledHours != 6 || old.WorkedHours != 0 || old.RemainingHours != 2 || old.BuyoutDueCents != 1000 {
		t.Fatalf("org default applies to a season with no requirement: %+v", old)
	}
	if rows[0].RequirementSource != "season" || rows[0].WorkedHours != 3.5 {
		t.Fatalf("current season: %+v", rows[0])
	}
	if _, err := s.HouseholdLedger(ctx, parent(householdA), testOrg, householdB); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent reading another household: %v", err)
	}
	if _, err := s.HouseholdLedger(ctx, swimmer(householdA), testOrg, householdA); err != nil {
		t.Fatalf("swimmer own household: %v", err)
	}
	if _, err := s.HouseholdLedger(ctx, coach(), testOrg, "ghost"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("unknown household: %v", err)
	}
	// With no household directory the id is taken at face value.
	s.SetHouseholdDirectory(nil)
	if rows, err := s.HouseholdLedger(ctx, coach(), testOrg, "ghost"); err != nil || len(rows) != 2 || rows[0].HouseholdName != "" {
		t.Fatalf("no directory: %+v %v", rows, err)
	}
}

func TestCov1xLedgerRowEdgeCases(t *testing.T) {
	season := SeasonRef{ID: testSeason, Name: "S"}
	hh := HouseholdRef{ID: householdA, Name: "Alpha"}
	over := &Job{ID: "j-over", HourCredit: 2, EndsAt: baseNow.Add(-time.Hour)}
	in := &ledgerInputs{
		settings: DefaultSettings(testOrg),
		jobs:     map[string]*Job{"j-over": over},
		jobSeas:  map[string]string{"j-over": testSeason, "j-gone": testSeason},
		signups: []*Signup{
			{ID: "s1", JobID: "j-gone", HouseholdID: householdA, Status: StatusConfirmed},
			{ID: "s2", JobID: "j-over", HouseholdID: householdA, Status: StatusConfirmed},
			{ID: "s3", JobID: "j-over", HouseholdID: householdA, Status: StatusNoShow},
			{ID: "s4", JobID: "j-over", HouseholdID: householdA, Status: StatusCancelled},
		},
		now: baseNow,
	}
	row := in.row(season, hh)
	if row.ScheduledHours != 0 || row.WorkedHours != 0 || row.Status != ComplianceExempt {
		t.Fatalf("finished/absent/cancelled/orphaned signups earn and schedule nothing: %+v", row)
	}
	// Without a clock reading every confirmed shift counts as scheduled.
	in.now = time.Time{}
	if row := in.row(season, hh); row.ScheduledHours != 2 {
		t.Fatalf("zero clock: %+v", row)
	}
	// A billed buyout wins over every other status.
	in.buyouts = []*Buyout{{HouseholdID: householdA, SeasonID: testSeason, Status: "billed"}}
	in.settings.DefaultRequiredHours = 5
	if row := in.row(season, hh); row.Status != ComplianceBoughtOut || row.BuyoutDueCents != 0 || row.Buyout == nil || row.Buyout.Status != "billed" {
		t.Fatalf("billed: %+v", row)
	}
}

func TestCov1xBillBuyoutPaths(t *testing.T) {
	w := cov1xBuildLedgerWorld(t)
	r, s, ctx := w.r, w.r.s, context.Background()
	in := BuyoutInput{SeasonID: testSeason, HouseholdID: householdA}

	if _, _, err := s.BillBuyout(ctx, coach(), testOrg, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdA, DueDate: "03/31/2026"}); !cov1xIsValidation(err, "due_date") {
		t.Fatalf("due date: %v", err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: "nope", HouseholdID: householdA}); !errors.Is(err, ErrSeasonNotFound) {
		t.Fatalf("season: %v", err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: "ghost"}); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("household: %v", err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: cov1xHHF}); !errors.Is(err, ErrBuyoutExists) {
		t.Fatalf("existing (pending) buyout: %v", err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: cov1xHHE}); !errors.Is(err, ErrNothingOwed) {
		t.Fatalf("nothing owed: %v", err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdB}); !errors.Is(err, ErrNothingOwed) {
		t.Fatalf("exempt households owe nothing: %v", err)
	}
	// Season 2 has hours (via org default) but no rate.
	hours := 3.0
	if _, err := s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{DefaultRequiredHours: &hours}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: cov1xSeason2, HouseholdID: householdB}); !errors.Is(err, ErrNoBuyoutRate) {
		t.Fatalf("no rate: %v", err)
	}
	r.fs.arm("listBuyouts")
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, in); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("ledger load failure: %v", err)
	}
	r.fs.disarm()
	r.fs.arm("reserveBuyout")
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, in); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("reserve failure: %v", err)
	}
	r.fs.disarm()
	if r.biller.calls != 0 {
		t.Fatalf("billing must not run before a reservation exists: %d calls", r.biller.calls)
	}

	// Billing failure releases the reservation so the buyout can be retried.
	r.biller.fail = errors.New("billing down")
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, in); err == nil || err.Error() != "billing down" {
		t.Fatalf("biller failure: %v", err)
	}
	if left, _ := s.ListBuyouts(ctx, admin(), testOrg, testSeason); len(left) != 1 || left[0].HouseholdID != cov1xHHF {
		t.Fatalf("reservation must be released: %+v", left)
	}
	// ...and a release failure is logged without masking the billing error.
	r.fs.arm("deleteBuyout")
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, in); err == nil || err.Error() != "billing down" {
		t.Fatalf("biller failure with release failure: %v", err)
	}
	if !strings.Contains(r.logs.String(), "release buyout reservation failed") {
		t.Fatalf("release failure must be logged: %s", r.logs.String())
	}
	r.fs.disarm()
	r.biller.fail = nil
	// The failed release left a reservation behind in the store: clean it up and retry.
	if err := r.fs.store.deleteBuyout(ctx, testOrg, r.lastBuyoutID(t, ctx, householdA)); err != nil {
		t.Fatal(err)
	}

	r.fs.arm("updateBuyout")
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, in); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("finalise failure: %v", err)
	}
	r.fs.disarm()
	if err := r.fs.store.deleteBuyout(ctx, testOrg, r.lastBuyoutID(t, ctx, householdA)); err != nil {
		t.Fatal(err)
	}

	b, res, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdA, Issue: true})
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "billed" || b.InvoiceID != "inv-1" || b.InvoiceItemID != "item-1" || b.AmountCents != 13125 || b.HoursShort != 5.25 || b.RateCentsPerHour != 2500 || res.InvoiceNumber != "VOL-1" {
		t.Fatalf("buyout: %+v %+v", b, res)
	}
	req := r.biller.last
	if req.AmountCents != 13125 || !req.Issue || req.DueDate != "2026-03-31" || req.HouseholdID != householdA || req.OrgID != testOrg || req.Principal == nil ||
		!strings.Contains(req.Description, "5.25 of 10 hours short at $25.00/hr") || !strings.Contains(req.Description, "2025-26") {
		t.Fatalf("invoice request: %+v", req)
	}
	rows, _ := s.SeasonLedger(ctx, admin(), testOrg, testSeason)
	for _, row := range rows {
		if row.HouseholdID == householdA && (row.Status != ComplianceBoughtOut || row.BuyoutDueCents != 0) {
			t.Fatalf("ledger after buyout: %+v", row)
		}
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, in); !errors.Is(err, ErrBuyoutExists) {
		t.Fatalf("a second buyout: %v", err)
	}
	all, err := s.ListBuyouts(ctx, admin(), testOrg, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("list: %+v %v", all, err)
	}
	r.fs.arm("listBuyouts")
	if _, err := s.ListBuyouts(ctx, admin(), testOrg, ""); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("list failure: %v", err)
	}
	r.fs.disarm()

	// Explicit due date and no-issue pass straight through.
	if err := r.fs.store.deleteBuyout(ctx, testOrg, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdA, DueDate: "2026-04-15"}); err != nil {
		t.Fatal(err)
	}
	if r.biller.last.DueDate != "2026-04-15" || r.biller.last.Issue {
		t.Fatalf("explicit due date: %+v", r.biller.last)
	}

	// Without a biller nothing is attempted.
	s.SetBiller(nil)
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, in); !errors.Is(err, ErrBillingUnavailable) {
		t.Fatalf("no biller: %v", err)
	}
}

// lastBuyoutID returns the id of the household's buyout row in the store.
func (r *cov1xRig) lastBuyoutID(t *testing.T, ctx context.Context, hh string) string {
	t.Helper()
	all, err := r.fs.store.listBuyouts(ctx, testOrg, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range all {
		if b.HouseholdID == hh {
			return b.ID
		}
	}
	t.Fatalf("no buyout row for %s", hh)
	return ""
}

func TestCov1xFormatHours(t *testing.T) {
	for in, want := range map[float64]string{0: "0", 3: "3", 2.5: "2.5", 10: "10", 5.25: "5.25", 100: "100", 0.1: "0.1"} {
		if got := formatHours(in); got != want {
			t.Fatalf("formatHours(%v) = %q want %q", in, got, want)
		}
	}
}

func TestCov1xEraseHouseholdData(t *testing.T) {
	r := cov1xNewRig(t)
	s, ctx := r.s, context.Background()
	job := mustJob(t, s, 5, 96*time.Hour)
	cov1xSignUp(t, s, householdA, job.ID, "Ann")
	cov1xSignUp(t, s, householdA, job.ID, "Al")
	cov1xSignUp(t, s, householdB, job.ID, "Bea")

	if n, err := s.EraseHouseholdData(ctx, " ", householdA); err != nil || n != 0 {
		t.Fatalf("blank org: %d %v", n, err)
	}
	if n, err := s.EraseHouseholdData(ctx, testOrg, " "); err != nil || n != 0 {
		t.Fatalf("blank household: %d %v", n, err)
	}
	if n, err := s.EraseHouseholdData(ctx, "other-org", householdA); err != nil || n != 0 {
		t.Fatalf("other org must be untouched: %d %v", n, err)
	}
	r.fs.arm("eraseHousehold")
	if _, err := s.EraseHouseholdData(ctx, testOrg, householdA); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("store failure: %v", err)
	}
	r.fs.disarm()
	if n, err := s.EraseHouseholdData(ctx, testOrg, householdA); err != nil || n != 2 {
		t.Fatalf("erase: %d %v", n, err)
	}
	if n, _ := s.EraseHouseholdData(ctx, testOrg, householdA); n != 0 {
		t.Fatalf("idempotent: %d", n)
	}
	left, _ := r.fs.store.listSignups(ctx, testOrg, SignupFilter{})
	if len(left) != 1 || left[0].HouseholdID != householdB {
		t.Fatalf("other households keep their signups: %+v", left)
	}
}
