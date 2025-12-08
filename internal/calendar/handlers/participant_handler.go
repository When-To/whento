// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/service"
)

// ParticipantHandler handles participant HTTP requests
type ParticipantHandler struct {
	calendarService *service.CalendarService
}

// NewParticipantHandler creates a new participant handler
func NewParticipantHandler(calendarService *service.CalendarService) *ParticipantHandler {
	return &ParticipantHandler{calendarService: calendarService}
}

// AddParticipant adds a participant to a calendar
//
//	@Summary		Add participant
//	@Description	Adds a new participant to a calendar. Owner or admin only.
//	@Tags			Participants
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string							true	"Calendar ID"
//	@Param			request	body		models.AddParticipantRequest	true	"Participant details"
//	@Success		201		{object}	models.Participant
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request"
//	@Failure		401		{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403		{object}	httputil.ErrorResponse	"Forbidden"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar not found"
//	@Failure		409		{object}	httputil.ErrorResponse	"Participant already exists"
//	@Router			/api/v1/calendars/{id}/participants [post]
func (h *ParticipantHandler) AddParticipant(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	userRole := middleware.GetUserRole(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	calendarID := chi.URLParam(r, "id")

	var req models.AddParticipantRequest
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

	participant, err := h.calendarService.AddParticipant(r.Context(), userID, userRole, calendarID, &req)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to modify this calendar")
			return
		}
		if errors.Is(err, service.ErrParticipantExists) {
			httputil.Error(w, http.StatusConflict, httputil.ErrCodeConflict, "Participant with this name already exists")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to add participant")
		return
	}

	httputil.JSON(w, http.StatusCreated, participant)
}

// UpdateParticipant updates a participant's name
//
//	@Summary		Update participant
//	@Description	Updates a participant's details. Owner or admin only.
//	@Tags			Participants
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string							true	"Calendar ID"
//	@Param			pid		path		string							true	"Participant ID"
//	@Param			request	body		models.UpdateParticipantRequest	true	"Updated details"
//	@Success		200		{object}	models.Participant
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request"
//	@Failure		401		{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403		{object}	httputil.ErrorResponse	"Forbidden"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar or participant not found"
//	@Router			/api/v1/calendars/{id}/participants/{pid} [patch]
func (h *ParticipantHandler) UpdateParticipant(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	userRole := middleware.GetUserRole(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	calendarID := chi.URLParam(r, "id")
	participantID := chi.URLParam(r, "pid")

	var req models.UpdateParticipantRequest
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

	participant, err := h.calendarService.UpdateParticipant(r.Context(), userID, userRole, calendarID, participantID, &req)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
			return
		}
		if errors.Is(err, service.ErrParticipantNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Participant not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to modify this calendar")
			return
		}
		if errors.Is(err, service.ErrParticipantExists) {
			httputil.Error(w, http.StatusConflict, httputil.ErrCodeConflict, "Participant with this name already exists")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to update participant")
		return
	}

	httputil.JSON(w, http.StatusOK, participant)
}

// RemoveParticipant removes a participant from a calendar
//
//	@Summary		Remove participant
//	@Description	Removes a participant from a calendar. Owner or admin only.
//	@Tags			Participants
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Calendar ID"
//	@Param			pid	path		string	true	"Participant ID"
//	@Success		200	{object}	map[string]string
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	httputil.ErrorResponse	"Forbidden"
//	@Failure		404	{object}	httputil.ErrorResponse	"Calendar or participant not found"
//	@Router			/api/v1/calendars/{id}/participants/{pid} [delete]
func (h *ParticipantHandler) RemoveParticipant(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	userRole := middleware.GetUserRole(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	calendarID := chi.URLParam(r, "id")
	participantID := chi.URLParam(r, "pid")

	err := h.calendarService.RemoveParticipant(r.Context(), userID, userRole, calendarID, participantID)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
			return
		}
		if errors.Is(err, service.ErrParticipantNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Participant not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to modify this calendar")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to remove participant")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "Participant removed successfully"})
}
