// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//	@title						WhenTo API
//	@version					1.0.0
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
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/database"
	"github.com/whento/pkg/email"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
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

	// Frontend embedding
	"github.com/whento/whento/web"

	// Swagger docs (generated)
	_ "github.com/whento/whento/docs/swagger"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	log := logger.New(cfg.LogLevel, "json")
	logger.SetDefault(log)

	log.Info("Starting WhenTo Application", "port", cfg.Port, "env", cfg.AppEnv)

	// Context for initialization
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to PostgreSQL (shared by all modules)
	dbConfig := &database.Config{
		URL: cfg.DatabaseURL,
	}
	pool, err := database.NewPool(ctx, dbConfig)
	if err != nil {
		log.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close(pool)
	log.Info("Connected to PostgreSQL")

	// Connect to Redis (shared by all modules) - Optional
	redisConfig := &database.RedisConfig{
		URL: cfg.RedisURL,
	}
	redisClient, err := database.NewRedisClient(ctx, redisConfig)
	if err != nil {
		log.Warn("Failed to connect to Redis - running without cache and rate limiting", "error", err)
		redisClient = nil
	} else {
		defer database.CloseRedis(redisClient)
		log.Info("Connected to Redis - cache and rate limiting enabled")
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
		log.Error("Failed to initialize JWT manager", "error", err)
		os.Exit(1)
	}
	log.Info("JWT manager initialized")

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

	// ========== LICENSING/SUBSCRIPTION MODULE ==========
	// Initialize build-specific services (Cloud: Stripe subscriptions, Self-hosted: License management)
	services, err := InitServices(ctx, cfg, pool)
	if err != nil {
		log.Error("Failed to initialize licensing/subscription services", "error", err)
		os.Exit(1)
	}

	// Start VAT refresh background task (Cloud only - no-op in self-hosted)
	StartVATRefreshTask(context.Background(), services)

	// ========== AUTH MODULE ==========
	// Initialize auth repositories
	userRepo := authRepo.NewUserRepository(pool)
	tokenRepo := authRepo.NewTokenRepository(pool)
	mfaRepository := mfaRepo.NewMFARepository(pool)

	// Initialize auth service (with MFA repository for 2FA checking)
	authSvc := authService.NewAuthService(userRepo, tokenRepo, mfaRepository, jwtManager, cfg.BcryptCost, cfg.AllowedRegister, cfg.AllowedEmails)

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
		log.Error("Failed to initialize passkey service", "error", err)
		os.Exit(1)
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
	authHealthHandler := authHandlers.NewHealthHandler()

	// ========== MFA MODULE ==========
	// Initialize MFA service (repository already created for auth service)
	mfaSvc := mfaService.NewMFAService(mfaRepository, userRepo, cfg, log)
	log.Info("MFA service initialized")

	// Initialize MFA handler (with auth service for completing login)
	mfaHandler := mfaHandlers.NewMFAHandler(mfaSvc, authSvc, jwtManager, log)

	// Initialize admin MFA handler for admin operations (disable 2FA)
	adminMFAHandler := authHandlers.NewAdminMFAHandler(mfaSvc, log)

	// ========== CALENDAR MODULE ==========
	// Initialize calendar repositories
	calendarRepository := calendarRepo.NewCalendarRepository(pool)
	participantRepository := calendarRepo.NewParticipantRepository(pool)

	// Initialize calendar service with cache
	calendarSvc := calendarService.NewCalendarService(calendarRepository, participantRepository, cacheInstance)

	// Initialize calendar handlers (with quota service for limit checking)
	calendarHandler := calendarHandlers.NewCalendarHandler(calendarSvc, services.QuotaService, userRepo, cfg)
	participantHandler := calendarHandlers.NewParticipantHandler(calendarSvc)

	// ========== AVAILABILITY MODULE ==========
	// Initialize availability repositories
	availabilityRepository := availabilityRepo.NewAvailabilityRepository(pool)
	availCalendarRepo := availabilityRepo.NewCalendarRepository(pool)
	availParticipantRepo := availabilityRepo.NewParticipantRepository(pool)
	recurrenceRepository := availabilityRepo.NewRecurrenceRepository(pool)

	// Initialize availability service with cache
	availabilitySvc := availabilityService.NewAvailabilityService(availabilityRepository, availCalendarRepo, availParticipantRepo, recurrenceRepository, cacheInstance)

	// Initialize availability handlers
	availabilityHandler := availabilityHandlers.NewAvailabilityHandler(availabilitySvc)
	recurrenceHandler := availabilityHandlers.NewRecurrenceHandler(availabilitySvc)

	// ========== ICS MODULE ==========
	// Initialize ICS repositories
	icsCalendarRepo := icsRepo.NewCalendarRepository(pool)
	icsAvailabilityRepo := icsRepo.NewAvailabilityRepository(pool)

	// Initialize ICS service (with quota checker to block feeds for over-quota users)
	icsSvc := icsService.NewICSService(icsCalendarRepo, icsAvailabilityRepo, services.QuotaService, cfg.AppURL)

	// Initialize ICS handlers
	icsHandler := icsHandlers.NewICSHandler(icsSvc)

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter(redisClient)

	// Setup router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.LimitRequestSize(1 * 1024 * 1024)) // 1MB max payload
	r.Use(middleware.CORS([]string{"*"}))               // Configure for production

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

			r.Post("/refresh", authHandler.Refresh)
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
			r.Post("/reset-password", passwordResetHandler.ResetPassword)

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
			r.Get("/magic-link/verify/{token}", magicLinkHandler.VerifyMagicLink)
			r.Get("/magic-link/available", magicLinkHandler.CheckAvailable)

			// Email verification (public - no auth required)
			r.Get("/verify-email/{token}", emailVerificationHandler.VerifyEmail)
		})

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtManager))

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
		r.Use(middleware.Auth(jwtManager))

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
		r.Use(middleware.Auth(jwtManager))

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
			} else {
				r.Get("/public/{token}", calendarHandler.GetPublicCalendar)
			}
		})

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtManager))

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

			// Admin routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("admin"))

				r.Get("/admin/users/{id}/calendars", calendarHandler.ListUserCalendars)
			})
		})
	})

	// ========== AVAILABILITY ROUTES ==========
	r.Route("/api/v1/availabilities", func(r chi.Router) {
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

	// ========== BILLING/LICENSING ROUTES ==========
	// Register build-specific routes (Cloud: Stripe billing, Self-hosted: License management)
	RegisterBillingRoutes(r, services, cfg, pool, jwtManager)

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
		log.Error("Failed to initialize SPA handler", "error", err)
		os.Exit(1)
	}
	log.Info("Frontend embedded and ready to serve")

	// Serve frontend on all non-API routes (SPA fallback)
	r.Handle("/*", spaHandler)

	// Create server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		log.Info("WhenTo Application listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
	}

	fmt.Println("Server exited")
}
