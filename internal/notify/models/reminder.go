// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"
)

// Reminder job states.
const (
	ReminderJobPending  = "pending"
	ReminderJobSent     = "sent"
	ReminderJobFailed   = "failed"
	ReminderJobCanceled = "canceled"
)

// ReminderRecipientType identifies who a reminder job reaches. Participant
// reminders are fanned out to the verified, available participants at delivery
// time, so one job covers them all.
const (
	ReminderRecipientOwner        = "owner"
	ReminderRecipientParticipants = "participants"
)

// ReminderJob is one scheduled delivery of one reminder: a channel, a recipient
// class, and an event date. Rows live in the reminder_jobs table, which is what
// makes reminders survive redeploys, get claimed by exactly one instance at a
// time, and get retried with backoff.
type ReminderJob struct {
	ID            uuid.UUID
	CalendarID    uuid.UUID
	EventDate     time.Time
	RecipientType string // owner | participants
	Channel       string // email | discord | slack | telegram
	ScheduledAt   time.Time
	Status        string
	Attempt       int
	MaxAttempts   int
	NextAttemptAt time.Time
	LastError     string
	LockedBy      string
}
