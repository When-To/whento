// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/whento/pkg/httputil"
	"github.com/whento/whento/internal/vat/models"
	"github.com/whento/whento/internal/vat/service"
)

// Handler handles VAT-related HTTP requests
type Handler struct {
	service *service.Service
	log     *slog.Logger
}

// New creates a new VAT handler
func New(service *service.Service, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

// HandleGetRates returns all VAT rates (admin only)
//
//	@Summary		Get all VAT rates (Cloud only, Admin)
//	@Description	Returns all stored VAT rates for EU countries. Admin-only endpoint. Cloud-specific.
//	@Tags			VAT
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	object{rates=[]object,count=int}	"VAT rates"
//	@Failure		401	{object}	httputil.ErrorResponse						"User not authenticated"
//	@Failure		403	{object}	httputil.ErrorResponse						"Admin role required"
//	@Failure		500	{object}	httputil.ErrorResponse						"Failed to retrieve VAT rates"
//	@Router			/api/v1/admin/vat/rates [get]
func (h *Handler) HandleGetRates(w http.ResponseWriter, r *http.Request) {
	rates, err := h.service.GetAllRates(r.Context())
	if err != nil {
		h.log.Error("Failed to get VAT rates", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to retrieve VAT rates")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"rates": rates,
		"count": len(rates),
	})
}

// HandleRefreshRates manually triggers a VAT rates refresh (admin only)
//
//	@Summary		Refresh VAT rates (Cloud only, Admin)
//	@Description	Triggers a manual refresh of VAT rates from external sources. Admin-only endpoint. Cloud-specific.
//	@Tags			VAT
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	object{message=string}	"VAT rates refreshed"
//	@Failure		401	{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		403	{object}	httputil.ErrorResponse	"Admin role required"
//	@Failure		500	{object}	httputil.ErrorResponse	"Failed to refresh VAT rates"
//	@Router			/api/v1/admin/vat/refresh [post]
func (h *Handler) HandleRefreshRates(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RefreshRates(r.Context()); err != nil {
		h.log.Error("Failed to refresh VAT rates", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to refresh VAT rates")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"message": "VAT rates refreshed successfully",
	})
}

// HandleGetReport generates a VAT report for a date range (admin only)
//
//	@Summary		Get VAT report (Cloud only, Admin)
//	@Description	Generates a VAT report for accounting purposes. Admin-only endpoint. Cloud-specific.
//	@Tags			VAT
//	@Produce		json
//	@Security		BearerAuth
//	@Param			start	query		string	true	"Start date (YYYY-MM-DD)"
//	@Param			end		query		string	true	"End date (YYYY-MM-DD)"
//	@Success		200		{object}	object	"VAT report"
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid date parameters"
//	@Failure		401		{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		403		{object}	httputil.ErrorResponse	"Admin role required"
//	@Failure		500		{object}	httputil.ErrorResponse	"Failed to generate report"
//	@Router			/api/v1/admin/vat/report [get]
func (h *Handler) HandleGetReport(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	startDateStr := r.URL.Query().Get("start")
	endDateStr := r.URL.Query().Get("end")

	if startDateStr == "" || endDateStr == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Missing required parameters: start and end dates")
		return
	}

	// Parse dates
	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid start date format (expected YYYY-MM-DD)")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid end date format (expected YYYY-MM-DD)")
		return
	}

	// Set end date to end of day
	endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	// Generate report
	report, err := h.service.GetVATReport(r.Context(), startDate, endDate)
	if err != nil {
		h.log.Error("Failed to generate VAT report", "error", err, "start", startDate, "end", endDate)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to generate VAT report")
		return
	}

	httputil.JSON(w, http.StatusOK, report)
}

// HandleCalculateVAT calculates VAT for a given subtotal, country and optional postal code (public endpoint)
//
//	@Summary		Calculate VAT (Cloud only)
//	@Description	Calculates VAT for checkout based on subtotal, country code, and optional postal code (for regional exceptions). Cloud-specific endpoint.
//	@Tags			Shop
//	@Accept			json
//	@Produce		json
//	@Param			request	body		object{subtotal_cents=int,country_code=string,postal_code=string}	true	"VAT calculation request"
//	@Success		200		{object}	object						"VAT calculation result"
//	@Failure		400		{object}	httputil.ErrorResponse		"Invalid request"
//	@Failure		500		{object}	httputil.ErrorResponse		"Failed to calculate VAT"
//	@Router			/api/v1/shop/calculate-vat [post]
func (h *Handler) HandleCalculateVAT(w http.ResponseWriter, r *http.Request) {
	var req models.CalculateVATRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.SubtotalCents < 0 {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Subtotal cannot be negative")
		return
	}

	if len(req.CountryCode) != 2 {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Country code must be 2 characters")
		return
	}

	// Calculate VAT (postal code is optional for regional exceptions)
	calc, err := h.service.CalculateVAT(r.Context(), req.SubtotalCents, req.CountryCode, req.PostalCode)
	if err != nil {
		h.log.Error("Failed to calculate VAT", "error", err, "country", req.CountryCode, "postal_code", req.PostalCode, "subtotal", req.SubtotalCents)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to calculate VAT")
		return
	}

	httputil.JSON(w, http.StatusOK, calc)
}
