// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/middleware"
	"github.com/whento/pkg/validator"
	authModels "github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/service"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/quota"
)

// CalendarService is the slice of the calendar domain this handler drives.
//
// Declared here, on the consuming side, rather than taking *service.CalendarService:
// the concrete service reaches two repositories and a cache, so a handler test had to
// build all three to exercise a 404. Go interfaces are structural, so the concrete
// service satisfies this and no call site changes. Everything it passes and returns
// lives in internal/calendar/models, which a fake can import without pulling in the
// repository layer — an interface whose signatures name repository types is not an
// abstraction, only a longer spelling of the same coupling.
type CalendarService interface {
	CreateCalendar(ctx context.Context, userID string, req *models.CreateCalendarRequest) (*models.CalendarResponse, error)
	GetCalendar(ctx context.Context, userID, userRole, calendarID string) (*models.CalendarResponse, error)
	ListMyCalendars(ctx context.Context, userID string) ([]*models.CalendarResponse, error)
	UpdateCalendar(ctx context.Context, userID, userRole, calendarID string, req *models.UpdateCalendarRequest) (*models.CalendarResponse, error)
	DeleteCalendar(ctx context.Context, userID, userRole, calendarID string) error
	RegenerateToken(ctx context.Context, userID, userRole, calendarID, tokenType string) (*models.CalendarResponse, error)
	GetPublicCalendar(ctx context.Context, token, participantID string) (*models.PublicCalendarResponse, error)
	ListUserCalendars(ctx context.Context, targetUserID string) ([]*models.CalendarResponse, error)
}

// UserLookup is the one question this handler asks of the user store: is the account
// creating a calendar allowed to, given that email verification may be required.
type UserLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*authModels.User, error)
}

// QuotaLock serialises the quota check and the creation that follows it.
//
// The implementation is *repository.QuotaLocker, which holds a PostgreSQL advisory
// lock for the duration of fn. It is an interface here so that the handler carries a
// scope instead of a connection pool: the transaction, the SQL and the errors that
// come with them are the repository's business.
//
// WithQuotaLock returns an error only when the lock could not be taken, i.e. only
// when fn never ran.
type QuotaLock interface {
	WithQuotaLock(ctx context.Context, key int64, fn func(context.Context) error) error
}

// CalendarHandler handles calendar HTTP requests
type CalendarHandler struct {
	calendarService CalendarService
	quotaService    quota.QuotaService
	userRepo        UserLookup
	cfg             *config.Config
	quotaLock       QuotaLock
}

// NewCalendarHandler creates a new calendar handler.
//
// quotaLock may be nil, in which case creation runs unserialised — the same escape
// hatch the nil *pgxpool.Pool used to provide.
func NewCalendarHandler(
	calendarService CalendarService,
	quotaService quota.QuotaService,
	userRepo UserLookup,
	cfg *config.Config,
	quotaLock QuotaLock,
) *CalendarHandler {
	return &CalendarHandler{
		calendarService: calendarService,
		quotaService:    quotaService,
		userRepo:        userRepo,
		cfg:             cfg,
		quotaLock:       quotaLock,
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
//
// quotaMessage explains a refused calendar creation.
//
// Only the hosted service caps anything: the self-hosted build reports an unlimited
// allowance and never refuses, so a positive limit means this is the cloud. Saying how
// many, and what the way out is, beats a bare refusal — and the way out is running your
// own instance, since there is no paid tier to move to.
//
// Extracted so it can be tested: the handler around it takes a database transaction
// before it ever reaches here. The message it replaced ("upgrade your subscription", and
// on the other branch "upgrade your license") outlived both by a good margin.
func quotaMessage(limit int) string {
	if limit <= 0 {
		return "Calendar limit reached."
	}

	return fmt.Sprintf(
		"You have reached the limit of %d calendars on the hosted service. "+
			"Host your own instance of WhenTo for unlimited calendars.", limit)
}

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

		// Check that threshold doesn't exceed participant count (unless anonymous registration is enabled)
		if !req.AllowAnonymousParticipants {
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

	// Check the quota and create under a per-user (cloud) or server-wide
	// (self-hosted) advisory lock, so that two concurrent requests cannot both
	// read a count below the allowance and both create a calendar.
	if h.quotaLock == nil {
		h.createUnderQuota(r.Context(), w, userID, userUUID, &req)
		return
	}

	lockKey := h.quotaService.QuotaLockKey(userUUID)
	err = h.quotaLock.WithQuotaLock(r.Context(), lockKey, func(ctx context.Context) error {
		h.createUnderQuota(ctx, w, userID, userUUID, &req)
		return nil
	})
	if err != nil {
		// The lock was never taken, so nothing has been written to w yet.
		logger.FromContext(r.Context()).Error("Failed to acquire quota advisory lock", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to check calendar quota")
	}
}

// createUnderQuota checks the allowance and creates the calendar, writing the
// response either way. It is called while the quota lock is held.
func (h *CalendarHandler) createUnderQuota(
	ctx context.Context,
	w http.ResponseWriter,
	userID string,
	userUUID uuid.UUID,
	req *models.CreateCalendarRequest,
) {
	canCreate, err := h.quotaService.CanCreateCalendar(ctx, userUUID)
	if err != nil {
		logger.FromContext(ctx).Error("Failed to check quota", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to check calendar quota")
		return
	}

	if !canCreate {
		limit, _ := h.quotaService.GetUserLimit(ctx, userUUID)

		httputil.Error(w, http.StatusForbidden, "quota_exceeded", quotaMessage(limit))
		return
	}

	calendar, err := h.calendarService.CreateCalendar(ctx, userID, req)
	if err != nil {
		logger.FromContext(ctx).Error("Failed to create calendar", "error", err, "user_id", userID)
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
