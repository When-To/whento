// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"

	"github.com/whento/pkg/httputil"
	ecommerceModels "github.com/whento/whento/internal/ecommerce/models"
	ecommerceService "github.com/whento/whento/internal/ecommerce/service"
	"github.com/whento/whento/internal/shop/email"
	"github.com/whento/whento/internal/shop/models"
	"github.com/whento/whento/internal/shop/service"
)

// WebhookHandler handles Stripe webhook events for shop
type WebhookHandler struct {
	shopService      *service.Service // Shop service for license generation
	ecommerceService *ecommerceService.Service
	emailSender      *email.Sender
	webhookSecret    string
	log              *slog.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(
	shopService *service.Service,
	ecommerceService *ecommerceService.Service,
	emailSender *email.Sender,
	webhookSecret string,
	log *slog.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		shopService:      shopService,
		ecommerceService: ecommerceService,
		emailSender:      emailSender,
		webhookSecret:    webhookSecret,
		log:              log,
	}
}

// HandleWebhook processes Stripe webhook events
//
//	@Summary		Stripe webhook for license purchases (Cloud only)
//	@Description	Processes Stripe webhook events for one-time license purchases. Handles checkout.session.completed to generate Ed25519-signed licenses and send delivery emails. Cloud-specific endpoint.
//	@Tags			Shop
//	@Accept			json
//	@Produce		json
//	@Param			Stripe-Signature	header		string	true	"Stripe webhook signature for verification"
//	@Success		200					{object}	object{received=bool}	"Webhook received"
//	@Failure		400					{object}	httputil.ErrorResponse	"Failed to read request body"
//	@Failure		401					{object}	httputil.ErrorResponse	"Invalid signature"
//	@Router			/api/v1/shop/webhook [post]
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

	// Handle event type
	switch event.Type {
	case "checkout.session.completed":
		h.handleCheckoutCompleted(r.Context(), event)
	default:
		h.log.Info("Unhandled webhook event type", "type", event.Type)
	}

	// Always return 200 to acknowledge receipt
	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"received": true,
	})
}

// handleCheckoutCompleted processes a completed checkout session
func (h *WebhookHandler) handleCheckoutCompleted(ctx context.Context, event stripe.Event) {
	// Parse checkout session
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		h.log.Error("Failed to parse checkout session", "error", err)
		return
	}

	// IMPORTANT: Only process one-time payments (license purchases)
	// Ignore subscription checkouts (handled by billing webhook)
	if session.Mode != stripe.CheckoutSessionModePayment {
		h.log.Info("Ignoring non-payment checkout session", "session_id", session.ID, "mode", session.Mode)
		return
	}

	h.log.Info("Processing checkout completion", "session_id", session.ID)

	// Extract metadata
	metadata := session.Metadata
	cartJSON := metadata["cart"]
	billingInfoJSON := metadata["billing_info"]
	country := metadata["country"]
	vatNumber := metadata["vat_number"]
	vatRateStr := metadata["vat_rate"]
	vatAmountStr := metadata["vat_amount"]

	// Parse cart
	var cart models.Cart
	if err := json.Unmarshal([]byte(cartJSON), &cart); err != nil {
		h.log.Error("Failed to parse cart from metadata", "error", err, "session_id", session.ID)
		return
	}

	// Parse billing info
	var billingInfo models.CheckoutRequest
	if err := json.Unmarshal([]byte(billingInfoJSON), &billingInfo); err != nil {
		h.log.Error("Failed to parse billing info from metadata", "error", err, "session_id", session.ID)
		return
	}

	// Parse VAT data
	vatRate, err := strconv.ParseFloat(vatRateStr, 64)
	if err != nil {
		h.log.Error("Failed to parse VAT rate", "error", err, "session_id", session.ID)
		vatRate = 0.0
	}

	vatAmount, err := strconv.Atoi(vatAmountStr)
	if err != nil {
		h.log.Error("Failed to parse VAT amount", "error", err, "session_id", session.ID)
		vatAmount = 0
	}

	// Calculate subtotal
	subtotalCents := 0
	for _, item := range cart.Items {
		subtotalCents += item.Price * item.Quantity
	}

	// Create or get client
	client, err := h.ecommerceService.GetOrCreateClient(ctx, ecommerceModels.CreateClientRequest{
		Name:      billingInfo.Name,
		Email:     billingInfo.Email,
		Company:   billingInfo.Company,
		VATNumber: vatNumber,
		Address:   billingInfo.Address,
		Country:   country,
	})
	if err != nil {
		h.log.Error("Failed to create client", "error", err, "session_id", session.ID)
		return
	}

	// Create order
	order, err := h.ecommerceService.CreateOrder(ctx, ecommerceModels.CreateOrderRequest{
		ClientID:        client.ID,
		AmountCents:     subtotalCents,
		Country:         country,
		VATRate:         vatRate,
		VATAmountCents:  vatAmount,
		StripeSessionID: session.ID,
		StripePaymentIntent: func() string {
			if session.PaymentIntent != nil {
				return session.PaymentIntent.ID
			}
			return ""
		}(),
	})
	if err != nil {
		h.log.Error("Failed to create order", "error", err, "session_id", session.ID)
		return
	}

	h.log.Info("Order created", "order_id", order.ID, "client_id", client.ID)

	// Generate licenses using shop service
	licenses, err := h.shopService.GenerateLicenses(ctx, cart, billingInfo)
	if err != nil {
		h.log.Error("Failed to generate licenses", "error", err, "order_id", order.ID)
		// Mark order as failed
		h.ecommerceService.UpdateOrderStatus(ctx, order.ID, ecommerceModels.OrderStatusFailed)
		return
	}

	// Store licenses in database
	for _, lic := range licenses {
		licenseJSON, err := json.Marshal(lic)
		if err != nil {
			h.log.Error("Failed to marshal license", "error", err, "order_id", order.ID)
			continue
		}

		_, err = h.ecommerceService.CreateLicense(ctx, ecommerceModels.CreateLicenseRequest{
			OrderID: order.ID,
			License: licenseJSON,
		})
		if err != nil {
			h.log.Error("Failed to store license", "error", err, "order_id", order.ID)
		}
	}

	h.log.Info("Licenses generated and stored", "order_id", order.ID, "count", len(licenses))

	// Mark order as completed
	if err := h.ecommerceService.UpdateOrderStatus(ctx, order.ID, ecommerceModels.OrderStatusCompleted); err != nil {
		h.log.Error("Failed to update order status", "error", err, "order_id", order.ID)
	}

	// Send email with licenses
	if err := h.emailSender.SendLicenses(ctx, email.LicenseEmail{
		To:          billingInfo.Email,
		ClientName:  billingInfo.Name,
		OrderID:     order.ID.String(),
		Licenses:    licenses,
		TotalAmount: subtotalCents + vatAmount,
		VATAmount:   vatAmount,
		Country:     country,
	}); err != nil {
		h.log.Error("Failed to send license email", "error", err, "order_id", order.ID, "email", billingInfo.Email)
		// Don't fail the webhook - licenses are already stored and customer can download from order page
	} else {
		h.log.Info("License email sent successfully", "order_id", order.ID, "email", billingInfo.Email)
	}
}
