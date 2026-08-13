// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/broadcast"
	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/email"
	"github.com/whento/pkg/jwt"
	"github.com/whento/whento/internal/config"

	// Auth module
	authHandlers "github.com/whento/whento/internal/auth/handlers"
	authRepo "github.com/whento/whento/internal/auth/repository"
	authService "github.com/whento/whento/internal/auth/service"

	// Calendar module
	calendarHandlers "github.com/whento/whento/internal/calendar/handlers"
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
	calendarService "github.com/whento/whento/internal/calendar/service"

	// Availability module
	availabilityHandlers "github.com/whento/whento/internal/availability/handlers"
	availabilityRepo "github.com/whento/whento/internal/availability/repository"
	availabilityService "github.com/whento/whento/internal/availability/service"

	// ICS module
	icsHandlers "github.com/whento/whento/internal/ics/handlers"
	icsRepo "github.com/whento/whento/internal/ics/repository"
	icsService "github.com/whento/whento/internal/ics/service"

	// Passkey module
	passkeyHandlers "github.com/whento/whento/internal/passkey/handlers"
	passkeyRepo "github.com/whento/whento/internal/passkey/repository"
	passkeyService "github.com/whento/whento/internal/passkey/service"

	// MFA module
	mfaHandlers "github.com/whento/whento/internal/mfa/handlers"
	mfaRepo "github.com/whento/whento/internal/mfa/repository"
	mfaService "github.com/whento/whento/internal/mfa/service"

	// Notification module
	notifyHandlers "github.com/whento/whento/internal/notify/handlers"
	notifyRepo "github.com/whento/whento/internal/notify/repository"
	notifyService "github.com/whento/whento/internal/notify/service"

	// SEO module
	"github.com/whento/whento/internal/seo"
)

// deps are the process-wide resources every module is wired from.
//
// They are built in run(), where their shutdown is deferred, and handed to
// buildHandlers and to the route registrations read-only. Nothing in here is
// owned by this struct: run() closes the pool, the broker and the limiter.
type deps struct {
	cfg        *config.Config
	log        *slog.Logger
	pool       *pgxpool.Pool
	cacheStore cache.Cache
	jwtManager *jwt.Manager
	mailer     *email.Service
	broker     broadcast.Broker
	limiter    *routeLimiter
	quota      *Services

	// cacheProbe is nil when no Redis client was created — see run(), where the
	// distinction between "no cache configured" and "cache down" is made.
	cacheProbe authHandlers.Probe
}

// handlers holds every HTTP handler the router mounts, one field per handler,
// grouped by the module it belongs to.
//
// It exists so that dependency injection and routing are two separate readings:
// buildHandlers below answers "what is this built from", and the routes_*.go
// files answer "what is it reachable at", without either having to carry the
// other's thirty-odd local variables.
type handlers struct {
	// Auth
	health            *authHandlers.HealthHandler
	auth              *authHandlers.AuthHandler
	emailVerification *authHandlers.EmailVerificationHandler
	passwordReset     *authHandlers.PasswordResetHandler
	magicLink         *authHandlers.MagicLinkHandler
	adminMFA          *authHandlers.AdminMFAHandler

	// Passkey and MFA
	passkey *passkeyHandlers.PasskeyHandler
	mfa     *mfaHandlers.MFAHandler

	// Calendar
	calendar    *calendarHandlers.CalendarHandler
	participant *calendarHandlers.ParticipantHandler

	// Notification
	notifyConfig     *notifyHandlers.NotifyConfigHandler
	participantEmail *notifyHandlers.ParticipantEmailHandler

	// Availability
	availability *availabilityHandlers.AvailabilityHandler
	recurrence   *availabilityHandlers.RecurrenceHandler
	events       *availabilityHandlers.EventsHandler

	// ICS
	ics         *icsHandlers.ICSHandler
	unifiedFeed *icsHandlers.UnifiedFeedConfigHandler

	// SEO
	seo *seo.Handler
}

// buildHandlers is the whole of the manual dependency injection: repositories,
// then services, then handlers, in that order.
//
// The order is not free. Three of these are shared across module boundaries and
// have to exist before their second consumer does:
//
//   - userRepo, mfaRepository and passkeyRepository are built with the auth
//     module but read by the MFA, passkey and auth handlers;
//   - the calendar repositories are built with the calendar module and read by
//     the notification service;
//   - the availability service is built *after* the notification service,
//     because it notifies through it. That single edge is what used to be
//     recorded as "availabilitySvc initialization moved after NOTIFICATION
//     MODULE" in the middle of the availability block; the repositories it needs
//     are still declared with the rest of its module, and only the service
//     construction waits.
func buildHandlers(d *deps) (*handlers, error) {
	// ========== AUTH MODULE ==========
	userRepo := authRepo.NewUserRepository(d.pool)
	tokenRepo := authRepo.NewTokenRepository(d.pool)
	mfaRepository := mfaRepo.NewMFARepository(d.pool)

	// The MFA repository is for the 2FA check during login; the cache is for
	// temporary-token replay prevention.
	authSvc := authService.NewAuthService(userRepo, tokenRepo, mfaRepository, d.jwtManager, d.cacheStore, d.cfg.BcryptCost, d.cfg.AllowedRegister, d.cfg.AllowedEmails)
	passwordResetSvc := authService.NewPasswordResetService(userRepo, tokenRepo, d.mailer, d.jwtManager, d.cfg, d.log, d.cfg.BcryptCost)
	magicLinkSvc := authService.NewMagicLinkService(userRepo, tokenRepo, d.mailer, d.jwtManager, d.cfg, d.log)

	// ========== PASSKEY MODULE ==========
	passkeyRepository := passkeyRepo.NewPasskeyRepository(d.pool)

	passkeySvc, err := passkeyService.NewPasskeyService(passkeyRepository, userRepo, d.cfg, d.cacheStore, d.log)
	if err != nil {
		return nil, fmt.Errorf("initialize passkey service: %w", err)
	}
	d.log.Info("Passkey service initialized")

	// The auth service is for completing a login once the ceremony succeeds.
	passkeyHandler := passkeyHandlers.NewPasskeyHandler(passkeySvc, authSvc, d.log)

	// ========== MFA MODULE ==========
	mfaSvc := mfaService.NewMFAService(mfaRepository, userRepo, tokenRepo, d.cfg, d.log)
	d.log.Info("MFA service initialized")

	mfaHandler := mfaHandlers.NewMFAHandler(mfaSvc, authSvc, d.jwtManager, d.cacheStore, d.log)

	// ========== AUTH HANDLERS (need the passkey and MFA repositories) ==========
	authHandler := authHandlers.NewAuthHandler(authSvc, userRepo, d.mailer, d.cfg, d.log, mfaRepository, passkeyRepository)

	// ========== CALENDAR MODULE ==========
	calendarRepository := calendarRepo.NewCalendarRepository(d.pool)
	participantRepository := calendarRepo.NewParticipantRepository(d.pool)

	// The user repository is what gives the owner participant an email address.
	calendarSvc := calendarService.NewCalendarService(calendarRepository, participantRepository, userRepo, d.cacheStore)

	// ========== AVAILABILITY MODULE (repositories) ==========
	availabilityRepository := availabilityRepo.NewAvailabilityRepository(d.pool)
	availCalendarRepo := availabilityRepo.NewCalendarRepository(d.pool)
	availParticipantRepo := availabilityRepo.NewParticipantRepository(d.pool)
	recurrenceRepository := availabilityRepo.NewRecurrenceRepository(d.pool)

	// ========== ICS MODULE ==========
	icsCalendarRepo := icsRepo.NewCalendarRepository(d.pool)
	icsAvailabilityRepo := icsRepo.NewAvailabilityRepository(d.pool)
	unifiedFeedRepo := icsRepo.NewUnifiedFeedRepository(d.pool)

	// The quota checker is what blocks feeds for an over-quota user.
	icsSvc := icsService.NewICSService(icsCalendarRepo, icsAvailabilityRepo, unifiedFeedRepo, d.quota.QuotaService, d.cfg.AppURL)
	unifiedFeedConfigSvc := icsService.NewUnifiedFeedConfigService(unifiedFeedRepo)

	// ========== NOTIFICATION MODULE ==========
	notificationLogRepo := notifyRepo.NewNotificationLogRepository(d.pool)

	thresholdDetector := notifyService.NewThresholdDetector(availabilityRepository, d.log)
	externalNotifier := notifyService.NewExternalNotifier(d.log)

	notifySvc := notifyService.NewNotifyService(
		calendarRepository,
		participantRepository,
		availabilityRepository,
		userRepo,
		notificationLogRepo,
		d.mailer,
		externalNotifier,
		thresholdDetector,
		d.cfg.AppURL,
		d.log,
	)

	participantEmailSvc := notifyService.NewParticipantEmailService(
		participantRepository,
		d.mailer,
		d.cfg,
		d.log,
	)

	// ========== AVAILABILITY SERVICE (depends on the notification service) ==========
	availabilitySvc := availabilityService.NewAvailabilityService(
		availabilityRepository,
		availCalendarRepo,
		availParticipantRepo,
		recurrenceRepository,
		notifySvc,
		d.cacheStore,
	)

	return &handlers{
		health:            authHandlers.NewHealthHandler(d.pool, d.cacheProbe),
		auth:              authHandler,
		emailVerification: authHandlers.NewEmailVerificationHandler(authSvc, userRepo, d.mailer, d.cfg, d.log),
		passwordReset:     authHandlers.NewPasswordResetHandler(passwordResetSvc),
		magicLink:         authHandlers.NewMagicLinkHandler(magicLinkSvc, d.mailer, d.log),
		adminMFA:          authHandlers.NewAdminMFAHandler(mfaSvc, d.log),

		passkey: passkeyHandler,
		mfa:     mfaHandler,

		// The quota service is what enforces the calendar limit on creation.
		calendar:    calendarHandlers.NewCalendarHandler(calendarSvc, d.quota.QuotaService, userRepo, d.cfg, d.pool),
		participant: calendarHandlers.NewParticipantHandler(calendarSvc),

		notifyConfig: notifyHandlers.NewNotifyConfigHandler(
			calendarRepository,
			d.log,
		),
		participantEmail: notifyHandlers.NewParticipantEmailHandler(
			participantEmailSvc,
			participantRepository,
			calendarRepository,
			d.log,
		),

		availability: availabilityHandlers.NewAvailabilityHandler(availabilitySvc),
		recurrence:   availabilityHandlers.NewRecurrenceHandler(availabilitySvc),
		events:       availabilityHandlers.NewEventsHandler(availCalendarRepo, d.broker),

		ics:         icsHandlers.NewICSHandler(icsSvc),
		unifiedFeed: icsHandlers.NewUnifiedFeedConfigHandler(unifiedFeedConfigSvc),

		seo: seo.NewHandler(d.cfg.AppURL, d.cfg.DisableRobots, buildType),
	}, nil
}
