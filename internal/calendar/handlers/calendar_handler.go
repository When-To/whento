// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/middleware"
	"github.com/whento/pkg/validator"
	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/service"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/quota"
)

// CalendarHandler handles calendar HTTP requests
type CalendarHandler struct {
	calendarService *service.CalendarService
	quotaService    quota.QuotaService
	userRepo        *authRepo.UserRepository
	cfg             *config.Config
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler(
	calendarService *service.CalendarService,
	quotaService quota.QuotaService,
	userRepo *authRepo.UserRepository,
	cfg *config.Config,
) *CalendarHandler {
	return &CalendarHandler{
		calendarService: calendarService,
		quotaService:    quotaService,
		userRepo:        userRepo,
		cfg:             cfg,
	}
}

// CreateCalendar handles calendar creation
//
//	@Summary		Create a calendar
//	@Description	Creates a new calendar for the authenticated user. Enforces quota limits.
//	@Tags			Calendars
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		models.CreateCalendarRequest	true	"Calendar details"
//	@Success		201		{object}	models.CalendarResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		401		{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403		{object}	httputil.ErrorResponse	"Quota exceeded"
//	@Router			/api/v1/calendars [post]
func (h *CalendarHandler) CreateCalendar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	var req models.CreateCalendarRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	// Additional validation for participants
	if len(req.Participants) > 0 {
		// Check for duplicate participant names
		participantNames := make(map[string]bool)
		for _, name := range req.Participants {
			if name == "" {
				continue
			}
			if participantNames[name] {
				httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, "Duplicate participant name: "+name)
				return
			}
			participantNames[name] = true
		}

		// Check that threshold doesn't exceed participant count
		nonEmptyCount := 0
		for _, name := range req.Participants {
			if name != "" {
				nonEmptyCount++
			}
		}
		if req.Threshold > 0 && req.Threshold > nonEmptyCount {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, "Threshold cannot exceed the number of participants")
			return
		}
	}

	// Parse user ID
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid user ID")
		return
	}

	// Check email verification if enabled
	if h.cfg.Email.VerificationEnabled {
		user, err := h.userRepo.GetByID(r.Context(), userUUID)
		if err != nil {
			logger.FromContext(r.Context()).Error("Failed to get user", "error", err, "user_id", userID)
			httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to verify user status")
			return
		}

		if !user.EmailVerified {
			httputil.Error(w, http.StatusForbidden, "email_not_verified", "Please verify your email address before creating calendars")
			return
		}
	}

	// Check quota limits before creating calendar
	canCreate, err := h.quotaService.CanCreateCalendar(r.Context(), userUUID)
	if err != nil {
		logger.FromContext(r.Context()).Error("Failed to check quota", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to check calendar quota")
		return
	}

	if !canCreate {
		// Get limit info for better error message
		userLimit, _ := h.quotaService.GetUserLimit(r.Context(), userUUID)
		serverLimit, _ := h.quotaService.GetServerLimit(r.Context())

		var errorMsg string
		if serverLimit > 0 {
			// Self-hosted: server-wide limit
			errorMsg = "Server calendar limit reached. Please upgrade your license at https://whento.be/pricing"
		} else if userLimit > 0 {
			// Cloud: per-user limit
			errorMsg = "Calendar limit reached for your plan. Please upgrade your subscription."
		} else {
			errorMsg = "Calendar limit reached"
		}

		httputil.Error(w, http.StatusForbidden, "quota_exceeded", errorMsg)
		return
	}

	calendar, err := h.calendarService.CreateCalendar(r.Context(), userID, &req)
	if err != nil {
		logger.FromContext(r.Context()).Error("Failed to create calendar", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to create calendar")
		return
	}

	httputil.JSON(w, http.StatusCreated, calendar)
}

// GetCalendar retrieves a calendar by ID
//
//	@Summary		Get a calendar
//	@Description	Retrieves a calendar by ID. Owner or admin only.
//	@Tags			Calendars
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Calendar ID"
//	@Success		200	{object}	models.CalendarResponse
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	httputil.ErrorResponse	"Forbidden"
//	@Failure		404	{object}	httputil.ErrorResponse	"Calendar not found"
//	@Router			/api/v1/calendars/{id} [get]
func (h *CalendarHandler) GetCalendar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	userRole := middleware.GetUserRole(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	calendarID := chi.URLParam(r, "id")

	calendar, err := h.calendarService.GetCalendar(r.Context(), userID, userRole, calendarID)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to access this calendar")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get calendar")
		return
	}

	httputil.JSON(w, http.StatusOK, calendar)
}

// ListMyCalendars lists all calendars owned by the user
//
//	@Summary		List my calendars
//	@Description	Returns all calendars owned by the authenticated user
//	@Tags			Calendars
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		models.CalendarResponse
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Router			/api/v1/calendars [get]
func (h *CalendarHandler) ListMyCalendars(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	calendars, err := h.calendarService.ListMyCalendars(r.Context(), userID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to list calendars")
		return
	}

	httputil.JSON(w, http.StatusOK, calendars)
}

// UpdateCalendar updates a calendar
//
//	@Summary		Update a calendar
//	@Description	Updates calendar details. Owner or admin only.
//	@Tags			Calendars
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string							true	"Calendar ID"
//	@Param			request	body		models.UpdateCalendarRequest	true	"Calendar updates"
//	@Success		200		{object}	models.CalendarResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request"
//	@Failure		401		{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403		{object}	httputil.ErrorResponse	"Forbidden"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar not found"
//	@Router			/api/v1/calendars/{id} [patch]
func (h *CalendarHandler) UpdateCalendar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	userRole := middleware.GetUserRole(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	calendarID := chi.URLParam(r, "id")

	var req models.UpdateCalendarRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	calendar, err := h.calendarService.UpdateCalendar(r.Context(), userID, userRole, calendarID, &req)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to modify this calendar")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to update calendar")
		return
	}

	httputil.JSON(w, http.StatusOK, calendar)
}

// DeleteCalendar deletes a calendar
//
//	@Summary		Delete a calendar
//	@Description	Deletes a calendar and all its data. Owner or admin only.
//	@Tags			Calendars
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Calendar ID"
//	@Success		200	{object}	map[string]string
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	httputil.ErrorResponse	"Forbidden"
//	@Failure		404	{object}	httputil.ErrorResponse	"Calendar not found"
//	@Router			/api/v1/calendars/{id} [delete]
func (h *CalendarHandler) DeleteCalendar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	userRole := middleware.GetUserRole(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	calendarID := chi.URLParam(r, "id")

	err := h.calendarService.DeleteCalendar(r.Context(), userID, userRole, calendarID)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to delete this calendar")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to delete calendar")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "Calendar deleted successfully"})
}

// RegenerateToken regenerates a calendar token
//
//	@Summary		Regenerate calendar token
//	@Description	Regenerates the public or ICS token for a calendar. Owner or admin only.
//	@Tags			Calendars
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string							true	"Calendar ID"
//	@Param			request	body		models.RegenerateTokenRequest	true	"Token type (public or ics)"
//	@Success		200		{object}	models.CalendarResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid token type"
//	@Failure		401		{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403		{object}	httputil.ErrorResponse	"Forbidden"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar not found"
//	@Router			/api/v1/calendars/{id}/regenerate-token [post]
func (h *CalendarHandler) RegenerateToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	userRole := middleware.GetUserRole(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	calendarID := chi.URLParam(r, "id")

	var req models.RegenerateTokenRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	calendar, err := h.calendarService.RegenerateToken(r.Context(), userID, userRole, calendarID, req.TokenType)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to modify this calendar")
			return
		}
		if errors.Is(err, service.ErrInvalidTokenType) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid token type")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to regenerate token")
		return
	}

	httputil.JSON(w, http.StatusOK, calendar)
}

// GetPublicCalendar retrieves a calendar by public token (no auth)
//
//	@Summary		Get public calendar
//	@Description	Retrieves a calendar using its public token. No authentication required.
//	@Tags			Calendars
//	@Produce		json
//	@Param			token			path		string	true	"Public calendar token"
//	@Param			participant_id	query		string	false	"Filter by participant ID"
//	@Success		200				{object}	models.PublicCalendarResponse
//	@Failure		404				{object}	httputil.ErrorResponse	"Calendar not found"
//	@Router			/api/v1/calendars/public/{token} [get]
func (h *CalendarHandler) GetPublicCalendar(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := r.URL.Query().Get("participant_id")

	calendar, err := h.calendarService.GetPublicCalendar(r.Context(), token, participantID)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get calendar")
		return
	}

	httputil.JSON(w, http.StatusOK, calendar)
}

// ListUserCalendars retrieves all calendars for a specific user (admin only)
//
//	@Summary		List user's calendars (Admin)
//	@Description	Returns all calendars owned by a specific user. Admin only.
//	@Tags			Admin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"User ID"
//	@Success		200	{array}		models.CalendarResponse
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	httputil.ErrorResponse	"Forbidden (requires admin role)"
//	@Router			/api/v1/calendars/admin/users/{id}/calendars [get]
func (h *CalendarHandler) ListUserCalendars(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	calendars, err := h.calendarService.ListUserCalendars(r.Context(), userID)
	if err != nil {
		logger.FromContext(r.Context()).Error("Failed to list user calendars", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to list user calendars")
		return
	}

	httputil.JSON(w, http.StatusOK, calendars)
}
