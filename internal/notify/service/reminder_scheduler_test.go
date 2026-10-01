// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	authModels "github.com/whento/whento/internal/auth/models"
	availabilityModels "github.com/whento/whento/internal/availability/models"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/notify/models"
)

// fixedReminderNow is the clock every scheduler test runs under.
var fixedReminderNow = func() time.Time {
	return time.Date(2026, 4, 1, 0, 2, 0, 0, time.UTC)
}

// The fakes used here are shared with the threshold-notification tests in
// notify_service_send_test.go; this file adds the reminder job queue.

type failRecord struct {
	id      uuid.UUID
	attempt int
	max     int
	next    time.Time
	reason  string
}

type cancelRecord struct {
	calendarID uuid.UUID
	eventDate  time.Time
}

// fakeReminderJobStore is an in-memory reminder queue for the scheduler tests.
// Enqueue records; ClaimDue returns whatever the test pre-seeded.
type fakeReminderJobStore struct {
	mu       sync.Mutex
	enqueued []models.ReminderJob
	claimed  []models.ReminderJob
	sent     []uuid.UUID
	failed   []failRecord
	canceled []cancelRecord

	enqueueErr error
	claimErr   error
}

func (f *fakeReminderJobStore) Enqueue(_ context.Context, job *models.ReminderJob) error {
	if f.enqueueErr != nil {
		return f.enqueueErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueued = append(f.enqueued, *job)
	return nil
}

func (f *fakeReminderJobStore) ClaimDue(_ context.Context, _ string, _ time.Time, _ time.Duration, _ int) ([]models.ReminderJob, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]models.ReminderJob(nil), f.claimed...), nil
}

func (f *fakeReminderJobStore) MarkSent(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, id)
	return nil
}

func (f *fakeReminderJobStore) MarkFailed(_ context.Context, id uuid.UUID, attempt, max int, next time.Time, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, failRecord{id, attempt, max, next, reason})
	return nil
}

func (f *fakeReminderJobStore) CancelPendingForEvent(_ context.Context, calendarID uuid.UUID, eventDate time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.canceled = append(f.canceled, cancelRecord{calendarID, eventDate})
	return nil
}

var _ ReminderJobStore = (*fakeReminderJobStore)(nil)

// reminderConfig is a notify config with reminders on, every channel enabled,
// addressed to owner and participants.
func reminderConfig() models.NotifyConfig {
	return models.NotifyConfig{
		Enabled:            true,
		NotifyOwner:        true,
		NotifyParticipants: true,
		Channels: models.ChannelConfig{
			Email: models.EmailChannelConfig{Enabled: true},
			Discord: models.DiscordChannelConfig{
				Enabled:    true,
				WebhookURL: "https://discord.test/hook",
			},
			Slack: models.SlackChannelConfig{
				Enabled:    true,
				WebhookURL: "https://slack.test/hook",
			},
			Telegram: models.TelegramChannelConfig{
				Enabled:  true,
				BotToken: "bot-token",
				ChatID:   "chat-id",
			},
		},
		Reminders: models.ReminderConfig{
			Enabled:     true,
			HoursBefore: 24,
		},
	}
}

type reminderFixture struct {
	calendar    *calendarModels.Calendar
	owner       *authModels.User
	participant calendarModels.Participant
	calendars   *fakeCalendarStore
	slots       *fakeAvailabilityStore
	people      *fakeParticipantStore
	users       *fakeUserStore
	log         *fakeNotificationLog
	mailer      *fakeMailer
	external    *fakeChannelNotifier
	jobs        *fakeReminderJobStore
	scheduler   *ReminderScheduler
}

func newReminderFixture(t *testing.T, config models.NotifyConfig) *reminderFixture {
	t.Helper()

	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal notify config: %v", err)
	}
	raw := string(configJSON)

	calendarID := uuid.New()
	ownerID := uuid.New()

	calendar := &calendarModels.Calendar{
		OwnerID:      ownerID,
		Name:         "Board game night",
		PublicToken:  "public-token",
		Threshold:    2,
		Timezone:     "UTC",
		NotifyConfig: &raw,
	}
	calendar.ID = calendarID

	owner := &authModels.User{
		Email:         "owner@example.test",
		DisplayName:   "Owner",
		Locale:        "en",
		EmailVerified: true,
	}
	owner.ID = ownerID

	participant := calendarModels.Participant{
		CalendarID:    calendarID,
		Name:          "Ada",
		Email:         strptr("ada@example.test"),
		EmailVerified: true,
		Locale:        "en",
	}
	participant.ID = uuid.New()

	available := []availabilityModels.AvailableParticipant{
		{ID: participant.ID, Name: participant.Name},
	}

	calendars := &fakeCalendarStore{list: []*calendarModels.Calendar{calendar}, calendar: calendar}
	slots := &fakeAvailabilityStore{available: available, count: calendar.Threshold}
	people := &fakeParticipantStore{verified: []calendarModels.Participant{participant}}
	users := &fakeUserStore{user: owner}
	log := &fakeNotificationLog{sentRecently: map[string]bool{}}
	mailer := &fakeMailer{configured: true}
	external := &fakeChannelNotifier{}
	jobs := &fakeReminderJobStore{}

	s := NewReminderScheduler(
		calendars, slots, people, users, log, mailer, external, jobs,
		"https://whento.test", "test-instance", quietLogger(),
	)
	s.now = fixedReminderNow

	return &reminderFixture{
		calendar:    calendar,
		owner:       owner,
		participant: participant,
		calendars:   calendars,
		slots:       slots,
		people:      people,
		users:       users,
		log:         log,
		mailer:      mailer,
		external:    external,
		jobs:        jobs,
		scheduler:   s,
	}
}

// TestReminderEnqueuesDueJobs verifies the issue half: an event whose send
// window has opened produces one job per (recipient class, channel) the config
// names, with the scheduled instant derived from hours_before.
func TestReminderEnqueuesDueJobs(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())

	f.scheduler.enqueueDue(t.Context())

	if len(f.jobs.enqueued) != 5 {
		t.Fatalf("enqueued %d jobs, want 5 (owner: email+discord+slack+telegram, participants: email)", len(f.jobs.enqueued))
	}

	// 2026-04-02 00:00 UTC was chosen so that scheduled_at = event - 24h lands
	// 2 minutes in the past, inside the catch-up grace.
	wantScheduled := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	wantDate := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)

	seen := map[string]bool{}
	for _, job := range f.jobs.enqueued {
		key := job.RecipientType + "/" + job.Channel
		if seen[key] {
			t.Errorf("duplicate delivery key %q enqueued", key)
		}
		seen[key] = true

		if job.CalendarID != f.calendar.ID || !job.EventDate.Equal(wantDate) {
			t.Errorf("job targets calendar/date %v/%v", job.CalendarID, job.EventDate)
		}
		if !job.ScheduledAt.Equal(wantScheduled) {
			t.Errorf("scheduled_at = %v, want %v", job.ScheduledAt, wantScheduled)
		}
	}

	for _, key := range []string{
		"owner/discord", "owner/slack", "owner/telegram", "owner/email", "participants/email",
	} {
		if !seen[key] {
			t.Errorf("no job for delivery key %q", key)
		}
	}
}

// TestReminderEnqueuesNothingWhenRemindersAreDisabled: the issue half must be a
// no-op when the calendar has not opted into reminders.
func TestReminderEnqueuesNothingWhenRemindersAreDisabled(t *testing.T) {
	config := reminderConfig()
	config.Reminders.Enabled = false
	f := newReminderFixture(t, config)

	f.scheduler.enqueueDue(t.Context())

	if len(f.jobs.enqueued) != 0 {
		t.Fatalf("enqueued %d jobs although reminders are disabled", len(f.jobs.enqueued))
	}
}

// TestReminderWindowOpen pins the boundaries of the issue window: not too far in
// the future, and not deeper in the past than the catch-up grace.
func TestReminderWindowOpen(t *testing.T) {
	s := &ReminderScheduler{interval: 5 * time.Minute, catchUp: 15 * time.Minute}
	now := fixedReminderNow()

	tests := []struct {
		name        string
		scheduledAt time.Time
		want        bool
	}{
		{"just before now (catch-up)", now.Add(-14 * time.Minute), true},
		{"exactly at the catch-up bound", now.Add(-15 * time.Minute), true},
		{"past the catch-up bound", now.Add(-16 * time.Minute), false},
		{"just after now (within interval)", now.Add(4 * time.Minute), true},
		{"exactly at the interval bound", now.Add(5 * time.Minute), true},
		{"past the interval bound", now.Add(6 * time.Minute), false},
		{"now itself", now, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.windowOpen(tt.scheduledAt, now); got != tt.want {
				t.Errorf("windowOpen(%v) = %v, want %v", tt.scheduledAt, got, tt.want)
			}
		})
	}
}

// TestReminderBackoffGrowsExponentiallyAndCaps pins the retry schedule.
func TestReminderBackoffGrowsExponentiallyAndCaps(t *testing.T) {
	s := &ReminderScheduler{backoffBase: reminderBackoffBase, backoffMax: reminderBackoffMax}

	if got := s.backoff(1); got != 1*time.Minute {
		t.Errorf("backoff(1) = %v, want 1m", got)
	}
	if got := s.backoff(2); got != 2*time.Minute {
		t.Errorf("backoff(2) = %v, want 2m", got)
	}
	if got := s.backoff(3); got != 4*time.Minute {
		t.Errorf("backoff(3) = %v, want 4m", got)
	}
	if got := s.backoff(25); got != reminderBackoffMax {
		t.Errorf("backoff(25) = %v, want the cap %v", got, reminderBackoffMax)
	}
}

// TestReminderDeliversAndMarksSent drives a claimed job through verification and
// delivery: a configured, still-qualifying event reaches the owner by email,
// and the job is marked sent.
func TestReminderDeliversAndMarksSent(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())

	job := models.ReminderJob{
		ID:            uuid.New(),
		CalendarID:    f.calendar.ID,
		EventDate:     time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		RecipientType: models.ReminderRecipientOwner,
		Channel:       "email",
		ScheduledAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Status:        models.ReminderJobPending,
		MaxAttempts:   5,
	}
	f.jobs.claimed = []models.ReminderJob{job}

	delivered, err := f.scheduler.deliverDue(t.Context())
	if err != nil {
		t.Fatalf("deliverDue: %v", err)
	}
	if !delivered {
		t.Fatal("deliverDue reported nothing to deliver")
	}

	if got := len(f.mailer.messages()); got != 1 {
		t.Fatalf("delivered %d owner emails, want 1", got)
	}
	if !strings.Contains(f.mailer.messages()[0].Body, "Board game night") {
		t.Errorf("email does not name the calendar: %q", f.mailer.messages()[0].Body)
	}
	if len(f.jobs.sent) != 1 || f.jobs.sent[0] != job.ID {
		t.Errorf("job not marked sent: %+v", f.jobs.sent)
	}
}

// TestReminderCancelsWhenRemindersAreTurnedOff: a job that was queued while
// reminders were on is canceled, not delivered, once the owner disables them.
func TestReminderCancelsWhenRemindersAreTurnedOff(t *testing.T) {
	config := reminderConfig()
	config.Reminders.Enabled = false
	f := newReminderFixture(t, config)

	job := models.ReminderJob{
		ID:            uuid.New(),
		CalendarID:    f.calendar.ID,
		EventDate:     time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		RecipientType: models.ReminderRecipientOwner,
		Channel:       "email",
		ScheduledAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Status:        models.ReminderJobPending,
		MaxAttempts:   5,
	}
	f.jobs.claimed = []models.ReminderJob{job}

	worked, err := f.scheduler.deliverDue(t.Context())
	if err != nil {
		t.Fatalf("deliverDue: %v", err)
	}
	if !worked {
		t.Fatal("deliverDue did not claim the stale job")
	}

	if len(f.mailer.messages()) != 0 {
		t.Fatalf("delivered %d emails although reminders are disabled", len(f.mailer.messages()))
	}
	if len(f.jobs.canceled) != 1 {
		t.Fatalf("job was not canceled: %+v", f.jobs.canceled)
	}
	if len(f.jobs.sent) != 0 {
		t.Fatal("job marked sent although it was canceled")
	}
}

// TestReminderReschedulesWhenHoursBeforeChanges: changing hours_before moves the
// scheduled instant, so a job recorded for the old instant is canceled rather
// than delivered at the wrong time.
func TestReminderReschedulesWhenHoursBeforeChanges(t *testing.T) {
	config := reminderConfig()
	config.Reminders.HoursBefore = 48
	f := newReminderFixture(t, config)

	// Job recorded for the old 24h schedule.
	job := models.ReminderJob{
		ID:            uuid.New(),
		CalendarID:    f.calendar.ID,
		EventDate:     time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		RecipientType: models.ReminderRecipientOwner,
		Channel:       "email",
		// 2026-04-01 00:00 = event (04-02) - 24h; with 48h it should be 04-04? (see below)
		ScheduledAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Status:      models.ReminderJobPending,
		MaxAttempts: 5,
	}
	f.jobs.claimed = []models.ReminderJob{job}

	worked, err := f.scheduler.deliverDue(t.Context())
	if err != nil {
		t.Fatalf("deliverDue: %v", err)
	}
	if !worked {
		t.Fatal("deliverDue did not claim the stale job")
	}

	if len(f.mailer.messages()) != 0 {
		t.Fatalf("delivered a stale-schedule reminder by email: %d", len(f.mailer.messages()))
	}
	if len(f.jobs.canceled) != 1 {
		t.Fatalf("stale job was not canceled: %+v", f.jobs.canceled)
	}
}

// TestReminderRetriesThenFailsPermanently drives the failure path: a delivery
// error schedules the next attempt with backoff, and once the attempt counter
// reaches max the job is permanently failed.
func TestReminderRetriesThenFailsPermanently(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.mailer.err = errReminderSend

	job := models.ReminderJob{
		ID:            uuid.New(),
		CalendarID:    f.calendar.ID,
		EventDate:     time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		RecipientType: models.ReminderRecipientOwner,
		Channel:       "email",
		ScheduledAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Status:        models.ReminderJobPending,
		MaxAttempts:   5,
	}
	f.jobs.claimed = []models.ReminderJob{job}

	delivered, err := f.scheduler.deliverDue(t.Context())
	if err != nil {
		t.Fatalf("deliverDue: %v", err)
	}
	if !delivered {
		t.Fatal("deliverDue reported nothing even though a job was claimed")
	}

	if len(f.jobs.failed) != 1 {
		t.Fatalf("delivery failure not recorded: %+v", f.jobs.failed)
	}
	rec := f.jobs.failed[0]
	if rec.attempt != 1 {
		t.Errorf("attempt = %d, want 1", rec.attempt)
	}
	wantNext := fixedReminderNow().Add(reminderBackoffBase)
	if !rec.next.Equal(wantNext) {
		t.Errorf("next_attempt_at = %v, want %v", rec.next, wantNext)
	}

	// Now seed a job already at max attempts - 1: the next failure is permanent.
	job.Attempt = 4
	f.jobs.failed = nil
	f.jobs.claimed = []models.ReminderJob{job}
	f.mailer.err = errReminderSend

	delivered, _ = f.scheduler.deliverDue(t.Context())
	if !delivered {
		t.Fatal("second deliverDue reported nothing")
	}
	if got := f.jobs.failed[0].attempt; got != job.MaxAttempts {
		t.Errorf("permanent failure attempt = %d, want %d", got, job.MaxAttempts)
	}
}

// errReminderSend is a stand-in transport failure.
var errReminderSend = &errTransport{}

type errTransport struct{}

func (*errTransport) Error() string { return "smtp: connection refused" }
