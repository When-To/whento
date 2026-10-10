// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/notify/models"
)

// The probes in this file are the acceptance spec of the reminder delivery
// repairs: calendar-local candidate horizons (an event in a zone east of UTC
// must not be lost), transient errors that must never cancel an event, and the
// SMTP-absence policy that suppresses email while leaving chat channels alone.

// TestReminderHorizonTokyo168Hours is the original defect: with hours_before at
// the 168h cap, a Tokyo calendar's event fires two minutes overdue, and a
// horizon computed from a fixed 8-day UTC window (or from truncating a UTC
// instant into a day) never enumerates it.
func TestReminderHorizonTokyo168Hours(t *testing.T) {
	cfg := reminderConfig()
	cfg.Reminders.HoursBefore = 168
	f := newReminderFixture(t, cfg)
	f.calendar.Timezone = "Asia/Tokyo"
	f.scheduler.now = func() time.Time { return time.Date(2026, 10, 1, 15, 2, 0, 0, time.UTC) }
	f.slots.counts = map[string]int{"2026-10-09": 2}

	f.scheduler.enqueueDue(t.Context())

	if len(f.jobs.enqueued) != 5 {
		t.Fatalf("Tokyo 168h event due two minutes ago: jobs=%d want=5", len(f.jobs.enqueued))
	}
	wantDate := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	wantScheduled := time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
	for _, job := range f.jobs.enqueued {
		if !job.EventDate.Equal(wantDate) {
			t.Errorf("job event date = %v, want %v", job.EventDate, wantDate)
		}
		if !job.ScheduledAt.Equal(wantScheduled) {
			t.Errorf("job scheduled_at = %v, want %v", job.ScheduledAt, wantScheduled)
		}
	}
}

// TestReminderHorizonKiritimati is the same defect one timezone further east:
// Pacific/Kiritimati is UTC+14, so its local midnight is 14 hours ahead of the
// UTC instant — the wrong extreme for a window or a day built in UTC.
func TestReminderHorizonKiritimati(t *testing.T) {
	cfg := reminderConfig()
	cfg.Reminders.HoursBefore = 24
	f := newReminderFixture(t, cfg)
	f.calendar.Timezone = "Pacific/Kiritimati"
	// Event civil date 2026-10-09 (UTC+14), local midnight = 2026-10-08 10:00
	// UTC, scheduled_at = 24h earlier = 2026-10-07 10:00 UTC.
	f.scheduler.now = func() time.Time { return time.Date(2026, 10, 7, 10, 2, 0, 0, time.UTC) }
	f.slots.counts = map[string]int{"2026-10-09": 2}

	f.scheduler.enqueueDue(t.Context())

	if len(f.jobs.enqueued) != 5 {
		t.Fatalf("Kiritimati event due two minutes ago: jobs=%d want=5", len(f.jobs.enqueued))
	}
	for _, job := range f.jobs.enqueued {
		if !job.EventDate.Equal(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("job event date = %v, want 2026-10-09", job.EventDate)
		}
		if !job.ScheduledAt.Equal(time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)) {
			t.Errorf("job scheduled_at = %v, want 2026-10-07T10:00Z", job.ScheduledAt)
		}
	}
}

// TestReminderHorizonBehindUTC covers a zone west of UTC, where local midnight
// is *later* than the UTC instant: the candidate civil date would be silently
// shifted back a day by a UTC-truncating scan.
func TestReminderHorizonBehindUTC(t *testing.T) {
	cfg := reminderConfig()
	cfg.Reminders.HoursBefore = 24
	f := newReminderFixture(t, cfg)
	f.calendar.Timezone = "Pacific/Honolulu"
	// Event civil date 2026-10-09 (UTC-10), local midnight = 2026-10-09 10:00
	// UTC, scheduled_at = 2026-10-08 10:00 UTC.
	f.scheduler.now = func() time.Time { return time.Date(2026, 10, 8, 10, 2, 0, 0, time.UTC) }
	f.slots.counts = map[string]int{"2026-10-09": 2}

	f.scheduler.enqueueDue(t.Context())

	if len(f.jobs.enqueued) != 5 {
		t.Fatalf("Honolulu event due two minutes ago: jobs=%d want=5", len(f.jobs.enqueued))
	}
	for _, job := range f.jobs.enqueued {
		if !job.EventDate.Equal(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("job event date = %v, want 2026-10-09", job.EventDate)
		}
		if !job.ScheduledAt.Equal(time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)) {
			t.Errorf("job scheduled_at = %v, want 2026-10-08T10:00Z", job.ScheduledAt)
		}
	}
}

// TestReminderHorizonLeadTimeBoundaries pins the effective lead time across the
// validated range and the operator fallback: hours_before is used as-is when
// set, and whatever the range is, the event's civil date keeps its own day.
func TestReminderHorizonLeadTimeBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		hoursBefore   int
		now           time.Time
		wantScheduled time.Time
	}{
		{
			name:          "one hour before midnight",
			hoursBefore:   1,
			now:           time.Date(2026, 3, 31, 23, 2, 0, 0, time.UTC),
			wantScheduled: time.Date(2026, 3, 31, 23, 0, 0, 0, time.UTC),
		},
		{
			name:          "the 167-hour offset is an odd day count of real hours",
			hoursBefore:   167,
			now:           time.Date(2026, 3, 25, 1, 2, 0, 0, time.UTC),
			wantScheduled: time.Date(2026, 3, 25, 1, 0, 0, 0, time.UTC),
		},
		{
			name:          "the 168-hour cap is a full seven days",
			hoursBefore:   168,
			now:           time.Date(2026, 3, 25, 0, 2, 0, 0, time.UTC),
			wantScheduled: time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		},
		{
			name:          "unset hours_before falls back to the operator default",
			hoursBefore:   0,
			now:           time.Date(2026, 3, 31, 0, 2, 0, 0, time.UTC),
			wantScheduled: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		},
	}

	// Every row has its event on the same civil date (2026-04-01, UTC), with
	// now = scheduled_at + 2 minutes so each is exactly inside the catch-up
	// window, and its count recorded under that date.
	wantDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := reminderConfig()
			cfg.Reminders.HoursBefore = tt.hoursBefore
			f := newReminderFixture(t, cfg)
			f.scheduler.now = func() time.Time { return tt.now }
			f.slots.counts = map[string]int{"2026-04-01": 2}

			f.scheduler.enqueueDue(t.Context())

			if len(f.jobs.enqueued) != 5 {
				t.Fatalf("enqueued %d jobs, want 5", len(f.jobs.enqueued))
			}
			for _, job := range f.jobs.enqueued {
				if !job.EventDate.Equal(wantDate) {
					t.Errorf("job event date = %v, want %v", job.EventDate, wantDate)
				}
				if !job.ScheduledAt.Equal(tt.wantScheduled) {
					t.Errorf("job scheduled_at = %v, want %v", job.ScheduledAt, tt.wantScheduled)
				}
			}
		})
	}
}

// TestReminderHorizonDSTSpringForward verifies the horizon and the scheduled
// instant across a DST transition: the event's civil date in a zone that jumped
// forward keeps its own day, and the scheduled instant subtracts a real number
// of hours from the local midnight.
func TestReminderHorizonDSTSpringForward(t *testing.T) {
	cfg := reminderConfig()
	cfg.Reminders.HoursBefore = 24
	f := newReminderFixture(t, cfg)
	f.calendar.Timezone = "America/New_York"
	// 2026-03-08 is the spring-forward day (02:00 EST -> 03:00 EDT). Local
	// midnight is 2026-03-08 00:00 EST = 05:00 UTC; scheduled_at = 2026-03-07
	// 05:00 UTC.
	f.scheduler.now = func() time.Time { return time.Date(2026, 3, 7, 5, 2, 0, 0, time.UTC) }
	f.slots.counts = map[string]int{"2026-03-08": 2}

	f.scheduler.enqueueDue(t.Context())

	if len(f.jobs.enqueued) != 5 {
		t.Fatalf("DST event due two minutes ago: jobs=%d want=5", len(f.jobs.enqueued))
	}
	for _, job := range f.jobs.enqueued {
		if !job.EventDate.Equal(time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("job event date = %v, want 2026-03-08 (the event's civil day)", job.EventDate)
		}
		if !job.ScheduledAt.Equal(time.Date(2026, 3, 7, 5, 0, 0, 0, time.UTC)) {
			t.Errorf("job scheduled_at = %v, want 2026-03-07T05:00Z", job.ScheduledAt)
		}
	}
}

// TestReminderTransientCountErrorMustNotCancel is the original defect: a count
// query hiccup while verifying a claimed job must retry the job with backoff,
// and must never cancel the whole event.
func TestReminderTransientCountErrorMustNotCancel(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.slots.err = errors.New("temporary database outage")
	f.scheduler.deliverOne(t.Context(), models.ReminderJob{
		ID:            uuid.New(),
		CalendarID:    f.calendar.ID,
		EventDate:     time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		ScheduledAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		RecipientType: models.ReminderRecipientOwner,
		Channel:       "email",
		MaxAttempts:   5,
	})

	if len(f.jobs.canceledClaim) > 0 {
		t.Errorf("transient database error canceled the delivery: %+v", f.jobs.canceledClaim)
	}
	if len(f.jobs.failed) != 1 {
		t.Errorf("transient error was not recorded as a retryable failure: %+v", f.jobs.failed)
	}
	if len(f.mailer.messages()) != 0 {
		t.Error("an email was sent although the count could not be read")
	}
}

// TestReminderSMTPAbsentMustNotEnqueueEmail is the original defect: with no SMTP
// the scheduler must not create email jobs it can never deliver, while chat
// channels keep theirs.
func TestReminderSMTPAbsentMustNotEnqueueEmail(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.mailer.configured = false

	f.scheduler.enqueueDue(t.Context())

	for _, job := range f.jobs.enqueued {
		if job.Channel == "email" {
			t.Errorf("SMTP absent but email job enqueued for %s", job.RecipientType)
		}
	}
	// The chat channels are unaffected: owner/discord, owner/slack,
	// owner/telegram still get jobs, participant email does not.
	if len(f.jobs.enqueued) != 3 {
		t.Errorf("enqueued %d jobs with SMTP absent, want the 3 chat channels", len(f.jobs.enqueued))
	}
}

// TestReminderChatContinuesWhenEmailSuppressed covers the delivery half of the
// SMTP policy: a calendar with a valid chat channel next to an email channel
// whose mailer disappeared cancels only the email delivery, and the chat job
// still sends.
func TestReminderChatContinuesWhenEmailSuppressed(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())

	f.scheduler.enqueueDue(t.Context())
	if got := countDelivery(f.jobs.enqueued, models.ReminderRecipientOwner, "email"); got != 1 {
		t.Fatalf("owner/email jobs before SMTP loss = %d, want 1", got)
	}

	// SMTP disappears between enqueue and delivery.
	f.mailer.configured = false

	emailJob := models.ReminderJob{
		ID:            uuid.New(),
		CalendarID:    f.calendar.ID,
		EventDate:     time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		ScheduledAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		RecipientType: models.ReminderRecipientOwner,
		Channel:       "email",
		Status:        models.ReminderJobPending,
		MaxAttempts:   5,
	}
	discordJob := emailJob
	discordJob.ID = uuid.New()
	discordJob.Channel = "discord"
	f.jobs.claimed = []models.ReminderJob{emailJob, discordJob}

	delivered, err := f.scheduler.deliverDue(t.Context())
	if err != nil {
		t.Fatalf("deliverDue: %v", err)
	}
	if !delivered {
		t.Fatal("deliverDue claimed two jobs and reported none")
	}

	// The email delivery was canceled under its claim — not attempted, not
	// failed through the access-budget — and the chat channel still sent.
	if len(f.jobs.canceledClaim) != 1 {
		t.Fatalf("email delivery not suppressed individually: %+v", f.jobs.canceledClaim)
	}
	if len(f.jobs.failed) != 0 {
		t.Fatalf("known-absent SMTP burned an attempt: %+v", f.jobs.failed)
	}
	if len(f.mailer.messages()) != 0 {
		t.Fatalf("email was sent although SMTP is gone: %d", len(f.mailer.messages()))
	}
	if len(f.jobs.sent) != 1 || f.jobs.sent[0] != discordJob.ID {
		t.Errorf("chat job not marked sent beside the suppressed email: %+v", f.jobs.sent)
	}
	if len(f.external.calls) != 1 || f.external.calls[0].channel != "discord" {
		t.Errorf("discord delivery did not proceed: %+v", f.external.calls)
	}
}

// TestReminderEmailRearmsOnceSMTPReturns pins the rearmable policy: with SMTP
// present the email delivery is issued again on the next scan, inside the
// catch-up window, so a re-enabled instance does not lose the reminder.
func TestReminderEmailRearmsOnceSMTPReturns(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())

	f.mailer.configured = false
	f.scheduler.enqueueDue(t.Context())
	if got := countDelivery(f.jobs.enqueued, models.ReminderRecipientOwner, "email"); got != 0 {
		t.Fatalf("email issued with SMTP absent: %d", got)
	}

	f.mailer.configured = true
	f.scheduler.enqueueDue(t.Context())
	if got := countDelivery(f.jobs.enqueued, models.ReminderRecipientOwner, "email"); got != 1 {
		t.Errorf("email delivery not re-issued once SMTP returned: %d", got)
	}
}

// TestReminderParticipantEmailConsentRevokedAtDelivery: a participant email job
// enqueued while the calendar's full consent configuration was on must not fire
// after the owner revokes participant delivery; the delivery is suppressed under
// its claim, and no email leaves the server.
func TestReminderParticipantEmailConsentRevokedAtDelivery(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())

	revoked := reminderConfig()
	revoked.NotifyParticipants = false
	raw, err := json.Marshal(revoked)
	if err != nil {
		t.Fatalf("marshal revoked config: %v", err)
	}
	f.calendar.NotifyConfig = strptr(string(raw))

	job := models.ReminderJob{
		ID:            uuid.New(),
		CalendarID:    f.calendar.ID,
		EventDate:     time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		ScheduledAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		RecipientType: models.ReminderRecipientParticipants,
		Channel:       "email",
		Status:        models.ReminderJobPending,
		MaxAttempts:   5,
	}
	f.jobs.claimed = []models.ReminderJob{job}

	delivered, err := f.scheduler.deliverDue(t.Context())
	if err != nil {
		t.Fatalf("deliverDue: %v", err)
	}
	if !delivered {
		t.Fatal("deliverDue did not claim the stale participant job")
	}

	if len(f.jobs.canceledClaim) != 1 {
		t.Fatalf("consent-revoked participant job not suppressed individually: %+v", f.jobs.canceledClaim)
	}
	if len(f.mailer.messages()) != 0 {
		t.Fatalf("a consent-revoked participant reminder was emailed: %d", len(f.mailer.messages()))
	}
	if len(f.jobs.sent) != 0 {
		t.Fatalf("consent-revoked job marked sent: %+v", f.jobs.sent)
	}
}

// TestReminderSMTPAbsentStillGatesParticipantEmail: participant email requires
// the calendar's consent *and* real SMTP at enqueue time; with the consent on
// but SMTP absent, no participant email job is created.
func TestReminderSMTPAbsentStillGatesParticipantEmail(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.mailer.configured = false

	f.scheduler.enqueueDue(t.Context())

	for _, job := range f.jobs.enqueued {
		if job.RecipientType == models.ReminderRecipientParticipants {
			t.Errorf("participant email job enqueued although SMTP is absent: %+v", job)
		}
	}
}

func countDelivery(jobs []models.ReminderJob, recipientType, channel string) int {
	n := 0
	for _, job := range jobs {
		if job.RecipientType == recipientType && job.Channel == channel {
			n++
		}
	}
	return n
}
