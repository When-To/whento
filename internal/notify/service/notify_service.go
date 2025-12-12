// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/whento/pkg/email"
	authRepo "github.com/whento/whento/internal/auth/repository"
	availabilityModels "github.com/whento/whento/internal/availability/models"
	availabilityRepo "github.com/whento/whento/internal/availability/repository"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/notify/models"
	notifyRepo "github.com/whento/whento/internal/notify/repository"
)

// NotifyService orchestrates notification sending
type NotifyService struct {
	calendarRepo     *calendarRepo.CalendarRepository
	participantRepo  *calendarRepo.ParticipantRepository
	availabilityRepo *availabilityRepo.AvailabilityRepository
	userRepo         *authRepo.UserRepository
	notificationLog  *notifyRepo.NotificationLogRepository
	emailService     *email.Service
	externalNotifier *ExternalNotifier
	detector         *ThresholdDetector
	appURL           string
	logger           *slog.Logger
}

// NewNotifyService creates a new notification service
func NewNotifyService(
	calendarRepo *calendarRepo.CalendarRepository,
	participantRepo *calendarRepo.ParticipantRepository,
	availabilityRepo *availabilityRepo.AvailabilityRepository,
	userRepo *authRepo.UserRepository,
	notificationLog *notifyRepo.NotificationLogRepository,
	emailService *email.Service,
	externalNotifier *ExternalNotifier,
	detector *ThresholdDetector,
	appURL string,
	logger *slog.Logger,
) *NotifyService {
	return &NotifyService{
		calendarRepo:     calendarRepo,
		participantRepo:  participantRepo,
		availabilityRepo: availabilityRepo,
		userRepo:         userRepo,
		notificationLog:  notificationLog,
		emailService:     emailService,
		externalNotifier: externalNotifier,
		detector:         detector,
		appURL:           appURL,
		logger:           logger,
	}
}

// emailRecipient holds information about an email notification recipient
type emailRecipient struct {
	Email         string
	Name          string
	Locale        string
	ParticipantID *uuid.UUID // nil for owner-only, set for participants
	RecipientID   uuid.UUID  // user ID for owner, participant ID for participants
	IsOwner       bool
}

// CheckThresholdAndNotify is the main entry point called from availability service
func (s *NotifyService) CheckThresholdAndNotify(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
	previousCount int, // Pass -1 if unknown
) error {
	s.logger.Debug("CheckThresholdAndNotify called",
		"calendar_id", calendarID,
		"date", date.Format("2006-01-02"),
		"previous_count", previousCount)

	// Get calendar with notify config
	calendar, err := s.calendarRepo.GetByID(ctx, calendarID)
	if err != nil {
		s.logger.Error("Failed to get calendar for notification", "calendar_id", calendarID, "error", err)
		return err
	}

	s.logger.Debug("Calendar retrieved",
		"calendar_id", calendarID,
		"notify_on_threshold", calendar.NotifyOnThreshold,
		"has_notify_config", calendar.NotifyConfig != nil)

	// Check if notifications enabled
	if !calendar.NotifyOnThreshold || calendar.NotifyConfig == nil {
		s.logger.Debug("Notifications disabled for calendar",
			"calendar_id", calendarID,
			"notify_on_threshold", calendar.NotifyOnThreshold,
			"has_config", calendar.NotifyConfig != nil)
		return nil // Notifications disabled
	}

	// Parse notify config
	var config models.NotifyConfig
	if err := json.Unmarshal([]byte(*calendar.NotifyConfig), &config); err != nil {
		s.logger.Error("Failed to parse notify config", "calendar_id", calendarID, "error", err)
		return fmt.Errorf("failed to parse notify config: %w", err)
	}

	s.logger.Debug("Notify config parsed",
		"calendar_id", calendarID,
		"enabled", config.Enabled,
		"notify_owner", config.NotifyOwner,
		"notify_participants", config.NotifyParticipants)

	if !config.Enabled {
		s.logger.Debug("Notifications disabled in config", "calendar_id", calendarID)
		return nil
	}

	// Detect threshold transition
	transition, err := s.detector.DetectTransition(ctx, calendarID, date, calendar.Threshold, previousCount)
	if err != nil {
		s.logger.Error("Failed to detect threshold transition", "calendar_id", calendarID, "error", err)
		return err
	}

	s.logger.Debug("Transition detected",
		"calendar_id", calendarID,
		"type", transition.TransitionType,
		"new_count", transition.NewCount,
		"previous_count", transition.PreviousCount,
		"threshold", calendar.Threshold)

	// Only notify on actual transitions (reached or lost)
	if transition.TransitionType == "none" {
		s.logger.Debug("No transition to notify", "calendar_id", calendarID)
		return nil
	}

	s.logger.Info("Threshold transition detected - SENDING NOTIFICATIONS",
		"calendar_id", calendarID,
		"date", date.Format("2006-01-02"),
		"type", transition.TransitionType,
		"count", transition.NewCount,
		"threshold", calendar.Threshold)

	// Send notifications to recipients
	// First handle external notifications (Discord, Slack, Telegram) - owner only
	if config.NotifyOwner {
		s.logger.Debug("Sending external notifications to owner", "calendar_id", calendarID)
		if err := s.notifyOwnerExternalChannels(ctx, calendar, transition, config); err != nil {
			s.logger.Error("Failed to send external notifications to owner", "calendar_id", calendarID, "error", err)
		}
	}

	// Then handle email notifications with deduplication
	if config.NotifyOwner || config.NotifyParticipants {
		s.logger.Debug("Sending email notifications (deduplicated)", "calendar_id", calendarID)
		if err := s.sendDeduplicatedEmailNotifications(ctx, calendar, transition, config); err != nil {
			s.logger.Error("Failed to send deduplicated email notifications", "calendar_id", calendarID, "error", err)
		}
	}

	return nil
}

// notifyOwnerExternalChannels sends external notifications (Discord, Slack, Telegram) to calendar owner
// Email notifications are handled separately via sendDeduplicatedEmailNotifications
func (s *NotifyService) notifyOwnerExternalChannels(
	ctx context.Context,
	calendar *calendarModels.Calendar,
	transition *models.ThresholdTransition,
	config models.NotifyConfig,
) error {
	s.logger.Debug("notifyOwnerExternalChannels called", "calendar_id", calendar.ID, "owner_id", calendar.OwnerID)

	// Get owner user
	owner, err := s.userRepo.GetByID(ctx, calendar.OwnerID)
	if err != nil {
		s.logger.Error("Failed to get owner user", "owner_id", calendar.OwnerID, "error", err)
		return fmt.Errorf("failed to get owner: %w", err)
	}

	s.logger.Debug("Owner retrieved for external notifications", "owner_id", owner.ID)

	// Build notification message for external channels (text-only)
	textMessage := s.buildNotificationMessage(calendar, transition)

	s.logger.Debug("Checking Discord channel",
		"enabled", config.Channels.Discord.Enabled,
		"has_webhook", config.Channels.Discord.WebhookURL != "")

	if config.Channels.Discord.Enabled && config.Channels.Discord.WebhookURL != "" {
		sent, _ := s.notificationLog.WasNotificationSentRecently(
			ctx, calendar.ID, transition.Date, transition.TransitionType, owner.ID, "discord",
		)
		if !sent {
			s.logger.Info("Sending Discord notification", "webhook_url", config.Channels.Discord.WebhookURL[:20]+"...")
			if err := s.externalNotifier.SendDiscord(ctx, config.Channels.Discord.WebhookURL, textMessage); err != nil {
				s.logger.Error("Failed to send Discord notification", "error", err)
			} else {
				s.logger.Info("Discord notification sent successfully")
				_ = s.notificationLog.LogNotification(
					ctx, calendar.ID, transition.Date, transition.TransitionType, "owner", owner.ID, "discord",
				)
			}
		} else {
			s.logger.Debug("Discord notification already sent recently")
		}
	} else {
		s.logger.Debug("Discord channel disabled or webhook not configured")
	}

	s.logger.Debug("Checking Slack channel",
		"enabled", config.Channels.Slack.Enabled,
		"has_webhook", config.Channels.Slack.WebhookURL != "")

	if config.Channels.Slack.Enabled && config.Channels.Slack.WebhookURL != "" {
		sent, _ := s.notificationLog.WasNotificationSentRecently(
			ctx, calendar.ID, transition.Date, transition.TransitionType, owner.ID, "slack",
		)
		if !sent {
			s.logger.Info("Sending Slack notification", "webhook_url", config.Channels.Slack.WebhookURL[:20]+"...")
			if err := s.externalNotifier.SendSlack(ctx, config.Channels.Slack.WebhookURL, textMessage); err != nil {
				s.logger.Error("Failed to send Slack notification", "error", err)
			} else {
				s.logger.Info("Slack notification sent successfully")
				_ = s.notificationLog.LogNotification(
					ctx, calendar.ID, transition.Date, transition.TransitionType, "owner", owner.ID, "slack",
				)
			}
		} else {
			s.logger.Debug("Slack notification already sent recently")
		}
	} else {
		s.logger.Debug("Slack channel disabled or webhook not configured")
	}

	s.logger.Debug("Checking Telegram channel",
		"enabled", config.Channels.Telegram.Enabled,
		"has_token", config.Channels.Telegram.BotToken != "",
		"has_chat_id", config.Channels.Telegram.ChatID != "")

	if config.Channels.Telegram.Enabled && config.Channels.Telegram.BotToken != "" && config.Channels.Telegram.ChatID != "" {
		sent, _ := s.notificationLog.WasNotificationSentRecently(
			ctx, calendar.ID, transition.Date, transition.TransitionType, owner.ID, "telegram",
		)
		if !sent {
			s.logger.Info("Sending Telegram notification", "chat_id", config.Channels.Telegram.ChatID)
			if err := s.externalNotifier.SendTelegram(
				ctx, config.Channels.Telegram.BotToken, config.Channels.Telegram.ChatID, textMessage,
			); err != nil {
				s.logger.Error("Failed to send Telegram notification", "error", err)
			} else {
				s.logger.Info("Telegram notification sent successfully")
				_ = s.notificationLog.LogNotification(
					ctx, calendar.ID, transition.Date, transition.TransitionType, "owner", owner.ID, "telegram",
				)
			}
		} else {
			s.logger.Debug("Telegram notification already sent recently")
		}
	} else {
		s.logger.Debug("Telegram channel disabled or credentials not configured")
	}

	s.logger.Debug("notifyOwnerExternalChannels completed")
	return nil
}

// sendDeduplicatedEmailNotifications collects all email recipients (owner + participants)
// and sends one email per unique email address to prevent duplicates
func (s *NotifyService) sendDeduplicatedEmailNotifications(
	ctx context.Context,
	calendar *calendarModels.Calendar,
	transition *models.ThresholdTransition,
	config models.NotifyConfig,
) error {
	s.logger.Debug("sendDeduplicatedEmailNotifications called", "calendar_id", calendar.ID)

	// Check if email channel is enabled and SMTP configured
	if !config.Channels.Email.Enabled || !s.emailService.IsConfigured() {
		s.logger.Debug("Email channel disabled or SMTP not configured, skipping email notifications")
		return nil
	}

	// Map to store unique email recipients (key = email address)
	recipients := make(map[string]*emailRecipient)

	// 1. Collect owner recipient if NotifyOwner is enabled
	if config.NotifyOwner {
		owner, err := s.userRepo.GetByID(ctx, calendar.OwnerID)
		if err != nil {
			s.logger.Error("Failed to get owner user", "owner_id", calendar.OwnerID, "error", err)
		} else {
			// Find owner's participant ID
			participants, err := s.participantRepo.GetByCalendarID(ctx, calendar.ID)
			if err != nil {
				s.logger.Error("Failed to get participants for owner", "calendar_id", calendar.ID, "error", err)
			} else {
				var ownerParticipantID *uuid.UUID
				for _, p := range participants {
					if p.Name == owner.DisplayName {
						ownerParticipantID = &p.ID
						break
					}
				}

				recipients[owner.Email] = &emailRecipient{
					Email:         owner.Email,
					Name:          owner.DisplayName,
					Locale:        owner.Locale,
					ParticipantID: ownerParticipantID,
					RecipientID:   owner.ID,
					IsOwner:       true,
				}

				s.logger.Debug("Owner added to email recipients",
					"email", owner.Email,
					"has_participant_id", ownerParticipantID != nil)
			}
		}
	}

	// Get availabilities for the specific date to find who has availability
	// This will be used both for participant notification filtering and for building the participant list
	availabilities, err := s.availabilityRepo.GetByDate(ctx, calendar.ID, transition.Date)
	if err != nil {
		s.logger.Error("Failed to get availabilities for date", "calendar_id", calendar.ID, "date", transition.Date, "error", err)
		availabilities = []*availabilityModels.Availability{} // Empty list on error
	}

	// Build a map of participant IDs who have availability on this date
	participantIDsWithAvailability := make(map[uuid.UUID]bool)
	for _, avail := range availabilities {
		participantIDsWithAvailability[avail.ParticipantID] = true
	}

	s.logger.Debug("Participants with availability on date", "count", len(participantIDsWithAvailability))

	// Build list of participant names for display in email
	participantNames := make([]string, 0, len(participantIDsWithAvailability))
	if len(participantIDsWithAvailability) > 0 {
		// Get all participants to build the name list
		allParticipants, err := s.participantRepo.GetByCalendarID(ctx, calendar.ID)
		if err != nil {
			s.logger.Error("Failed to get all participants for name list", "calendar_id", calendar.ID, "error", err)
		} else {
			for _, p := range allParticipants {
				if participantIDsWithAvailability[p.ID] {
					participantNames = append(participantNames, p.Name)
				}
			}
		}
	}

	s.logger.Debug("Participant names collected for email", "count", len(participantNames), "names", participantNames)

	// 2. Collect participant recipients if NotifyParticipants is enabled
	if config.NotifyParticipants {
		if len(participantIDsWithAvailability) == 0 {
			s.logger.Debug("No participants with availability on this date, skipping participant notifications")
		} else {

			// Get verified participants
			verifiedParticipants, err := s.participantRepo.GetVerifiedParticipantsByCalendar(ctx, calendar.ID)
			if err != nil {
				s.logger.Error("Failed to get verified participants", "calendar_id", calendar.ID, "error", err)
			} else {
				s.logger.Debug("Verified participants retrieved", "count", len(verifiedParticipants))

				// Add participants with verified emails who have availability on this date
				for _, p := range verifiedParticipants {
					// Only notify participants who have availability on this specific date
					if !participantIDsWithAvailability[p.ID] {
						s.logger.Debug("Skipping participant - no availability on this date",
							"participant_id", p.ID,
							"name", p.Name,
							"date", transition.Date)
						continue
					}

					if p.Email != nil && p.EmailVerified {
						// If this email already exists in recipients (e.g., owner), keep the existing one
						// The owner record has more complete info (IsOwner flag, user ID)
						if existing, exists := recipients[*p.Email]; exists {
							s.logger.Debug("Email already in recipients (likely owner), skipping duplicate",
								"email", *p.Email,
								"participant_name", p.Name,
								"existing_name", existing.Name)
							continue
						}

						pid := p.ID
						recipients[*p.Email] = &emailRecipient{
							Email:         *p.Email,
							Name:          p.Name,
							Locale:        p.Locale,
							ParticipantID: &pid,
							RecipientID:   p.ID,
							IsOwner:       false,
						}

						s.logger.Debug("Participant added to email recipients",
							"email", *p.Email,
							"name", p.Name)
					}
				}
			}
		}
	}

	s.logger.Info("Email recipients collected (deduplicated)",
		"total_unique_recipients", len(recipients),
		"calendar_id", calendar.ID)

	// 3. Send one email per unique recipient
	for email, recipient := range recipients {
		// Check if not sent recently (anti-spam)
		sent, err := s.notificationLog.WasNotificationSentRecently(
			ctx, calendar.ID, transition.Date, transition.TransitionType, recipient.RecipientID, "email",
		)
		if err != nil {
			s.logger.Error("Failed to check notification log", "email", email, "error", err)
		}

		if sent {
			s.logger.Debug("Email notification already sent recently, skipping",
				"email", email,
				"recipient_id", recipient.RecipientID)
			continue
		}

		// Build recipient-specific calendar URL
		var calendarURL string
		if recipient.ParticipantID != nil {
			// Recipient has a participant - link to their participant view
			calendarURL = fmt.Sprintf("%s/c/%s/p/%s", s.appURL, calendar.PublicToken, recipient.ParticipantID.String())
		} else {
			// Fallback to public calendar view
			calendarURL = fmt.Sprintf("%s/c/%s", s.appURL, calendar.PublicToken)
		}

		htmlMessage := s.buildHTMLNotificationMessage(calendar, transition, calendarURL, recipient.ParticipantID != nil, recipient.Locale, participantNames)

		s.logger.Info("Sending email notification",
			"email", email,
			"name", recipient.Name,
			"is_owner", recipient.IsOwner,
			"url", calendarURL)

		if err := s.sendEmailNotification(ctx, recipient.Email, recipient.Name, htmlMessage, recipient.Locale, true); err != nil {
			s.logger.Error("Failed to send email",
				"email", email,
				"recipient_id", recipient.RecipientID,
				"error", err)
		} else {
			s.logger.Info("Email notification sent successfully",
				"email", email,
				"recipient_id", recipient.RecipientID)

			// Log notification
			recipientType := "participant"
			if recipient.IsOwner {
				recipientType = "owner"
			}
			_ = s.notificationLog.LogNotification(
				ctx, calendar.ID, transition.Date, transition.TransitionType, recipientType, recipient.RecipientID, "email",
			)
		}
	}

	s.logger.Debug("sendDeduplicatedEmailNotifications completed")
	return nil
}

// buildNotificationMessage creates the notification content (text for non-email channels)
func (s *NotifyService) buildNotificationMessage(
	calendar *calendarModels.Calendar,
	transition *models.ThresholdTransition,
) string {
	dateStr := transition.Date.Format("2006-01-02")

	if transition.TransitionType == "threshold_reached" {
		return fmt.Sprintf(
			"🎉 Calendar '%s': Threshold reached for %s! (%d/%d participants available)",
			calendar.Name,
			dateStr,
			transition.NewCount,
			transition.Threshold,
		)
	} else if transition.TransitionType == "threshold_lost" {
		return fmt.Sprintf(
			"⚠️ Calendar '%s': Threshold lost for %s (%d/%d participants)",
			calendar.Name,
			dateStr,
			transition.NewCount,
			transition.Threshold,
		)
	}

	return fmt.Sprintf(
		"Calendar '%s': Availability changed for %s (%d/%d participants)",
		calendar.Name,
		dateStr,
		transition.NewCount,
		transition.Threshold,
	)
}

// buildHTMLNotificationMessage creates HTML notification with calendar link
func (s *NotifyService) buildHTMLNotificationMessage(
	calendar *calendarModels.Calendar,
	transition *models.ThresholdTransition,
	calendarURL string,
	hasParticipantID bool,
	locale string,
	participantNames []string,
) string {
	dateStr := transition.Date.Format("2006-01-02")

	// Translations
	var emoji, messageText, calendarLabel, dateLabel, participantsLabel, participantListLabel, viewButton, cancelButtonText string

	if locale == "fr" {
		calendarLabel = "Calendrier :"
		dateLabel = "Date :"
		participantsLabel = "Participants disponibles :"
		participantListLabel = "Liste des participants :"
		viewButton = "Voir le calendrier"
		cancelButtonText = "Annuler ma participation"

		if transition.TransitionType == "threshold_reached" {
			emoji = "🎉"
			messageText = fmt.Sprintf(
				"Seuil atteint pour %s ! (%d/%d participants disponibles)",
				dateStr,
				transition.NewCount,
				transition.Threshold,
			)
		} else if transition.TransitionType == "threshold_lost" {
			emoji = "⚠️"
			messageText = fmt.Sprintf(
				"Seuil perdu pour %s (%d/%d participants)",
				dateStr,
				transition.NewCount,
				transition.Threshold,
			)
		} else {
			messageText = fmt.Sprintf(
				"Disponibilité modifiée pour %s (%d/%d participants)",
				dateStr,
				transition.NewCount,
				transition.Threshold,
			)
		}
	} else {
		// Default to English
		calendarLabel = "Calendar:"
		dateLabel = "Date:"
		participantsLabel = "Participants available:"
		participantListLabel = "Participant list:"
		viewButton = "View Calendar"
		cancelButtonText = "Cancel my participation"

		if transition.TransitionType == "threshold_reached" {
			emoji = "🎉"
			messageText = fmt.Sprintf(
				"Threshold reached for %s! (%d/%d participants available)",
				dateStr,
				transition.NewCount,
				transition.Threshold,
			)
		} else if transition.TransitionType == "threshold_lost" {
			emoji = "⚠️"
			messageText = fmt.Sprintf(
				"Threshold lost for %s (%d/%d participants)",
				dateStr,
				transition.NewCount,
				transition.Threshold,
			)
		} else {
			messageText = fmt.Sprintf(
				"Availability changed for %s (%d/%d participants)",
				dateStr,
				transition.NewCount,
				transition.Threshold,
			)
		}
	}

	// Build participant list HTML
	var participantListHTML string
	if len(participantNames) > 0 {
		participantListHTML = fmt.Sprintf(`<div class="participant-list">
			<div class="participant-list-header">%s</div>
			<ul class="participant-names">`, participantListLabel)
		for _, name := range participantNames {
			participantListHTML += fmt.Sprintf(`<li>%s</li>`, name)
		}
		participantListHTML += `</ul></div>`
	}

	// Build cancel URL with date parameter (only if recipient has participant ID)
	var cancelButton string
	if hasParticipantID {
		cancelURL := fmt.Sprintf("%s?cancel=%s", calendarURL, dateStr)
		cancelButton = fmt.Sprintf(`<a href="%s" class="btn btn-danger">%s</a>`, cancelURL, cancelButtonText)
	}

	// Build HTML with clickable calendar link and conditional cancel button
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; background-color: #f4f4f4; }
		.container { max-width: 600px; margin: 20px auto; padding: 30px; background-color: white; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
		.header { font-size: 24px; margin-bottom: 20px; color: #333; }
		.calendar-name { color: #007bff; font-weight: bold; }
		.message { font-size: 16px; margin-bottom: 10px; line-height: 1.8; }
		.date-info { font-size: 18px; font-weight: bold; color: #555; margin: 15px 0; }
		.buttons { margin-top: 30px; text-align: center; }
		.btn {
			display: inline-block;
			padding: 14px 28px;
			margin: 5px;
			text-decoration: none;
			border-radius: 5px;
			font-weight: bold;
			font-size: 16px;
			transition: background-color 0.3s;
		}
		.btn-primary {
			background-color: #007bff;
			color: white !important;
		}
		.btn-primary:hover {
			background-color: #0056b3;
		}
		.btn-danger {
			background-color: #dc3545;
			color: white !important;
		}
		.btn-danger:hover {
			background-color: #c82333;
		}
		.participant-list {
			margin: 20px 0;
			padding: 15px;
			background-color: #f8f9fa;
			border-radius: 5px;
			border-left: 4px solid #007bff;
		}
		.participant-list-header {
			font-weight: bold;
			margin-bottom: 10px;
			color: #333;
		}
		.participant-names {
			list-style: none;
			padding: 0;
			margin: 0;
		}
		.participant-names li {
			padding: 5px 0;
			color: #555;
		}
		.participant-names li:before {
			content: "✓ ";
			color: #28a745;
			font-weight: bold;
			margin-right: 8px;
		}
	</style>
</head>
<body>
	<div class="container">
		<div class="header">%s %s</div>
		<div class="message">
			%s <span class="calendar-name">%s</span>
		</div>
		<div class="date-info">%s %s</div>
		<div class="message">
			%s <strong>%d/%d</strong>
		</div>
		%s
		<div class="buttons">
			<a href="%s" class="btn btn-primary">%s</a>
			%s
		</div>
	</div>
</body>
</html>
	`, emoji, messageText, calendarLabel, calendar.Name, dateLabel, dateStr, participantsLabel, transition.NewCount, transition.Threshold, participantListHTML, calendarURL, viewButton, cancelButton)

	return html
}

// sendEmailNotification sends email notification
func (s *NotifyService) sendEmailNotification(
	ctx context.Context,
	to string,
	name string,
	message string,
	locale string,
	isHTML bool,
) error {
	subject := "WhenTo Calendar Notification"
	if locale == "fr" {
		subject = "Notification de Calendrier WhenTo"
	}

	return s.emailService.Send(email.Email{
		To:      []string{to},
		Subject: subject,
		Body:    message,
		HTML:    isHTML,
	})
}
