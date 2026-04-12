// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/ics/repository"
)

var (
	ErrFeedNotFound         = errors.New("unified feed not found")
	ErrFeedAlreadyExists    = errors.New("unified feed already exists")
	ErrInvalidCalendarOwner = errors.New("one or more calendars do not belong to the user")
)

// UnifiedFeedConfigRepository defines the interface for unified feed config operations
type UnifiedFeedConfigRepository interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*repository.UnifiedFeed, []uuid.UUID, error)
	Create(ctx context.Context, userID uuid.UUID) (*repository.UnifiedFeed, error)
	UpdateCalendars(ctx context.Context, feedID uuid.UUID, calendarIDs []uuid.UUID) error
	RegenerateToken(ctx context.Context, feedID uuid.UUID) (string, error)
	ValidateCalendarOwnership(ctx context.Context, userID uuid.UUID, calendarIDs []uuid.UUID) (bool, error)
}

// UnifiedFeedConfig represents the configuration returned to the frontend
type UnifiedFeedConfig struct {
	Configured          bool      `json:"configured"`
	ICSToken            string    `json:"ics_token,omitempty"`
	IncludedCalendarIDs []string  `json:"included_calendar_ids"`
}

type UnifiedFeedConfigService struct {
	feedRepo UnifiedFeedConfigRepository
}

func NewUnifiedFeedConfigService(feedRepo UnifiedFeedConfigRepository) *UnifiedFeedConfigService {
	return &UnifiedFeedConfigService{
		feedRepo: feedRepo,
	}
}

// GetConfig returns the unified feed configuration for a user
func (s *UnifiedFeedConfigService) GetConfig(ctx context.Context, userID uuid.UUID) (*UnifiedFeedConfig, error) {
	feed, calendarIDs, err := s.feedRepo.GetByUserID(ctx, userID)
	if err != nil {
		// If no feed exists, return unconfigured state
		return &UnifiedFeedConfig{
			Configured:          false,
			IncludedCalendarIDs: []string{},
		}, nil
	}

	ids := make([]string, len(calendarIDs))
	for i, id := range calendarIDs {
		ids[i] = id.String()
	}

	return &UnifiedFeedConfig{
		Configured:          true,
		ICSToken:            feed.ICSToken,
		IncludedCalendarIDs: ids,
	}, nil
}

// CreateFeed creates a new unified feed for a user
func (s *UnifiedFeedConfigService) CreateFeed(ctx context.Context, userID uuid.UUID) (*UnifiedFeedConfig, error) {
	// Check if feed already exists
	existing, _, _ := s.feedRepo.GetByUserID(ctx, userID)
	if existing != nil {
		return nil, ErrFeedAlreadyExists
	}

	feed, err := s.feedRepo.Create(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create unified feed: %w", err)
	}

	return &UnifiedFeedConfig{
		Configured:          true,
		ICSToken:            feed.ICSToken,
		IncludedCalendarIDs: []string{},
	}, nil
}

// UpdateCalendars updates which calendars are included in the unified feed
func (s *UnifiedFeedConfigService) UpdateCalendars(ctx context.Context, userID uuid.UUID, calendarIDStrings []string) error {
	feed, _, err := s.feedRepo.GetByUserID(ctx, userID)
	if err != nil {
		return ErrFeedNotFound
	}

	// Parse calendar IDs
	calendarIDs := make([]uuid.UUID, 0, len(calendarIDStrings))
	for _, idStr := range calendarIDStrings {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return fmt.Errorf("invalid calendar ID: %s", idStr)
		}
		calendarIDs = append(calendarIDs, id)
	}

	// Validate ownership
	if len(calendarIDs) > 0 {
		valid, err := s.feedRepo.ValidateCalendarOwnership(ctx, userID, calendarIDs)
		if err != nil {
			return fmt.Errorf("failed to validate ownership: %w", err)
		}
		if !valid {
			return ErrInvalidCalendarOwner
		}
	}

	return s.feedRepo.UpdateCalendars(ctx, feed.ID, calendarIDs)
}

// RegenerateToken regenerates the ICS token for a user's unified feed
func (s *UnifiedFeedConfigService) RegenerateToken(ctx context.Context, userID uuid.UUID) (string, error) {
	feed, _, err := s.feedRepo.GetByUserID(ctx, userID)
	if err != nil {
		return "", ErrFeedNotFound
	}

	return s.feedRepo.RegenerateToken(ctx, feed.ID)
}
