// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/service"
)

// AvailabilityHandler handles availability HTTP requests
type AvailabilityHandler struct {
	availabilityService *service.AvailabilityService
}

// NewAvailabilityHandler creates a new availability handler
func NewAvailabilityHandler(availabilityService *service.AvailabilityService) *AvailabilityHandler {
	return &AvailabilityHandler{availabilityService: availabilityService}
}

// CreateAvailability handles creating a new availability
//
//	@Summary		Create availability
//	@Description	Creates a new availability slot for a participant. Public endpoint (uses calendar token).
//	@Tags			Availabilities
//	@Accept			json
//	@Produce		json
//	@Param			token	path		string								true	"Calendar public token"
//	@Param			pid		path		string								true	"Participant ID"
//	@Param			request	body		models.CreateAvailabilityRequest	true	"Availability details"
//	@Success		201		{object}	models.AvailabilityResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar or participant not found"
//	@Router			/api/v1/availabilities/calendar/{token}/participant/{pid} [post]
func (h *AvailabilityHandler) CreateAvailability(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")

	var req models.CreateAvailabilityRequest
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

	availability, err := h.availabilityService.CreateAvailability(r.Context(), token, participantID, &req)
	if err != nil {
		handleAvailabilityError(w, r, err, "Failed to create availability")
		return
	}

	httputil.JSON(w, http.StatusCreated, availability)
}

// GetParticipantAvailabilities retrieves all availabilities for a participant
//
//	@Summary		Get participant availabilities
//	@Description	Returns all availability slots for a participant. Public endpoint. Optionally filter by date range using query parameters.
//	@Tags			Availabilities
//	@Produce		json
//	@Param			token	path		string	true	"Calendar public token"
//	@Param			pid		path		string	true	"Participant ID"
//	@Param			start	query		string	false	"Start date (YYYY-MM-DD)"
//	@Param			end		query		string	false	"End date (YYYY-MM-DD)"
//	@Success		200		{object}	models.ParticipantAvailabilitiesResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid date format"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar or participant not found"
//	@Router			/api/v1/availabilities/calendar/{token}/participant/{pid} [get]
func (h *AvailabilityHandler) GetParticipantAvailabilities(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")

	// Get optional date range query parameters
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	availabilities, err := h.availabilityService.GetParticipantAvailabilities(r.Context(), token, participantID, startDate, endDate)
	if err != nil {
		handleAvailabilityError(w, r, err, "Failed to get availabilities")
		return
	}

	httputil.JSON(w, http.StatusOK, availabilities)
}

// UpdateAvailability updates an existing availability
//
//	@Summary		Update availability
//	@Description	Updates an availability slot for a specific date. Public endpoint.
//	@Tags			Availabilities
//	@Accept			json
//	@Produce		json
//	@Param			token	path		string								true	"Calendar public token"
//	@Param			pid		path		string								true	"Participant ID"
//	@Param			date	path		string								true	"Date (YYYY-MM-DD)"
//	@Param			request	body		models.UpdateAvailabilityRequest	true	"Updated availability"
//	@Success		200		{object}	models.AvailabilityResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request"
//	@Failure		404		{object}	httputil.ErrorResponse	"Availability not found"
//	@Router			/api/v1/availabilities/calendar/{token}/participant/{pid}/{date} [patch]
func (h *AvailabilityHandler) UpdateAvailability(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")
	date := chi.URLParam(r, "date")

	var req models.UpdateAvailabilityRequest
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

	availability, err := h.availabilityService.UpdateAvailability(r.Context(), token, participantID, date, &req)
	if err != nil {
		handleAvailabilityError(w, r, err, "Failed to update availability")
		return
	}

	httputil.JSON(w, http.StatusOK, availability)
}

// DeleteAvailability deletes an availability
//
//	@Summary		Delete availability
//	@Description	Deletes an availability slot for a specific date. Public endpoint.
//	@Tags			Availabilities
//	@Produce		json
//	@Param			token	path		string	true	"Calendar public token"
//	@Param			pid		path		string	true	"Participant ID"
//	@Param			date	path		string	true	"Date (YYYY-MM-DD)"
//	@Success		200		{object}	map[string]string
//	@Failure		404		{object}	httputil.ErrorResponse	"Availability not found"
//	@Router			/api/v1/availabilities/calendar/{token}/participant/{pid}/{date} [delete]
func (h *AvailabilityHandler) DeleteAvailability(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")
	date := chi.URLParam(r, "date")

	err := h.availabilityService.DeleteAvailability(r.Context(), token, participantID, date)
	if err != nil {
		handleAvailabilityError(w, r, err, "Failed to delete availability")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "Availability deleted successfully"})
}

// GetDateSummary gets all participants available on a specific date
//
//	@Summary		Get date summary
//	@Description	Returns all participants available on a specific date with their time slots. Public endpoint.
//	@Tags			Availabilities
//	@Produce		json
//	@Param			token	path		string	true	"Calendar public token"
//	@Param			date	path		string	true	"Date (YYYY-MM-DD)"
//	@Success		200		{object}	models.DateAvailabilitySummary
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar not found"
//	@Router			/api/v1/availabilities/calendar/{token}/dates/{date} [get]
func (h *AvailabilityHandler) GetDateSummary(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	date := chi.URLParam(r, "date")

	summary, err := h.availabilityService.GetDateSummary(r.Context(), token, date)
	if err != nil {
		handleAvailabilityError(w, r, err, "Failed to get date summary")
		return
	}

	httputil.JSON(w, http.StatusOK, summary)
}

// GetRangeSummary gets availability summary over a date range
//
//	@Summary		Get range summary
//	@Description	Returns availability summary for all dates in a range. Public endpoint.
//	@Tags			Availabilities
//	@Produce		json
//	@Param			token	path		string	true	"Calendar public token"
//	@Param			start	query		string	true	"Start date (YYYY-MM-DD)"
//	@Param			end		query		string	true	"End date (YYYY-MM-DD)"
//	@Success		200		{array}		models.DateAvailabilitySummary
//	@Failure		400		{object}	httputil.ErrorResponse	"Missing start/end parameters"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar not found"
//	@Router			/api/v1/availabilities/calendar/{token}/range [get]
func (h *AvailabilityHandler) GetRangeSummary(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	if startDate == "" || endDate == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "start and end query parameters are required")
		return
	}

	summaries, err := h.availabilityService.GetRangeSummary(r.Context(), token, startDate, endDate, r.URL.Query().Get("participant_id"))
	if err != nil {
		handleAvailabilityError(w, r, err, "Failed to get range summary")
		return
	}

	httputil.JSON(w, http.StatusOK, summaries)
}

// handleAvailabilityError handles common error cases
func handleAvailabilityError(w http.ResponseWriter, r *http.Request, err error, defaultMsg string) {
	log := logger.FromContext(r.Context())

	switch {
	case errors.Is(err, service.ErrCalendarNotFound):
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
	case errors.Is(err, service.ErrParticipantNotFound):
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Participant not found")
	case errors.Is(err, service.ErrAvailabilityNotFound):
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Availability not found")
	case errors.Is(err, service.ErrAvailabilityExists):
		httputil.Error(w, http.StatusConflict, httputil.ErrCodeConflict, "Availability already exists for this date")
	case errors.Is(err, service.ErrInvalidDate):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid date format, expected YYYY-MM-DD")
	case errors.Is(err, service.ErrInvalidTime):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid time format, expected HH:MM")
	case errors.Is(err, service.ErrInvalidTimeRange):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "End time must be after start time")
	case errors.Is(err, service.ErrDurationTooShort):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Availability duration is less than the minimum required for this calendar")
	case errors.Is(err, service.ErrWeekdayNotAllowed):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "This day of the week is not allowed for this calendar")
	case errors.Is(err, service.ErrDateInPast):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Cannot modify availability for past dates")
	default:
		log.Error(defaultMsg, "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, defaultMsg)
	}
}
