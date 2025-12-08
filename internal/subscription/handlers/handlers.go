// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/subscription/models"
	"github.com/whento/whento/internal/subscription/service"
)

// Handler handles subscription-related HTTP requests
type Handler struct {
	service       *service.Service
	webhookSecret string
	log           *slog.Logger
}

// New creates a new subscription handler
func New(service *service.Service, webhookSecret string, log *slog.Logger) *Handler {
	return &Handler{
		service:       service,
		webhookSecret: webhookSecret,
		log:           log,
	}
}

// HandleCreateCheckout creates a Stripe checkout session for upgrading
// @Summary Create checkout session (Cloud only)
// @Description Creates a Stripe checkout session for upgrading to Pro or Power plan. Cloud-specific endpoint.
// @Tags Billing
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object{plan=string,success_url=string,cancel_url=string,name=string,email=string,company=string,vat_number=string,address=string,postal_code=string,country=string} true "Checkout request with plan and billing details"
// @Success 200 {object} object{checkout_url=string,session_id=string} "Checkout session created successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid request body or validation error"
// @Failure 401 {object} httputil.ErrorResponse "User not authenticated"
// @Failure 500 {object} httputil.ErrorResponse "Failed to create checkout session"
// @Router /api/v1/billing/checkout [post]
func (h *Handler) HandleCreateCheckout(w http.ResponseWriter, r *http.Request) {
	var req models.CreateCheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	// Get user ID from context (set by auth middleware)
	userIDStr := middleware.GetUserID(r.Context())
	if userIDStr == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "User not authenticated")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid user ID")
		return
	}

	// Create checkout session
	resp, err := h.service.CreateCheckoutSession(r.Context(), userID, req)
	if err != nil {
		h.log.Error("Failed to create checkout session", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to create checkout session")
		return
	}

	httputil.JSON(w, http.StatusOK, resp)
}

// HandleCreatePortal creates a Stripe customer portal session
// @Summary Create customer portal session (Cloud only)
// @Description Creates a Stripe customer portal session for managing subscription and billing. Cloud-specific endpoint.
// @Tags Billing
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object{return_url=string} true "Portal request with return URL"
// @Success 200 {object} object{portal_url=string} "Portal session created successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid request body or validation error"
// @Failure 401 {object} httputil.ErrorResponse "User not authenticated"
// @Failure 500 {object} httputil.ErrorResponse "Failed to create portal session"
// @Router /api/v1/billing/portal [post]
func (h *Handler) HandleCreatePortal(w http.ResponseWriter, r *http.Request) {
	var req models.CreatePortalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	// Get user ID from context
	userIDStr := middleware.GetUserID(r.Context())
	if userIDStr == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "User not authenticated")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid user ID")
		return
	}

	// Create portal session
	resp, err := h.service.CreatePortalSession(r.Context(), userID, req)
	if err != nil {
		h.log.Error("Failed to create portal session", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to create portal session")
		return
	}

	httputil.JSON(w, http.StatusOK, resp)
}

// HandleGetSubscription returns the current user's subscription
// @Summary Get current subscription (Cloud only)
// @Description Returns the current user's subscription details including plan, limits, and usage. Cloud-specific endpoint.
// @Tags Billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{subscription=object,plan_config=object} "Subscription retrieved successfully"
// @Failure 401 {object} httputil.ErrorResponse "User not authenticated"
// @Failure 500 {object} httputil.ErrorResponse "Failed to get subscription"
// @Router /api/v1/billing/subscription [get]
func (h *Handler) HandleGetSubscription(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userIDStr := middleware.GetUserID(r.Context())
	if userIDStr == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "User not authenticated")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid user ID")
		return
	}

	// Get subscription
	sub, err := h.service.GetUserSubscription(r.Context(), userID)
	if err != nil {
		h.log.Error("Failed to get subscription", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get subscription")
		return
	}

	// Get plan config (fetched from Stripe)
	planConfig := h.service.GetPlanConfig(sub.Plan)

	resp := models.SubscriptionResponse{
		Subscription: sub,
		PlanConfig:   planConfig,
	}

	httputil.JSON(w, http.StatusOK, resp)
}

// HandleStripeWebhook handles Stripe webhook events
// @Summary Stripe webhook handler (Cloud only)
// @Description Handles Stripe webhook events for subscription management (checkout.session.completed, customer.subscription.updated, customer.subscription.deleted). Verified by Stripe signature. Cloud-specific endpoint.
// @Tags Billing
// @Accept json
// @Produce json
// @Param Stripe-Signature header string true "Stripe webhook signature"
// @Success 200 {string} string "Webhook processed successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid signature or payload"
// @Failure 500 {object} httputil.ErrorResponse "Failed to process webhook event"
// @Router /api/v1/billing/webhook [post]
func (h *Handler) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	// Read request body
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error("Failed to read webhook payload", "error", err)
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Failed to read request body")
		return
	}

	// Verify webhook signature
	event, err := webhook.ConstructEventWithOptions(
		payload,
		r.Header.Get("Stripe-Signature"),
		h.webhookSecret,
		webhook.ConstructEventOptions{
			IgnoreAPIVersionMismatch: true,
		},
	)
	if err != nil {
		h.log.Error("Failed to verify webhook signature", "error", err)
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid signature")
		return
	}

	h.log.Info("Received Stripe webhook", "type", event.Type, "id", event.ID)

	// Handle different event types
	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			h.log.Error("Failed to parse checkout session", "error", err)
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Failed to parse event data")
			return
		}

		// IMPORTANT: Only process subscription checkouts
		// Ignore one-time payment checkouts (handled by shop webhook)
		if session.Mode != stripe.CheckoutSessionModeSubscription {
			h.log.Info("Ignoring non-subscription checkout session", "session_id", session.ID, "mode", session.Mode)
			w.WriteHeader(http.StatusOK)
			return
		}

		if err := h.service.HandleCheckoutComplete(r.Context(), &session); err != nil {
			h.log.Error("Failed to handle checkout completion", "error", err, "session_id", session.ID)
			httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to process checkout")
			return
		}

	case "customer.subscription.updated":
		var subscription stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
			h.log.Error("Failed to parse subscription", "error", err)
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Failed to parse event data")
			return
		}

		if err := h.service.HandleSubscriptionUpdated(r.Context(), &subscription); err != nil {
			h.log.Error("Failed to handle subscription update", "error", err, "subscription_id", subscription.ID)
			httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to process subscription update")
			return
		}

	case "customer.subscription.deleted":
		var subscription stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
			h.log.Error("Failed to parse subscription", "error", err)
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Failed to parse event data")
			return
		}

		if err := h.service.HandleSubscriptionDeleted(r.Context(), &subscription); err != nil {
			h.log.Error("Failed to handle subscription deletion", "error", err, "subscription_id", subscription.ID)
			httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to process subscription deletion")
			return
		}

	default:
		h.log.Info("Unhandled webhook event type", "type", event.Type)
	}

	// Return success
	w.WriteHeader(http.StatusOK)
}

// HandleGetAccounting returns accounting data grouped by country (admin only)
// @Summary Get accounting data (Cloud only, Admin)
// @Description Returns subscription revenue data grouped by country for accounting purposes. Cloud-specific admin endpoint.
// @Tags Billing
// @Produce json
// @Security BearerAuth
// @Param year query int true "Year for accounting data (e.g., 2025)"
// @Param month query int false "Month for accounting data (1-12, omit for whole year)"
// @Success 200 {object} object{year=int,month=int,rows=[]object,total_ht=number,total_vat=number,total_ttc=number} "Accounting data retrieved successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid or missing year parameter"
// @Failure 401 {object} httputil.ErrorResponse "User not authenticated"
// @Failure 403 {object} httputil.ErrorResponse "Admin role required"
// @Failure 500 {object} httputil.ErrorResponse "Failed to get accounting data"
// @Router /api/v1/billing/accounting [get]
func (h *Handler) HandleGetAccounting(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	var req models.AccountingRequest

	yearStr := r.URL.Query().Get("year")
	if yearStr == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "year parameter is required")
		return
	}

	year := 0
	if _, err := fmt.Sscanf(yearStr, "%d", &year); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid year parameter")
		return
	}
	req.Year = year

	monthStr := r.URL.Query().Get("month")
	if monthStr != "" {
		month := 0
		if _, err := fmt.Sscanf(monthStr, "%d", &month); err != nil {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid month parameter")
			return
		}
		req.Month = month
	}

	// Get accounting data
	resp, err := h.service.GetAccountingData(r.Context(), req)
	if err != nil {
		h.log.Error("Failed to get accounting data", "error", err, "year", req.Year, "month", req.Month)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get accounting data")
		return
	}

	httputil.JSON(w, http.StatusOK, resp)
}
