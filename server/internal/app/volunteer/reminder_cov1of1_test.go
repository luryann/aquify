package volunteer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/workers"
)

func cov1xReminderJobAndSignup(t *testing.T, r *cov1xRig) (*Job, *SignupView, ShiftReminderPayload) {
	t.Helper()
	job := mustJob(t, r.s, 3, 96*time.Hour)
	su := cov1xSignUp(t, r.s, householdA, job.ID, "Pat")
	if len(r.q.tasks) != 1 {
		t.Fatalf("sign up should schedule exactly one reminder, got %d", len(r.q.tasks))
	}
	var p ShiftReminderPayload
	if err := json.Unmarshal(r.q.tasks[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.OrgID != testOrg || p.SignupID != su.ID || p.StartsAtUnix != job.StartsAt.Unix() {
		t.Fatalf("payload pins org, signup and shift time: %+v", p)
	}
	return job, su, p
}

func TestCov1xScheduleReminderBranches(t *testing.T) {
	r := cov1xNewRig(t)
	ctx := context.Background()
	job := &Job{ID: "j", OrgID: testOrg, StartsAt: baseNow.Add(96 * time.Hour)}
	su := &Signup{ID: "s", Status: StatusConfirmed}

	// No queue, no transport, or a non-confirmed signup: nothing is enqueued.
	r.s.SetReminderQueue(nil)
	r.s.scheduleReminder(ctx, job, su)
	r.s.SetReminderQueue(r.q)
	r.s.SetReminderSender(nil)
	r.s.scheduleReminder(ctx, job, su)
	r.s.SetReminderSender(func(context.Context, ReminderEmail) error { return nil })
	r.s.scheduleReminder(ctx, job, &Signup{ID: "s", Status: StatusCancelled})
	if len(r.q.tasks) != 0 {
		t.Fatalf("guards must not enqueue: %d", len(r.q.tasks))
	}

	// Settings lookup failure is logged, never fatal.
	r.fs.arm("getSettings")
	r.s.scheduleReminder(ctx, job, su)
	r.fs.disarm()
	if len(r.q.tasks) != 0 || !strings.Contains(r.logs.String(), "settings lookup failed") {
		t.Fatalf("settings failure should warn and skip; logs=%s", r.logs.String())
	}

	// Reminders disabled for the org.
	off := false
	if _, err := r.s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{RemindersEnabled: &off}); err != nil {
		t.Fatal(err)
	}
	r.s.scheduleReminder(ctx, job, su)
	if len(r.q.tasks) != 0 {
		t.Fatal("disabled reminders must not enqueue")
	}
	on := true
	if _, err := r.s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{RemindersEnabled: &on}); err != nil {
		t.Fatal(err)
	}

	// Inside the 24h window: the on-screen confirmation is the reminder.
	r.s.scheduleReminder(ctx, &Job{ID: "j", OrgID: testOrg, StartsAt: baseNow.Add(24*time.Hour + 30*time.Second)}, su)
	if len(r.q.tasks) != 0 {
		t.Fatal("signup inside reminder window must not enqueue")
	}

	// Enqueue failure is logged and swallowed.
	r.q.err = errCov1xBoom
	r.s.scheduleReminder(ctx, job, su)
	if len(r.q.tasks) != 0 || !strings.Contains(r.logs.String(), "enqueue shift reminder failed") {
		t.Fatalf("enqueue failure should be logged; logs=%s", r.logs.String())
	}
	r.q.err = nil

	// Happy path fires 24h before the shift.
	r.s.scheduleReminder(ctx, job, su)
	if len(r.q.tasks) != 1 || r.q.delay[0] != 72*time.Hour {
		t.Fatalf("want one task in 72h, got %d %v", len(r.q.tasks), r.q.delay)
	}
	if r.q.tasks[0].Type != TaskTypeShiftReminder {
		t.Fatalf("task type %q", r.q.tasks[0].Type)
	}
}

func TestCov1xReminderHandler(t *testing.T) {
	r := cov1xNewRig(t)
	ctx := context.Background()
	_, su, p := cov1xReminderJobAndSignup(t, r)
	h := r.s.ReminderHandler()

	// Malformed payload.
	err := h(ctx, &workers.Task{Type: TaskTypeShiftReminder, Payload: []byte("{not json")})
	if err == nil || !strings.Contains(err.Error(), "decode payload") {
		t.Fatalf("bad payload: %v", err)
	}
	// Missing identifiers surface the send error.
	bad, _ := json.Marshal(ShiftReminderPayload{OrgID: testOrg})
	err = h(ctx, &workers.Task{Type: TaskTypeShiftReminder, Payload: bad})
	if err == nil || !strings.Contains(err.Error(), "missing org_id or signup_id") {
		t.Fatalf("missing ids: %v", err)
	}
	if !strings.Contains(r.logs.String(), "send failed") {
		t.Fatalf("handler should log failure: %s", r.logs.String())
	}
	// Valid payload sends an email.
	if err := h(ctx, r.q.tasks[0]); err != nil {
		t.Fatal(err)
	}
	if len(*r.sent) != 1 || (*r.sent)[0].To != "a@example.com" || (*r.sent)[0].OrgID != testOrg {
		t.Fatalf("sent: %+v", *r.sent)
	}
	if !strings.Contains(r.logs.String(), "processed") {
		t.Fatalf("handler should log success: %s", r.logs.String())
	}
	if p.SignupID != su.ID {
		t.Fatalf("handler payload must target the signup: %+v", p)
	}
}

func TestCov1xSendShiftReminderSkipReasons(t *testing.T) {
	r := cov1xNewRig(t)
	ctx := context.Background()
	job, su, p := cov1xReminderJobAndSignup(t, r)

	// Missing identifiers.
	if ok, _, err := r.s.SendShiftReminder(ctx, ShiftReminderPayload{SignupID: su.ID}); ok || err == nil {
		t.Fatalf("missing org: %v %v", ok, err)
	}
	if ok, _, err := r.s.SendShiftReminder(ctx, ShiftReminderPayload{OrgID: testOrg}); ok || err == nil {
		t.Fatalf("missing signup: %v %v", ok, err)
	}
	// Unknown signup.
	missing := p
	missing.SignupID = "00000000-0000-0000-0000-00000000dead"
	if ok, reason, err := r.s.SendShiftReminder(ctx, missing); ok || reason != "signup_deleted" || err != nil {
		t.Fatalf("deleted signup: %v %q %v", ok, reason, err)
	}
	// Signup lookup failure propagates.
	r.fs.arm("getSignup")
	if ok, _, err := r.s.SendShiftReminder(ctx, p); ok || !errors.Is(err, errCov1xBoom) {
		t.Fatalf("getSignup failure: %v %v", ok, err)
	}
	r.fs.disarm()
	// Job gone / job lookup failure.
	r.fs.armWith("getJob", ErrJobNotFound)
	if ok, reason, err := r.s.SendShiftReminder(ctx, p); ok || reason != "job_deleted" || err != nil {
		t.Fatalf("deleted job: %v %q %v", ok, reason, err)
	}
	r.fs.arm("getJob")
	if _, _, err := r.s.SendShiftReminder(ctx, p); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("getJob failure: %v", err)
	}
	r.fs.disarm()
	// Shift already started.
	*r.now = job.StartsAt.Add(time.Minute)
	if ok, reason, _ := r.s.SendShiftReminder(ctx, p); ok || reason != "shift_started" {
		t.Fatalf("started: %v %q", ok, reason)
	}
	*r.now = baseNow
	// Settings failure propagates.
	r.fs.arm("getSettings")
	if _, _, err := r.s.SendShiftReminder(ctx, p); !errors.Is(err, errCov1xBoom) {
		t.Fatalf("settings failure: %v", err)
	}
	r.fs.disarm()
	// No email transport.
	r.s.SetReminderSender(nil)
	if ok, reason, _ := r.s.SendShiftReminder(ctx, p); ok || reason != "no_transport" {
		t.Fatalf("transport: %v %q", ok, reason)
	}
	// Reminders disabled wins before transport.
	off := false
	if _, err := r.s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{RemindersEnabled: &off}); err != nil {
		t.Fatal(err)
	}
	if ok, reason, _ := r.s.SendShiftReminder(ctx, p); ok || reason != "reminders_disabled" {
		t.Fatalf("disabled: %v %q", ok, reason)
	}
	on := true
	if _, err := r.s.UpdateSettings(ctx, admin(), testOrg, SettingsUpdate{RemindersEnabled: &on}); err != nil {
		t.Fatal(err)
	}
	// Household has no usable email.
	r.s.SetReminderSender(func(context.Context, ReminderEmail) error { return nil })
	r.s.SetHouseholdDirectory(func(string) ([]HouseholdRef, error) {
		return []HouseholdRef{{ID: householdA, Name: "Alpha", Email: "   "}}, nil
	})
	if ok, reason, _ := r.s.SendShiftReminder(ctx, p); ok || reason != "no_household_email" {
		t.Fatalf("no email: %v %q", ok, reason)
	}
	// Transport failure propagates.
	r.s.SetHouseholdDirectory(func(string) ([]HouseholdRef, error) {
		return []HouseholdRef{{ID: householdA, Name: "Alpha", Email: " a@example.com "}}, nil
	})
	r.s.SetReminderSender(func(context.Context, ReminderEmail) error { return errCov1xBoom })
	if ok, _, err := r.s.SendShiftReminder(ctx, p); ok || !errors.Is(err, errCov1xBoom) {
		t.Fatalf("send failure: %v %v", ok, err)
	}
	// Success trims the address.
	var got ReminderEmail
	r.s.SetReminderSender(func(_ context.Context, e ReminderEmail) error { got = e; return nil })
	if ok, reason, err := r.s.SendShiftReminder(ctx, p); !ok || reason != "sent" || err != nil {
		t.Fatalf("send: %v %q %v", ok, reason, err)
	}
	if got.To != "a@example.com" || !strings.Contains(got.Subject, "Timer") || !strings.Contains(got.TextBody, "Spring Invite") {
		t.Fatalf("email: %+v", got)
	}
}

func TestCov1xMeetLabelAndRenderReminder(t *testing.T) {
	r := cov1xNewRig(t)
	withMeet := &Job{OrgID: testOrg, MeetID: "meet-1", EventName: "Fallback"}
	if got := r.s.meetLabel(withMeet); got != "Spring Invite" {
		t.Fatalf("meet name should win: %q", got)
	}
	unknown := &Job{OrgID: testOrg, MeetID: "gone", EventName: "Fallback"}
	if got := r.s.meetLabel(unknown); got != "Fallback" {
		t.Fatalf("unknown meet falls back to event name: %q", got)
	}
	r.s.SetMeetLookup(func(string, string) (string, bool) { return "", true })
	if got := r.s.meetLabel(withMeet); got != "Fallback" {
		t.Fatalf("empty meet name falls back: %q", got)
	}
	r.s.SetMeetLookup(nil)
	if got := r.s.meetLabel(withMeet); got != "Fallback" {
		t.Fatalf("no lookup falls back: %q", got)
	}

	start := time.Date(2026, 3, 7, 14, 0, 0, 0, time.UTC)
	job := &Job{Title: "Starter", StartsAt: start, EndsAt: start.Add(150 * time.Minute), HourCredit: 2.5,
		Location: "Main deck", Description: "Bring a clipboard"}
	su := &Signup{VolunteerName: "Pat"}
	em := RenderReminder(job, su, "Spring Invite", "America/New_York")
	if em.Subject != "Volunteer reminder: Starter tomorrow" {
		t.Fatalf("subject %q", em.Subject)
	}
	for _, want := range []string{"Hi Pat,", "Job: Starter", "Event: Spring Invite", "Sat Mar 7, 9:00 AM", "EST", "Where: Main deck", "Credit: 2.5 hours", "Bring a clipboard"} {
		if !strings.Contains(em.TextBody, want) {
			t.Errorf("body missing %q:\n%s", want, em.TextBody)
		}
	}
	bare := RenderReminder(&Job{Title: "Timer", StartsAt: start, EndsAt: start.Add(time.Hour), HourCredit: 1}, su, "", "Not/AZone")
	if strings.Contains(bare.TextBody, "Event:") || strings.Contains(bare.TextBody, "Where:") {
		t.Fatalf("optional lines must be omitted:\n%s", bare.TextBody)
	}
	if !strings.Contains(bare.TextBody, "2:00 PM") || !strings.Contains(bare.TextBody, "UTC") {
		t.Fatalf("invalid tz falls back to UTC:\n%s", bare.TextBody)
	}
	blank := RenderReminder(&Job{Title: "Timer", StartsAt: start, EndsAt: start.Add(time.Hour)}, su, "", "  ")
	if !strings.Contains(blank.TextBody, "UTC") {
		t.Fatalf("blank tz falls back to UTC:\n%s", blank.TextBody)
	}
}
