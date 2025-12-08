// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package main

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/quota"

	// Licensing module (Self-hosted only)
	licensingHandlers "github.com/whento/whento/internal/licensing/handlers"
	licensingRepo "github.com/whento/whento/internal/licensing/repository"
	licensingService "github.com/whento/whento/internal/licensing/service"

	// Calendar repo for quota checks
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
)

const buildType = "selfhosted"

// InitServices initializes self-hosted specific services (License management)
func InitServices(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool) (*Services, error) {
	log := logger.Default()

	// Initialize licensing repository
	licRepo := licensingRepo.New(pool)

	// Initialize licensing service with Ed25519 public key
	licService, err := licensingService.New(licRepo, licensingService.Config{}, log)
	if err != nil {
		return nil, err
	}

	log.Info("Licensing service initialized (Self-hosted mode)")

	// Load license from database into RAM first
	if err := licService.LoadLicenseFromDB(ctx); err != nil {
		log.Error("Failed to load license from database", "error", err)
		return nil, err
	}

	// Auto-activate license from environment ONLY if no license exists in DB
	// This allows users to deactivate licenses via UI without having them re-activated on reboot
	currentLicense := licService.GetActiveLicense()
	if cfg.License.Key != "" && currentLicense.Tier == "community" {
		if err := licService.ActivateLicense(ctx, cfg.License.Key); err != nil {
			log.Warn("Failed to activate license from environment", "error", err)
		} else {
			log.Info("License activated successfully from environment (no license in database)")
		}
	} else if cfg.License.Key != "" {
		log.Info("License already exists in database, skipping auto-activation from environment")
	}

	// Initialize calendar repository for quota checks
	calendarRepository := calendarRepo.NewCalendarRepository(pool)

	// Initialize quota service (selfhosted version - server-wide limits)
	quotaService := quota.NewSelfHostedService(licService, calendarRepository)

	// Get current license info
	activeLicense := licService.GetActiveLicense()
	log.Info("Active license loaded",
		"tier", activeLicense.Tier,
		"limit", activeLicense.CalendarLimit,
		"issued_to", activeLicense.IssuedTo,
	)

	log.Info("Quota service initialized (Self-hosted mode - server-wide limits)")

	return &Services{
		QuotaService:     quotaService,
		LicensingService: licService,
	}, nil
}

// RegisterBillingRoutes registers self-hosted specific licensing routes
func RegisterBillingRoutes(r chi.Router, services *Services, cfg *config.Config, pool *pgxpool.Pool, jwtManager interface{}) {
	log := logger.Default()

	// Reuse the licensing service from InitServices (already has license loaded in RAM)
	licService, ok := services.LicensingService.(*licensingService.Service)
	if !ok || licService == nil {
		log.Error("Licensing service not initialized")
		return
	}

	// Initialize licensing handlers (with quota service for usage calculation)
	licHandler := licensingHandlers.New(licService, services.QuotaService, log)

	// Initialize quota handlers
	quotaHandler := quota.NewHandler(services.QuotaService, log)

	log.Info("Registering Self-hosted licensing routes")

	// License management routes
	r.Route("/api/v1/license", func(r chi.Router) {
		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtManager.(*jwt.Manager)))

			// Public info (any authenticated user can view)
			r.Get("/info", licHandler.HandleGetLicenseInfo)

			// Admin only routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("admin"))

				r.Post("/activate", licHandler.HandleActivateLicense)
				r.Post("/reload", licHandler.HandleReloadLicense)
				r.Delete("/", licHandler.HandleRemoveLicense)
			})
		})
	})

	// Quota routes (authenticated users)
	r.Route("/api/v1/quota", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager.(*jwt.Manager)))
		r.Get("/limits", quotaHandler.HandleGetLimits)
	})

	log.Info("Self-hosted licensing routes registered successfully")
}

// StartVATRefreshTask is a no-op in self-hosted mode (VAT management is cloud-only)
func StartVATRefreshTask(ctx context.Context, services *Services) {
	// No-op: VAT refresh only runs in cloud mode
}
