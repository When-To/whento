// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//	@title						WhenTo API
//	@version					1.6.3
//	@description				WhenTo is a self-hosted web application for organizing events among friends through collaborative calendars.
//	@description				Each calendar answers a simple question: **when can we meet?** Participants indicate their availability, and time slots reaching a defined threshold become events accessible via an **iCalendar subscription URL** — automatic synchronization in Google Calendar, Apple Calendar, Outlook, etc.
//	@description
//	@description				## Authentication
//	@description				Most endpoints require authentication via JWT Bearer token. To authenticate:
//	@description				1. Register a new account via `/api/v1/auth/register` or login via `/api/v1/auth/login`
//	@description				2. Use the returned `access_token` in the `Authorization` header: `Bearer <access_token>`
//	@description				3. Refresh your token when needed via `/api/v1/auth/refresh`
//	@description
//	@description				## Rate Limiting
//	@description				The API implements rate limiting to prevent abuse:
//	@description				- **Public auth endpoints**: 3-5 requests/minute/IP
//	@description				- **Public calendar endpoints**: 60 requests/minute/IP
//	@description				- **ICS feed endpoints**: 30 requests/minute/IP
//	@description				- **Live availability stream (SSE)**: 300 connections/minute/IP per calendar
//	@description				- **Authenticated endpoints**: 100 requests/minute/user
//	@description
//	@description				Rate limit headers are included in responses: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`
//	@description
//	@description				## Deployment Modes
//	@description				WhenTo supports two deployment modes: **Cloud** (SaaS with Stripe subscriptions) and **Self-hosted** (Ed25519 cryptographic licenses). Some endpoints are only available in specific deployment modes.
//
//	@contact.name				WhenTo Support
//	@contact.url				https://github.com/When-To/whento
//
//	@license.name				Business Source License 1.1
//	@license.url				https://github.com/When-To/whento/blob/main/LICENSE
//
//	@BasePath					/
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and JWT token.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/whento/pkg/broadcast"
	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/database"
	"github.com/whento/pkg/email"
	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/metrics"
	"github.com/whento/pkg/middleware"
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

	// SEO module
	"github.com/whento/whento/internal/seo"

	// Notification module
	notifyHandlers "github.com/whento/whento/internal/notify/handlers"
	notifyRepo "github.com/whento/whento/internal/notify/repository"
	notifyService "github.com/whento/whento/internal/notify/service"

	// Frontend embedding
	"github.com/whento/whento/web"

	// Swagger docs (generated)
	swaggerDocs "github.com/whento/whento/docs/swagger"
)

// main does nothing but set the exit status.
//
// os.Exit skips every deferred call, so a single one of them anywhere inside
// the startup sequence would leak the connection pool, the Redis client and the
// broadcast broker. Everything therefore happens in run(), which reports
// failures by returning them, and os.Exit is reached only once run() and all its
// defers are done.
func main() {
	if err := run(); err != nil {
		logger.Error("WhenTo exited with an error", "error", err)
		os.Exit(1)
	}
}

// shutdownBudget is how long the server is given to finish in-flight requests.
const shutdownBudget = 10 * time.Second

// streamDrainDelay is how long ordinary requests get to finish before the base
// context is cancelled and the event streams are told to leave.
//
// A stream has no natural end, so without this every open SSE connection sits
// out the whole shutdown budget and the process takes shutdownBudget to stop
// however idle it is. Cancelling immediately instead would abort the ordinary
// requests that Shutdown is there to protect — a POST in the middle of its
// transaction would see a cancelled context. Two seconds is longer than any
// non-streaming handler here takes and far shorter than the budget.
const streamDrainDelay = 2 * time.Second

// defaultMetricsPort is where the exposition lands when metrics are enabled
// without naming a port.
const defaultMetricsPort = "9090"

// run wires every component together, serves, and blocks until a signal or a
// fatal error. It returns instead of exiting so that its defers — the pool, the
// Redis client, the broker, the rate limiter's sweeper — all get to run.
func run() error {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	log := logger.New(cfg.LogLevel, "json")
	logger.SetDefault(log)

	// Fail fast on malformed URL-shaped env vars (DATABASE_URL, REDIS_URL, APP_URL).
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// The incoherences that leave one feature dead and the rest of the instance
	// serviceable. Refusing to start over them would turn an upgrade into an
	// outage for a deployment that was already running that way.
	for _, warning := range cfg.Warnings() {
		log.Warn(warning)
	}

	logBuildInfo(log)
	log.Info("Starting WhenTo Application", "port", cfg.Port, "env", cfg.AppEnv)

	// The generated spec carries the version frozen into the annotation at the last
	// `make swagger`; the linker knows the real one. Overriding it here keeps
	// /swagger honest for the binary that is serving it.
	swaggerDocs.SwaggerInfo.Version = Version

	// One series, two labels, both fixed at link time: this is the only place a
	// metric label is allowed to carry free-form text.
	metrics.SetBuildInfo(Version, buildType)

	// Context for initialization
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to PostgreSQL (shared by all modules)
	dbConfig := &database.Config{
		URL: cfg.DatabaseURL,
	}
	pool, err := database.NewPool(ctx, dbConfig)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer database.Close(pool)
	log.Info("Connected to PostgreSQL")

	// Pool saturation is the first thing to look at when latency rises, and the
	// collector reads pool.Stat() at scrape time, so this costs nothing until
	// somebody scrapes. A duplicate registration is a programming error, not a
	// reason to refuse to serve calendars.
	if err := metrics.RegisterPool(database.PoolStatsFunc(pool)); err != nil {
		log.Warn("Database pool metrics not registered", "error", err)
	}

	// Connect to Redis (shared by all modules) - Optional
	redisConfig := &database.RedisConfig{
		URL: cfg.RedisURL,
	}
	redisClient, err := database.NewRedisClient(ctx, redisConfig)
	if err != nil {
		log.Warn("Failed to connect to Redis - running without cache", "error", err)
		redisClient = nil
	} else {
		defer func() { _ = database.CloseRedis(redisClient) }()
		log.Info("Connected to Redis - cache enabled")
	}

	// Initialize JWT manager
	jwtConfig := &jwt.Config{
		PrivateKeyPath: cfg.JWTPrivateKeyPath,
		PublicKeyPath:  cfg.JWTPublicKeyPath,
		AccessExpiry:   cfg.JWTAccessExpiry,
		RefreshExpiry:  cfg.JWTRefreshExpiry,
		Issuer:         cfg.JWTIssuer,
	}
	jwtManager, err := jwt.NewManager(jwtConfig)
	if err != nil {
		return fmt.Errorf("initialize JWT manager: %w", err)
	}
	log.Info("JWT manager initialized")

	// Cache keys are stored as digests so that Redis never holds a calendar
	// token, an ICS token, a participant id or an email address in a key name.
	// The default salt is a fixed constant, which is what lets several instances
	// derive the same key for the same row — pinning a secret here additionally
	// makes those digests untestable against a list of candidate addresses.
	//
	// RATE_LIMIT_KEY_SALT is reused deliberately: both are "the secret that salts
	// what we put in Redis", and asking an operator for two of them would mostly
	// produce one that is set and one that is not. Unlike the rate limiter, an
	// unset salt here costs nothing operationally, so it is not warned about.
	cache.SetKeySalt(cfg.RateLimitKeySalt)

	// Initialize cache (uses Redis if available, NoOp otherwise)
	cacheInstance := cache.NewRedisCache(redisClient)
	if cacheInstance.IsEnabled() {
		log.Info("Cache enabled (Redis)")
	} else {
		log.Info("Cache disabled (Redis not available)")
	}

	// Initialize email service
	emailService := email.NewService(email.Config{
		Host:        cfg.Email.SMTPHost,
		Port:        cfg.Email.SMTPPort,
		Username:    cfg.Email.SMTPUsername,
		Password:    cfg.Email.SMTPPassword,
		FromAddress: cfg.Email.FromAddress,
		FromName:    cfg.Email.FromName,
	}, log)
	if emailService.IsConfigured() {
		log.Info("Email service configured", "smtp_host", cfg.Email.SMTPHost)
		log.Info("Email verification", "enable", cfg.Email.VerificationEnabled)
	} else {
		log.Info("Email service not configured (email features disabled)")
	}

	// ========== QUOTA MODULE ==========
	// Initialize build-specific services (Cloud: per-user limit, Self-hosted: unlimited)
	services, err := InitServices(ctx, cfg, pool)
	if err != nil {
		return fmt.Errorf("initialize quota services: %w", err)
	}

	// ========== AUTH MODULE ==========
	// Initialize auth repositories
	userRepo := authRepo.NewUserRepository(pool)
	tokenRepo := authRepo.NewTokenRepository(pool)
	mfaRepository := mfaRepo.NewMFARepository(pool)

	// Initialize auth service (with MFA repository for 2FA checking and cache for temp token replay prevention)
	authSvc := authService.NewAuthService(userRepo, tokenRepo, mfaRepository, jwtManager, cacheInstance, cfg.BcryptCost, cfg.AllowedRegister, cfg.AllowedEmails)

	// Initialize password reset service
	passwordResetSvc := authService.NewPasswordResetService(userRepo, tokenRepo, emailService, jwtManager, cfg, log, cfg.BcryptCost)

	// Initialize magic link service
	magicLinkSvc := authService.NewMagicLinkService(userRepo, tokenRepo, emailService, jwtManager, cfg, log)

	// ========== PASSKEY MODULE ==========
	// Initialize passkey repository
	passkeyRepository := passkeyRepo.NewPasskeyRepository(pool)

	// Initialize passkey service
	passkeySvc, err := passkeyService.NewPasskeyService(passkeyRepository, userRepo, cfg, cacheInstance, log)
	if err != nil {
		return fmt.Errorf("initialize passkey service: %w", err)
	}
	log.Info("Passkey service initialized")

	// Initialize passkey handler (with auth service for completing login)
	passkeyHandler := passkeyHandlers.NewPasskeyHandler(passkeySvc, authSvc, log)

	// ========== AUTH HANDLERS (requires passkey and MFA repositories) ==========
	// Initialize auth handlers (with MFA and passkey repos for status checking)
	authHandler := authHandlers.NewAuthHandler(authSvc, userRepo, emailService, cfg, log, mfaRepository, passkeyRepository)
	emailVerificationHandler := authHandlers.NewEmailVerificationHandler(authSvc, userRepo, emailService, cfg, log)
	passwordResetHandler := authHandlers.NewPasswordResetHandler(passwordResetSvc)
	magicLinkHandler := authHandlers.NewMagicLinkHandler(magicLinkSvc, emailService, log)

	// Readiness probes what the instance actually needs: PostgreSQL, which is
	// fatal, and Redis, which is not. The Redis probe stays nil when no client
	// was created — assigning a nil *RedisPinger to the interface would produce
	// a non-nil interface holding nil, and readiness would report a hard "down"
	// for a dependency the operator chose not to run.
	var cacheProbe authHandlers.Probe
	if redisPinger := database.NewRedisPinger(redisClient); redisPinger != nil {
		cacheProbe = redisPinger
	}
	authHealthHandler := authHandlers.NewHealthHandler(pool, cacheProbe)

	// ========== MFA MODULE ==========
	// Initialize MFA service (repository already created for auth service)
	mfaSvc := mfaService.NewMFAService(mfaRepository, userRepo, tokenRepo, cfg, log)
	log.Info("MFA service initialized")

	// Initialize MFA handler (with auth service for completing login)
	mfaHandler := mfaHandlers.NewMFAHandler(mfaSvc, authSvc, jwtManager, cacheInstance, log)

	// Initialize admin MFA handler for admin operations (disable 2FA)
	adminMFAHandler := authHandlers.NewAdminMFAHandler(mfaSvc, log)

	// ========== CALENDAR MODULE ==========
	// Initialize calendar repositories
	calendarRepository := calendarRepo.NewCalendarRepository(pool)
	participantRepository := calendarRepo.NewParticipantRepository(pool)

	// Initialize calendar service with cache and user repo (for owner participant email)
	calendarSvc := calendarService.NewCalendarService(calendarRepository, participantRepository, userRepo, cacheInstance)

	// Initialize calendar handlers (with quota service for limit checking)
	calendarHandler := calendarHandlers.NewCalendarHandler(calendarSvc, services.QuotaService, userRepo, cfg, pool)
	participantHandler := calendarHandlers.NewParticipantHandler(calendarSvc)

	// ========== AVAILABILITY MODULE ==========
	// Initialize availability repositories
	availabilityRepository := availabilityRepo.NewAvailabilityRepository(pool)
	availCalendarRepo := availabilityRepo.NewCalendarRepository(pool)
	availParticipantRepo := availabilityRepo.NewParticipantRepository(pool)
	recurrenceRepository := availabilityRepo.NewRecurrenceRepository(pool)

	// Note: availabilitySvc initialization moved after NOTIFICATION MODULE
	// because it depends on notifySvc

	// ========== ICS MODULE ==========
	// Initialize ICS repositories
	icsCalendarRepo := icsRepo.NewCalendarRepository(pool)
	icsAvailabilityRepo := icsRepo.NewAvailabilityRepository(pool)
	unifiedFeedRepo := icsRepo.NewUnifiedFeedRepository(pool)

	// Initialize ICS service (with quota checker to block feeds for over-quota users)
	icsSvc := icsService.NewICSService(icsCalendarRepo, icsAvailabilityRepo, unifiedFeedRepo, services.QuotaService, cfg.AppURL)

	// Initialize unified feed config service and handler
	unifiedFeedConfigSvc := icsService.NewUnifiedFeedConfigService(unifiedFeedRepo)
	unifiedFeedConfigHandler := icsHandlers.NewUnifiedFeedConfigHandler(unifiedFeedConfigSvc)

	// Initialize ICS handlers
	icsHandler := icsHandlers.NewICSHandler(icsSvc)

	// ========== NOTIFICATION MODULE ==========
	// Initialize notification repositories
	notificationLogRepo := notifyRepo.NewNotificationLogRepository(pool)

	// Initialize notification services
	thresholdDetector := notifyService.NewThresholdDetector(availabilityRepository, log)
	externalNotifier := notifyService.NewExternalNotifier(log)

	notifySvc := notifyService.NewNotifyService(
		calendarRepository,
		participantRepository,
		availabilityRepository,
		userRepo,
		notificationLogRepo,
		emailService,
		externalNotifier,
		thresholdDetector,
		cfg.AppURL,
		log,
	)

	participantEmailSvc := notifyService.NewParticipantEmailService(
		participantRepository,
		emailService,
		cfg,
		log,
	)

	// Initialize notification handlers
	participantEmailHandler := notifyHandlers.NewParticipantEmailHandler(
		participantEmailSvc,
		participantRepository,
		calendarRepository,
		log,
	)

	notifyConfigHandler := notifyHandlers.NewNotifyConfigHandler(
		calendarRepository,
		log,
	)

	// ========== AVAILABILITY SERVICE (depends on notification service) ==========
	// Initialize availability service with cache and notification service
	availabilitySvc := availabilityService.NewAvailabilityService(
		availabilityRepository,
		availCalendarRepo,
		availParticipantRepo,
		recurrenceRepository,
		notifySvc,
		cacheInstance,
	)

	// Initialize availability handlers
	availabilityHandler := availabilityHandlers.NewAvailabilityHandler(availabilitySvc)
	recurrenceHandler := availabilityHandlers.NewRecurrenceHandler(availabilitySvc)

	// Initialize rate limiter.
	//
	// Buckets are stored under a salted hash so that Redis never holds an IP, a
	// calendar token or a participant id. That salt has to be shared by every
	// instance pointing at the same Redis: with a per-process one, the same
	// client hashes to a different bucket on each instance and N instances hand
	// out N times the configured allowance. A single instance is unaffected,
	// which is why an empty salt is allowed rather than fatal.
	middleware.SetRateLimitKeySalt(cfg.RateLimitKeySalt)
	if cfg.RateLimitEnabled && cfg.RateLimitKeySalt == "" && redisClient != nil {
		log.Warn("RATE_LIMIT_KEY_SALT is unset; rate limit buckets are per-instance. " +
			"Set it to a shared secret if more than one instance uses this Redis.")
	}
	rateLimiter := middleware.NewRateLimiter(redisClient)

	// The in-memory backend runs a sweeper goroutine for as long as the limiter
	// lives; without this it outlives the shutdown.
	defer rateLimiter.Stop()

	// Live calendar updates. Redis fans the notices out across instances; without it a
	// single-instance deployment still updates its own viewers, which is the whole of a
	// normal self-hosted install.
	broker := broadcast.NewRedisBroker(redisClient, log)
	defer func() { _ = broker.Close() }()
	eventsHandler := availabilityHandlers.NewEventsHandler(availCalendarRepo, broker)

	// Setup router
	r := chi.NewRouter()

	// Global middleware.
	//
	// chi's RealIP is deliberately absent, and must stay absent: it overwrites
	// r.RemoteAddr from X-Forwarded-For / X-Real-IP for *every* request, before
	// anything gets to ask whether the connection came from a trusted proxy. That
	// turns TRUSTED_PROXIES (below) into decoration and makes every per-IP rate
	// limit a client-chosen header away from being bypassed. Proxy headers are
	// honoured in exactly one place instead — middleware.IPKeyFunc — and only for
	// connections that actually originate from a configured proxy.
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	// Outside Recoverer, so a panic is counted as the 500 the client received
	// rather than not counted at all.
	r.Use(middleware.Metrics)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.LimitRequestSize(1 * 1024 * 1024)) // 1MB max payload
	r.Use(middleware.CORS(cfg.CORSOrigins))

	// Configure trusted proxies for X-Forwarded-For validation
	middleware.SetTrustedProxies(cfg.TrustedProxies)

	// Health routes (use auth health handler as primary)
	r.Get("/api/health", authHealthHandler.Health)
	r.Get("/api/ready", authHealthHandler.Ready)

	// ========== AUTH ROUTES ==========
	r.Route("/api/v1/auth", func(r chi.Router) {
		// Public routes with rate limiting
		r.Group(func(r chi.Router) {
			if cfg.RateLimitEnabled {
				// Login: 5 requests/minute/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Post("/login", authHandler.Login)

				// Register: 3 requests/minute/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 3,
					Window:   time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Post("/register", authHandler.Register)
			} else {
				r.Post("/login", authHandler.Login)
				r.Post("/register", authHandler.Register)
			}

			if cfg.RateLimitEnabled {
				// Refresh: 5 requests/minute/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Post("/refresh", authHandler.Refresh)
			} else {
				r.Post("/refresh", authHandler.Refresh)
			}
			r.Post("/logout", authHandler.Logout)

			// Password reset (public - no auth required)
			if cfg.RateLimitEnabled {
				// Forgot password: 3 requests/15 minutes/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 3,
					Window:   15 * time.Minute,
					KeyFunc:  middleware.IPKeyFunc,
				})).Post("/forgot-password", passwordResetHandler.ForgotPassword)
			} else {
				r.Post("/forgot-password", passwordResetHandler.ForgotPassword)
			}
			if cfg.RateLimitEnabled {
				// Reset password: 5 requests/15 minutes/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   15 * time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Post("/reset-password", passwordResetHandler.ResetPassword)
			} else {
				r.Post("/reset-password", passwordResetHandler.ResetPassword)
			}

			// Magic link authentication (public)
			if cfg.RateLimitEnabled {
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 3,
					Window:   15 * time.Minute,
					KeyFunc:  middleware.IPKeyFunc,
				})).Post("/magic-link/request", magicLinkHandler.RequestMagicLink)
			} else {
				r.Post("/magic-link/request", magicLinkHandler.RequestMagicLink)
			}
			if cfg.RateLimitEnabled {
				// Magic link verify: 5 requests/15 minutes/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   15 * time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Get("/magic-link/verify/{token}", magicLinkHandler.VerifyMagicLink)
			} else {
				r.Get("/magic-link/verify/{token}", magicLinkHandler.VerifyMagicLink)
			}
			r.Get("/magic-link/available", magicLinkHandler.CheckAvailable)

			// Email verification (public - no auth required)
			if cfg.RateLimitEnabled {
				// Verify email: 5 requests/15 minutes/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   15 * time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Get("/verify-email/{token}", emailVerificationHandler.VerifyEmail)
			} else {
				r.Get("/verify-email/{token}", emailVerificationHandler.VerifyEmail)
			}
		})

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtManager, cacheInstance))

			r.Get("/me", authHandler.GetMe)
			r.Patch("/me", authHandler.UpdateMe)
			r.Patch("/me/password", authHandler.ChangePassword)

			// Email verification (authenticated - requires login)
			r.Post("/send-verification", emailVerificationHandler.SendVerificationEmail)

			// Admin routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("admin"))

				r.Get("/admin/users", authHandler.ListUsers)
				r.Patch("/admin/users/{id}/role", authHandler.UpdateUserRole)
				r.Delete("/admin/users/{id}", authHandler.DeleteUser)
				r.Post("/admin/users/{id}/disable-2fa", adminMFAHandler.AdminDisable2FA)
			})
		})

		// Passkey authentication (public)
		r.Group(func(r chi.Router) {
			if cfg.RateLimitEnabled {
				// Passkey login (usernameless/passwordless): 5 requests/minute/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Post("/passkey/login/begin", passkeyHandler.BeginDiscoverableAuthentication)

				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Post("/passkey/login/finish", passkeyHandler.FinishAuthentication)
			} else {
				r.Post("/passkey/login/begin", passkeyHandler.BeginDiscoverableAuthentication)
				r.Post("/passkey/login/finish", passkeyHandler.FinishAuthentication)
			}
		})

		// MFA verification (public - during login)
		r.Group(func(r chi.Router) {
			if cfg.RateLimitEnabled {
				// MFA verification: 5 requests/5 minutes/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   5 * time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Post("/mfa/verify", mfaHandler.VerifyLogin)
			} else {
				r.Post("/mfa/verify", mfaHandler.VerifyLogin)
			}
		})
	})

	// ========== PASSKEY ROUTES (Authenticated) ==========
	r.Route("/api/v1/passkey", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager, cacheInstance))

		if cfg.RateLimitEnabled {
			// Passkey operations: 5 requests/minute/user
			r.With(rateLimiter.Limit(middleware.RateLimitConfig{
				Requests: 5,
				Window:   time.Minute,
				KeyFunc:  middleware.UserKeyFunc,
			})).Post("/register/begin", passkeyHandler.BeginRegistration)

			r.With(rateLimiter.Limit(middleware.RateLimitConfig{
				Requests: 5,
				Window:   time.Minute,
				KeyFunc:  middleware.UserKeyFunc,
			})).Post("/register/finish", passkeyHandler.FinishRegistration)
		} else {
			r.Post("/register/begin", passkeyHandler.BeginRegistration)
			r.Post("/register/finish", passkeyHandler.FinishRegistration)
		}

		r.Get("/list", passkeyHandler.List)
		r.Patch("/{id}/name", passkeyHandler.Rename)
		r.Delete("/{id}", passkeyHandler.Delete)
	})

	// ========== MFA ROUTES (Authenticated) ==========
	r.Route("/api/v1/mfa", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager, cacheInstance))

		if cfg.RateLimitEnabled {
			// MFA setup/disable: 5 requests/minute/user
			r.With(rateLimiter.Limit(middleware.RateLimitConfig{
				Requests: 5,
				Window:   time.Minute,
				KeyFunc:  middleware.UserKeyFunc,
			})).Post("/setup/begin", mfaHandler.BeginSetup)

			r.With(rateLimiter.Limit(middleware.RateLimitConfig{
				Requests: 5,
				Window:   time.Minute,
				KeyFunc:  middleware.UserKeyFunc,
			})).Post("/setup/finish", mfaHandler.FinishSetup)

			r.With(rateLimiter.Limit(middleware.RateLimitConfig{
				Requests: 3,
				Window:   time.Minute,
				KeyFunc:  middleware.UserKeyFunc,
			})).Post("/disable", mfaHandler.Disable)
		} else {
			r.Post("/setup/begin", mfaHandler.BeginSetup)
			r.Post("/setup/finish", mfaHandler.FinishSetup)
			r.Post("/disable", mfaHandler.Disable)
		}

		r.Get("/status", mfaHandler.GetStatus)
		r.Post("/backup-codes/regenerate", mfaHandler.RegenerateBackupCodes)
	})

	// ========== CALENDAR ROUTES ==========
	r.Route("/api/v1/calendars", func(r chi.Router) {
		// Public routes
		r.Group(func(r chi.Router) {
			if cfg.RateLimitEnabled {
				// Public calendar access: 60 requests/minute/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 60,
					Window:   time.Minute,
					KeyFunc:  middleware.IPKeyFunc,
				})).Get("/public/{token}", calendarHandler.GetPublicCalendar)

				// Anonymous participant registration: 10 requests/minute/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 10,
					Window:   time.Minute,
					KeyFunc:  middleware.IPKeyFunc,
				})).Post("/public/{token}/participants", participantHandler.AddAnonymousParticipant)
			} else {
				r.Get("/public/{token}", calendarHandler.GetPublicCalendar)
				r.Post("/public/{token}/participants", participantHandler.AddAnonymousParticipant)
			}

			// Public participant email verification
			if cfg.RateLimitEnabled {
				// Participant verify email: 5 requests/15 minutes/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 5,
					Window:   15 * time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				})).Get("/participants/verify-email/{token}", participantEmailHandler.VerifyEmail)
			} else {
				r.Get("/participants/verify-email/{token}", participantEmailHandler.VerifyEmail)
			}

			// Public participant email management (requires calendar token validation)
			r.Post("/{token}/participants/{pid}/email", participantEmailHandler.AddEmail)
			if cfg.RateLimitEnabled {
				// Resend verification: 3 requests/15 minutes/IP
				r.With(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 3,
					Window:   15 * time.Minute,
					KeyFunc:  middleware.IPKeyFunc,
				})).Post("/{token}/participants/{pid}/resend-verification", participantEmailHandler.ResendVerification)
			} else {
				r.Post("/{token}/participants/{pid}/resend-verification", participantEmailHandler.ResendVerification)
			}
		})

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtManager, cacheInstance))

			if cfg.RateLimitEnabled {
				// Authenticated routes: 100 requests/minute/user
				r.Use(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 100,
					Window:   time.Minute,
					KeyFunc:  middleware.UserKeyFunc,
				}))
			}

			// Calendar CRUD
			r.Post("/", calendarHandler.CreateCalendar)
			r.Get("/", calendarHandler.ListMyCalendars)
			r.Get("/{id}", calendarHandler.GetCalendar)
			r.Patch("/{id}", calendarHandler.UpdateCalendar)
			r.Delete("/{id}", calendarHandler.DeleteCalendar)

			// Token regeneration
			r.Post("/{id}/regenerate-token", calendarHandler.RegenerateToken)

			// Participant management
			r.Post("/{id}/participants", participantHandler.AddParticipant)
			r.Patch("/{id}/participants/{pid}", participantHandler.UpdateParticipant)
			r.Delete("/{id}/participants/{pid}", participantHandler.RemoveParticipant)

			// Notification config (owner only)
			r.Get("/{id}/notify-config", notifyConfigHandler.GetConfig)
			r.Patch("/{id}/notify-config", notifyConfigHandler.UpdateConfig)

			// Admin routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("admin"))

				r.Get("/admin/users/{id}/calendars", calendarHandler.ListUserCalendars)
			})
		})
	})

	// ========== AVAILABILITY ROUTES ==========
	r.Route("/api/v1/availabilities", func(r chi.Router) {
		// Live updates: the browser subscribes here and refetches on each notice.
		//
		// Kept out of the 60/min bucket below. An EventSource is a long-lived
		// connection that the browser re-establishes on its own after every hiccup,
		// deploy or idle timeout, so sharing a bucket with the participant API meant
		// a handful of reconnects could lock a participant out of the calendar they
		// were reconnecting to. Its own bucket, keyed on path+IP so one calendar's
		// reconnect loop cannot starve another, and sized for several tabs behind a
		// shared NAT rather than for API call volume.
		r.Group(func(r chi.Router) {
			if cfg.RateLimitEnabled {
				r.Use(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 300,
					Window:   time.Minute,
					KeyFunc:  middleware.CombinedKeyFunc,
				}))
			}

			r.Get("/calendar/{token}/events", eventsHandler.Stream)
		})

		// Public routes with rate limiting (all availability endpoints are public)
		r.Group(func(r chi.Router) {
			if cfg.RateLimitEnabled {
				// Rate limiting: 60 requests/minute/IP for public availability access
				r.Use(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 60,
					Window:   time.Minute,
					KeyFunc:  middleware.IPKeyFunc,
				}))
			}

			// One notice per successful write, for every route below. Placed here
			// rather than in the nine service methods so a tenth write route cannot
			// silently stop notifying anyone.
			r.Use(availabilityHandlers.PublishChanges(broker))

			// Participant availability management
			r.Get("/calendar/{token}/participant/{pid}", availabilityHandler.GetParticipantAvailabilities)
			r.Post("/calendar/{token}/participant/{pid}", availabilityHandler.CreateAvailability)
			r.Patch("/calendar/{token}/participant/{pid}/{date}", availabilityHandler.UpdateAvailability)
			r.Delete("/calendar/{token}/participant/{pid}/{date}", availabilityHandler.DeleteAvailability)

			// Recurrence management
			r.Post("/calendar/{token}/participant/{pid}/recurrence", recurrenceHandler.CreateRecurrence)
			r.Get("/calendar/{token}/participant/{pid}/recurrences", recurrenceHandler.GetParticipantRecurrences)
			r.Patch("/calendar/{token}/participant/{pid}/recurrence/{rid}", recurrenceHandler.UpdateRecurrence)
			r.Delete("/calendar/{token}/participant/{pid}/recurrence/{rid}", recurrenceHandler.DeleteRecurrence)

			// Recurrence exceptions
			r.Post("/calendar/{token}/participant/{pid}/recurrence/{rid}/exception", recurrenceHandler.CreateException)
			r.Delete("/calendar/{token}/participant/{pid}/recurrence/{rid}/exception/{date}", recurrenceHandler.DeleteException)

			// Date summaries
			r.Get("/calendar/{token}/dates/{date}", availabilityHandler.GetDateSummary)
			r.Get("/calendar/{token}/range", availabilityHandler.GetRangeSummary)
		})
	})

	// ========== QUOTA ROUTES ==========
	RegisterQuotaRoutes(r, services, jwtManager, cacheInstance)

	// ========== ICS ROUTES ==========
	r.Route("/api/v1/ics", func(r chi.Router) {
		// Public routes with rate limiting
		r.Group(func(r chi.Router) {
			if cfg.RateLimitEnabled {
				// ICS feed access: 30 requests/minute/IP
				r.Use(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 30,
					Window:   time.Minute,
					KeyFunc:  middleware.IPKeyFunc,
				}))
			}

			// ICS feed endpoint (accepts both /feed/{token} and /feed/{token}.ics)
			r.Get("/feed/{token}", icsHandler.GetFeed)
			// Unified ICS feed endpoint
			r.Get("/unified/{token}", icsHandler.GetUnifiedFeed)
		})

		// Authenticated routes for managing unified feed
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtManager, cacheInstance))
			if cfg.RateLimitEnabled {
				r.Use(rateLimiter.Limit(middleware.RateLimitConfig{
					Requests: 30,
					Window:   time.Minute,
					KeyFunc:  middleware.UserKeyFunc,
				}))
			}
			r.Get("/unified-feed", unifiedFeedConfigHandler.GetConfig)
			r.Post("/unified-feed", unifiedFeedConfigHandler.Create)
			r.Patch("/unified-feed/calendars", unifiedFeedConfigHandler.UpdateCalendars)
			r.Post("/unified-feed/regenerate-token", unifiedFeedConfigHandler.RegenerateToken)
		})
	})

	// ========== SEO ROUTES (robots.txt, sitemap.xml) ==========
	seoHandler := seo.NewHandler(cfg.AppURL, cfg.DisableRobots, buildType)
	r.Get("/robots.txt", seoHandler.HandleRobotsTxt)
	r.Get("/sitemap.xml", seoHandler.HandleSitemapXML)

	// ========== SWAGGER DOCUMENTATION ==========
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	// ========== FRONTEND (SPA) ==========
	// Serve embedded frontend for all non-API routes
	spaHandler, err := web.NewSPAHandler(cfg.AppURL, buildType)
	if err != nil {
		return fmt.Errorf("initialize SPA handler: %w", err)
	}
	log.Info("Frontend embedded and ready to serve")

	// An unmatched /api path is a mistake, not a deep link. Serving the SPA for it —
	// 200, and a page of HTML — makes a typo in a route look like success to any
	// client that checks the status and not the content type. It hid a broken seed
	// script, and it hid a health check in this repository's own CI pointing at
	// /api/v1/health, a route that has never existed.
	apiNotFound := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Endpoint not found")
	})

	// Two registrations are needed, because a miss arrives by one of two paths.
	// A path with no matching sub-router prefix (/api/v1/nope) falls through to the
	// "/*" catch-all below, so it has to be intercepted here first.
	r.Handle("/api/*", apiNotFound)

	// A path that *does* match a sub-router prefix but no route inside it
	// (/api/v1/calendars/xyz/summary) is answered by that sub-router's NotFound, which
	// otherwise defaults to chi's plain-text one. Setting it here, after every route is
	// registered, propagates it to the sub-routers that do not already have one — so
	// every /api miss now answers with the same JSON envelope. Non-API paths never
	// reach it: "/*" matches them first.
	r.NotFound(apiNotFound)

	// Serve frontend on all non-API routes (SPA fallback)
	r.Handle("/*", spaHandler)

	// The context every request descends from.
	//
	// Without a BaseContext, net/http gives each connection a background context
	// and nothing can tell an in-flight handler that the process is stopping.
	// That is invisible for a handler that returns in milliseconds and fatal for
	// the SSE streams, which never return on their own: each would sit out the
	// entire shutdown budget. Cancelling this is how they are told to leave.
	baseCtx, baseCancel := context.WithCancel(context.Background())
	defer baseCancel()

	// Create server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext:  func(net.Listener) context.Context { return baseCtx },
	}

	// A failed listen is reported, not exited on: os.Exit from this goroutine
	// would skip every deferred close in run(). The buffer keeps the goroutine
	// from leaking when the process is stopping for another reason.
	serverErr := make(chan error, 1)
	go func() {
		log.Info("WhenTo Application listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("http server: %w", err)
		}
	}()

	// Metrics listener, when enabled. Never a route on the router above: see
	// startMetricsServer.
	stopMetrics := startMetricsServer(cfg, log)
	defer stopMetrics()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case sig := <-quit:
		log.Info("Shutting down", "signal", sig.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownBudget)
	defer shutdownCancel()

	// Ordinary requests get streamDrainDelay to finish before the streams are
	// released; see the constant. Shutdown has already stopped accepting new
	// connections by the time this fires.
	go func() {
		select {
		case <-time.After(streamDrainDelay):
		case <-shutdownCtx.Done():
		}
		baseCancel()
	}()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
	}

	// Structured, like every other line: this process logs JSON, and a bare
	// Println is a line no log pipeline can parse.
	log.Info("Server exited")

	return nil
}

// startMetricsServer starts the Prometheus listener when the configuration asks
// for it, and returns the function that stops it.
//
// The exposition never goes on the application router. It names every route,
// counts every error, and reports pool saturation and Go build details: useful
// to an operator, and free reconnaissance to anyone else. On its own listener,
// exposure is whatever the operator publishes — a port mapping, a Kubernetes
// Service — instead of depending on middleware ordering on a router that also
// serves anonymous traffic. It is off unless METRICS_ENABLED says otherwise.
func startMetricsServer(cfg *config.Config, log *slog.Logger) func() {
	if !cfg.MetricsEnabled {
		return func() {}
	}

	port := cfg.MetricsPort
	if port == "" {
		// Refusing to fall back to the application listener is deliberate: the
		// one thing this endpoint must never be is public.
		port = defaultMetricsPort
		log.Warn("METRICS_PORT is unset; serving metrics on a listener of its own",
			"port", port,
			"path", metrics.MetricsPath,
		)
	}

	srv := metrics.NewServer(":" + port)
	go func() {
		log.Info("Metrics listening", "port", port, "path", metrics.MetricsPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// A metrics port that cannot be bound is an operational problem, not
			// a reason to stop serving calendars.
			log.Error("Metrics server error", "error", err)
		}
	}()

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownBudget)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("Metrics server forced to shutdown", "error", err)
		}
	}
}
