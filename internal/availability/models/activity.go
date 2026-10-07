// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"
)

// ParticipantRef names one participant and when they did the thing that was journalled.
//
// The name is never stored alongside the id — it is joined when the row is read. That is
// what makes deleting a participant enough to forget them: the id becomes NULL, the join
// finds nothing, and the entry stops existing without leaving an orphan behind.
type ParticipantRef struct {
	ID   uuid.UUID `json:"participant_id"`
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

// DateActivity is the whole of what the journal remembers about one calendar date:
// who joined last, and who withdrew last while the date had already reached its
// threshold. Each is overwritten by the next of its own kind, and the two are
// independent — neither erases the other.
//
// Either field is nil when there is no entry, which is also how a deleted participant
// reads. A nil ref carries no timestamp, so the stranded last_joined_at of a forgotten
// participant is never exposed.
type DateActivity struct {
	Date          time.Time
	LastJoined    *ParticipantRef
	LastWithdrawn *ParticipantRef
}

// DateActivityEntry is one row of the owner-facing activity endpoint.
//
// Available is deliberately not part of DateActivity: it is not journalled at all, but
// derived from the live occurrence expansion when this is built, so the journal can
// never hold a second, staler answer to a question the calendar already answers.
type DateActivityEntry struct {
	// Date is "2006-01-02".
	Date          string                 `json:"date"`
	Threshold     int                    `json:"threshold"`
	Count         int                    `json:"count"`
	Available     []AvailableParticipant `json:"available"`
	LastJoined    *ParticipantRef        `json:"last_joined"`
	LastWithdrawn *ParticipantRef        `json:"last_withdrawn"`
}

// DateActivityResponse is the activity endpoint's payload.
type DateActivityResponse struct {
	Activity []DateActivityEntry `json:"activity"`
}
