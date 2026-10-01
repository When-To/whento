// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	// Aliased: the constructor below takes a *slog.Logger named `logger`, which
	// would otherwise shadow the package.
	pkglog "github.com/whento/pkg/logger"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/calendar/models"
	notifyModels "github.com/whento/whento/internal/notify/models"
)

// ParticipantEmailService is what this handler asks of the notification domain.
//
// It was taking *service.ParticipantEmailService, which parses templates and talks
// SMTP from its constructor, so none of the three endpoints below could be reached
// from a test. The interface names only the calls that are actually made.
type ParticipantEmailService interface {
	AddEmail(ctx context.Context, participantID uuid.UUID, emailAddress, participantName, locale string) error
	VerifyEmail(ctx context.Context, token string) error
	ResendVerification(ctx context.Context, participantID uuid.UUID, locale string) error
	IsConfigured() bool
}

// CalendarByToken resolves a public calendar token, which is what authorises these
// endpoints. Two calendar repositories were reached into directly from here, past
// both this domain's service and the calendar domain's; what was actually needed of
// them is this lookup and the one below.
type CalendarByToken interface {
	GetByPublicToken(ctx context.Context, token string) (*models.Calendar, error)
}

// ParticipantByID fetches the participant named in the path, so that it can be
// checked against the calendar the token names.
type ParticipantByID interface {
	GetByID(ctx context.Context, id uuid.UUID) (*models.Participant, error)
}

// ParticipantEmailHandler handles participant email verification HTTP requests
type ParticipantEmailHandler struct {
	emailService    ParticipantEmailService
	participantRepo ParticipantByID
	calendarRepo    CalendarByToken
	logger          *slog.Logger
}

// NewParticipantEmailHandler creates a new participant email handler
func NewParticipantEmailHandler(
	emailService ParticipantEmailService,
	participantRepo ParticipantByID,
	calendarRepo CalendarByToken,
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
//	@Param			token	path		string								true	"Calendar public token"
//	@Param			pid		path		string								true	"Participant ID"
//	@Param			request	body		models.AddParticipantEmailRequest	true	"Email address"
//	@Success		200		{object}	models.ParticipantEmailResponse
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
	var req models.AddParticipantEmailRequest
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

	// The calendar's notify config is the gate for participant email addresses:
	// notifications must be enabled, and the notify_participants switch must be on.
	// Without this check an attacker (or a stale bookmark) could register any email
	// against a calendar whose owner never asked for participant notifications.
	if !h.participantEmailEnabled(calendar) {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "This calendar does not accept participant email addresses")
		return
	}

	// Add email and send verification, in the participant's own language.
	if err := h.emailService.AddEmail(ctx, pid, req.Email, participant.Name, participant.Locale); err != nil {
		h.logger.Error("Failed to add participant email",
			"participant_ref", pkglog.Fingerprint(pid.String()), "error", err)
		// Deliberately generic: the internal error can name storage internals.
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to add participant email")
		return
	}

	httputil.JSON(w, http.StatusOK, models.ParticipantEmailResponse{
		ParticipantID: pid,
		Email:         req.Email,
		Verified:      false,
		Message:       "Verification email sent",
	})
}

// participantEmailEnabled reports whether a calendar allows participants to add
// email addresses. Every layer of consent has to line up:
//
//   - notifications must be switched on (config.Enabled);
//   - participant delivery must be switched on (config.NotifyParticipants);
//   - the email channel must be enabled (config.Channels.Email.Enabled);
//   - the instance must actually have SMTP configured (h.emailService.IsConfigured).
//
// Parse failure is treated as "not enabled" — a corrupt config must not silently
// open a channel the owner never opened. VerifyEmail deliberately skips this
// gate so an already-sent verification link still works after settings change.
func (h *ParticipantEmailHandler) participantEmailEnabled(calendar *models.Calendar) bool {
	if calendar.NotifyConfig == nil || *calendar.NotifyConfig == "" {
		return false
	}

	var cfg notifyModels.NotifyConfig
	if err := json.Unmarshal([]byte(*calendar.NotifyConfig), &cfg); err != nil {
		return false
	}

	return cfg.Enabled &&
		cfg.NotifyParticipants &&
		cfg.Channels.Email.Enabled &&
		h.emailService.IsConfigured()
}

// VerifyEmail verifies participant email with token
//
//	@Summary		Verify participant email
//	@Description	Verifies a participant's email address using the verification token
//	@Tags			Notifications
//	@Produce		json
//	@Param			token	path		string	true	"Verification token"
//	@Success		200		{object}	models.ParticipantEmailMessageResponse
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

	httputil.JSON(w, http.StatusOK, models.ParticipantEmailMessageResponse{
		Message: "Email verified successfully",
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
//	@Success		200		{object}	models.ParticipantEmailMessageResponse
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

	// Same consent gate as adding an address: an owner who turned participant
	// mail off, or an instance without SMTP, must not be used as a relaying UI.
	if !h.participantEmailEnabled(calendar) {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "This calendar does not accept participant email addresses")
		return
	}

	// Resend verification, in the participant's own language.
	if err := h.emailService.ResendVerification(ctx, pid, participant.Locale); err != nil {
		h.logger.Error("Failed to resend verification",
			"participant_ref", pkglog.Fingerprint(pid.String()), "error", err)
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, models.ParticipantEmailMessageResponse{
		Message: "Verification email resent",
	})
}
