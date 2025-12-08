// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/whento/pkg/models"
)

// TimeRange represents a time range with min and max times
type TimeRange struct {
	MinTime string `json:"min_time,omitempty"`
	MaxTime string `json:"max_time,omitempty"`
}

// Calendar represents a calendar with availability tracking
type Calendar struct {
	models.TimestampedEntity
	OwnerID           uuid.UUID  `json:"owner_id"`
	Name              string     `json:"name"`
	Description       string     `json:"description,omitempty"`
	PublicToken       string     `json:"public_token"`
	ICSToken          string     `json:"ics_token"`
	Threshold         int        `json:"threshold"`
	AllowedWeekdays   []int      `json:"allowed_weekdays"`
	MinDurationHours  int        `json:"min_duration_hours"`
	Timezone          string     `json:"timezone"`
	HolidaysPolicy    string     `json:"holidays_policy"`
	AllowHolidayEves  bool       `json:"allow_holiday_eves"`
	AllowedHours      *string    `json:"allowed_hours,omitempty"` // JSONB stored as nullable string
	NotifyOnThreshold bool       `json:"notify_on_threshold"`
	NotifyConfig      *string    `json:"notify_config,omitempty"` // JSONB stored as nullable string
	LockParticipants  bool       `json:"lock_participants"`
	StartDate         *time.Time `json:"start_date,omitempty"`
	EndDate           *time.Time `json:"end_date,omitempty"`
}

// Participant represents a participant in a calendar
type Participant struct {
	models.Entity
	CalendarID uuid.UUID `json:"calendar_id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
}

// PublicParticipant represents a participant in a public calendar response
// The ID field is nullable to support masking when lock_participants is enabled
type PublicParticipant struct {
	ID         *uuid.UUID `json:"id,omitempty"`
	CalendarID uuid.UUID  `json:"calendar_id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateCalendarRequest represents a request to create a calendar
type CreateCalendarRequest struct {
	Name              string               `json:"name" validate:"required,min=2,max=200"`
	Description       string               `json:"description,omitempty" validate:"max=1000"`
	Threshold         int                  `json:"threshold,omitempty" validate:"omitempty,min=1"`
	AllowedWeekdays   []int                `json:"allowed_weekdays,omitempty" validate:"omitempty,dive,min=0,max=6"`
	MinDurationHours  int                  `json:"min_duration_hours,omitempty" validate:"omitempty,min=0"`
	Timezone          string               `json:"timezone,omitempty" validate:"omitempty"`
	HolidaysPolicy    string               `json:"holidays_policy,omitempty" validate:"omitempty,oneof=ignore allow block" enums:"ignore,allow,block"`
	AllowHolidayEves  bool                 `json:"allow_holiday_eves,omitempty"`
	WeekdayTimes      map[string]TimeRange `json:"weekday_times,omitempty"`
	HolidayMinTime    string               `json:"holiday_min_time,omitempty"`
	HolidayMaxTime    string               `json:"holiday_max_time,omitempty"`
	HolidayEveMinTime string               `json:"holiday_eve_min_time,omitempty"`
	HolidayEveMaxTime string               `json:"holiday_eve_max_time,omitempty"`
	NotifyOnThreshold bool                 `json:"notify_on_threshold,omitempty"`
	LockParticipants  bool                 `json:"lock_participants,omitempty"`
	StartDate         string               `json:"start_date,omitempty"`
	EndDate           string               `json:"end_date,omitempty"`
	Participants      []string             `json:"participants,omitempty" validate:"omitempty,dive,min=1,max=100"`
}

// UpdateCalendarRequest represents a request to update a calendar
type UpdateCalendarRequest struct {
	Name              *string              `json:"name,omitempty" validate:"omitempty,min=2,max=200"`
	Description       *string              `json:"description,omitempty" validate:"omitempty,max=1000"`
	Threshold         *int                 `json:"threshold,omitempty" validate:"omitempty,min=1"`
	AllowedWeekdays   []int                `json:"allowed_weekdays,omitempty" validate:"omitempty,dive,min=0,max=6"`
	MinDurationHours  *int                 `json:"min_duration_hours,omitempty" validate:"omitempty,min=0"`
	Timezone          *string              `json:"timezone,omitempty" validate:"omitempty"`
	HolidaysPolicy    *string              `json:"holidays_policy,omitempty" validate:"omitempty,oneof=ignore allow block" enums:"ignore,allow,block"`
	AllowHolidayEves  *bool                `json:"allow_holiday_eves,omitempty"`
	WeekdayTimes      map[string]TimeRange `json:"weekday_times,omitempty"`
	HolidayMinTime    *string              `json:"holiday_min_time,omitempty"`
	HolidayMaxTime    *string              `json:"holiday_max_time,omitempty"`
	HolidayEveMinTime *string              `json:"holiday_eve_min_time,omitempty"`
	HolidayEveMaxTime *string              `json:"holiday_eve_max_time,omitempty"`
	NotifyOnThreshold *bool                `json:"notify_on_threshold,omitempty"`
	LockParticipants  *bool                `json:"lock_participants,omitempty"`
	StartDate         *string              `json:"start_date,omitempty"`
	EndDate           *string              `json:"end_date,omitempty"`
}

// AddParticipantRequest represents a request to add a participant
type AddParticipantRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

// UpdateParticipantRequest represents a request to update a participant
type UpdateParticipantRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

// RegenerateTokenRequest represents a request to regenerate a token
type RegenerateTokenRequest struct {
	TokenType string `json:"token_type" validate:"required,oneof=public ics"`
}

// CalendarResponse represents the response when returning a calendar
type CalendarResponse struct {
	ID                uuid.UUID            `json:"id"`
	OwnerID           uuid.UUID            `json:"owner_id"`
	Name              string               `json:"name"`
	Description       string               `json:"description,omitempty"`
	PublicToken       string               `json:"public_token"`
	ICSToken          string               `json:"ics_token"`
	Threshold         int                  `json:"threshold"`
	AllowedWeekdays   []int                `json:"allowed_weekdays"`
	MinDurationHours  int                  `json:"min_duration_hours"`
	Timezone          string               `json:"timezone"`
	HolidaysPolicy    string               `json:"holidays_policy" enums:"ignore,allow,block"`
	AllowHolidayEves  bool                 `json:"allow_holiday_eves"`
	WeekdayTimes      map[string]TimeRange `json:"weekday_times,omitempty"`
	HolidayMinTime    string               `json:"holiday_min_time,omitempty"`
	HolidayMaxTime    string               `json:"holiday_max_time,omitempty"`
	HolidayEveMinTime string               `json:"holiday_eve_min_time,omitempty"`
	HolidayEveMaxTime string               `json:"holiday_eve_max_time,omitempty"`
	NotifyOnThreshold bool                 `json:"notify_on_threshold"`
	LockParticipants  bool                 `json:"lock_participants"`
	StartDate         *time.Time           `json:"start_date,omitempty"`
	EndDate           *time.Time           `json:"end_date,omitempty"`
	Participants      []Participant        `json:"participants,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}

// PublicCalendarResponse represents the public view of a calendar
type PublicCalendarResponse struct {
	ID                uuid.UUID            `json:"id"`
	Name              string               `json:"name"`
	Description       string               `json:"description,omitempty"`
	Threshold         int                  `json:"threshold"`
	AllowedWeekdays   []int                `json:"allowed_weekdays"`
	MinDurationHours  int                  `json:"min_duration_hours"`
	Timezone          string               `json:"timezone"`
	HolidaysPolicy    string               `json:"holidays_policy" enums:"ignore,allow,block"`
	AllowHolidayEves  bool                 `json:"allow_holiday_eves"`
	WeekdayTimes      map[string]TimeRange `json:"weekday_times,omitempty"`
	HolidayMinTime    string               `json:"holiday_min_time,omitempty"`
	HolidayMaxTime    string               `json:"holiday_max_time,omitempty"`
	HolidayEveMinTime string               `json:"holiday_eve_min_time,omitempty"`
	HolidayEveMaxTime string               `json:"holiday_eve_max_time,omitempty"`
	LockParticipants  bool                 `json:"lock_participants"`
	ICSToken          string               `json:"ics_token"`
	StartDate         *time.Time           `json:"start_date,omitempty"`
	EndDate           *time.Time           `json:"end_date,omitempty"`
	Participants      []PublicParticipant  `json:"participants"`
	CreatedAt         time.Time            `json:"created_at"`
}
