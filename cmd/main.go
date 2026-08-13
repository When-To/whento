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

	"github.com/whento/pkg/broadcast"
	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/database"
	"github.com/whento/pkg/email"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/metrics"
	"github.com/whento/pkg/middleware"
	authHandlers "github.com/whento/whento/internal/auth/handlers"
	"github.com/whento/whento/internal/config"

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

// run brings the process up, serves, and blocks until a signal or a fatal
// error. It returns instead of exiting so that its defers — the pool, the Redis
// client, the broker, the rate limiter's sweeper — all get to run.
//
// It owns the things that have to be closed: everything below that outlives a
// request is created here, next to the defer that releases it. What is built
// *from* those resources is not here — buildHandlers (wire.go) does the
// dependency injection and newRouter (router.go) does the routing.
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

	// Readiness probes what the instance actually needs: PostgreSQL, which is
	// fatal, and Redis, which is not. The Redis probe stays nil when no client
	// was created — assigning a nil *RedisPinger to the interface would produce
	// a non-nil interface holding nil, and readiness would report a hard "down"
	// for a dependency the operator chose not to run.
	var cacheProbe authHandlers.Probe
	if redisPinger := database.NewRedisPinger(redisClient); redisPinger != nil {
		cacheProbe = redisPinger
	}

	d := &deps{
		cfg:        cfg,
		log:        log,
		pool:       pool,
		cacheStore: cacheInstance,
		jwtManager: jwtManager,
		mailer:     emailService,
		broker:     broker,
		limiter:    newRouteLimiter(rateLimiter, cfg.RateLimitEnabled),
		quota:      services,
		cacheProbe: cacheProbe,
	}

	h, err := buildHandlers(d)
	if err != nil {
		return err
	}

	// ========== FRONTEND (SPA) ==========
	// Serve embedded frontend for all non-API routes
	spaHandler, err := web.NewSPAHandler(cfg.AppURL, buildType)
	if err != nil {
		return fmt.Errorf("initialize SPA handler: %w", err)
	}
	log.Info("Frontend embedded and ready to serve")

	r := newRouter(d, h, spaHandler)

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
//
// It stays in this file on purpose: the "path" field it logs is the build-time
// exposition path, and pkg/logger's log-field guard lifts that field name for
// cmd/main.go by name. Moving this function moves it out from under the
// exception and fails that test.
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
