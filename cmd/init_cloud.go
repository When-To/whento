// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/quota"

	// Subscription module (Cloud only)
	subscriptionHandlers "github.com/whento/whento/internal/subscription/handlers"
	subscriptionRepo "github.com/whento/whento/internal/subscription/repository"
	subscriptionService "github.com/whento/whento/internal/subscription/service"

	// E-commerce module (Cloud only - license sales management)
	ecommerceHandlers "github.com/whento/whento/internal/ecommerce/handlers"
	ecommerceRepo "github.com/whento/whento/internal/ecommerce/repository"
	ecommerceService "github.com/whento/whento/internal/ecommerce/service"

	// VAT module (Cloud only - VAT rate management)
	vatHandlers "github.com/whento/whento/internal/vat/handlers"
	vatRepo "github.com/whento/whento/internal/vat/repository"
	vatService "github.com/whento/whento/internal/vat/service"

	// Shop module (Cloud only - license sales and cart)
	shopEmail "github.com/whento/whento/internal/shop/email"
	shopHandlers "github.com/whento/whento/internal/shop/handlers"
	shopRepo "github.com/whento/whento/internal/shop/repository"
	shopService "github.com/whento/whento/internal/shop/service"

	// Pricing module (Cloud only - price webhook handler)
	"github.com/whento/whento/internal/pricing"

	// Calendar repo for quota checks
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
)

const buildType = "cloud"

// InitServices initializes cloud-specific services (Stripe subscriptions)
func InitServices(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool) (*Services, error) {
	log := logger.Default()

	// Initialize VAT repository and service first (needed by subscription service)
	vatRepository := vatRepo.New(pool)
	vatSvc := vatService.New(vatRepository, log)

	log.Info("VAT service initialized (Cloud mode - EU VAT management)")

	// Initialize subscription repository
	subRepo := subscriptionRepo.New(pool)

	// Initialize subscription service with Stripe and VAT service
	subService := subscriptionService.New(subRepo, vatSvc, subscriptionService.Config{
		StripeSecretKey:  cfg.Stripe.SecretKey,
		StripePricePro:   cfg.Stripe.PricePro,
		StripePricePower: cfg.Stripe.PricePower,
	}, log)

	log.Info("Subscription service initialized (Cloud mode)")

	// Initialize calendar repository for quota checks
	calendarRepository := calendarRepo.NewCalendarRepository(pool)

	// Initialize quota service (cloud version - per-user limits)
	quotaService := quota.NewCloudService(subService, calendarRepository)

	log.Info("Quota service initialized (Cloud mode - per-user limits)")

	// Initialize e-commerce repository and service
	ecommRepo := ecommerceRepo.New(pool)
	ecommService := ecommerceService.New(ecommRepo, log)

	log.Info("E-commerce service initialized (Cloud mode - license sales management)")

	// Initialize shop repository and service
	shopRepository := shopRepo.New(pool)
	shopSvc, err := shopService.New(
		shopRepository,
		vatSvc,
		ecommService,
		shopService.Config{
			StripePriceProLicense:        cfg.Shop.StripePriceProLicense,
			StripePriceEnterpriseLicense: cfg.Shop.StripePriceEnterpriseLicense,
			LicensePrivateKeyBase64:      cfg.Shop.LicensePrivateKeyBase64,
			AppURL:                       cfg.AppURL,
		},
		log,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize shop service: %w", err)
	}

	log.Info("Shop service initialized (Cloud mode - license sales and cart)")

	return &Services{
		QuotaService:     quotaService,
		EcommerceService: ecommService,
		VATService:       vatSvc,
		ShopService:      shopSvc,
	}, nil
}

// RegisterBillingRoutes registers cloud-specific billing routes (Stripe)
func RegisterBillingRoutes(r chi.Router, services *Services, cfg *config.Config, pool *pgxpool.Pool, jwtManager interface{}) {
	log := logger.Default()

	// Re-initialize subscription service for handlers
	subRepo := subscriptionRepo.New(pool)
	subService := subscriptionService.New(subRepo, services.VATService.(*vatService.Service), subscriptionService.Config{
		StripeSecretKey:  cfg.Stripe.SecretKey,
		StripePricePro:   cfg.Stripe.PricePro,
		StripePricePower: cfg.Stripe.PricePower,
	}, log)

	// Initialize subscription handlers
	subHandler := subscriptionHandlers.New(subService, cfg.Stripe.WebhookSubscriptionSecret, log)

	// Initialize quota handlers
	quotaHandler := quota.NewHandler(services.QuotaService, log)

	log.Info("Registering Cloud billing routes (Stripe)")

	// Billing routes
	r.Route("/api/v1/billing", func(r chi.Router) {
		// Webhook (no auth required - verified by Stripe signature)
		r.Post("/webhook", subHandler.HandleStripeWebhook)

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtManager.(*jwt.Manager)))

			r.Post("/checkout", subHandler.HandleCreateCheckout)
			r.Post("/portal", subHandler.HandleCreatePortal)
			r.Get("/subscription", subHandler.HandleGetSubscription)
		})

		// Admin-only routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtManager.(*jwt.Manager)))
			r.Use(middleware.RequireRole("admin"))

			r.Get("/accounting", subHandler.HandleGetAccounting)
		})
	})

	// Quota routes (authenticated)
	r.Route("/api/v1/quota", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager.(*jwt.Manager)))
		r.Get("/limits", quotaHandler.HandleGetLimits)
	})

	// E-commerce admin routes (admin only - license sales management)
	ecommRepo := ecommerceRepo.New(pool)
	ecommService := ecommerceService.New(ecommRepo, log)
	ecommHandler := ecommerceHandlers.New(ecommService, log)

	r.Route("/api/v1/admin/ecommerce", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager.(*jwt.Manager)))
		r.Use(middleware.RequireRole("admin"))

		// License search by support key
		r.Get("/licenses/search", ecommHandler.HandleSearchLicense)
		r.Get("/licenses/{id}", ecommHandler.HandleGetLicense)

		// Clients management
		r.Get("/clients", ecommHandler.HandleListClients)
		r.Get("/clients/{id}", ecommHandler.HandleGetClient)

		// Orders management
		r.Get("/orders", ecommHandler.HandleListOrders)
		r.Get("/orders/{id}", ecommHandler.HandleGetOrder)
	})

	log.Info("Cloud billing routes registered successfully")
	log.Info("E-commerce admin routes registered successfully")

	// VAT routes
	vatRepository := vatRepo.New(pool)
	vatSvc := vatService.New(vatRepository, log)
	vatHandler := vatHandlers.New(vatSvc, log)

	// VAT admin routes (admin only)
	r.Route("/api/v1/admin/vat", func(r chi.Router) {
		r.Use(middleware.Auth(jwtManager.(*jwt.Manager)))
		r.Use(middleware.RequireRole("admin"))

		r.Get("/rates", vatHandler.HandleGetRates)
		r.Post("/refresh", vatHandler.HandleRefreshRates)
		r.Get("/report", vatHandler.HandleGetReport)
	})

	// VAT public route for checkout (no auth required)
	r.Route("/api/v1/shop/vat", func(r chi.Router) {
		r.Post("/calculate", vatHandler.HandleCalculateVAT)
	})

	log.Info("VAT routes registered successfully")

	// Shop routes
	shopRepository := shopRepo.New(pool)
	shopSvc, err := shopService.New(
		shopRepository,
		vatSvc,
		ecommService,
		shopService.Config{
			StripePriceProLicense:        cfg.Shop.StripePriceProLicense,
			StripePriceEnterpriseLicense: cfg.Shop.StripePriceEnterpriseLicense,
			LicensePrivateKeyBase64:      cfg.Shop.LicensePrivateKeyBase64,
			AppURL:                       cfg.AppURL,
		},
		log,
	)
	if err != nil {
		log.Error("Failed to initialize shop service for routes", "error", err)
		return
	}
	shopHandler := shopHandlers.New(shopSvc, ecommService, log)

	// Shop email sender
	emailSender := shopEmail.New(
		shopEmail.Config{
			SMTPHost:     cfg.Email.SMTPHost,
			SMTPPort:     fmt.Sprintf("%d", cfg.Email.SMTPPort),
			SMTPUsername: cfg.Email.SMTPUsername,
			SMTPPassword: cfg.Email.SMTPPassword,
			FromEmail:    cfg.Email.FromAddress,
			FromName:     cfg.Email.FromName,
			AppURL:       cfg.AppURL,
		},
		log,
	)

	// Shop webhook handler
	webhookHandler := shopHandlers.NewWebhookHandler(
		shopSvc,
		ecommService,
		emailSender,
		cfg.Shop.StripeWebhookLicenceSecret,
		log,
	)

	// Shop routes (public - no auth required for guest checkout)
	r.Route("/api/v1/shop", func(r chi.Router) {
		// Public shopping endpoints
		r.Get("/products", shopHandler.HandleGetProducts)
		r.Get("/cart", shopHandler.HandleGetCart)
		r.Post("/cart/items", shopHandler.HandleAddToCart)
		r.Patch("/cart/items/{tier}", shopHandler.HandleUpdateQuantity)
		r.Delete("/cart/items/{tier}", shopHandler.HandleRemoveItem)
		r.Delete("/cart", shopHandler.HandleClearCart)
		r.Post("/checkout", shopHandler.HandleCheckout)
		r.Post("/validate-vat", shopHandler.HandleValidateVAT)

		// Order retrieval (public with order ID or session ID)
		r.Get("/orders/by-session/{session_id}", shopHandler.HandleGetOrderBySession)
		r.Get("/orders/{order_id}", shopHandler.HandleGetOrder)
		r.Get("/orders/{order_id}/download", shopHandler.HandleDownloadLicenses)
		r.Get("/orders/{order_id}/licenses/{license_id}/download", shopHandler.HandleDownloadSingleLicense)

		// Webhook (verified by Stripe signature)
		r.Post("/webhook", webhookHandler.HandleWebhook)
	})

	log.Info("Shop routes registered successfully")

	// Pricing routes (public plans endpoint and webhook)
	pricingHandler := pricing.NewHandler(subService, log)

	r.Route("/api/v1/pricing", func(r chi.Router) {
		// Public endpoint - returns plan configs with prices from Stripe
		r.Get("/plans", pricingHandler.HandleGetPlans)

		// Webhook (verified by Stripe signature) - only if secret configured
		if cfg.Shop.StripeWebhookPriceSecret != "" {
			pricingWebhookHandler := pricing.NewWebhookHandler(
				shopSvc,
				subService,
				cfg.Shop.StripeWebhookPriceSecret,
				log,
			)
			r.Post("/webhook", pricingWebhookHandler.HandleWebhook)
		}
	})

	if cfg.Shop.StripeWebhookPriceSecret != "" {
		log.Info("Pricing routes registered successfully (plans + webhook)")
	} else {
		log.Info("Pricing routes registered successfully (plans only - webhook disabled)")
		log.Warn("STRIPE_WEBHOOK_PRICE_SECRET not configured, pricing webhook disabled")
	}
}

// StartVATRefreshTask starts a background task that refreshes VAT rates daily (Cloud only)
func StartVATRefreshTask(ctx context.Context, services *Services) {
	log := logger.Default()

	// Type assert to get the actual VAT service
	vatSvc, ok := services.VATService.(*vatService.Service)
	if !ok || vatSvc == nil {
		log.Warn("VAT service not available, skipping VAT refresh task")
		return
	}

	log.Info("Starting VAT refresh background task (daily at 3 AM)")

	// Start goroutine for periodic refresh
	go func() {
		// Initial refresh on startup
		log.Info("Performing initial VAT rates refresh")
		if err := vatSvc.RefreshRates(ctx); err != nil {
			log.Error("Failed to refresh VAT rates on startup", "error", err)
		} else {
			log.Info("Initial VAT rates refresh completed successfully")
		}

		// Calculate time until next 3 AM
		now := time.Now()
		next3AM := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if next3AM.Before(now) {
			next3AM = next3AM.Add(24 * time.Hour)
		}
		timeUntilNext := time.Until(next3AM)

		log.Info("Next VAT refresh scheduled", "time", next3AM.Format(time.RFC3339), "in", timeUntilNext)

		// Wait until 3 AM
		timer := time.NewTimer(timeUntilNext)
		defer timer.Stop()

		<-timer.C

		// Create daily ticker starting from 3 AM
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Info("VAT refresh task stopped (context cancelled)")
				return
			case <-ticker.C:
				log.Info("Running scheduled VAT rates refresh")
				refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				if err := vatSvc.RefreshRates(refreshCtx); err != nil {
					log.Error("Failed to refresh VAT rates", "error", err)
				} else {
					log.Info("Scheduled VAT rates refresh completed successfully")
				}
				cancel()
			}
		}
	}()
}
