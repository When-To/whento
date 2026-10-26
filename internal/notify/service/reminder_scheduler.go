// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/email"
	// Aliased: the constructor below takes a *slog.Logger named `logger`, which
	// would otherwise shadow the package.
	pkglog "github.com/whento/pkg/logger"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/notify/models"
)

// Reminder state and constants for the delivery loop.
const (
	// reminderEventType is the notification_log event_type for reminder messages,
	// distinct from the threshold transitions so the two never share a dedup slot.
	reminderEventType = "reminder"

	// defaultReminderLeaseTTL is how long a claimed-but-undelivered job stays
	// held. A crashed worker hands its jobs back after this.
	defaultReminderLeaseTTL = 10 * time.Minute

	reminderSendTimeout       = 30 * time.Second
	reminderCompletionTimeout = 2 * time.Second
	reminderRetention         = 30 * 24 * time.Hour

	// defaultReminderBatchSize caps how many jobs one scan delivers, so a backlog
	// of due senders does not monopolise the loop.
	defaultReminderBatchSize = 50

	// reminderBackoffMax caps the exponential retry schedule.
	reminderBackoffMax = 24 * time.Hour
	// reminderTimeTolerance is how much drift between a job's recorded
	// scheduled_at and the freshly computed one is accepted before the job is
	// rescheduled. A change to hours_before shows up here.
	reminderTimeTolerance = 30 * time.Second
)

// ReminderTunables are validated, defaulted operator settings from config.Load.
type ReminderTunables struct {
	// HoursBefore is the default lead time (calendar value wins when it is set).
	HoursBefore time.Duration
	// Interval is the scan cadence of the scheduler loop.
	Interval time.Duration
	// CatchUpWindow is how far into the past a rearmed job may be.
	CatchUpWindow time.Duration
	// MaxAttempts bounds send attempts before a delivery is permanently failed.
	MaxAttempts int
	// RetryBackoff is the base of the exponential retry schedule.
	RetryBackoff time.Duration
}

// ReminderJobStore is the persistence behind the delivery loop.
//
// Declared here rather than taking *repository.ReminderJobRepository so the
// scheduler can be exercised without a database. The concrete repository
// satisfies it structurally. Worker writes are fenced by the acquired token;
// models.ErrClaimLost means stop, not retry or cancel another worker's claim.
type ReminderJobStore interface {
	Enqueue(ctx context.Context, job *models.ReminderJob) error
	RecordMissed(ctx context.Context, job *models.ReminderJob) (bool, error)
	Cleanup(ctx context.Context, before time.Time, limit int) (int64, error)
	ClaimDue(ctx context.Context, instanceID string, now time.Time, leaseTTL time.Duration, limit int) ([]models.ReminderJob, error)
	RenewLease(ctx context.Context, id, token uuid.UUID, now time.Time, ttl time.Duration) error
	MarkSent(ctx context.Context, id, token uuid.UUID) error
	MarkFailed(ctx context.Context, id, token uuid.UUID, attempt, maxAttempts int, nextAttemptAt time.Time, reason string) error
	CancelClaim(ctx context.Context, id, token uuid.UUID) error
}

// ReminderNotificationLog keeps recipient completion for the whole event, not
// the one-hour anti-spam window used by threshold-transition notifications.
type ReminderNotificationLog interface {
	NotificationLog
	WasReminderSent(ctx context.Context, calendarID uuid.UUID, date time.Time, recipientID uuid.UUID, channel string) (bool, error)
	CleanupReminderLogs(ctx context.Context, before time.Time, limit int) (int64, error)
	// CleanupOldLogs applies the 30-day retention to the other notification rows.
	CleanupOldLogs(ctx context.Context) error
}

// ReminderMailer must honor cancellation and a delivery deadline. The ordinary
// notification Mailer also serves older callers, which only require Send.
type ReminderMailer interface {
	IsConfigured() bool
	SendContext(ctx context.Context, msg email.Email) error
}

// Provider errors can embed webhook URLs/bot tokens or SMTP recipient addresses.
// Retain error identity for cancellation checks without copying those values to
// application logs or the durable queue's last_error field.
type reminderProviderError struct {
	channel string
	cause   error
}

var errReminderEventElapsed = errors.New("reminder event elapsed during delivery")

func (e *reminderProviderError) Error() string {
	return e.channel + " reminder provider delivery failed"
}
func (e *reminderProviderError) Unwrap() error { return e.cause }

// ReminderCalendarStore lists the calendars the scheduler has to inspect, and
// lets a claimed job's calendar be re-read at delivery time.
type ReminderCalendarStore interface {
	ListForReminderScan(ctx context.Context) ([]*calendarModels.Calendar, error)
	GetByID(ctx context.Context, id uuid.UUID) (*calendarModels.Calendar, error)
}

// ReminderAvailabilityStore doubles the threshold-notification availability seam:
// the scheduler both counts the participants who decide whether a date is an
// event and, when it is, lists who among them is available to be told.
type ReminderAvailabilityStore interface {
	AvailabilityStore
	GetParticipantCountForDate(ctx context.Context, calendarID uuid.UUID, date time.Time) (int, error)
}

// ReminderScheduler issues and delivers reminder jobs. The reminders feature
// used to be a checkbox that stored its settings and did nothing else; this is
// the code that keeps the promise, and it is built to the standard a scheduled
// job deserves:
//
//   - jobs are persisted (unique key: calendar + event date + recipient + channel);
//   - a job is claimed atomically by exactly one delivery (ClaimDue, FOR UPDATE
//     SKIP LOCKED) under a UUID claim token and a lease, so several instances —
//     or one instance claiming twice — cannot share an unexpired claim;
//   - deliveries are retried with backed-off fenced failures and permanently
//     failed after N tries;
//   - a redeploy is survived: send-windows that passed while the process was
//     down are re-enqueued into a catch-up grace period;
//   - nothing is sent before being verified again at delivery time, so a config
//     change or a lost event cancels the job instead of producing stale mail;
//   - the send-window horizon is computed in the calendar's own timezone, so a
//     calendar east of UTC does not lose the day its event actually lands on.
//
// Delivery is at-least-once: a provider may accept a send before completion is
// recorded in the database, so a crash between the two can cause a retry. Claim
// fencing protects queue state, not a provider's side effects; exactly-once
// delivery is not promised.
type ReminderScheduler struct {
	calendarRepo     ReminderCalendarStore
	availabilityRepo ReminderAvailabilityStore
	participantRepo  ParticipantStore
	userRepo         UserStore
	notificationLog  ReminderNotificationLog
	emailService     ReminderMailer
	externalNotifier ChannelNotifier
	jobs             ReminderJobStore
	appURL           string
	instanceID       string
	interval         time.Duration
	lockTTL          time.Duration
	batchSize        int
	catchUp          time.Duration
	backoffBase      time.Duration
	backoffMax       time.Duration
	maxAttempts      int
	defaultHours     time.Duration
	lastCleanup      time.Time
	now              func() time.Time
	logger           *slog.Logger
	// watchingSince is when this process first scanned. A send window that
	// opened after it was observed by a live scan, so an event qualifying past
	// the catch-up window qualified late rather than during an outage.
	watchingSince time.Time
}

// NewReminderScheduler uses settings already defaulted and validated by config.Load.
func NewReminderScheduler(
	calendarRepo ReminderCalendarStore,
	availabilityRepo ReminderAvailabilityStore,
	participantRepo ParticipantStore,
	userRepo UserStore,
	notificationLog ReminderNotificationLog,
	emailService ReminderMailer,
	externalNotifier ChannelNotifier,
	jobs ReminderJobStore,
	appURL string,
	instanceID string,
	tunables ReminderTunables,
	logger *slog.Logger,
) *ReminderScheduler {
	s := &ReminderScheduler{
		calendarRepo:     calendarRepo,
		availabilityRepo: availabilityRepo,
		participantRepo:  participantRepo,
		userRepo:         userRepo,
		notificationLog:  notificationLog,
		emailService:     emailService,
		externalNotifier: externalNotifier,
		jobs:             jobs,
		appURL:           appURL,
		instanceID:       instanceID,
		interval:         tunables.Interval,
		lockTTL:          defaultReminderLeaseTTL,
		batchSize:        defaultReminderBatchSize,
		catchUp:          tunables.CatchUpWindow,
		backoffBase:      tunables.RetryBackoff,
		backoffMax:       reminderBackoffMax,
		maxAttempts:      tunables.MaxAttempts,
		defaultHours:     tunables.HoursBefore,
		now:              time.Now,
		logger:           logger,
	}
	return s
}

// Run scans on the configured interval until ctx is cancelled. Each pass has
// two halves: issue the jobs due for upcoming events, then deliver the due ones
// already on the queue. Errors never kill the loop — a transient outage must
// not stop the one process that keeps the instance's reminder promise.
func (s *ReminderScheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.Info("Reminder scheduler started",
		"interval", s.interval.String(),
		"instance", s.instanceID)

	s.safeRunOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Reminder scheduler stopped")
			return
		case <-ticker.C:
			s.safeRunOnce(ctx)
		}
	}
}

// safeRunOnce contains a panic to one pass. The worker goroutine has no
// Recoverer, so a panic would otherwise take the whole server down; a claimed
// job's lease simply expires and the job is handed back.
func (s *ReminderScheduler) safeRunOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Reminder scheduler pass panicked",
				"panic", r,
				"stack", string(debug.Stack()))
		}
	}()
	s.runOnce(ctx)
}

// runOnce issues then delivers.
func (s *ReminderScheduler) runOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	// Bound each cleanup batch; repeat on subsequent passes while a backlog remains.
	now := s.now()
	if s.lastCleanup.IsZero() || now.Sub(s.lastCleanup) >= 24*time.Hour {
		count, err := s.jobs.Cleanup(ctx, now.Add(-reminderRetention), 1000)
		if err != nil {
			s.logger.Error("Reminder retention cleanup failed", "error", err)
		} else {
			logs, err := s.notificationLog.CleanupReminderLogs(ctx, now.Add(-reminderRetention), 1000)
			if err != nil {
				s.logger.Error("Reminder recipient retention cleanup failed", "error", err)
			} else {
				// Threshold-transition rows only need their one-hour anti-spam window;
				// nothing else deleted them, so they accumulated forever.
				if err := s.notificationLog.CleanupOldLogs(ctx); err != nil {
					s.logger.Error("Notification log retention cleanup failed", "error", err)
				}
				if count < 1000 && logs < 1000 {
					s.lastCleanup = now
				}
			}
		}
	}
	s.enqueueDue(ctx)

	// Deliver in batches until the queue is drained or the interval elapses.
	deadline := s.now().Add(s.interval)
	for ctx.Err() == nil && s.now().Before(deadline) {
		// worked means the claim pass took possession of at least one job — a
		// job that was then canceled during its own verification still counts,
		// so a batch full of stale jobs does not make the loop stop early.
		worked, err := s.deliverDue(ctx)
		if err != nil {
			s.logger.Error("Reminder delivery pass failed", "error", err)
			return
		}
		if !worked {
			break
		}
	}
}

// Calendars with reminders enabled get delivery jobs for the events whose
// send-window has opened. Jobs already queued for the same delivery key are
// reconciled, not blindly re-inserted: a canceled job is rearmed, a pending job
// with a live claim is left alone, and a sent or permanently failed job is
// never revived.
func (s *ReminderScheduler) enqueueDue(ctx context.Context) {
	calendars, err := s.calendarRepo.ListForReminderScan(ctx)
	if err != nil {
		s.logger.Error("Failed to list calendars for reminder scan", "error", err)
		return
	}

	now := s.now()
	if s.watchingSince.IsZero() {
		s.watchingSince = now
	}
	for _, calendar := range calendars {
		// A shutdown mid-scan would otherwise run, and log, a failing count
		// query for every remaining calendar.
		if ctx.Err() != nil {
			return
		}
		s.enqueueCalendar(ctx, calendar, now)
	}
}

func (s *ReminderScheduler) enqueueCalendar(ctx context.Context, calendar *calendarModels.Calendar, now time.Time) {
	cfg, err := s.calendarConfig(calendar)
	if err != nil {
		s.logger.Error("Failed to parse notify config for reminder scan",
			"calendar_id", calendar.ID, "error", err)
		return
	}
	if !cfg.Enabled || !cfg.Reminders.Enabled {
		return
	}

	hoursBefore := s.effectiveHoursBefore(&cfg)
	if hoursBefore <= 0 {
		return
	}

	specs := s.planDelivery(calendar, &cfg)
	if len(specs) == 0 {
		return
	}

	loc := s.timezoneOf(calendar)

	for _, date := range s.upcomingEventDates(ctx, calendar, now, hoursBefore) {
		// The scheduled instant is the event's local midnight minus the lead
		// time, in the calendar's own timezone.
		scheduledAt := s.eventMidnight(date, loc).Add(-hoursBefore).UTC()
		for _, spec := range specs {
			// A backend with no SMTP cannot ever deliver an email job; other
			// channels are unaffected.
			if spec.channel == "email" && !s.emailService.IsConfigured() {
				continue
			}

			job := &models.ReminderJob{
				CalendarID:    calendar.ID,
				EventDate:     date,
				RecipientType: spec.recipientType,
				Channel:       spec.channel,
				ScheduledAt:   scheduledAt,
				MaxAttempts:   s.maxAttempts,
				NextAttemptAt: scheduledAt,
			}
			// The catch-up window bounds what an outage may deliver late. A window
			// that opened while this process was scanning was not missed: the
			// event qualified afterwards (someone joined the day before), and
			// it is delivered now while the event is still ahead.
			if now.Sub(scheduledAt) > s.catchUp && scheduledAt.Before(s.watchingSince) {
				recorded, err := s.jobs.RecordMissed(ctx, job)
				if err != nil {
					s.logger.Error("Failed to record missed reminder", "calendar_id", calendar.ID, "error", err)
				} else if recorded {
					s.logger.Warn("Reminder catch-up window missed", "calendar_id", calendar.ID,
						"event_date", date.Format("2006-01-02"), "channel", spec.channel,
						"scheduled_at", scheduledAt, "catch_up_window", s.catchUp.String())
				}
				continue
			}
			if err := s.jobs.Enqueue(ctx, job); err != nil {
				s.logger.Error("Failed to enqueue reminder job",
					"calendar_id", calendar.ID,
					"date", date.Format("2006-01-02"),
					"channel", spec.channel,
					"error", err)
			}
		}
	}
}

// effectiveHoursBefore resolves the lead time a calendar actually uses: the
// calendar's own hours_before if set, the operator default otherwise.
func (s *ReminderScheduler) effectiveHoursBefore(cfg *models.NotifyConfig) time.Duration {
	hours := cfg.Reminders.HoursBefore
	if hours <= 0 {
		hours = int(s.defaultHours / time.Hour)
	}
	if hours <= 0 {
		return 0
	}
	return time.Duration(hours) * time.Hour
}

func (s *ReminderScheduler) timezoneOf(calendar *calendarModels.Calendar) *time.Location {
	loc, err := time.LoadLocation(calendar.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (s *ReminderScheduler) calendarConfig(calendar *calendarModels.Calendar) (models.NotifyConfig, error) {
	var cfg models.NotifyConfig
	if calendar.NotifyConfig == nil || *calendar.NotifyConfig == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(*calendar.NotifyConfig), &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// planDelivery is the static half of the delivery plan: which (recipient class,
// channel) pairs a reminder opens, derived from the notify config. The dynamic
// half — who among participants is available — is resolved at send time.
// Participant email also requires real SMTP, which the enqueue and delivery
// paths check separately; this method only encodes the owner's consent.
type jobSpec struct {
	recipientType string
	channel       string
}

func (s *ReminderScheduler) planDelivery(calendar *calendarModels.Calendar, cfg *models.NotifyConfig) []jobSpec {
	var specs []jobSpec

	if cfg.NotifyOwner {
		for _, ch := range s.enabledChannels(cfg) {
			specs = append(specs, jobSpec{models.ReminderRecipientOwner, ch})
		}
	}
	// Participant reminders are email-only, and only when the calendar's full
	// consent configuration says participants should be told.
	if cfg.NotifyParticipants && cfg.Channels.Email.Enabled {
		specs = append(specs, jobSpec{models.ReminderRecipientParticipants, "email"})
	}

	return specs
}

func (s *ReminderScheduler) enabledChannels(cfg *models.NotifyConfig) []string {
	var channels []string
	if cfg.Channels.Email.Enabled {
		channels = append(channels, "email")
	}
	if cfg.Channels.Discord.Enabled && cfg.Channels.Discord.WebhookURL != "" {
		channels = append(channels, "discord")
	}
	if cfg.Channels.Slack.Enabled && cfg.Channels.Slack.WebhookURL != "" {
		channels = append(channels, "slack")
	}
	if cfg.Channels.Telegram.Enabled && cfg.Channels.Telegram.BotToken != "" && cfg.Channels.Telegram.ChatID != "" {
		channels = append(channels, "telegram")
	}
	return channels
}

// upcomingEventDates returns qualifying upcoming dates whose schedule is due
// (including missed catch-up windows) or opens within the next scan interval.
//
// The horizon is anchored to the calendar's own timezone, never to UTC: a day
// is a civil day in the calendar's zone, and the candidate dates run from the
// current civil date up to the civil date that now + hours_before +
// one scan interval lands on, walking with time.Date/AddDate(0,0,1) rather than
// a fixed number of hours. An event whose scheduled instant lies within the
// candidate horizon is then qualified through the same check as delivery.
func (s *ReminderScheduler) upcomingEventDates(
	ctx context.Context,
	calendar *calendarModels.Calendar,
	now time.Time,
	hoursBefore time.Duration,
) []time.Time {
	loc := s.timezoneOf(calendar)

	// Both bounds are instants converted into the calendar's zone; truncating a
	// UTC instant into a day would lose the civil date for a zone ahead of UTC.
	lower := now.In(loc)
	upper := now.Add(hoursBefore + s.interval).In(loc)

	first := time.Date(lower.Year(), lower.Month(), lower.Day(), 0, 0, 0, 0, loc)
	last := time.Date(upper.Year(), upper.Month(), upper.Day(), 0, 0, 0, 0, loc)

	var dates []time.Time
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		// The day marker stored on the job and used for availability queries is
		// the civil date at UTC midnight, matching every other day-shaped value
		// in the repository; the scheduled instant is derived from local midnight.
		marker := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		scheduledAt := s.eventMidnight(marker, loc).Add(-hoursBefore).UTC()
		if scheduledAt.Sub(now) > s.interval {
			continue
		}
		qualified, err := s.eventQualifiesAt(ctx, calendar, marker, now)
		if err != nil {
			s.logger.Error("Failed to count participants for reminder scan",
				"calendar_id", calendar.ID, "date", marker.Format("2006-01-02"), "error", err)
			continue
		}
		if qualified {
			dates = append(dates, marker)
		}
	}

	return dates
}

func (s *ReminderScheduler) eventMidnight(date time.Time, loc *time.Location) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
}

// deliverDue claims and delivers one batch of due jobs. It returns false when
// no more due jobs remain, so the caller can stop looping.
func (s *ReminderScheduler) deliverDue(ctx context.Context) (bool, error) {
	jobs, err := s.jobs.ClaimDue(ctx, s.instanceID, s.now(), s.lockTTL, s.batchSize)
	if err != nil {
		return false, err
	}
	if len(jobs) == 0 {
		return false, nil
	}

	for _, job := range jobs {
		s.deliverOne(ctx, job)
	}

	return true, nil
}

// deliverOne verifies and sends a single claimed job, fenced by the claim token
// the worker was handed. Verifying here is the cancellation half of the
// reliability story: a config change or a lost event since the job was enqueued
// cancels the job instead of sending stale mail.
//
// The three outcomes are deliberately distinct:
//
//   - individual delivery no longer wanted (channel/recipient off, SMTP gone)
//     cancels only this pending delivery, under its claim token — other
//     channels and recipients stay valid;
//   - an authoritatively confirmed non-qualifying or elapsed event cancels
//     this job under its claim; other jobs independently recheck eligibility;
//   - a transient error (calendar read, count query) retries the job with fenced
//     backoff, never cancels anything.
//
// A lost claim anywhere in here is a recognizable no-op: the token stops a late
// worker from retrying or canceling on behalf of the worker that won the claim.
func (s *ReminderScheduler) deliverOne(ctx context.Context, job models.ReminderJob) {
	if ctx.Err() != nil {
		return
	}
	jobLog := s.logger.With("job_id", job.ID,
		"calendar_id", job.CalendarID,
		"date", job.EventDate.Format("2006-01-02"),
		"channel", job.Channel)

	calendar, err := s.calendarRepo.GetByID(ctx, job.CalendarID)
	if err != nil {
		s.logger.Error("Failed to load calendar for reminder delivery",
			"calendar_id", job.CalendarID, "error", err)
		s.recordFailure(ctx, job, err)
		return
	}

	cfg, err := s.calendarConfig(calendar)
	if err != nil {
		s.logger.Error("Failed to parse notify config for reminder delivery",
			"calendar_id", job.CalendarID, "error", err)
		s.recordFailure(ctx, job, err)
		return
	}

	// Individual delivery validation: cancel this one pending delivery under its
	// own claim, leaving the event's other deliveries alone.
	if !s.jobStillWanted(calendar, &cfg, job) {
		jobLog.Info("Reminder job canceled: no longer configured")
		s.cancelClaim(ctx, job)
		return
	}

	// Email capability can vanish between enqueue and delivery. Suppress only
	// this delivery (rearmable under the reconciliation policy when SMTP
	// returns) rather than burning the attempt budget on a known-absent mailer.
	if job.Channel == "email" && !s.emailService.IsConfigured() {
		jobLog.Info("Reminder email suppressed: SMTP is not configured")
		s.cancelClaim(ctx, job)
		return
	}

	qualified, err := s.eventQualifies(ctx, calendar, job.EventDate)
	if err != nil {
		// Transient error checking the event: retry with backoff. A count query
		// failing must not cancel the whole event.
		s.logger.Error("Reminder event qualification check failed; will retry", "job_id", job.ID, "error", err)
		s.recordFailure(ctx, job, err)
		return
	}
	if !qualified {
		// This snapshot must not invalidate another worker's newer claim. Each
		// delivery checks the event itself and cancels only the claim it owns.
		jobLog.Info("Reminder event no longer qualifies; canceling owned delivery")
		s.cancelClaim(ctx, job)
		return
	}

	if err := s.sendJob(ctx, calendar, &cfg, job); err != nil {
		if errors.Is(err, errReminderEventElapsed) {
			jobLog.Info("Reminder event elapsed during fanout; canceling owned delivery")
			s.cancelClaim(ctx, job)
			return
		}
		if errors.Is(err, models.ErrClaimLost) || ctx.Err() != nil {
			jobLog.Info("Reminder delivery stopped: claim lost or context canceled", "error", err)
			return
		}
		s.logger.Error("Reminder delivery failed",
			"job_id", job.ID, "calendar_id", job.CalendarID,
			"date", job.EventDate.Format("2006-01-02"), "channel", job.Channel, "error", err)
		s.recordFailure(ctx, job, err)
		return
	}

	completionCtx, cancel := s.completionContext(ctx)
	defer cancel()
	if err := s.jobs.MarkSent(completionCtx, job.ID, job.ClaimToken); err != nil {
		s.logger.Error("Failed to mark reminder job sent", "job_id", job.ID, "error", err)
	}
}

// cancelClaim cancels one delivery under its claim token. If the claim was
// already lost, this is the worker that no longer owns the job and the no-op is
// real.
func (s *ReminderScheduler) cancelClaim(ctx context.Context, job models.ReminderJob) {
	if err := s.jobs.CancelClaim(ctx, job.ID, job.ClaimToken); err != nil {
		if errors.Is(err, models.ErrClaimLost) {
			s.logger.Info("Reminder job cancel lost its claim; treating as no-op", "job_id", job.ID)
			return
		}
		s.logger.Error("Failed to cancel reminder job", "job_id", job.ID, "error", err)
	}
}

// jobStillWanted reports whether a delivered job is still configured: reminders
// on, notifications on, the right recipients and channel enabled, and the
// scheduled time still matching the current hours_before. For participant email
// this re-reads the calendar's full consent configuration; SMTP capability is
// checked separately at delivery.
func (s *ReminderScheduler) jobStillWanted(calendar *calendarModels.Calendar, cfg *models.NotifyConfig, job models.ReminderJob) bool {
	if !cfg.Enabled || !cfg.Reminders.Enabled {
		return false
	}

	matched := false
	for _, spec := range s.planDelivery(calendar, cfg) {
		if spec.recipientType == job.RecipientType && spec.channel == job.Channel {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}

	// Timezone/hours-before change: the pending schedule is recomputed instead
	// of being delivered at the wrong instant. A drift beyond tolerance means
	// the job is rescheduled (cancel + rearm), not permanently canceled.
	loc := s.timezoneOf(calendar)
	scheduledAt := s.eventMidnight(job.EventDate, loc).
		Add(-s.effectiveHoursBefore(cfg)).UTC()

	return job.ScheduledAt.Sub(scheduledAt) <= reminderTimeTolerance &&
		scheduledAt.Sub(job.ScheduledAt) <= reminderTimeTolerance
}

func (s *ReminderScheduler) eventQualifies(ctx context.Context, calendar *calendarModels.Calendar, date time.Time) (bool, error) {
	return s.eventQualifiesAt(ctx, calendar, date, s.now())
}

func (s *ReminderScheduler) eventQualifiesAt(ctx context.Context, calendar *calendarModels.Calendar, date, now time.Time) (bool, error) {
	// Persisted retries can outlive the catch-up window. Do not send an
	// "upcoming" reminder after the event's calendar-local day has begun.
	if !s.eventMidnight(date, s.timezoneOf(calendar)).After(now) {
		return false, nil
	}
	if calendar.StartDate != nil && date.Before(*calendar.StartDate) {
		return false, nil
	}
	if calendar.EndDate != nil && date.After(*calendar.EndDate) {
		return false, nil
	}

	count, err := s.availabilityRepo.GetParticipantCountForDate(ctx, calendar.ID, date)
	if err != nil {
		return false, err
	}
	return count >= calendar.Threshold, nil
}

// recordFailure advances a job's attempt counter and schedules the next retry,
// or permanently fails it when attempts are exhausted. Fenced by the claim
// token: a lost claim is a no-op and does not burn an attempt against a job
// somebody else owns.
func (s *ReminderScheduler) recordFailure(ctx context.Context, job models.ReminderJob, cause error) {
	attempt := job.Attempt + 1
	next := s.now().Add(s.backoff(attempt))
	reason := "reminder_internal_failure"
	var providerErr *reminderProviderError
	if errors.As(cause, &providerErr) {
		reason = providerErr.channel + "_delivery_failed"
	}
	if err := s.jobs.MarkFailed(ctx, job.ID, job.ClaimToken, attempt, job.MaxAttempts, next, reason); err != nil {
		if errors.Is(err, models.ErrClaimLost) {
			s.logger.Info("Reminder job failure report lost its claim; treating as no-op", "job_id", job.ID)
			return
		}
		s.logger.Error("Failed to record reminder job failure", "job_id", job.ID, "error", err)
	}
}

// backoff computes the exponential backoff for a given attempt number: base
// doubled per attempt, capped at max.
func (s *ReminderScheduler) backoff(attempt int) time.Duration {
	d := s.backoffBase
	if d >= s.backoffMax {
		return s.backoffMax
	}
	for retry := 1; retry < attempt; retry++ {
		// Saturate before multiplying: converting an overflowed float to a
		// duration can wrap negative and produce a hot-loop retry schedule.
		if d > s.backoffMax/2 {
			return s.backoffMax
		}
		d *= 2
	}
	return d
}

// deliveryContext renews immediately before EACH external send, not once for
// an entire participant fanout. Renewal and delivery share a deadline shorter
// than the lease, leaving time to record the result. Lost claims stop fanout.
func (s *ReminderScheduler) deliveryContext(ctx context.Context, calendar *calendarModels.Calendar, job models.ReminderJob) (context.Context, context.CancelFunc, error) {
	if !s.eventMidnight(job.EventDate, s.timezoneOf(calendar)).After(s.now()) {
		return nil, nil, errReminderEventElapsed
	}
	bounded, cancel := context.WithTimeout(ctx, reminderSendTimeout)
	if err := bounded.Err(); err != nil {
		cancel()
		return nil, nil, err
	}
	if err := s.jobs.RenewLease(bounded, job.ID, job.ClaimToken, s.now(), s.lockTTL); err != nil {
		cancel()
		return nil, nil, err
	}
	return bounded, cancel, nil
}

// A confirmed provider acceptance must be recorded even if shutdown canceled
// the send context. This short, detached context is only for completion writes.
func (s *ReminderScheduler) completionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), reminderCompletionTimeout)
}

// sendJob delivers one job through the channel it names.
func (s *ReminderScheduler) sendJob(
	ctx context.Context,
	calendar *calendarModels.Calendar,
	cfg *models.NotifyConfig,
	job models.ReminderJob,
) error {
	switch job.Channel {
	case "discord", "slack", "telegram":
		if job.RecipientType != models.ReminderRecipientOwner {
			return fmt.Errorf("chat channel %q only reaches the owner", job.Channel)
		}
		return s.sendOwnerChat(ctx, calendar, cfg, job)
	case "email":
		if job.RecipientType == models.ReminderRecipientOwner {
			owner, err := s.userRepo.GetByID(ctx, calendar.OwnerID)
			if err != nil {
				return fmt.Errorf("load owner: %w", err)
			}
			return s.sendEmail(ctx, calendar, job, owner.Email, owner.Locale, nil)
		}
		return s.sendParticipantEmails(ctx, calendar, job)
	default:
		return fmt.Errorf("unknown reminder channel %q", job.Channel)
	}
}

// sendOwnerChat posts the reminder to the named chat channel for the owner,
// deduplicated by the notification ledger.
func (s *ReminderScheduler) sendOwnerChat(
	ctx context.Context,
	calendar *calendarModels.Calendar,
	cfg *models.NotifyConfig,
	job models.ReminderJob,
) error {
	channel, date := job.Channel, job.EventDate
	owner, err := s.userRepo.GetByID(ctx, calendar.OwnerID)
	if err != nil {
		return fmt.Errorf("load owner: %w", err)
	}

	text := s.buildReminderText(calendar, date)

	sent, err := s.notificationLog.WasReminderSent(
		ctx, calendar.ID, date, owner.ID, channel,
	)
	if err != nil {
		return fmt.Errorf("check notification log: %w", err)
	}
	if sent {
		return nil
	}
	sendCtx, cancel, err := s.deliveryContext(ctx, calendar, job)
	if err != nil {
		return err
	}
	defer cancel()

	var sendErr error
	switch channel {
	case "discord":
		sendErr = s.externalNotifier.SendDiscord(sendCtx, cfg.Channels.Discord.WebhookURL, text)
	case "slack":
		sendErr = s.externalNotifier.SendSlack(sendCtx, cfg.Channels.Slack.WebhookURL, text)
	case "telegram":
		sendErr = s.externalNotifier.SendTelegram(sendCtx, cfg.Channels.Telegram.BotToken, cfg.Channels.Telegram.ChatID, text)
	}
	if sendErr != nil {
		return &reminderProviderError{channel: channel, cause: sendErr}
	}

	completionCtx, complete := s.completionContext(ctx)
	defer complete()
	return s.notificationLog.LogNotification(
		completionCtx, calendar.ID, date, reminderEventType, "owner", owner.ID, channel,
	)
}

// sendParticipantEmails fans the reminder out to the verified participants who
// declared themselves available on the event date. Consent is re-read by the
// caller before this is reached: an owner who revoked participant mail, or an
// instance whose SMTP vanished, must not have a queued job fire afterwards.
func (s *ReminderScheduler) sendParticipantEmails(ctx context.Context, calendar *calendarModels.Calendar, job models.ReminderJob) error {
	date := job.EventDate
	available, err := s.availabilityRepo.GetAvailableParticipantsForDate(ctx, calendar.ID, date)
	if err != nil {
		return fmt.Errorf("load available participants: %w", err)
	}

	availableIDs := make(map[uuid.UUID]struct{}, len(available))
	for _, p := range available {
		availableIDs[p.ID] = struct{}{}
	}

	verified, err := s.participantRepo.GetVerifiedParticipantsByCalendar(ctx, calendar.ID)
	if err != nil {
		return fmt.Errorf("load verified participants: %w", err)
	}

	var lastErr error
	for _, p := range verified {
		_, availableNow := availableIDs[p.ID]
		if !availableNow || p.Email == nil || !p.EmailVerified {
			continue
		}
		if err := s.sendEmail(ctx, calendar, job, *p.Email, p.Locale, &p.ID); err != nil {
			if errors.Is(err, models.ErrClaimLost) || errors.Is(err, errReminderEventElapsed) || ctx.Err() != nil {
				return err
			}
			s.logger.Error("Failed to send participant reminder email",
				"recipient_ref", pkglog.Fingerprint(*p.Email),
				"participant_ref", pkglog.Fingerprint(p.ID.String()),
				"error", err)
			lastErr = err
		}
	}
	return lastErr
}

// sendEmail sends one reminder email, deduplicated by the notification ledger.
func (s *ReminderScheduler) sendEmail(
	ctx context.Context,
	calendar *calendarModels.Calendar,
	job models.ReminderJob,
	to, locale string,
	participantID *uuid.UUID,
) error {
	date := job.EventDate
	if !s.emailService.IsConfigured() {
		return fmt.Errorf("email not configured")
	}

	recipientID := calendar.OwnerID
	recipientType := "owner"
	if participantID != nil {
		recipientID = *participantID
		recipientType = "participant"
	}

	sent, err := s.notificationLog.WasReminderSent(
		ctx, calendar.ID, date, recipientID, "email",
	)
	if err != nil {
		return fmt.Errorf("check notification log: %w", err)
	}
	if sent {
		return nil
	}

	var calendarURL string
	if participantID != nil {
		calendarURL = fmt.Sprintf("%s/c/%s/p/%s", s.appURL, calendar.PublicToken, participantID.String())
	} else {
		calendarURL = fmt.Sprintf("%s/c/%s", s.appURL, calendar.PublicToken)
	}

	body := s.buildReminderEmail(calendar, date, calendarURL, locale, participantID != nil)

	s.logger.Info("Sending reminder email",
		"recipient_ref", pkglog.Fingerprint(to),
		"is_owner", participantID == nil)

	sendCtx, cancel, err := s.deliveryContext(ctx, calendar, job)
	if err != nil {
		return err
	}
	defer cancel()
	if err := s.emailService.SendContext(sendCtx, email.Email{
		To:      []string{to},
		Subject: reminderSubject(locale),
		Body:    body,
		HTML:    true,
	}); err != nil {
		return &reminderProviderError{channel: "email", cause: err}
	}

	completionCtx, complete := s.completionContext(ctx)
	defer complete()
	return s.notificationLog.LogNotification(
		completionCtx, calendar.ID, date, reminderEventType, recipientType, recipientID, "email",
	)
}

// buildReminderText is the plain-text form used by the chat channels.
func (s *ReminderScheduler) buildReminderText(calendar *calendarModels.Calendar, date time.Time) string {
	return fmt.Sprintf("🔔 Reminder: calendar '%s' has an event on %s.",
		calendar.Name, date.Format("2006-01-02"))
}

// buildReminderEmail is the HTML form used for the email channel.
func (s *ReminderScheduler) buildReminderEmail(
	calendar *calendarModels.Calendar,
	date time.Time,
	calendarURL string,
	locale string,
	participant bool,
) string {
	var title, bodyText, viewButton, cancelLabel string
	if locale == "fr" {
		title = "Rappel : un événement approche"
		bodyText = fmt.Sprintf("N'oubliez pas : l'événement \"%s\" a lieu le %s.",
			html.EscapeString(calendar.Name), date.Format("02/01/2006"))
		viewButton = "Voir le calendrier"
		cancelLabel = "Annuler ma participation"
	} else {
		title = "Reminder: an event is coming up"
		bodyText = fmt.Sprintf("Don't forget: the event \"%s\" is happening on %s.",
			html.EscapeString(calendar.Name), date.Format("2006-01-02"))
		viewButton = "View Calendar"
		cancelLabel = "Cancel my participation"
	}
	var cancelButton string
	if participant {
		cancelButton = fmt.Sprintf(`<p><a href="%s?cancel=%s" class="btn">%s</a></p>`,
			html.EscapeString(calendarURL), date.Format("2006-01-02"), cancelLabel)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; background-color: #f4f4f4; }
		.container { max-width: 600px; margin: 20px auto; padding: 30px; background-color: white; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
		.btn { display: inline-block; padding: 14px 28px; margin: 5px; background-color: #007bff; color: white !important; text-decoration: none; border-radius: 5px; font-weight: bold; }
	</style>
</head>
<body>
	<div class="container">
		<h1>🔔 %s</h1>
		<p>%s</p>
		<p><a href="%s" class="btn">%s</a></p>
		%s
	</div>
</body>
</html>`, title, bodyText, html.EscapeString(calendarURL), viewButton, cancelButton)
}

// reminderSubject localises the email subject line.
func reminderSubject(locale string) string {
	if locale == "fr" {
		return "Rappel WhenTo"
	}
	return "WhenTo Reminder"
}
