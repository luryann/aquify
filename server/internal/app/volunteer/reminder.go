package volunteer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/workers"
)

// TaskTypeShiftReminder is the background job that emails a volunteer the day
// before their shift. It runs on the email queue.
const TaskTypeShiftReminder = "email:volunteer_shift_reminder"

// reminderLead is how long before a shift the reminder fires.
const reminderLead = 24 * time.Hour

// ShiftReminderPayload is the job payload. StartsAtUnix pins the reminder to
// the shift time it was scheduled for; a rescheduled job enqueues a fresh
// reminder and the stale one becomes a no-op.
type ShiftReminderPayload struct {
	OrgID        string `json:"org_id"`
	SignupID     string `json:"signup_id"`
	StartsAtUnix int64  `json:"starts_at_unix"`
}

// scheduleReminder enqueues a reminder for a confirmed signup. Failures are
// logged and never block the signup itself.
func (s *Service) scheduleReminder(ctx context.Context, job *Job, su *Signup) {
	if s.queue == nil || s.sendEmail == nil || su.Status != StatusConfirmed {
		return
	}
	settings, err := s.settings(ctx, job.OrgID)
	if err != nil {
		s.logger.Warn("volunteer: shift reminder not scheduled; settings lookup failed",
			"signup_id", su.ID, "job_id", job.ID, "error", err)
		return
	}
	if !settings.RemindersEnabled {
		return
	}
	delay := job.StartsAt.Add(-reminderLead).Sub(s.now())
	if delay < time.Minute {
		// Signed up inside the reminder window: the confirmation on screen is
		// the reminder. Avoid an immediate duplicate email.
		return
	}
	task, err := workers.NewTask(TaskTypeShiftReminder, ShiftReminderPayload{
		OrgID: job.OrgID, SignupID: su.ID, StartsAtUnix: job.StartsAt.Unix(),
	})
	if err != nil {
		s.logger.Error("volunteer: shift reminder dropped; building task failed", "signup_id", su.ID, "job_id", job.ID, "error", err)
		return
	}
	key := fmt.Sprintf("volunteer-reminder:%s:%d", su.ID, job.StartsAt.Unix())
	if _, err := s.queue.EnqueueIn(ctx, delay, task, workers.WithQueue(workers.QueueEmail), workers.WithUniqueKey(key)); err != nil {
		s.logger.Warn("volunteer: enqueue shift reminder failed", "signup_id", su.ID, "job_id", job.ID, "error", err)
		return
	}
	s.logger.Info("volunteer: shift reminder scheduled", "signup_id", su.ID, "job_id", job.ID, "fires_in", delay.Round(time.Minute).String())
}

// ReminderHandler processes TaskTypeShiftReminder jobs.
func (s *Service) ReminderHandler() workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p ShiftReminderPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("volunteer reminder: decode payload: %w", err)
		}
		sent, reason, err := s.SendShiftReminder(ctx, p)
		if err != nil {
			s.logger.Error("volunteer reminder: send failed", "signup_id", p.SignupID, "error", err)
			return err
		}
		s.logger.Info("volunteer reminder: processed", "signup_id", p.SignupID, "sent", sent, "reason", reason)
		return nil
	}
}

// SendShiftReminder sends one reminder if the signup is still live and the
// shift has not moved. It returns (false, reason, nil) for intentional skips.
func (s *Service) SendShiftReminder(ctx context.Context, p ShiftReminderPayload) (bool, string, error) {
	if p.OrgID == "" || p.SignupID == "" {
		return false, "", fmt.Errorf("volunteer reminder: missing org_id or signup_id")
	}
	su, err := s.store.getSignup(ctx, p.OrgID, p.SignupID)
	if err == ErrSignupNotFound {
		return false, "signup_deleted", nil
	}
	if err != nil {
		return false, "", err
	}
	if su.Status != StatusConfirmed {
		return false, "signup_" + su.Status, nil
	}
	job, err := s.store.getJob(ctx, p.OrgID, su.JobID)
	if err == ErrJobNotFound {
		return false, "job_deleted", nil
	}
	if err != nil {
		return false, "", err
	}
	if job.StartsAt.Unix() != p.StartsAtUnix {
		return false, "job_rescheduled", nil
	}
	if !job.StartsAt.After(s.now()) {
		return false, "shift_started", nil
	}
	settings, err := s.settings(ctx, p.OrgID)
	if err != nil {
		return false, "", err
	}
	if !settings.RemindersEnabled {
		return false, "reminders_disabled", nil
	}
	if s.sendEmail == nil {
		return false, "no_transport", nil
	}
	var to string
	for _, h := range s.householdList(p.OrgID) {
		if h.ID == su.HouseholdID {
			to = strings.TrimSpace(h.Email)
		}
	}
	if to == "" {
		return false, "no_household_email", nil
	}
	email := RenderReminder(job, su, s.meetLabel(job), settings.Timezone)
	email.OrgID = p.OrgID
	email.To = to
	if err := s.sendEmail(ctx, email); err != nil {
		return false, "", err
	}
	return true, "sent", nil
}

func (s *Service) meetLabel(job *Job) string {
	if job.MeetID != "" && s.meetName != nil {
		if name, ok := s.meetName(job.OrgID, job.MeetID); ok && name != "" {
			return name
		}
	}
	return job.EventName
}

// RenderReminder builds the reminder email body.
func RenderReminder(job *Job, su *Signup, eventLabel, timezone string) ReminderEmail {
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil || strings.TrimSpace(timezone) == "" {
		loc = time.UTC
	}
	when := job.StartsAt.In(loc).Format("Mon Jan 2, 3:04 PM") + " – " + job.EndsAt.In(loc).Format("3:04 PM MST")
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\n", su.VolunteerName)
	fmt.Fprintf(&b, "This is a reminder that you are signed up to volunteer tomorrow.\n\n")
	fmt.Fprintf(&b, "Job: %s\n", job.Title)
	if eventLabel != "" {
		fmt.Fprintf(&b, "Event: %s\n", eventLabel)
	}
	fmt.Fprintf(&b, "When: %s\n", when)
	if job.Location != "" {
		fmt.Fprintf(&b, "Where: %s\n", job.Location)
	}
	fmt.Fprintf(&b, "Credit: %s hours\n", formatHours(job.HourCredit))
	if job.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", job.Description)
	}
	fmt.Fprintf(&b, "\nIf you can no longer make it, please contact the club as soon as possible.\n")
	return ReminderEmail{
		Subject:  "Volunteer reminder: " + job.Title + " tomorrow",
		TextBody: b.String(),
	}
}
