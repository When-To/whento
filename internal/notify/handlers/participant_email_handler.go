// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package handlers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/validator"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/notify/service"
)

// ParticipantEmailHandler handles participant email verification HTTP requests
type ParticipantEmailHandler struct {
	emailService    *service.ParticipantEmailService
	participantRepo *calendarRepo.ParticipantRepository
	calendarRepo    *calendarRepo.CalendarRepository
	logger          *slog.Logger
}

// NewParticipantEmailHandler creates a new participant email handler
func NewParticipantEmailHandler(
	emailService *service.ParticipantEmailService,
	participantRepo *calendarRepo.ParticipantRepository,
	calendarRepo *calendarRepo.CalendarRepository,
	logger *slog.Logger,
) *ParticipantEmailHandler {
	return &ParticipantEmailHandler{
		emailService:    emailService,
		participantRepo: participantRepo,
		calendarRepo:    calendarRepo,
		logger:          logger,
	}
}

// AddEmail adds email to participant and sends verification
//
//	@Summary		Add email to participant
//	@Description	Adds an email address to a participant and sends a verification email
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			token	path		string							true	"Calendar public token"
//	@Param			pid		path		string							true	"Participant ID"
//	@Param			request	body		object{email=string}			true	"Email address"
//	@Success		200		{object}	object{participant_id=string,email=string,verified=bool,message=string}
//	@Failure		400		{object}	httputil.ErrorResponse
//	@Failure		404		{object}	httputil.ErrorResponse
//	@Failure		500		{object}	httputil.ErrorResponse
//	@Router			/api/v1/calendars/{token}/participants/{pid}/email [post]
func (h *ParticipantEmailHandler) AddEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")

	// Validate participant ID
	pid, err := uuid.Parse(participantID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid participant ID")
		return
	}

	// Parse request
	var req calendarModels.AddParticipantEmailRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	// Validate request
	if err := validator.Validate(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	// Verify calendar token exists
	calendar, err := h.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
		return
	}

	// Get participant to verify it belongs to this calendar
	participant, err := h.participantRepo.GetByID(ctx, pid)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Participant not found")
		return
	}

	if participant.CalendarID != calendar.ID {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Participant not found in this calendar")
		return
	}

	// Parse notify config to check if participants can add emails
	// For now, we'll allow it if notifications are enabled
	// TODO: Add specific check for notify_participants in config

	// Add email and send verification
	if err := h.emailService.AddEmail(ctx, pid, req.Email, participant.Name, "en"); err != nil {
		h.logger.Error("Failed to add participant email", "participant_id", pid, "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, calendarModels.ParticipantEmailResponse{
		ParticipantID: pid,
		Email:         req.Email,
		Verified:      false,
		Message:       "Verification email sent",
	})
}

// VerifyEmail verifies participant email with token
//
//	@Summary		Verify participant email
//	@Description	Verifies a participant's email address using the verification token
//	@Tags			Notifications
//	@Produce		json
//	@Param			token	path		string	true	"Verification token"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	httputil.ErrorResponse
//	@Failure		500		{object}	httputil.ErrorResponse
//	@Router			/api/v1/calendars/participants/verify-email/{token} [get]
func (h *ParticipantEmailHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := chi.URLParam(r, "token")

	if token == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Verification token is required")
		return
	}

	if err := h.emailService.VerifyEmail(ctx, token); err != nil {
		h.logger.Error("Failed to verify participant email", "error", err)
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{
		"message": "Email verified successfully",
	})
}

// ResendVerification resends the verification email
//
//	@Summary		Resend verification email
//	@Description	Resends the verification email to a participant
//	@Tags			Notifications
//	@Produce		json
//	@Param			token	path		string	true	"Calendar public token"
//	@Param			pid		path		string	true	"Participant ID"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	httputil.ErrorResponse
//	@Failure		404		{object}	httputil.ErrorResponse
//	@Failure		500		{object}	httputil.ErrorResponse
//	@Router			/api/v1/calendars/{token}/participants/{pid}/resend-verification [post]
func (h *ParticipantEmailHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := chi.URLParam(r, "token")
	participantID := chi.URLParam(r, "pid")

	// Validate participant ID
	pid, err := uuid.Parse(participantID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid participant ID")
		return
	}

	// Verify calendar token exists
	calendar, err := h.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
		return
	}

	// Get participant to verify it belongs to this calendar
	participant, err := h.participantRepo.GetByID(ctx, pid)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Participant not found")
		return
	}

	if participant.CalendarID != calendar.ID {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Participant not found in this calendar")
		return
	}

	// Resend verification
	if err := h.emailService.ResendVerification(ctx, pid, "en"); err != nil {
		h.logger.Error("Failed to resend verification", "participant_id", pid, "error", err)
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{
		"message": "Verification email resent",
	})
}
