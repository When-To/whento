// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"math"
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

	// defaultReminderScanInterval is how often the scheduler issues new jobs and
	// attempts delivery of due ones. It bounds how late a reminder can be.
	defaultReminderScanInterval = 5 * time.Minute

	// defaultReminderLockTTL is how long a claimed-but-undelivered job stays
	// locked. A crashed instance hands its jobs back after this.
	defaultReminderLockTTL = 10 * time.Minute

	// defaultReminderBatchSize caps how many jobs one scan delivers, so a backlog
	// of due senders does not monopolise the loop.
	defaultReminderBatchSize = 50

	// defaultReminderCatchUp is how far into the past a freshly enqueued job may
	// be. After a restart this lets reminders that became due while the process
	// was down still go out, while ones that expired longer ago are dropped.
	defaultReminderCatchUp = 15 * time.Minute

	// reminderBackoffBase and reminderBackoffMax scale the retry schedule:
	// 1m, 2m, 4m, ... capped at the max, permanently failing after max_attempts.
	reminderBackoffBase = 1 * time.Minute
	reminderBackoffMax  = 24 * time.Hour
	reminderMaxAttempts = 5

	// reminderTimeTolerance is how much drift between a job's recorded
	// scheduled_at and the freshly computed one is accepted before the job is
	// rescheduled. A change to hours_before shows up here.
	reminderTimeTolerance = 30 * time.Second
)

// ReminderJobStore is the persistence behind the delivery loop.
//
// Declared here rather than taking *repository.ReminderJobRepository so the
// scheduler can be exercised without a database. The concrete repository
// satisfies it structurally; every operation it names is atomic in SQL, which
// is the whole point of a reliable scheduler.
type ReminderJobStore interface {
	Enqueue(ctx context.Context, job *models.ReminderJob) error
	ClaimDue(ctx context.Context, instanceID string, now time.Time, lockTTL time.Duration, limit int) ([]models.ReminderJob, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, attempt, maxAttempts int, nextAttemptAt time.Time, reason string) error
	CancelPendingForEvent(ctx context.Context, calendarID uuid.UUID, eventDate time.Time) error
}

// ReminderCalendarStore lists the calendars the scheduler has to inspect, and
// lets a claimed job's calendar be re-read at delivery time.
type ReminderCalendarStore interface {
	ListWithNotifyConfig(ctx context.Context) ([]*calendarModels.Calendar, error)
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
//   - a job is claimed atomically by exactly one instance (ClaimDue), with
//     SKIP LOCKED so several instances can never double-deliver;
//   - deliveries are retried with backoff and permanently failed after N tries;
//   - a redeploy is survived: send-windows that passed while the process was
//     down are re-enqueued into a catch-up grace period;
//   - nothing is sent before being verified again at delivery time, so a config
//     change or a lost event cancels the job instead of producing stale mail.
type ReminderScheduler struct {
	calendarRepo     ReminderCalendarStore
	availabilityRepo ReminderAvailabilityStore
	participantRepo  ParticipantStore
	userRepo         UserStore
	notificationLog  NotificationLog
	emailService     Mailer
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
	now              func() time.Time
	logger           *slog.Logger
}

// NewReminderScheduler creates a reminder scheduler with the production
// defaults. Tests can shrink the tunables through the exported fields to get a
// deterministic, fast schedule.
func NewReminderScheduler(
	calendarRepo ReminderCalendarStore,
	availabilityRepo ReminderAvailabilityStore,
	participantRepo ParticipantStore,
	userRepo UserStore,
	notificationLog NotificationLog,
	emailService Mailer,
	externalNotifier ChannelNotifier,
	jobs ReminderJobStore,
	appURL string,
	instanceID string,
	logger *slog.Logger,
) *ReminderScheduler {
	return &ReminderScheduler{
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
		interval:         defaultReminderScanInterval,
		lockTTL:          defaultReminderLockTTL,
		batchSize:        defaultReminderBatchSize,
		catchUp:          defaultReminderCatchUp,
		backoffBase:      reminderBackoffBase,
		backoffMax:       reminderBackoffMax,
		maxAttempts:      reminderMaxAttempts,
		now:              time.Now,
		logger:           logger,
	}
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

	s.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Reminder scheduler stopped")
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

// runOnce issues then delivers.
func (s *ReminderScheduler) runOnce(ctx context.Context) {
	s.enqueueDue(ctx)

	// Deliver in batches until the queue is drained or the interval elapses.
	deadline := s.now().Add(s.interval)
	for s.now().Before(deadline) {
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
// left alone (idempotent enqueue), which is what makes redeploys safe.
func (s *ReminderScheduler) enqueueDue(ctx context.Context) {
	calendars, err := s.calendarRepo.ListWithNotifyConfig(ctx)
	if err != nil {
		s.logger.Error("Failed to list calendars for reminder scan", "error", err)
		return
	}

	now := s.now()
	for _, calendar := range calendars {
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

	hoursBefore := time.Duration(cfg.Reminders.HoursBefore) * time.Hour
	if hoursBefore <= 0 {
		return
	}

	loc, err := time.LoadLocation(calendar.Timezone)
	if err != nil {
		loc = time.UTC
	}

	specs := s.planDelivery(calendar, &cfg)

	for _, date := range s.upcomingEventDates(ctx, calendar, now) {
		scheduledAt := s.eventMidnight(date, loc).Add(-hoursBefore).UTC()
		if !s.windowOpen(scheduledAt, now) {
			continue
		}

		for _, spec := range specs {
			job := &models.ReminderJob{
				CalendarID:    calendar.ID,
				EventDate:     date,
				RecipientType: spec.recipientType,
				Channel:       spec.channel,
				ScheduledAt:   scheduledAt,
				MaxAttempts:   s.maxAttempts,
				NextAttemptAt: scheduledAt,
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

// upcomingEventDates returns the calendar's event dates (participant count at or
// above threshold, within its date range) in the next daysToScan days.
func (s *ReminderScheduler) upcomingEventDates(ctx context.Context, calendar *calendarModels.Calendar, now time.Time) []time.Time {
	// hours_before ≤ 168h = 7 days; one extra day of margin covers the boundary.
	const daysToScan = 8
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var dates []time.Time
	for day := 0; day < daysToScan; day++ {
		date := today.AddDate(0, 0, day)

		if calendar.StartDate != nil && date.Before(*calendar.StartDate) {
			continue
		}
		if calendar.EndDate != nil && date.After(*calendar.EndDate) {
			continue
		}

		count, err := s.availabilityRepo.GetParticipantCountForDate(ctx, calendar.ID, date)
		if err != nil {
			s.logger.Error("Failed to count participants for reminder scan",
				"calendar_id", calendar.ID, "date", date.Format("2006-01-02"), "error", err)
			continue
		}
		if count >= calendar.Threshold {
			dates = append(dates, date)
		}
	}

	return dates
}

func (s *ReminderScheduler) eventMidnight(date time.Time, loc *time.Location) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
}

// windowOpen reports whether a job scheduled at `scheduledAt` should be issued
// now: not too far into the future, and not so far in the past that it has
// already expired. The past threshold is the catch-up grace that makes restarts
// safe.
func (s *ReminderScheduler) windowOpen(scheduledAt, now time.Time) bool {
	return scheduledAt.Sub(now) <= s.interval && now.Sub(scheduledAt) <= s.catchUp
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

// deliverOne verifies and sends a single claimed job. Verifying here is the
// cancellation half of the reliability story: a config change or a lost event
// since the job was enqueued cancels the job instead of sending stale mail.
func (s *ReminderScheduler) deliverOne(ctx context.Context, job models.ReminderJob) {
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

	if !s.jobStillWanted(calendar, &cfg, job) {
		s.logger.Info("Reminder job canceled: no longer configured",
			"job_id", job.ID, "date", job.EventDate.Format("2006-01-02"), "channel", job.Channel)
		_ = s.jobs.CancelPendingForEvent(ctx, job.CalendarID, job.EventDate)
		return
	}

	qualified, err := s.eventQualifies(ctx, calendar, job.EventDate)
	if err != nil || !qualified {
		s.logger.Info("Reminder job canceled: event no longer qualifies",
			"job_id", job.ID, "date", job.EventDate.Format("2006-01-02"), "error", err)
		_ = s.jobs.CancelPendingForEvent(ctx, job.CalendarID, job.EventDate)
		return
	}

	if err := s.sendJob(ctx, calendar, &cfg, job); err != nil {
		s.logger.Error("Reminder delivery failed",
			"job_id", job.ID, "calendar_id", job.CalendarID,
			"date", job.EventDate.Format("2006-01-02"), "channel", job.Channel, "error", err)
		s.recordFailure(ctx, job, err)
		return
	}

	if err := s.jobs.MarkSent(ctx, job.ID); err != nil {
		s.logger.Error("Failed to mark reminder job sent", "job_id", job.ID, "error", err)
	}
}

// jobStillWanted reports whether a delivered job is still configured: reminders
// on, notifications on, the right recipients and channel enabled, and the
// scheduled time still matching the current hours_before.
func (s *ReminderScheduler) jobStillWanted(calendar *calendarModels.Calendar, cfg *models.NotifyConfig, job models.ReminderJob) bool {
	if !cfg.Enabled || !cfg.Reminders.Enabled || cfg.Reminders.HoursBefore <= 0 {
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

	loc, err := time.LoadLocation(calendar.Timezone)
	if err != nil {
		loc = time.UTC
	}
	scheduledAt := s.eventMidnight(job.EventDate, loc).
		Add(-time.Duration(cfg.Reminders.HoursBefore) * time.Hour).UTC()

	return job.ScheduledAt.Sub(scheduledAt) <= reminderTimeTolerance &&
		scheduledAt.Sub(job.ScheduledAt) <= reminderTimeTolerance
}

func (s *ReminderScheduler) eventQualifies(ctx context.Context, calendar *calendarModels.Calendar, date time.Time) (bool, error) {
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
// or permanently fails it when attempts are exhausted.
func (s *ReminderScheduler) recordFailure(ctx context.Context, job models.ReminderJob, cause error) {
	attempt := job.Attempt + 1
	next := s.now().Add(s.backoff(attempt))
	if err := s.jobs.MarkFailed(ctx, job.ID, attempt, s.maxAttempts, next, cause.Error()); err != nil {
		s.logger.Error("Failed to record reminder job failure", "job_id", job.ID, "error", err)
	}
}

// backoff computes the exponential backoff for a given attempt number: base
// doubled per attempt, capped at max.
func (s *ReminderScheduler) backoff(attempt int) time.Duration {
	multiplier := math.Pow(2, float64(maxInt(0, attempt-1)))
	d := time.Duration(float64(s.backoffBase) * multiplier)
	if d > s.backoffMax {
		return s.backoffMax
	}
	return d
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
		return s.sendOwnerChat(ctx, calendar, cfg, job.Channel, job.EventDate)
	case "email":
		if job.RecipientType == models.ReminderRecipientOwner {
			owner, err := s.userRepo.GetByID(ctx, calendar.OwnerID)
			if err != nil {
				return fmt.Errorf("load owner: %w", err)
			}
			return s.sendEmail(ctx, calendar, job.EventDate, owner.Email, owner.Locale, nil)
		}
		return s.sendParticipantEmails(ctx, calendar, job.EventDate)
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
	channel string,
	date time.Time,
) error {
	owner, err := s.userRepo.GetByID(ctx, calendar.OwnerID)
	if err != nil {
		return fmt.Errorf("load owner: %w", err)
	}

	text := s.buildReminderText(calendar, date)

	sent, err := s.notificationLog.WasNotificationSentRecently(
		ctx, calendar.ID, date, reminderEventType, owner.ID, channel,
	)
	if err != nil {
		return fmt.Errorf("check notification log: %w", err)
	}
	if sent {
		return nil
	}

	var sendErr error
	switch channel {
	case "discord":
		sendErr = s.externalNotifier.SendDiscord(ctx, cfg.Channels.Discord.WebhookURL, text)
	case "slack":
		sendErr = s.externalNotifier.SendSlack(ctx, cfg.Channels.Slack.WebhookURL, text)
	case "telegram":
		sendErr = s.externalNotifier.SendTelegram(ctx, cfg.Channels.Telegram.BotToken, cfg.Channels.Telegram.ChatID, text)
	}
	if sendErr != nil {
		return sendErr
	}

	_ = s.notificationLog.LogNotification(
		ctx, calendar.ID, date, reminderEventType, "owner", owner.ID, channel,
	)
	return nil
}

// sendParticipantEmails fans the reminder out to the verified participants who
// declared themselves available on the event date.
func (s *ReminderScheduler) sendParticipantEmails(ctx context.Context, calendar *calendarModels.Calendar, date time.Time) error {
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
		if err := s.sendEmail(ctx, calendar, date, *p.Email, p.Locale, &p.ID); err != nil {
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
	date time.Time,
	to, locale string,
	participantID *uuid.UUID,
) error {
	if !s.emailService.IsConfigured() {
		return fmt.Errorf("email not configured")
	}

	recipientID := calendar.OwnerID
	recipientType := "owner"
	if participantID != nil {
		recipientID = *participantID
		recipientType = "participant"
	}

	sent, err := s.notificationLog.WasNotificationSentRecently(
		ctx, calendar.ID, date, reminderEventType, recipientID, "email",
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

	body := s.buildReminderEmail(calendar, date, calendarURL, locale)

	s.logger.Info("Sending reminder email",
		"recipient_ref", pkglog.Fingerprint(to),
		"is_owner", participantID == nil)

	if err := s.emailService.Send(email.Email{
		To:      []string{to},
		Subject: reminderSubject(locale),
		Body:    body,
		HTML:    true,
	}); err != nil {
		return err
	}

	return s.notificationLog.LogNotification(
		ctx, calendar.ID, date, reminderEventType, recipientType, recipientID, "email",
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
) string {
	var title, bodyText, viewButton string
	if locale == "fr" {
		title = "Rappel : un événement approche"
		bodyText = fmt.Sprintf("N'oubliez pas : l'événement \"%s\" a lieu le %s.",
			html.EscapeString(calendar.Name), date.Format("02/01/2006"))
		viewButton = "Voir le calendrier"
	} else {
		title = "Reminder: an event is coming up"
		bodyText = fmt.Sprintf("Don't forget: the event \"%s\" is happening on %s.",
			html.EscapeString(calendar.Name), date.Format("2006-01-02"))
		viewButton = "View Calendar"
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
	</div>
</body>
</html>`, title, bodyText, calendarURL, viewButton)
}

// reminderSubject localises the email subject line.
func reminderSubject(locale string) string {
	if locale == "fr" {
		return "Rappel WhenTo"
	}
	return "WhenTo Reminder"
}
