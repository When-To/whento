// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/quota"
)

// Services holds the services whose construction differs between build variants.
// InitServices is implemented per variant in init_cloud.go and init_selfhosted.go.
type Services struct {
	QuotaService quota.QuotaService
}

// RegisterQuotaRoutes mounts the quota endpoint. The route is the same in both
// variants; only the QuotaService behind it differs.
func RegisterQuotaRoutes(r chi.Router, services *Services, jwtManager *jwt.Manager, cacheInstance cache.Cache) {
	quotaHandler := quota.NewHandler(services.QuotaService, logger.Default())

	r.Route("/api/v1/quota", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager, cacheInstance))
		r.Get("/limits", quotaHandler.HandleGetLimits)
	})
}
