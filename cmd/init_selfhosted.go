// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package main

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/middleware"
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/quota"
)

const buildType = "selfhosted"

// InitServices initializes the build-specific services.
//
// Self-hosting is unrestricted, so this is only the quota service — which reports no
// limit. It used to load and verify a signed licence file first.
func InitServices(_ context.Context, _ *config.Config, pool *pgxpool.Pool) (*Services, error) {
	log := logger.Default()

	calendarRepository := calendarRepo.NewCalendarRepository(pool)
	quotaService := quota.NewSelfHostedService(calendarRepository)

	log.Info("Quota service initialized (Self-hosted mode - unlimited calendars)")

	return &Services{QuotaService: quotaService}, nil
}

// RegisterQuotaRoutes mounts the quota endpoint.
//
// Kept as a build-specific function because it is half of a symmetric pair: the cloud
// variant must expose the same symbol, and CI compiles both.
func RegisterQuotaRoutes(r chi.Router, services *Services, jwtManager *jwt.Manager, cacheInstance cache.Cache) {
	log := logger.Default()
	quotaHandler := quota.NewHandler(services.QuotaService, log)

	r.Route("/api/v1/quota", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager, cacheInstance))
		r.Get("/limits", quotaHandler.HandleGetLimits)
	})
}
