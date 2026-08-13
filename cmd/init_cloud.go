// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

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

const buildType = "cloud"

// InitServices initializes the build-specific services.
//
// Every hosted account gets the same calendar allowance, so this is only the quota
// service. It used to build the VAT, subscription, e-commerce and shop stacks, all of
// which existed to answer one question: how many calendars is this user allowed.
func InitServices(_ context.Context, _ *config.Config, pool *pgxpool.Pool) (*Services, error) {
	log := logger.Default()

	calendarRepository := calendarRepo.NewCalendarRepository(pool)
	quotaService := quota.NewCloudService(calendarRepository)

	log.Info("Quota service initialized (Cloud mode)", "calendar_limit", quota.CloudCalendarLimit)

	return &Services{QuotaService: quotaService}, nil
}

// RegisterQuotaRoutes mounts the quota endpoint.
//
// Kept as a build-specific function because it is half of a symmetric pair: the
// self-hosted variant must expose the same symbol, and CI compiles both.
func RegisterQuotaRoutes(r chi.Router, services *Services, jwtManager *jwt.Manager, cacheInstance cache.Cache) {
	log := logger.Default()
	quotaHandler := quota.NewHandler(services.QuotaService, log)

	r.Route("/api/v1/quota", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager, cacheInstance))
		r.Get("/limits", quotaHandler.HandleGetLimits)
	})
}
