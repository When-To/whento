// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package pricing

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/stripe/stripe-go/v84/webhook"

	"github.com/whento/pkg/httputil"
)

// PriceRefresher is an interface for services that can refresh their prices from Stripe
type PriceRefresher interface {
	RefreshProducts() error
}

// PlanRefresher is an interface for services that can refresh their plan configs from Stripe
type PlanRefresher interface {
	RefreshPlanConfigs() error
}

// PlanConfigProvider is an interface for services that can provide plan configurations
// Returns a map[string]PlanConfigAPI where PlanConfigAPI has Name, CalendarLimit, PriceYearly, Features
type PlanConfigProvider interface {
	GetAllPlanConfigsForAPI() map[string]interface{}
}

// Handler handles pricing-related HTTP requests (public endpoints)
type Handler struct {
	subscriptionService PlanConfigProvider
	log                 *slog.Logger
}

// NewHandler creates a new pricing handler
func NewHandler(subscriptionService PlanConfigProvider, log *slog.Logger) *Handler {
	return &Handler{
		subscriptionService: subscriptionService,
		log:                 log,
	}
}

// HandleGetPlans returns all subscription plan configurations
//
//	@Summary		Get subscription plans (Cloud only)
//	@Description	Returns all available subscription plans with prices and features. Cloud-specific endpoint.
//	@Tags			Pricing
//	@Produce		json
//	@Success		200	{object}	object{plans=map[string]interface{}}	"Plan configurations"
//	@Router			/api/v1/pricing/plans [get]
func (h *Handler) HandleGetPlans(w http.ResponseWriter, r *http.Request) {
	plans := h.subscriptionService.GetAllPlanConfigsForAPI()
	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"plans": plans,
	})
}

// WebhookHandler handles Stripe webhook events for price and product updates
type WebhookHandler struct {
	shopService         PriceRefresher
	subscriptionService PlanRefresher
	webhookSecret       string
	log                 *slog.Logger
}

// NewWebhookHandler creates a new price webhook handler
func NewWebhookHandler(
	shopService PriceRefresher,
	subscriptionService PlanRefresher,
	webhookSecret string,
	log *slog.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		shopService:         shopService,
		subscriptionService: subscriptionService,
		webhookSecret:       webhookSecret,
		log:                 log,
	}
}

// HandleWebhook processes Stripe webhook events for price/product updates
//
//	@Summary		Stripe price webhook (Cloud only)
//	@Description	Handles Stripe webhook events for price.updated and product.updated. Refreshes cached prices. Cloud-specific endpoint.
//	@Tags			Pricing
//	@Accept			json
//	@Produce		json
//	@Param			Stripe-Signature	header		string	true	"Stripe webhook signature"
//	@Success		200					{object}	object{received=bool}		"Webhook acknowledged"
//	@Failure		400					{object}	httputil.ErrorResponse		"Failed to read request body"
//	@Failure		401					{object}	httputil.ErrorResponse		"Invalid signature"
//	@Router			/api/v1/pricing/webhook [post]
func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error("Failed to read webhook body", "error", err)
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Failed to read request body")
		return
	}

	// Verify webhook signature
	event, err := webhook.ConstructEventWithOptions(
		body,
		r.Header.Get("Stripe-Signature"),
		h.webhookSecret,
		webhook.ConstructEventOptions{
			IgnoreAPIVersionMismatch: true,
		},
	)
	if err != nil {
		h.log.Error("Failed to verify webhook signature", "error", err)
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid signature")
		return
	}

	h.log.Info("Received Stripe price webhook", "type", event.Type, "id", event.ID)

	// Handle price and product update events
	// Note: We only handle "updated" events because:
	// - "created" is useless: new products/prices must be configured via env vars first
	// - "deleted" is useless: we fall back to defaults if Stripe fetch fails
	switch event.Type {
	case "price.updated":
		h.refreshAllPrices("price updated")
	case "product.updated":
		h.refreshAllPrices("product metadata updated")
	default:
		h.log.Info("Ignoring webhook event type (only price.updated and product.updated are handled)", "type", event.Type)
	}

	// Always return 200 to acknowledge receipt
	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"received": true,
	})
}

// refreshAllPrices refreshes prices in both shop and subscription services
func (h *WebhookHandler) refreshAllPrices(reason string) {
	h.log.Info("Refreshing all prices from Stripe", "reason", reason)
	// Refresh shop products
	if h.shopService != nil {
		if err := h.shopService.RefreshProducts(); err != nil {
			h.log.Error("Failed to refresh shop products", "error", err)
		} else {
			h.log.Info("Shop products refreshed successfully")
		}
	}

	// Refresh subscription plan configs
	if h.subscriptionService != nil {
		if err := h.subscriptionService.RefreshPlanConfigs(); err != nil {
			h.log.Error("Failed to refresh subscription plan configs", "error", err)
		} else {
			h.log.Info("Subscription plan configs refreshed successfully")
		}
	}
}
