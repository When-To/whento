// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/email"
	availabilityModels "github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/notify/models"
)

type leaseCheckingMailer struct {
	*fakeMailer
	now       *time.Time
	deadlines []time.Duration
}

func (m *leaseCheckingMailer) SendContext(ctx context.Context, msg email.Email) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		return errors.New("reminder send has no deadline")
	}
	m.deadlines = append(m.deadlines, time.Until(deadline))
	if err := ctx.Err(); err != nil {
		return err
	}
	*m.now = m.now.Add(25 * time.Second)
	return m.Send(msg)
}

type lostFanoutClaim struct {
	*fakeReminderJobStore
	checks int
}

func (s *lostFanoutClaim) RenewLease(ctx context.Context, id, token uuid.UUID, now time.Time, ttl time.Duration) error {
	s.checks++
	if s.checks > 1 {
		return models.ErrClaimLost
	}
	return s.fakeReminderJobStore.RenewLease(ctx, id, token, now, ttl)
}

func participantFanout(t *testing.T) (*reminderFixture, models.ReminderJob) {
	t.Helper()
	f := newReminderFixture(t, reminderConfig())
	for range 2 {
		p := f.participant
		p.ID = uuid.New()
		f.people.verified = append(f.people.verified, p)
		f.slots.available = append(f.slots.available, availabilityModels.AvailableParticipant{ID: p.ID})
	}
	job := enqueueDueJob()
	job.CalendarID = f.calendar.ID
	job.RecipientType = models.ReminderRecipientParticipants
	job.ClaimToken = uuid.New()
	return f, job
}

func TestReminderFanoutRenewsBeforeEveryBoundedSend(t *testing.T) {
	f, job := participantFanout(t)
	now := fixedReminderNow()
	f.scheduler.now = func() time.Time { return now }
	mailer := &leaseCheckingMailer{fakeMailer: f.mailer, now: &now}
	f.scheduler.emailService = mailer
	f.scheduler.deliverOne(t.Context(), job)
	if len(mailer.deadlines) != 3 || len(f.jobs.renewed) != 3 {
		t.Fatalf("context sends=%d lease renewals=%d, want one of each per recipient", len(mailer.deadlines), len(f.jobs.renewed))
	}
	for _, deadline := range mailer.deadlines {
		if deadline <= 0 || deadline >= f.scheduler.lockTTL {
			t.Fatalf("send deadline %s does not fit inside lease", deadline)
		}
	}
	for _, renewal := range f.jobs.renewed {
		if renewal.ttl != f.scheduler.lockTTL {
			t.Fatalf("renewal shortened the claim lease to %s", renewal.ttl)
		}
	}
	if len(f.jobs.sent) != 1 {
		t.Fatal("successful fanout was not completed")
	}
}

func TestReminderFanoutStopsImmediatelyWhenClaimIsLost(t *testing.T) {
	f, job := participantFanout(t)
	lost := &lostFanoutClaim{fakeReminderJobStore: f.jobs}
	f.scheduler.jobs = lost
	f.scheduler.deliverOne(t.Context(), job)
	if len(f.mailer.messages()) != 1 {
		t.Fatalf("sent %d emails after losing claim, want only first", len(f.mailer.messages()))
	}
	if len(f.jobs.sent) != 0 || len(f.jobs.failed) != 0 {
		t.Fatal("lost worker mutated the queue")
	}
}

func TestReminderFanoutStopsWhenEventStartsBetweenRecipients(t *testing.T) {
	f, job := participantFanout(t)
	now := job.EventDate.Add(-15 * time.Second)
	f.scheduler.now = func() time.Time { return now }
	f.scheduler.emailService = &leaseCheckingMailer{fakeMailer: f.mailer, now: &now}
	f.scheduler.deliverOne(t.Context(), job)
	if len(f.mailer.messages()) != 1 || len(f.jobs.canceledClaim) != 1 {
		t.Fatal("fanout continued sending after the calendar-local event began")
	}
	if len(f.jobs.failed) != 0 || len(f.jobs.sent) != 0 {
		t.Fatal("elapsed event was retried or marked fully delivered")
	}
}

func TestReminderCancelledContextDoesNotSend(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	job := enqueueDueJob()
	job.CalendarID = f.calendar.ID
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f.scheduler.deliverOne(ctx, job)
	if len(f.mailer.messages()) != 0 {
		t.Fatal("shutdown/cancelled context sent mail")
	}
}

func TestReminderPastEventDoesNotSend(t *testing.T) {
	for _, zone := range []string{"UTC", "Asia/Tokyo", "Pacific/Honolulu"} {
		t.Run(zone, func(t *testing.T) {
			f := newReminderFixture(t, reminderConfig())
			f.calendar.Timezone = zone
			job := enqueueDueJob()
			job.CalendarID = f.calendar.ID
			loc, err := time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			job.ScheduledAt = f.scheduler.eventMidnight(job.EventDate, loc).Add(-24 * time.Hour)
			f.scheduler.now = func() time.Time { return f.scheduler.eventMidnight(job.EventDate, loc) }
			f.scheduler.deliverOne(t.Context(), job)
			if len(f.mailer.messages()) != 0 || len(f.jobs.canceledClaim) != 1 {
				t.Fatal("overdue event was not canceled under its own claim")
			}
		})
	}
}

func TestReminderDisqualifiedEventOnlyCancelsOwnedClaim(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.slots.count = 0
	job := enqueueDueJob()
	job.CalendarID = f.calendar.ID
	f.scheduler.deliverOne(t.Context(), job)
	if len(f.jobs.canceledClaim) != 1 {
		t.Fatal("event snapshot used an unfenced whole-event cancellation")
	}
}

func TestReminderBackoffSaturatesBeforeOverflow(t *testing.T) {
	s := &ReminderScheduler{backoffBase: time.Minute, backoffMax: 24 * time.Hour}
	for _, attempt := range []int{25, 40, 100, int(^uint(0) >> 1)} {
		if got := s.backoff(attempt); got != s.backoffMax {
			t.Fatalf("attempt %d backoff=%s want cap=%s", attempt, got, s.backoffMax)
		}
	}
}

func TestReminderFailureUsesPersistedAttemptBudget(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	job := enqueueDueJob()
	job.MaxAttempts = 2
	f.scheduler.recordFailure(t.Context(), job, errReminderSend)
	if f.jobs.failed[0].max != job.MaxAttempts {
		t.Fatal("delivery ignored persisted attempt budget")
	}
}

func TestReminderEmailEscapesCalendarNameAndURL(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.calendar.Name = `<img src=x onerror=alert(1)>`
	body := f.scheduler.buildReminderEmail(f.calendar, fixedReminderNow(), `https://whento.test/" onmouseover="evil`, "en", true)
	if strings.Contains(body, `<img`) || strings.Contains(body, `href="https://whento.test/" onmouseover=`) {
		t.Fatal("reminder HTML contains unescaped data")
	}
}

type canceledReminderMailer struct {
	*fakeMailer
	started chan struct{}
}

func (m *canceledReminderMailer) SendContext(ctx context.Context, _ email.Email) error {
	close(m.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestReminderShutdownInterruptsInFlightSend(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	mailer := &canceledReminderMailer{fakeMailer: f.mailer, started: make(chan struct{})}
	f.scheduler.emailService = mailer
	job := enqueueDueJob()
	job.CalendarID = f.calendar.ID
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); f.scheduler.deliverOne(ctx, job) }()
	select {
	case <-mailer.started:
	case <-time.After(time.Second):
		t.Fatal("send did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("send ignored shutdown")
	}
	if len(f.jobs.sent) != 0 || len(f.jobs.failed) != 0 {
		t.Fatal("shutdown recorded a send or burned retry budget")
	}
}

type failedReminderLedger struct{ *fakeNotificationLog }

func (l *failedReminderLedger) LogNotification(context.Context, uuid.UUID, time.Time, string, string, uuid.UUID, string) error {
	return errors.New("ledger unavailable")
}

func TestReminderChatLedgerFailureIsRetried(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.scheduler.notificationLog = &failedReminderLedger{fakeNotificationLog: f.log}
	job := enqueueDueJob()
	job.CalendarID, job.Channel = f.calendar.ID, "discord"
	f.scheduler.deliverOne(t.Context(), job)
	if len(f.external.calls) != 1 || len(f.jobs.failed) != 1 || len(f.jobs.sent) != 0 {
		t.Fatal("chat delivery ignored a failed ledger write")
	}
}

func TestReminderProviderSecretsDoNotReachLogsOrQueueErrors(t *testing.T) {
	for _, channel := range []string{"email", "discord", "slack", "telegram"} {
		t.Run(channel, func(t *testing.T) {
			f := newReminderFixture(t, reminderConfig())
			const secret = "sensitive-provider-token-or-address"
			cause := errors.New("Post https://provider.test/" + secret + ": transport failed")
			f.mailer.err, f.external.err = cause, cause
			var output bytes.Buffer
			f.scheduler.logger = slog.New(slog.NewJSONHandler(&output, nil))
			job := enqueueDueJob()
			job.CalendarID, job.Channel = f.calendar.ID, channel
			f.scheduler.deliverOne(t.Context(), job)
			if len(f.jobs.failed) != 1 {
				t.Fatal("provider failure did not schedule a retry")
			}
			if strings.Contains(output.String(), secret) || strings.Contains(f.jobs.failed[0].reason, secret) {
				t.Fatal("provider credential leaked into reminder log/last_error")
			}
			if err := f.scheduler.sendJob(t.Context(), f.calendar, func() *models.NotifyConfig { cfg := reminderConfig(); return &cfg }(), job); !errors.Is(err, cause) {
				t.Fatalf("provider error identity lost: %v", err)
			}
		})
	}
}

type reminderFailingTransport struct{}

func (reminderFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("simulated DNS failure")
}

func TestReminderConcreteHTTPProviderErrorsDoNotLeakTokens(t *testing.T) {
	for _, channel := range []string{"discord", "slack", "telegram"} {
		t.Run(channel, func(t *testing.T) {
			cfg := reminderConfig()
			const secret = "PRIVATE_PROVIDER_CREDENTIAL"
			cfg.Channels.Discord.WebhookURL = "https://discord.test/" + secret
			cfg.Channels.Slack.WebhookURL = "https://slack.test/" + secret
			cfg.Channels.Telegram.BotToken = "123456:" + secret
			f := newReminderFixture(t, cfg)
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			f.scheduler.logger = logger
			f.scheduler.externalNotifier = &ExternalNotifier{httpClient: &http.Client{Transport: reminderFailingTransport{}}, logger: logger}
			job := enqueueDueJob()
			job.CalendarID, job.Channel = f.calendar.ID, channel
			f.scheduler.deliverOne(t.Context(), job)
			if len(f.jobs.failed) != 1 || f.jobs.failed[0].reason != channel+"_delivery_failed" {
				t.Fatal("HTTP provider failure did not record a safe category")
			}
			if strings.Contains(output.String(), secret) {
				t.Fatal("HTTP url.Error leaked provider URL/token")
			}
		})
	}
}

func TestReminderMissedWindowIsRecordedAndWarnedOnlyOnce(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.scheduler.catchUp = time.Minute
	f.scheduler.interval = time.Minute
	var output bytes.Buffer
	f.scheduler.logger = slog.New(slog.NewJSONHandler(&output, nil))
	f.scheduler.enqueueDue(t.Context())
	f.scheduler.enqueueDue(t.Context())
	if len(f.jobs.enqueued) != 0 || len(f.jobs.missed) != 5 {
		t.Fatalf("missed=%d enqueued=%d", len(f.jobs.missed), len(f.jobs.enqueued))
	}
	if warnings := strings.Count(output.String(), "Reminder catch-up window missed"); warnings != 5 {
		t.Fatalf("missed warnings=%d, want one per delivery", warnings)
	}
	if strings.Contains(output.String(), f.calendar.PublicToken) || strings.Contains(output.String(), *f.participant.Email) {
		t.Fatal("missed warning leaked calendar credential or recipient")
	}
}

// A date that reaches its threshold after its send window, while the scheduler
// was running, qualified late: it still gets its reminder before the event.
func TestReminderLateQualificationIsDeliveredWhileWatching(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.scheduler.catchUp = time.Minute
	f.slots.count = 0

	f.scheduler.now = func() time.Time { return time.Date(2026, 3, 31, 23, 0, 0, 0, time.UTC) }
	f.scheduler.enqueueDue(t.Context())

	// 2026-04-02 qualifies at 01:02, an hour after its 00:00 send window.
	f.slots.count = f.calendar.Threshold
	f.scheduler.now = func() time.Time { return time.Date(2026, 4, 1, 1, 2, 0, 0, time.UTC) }
	f.scheduler.enqueueDue(t.Context())

	if len(f.jobs.missed) != 0 || len(f.jobs.enqueued) != 5 {
		t.Fatalf("missed=%d enqueued=%d, want the late event enqueued", len(f.jobs.missed), len(f.jobs.enqueued))
	}
}

type panickingReminderJobStore struct{ *fakeReminderJobStore }

func (panickingReminderJobStore) Cleanup(context.Context, time.Time, int) (int64, error) {
	panic("boom")
}

func TestReminderPassPanicDoesNotEscape(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.scheduler.jobs = panickingReminderJobStore{f.jobs}
	f.scheduler.safeRunOnce(t.Context())
}

func TestReminderCleanupRunsDailyAfterDrainingBacklog(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	f.scheduler.runOnce(t.Context())
	f.scheduler.runOnce(t.Context())
	if f.jobs.cleanups != 1 || f.log.oldCleanups != 1 {
		t.Fatalf("cleanups=%d notification-log cleanups=%d, want 1 each", f.jobs.cleanups, f.log.oldCleanups)
	}
	f.scheduler.now = func() time.Time { return fixedReminderNow().Add(24 * time.Hour) }
	f.scheduler.runOnce(t.Context())
	if f.jobs.cleanups != 2 || f.log.oldCleanups != 2 {
		t.Fatalf("daily cleanup did not run: jobs=%d notification-log=%d", f.jobs.cleanups, f.log.oldCleanups)
	}
}

func TestReminderParticipantCancelLinkIsLocalizedAndOwnerHasNoCancel(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	for _, locale := range []string{"en", "fr"} {
		url := "https://whento.test/c/token/p/participant"
		body := f.scheduler.buildReminderEmail(f.calendar, fixedReminderNow(), url, locale, true)
		label := "Cancel my participation"
		if locale == "fr" {
			label = "Annuler ma participation"
		}
		if !strings.Contains(body, url+"?cancel=2026-04-01") || !strings.Contains(body, label) {
			t.Fatalf("missing %s cancellation link: %s", locale, body)
		}
		owner := f.scheduler.buildReminderEmail(f.calendar, fixedReminderNow(), url, locale, false)
		if strings.Contains(owner, "?cancel=") {
			t.Fatal("owner was offered participant cancellation")
		}
	}
}

type acceptedDuringShutdownMailer struct {
	*fakeMailer
	cancel context.CancelFunc
}

func (m *acceptedDuringShutdownMailer) SendContext(ctx context.Context, msg email.Email) error {
	m.cancel() // The provider accepted just as the process began shutting down.
	return m.Send(msg)
}

type contextCheckingCompletionLog struct{ *fakeNotificationLog }

func (l *contextCheckingCompletionLog) LogNotification(ctx context.Context, calendarID uuid.UUID, date time.Time, event, recipientType string, recipientID uuid.UUID, channel string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("completion write has no deadline")
	}
	return l.fakeNotificationLog.LogNotification(ctx, calendarID, date, event, recipientType, recipientID, channel)
}

type contextCheckingCompletionJobs struct{ *fakeReminderJobStore }

func (s *contextCheckingCompletionJobs) MarkSent(ctx context.Context, id, token uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("MarkSent has no deadline")
	}
	return s.fakeReminderJobStore.MarkSent(ctx, id, token)
}

func TestReminderAcceptedSendIsRecordedDespiteConcurrentShutdown(t *testing.T) {
	f := newReminderFixture(t, reminderConfig())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.scheduler.emailService = &acceptedDuringShutdownMailer{fakeMailer: f.mailer, cancel: cancel}
	f.scheduler.notificationLog = &contextCheckingCompletionLog{fakeNotificationLog: f.log}
	f.scheduler.jobs = &contextCheckingCompletionJobs{fakeReminderJobStore: f.jobs}
	job := enqueueDueJob()
	job.CalendarID = f.calendar.ID
	f.scheduler.deliverOne(ctx, job)
	if len(f.jobs.sent) != 1 || len(f.log.logged) != 1 {
		t.Fatal("accepted send lost its completion at shutdown")
	}
}
