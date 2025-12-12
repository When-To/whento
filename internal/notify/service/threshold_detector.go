// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/availability/repository"
	"github.com/whento/whento/internal/notify/models"
)

// ThresholdDetector handles threshold detection logic
type ThresholdDetector struct {
	availabilityRepo *repository.AvailabilityRepository
	logger           *slog.Logger
}

// NewThresholdDetector creates a new threshold detector
func NewThresholdDetector(availabilityRepo *repository.AvailabilityRepository, logger *slog.Logger) *ThresholdDetector {
	return &ThresholdDetector{
		availabilityRepo: availabilityRepo,
		logger:           logger,
	}
}

// DetectTransition compares participant count vs threshold to detect transitions
func (d *ThresholdDetector) DetectTransition(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
	threshold int,
	previousCount int, // Pass -1 if unknown (will only check current state)
) (*models.ThresholdTransition, error) {
	d.logger.Debug("DetectTransition called",
		"calendar_id", calendarID,
		"date", date.Format("2006-01-02"),
		"threshold", threshold,
		"previous_count", previousCount)

	// Get current participant count for this date
	newCount, err := d.availabilityRepo.GetParticipantCountForDate(ctx, calendarID, date)
	if err != nil {
		d.logger.Error("Failed to get participant count", "calendar_id", calendarID, "date", date, "error", err)
		return nil, err
	}

	d.logger.Debug("Current participant count retrieved",
		"calendar_id", calendarID,
		"new_count", newCount)

	transition := &models.ThresholdTransition{
		CalendarID:    calendarID,
		Date:          date,
		PreviousCount: previousCount,
		NewCount:      newCount,
		Threshold:     threshold,
	}

	// Determine transition type
	if previousCount >= 0 {
		// We know the previous count, can detect transitions
		wasMet := previousCount >= threshold
		nowMet := newCount >= threshold

		d.logger.Debug("Transition detection with previous count",
			"was_met", wasMet,
			"now_met", nowMet,
			"previous", previousCount,
			"new", newCount,
			"threshold", threshold)

		if !wasMet && nowMet {
			transition.TransitionType = "threshold_reached"
			d.logger.Info("THRESHOLD REACHED TRANSITION DETECTED",
				"calendar_id", calendarID,
				"date", date.Format("2006-01-02"),
				"previous", previousCount,
				"new", newCount,
				"threshold", threshold)
		} else if wasMet && !nowMet {
			transition.TransitionType = "threshold_lost"
			d.logger.Info("THRESHOLD LOST TRANSITION DETECTED",
				"calendar_id", calendarID,
				"date", date.Format("2006-01-02"),
				"previous", previousCount,
				"new", newCount,
				"threshold", threshold)
		} else {
			transition.TransitionType = "none"
			d.logger.Debug("No transition detected (threshold state unchanged)")
		}
	} else {
		// Previous count unknown, just check current state
		d.logger.Debug("Transition detection without previous count", "new", newCount, "threshold", threshold)

		if newCount >= threshold {
			transition.TransitionType = "threshold_reached"
			d.logger.Info("THRESHOLD REACHED (no previous count)",
				"calendar_id", calendarID,
				"date", date.Format("2006-01-02"),
				"new", newCount,
				"threshold", threshold)
		} else {
			transition.TransitionType = "none"
			d.logger.Debug("Threshold not met (no previous count)", "new", newCount, "threshold", threshold)
		}
	}

	d.logger.Debug("Transition detection result", "type", transition.TransitionType)
	return transition, nil
}

// GetCurrentCount gets the current participant count for a date
func (d *ThresholdDetector) GetCurrentCount(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
) (int, error) {
	return d.availabilityRepo.GetParticipantCountForDate(ctx, calendarID, date)
}
