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

type RecurrenceHandler struct {
	service *service.AvailabilityService
}

func NewRecurrenceHandler(service *service.AvailabilityService) *RecurrenceHandler {
	return &RecurrenceHandler{
		service: service,
	}
}

// CreateRecurrence handles POST /calendar/{token}/participant/{pid}/recurrence
// @Summary Create a new recurrence pattern
// @Description Creates a recurring availability pattern for a participant (e.g., every Monday 9:00-17:00)
// @Tags Recurrences
// @Accept json
// @Produce json
// @Param token path string true "Calendar public token"
// @Param pid path string true "Participant ID (UUID)"
// @Param body body models.CreateRecurrenceRequest true "Recurrence details"
// @Success 201 {object} models.Recurrence "Recurrence created successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid request body or validation error"
// @Failure 404 {object} httputil.ErrorResponse "Calendar or participant not found"
// @Failure 409 {object} httputil.ErrorResponse "Recurrence with overlapping dates already exists"
// @Failure 500 {object} httputil.ErrorResponse "Internal server error"
// @Router /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence [post]
func (h *RecurrenceHandler) CreateRecurrence(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")

	var req models.CreateRecurrenceRequest
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

	recurrence, err := h.service.CreateRecurrence(r.Context(), token, participantID, &req)
	if err != nil {
		handleRecurrenceError(w, r, err, "Failed to create recurrence")
		return
	}

	httputil.JSON(w, http.StatusCreated, recurrence)
}

// GetParticipantRecurrences handles GET /calendar/{token}/participant/{pid}/recurrences
// @Summary Get all recurrence patterns for a participant
// @Description Retrieves all recurring availability patterns configured for a specific participant
// @Tags Recurrences
// @Produce json
// @Param token path string true "Calendar public token"
// @Param pid path string true "Participant ID (UUID)"
// @Success 200 {array} models.RecurrenceWithExceptions "List of recurrence patterns with their exceptions"
// @Failure 400 {object} httputil.ErrorResponse "Invalid participant ID"
// @Failure 404 {object} httputil.ErrorResponse "Calendar or participant not found"
// @Failure 500 {object} httputil.ErrorResponse "Internal server error"
// @Router /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrences [get]
func (h *RecurrenceHandler) GetParticipantRecurrences(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")

	recurrences, err := h.service.GetParticipantRecurrences(r.Context(), token, participantID)
	if err != nil {
		handleRecurrenceError(w, r, err, "Failed to get recurrences")
		return
	}

	httputil.JSON(w, http.StatusOK, recurrences)
}

// UpdateRecurrence handles PATCH /calendar/{token}/participant/{pid}/recurrence/{rid}
// @Summary Update a recurrence pattern
// @Description Updates an existing recurring availability pattern (e.g., change time from 9:00-17:00 to 10:00-18:00)
// @Tags Recurrences
// @Accept json
// @Produce json
// @Param token path string true "Calendar public token"
// @Param pid path string true "Participant ID (UUID)"
// @Param rid path string true "Recurrence ID (UUID)"
// @Param body body models.UpdateRecurrenceRequest true "Updated recurrence details"
// @Success 200 {object} models.Recurrence "Recurrence updated successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid request body or validation error"
// @Failure 404 {object} httputil.ErrorResponse "Calendar, participant, or recurrence not found"
// @Failure 409 {object} httputil.ErrorResponse "Updated recurrence would overlap with another recurrence"
// @Failure 500 {object} httputil.ErrorResponse "Internal server error"
// @Router /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid} [patch]
func (h *RecurrenceHandler) UpdateRecurrence(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")
	recurrenceID := chi.URLParam(r, "rid")

	var req models.UpdateRecurrenceRequest
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

	recurrence, err := h.service.UpdateRecurrence(r.Context(), token, participantID, recurrenceID, &req)
	if err != nil {
		handleRecurrenceError(w, r, err, "Failed to update recurrence")
		return
	}

	httputil.JSON(w, http.StatusOK, recurrence)
}

// DeleteRecurrence handles DELETE /calendar/{token}/participant/{pid}/recurrence/{rid}
// @Summary Delete a recurrence pattern
// @Description Deletes a recurring availability pattern and all associated exceptions
// @Tags Recurrences
// @Produce json
// @Param token path string true "Calendar public token"
// @Param pid path string true "Participant ID (UUID)"
// @Param rid path string true "Recurrence ID (UUID)"
// @Success 200 {object} map[string]string "Recurrence deleted successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid participant or recurrence ID"
// @Failure 404 {object} httputil.ErrorResponse "Calendar, participant, or recurrence not found"
// @Failure 500 {object} httputil.ErrorResponse "Internal server error"
// @Router /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid} [delete]
func (h *RecurrenceHandler) DeleteRecurrence(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")
	recurrenceID := chi.URLParam(r, "rid")

	if err := h.service.DeleteRecurrence(r.Context(), token, participantID, recurrenceID); err != nil {
		handleRecurrenceError(w, r, err, "Failed to delete recurrence")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{
		"message": "Recurrence deleted successfully",
	})
}

// CreateException handles POST /calendar/{token}/participant/{pid}/recurrence/{rid}/exception
// @Summary Add an exception date to a recurrence pattern
// @Description Excludes a specific date from a recurrence pattern (e.g., skip Monday 2025-01-06)
// @Tags Recurrences
// @Accept json
// @Produce json
// @Param token path string true "Calendar public token"
// @Param pid path string true "Participant ID (UUID)"
// @Param rid path string true "Recurrence ID (UUID)"
// @Param body body models.CreateExceptionRequest true "Exception date to exclude (YYYY-MM-DD)"
// @Success 201 {object} models.RecurrenceException "Exception created successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid request body, date format, or validation error"
// @Failure 404 {object} httputil.ErrorResponse "Calendar, participant, or recurrence not found"
// @Failure 500 {object} httputil.ErrorResponse "Internal server error"
// @Router /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}/exception [post]
func (h *RecurrenceHandler) CreateException(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")
	recurrenceID := chi.URLParam(r, "rid")

	var req models.CreateExceptionRequest
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

	exception, err := h.service.CreateException(r.Context(), token, participantID, recurrenceID, &req)
	if err != nil {
		handleRecurrenceError(w, r, err, "Failed to create exception")
		return
	}

	httputil.JSON(w, http.StatusCreated, exception)
}

// DeleteException handles DELETE /calendar/{token}/participant/{pid}/recurrence/{rid}/exception/{date}
// @Summary Remove an exception date from a recurrence pattern
// @Description Removes a previously excluded date from a recurrence pattern, re-enabling it
// @Tags Recurrences
// @Produce json
// @Param token path string true "Calendar public token"
// @Param pid path string true "Participant ID (UUID)"
// @Param rid path string true "Recurrence ID (UUID)"
// @Param date path string true "Exception date to remove (YYYY-MM-DD)"
// @Success 200 {object} map[string]string "Exception deleted successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid date format"
// @Failure 404 {object} httputil.ErrorResponse "Calendar, participant, recurrence, or exception not found"
// @Failure 500 {object} httputil.ErrorResponse "Internal server error"
// @Router /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}/exception/{date} [delete]
func (h *RecurrenceHandler) DeleteException(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")
	recurrenceID := chi.URLParam(r, "rid")
	date := chi.URLParam(r, "date")

	if err := h.service.DeleteException(r.Context(), token, participantID, recurrenceID, date); err != nil {
		handleRecurrenceError(w, r, err, "Failed to delete exception")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{
		"message": "Exception deleted successfully",
	})
}

// handleRecurrenceError handles common error cases for recurrences
func handleRecurrenceError(w http.ResponseWriter, r *http.Request, err error, defaultMsg string) {
	log := logger.FromContext(r.Context())

	switch {
	case errors.Is(err, service.ErrCalendarNotFound):
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
	case errors.Is(err, service.ErrParticipantNotFound):
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Participant not found")
	case errors.Is(err, service.ErrInvalidParticipantID):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid participant ID")
	case errors.Is(err, service.ErrRecurrenceNotFound):
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Recurrence not found")
	case errors.Is(err, service.ErrInvalidDate):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid date format, expected YYYY-MM-DD")
	case errors.Is(err, service.ErrInvalidTime):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid time format, expected HH:MM")
	case errors.Is(err, service.ErrInvalidTimeRange):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "End time must be after start time")
	case errors.Is(err, service.ErrTimeOutsideAllowedHours):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Time range does not fit within allowed hours for this day")
	case errors.Is(err, service.ErrDurationTooShort):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Availability duration is less than the minimum required for this calendar")
	case errors.Is(err, service.ErrInvalidDayOfWeek):
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "day_of_week must be between 0 (Sunday) and 6 (Saturday)")
	case errors.Is(err, service.ErrRecurrenceOverlap):
		httputil.Error(w, http.StatusConflict, httputil.ErrCodeConflict, "A recurrence already exists for this day with overlapping dates")
	default:
		log.Error(defaultMsg, "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, defaultMsg)
	}
}
