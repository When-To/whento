// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	ecommerceService "github.com/whento/whento/internal/ecommerce/service"
	"github.com/whento/whento/internal/shop/models"
	"github.com/whento/whento/internal/shop/service"
)

const (
	sessionCookieName = "whento_shop_session"
	sessionMaxAge     = 24 * 60 * 60 // 24 hours in seconds
)

// Handler handles shop-related HTTP requests
type Handler struct {
	service          *service.Service
	ecommerceService *ecommerceService.Service
	log              *slog.Logger
}

// New creates a new shop handler
func New(service *service.Service, ecommerceService *ecommerceService.Service, log *slog.Logger) *Handler {
	return &Handler{
		service:          service,
		ecommerceService: ecommerceService,
		log:              log,
	}
}

// getOrCreateSessionID retrieves or creates a shop session ID
func (h *Handler) getOrCreateSessionID(w http.ResponseWriter, r *http.Request) string {
	// Try to get existing session from cookie
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		// Validate UUID format
		if _, err := uuid.Parse(cookie.Value); err == nil {
			return cookie.Value
		}
	}

	// Generate new session ID
	sessionID := uuid.New().String()

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   sessionMaxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil, // Secure flag only if HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	return sessionID
}

// HandleGetProducts returns available license products
// @Summary Get available license products (Cloud only)
// @Description Returns list of available self-hosted license products (Pro and Enterprise tiers). Cloud-specific endpoint for license sales.
// @Tags Shop
// @Produce json
// @Success 200 {object} object{products=[]object} "Products retrieved successfully"
// @Router /api/v1/shop/products [get]
func (h *Handler) HandleGetProducts(w http.ResponseWriter, r *http.Request) {
	products := h.service.GetProducts()
	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"products": products,
	})
}

// HandleAddToCart adds an item to the cart
// @Summary Add item to shopping cart (Cloud only)
// @Description Adds a license product to the shopping cart. Uses session cookie for cart persistence. Cloud-specific endpoint.
// @Tags Shop
// @Accept json
// @Produce json
// @Param request body object{tier=string,quantity=int} true "Cart item (tier: pro/enterprise, quantity: 1-99)"
// @Success 200 {object} object{cart=object} "Item added successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} httputil.ErrorResponse "Failed to add item to cart"
// @Router /api/v1/shop/cart/items [post]
func (h *Handler) HandleAddToCart(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getOrCreateSessionID(w, r)

	var req struct {
		Tier     string `json:"tier"`
		Quantity int    `json:"quantity"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	// Validate input
	if req.Tier == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Tier is required")
		return
	}

	if req.Quantity <= 0 {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Quantity must be positive")
		return
	}

	// Add to cart
	if err := h.service.AddToCart(r.Context(), sessionID, req.Tier, req.Quantity); err != nil {
		h.log.Error("Failed to add item to cart", "error", err, "session_id", sessionID, "tier", req.Tier)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to add item to cart")
		return
	}

	// Return updated cart
	cart, err := h.service.GetOrCreateCart(r.Context(), sessionID)
	if err != nil {
		h.log.Error("Failed to get cart after adding item", "error", err, "session_id", sessionID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to retrieve cart")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"cart": cart,
	})
}

// HandleGetCart returns the current cart
// @Summary Get shopping cart (Cloud only)
// @Description Returns the current session's shopping cart contents. Cloud-specific endpoint.
// @Tags Shop
// @Produce json
// @Success 200 {object} object{cart=object} "Cart retrieved successfully"
// @Failure 500 {object} httputil.ErrorResponse "Failed to retrieve cart"
// @Router /api/v1/shop/cart [get]
func (h *Handler) HandleGetCart(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getOrCreateSessionID(w, r)

	cart, err := h.service.GetOrCreateCart(r.Context(), sessionID)
	if err != nil {
		h.log.Error("Failed to get cart", "error", err, "session_id", sessionID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to retrieve cart")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"cart": cart,
	})
}

// HandleUpdateQuantity updates the quantity of a cart item
// @Summary Update cart item quantity (Cloud only)
// @Description Updates the quantity of a specific cart item. Cloud-specific endpoint.
// @Tags Shop
// @Accept json
// @Produce json
// @Param tier path string true "License tier (pro/enterprise)"
// @Param request body object{quantity=int} true "New quantity (1-99)"
// @Success 200 {object} object{cart=object} "Quantity updated successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid tier or quantity"
// @Failure 500 {object} httputil.ErrorResponse "Failed to update quantity"
// @Router /api/v1/shop/cart/items/{tier} [patch]
func (h *Handler) HandleUpdateQuantity(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getOrCreateSessionID(w, r)
	tier := chi.URLParam(r, "tier")

	if tier == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Tier is required")
		return
	}

	var req struct {
		Quantity int `json:"quantity"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if req.Quantity <= 0 {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Quantity must be positive")
		return
	}

	// Update quantity
	if err := h.service.UpdateQuantity(r.Context(), sessionID, tier, req.Quantity); err != nil {
		h.log.Error("Failed to update quantity", "error", err, "session_id", sessionID, "tier", tier)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to update quantity")
		return
	}

	// Return updated cart
	cart, err := h.service.GetOrCreateCart(r.Context(), sessionID)
	if err != nil {
		h.log.Error("Failed to get cart after updating quantity", "error", err, "session_id", sessionID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to retrieve cart")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"cart": cart,
	})
}

// HandleRemoveItem removes an item from the cart
// @Summary Remove item from cart (Cloud only)
// @Description Removes a specific item from the shopping cart. Cloud-specific endpoint.
// @Tags Shop
// @Produce json
// @Param tier path string true "License tier to remove (pro/enterprise)"
// @Success 200 {object} object{cart=object} "Item removed successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid tier"
// @Failure 500 {object} httputil.ErrorResponse "Failed to remove item"
// @Router /api/v1/shop/cart/items/{tier} [delete]
func (h *Handler) HandleRemoveItem(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getOrCreateSessionID(w, r)
	tier := chi.URLParam(r, "tier")

	if tier == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Tier is required")
		return
	}

	// Remove item
	if err := h.service.RemoveItem(r.Context(), sessionID, tier); err != nil {
		h.log.Error("Failed to remove item", "error", err, "session_id", sessionID, "tier", tier)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to remove item")
		return
	}

	// Return updated cart
	cart, err := h.service.GetOrCreateCart(r.Context(), sessionID)
	if err != nil {
		h.log.Error("Failed to get cart after removing item", "error", err, "session_id", sessionID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to retrieve cart")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"cart": cart,
	})
}

// HandleClearCart clears all items from the cart
// @Summary Clear shopping cart (Cloud only)
// @Description Removes all items from the shopping cart. Cloud-specific endpoint.
// @Tags Shop
// @Produce json
// @Success 200 {object} object{message=string} "Cart cleared successfully"
// @Failure 500 {object} httputil.ErrorResponse "Failed to clear cart"
// @Router /api/v1/shop/cart [delete]
func (h *Handler) HandleClearCart(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getOrCreateSessionID(w, r)

	if err := h.service.ClearCart(r.Context(), sessionID); err != nil {
		h.log.Error("Failed to clear cart", "error", err, "session_id", sessionID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to clear cart")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Cart cleared successfully",
	})
}

// HandleCheckout creates a Stripe checkout session
// @Summary Create license checkout session (Cloud only)
// @Description Creates a Stripe checkout session for purchasing self-hosted licenses. Supports guest checkout. Cloud-specific endpoint.
// @Tags Shop
// @Accept json
// @Produce json
// @Param request body object{name=string,email=string,company=string,vat_number=string,address=string,postal_code=string,country=string,success_url=string,cancel_url=string} true "Checkout details with customer info and billing address"
// @Success 200 {object} object{checkout_url=string,session_id=string} "Checkout session created successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} httputil.ErrorResponse "Failed to create checkout session"
// @Router /api/v1/shop/checkout [post]
func (h *Handler) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getOrCreateSessionID(w, r)

	var req models.CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.Name == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Name is required")
		return
	}

	if req.Email == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Email is required")
		return
	}

	if len(req.Country) != 2 {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Country must be 2-character ISO code")
		return
	}

	// If VAT number is provided, company name is required
	if req.VATNumber != "" && req.Company == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Company name is required when VAT number is provided")
		return
	}

	// Create checkout session
	checkout, err := h.service.CreateCheckoutSession(r.Context(), sessionID, req)
	if err != nil {
		h.log.Error("Failed to create checkout session", "error", err, "session_id", sessionID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to create checkout session")
		return
	}

	httputil.JSON(w, http.StatusOK, checkout)
}

// HandleGetOrder retrieves an order with its licenses
// @Summary Get order details (Cloud only)
// @Description Retrieves order information including purchased licenses. No authentication required (public access with order ID). Cloud-specific endpoint.
// @Tags Shop
// @Produce json
// @Param order_id path string true "Order UUID"
// @Success 200 {object} object "Order retrieved successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid order ID"
// @Failure 404 {object} httputil.ErrorResponse "Order not found"
// @Router /api/v1/shop/orders/{order_id} [get]
func (h *Handler) HandleGetOrder(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "order_id")

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid order ID")
		return
	}

	order, err := h.service.GetOrderWithLicenses(r.Context(), orderID)
	if err != nil {
		h.log.Error("Failed to get order", "error", err, "order_id", orderID)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Order not found")
		return
	}

	httputil.JSON(w, http.StatusOK, order)
}

// HandleGetOrderBySession retrieves an order with its licenses by Stripe session ID
// @Summary Get order by Stripe session ID (Cloud only)
// @Description Retrieves order information by Stripe checkout session ID. Used after successful checkout redirect. Cloud-specific endpoint.
// @Tags Shop
// @Produce json
// @Param session_id path string true "Stripe checkout session ID"
// @Success 200 {object} object "Order retrieved successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid session ID"
// @Failure 404 {object} httputil.ErrorResponse "Order not found"
// @Router /api/v1/shop/orders/by-session/{session_id} [get]
func (h *Handler) HandleGetOrderBySession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")

	if sessionID == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Session ID is required")
		return
	}

	// Get order by session ID via ecommerce service
	order, err := h.ecommerceService.GetOrderByStripeSessionID(r.Context(), sessionID)
	if err != nil {
		h.log.Error("Failed to get order by session", "error", err, "session_id", sessionID)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Order not found")
		return
	}

	// Get full order details with licenses
	orderWithLicenses, err := h.service.GetOrderWithLicenses(r.Context(), order.ID)
	if err != nil {
		h.log.Error("Failed to get order with licenses", "error", err, "order_id", order.ID)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Order not found")
		return
	}

	httputil.JSON(w, http.StatusOK, orderWithLicenses)
}

// HandleValidateVAT validates a VAT number using VIES
// @Summary Validate VAT number (Cloud only)
// @Description Validates EU VAT number using VIES service. Cloud-specific endpoint.
// @Tags Shop
// @Accept json
// @Produce json
// @Param request body object{vat_number=string} true "VAT number to validate (e.g., FRXX123456789)"
// @Success 200 {object} object "VAT validation result"
// @Failure 400 {object} httputil.ErrorResponse "Invalid request body or VAT number"
// @Failure 500 {object} httputil.ErrorResponse "Failed to validate VAT number"
// @Router /api/v1/shop/validate-vat [post]
func (h *Handler) HandleValidateVAT(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VATNumber string `json:"vat_number"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if req.VATNumber == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "VAT number is required")
		return
	}

	// Validate VAT number
	validation, err := h.service.ValidateVATNumber(r.Context(), req.VATNumber)
	if err != nil {
		h.log.Error("Failed to validate VAT number", "error", err, "vat_number", req.VATNumber)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to validate VAT number")
		return
	}

	httputil.JSON(w, http.StatusOK, validation)
}

// HandleDownloadLicenses generates a ZIP file with all licenses for an order (one file per license)
// @Summary Download all order licenses as ZIP (Cloud only)
// @Description Generates and downloads a ZIP file containing all license JSON files for an order. Cloud-specific endpoint.
// @Tags Shop
// @Produce application/zip
// @Param order_id path string true "Order UUID"
// @Success 200 {file} file "ZIP file with all licenses"
// @Failure 400 {object} httputil.ErrorResponse "Invalid order ID"
// @Failure 404 {object} httputil.ErrorResponse "Order not found"
// @Failure 500 {object} httputil.ErrorResponse "Failed to generate ZIP file"
// @Router /api/v1/shop/orders/{order_id}/download [get]
func (h *Handler) HandleDownloadLicenses(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "order_id")

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid order ID")
		return
	}

	order, err := h.service.GetOrderWithLicenses(r.Context(), orderID)
	if err != nil {
		h.log.Error("Failed to get order for download", "error", err, "order_id", orderID)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Order not found")
		return
	}

	// Create ZIP file in memory
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add each license as a separate file in the ZIP
	for _, license := range order.Licenses {
		// Create filename: Licence_tier-support_key.json
		filename := fmt.Sprintf("Licence_%s-%s.json", license.Tier, license.SupportKey)

		// Add file to ZIP
		fileWriter, err := zipWriter.Create(filename)
		if err != nil {
			h.log.Error("Failed to create file in ZIP", "error", err, "filename", filename)
			continue
		}

		if _, err := fileWriter.Write([]byte(license.LicenseJSON)); err != nil {
			h.log.Error("Failed to write license to ZIP", "error", err, "filename", filename)
			continue
		}
	}

	// Close the ZIP writer
	if err := zipWriter.Close(); err != nil {
		h.log.Error("Failed to close ZIP writer", "error", err, "order_id", orderID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to generate ZIP file")
		return
	}

	// Set headers for ZIP download
	zipFilename := fmt.Sprintf("whento-licenses-%s.zip", orderID.String()[:8])
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFilename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))

	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// HandleDownloadSingleLicense generates a downloadable JSON file for a single license
// @Summary Download single license file (Cloud only)
// @Description Downloads a single license JSON file from an order. Cloud-specific endpoint.
// @Tags Shop
// @Produce application/json
// @Param order_id path string true "Order UUID"
// @Param license_id path string true "License UUID"
// @Success 200 {file} file "License JSON file"
// @Failure 400 {object} httputil.ErrorResponse "Invalid order ID or license ID"
// @Failure 404 {object} httputil.ErrorResponse "Order or license not found"
// @Router /api/v1/shop/orders/{order_id}/licenses/{license_id}/download [get]
func (h *Handler) HandleDownloadSingleLicense(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "order_id")
	licenseIDStr := chi.URLParam(r, "license_id")

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid order ID")
		return
	}

	licenseID, err := uuid.Parse(licenseIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid license ID")
		return
	}

	order, err := h.service.GetOrderWithLicenses(r.Context(), orderID)
	if err != nil {
		h.log.Error("Failed to get order for license download", "error", err, "order_id", orderID)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Order not found")
		return
	}

	// Find the specific license
	var targetLicense *models.LicenseInfo
	for i := range order.Licenses {
		if order.Licenses[i].ID == licenseID {
			targetLicense = &order.Licenses[i]
			break
		}
	}

	if targetLicense == nil {
		h.log.Error("License not found in order", "order_id", orderID, "license_id", licenseID)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "License not found")
		return
	}

	// Set headers for file download with format: Licence_tier-support_key.json
	filename := fmt.Sprintf("Licence_%s-%s.json", targetLicense.Tier, targetLicense.SupportKey)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(targetLicense.LicenseJSON)))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(targetLicense.LicenseJSON))
}
