// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/availability/models"
	calendarModels "github.com/whento/whento/internal/calendar/models"
)

// ActivityCalendarStore is the slice of the calendar repository this handler needs:
// one row, read to answer "does the caller own this calendar" before anything else.
//
// Declared here on the consuming side, naming only a models type, so the handler can be
// exercised without a database — which is what the privacy tests beside this file do.
type ActivityCalendarStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*calendarModels.Calendar, error)
}

// ActivityService is the read the handler serves.
type ActivityService interface {
	GetDateActivity(
		ctx context.Context,
		calendarID uuid.UUID,
		threshold, minDurationHours int,
		startDateStr, endDateStr string,
	) (*models.DateActivityResponse, error)
}

// ActivityHandler serves the per-date activity journal to the calendar owner.
//
// There is deliberately no public and no token-addressed counterpart. A participant
// token is a participant capability (ADR-0002) — it says "I am this person on this
// calendar", not "I administer it" — and this endpoint answers a question about
// everybody else.
type ActivityHandler struct {
	calendarRepo    ActivityCalendarStore
	activityService ActivityService
}

// NewActivityHandler creates a new activity handler
func NewActivityHandler(calendarRepo ActivityCalendarStore, activityService ActivityService) *ActivityHandler {
	return &ActivityHandler{calendarRepo: calendarRepo, activityService: activityService}
}

// GetActivity returns the activity journal for a calendar over a date range
//
//	@Summary		Get the per-date activity journal
//	@Description	Returns, for each date of the range with a journal entry or any availability, who is currently available, who joined last and who withdrew last while the date had reached its threshold (owner only). The range must not exceed 92 days.
//	@Tags			Availabilities
//	@Security		BearerAuth
//	@Produce		json
//	@Param			id		path		string	true	"Calendar ID"
//	@Param			start	query		string	true	"Range start (YYYY-MM-DD)"
//	@Param			end		query		string	true	"Range end (YYYY-MM-DD)"
//	@Success		200		{object}	models.DateActivityResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid calendar ID or date range"
//	@Failure		401		{object}	httputil.ErrorResponse	"Not authenticated"
//	@Failure		403		{object}	httputil.ErrorResponse	"Not the calendar owner"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar not found"
//	@Failure		500		{object}	httputil.ErrorResponse
//	@Router			/api/v1/calendars/{id}/activity [get]
func (h *ActivityHandler) GetActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	calendarID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid calendar ID")
		return
	}

	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "start and end are required")
		return
	}

	// Ownership, exactly as the notification config endpoints check it: a calendar
	// nobody can read is a 404, one somebody else owns is a 403.
	calendar, err := h.calendarRepo.GetByID(ctx, calendarID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
		return
	}

	userID, _ := uuid.Parse(middleware.GetUserID(ctx))
	if calendar.OwnerID != userID {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't own this calendar")
		return
	}

	// The calendar is already in hand, so the threshold the response reports and the
	// minimum duration the count depends on are passed down rather than read again.
	//
	// Names are returned here even when lock_participants is set. That setting hides
	// participants from each other in the public view; it was never a promise to the
	// owner, who reads every name on their own settings page. This route is owner-only
	// and behind middleware.Auth — see activity_handler_privacy_test.go.
	activity, err := h.activityService.GetDateActivity(
		ctx, calendarID, calendar.Threshold, calendar.MinDurationHours, start, end,
	)
	if err != nil {
		handleAvailabilityError(w, r, err, "Failed to get the activity journal")
		return
	}

	httputil.JSON(w, http.StatusOK, activity)
}
