package volunteer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

const (
	testOrg     = "org-vol"
	testSeason  = "11111111-1111-1111-1111-111111111111"
	householdA  = "hh-a"
	householdB  = "hh-b"
	teenSwimmer = "sw-teen"
	kidSwimmer  = "sw-kid"
)

var baseNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func admin() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "u-admin", OrgID: testOrg, Roles: []string{"admin"}}
}
func coach() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "u-coach", OrgID: testOrg, Roles: []string{"coach"}}
}
func parent(hh string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "u-parent-" + hh, OrgID: testOrg, HouseholdID: hh, Roles: []string{"parent"}}
}
func swimmer(hh string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "u-swim-" + hh, OrgID: testOrg, HouseholdID: hh, Roles: []string{"swimmer"}}
}

type fakeBiller struct {
	calls int32
	fail  error
	last  BuyoutInvoiceRequest
}

func (f *fakeBiller) CreateBuyoutInvoice(_ context.Context, req BuyoutInvoiceRequest) (BuyoutInvoiceResult, error) {
	atomic.AddInt32(&f.calls, 1)
	f.last = req
	if f.fail != nil {
		return BuyoutInvoiceResult{}, f.fail
	}
	return BuyoutInvoiceResult{InvoiceID: "inv-1", InvoiceItemID: "item-1", InvoiceNumber: "VOL-1", InvoiceStatus: "draft"}, nil
}

type fakeQueue struct {
	mu    sync.Mutex
	tasks []*workers.Task
	delay []time.Duration
}

func (q *fakeQueue) EnqueueIn(_ context.Context, d time.Duration, t *workers.Task, _ ...workers.Option) (*workers.TaskInfo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, t)
	q.delay = append(q.delay, d)
	return &workers.TaskInfo{ID: fmt.Sprint(len(q.tasks))}, nil
}

func newTestService(t *testing.T) (*Service, *fakeBiller, *fakeQueue, *[]ReminderEmail) {
	t.Helper()
	s := NewService()
	now := baseNow
	s.SetClock(func() time.Time { return now })
	s.SetSeasonLookup(func(orgID string) ([]SeasonRef, error) {
		return []SeasonRef{{ID: testSeason, Name: "2025–26", Status: "active", StartDate: "2025-09-01", EndDate: "2026-08-31"}}, nil
	})
	s.SetHouseholdDirectory(func(orgID string) ([]HouseholdRef, error) {
		return []HouseholdRef{
			{ID: householdA, Name: "Alpha", Email: "a@example.com", Status: "active"},
			{ID: householdB, Name: "Bravo", Email: "b@example.com", Status: "active"},
		}, nil
	})
	s.SetSwimmerLookup(func(orgID, id string) (SwimmerRef, bool) {
		switch id {
		case teenSwimmer:
			return SwimmerRef{HouseholdID: householdA, DateOfBirth: "2008-01-01", Name: "Teen"}, true
		case kidSwimmer:
			return SwimmerRef{HouseholdID: householdA, DateOfBirth: "2016-01-01", Name: "Kid"}, true
		}
		return SwimmerRef{}, false
	})
	s.SetMeetLookup(func(orgID, meetID string) (string, bool) { return "Spring Invite", meetID == "meet-1" })
	b := &fakeBiller{}
	s.SetBiller(b)
	q := &fakeQueue{}
	s.SetReminderQueue(q)
	sent := &[]ReminderEmail{}
	s.SetReminderSender(func(_ context.Context, e ReminderEmail) error {
		*sent = append(*sent, e)
		return nil
	})
	return s, b, q, sent
}

func mustJob(t *testing.T, s *Service, capacity int, startsIn time.Duration) *Job {
	t.Helper()
	job, err := s.CreateJob(context.Background(), coach(), testOrg, JobInput{
		MeetID: "meet-1", Role: "timer", Title: "Timer", StartsAt: baseNow.Add(startsIn),
		EndsAt: baseNow.Add(startsIn + 3*time.Hour), Capacity: capacity,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job
}

func TestCreateJobValidationAndRoles(t *testing.T) {
	s, _, _, _ := newTestService(t)
	ctx := context.Background()
	in := JobInput{MeetID: "meet-1", Role: "timer", Title: "Timer", StartsAt: baseNow.Add(72 * time.Hour), EndsAt: baseNow.Add(75*time.Hour + 20*time.Minute), Capacity: 4}
	if _, err := s.CreateJob(ctx, parent(householdA), testOrg, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent create: want forbidden, got %v", err)
	}
	job, err := s.CreateJob(ctx, coach(), testOrg, in)
	if err != nil {
		t.Fatalf("coach create: %v", err)
	}
	if job.HourCredit != 3.25 {
		t.Fatalf("hour credit derived from shift: want 3.25, got %v", job.HourCredit)
	}
	bad := in
	bad.MeetID = "missing"
	if _, err := s.CreateJob(ctx, admin(), testOrg, bad); !errors.Is(err, ErrMeetNotFound) {
		t.Fatalf("unknown meet: got %v", err)
	}
	bad = in
	bad.MeetID, bad.EventName = "", ""
	var ve *ValidationError
	if _, err := s.CreateJob(ctx, admin(), testOrg, bad); !errors.As(err, &ve) {
		t.Fatalf("missing anchor: want validation error, got %v", err)
	}
	bad = in
	bad.EndsAt = bad.StartsAt
	if _, err := s.CreateJob(ctx, admin(), testOrg, bad); !errors.As(err, &ve) {
		t.Fatalf("end before start: want validation error, got %v", err)
	}
	bad = in
	bad.Role = "lifeguard"
	if _, err := s.CreateJob(ctx, admin(), testOrg, bad); !errors.As(err, &ve) {
		t.Fatalf("bad role: want validation error, got %v", err)
	}
}

func TestSignupCapacityIsAtomicUnderConcurrency(t *testing.T) {
	s, _, _, _ := newTestService(t)
	job := mustJob(t, s, 3, 96*time.Hour)
	var wg sync.WaitGroup
	var ok, full int32
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hh := householdA
			if i%2 == 1 {
				hh = householdB
			}
			_, err := s.SignUp(context.Background(), parent(hh), testOrg, job.ID, SignupInput{VolunteerName: fmt.Sprintf("Volunteer %d", i)})
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, ErrJobFull):
				atomic.AddInt32(&full, 1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if ok != 3 || full != 37 {
		t.Fatalf("want 3 ok / 37 full, got %d / %d", ok, full)
	}
	view, err := s.GetJob(context.Background(), coach(), testOrg, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.FilledCount != 3 || view.OpenSpots != 0 || len(view.Signups) != 3 {
		t.Fatalf("unexpected fill: %+v", view)
	}
}

func TestSignupRules(t *testing.T) {
	s, _, q, _ := newTestService(t)
	ctx := context.Background()
	job := mustJob(t, s, 5, 96*time.Hour)

	if _, err := s.SignUp(ctx, swimmer(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Sam"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer signup: want forbidden, got %v", err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Pat", HouseholdID: householdB}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent signing up other household: want forbidden, got %v", err)
	}
	su, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "  Pat   Alpha "})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if su.VolunteerName != "Pat Alpha" || su.HouseholdID != householdA || !su.CanCancel {
		t.Fatalf("unexpected signup: %+v", su)
	}
	if len(q.tasks) != 1 || q.delay[0] != 72*time.Hour {
		t.Fatalf("reminder: want one task delayed 72h, got %d %v", len(q.tasks), q.delay)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "pat alpha"}); !errors.Is(err, ErrDuplicateSignup) {
		t.Fatalf("duplicate person: got %v", err)
	}
	// Staff must name the household and it must exist.
	if _, err := s.SignUp(ctx, coach(), testOrg, job.ID, SignupInput{VolunteerName: "Bo"}); err == nil {
		t.Fatal("staff signup without household should fail")
	}
	if _, err := s.SignUp(ctx, coach(), testOrg, job.ID, SignupInput{VolunteerName: "Bo", HouseholdID: "nope"}); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("staff unknown household: got %v", err)
	}
	if _, err := s.SignUp(ctx, coach(), testOrg, job.ID, SignupInput{VolunteerName: "Bo", HouseholdID: householdB}); err != nil {
		t.Fatalf("staff signup: %v", err)
	}

	// Certification and minimum age.
	official, err := s.CreateJob(ctx, admin(), testOrg, JobInput{EventName: "Clinic", Role: "official", Title: "S&T", StartsAt: baseNow.Add(80 * time.Hour), EndsAt: baseNow.Add(84 * time.Hour), Capacity: 2, MinAge: 16, RequiresCertification: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, official.ID, SignupInput{VolunteerName: "Pat"}); !errors.Is(err, ErrCertificationRequired) {
		t.Fatalf("certification: got %v", err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, official.ID, SignupInput{VolunteerName: "Kid", SwimmerID: kidSwimmer, Certified: true}); !errors.Is(err, ErrAgeRequirement) {
		t.Fatalf("min age: got %v", err)
	}
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, official.ID, SignupInput{VolunteerName: "Teen", SwimmerID: teenSwimmer, Certified: true}); err != nil {
		t.Fatalf("teen signup: %v", err)
	}
	if _, err := s.SignUp(ctx, parent(householdB), testOrg, official.ID, SignupInput{VolunteerName: "Teen", SwimmerID: teenSwimmer, Certified: true}); err == nil {
		t.Fatal("swimmer from another household must be rejected")
	}

	// Started jobs are closed.
	past := mustJob(t, s, 2, -time.Hour)
	if _, err := s.SignUp(ctx, parent(householdA), testOrg, past.ID, SignupInput{VolunteerName: "Pat"}); !errors.Is(err, ErrJobStarted) {
		t.Fatalf("started job: got %v", err)
	}
}

func TestCancellationCutoff(t *testing.T) {
	s, _, _, _ := newTestService(t)
	ctx := context.Background()
	soon := mustJob(t, s, 4, 24*time.Hour) // inside default 48h cutoff
	later := mustJob(t, s, 4, 72*time.Hour)

	a1, _ := s.SignUp(ctx, parent(householdA), testOrg, soon.ID, SignupInput{VolunteerName: "Pat"})
	a2, _ := s.SignUp(ctx, parent(householdA), testOrg, later.ID, SignupInput{VolunteerName: "Pat"})
	if a1.CanCancel {
		t.Fatal("can_cancel should be false inside cutoff")
	}
	if _, _, err := s.Cancel(ctx, parent(householdA), testOrg, a1.ID, ""); !errors.Is(err, ErrCancellationClosed) {
		t.Fatalf("parent inside cutoff: got %v", err)
	}
	if _, _, err := s.Cancel(ctx, parent(householdB), testOrg, a2.ID, ""); !errors.Is(err, ErrSignupNotFound) {
		t.Fatalf("foreign household cancel: got %v", err)
	}
	if _, _, err := s.Cancel(ctx, swimmer(householdA), testOrg, a2.ID, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer cancel: got %v", err)
	}
	if _, su, err := s.Cancel(ctx, parent(householdA), testOrg, a2.ID, "sick"); err != nil || su.Status != StatusCancelled {
		t.Fatalf("parent cancel outside cutoff: %v %+v", err, su)
	}
	if _, _, err := s.Cancel(ctx, parent(householdA), testOrg, a2.ID, ""); !errors.Is(err, ErrSignupNotActive) {
		t.Fatalf("double cancel: got %v", err)
	}
	// Staff override the cutoff.
	if _, _, err := s.Cancel(ctx, coach(), testOrg, a1.ID, "swap"); err != nil {
		t.Fatalf("coach cancel inside cutoff: %v", err)
	}
	// Admin can tighten the cutoff to zero hours.
	zero := 0
	if _, err := s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{CancellationCutoffHours: &zero}); err != nil {
		t.Fatal(err)
	}
	a3, _ := s.SignUp(ctx, parent(householdA), testOrg, soon.ID, SignupInput{VolunteerName: "Pat"})
	if _, _, err := s.Cancel(ctx, parent(householdA), testOrg, a3.ID, ""); err != nil {
		t.Fatalf("cancel with zero cutoff: %v", err)
	}
	// A cancelled spot frees capacity.
	view, _ := s.GetJob(ctx, admin(), testOrg, soon.ID)
	if view.FilledCount != 0 {
		t.Fatalf("filled after cancels: %d", view.FilledCount)
	}
}

func TestLedgerCheckInCreditsAndBuyout(t *testing.T) {
	s, biller, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := s.UpsertRequirement(ctx, coach(), testOrg, RequirementInput{SeasonID: testSeason, RequiredHours: 10}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach requirement: got %v", err)
	}
	if _, err := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: testSeason, RequiredHours: 10, BuyoutCentsPerHour: 2500}); err != nil {
		t.Fatal(err)
	}
	// Household B is exempt via an override.
	if _, err := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: testSeason, HouseholdID: householdB, RequiredHours: 0}); err != nil {
		t.Fatal(err)
	}
	job := mustJob(t, s, 4, 72*time.Hour) // 3h credit
	worked, _ := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Pat"})
	scheduled, _ := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Chris"})
	_ = scheduled

	if _, _, err := s.CheckIn(ctx, parent(householdA), testOrg, worked.ID, StatusWorked, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent check-in: got %v", err)
	}
	hours := 3.5
	if _, su, err := s.CheckIn(ctx, coach(), testOrg, worked.ID, StatusWorked, &hours); err != nil || su.HoursCredited == nil || *su.HoursCredited != 3.5 {
		t.Fatalf("coach check-in: %v %+v", err, su)
	}
	if _, err := s.AddCredit(ctx, coach(), testOrg, CreditInput{SeasonID: testSeason, HouseholdID: householdA, Hours: 1.25, Reason: "Pool cleanup day"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCredit(ctx, parent(householdA), testOrg, CreditInput{SeasonID: testSeason, HouseholdID: householdA, Hours: 5, Reason: "x"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent credit: got %v", err)
	}

	rows, err := s.SeasonLedger(ctx, admin(), testOrg, testSeason)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	a := rows[0]
	if a.HouseholdID != householdA || a.WorkedHours != 3.5 || a.CreditedHours != 1.25 || a.EarnedHours != 4.75 ||
		a.ScheduledHours != 3 || a.RemainingHours != 5.25 || a.Status != ComplianceShort || a.BuyoutDueCents != 13125 || a.RequirementSource != "season" {
		t.Fatalf("household A row: %+v", a)
	}
	if rows[1].Status != ComplianceExempt || rows[1].RequirementSource != "household" {
		t.Fatalf("household B row: %+v", rows[1])
	}
	if _, err := s.SeasonLedger(ctx, parent(householdA), testOrg, testSeason); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent season ledger: got %v", err)
	}
	own, err := s.HouseholdLedger(ctx, parent(householdA), testOrg, householdA)
	if err != nil || len(own) != 1 || own[0].EarnedHours != 4.75 {
		t.Fatalf("parent own ledger: %v %+v", err, own)
	}
	if _, err := s.HouseholdLedger(ctx, parent(householdA), testOrg, householdB); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent foreign ledger: got %v", err)
	}
	if _, err := s.HouseholdLedger(ctx, swimmer(householdA), testOrg, householdA); err != nil {
		t.Fatalf("swimmer own ledger: %v", err)
	}

	// Buyout: coach denied, admin bills once, second attempt conflicts.
	if _, _, err := s.BillBuyout(ctx, coach(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdA}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach buyout: got %v", err)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdB}); !errors.Is(err, ErrNothingOwed) {
		t.Fatalf("exempt buyout: got %v", err)
	}
	biller.fail = errors.New("stripe down")
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdA}); err == nil {
		t.Fatal("billing failure should surface")
	}
	biller.fail = nil
	b, inv, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdA, Issue: true})
	if err != nil {
		t.Fatalf("buyout after released reservation: %v", err)
	}
	if b.AmountCents != 13125 || b.Status != "billed" || inv.InvoiceID != "inv-1" || !biller.last.Issue || biller.last.AmountCents != 13125 {
		t.Fatalf("buyout: %+v %+v %+v", b, inv, biller.last)
	}
	if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdA}); !errors.Is(err, ErrBuyoutExists) {
		t.Fatalf("double buyout: got %v", err)
	}
	rows, _ = s.SeasonLedger(ctx, admin(), testOrg, testSeason)
	if rows[0].Status != ComplianceBoughtOut && rows[1].Status != ComplianceBoughtOut {
		t.Fatalf("expected bought_out status: %+v", rows)
	}
	// A worked signup blocks job deletion.
	if _, err := s.DeleteJob(ctx, admin(), testOrg, job.ID); !errors.Is(err, ErrJobHasWorkedSignups) {
		t.Fatalf("delete job with worked signups: got %v", err)
	}
}

func TestConcurrentBuyoutBillsOnce(t *testing.T) {
	s, biller, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := s.UpsertRequirement(ctx, admin(), testOrg, RequirementInput{SeasonID: testSeason, RequiredHours: 4, BuyoutCentsPerHour: 1000}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var ok int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := s.BillBuyout(ctx, admin(), testOrg, BuyoutInput{SeasonID: testSeason, HouseholdID: householdA}); err == nil {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || biller.calls != 1 {
		t.Fatalf("want exactly one buyout billed, got ok=%d calls=%d", ok, biller.calls)
	}
}

func TestUpdateJobCapacityAndReschedule(t *testing.T) {
	s, _, q, _ := newTestService(t)
	ctx := context.Background()
	job := mustJob(t, s, 3, 96*time.Hour)
	_, _ = s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "A"})
	_, _ = s.SignUp(ctx, parent(householdB), testOrg, job.ID, SignupInput{VolunteerName: "B"})
	one := 1
	if _, _, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{Capacity: &one}); !errors.Is(err, ErrCapacityBelowFilled) {
		t.Fatalf("shrink below filled: got %v", err)
	}
	newStart := job.StartsAt.Add(24 * time.Hour)
	newEnd := job.EndsAt.Add(24 * time.Hour)
	before := len(q.tasks)
	if _, _, err := s.UpdateJob(ctx, coach(), testOrg, job.ID, JobUpdate{StartsAt: &newStart, EndsAt: &newEnd}); err != nil {
		t.Fatal(err)
	}
	if len(q.tasks)-before != 2 {
		t.Fatalf("reschedule should enqueue 2 reminders, got %d", len(q.tasks)-before)
	}
	if _, _, err := s.UpdateJob(ctx, parent(householdA), testOrg, job.ID, JobUpdate{Capacity: &one}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent update: got %v", err)
	}
}

func TestShiftReminderSkipsStaleAndCancelled(t *testing.T) {
	s, _, q, sent := newTestService(t)
	ctx := context.Background()
	job := mustJob(t, s, 3, 96*time.Hour)
	su, _ := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Pat"})
	var p ShiftReminderPayload
	if err := json.Unmarshal(q.tasks[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if err := s.ReminderHandler()(ctx, q.tasks[0]); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != 1 || (*sent)[0].To != "a@example.com" {
		t.Fatalf("reminder not sent: %+v", *sent)
	}
	stale := p
	stale.StartsAtUnix++
	if ok, reason, _ := s.SendShiftReminder(ctx, stale); ok || reason != "job_rescheduled" {
		t.Fatalf("stale reminder: %v %s", ok, reason)
	}
	if _, _, err := s.Cancel(ctx, parent(householdA), testOrg, su.ID, ""); err != nil {
		t.Fatal(err)
	}
	if ok, reason, _ := s.SendShiftReminder(ctx, p); ok || reason != "signup_cancelled" {
		t.Fatalf("cancelled reminder: %v %s", ok, reason)
	}
}

func TestBulkCreateFromTemplate(t *testing.T) {
	s, _, _, _ := newTestService(t)
	ctx := context.Background()
	shifts := []Shift{
		{StartsAt: baseNow.Add(72 * time.Hour), EndsAt: baseNow.Add(76 * time.Hour), Label: "Sat AM"},
		{StartsAt: baseNow.Add(78 * time.Hour), EndsAt: baseNow.Add(82 * time.Hour), Label: "Sat PM"},
	}
	jobs, err := s.BulkCreateJobs(ctx, coach(), testOrg, BulkInput{MeetID: "meet-1", Shifts: shifts})
	if err != nil {
		t.Fatal(err)
	}
	if want := 2 * len(Templates()[0].Roles); len(jobs) != want {
		t.Fatalf("want %d jobs, got %d", want, len(jobs))
	}
	if jobs[0].Title != "Timer · Sat AM" || jobs[0].HourCredit != 4 {
		t.Fatalf("unexpected first job: %+v", jobs[0])
	}
	list, _ := s.ListJobs(ctx, parent(householdA), testOrg, JobFilter{MeetID: "meet-1"})
	if len(list) != len(jobs) || list[0].MeetName != "Spring Invite" || list[0].EffectiveSeasonID != testSeason {
		t.Fatalf("list: %d %+v", len(list), list[0])
	}
	if _, err := s.BulkCreateJobs(ctx, parent(householdA), testOrg, BulkInput{MeetID: "meet-1", Shifts: shifts}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent bulk: got %v", err)
	}
	if _, err := s.BulkCreateJobs(ctx, admin(), testOrg, BulkInput{MeetID: "meet-1", TemplateID: "nope", Shifts: shifts}); err == nil {
		t.Fatal("unknown template should fail")
	}
}

func TestListSignupsHouseholdScoping(t *testing.T) {
	s, _, _, _ := newTestService(t)
	ctx := context.Background()
	job := mustJob(t, s, 5, 96*time.Hour)
	_, _ = s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "A"})
	_, _ = s.SignUp(ctx, parent(householdB), testOrg, job.ID, SignupInput{VolunteerName: "B"})
	mine, err := s.ListSignups(ctx, swimmer(householdA), testOrg, SignupFilter{})
	if err != nil || len(mine) != 1 || mine[0].HouseholdID != householdA {
		t.Fatalf("swimmer scoped list: %v %+v", err, mine)
	}
	if _, err := s.ListSignups(ctx, parent(householdA), testOrg, SignupFilter{HouseholdID: householdB}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent foreign filter: got %v", err)
	}
	all, _ := s.ListSignups(ctx, coach(), testOrg, SignupFilter{})
	if len(all) != 2 {
		t.Fatalf("coach list: %d", len(all))
	}
	// Parents browsing a job never see the roster.
	view, _ := s.GetJob(ctx, parent(householdA), testOrg, job.ID)
	if len(view.Signups) != 0 || view.FilledCount != 2 {
		t.Fatalf("parent job view leaked roster: %+v", view)
	}
	if _, err := s.ListJobs(ctx, &appmiddleware.Principal{UserID: "x", Roles: []string{"guest"}}, testOrg, JobFilter{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unknown role: got %v", err)
	}
}
