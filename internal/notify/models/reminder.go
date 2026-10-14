// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Reminder job states.
const (
	ReminderJobPending = "pending"
	ReminderJobSent    = "sent"
	ReminderJobFailed  = "failed"
)

// ReminderRecipientType identifies who a reminder job reaches. Participant
// reminders are fanned out to the verified, available participants at delivery
// time, so one job covers them all: recipient_type is the owner *or* the
// participant group, never one row per address.
const (
	ReminderRecipientOwner        = "owner"
	ReminderRecipientParticipants = "participants"
)

// ErrClaimLost is returned by the reminder job store when a fenced worker
// operation did not match the job's current claim. It is a recognizable no-op:
// the caller is not authorized to retry, and it is never a reason to cancel the
// event. The reminder scheduler logs it and moves on.
var ErrClaimLost = errors.New("reminder job claim lost")

// ReminderJob is one scheduled delivery of one reminder: a channel, a recipient
// class, and an event date. Rows live in the reminder_jobs table, which is what
// makes reminders survive redeploys, get claimed by exactly one delivery at a
// time (a UUID claim token, not an instance id — a single instance can claim
// twice across a lease expiry), and get retried with backoff.
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
	// ClaimToken is the UUID ClaimDue hands the worker that acquired the job,
	// and the fence every subsequent worker side-effect must present.
	ClaimToken uuid.UUID
	// LeaseUntil is when this acquisition's claim expires; after that another
	// delivery may claim the job again.
	LeaseUntil time.Time
}
