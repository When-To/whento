// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/whento/internal/ecommerce/service"
)

// Handler handles e-commerce-related HTTP requests (admin only)
type Handler struct {
	service *service.Service
	log     *slog.Logger
}

// New creates a new e-commerce handler
func New(service *service.Service, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

// HandleSearchLicense searches for a license by support key
//
//	@Summary		Search license by support key (Cloud only, Admin)
//	@Description	Searches for a sold license using the support key. Used by WhenTo support team to verify customer support status. Admin-only endpoint. Cloud-specific.
//	@Tags			E-Commerce
//	@Produce		json
//	@Security		BearerAuth
//	@Param			support_key	query		string	true	"Support key (e.g., SUPP-XXXX-XXXX-XXXX)"
//	@Success		200			{object}	object	"License found with customer and support details"
//	@Failure		400			{object}	httputil.ErrorResponse	"Missing support_key parameter"
//	@Failure		401			{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		403			{object}	httputil.ErrorResponse	"Admin role required"
//	@Failure		404			{object}	httputil.ErrorResponse	"License not found"
//	@Failure		500			{object}	httputil.ErrorResponse	"Internal server error"
//	@Router			/api/v1/admin/licenses/search [get]
func (h *Handler) HandleSearchLicense(w http.ResponseWriter, r *http.Request) {
	supportKey := r.URL.Query().Get("support_key")
	if supportKey == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "support_key query parameter is required")
		return
	}

	license, err := h.service.SearchBySupportKey(r.Context(), supportKey)
	if err != nil {
		h.log.Error("Failed to search license", "error", err, "support_key", supportKey)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to search license")
		return
	}

	if license == nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "License not found")
		return
	}

	httputil.JSON(w, http.StatusOK, license)
}

// HandleGetLicense retrieves a license by ID
//
//	@Summary		Get license by ID (Cloud only, Admin)
//	@Description	Retrieves a sold license by its UUID. Admin-only endpoint. Cloud-specific.
//	@Tags			E-Commerce
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"License ID (UUID)"
//	@Success		200	{object}	object	"License details"
//	@Failure		400	{object}	httputil.ErrorResponse	"Invalid license ID"
//	@Failure		401	{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		403	{object}	httputil.ErrorResponse	"Admin role required"
//	@Failure		404	{object}	httputil.ErrorResponse	"License not found"
//	@Router			/api/v1/admin/licenses/{id} [get]
func (h *Handler) HandleGetLicense(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid license ID")
		return
	}

	license, err := h.service.GetSoldLicense(r.Context(), id)
	if err != nil {
		h.log.Error("Failed to get license", "error", err, "id", id)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "License not found")
		return
	}

	httputil.JSON(w, http.StatusOK, license)
}

// HandleListClients retrieves all clients with pagination
//
//	@Summary		List clients (Cloud only, Admin)
//	@Description	Retrieves all clients (license purchasers) with pagination. Admin-only endpoint. Cloud-specific.
//	@Tags			E-Commerce
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit	query		int	false	"Number of results per page (default: 20)"
//	@Param			offset	query		int	false	"Offset for pagination (default: 0)"
//	@Success		200		{object}	object	"List of clients"
//	@Failure		401		{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		403		{object}	httputil.ErrorResponse	"Admin role required"
//	@Failure		500		{object}	httputil.ErrorResponse	"Internal server error"
//	@Router			/api/v1/admin/clients [get]
func (h *Handler) HandleListClients(w http.ResponseWriter, r *http.Request) {
	limit, offset := getPagination(r)

	resp, err := h.service.ListClients(r.Context(), limit, offset)
	if err != nil {
		h.log.Error("Failed to list clients", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to list clients")
		return
	}

	httputil.JSON(w, http.StatusOK, resp)
}

// HandleGetClient retrieves a client by ID with their orders
//
//	@Summary		Get client details (Cloud only, Admin)
//	@Description	Retrieves a client with their order history. Admin-only endpoint. Cloud-specific.
//	@Tags			E-Commerce
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Client ID (UUID)"
//	@Success		200	{object}	object	"Client details with orders"
//	@Failure		400	{object}	httputil.ErrorResponse	"Invalid client ID"
//	@Failure		401	{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		403	{object}	httputil.ErrorResponse	"Admin role required"
//	@Failure		404	{object}	httputil.ErrorResponse	"Client not found"
//	@Router			/api/v1/admin/clients/{id} [get]
func (h *Handler) HandleGetClient(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid client ID")
		return
	}

	client, err := h.service.GetClientWithOrders(r.Context(), id)
	if err != nil {
		h.log.Error("Failed to get client", "error", err, "id", id)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Client not found")
		return
	}

	httputil.JSON(w, http.StatusOK, client)
}

// HandleListOrders retrieves all orders with pagination
//
//	@Summary		List orders (Cloud only, Admin)
//	@Description	Retrieves all license orders with pagination. Admin-only endpoint. Cloud-specific.
//	@Tags			E-Commerce
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit	query		int	false	"Number of results per page (default: 20)"
//	@Param			offset	query		int	false	"Offset for pagination (default: 0)"
//	@Success		200		{object}	object	"List of orders"
//	@Failure		401		{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		403		{object}	httputil.ErrorResponse	"Admin role required"
//	@Failure		500		{object}	httputil.ErrorResponse	"Internal server error"
//	@Router			/api/v1/admin/orders [get]
func (h *Handler) HandleListOrders(w http.ResponseWriter, r *http.Request) {
	limit, offset := getPagination(r)

	resp, err := h.service.ListOrders(r.Context(), limit, offset)
	if err != nil {
		h.log.Error("Failed to list orders", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to list orders")
		return
	}

	httputil.JSON(w, http.StatusOK, resp)
}

// HandleGetOrder retrieves an order by ID
//
//	@Summary		Get order details (Cloud only, Admin)
//	@Description	Retrieves an order by its UUID. Admin-only endpoint. Cloud-specific.
//	@Tags			E-Commerce
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Order ID (UUID)"
//	@Success		200	{object}	object	"Order details"
//	@Failure		400	{object}	httputil.ErrorResponse	"Invalid order ID"
//	@Failure		401	{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		403	{object}	httputil.ErrorResponse	"Admin role required"
//	@Failure		404	{object}	httputil.ErrorResponse	"Order not found"
//	@Router			/api/v1/admin/orders/{id} [get]
func (h *Handler) HandleGetOrder(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid order ID")
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		h.log.Error("Failed to get order", "error", err, "id", id)
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Order not found")
		return
	}

	httputil.JSON(w, http.StatusOK, order)
}

// getPagination extracts pagination parameters from request
func getPagination(r *http.Request) (limit, offset int) {
	limit = 20
	offset = 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	return limit, offset
}
